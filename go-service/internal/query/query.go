package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Mitesh0007/tracelight-go/internal/ingestion"
)

type TraceReader interface {
	ListTraces(
		ctx context.Context,
	) ([]ingestion.TracePayload, error)

	GetTrace(
		ctx context.Context,
		traceID string,
	) (*ingestion.TracePayload, error)
}

type Handler struct {
	Storage TraceReader
}

func NewHandler(
	storage TraceReader,
) *Handler {
	return &Handler{
		Storage: storage,
	}
}

func (qh *Handler) ListTraces(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodGet {
		http.Error(
			w,
			"method not allowed",
			http.StatusMethodNotAllowed,
		)

		return
	}

	traces, err := qh.Storage.ListTraces(
		r.Context(),
	)
	if err != nil {
		http.Error(
			w,
			"could not fetch traces",
			http.StatusInternalServerError,
		)

		return
	}

	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	json.NewEncoder(w).Encode(traces)
}

func (qh *Handler) GetTrace(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodGet {
		http.Error(
			w,
			"method not allowed",
			http.StatusMethodNotAllowed,
		)

		return
	}

	traceID := strings.TrimPrefix(
		r.URL.Path,
		"/traces/",
	)

	if traceID == "" {
		http.Error(
			w,
			"trace_id is required",
			http.StatusBadRequest,
		)

		return
	}

	trace, err := qh.Storage.GetTrace(
		r.Context(),
		traceID,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(
				w,
				"trace not found",
				http.StatusNotFound,
			)

			return
		}

		http.Error(
			w,
			"could not fetch trace",
			http.StatusInternalServerError,
		)

		return
	}

	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	json.NewEncoder(w).Encode(trace)
}
