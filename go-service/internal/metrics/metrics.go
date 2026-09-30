package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	TraceEventsAcceptedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "tracelight_trace_events_accepted_total",
			Help: "Total number of trace events accepted into the ingestion queue.",
		},
	)

	TraceEventsOverflowedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "tracelight_trace_events_overflowed_total",
			Help: "Total number of trace events that overflowed the channel and were sent to Redis.",
		},
	)

	TraceQueueDepth = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "tracelight_trace_queue_depth",
			Help: "Current number of trace events waiting in the ingestion queue.",
		},
	)

	TraceEventsProcessedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tracelight_trace_events_processed_total",
			Help: "Total number of trace events processed by workers, partitioned by final status.",
		},
		[]string{"status"},
	)

	TraceEventsRejectedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tracelight_trace_events_rejected_total",
			Help: "Total number of trace events rejected, partitioned by reason.",
		},
		[]string{"reason"},
	)

	TraceProcessingDurationSeconds = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "tracelight_trace_processing_duration_seconds",
			Help:    "Time taken to persist a batch of trace events to storage.",
			Buckets: prometheus.DefBuckets,
		},
	)

	TraceBatchSize = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "tracelight_trace_batch_size",
			Help:    "Number of trace events written per storage flush.",
			Buckets: []float64{1, 2, 5, 10, 20, 50, 100},
		},
	)
)
