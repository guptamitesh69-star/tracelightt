package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Mitesh0007/tracelight-go/internal/ingestion"
	"github.com/Mitesh0007/tracelight-go/internal/query"
	"github.com/Mitesh0007/tracelight-go/internal/storage"
	"github.com/Mitesh0007/tracelight-go/internal/stream"
	"github.com/Mitesh0007/tracelight-go/internal/worker"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}

func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func withCORS(next http.Handler, allowedCSV string) http.Handler {
	allowed := map[string]bool{}
	for _, o := range strings.Split(allowedCSV, ",") {
		allowed[strings.TrimSpace(o)] = true
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		if allowed[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}

		w.Header().Set(
			"Access-Control-Allow-Methods",
			"GET, POST, OPTIONS",
		)
		w.Header().Set(
			"Access-Control-Allow-Headers",
			"Content-Type",
		)

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	workerCount := getEnvInt("WORKER_COUNT", 4)
	batchSize := getEnvInt("WORKER_BATCH_SIZE", 20)
	flushIntervalMs := getEnvInt("WORKER_FLUSH_INTERVAL_MS", 500)
	channelBuffer := getEnvInt("TRACE_CHANNEL_BUFFER", 1000)

	traceChan := make(chan ingestion.TraceJob, channelBuffer)

	redisAddr := getEnv(
		"REDIS_ADDR",
		"localhost:6379",
	)

	postgresURL := getEnv(
		"POSTGRES_URL",
		"postgres://tracelight:tracelight@localhost:5432/tracelight?sslmode=disable",
	)

	port := getEnv("PORT", "8080")
	allowedOrigins := getEnv("ALLOWED_ORIGINS", "http://localhost:3000")

	var redisOpts *redis.Options
	if redisURL := os.Getenv("REDIS_URL"); redisURL != "" {
		parsed, err := redis.ParseURL(redisURL)
		if err != nil {
			log.Fatalf("invalid REDIS_URL: %v", err)
		}
		redisOpts = parsed
	} else {
		redisOpts = &redis.Options{
			Addr: redisAddr,
		}
	}

	redisClient := redis.NewClient(redisOpts)
	defer redisClient.Close()

	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf(
			"redis connection failed at %s: %v",
			redisOpts.Addr,
			err,
		)
	}

	redisStream := &stream.Redis{
		Client:   redisClient,
		Stream:   "trace-overflow",
		Group:    "trace-workers",
		Consumer: "server-1",
	}

	if err := redisStream.CreateGroup(ctx); err != nil {
		log.Fatal(err)
	}

	postgres, err := storage.NewPostgres(postgresURL)
	if err != nil {
		log.Fatal(err)
	}
	defer postgres.Close()

	if err := postgres.CreateTables(ctx); err != nil {
		log.Fatal(err)
	}

	handler := ingestion.NewTraceHandler(
		traceChan,
		redisStream,
	)

	queryHandler := query.NewHandler(
		postgres,
	)

	redisStream.StartDrain(
		ctx,
		traceChan,
	)

	workerPool := worker.NewPool(
		traceChan,
		postgres,
		redisStream,
		workerCount,
		batchSize,
		time.Duration(flushIntervalMs)*time.Millisecond,
	)
	workerPool.Start(ctx)

	mux := http.NewServeMux()

	mux.HandleFunc(
		"/trace",
		handler.IngestTrace,
	)

	mux.HandleFunc(
		"/traces",
		queryHandler.ListTraces,
	)

	mux.HandleFunc(
		"/traces/",
		queryHandler.GetTrace,
	)

	mux.Handle(
		"/metrics",
		promhttp.Handler(),
	)

	server := &http.Server{
		Addr:    ":" + port,
		Handler: withCORS(mux, allowedOrigins),
		BaseContext: func(
			_ net.Listener,
		) context.Context {
			return ctx
		},
	}

	go func() {
		log.Printf("server listening on :%s", port)

		if err := server.ListenAndServe(); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			log.Printf("server error: %v", err)
			stop()
		}
	}()

	<-ctx.Done()

	log.Println("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf(
			"server shutdown failed: %v",
			err,
		)
	}

	workerPool.Wait()

	log.Println("server stopped")
}