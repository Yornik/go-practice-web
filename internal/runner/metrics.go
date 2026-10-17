package runner

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics are the runner's Prometheus collectors. They are registered on a
// registry the caller owns, so tests can build as many servers as they like.
type Metrics struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	wait     prometheus.Histogram
	queued   prometheus.Gauge
	running  prometheus.Gauge
	strays   prometheus.Counter
}

// NewMetrics creates the collectors and registers them on reg.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "runner",
			Name:      "requests_total",
			Help:      "Runner requests by kind (check, format, test) and result.",
		}, []string{"kind", "result"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "runner",
			Name:      "request_duration_seconds",
			Help:      "Time from the start of the work to the response, without the queue wait, by kind and result.",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 4, 8, 15, 30, 60},
		}, []string{"kind", "result"}),
		wait: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "runner",
			Name:      "queue_wait_seconds",
			Help:      "Time a request waited for a free slot.",
			Buckets:   []float64{0.001, 0.01, 0.1, 0.5, 1, 2, 5, 10, 30, 60},
		}),
		queued: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "runner",
			Name:      "queue_length",
			Help:      "Requests waiting for a free slot.",
		}),
		running: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "runner",
			Name:      "running",
			Help:      "go invocations in progress.",
		}),
		strays: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "runner",
			Name:      "stray_processes_killed_total",
			Help:      "Processes killed because they left their run's session or outlived the run.",
		}),
	}
	reg.MustRegister(m.requests, m.duration, m.wait, m.queued, m.running, m.strays)
	return m
}

func (m *Metrics) observe(kind, result string, d time.Duration) {
	m.requests.WithLabelValues(kind, result).Inc()
	m.duration.WithLabelValues(kind, result).Observe(d.Seconds())
}

func (m *Metrics) limiterChanged(running, queued int) {
	m.running.Set(float64(running))
	m.queued.Set(float64(queued))
}
