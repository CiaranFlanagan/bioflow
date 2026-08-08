// Package metrics exposes pipeline execution metrics in Prometheus format.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// StepDuration is the wall-clock time of each step, including retries.
	// Buckets run from a second to about two hours, since alignment and
	// variant calling routinely take that long.
	StepDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "bioflow_step_duration_seconds",
		Help:    "Wall-clock duration of each pipeline step.",
		Buckets: prometheus.ExponentialBuckets(1, 2, 13),
	}, []string{"step"})

	StepsCompleted = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "bioflow_steps_completed_total",
		Help: "Steps that finished successfully.",
	}, []string{"step"})

	StepsFailed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "bioflow_steps_failed_total",
		Help: "Steps that failed after exhausting retries.",
	}, []string{"step"})

	StepRetries = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "bioflow_step_retries_total",
		Help: "Individual step attempts that failed and were retried.",
	}, []string{"step"})

	// StepsSkipped is the cache hit counter — the headline number for whether
	// content-addressed caching is actually earning its complexity.
	StepsSkipped = promauto.NewCounter(prometheus.CounterOpts{
		Name: "bioflow_steps_skipped_total",
		Help: "Steps skipped because their inputs were unchanged.",
	})
)

// Serve exposes /metrics on addr in the background. It returns the server so
// the caller can shut it down; errors from ListenAndServe are ignored because
// a failed metrics endpoint should never abort a pipeline run.
func Serve(addr string) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	srv := &http.Server{Addr: addr, Handler: mux}
	go func() { _ = srv.ListenAndServe() }()
	return srv
}
