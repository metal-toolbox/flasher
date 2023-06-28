package metrics

import (
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	MetricsEndpoint = "0.0.0.0:9090"
)

var (
	EventsCounter *prometheus.CounterVec

	ConditionRunTimeSummary     *prometheus.SummaryVec
	ActionRuntimeSummary        *prometheus.SummaryVec
	ActionHandlerRunTimeSummary *prometheus.SummaryVec

	DownloadBytes          *prometheus.CounterVec
	DownloadRunTimeSummary *prometheus.SummaryVec
	UploadBytes            *prometheus.CounterVec
	UploadRunTimeSummary   *prometheus.SummaryVec

	StoreQueryErrorCount *prometheus.CounterVec
)

func init() {
	EventsCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "flasher_events_received",
			Help: "A counter metric to measure the total count of events received",
		},
		[]string{"valid", "response"}, // valid is true/false, response is ack/nack
	)

	ConditionRunTimeSummary = promauto.NewSummaryVec(
		prometheus.SummaryOpts{
			Name: "flasher_condition_duration_seconds",
			Help: "A summary metric to measure the total time spent in completing each condition",
		},
		[]string{"condition", "state"},
	)

	ActionRuntimeSummary = promauto.NewSummaryVec(
		prometheus.SummaryOpts{
			Name: "flasher_install_action_runtime_seconds",
			Help: "A summary metric to measure the total time spent in each install action",
		},
		[]string{"vendor", "component", "state"},
	)

	ActionHandlerRunTimeSummary = promauto.NewSummaryVec(
		prometheus.SummaryOpts{
			Name: "flasher_install_action_handler_seconds",
			Help: "A summary metric to measure the total time spent in each install action handler being executed",
		},
		[]string{"transition", "vendor", "component", "state"},
	)

	DownloadBytes = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "flasher_download_bytes",
			Help: "A counter metric to measure firmware downloaded in bytes",
		},
		[]string{"component", "vendor"},
	)

	UploadBytes = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "flasher_upload_bytes",
			Help: "A counter metric to measure firmware uploaded in bytes",
		},
		[]string{"component", "vendor"},
	)

	StoreQueryErrorCount = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "flasher_store_query_error_count",
			Help: "A counter metric to measure the total count of errors querying the asset store.",
		},
		[]string{"storeKind", "queryKind"},
	)
}

// ListenAndServeMetrics exposes prometheus metrics as /metrics
func ListenAndServe() {
	go func() {
		http.Handle("/metrics", promhttp.Handler())

		server := &http.Server{
			Addr:              MetricsEndpoint,
			ReadHeaderTimeout: 2 * time.Second, // nolint:gomnd // time duration value is clear as is.
		}

		if err := server.ListenAndServe(); err != nil {
			log.Println(err)
		}
	}()
}
