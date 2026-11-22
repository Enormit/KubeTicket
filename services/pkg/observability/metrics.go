package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds RED Method HTTP metrics and KubeTicket domain gauges/counters.
type Metrics struct {
	Registry            *prometheus.Registry
	HTTPRequestsTotal   *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec
	WSActiveConnections *prometheus.GaugeVec
	SeatLockConflicts   *prometheus.CounterVec
	QueueProcessedTotal *prometheus.CounterVec
}

// NewMetrics initializes an isolated Prometheus registry for a service.
func NewMetrics(serviceName string) *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	constLabels := prometheus.Labels{"service": serviceName}

	m := &Metrics{
		Registry: reg,
		HTTPRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "http_requests_total",
			Help:        "Total HTTP requests processed.",
			ConstLabels: constLabels,
		}, []string{"method", "path", "status"}),
		HTTPRequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:        "http_request_duration_seconds",
			Help:        "HTTP request latency histogram.",
			ConstLabels: constLabels,
			Buckets:     []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		}, []string{"method", "path"}),
		WSActiveConnections: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "ws_active_connections",
			Help:        "Current active WebSocket connections by event.",
			ConstLabels: constLabels,
		}, []string{"event_id"}),
		SeatLockConflicts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "seat_lock_conflicts_total",
			Help:        "Total seat lock conflict rejections.",
			ConstLabels: constLabels,
		}, []string{"event_id"}),
		QueueProcessedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "queue_messages_processed_total",
			Help:        "Total queue messages processed by status.",
			ConstLabels: constLabels,
		}, []string{"queue", "status"}),
	}

	reg.MustRegister(
		m.HTTPRequestsTotal,
		m.HTTPRequestDuration,
		m.WSActiveConnections,
		m.SeatLockConflicts,
		m.QueueProcessedTotal,
	)

	return m
}

// Handler returns the Prometheus /metrics HTTP handler.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.status = code
	sr.ResponseWriter.WriteHeader(code)
}

// HTTPMetricsMiddleware records RED metrics (Rate, Errors, Duration) per request.
func (m *Metrics) HTTPMetricsMiddleware(routeLabel func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)

			path := r.URL.Path
			if routeLabel != nil {
				if lbl := routeLabel(r); lbl != "" {
					path = lbl
				}
			}

			m.HTTPRequestsTotal.WithLabelValues(r.Method, path, strconv.Itoa(rec.status)).Inc()
			m.HTTPRequestDuration.WithLabelValues(r.Method, path).Observe(time.Since(start).Seconds())
		})
	}
}
