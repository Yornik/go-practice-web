package web

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/yornik/go-practice-web/internal/store"
)

// metrics are the web server's Prometheus metrics (docs/DESIGN.md,
// "Observability").
type metrics struct {
	httpRequests   *prometheus.CounterVec
	httpDuration   *prometheus.HistogramVec
	runnerRequests *prometheus.CounterVec
	runnerDuration *prometheus.HistogramVec
}

func newMetrics(reg prometheus.Registerer) (*metrics, error) {
	m := &metrics{
		httpRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gpw_http_requests_total",
			Help: "HTTP requests by route pattern, method and status.",
		}, []string{"route", "method", "status"}),
		httpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "gpw_http_request_duration_seconds",
			Help:    "HTTP request duration by route pattern. Test runs stream for as long as the run.",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60},
		}, []string{"route"}),
		runnerRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gpw_runner_requests_total",
			Help: "Calls to the runner by kind (check, format, test) and result (ok or an error code).",
		}, []string{"kind", "result"}),
		runnerDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "gpw_runner_request_duration_seconds",
			Help:    "Duration of calls to the runner, queue wait included.",
			Buckets: []float64{.05, .1, .25, .5, 1, 2.5, 5, 10, 20, 30, 45, 60, 120},
		}, []string{"kind", "result"}),
	}
	if reg == nil {
		return m, nil
	}
	for _, c := range []prometheus.Collector{m.httpRequests, m.httpDuration, m.runnerRequests, m.runnerDuration} {
		if err := reg.Register(c); err != nil {
			return nil, fmt.Errorf("web: register metrics: %w", err)
		}
	}
	return m, nil
}

func (m *metrics) httpRequest(route, method string, status int, d time.Duration) {
	m.httpRequests.WithLabelValues(route, method, strconv.Itoa(status)).Inc()
	m.httpDuration.WithLabelValues(route).Observe(d.Seconds())
}

func (m *metrics) runnerCall(kind, result string, d time.Duration) {
	m.runnerRequests.WithLabelValues(kind, result).Inc()
	m.runnerDuration.WithLabelValues(kind, result).Observe(d.Seconds())
}

// StoreCollector exports the database totals (users, active sessions,
// passed exercises, valid certificates) at scrape time, so the numbers are
// right after a restart and need no bookkeeping in the handlers.
type StoreCollector struct {
	st                                    *store.Store
	users, sessions, passed, certificates *prometheus.Desc
}

// NewStoreCollector returns a collector over st.
func NewStoreCollector(st *store.Store) *StoreCollector {
	return &StoreCollector{
		st:           st,
		users:        prometheus.NewDesc("gpw_users", "Accounts.", nil, nil),
		sessions:     prometheus.NewDesc("gpw_active_sessions", "Unexpired sessions.", nil, nil),
		passed:       prometheus.NewDesc("gpw_passed_exercises", "Passed exercises over all users.", nil, nil),
		certificates: prometheus.NewDesc("gpw_certificates", "Valid certificates.", nil, nil),
	}
}

// Describe implements prometheus.Collector.
func (c *StoreCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.users
	ch <- c.sessions
	ch <- c.passed
	ch <- c.certificates
}

// Collect implements prometheus.Collector.
func (c *StoreCollector) Collect(ch chan<- prometheus.Metric) {
	// A scrape must not hang on a locked database.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	n, err := c.st.Counts(ctx)
	if err != nil {
		for _, d := range []*prometheus.Desc{c.users, c.sessions, c.passed, c.certificates} {
			ch <- prometheus.NewInvalidMetric(d, err)
		}
		return
	}
	ch <- prometheus.MustNewConstMetric(c.users, prometheus.GaugeValue, float64(n.Users))
	ch <- prometheus.MustNewConstMetric(c.sessions, prometheus.GaugeValue, float64(n.ActiveSessions))
	ch <- prometheus.MustNewConstMetric(c.passed, prometheus.GaugeValue, float64(n.Passed))
	ch <- prometheus.MustNewConstMetric(c.certificates, prometheus.GaugeValue, float64(n.Certificates))
}
