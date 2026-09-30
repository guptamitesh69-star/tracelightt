package ingestion

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/Mitesh0007/tracelight-go/internal/metrics"
)

type TraceError struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

type TracePayload struct {
	TraceID            string      `json:"trace_id"`
	ProviderResponseID *string     `json:"provider_response_id"`
	Model              string      `json:"model"`
	PromptTokens       *int        `json:"prompt_tokens"`
	CompletionTokens   *int        `json:"completion_tokens"`
	TotalTokens        *int        `json:"total_tokens"`
	DurationMs         int         `json:"duration_ms"`
	Success            bool        `json:"success"`
	Error              *TraceError `json:"error"`
	Timestamp          string      `json:"timestamp"`
}

type RedisOverflowPublisher interface {
	AddTrace(
		ctx context.Context,
		payload TracePayload,
	) (string, error)
}

type TraceJob struct {
	Payload        TracePayload
	RedisMessageID string
	Source         string
}

type TraceHandler struct {
	TraceChan chan<- TraceJob
	Redis     RedisOverflowPublisher
}

func NewTraceHandler(
	traceChan chan<- TraceJob,
	redis RedisOverflowPublisher,
) *TraceHandler {
	return &TraceHandler{
		TraceChan: traceChan,
		Redis:     redis,
	}
}

func (tc *TraceHandler) IngestTrace(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodPost {
		metrics.TraceEventsRejectedTotal.
			WithLabelValues("method_not_allowed").
			Inc()

		http.Error(
			w,
			"method not allowed",
			http.StatusMethodNotAllowed,
		)
		return
	}

	defer r.Body.Close()

	r.Body = http.MaxBytesReader(
		w,
		r.Body,
		1<<20,
	)

	var payload TracePayload

	decoder := json.NewDecoder(r.Body)

	if err := decoder.Decode(&payload); err != nil {
		metrics.TraceEventsRejectedTotal.
			WithLabelValues("invalid_json").
			Inc()

		http.Error(
			w,
			"invalid request payload",
			http.StatusBadRequest,
		)
		return
	}

	job := TraceJob{
		Payload: payload,
		Source:  "channel",
	}

	select {
	case tc.TraceChan <- job:
		metrics.TraceEventsAcceptedTotal.Inc()
		metrics.TraceQueueDepth.Set(
			float64(len(tc.TraceChan)),
		)

		writeJSON(
			w,
			http.StatusAccepted,
			map[string]string{
				"status":   "trace ingested successfully",
				"trace_id": payload.TraceID,
				"source":   "channel",
			},
		)
		return

	default:
		metrics.TraceEventsOverflowedTotal.Inc()
	}

	if tc.Redis == nil {
		metrics.TraceEventsRejectedTotal.
			WithLabelValues("redis_unavailable").
			Inc()

		http.Error(
			w,
			"queue unavailable",
			http.StatusServiceUnavailable,
		)
		return
	}

	_, err := tc.Redis.AddTrace(
		r.Context(),
		payload,
	)
	if err != nil {
		metrics.TraceEventsRejectedTotal.
			WithLabelValues("redis_write_failed").
			Inc()

		http.Error(
			w,
			"could not queue trace",
			http.StatusServiceUnavailable,
		)
		return
	}

	metrics.TraceEventsAcceptedTotal.Inc()

	writeJSON(
		w,
		http.StatusAccepted,
		map[string]string{
			"status":   "trace ingested successfully",
			"trace_id": payload.TraceID,
			"source":   "redis_overflow",
		},
	)
}

func writeJSON(
	w http.ResponseWriter,
	status int,
	value any,
) {
	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(value); err != nil {
		return
	}
}
