package worker

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/Mitesh0007/tracelight-go/internal/ingestion"
	"github.com/Mitesh0007/tracelight-go/internal/metrics"
)

type TraceStorage interface {
	SaveTraces(
		ctx context.Context,
		payloads []ingestion.TracePayload,
	) error
}

type StreamAcker interface {
	Ack(
		ctx context.Context,
		messageID string,
	) error
}

type Pool struct {
	TraceChan     <-chan ingestion.TraceJob
	Storage       TraceStorage
	Stream        StreamAcker
	Workers       int
	BatchSize     int
	FlushInterval time.Duration
	WaitGroup     sync.WaitGroup
}

func NewPool(
	traceChan chan ingestion.TraceJob,
	storage TraceStorage,
	stream StreamAcker,
	workers int,
	batchSize int,
	flushInterval time.Duration,
) *Pool {
	return &Pool{
		TraceChan:     traceChan,
		Storage:       storage,
		Stream:        stream,
		Workers:       workers,
		BatchSize:     batchSize,
		FlushInterval: flushInterval,
	}
}

func (wp *Pool) Start(
	ctx context.Context,
) {
	if wp.Workers <= 0 {
		return
	}

	wp.WaitGroup.Add(wp.Workers)

	for i := 0; i < wp.Workers; i++ {
		go func() {
			defer wp.WaitGroup.Done()

			wp.run(ctx)
		}()
	}
}

func (wp *Pool) Wait() {
	wp.WaitGroup.Wait()
}

func (wp *Pool) run(
	ctx context.Context,
) {
	batch := make(
		[]ingestion.TraceJob,
		0,
		wp.BatchSize,
	)

	ticker := time.NewTicker(wp.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			wp.flush(context.Background(), batch)
			return

		case job, ok := <-wp.TraceChan:
			if !ok {
				wp.flush(context.Background(), batch)
				return
			}

			batch = append(batch, job)

			if len(batch) >= wp.BatchSize {
				wp.flush(ctx, batch)
				batch = batch[:0]
				ticker.Reset(wp.FlushInterval)
			}

		case <-ticker.C:
			if len(batch) > 0 {
				wp.flush(ctx, batch)
				batch = batch[:0]
			}
		}
	}
}

func (wp *Pool) flush(
	ctx context.Context,
	batch []ingestion.TraceJob,
) {
	if len(batch) == 0 {
		return
	}

	payloads := make(
		[]ingestion.TracePayload,
		0,
		len(batch),
	)

	for _, job := range batch {
		payloads = append(payloads, job.Payload)
	}

	start := time.Now()
	err := wp.Storage.SaveTraces(ctx, payloads)
	metrics.TraceProcessingDurationSeconds.Observe(time.Since(start).Seconds())
	metrics.TraceBatchSize.Observe(float64(len(batch)))

	if err != nil {
		metrics.TraceEventsRejectedTotal.
			WithLabelValues("storage_write_failed").
			Add(float64(len(batch)))

		log.Printf(
			"batch write failed for %d trace(s): %v",
			len(batch),
			err,
		)

		return
	}

	metrics.TraceEventsProcessedTotal.
		WithLabelValues("success").
		Add(float64(len(batch)))

	wp.ackRedisSourced(ctx, batch)
}

func (wp *Pool) ackRedisSourced(
	ctx context.Context,
	batch []ingestion.TraceJob,
) {
	if wp.Stream == nil {
		return
	}

	for _, job := range batch {
		if job.Source != "redis_overflow" {
			continue
		}

		if err := wp.Stream.Ack(
			ctx,
			job.RedisMessageID,
		); err != nil {
			metrics.TraceEventsRejectedTotal.
				WithLabelValues("redis_ack_failed").
				Inc()

			log.Printf(
				"redis acknowledgement failed for message %s: %v",
				job.RedisMessageID,
				err,
			)
		}
	}
}
