package observability

import (
	"bufio"
	"bytes"
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoggerAndTracePropagation(t *testing.T) {
	tp, tracer := InitTracer("test-service")
	defer func() { _ = tp.Shutdown(context.Background()) }()

	ctx, span := tracer.Start(context.Background(), "parent-op")
	defer span.End()

	carrier := InjectTraceContext(ctx)
	if carrier["traceparent"] == "" {
		t.Fatalf("expected non-empty traceparent carrier")
	}

	extracted := ExtractTraceContext(context.Background(), carrier)
	var buf bytes.Buffer
	base := slog.New(slog.NewJSONHandler(&buf, nil))
	WithContext(extracted, base).Info("hello")

	out := buf.String()
	if !strings.Contains(out, span.SpanContext().TraceID().String()) {
		t.Fatalf("expected log output to contain trace_id, got %s", out)
	}
}

func TestMetricsMiddlewareAndEndpoint(t *testing.T) {
	m := NewMetrics("gateway-api")
	handler := m.HTTPMetricsMiddleware(func(r *http.Request) string { return "/api/v1/events" })(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
		}),
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}

	mReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	mRec := httptest.NewRecorder()
	m.Handler().ServeHTTP(mRec, mReq)

	body := mRec.Body.String()
	if !strings.Contains(body, `http_requests_total{method="POST",path="/api/v1/events",service="gateway-api",status="201"} 1`) {
		t.Fatalf("expected counter metric in /metrics output, got:\n%s", body)
	}
}

type hijackRecorder struct {
	http.ResponseWriter
	hijacked bool
}

func (h *hijackRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h.hijacked = true
	return nil, nil, nil
}

func TestMetricsMiddlewareForwardsHijacker(t *testing.T) {
	m := NewMetrics("flashsale-engine")
	hr := &hijackRecorder{ResponseWriter: httptest.NewRecorder()}

	handler := m.HTTPMetricsMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("expected wrapped ResponseWriter to implement http.Hijacker")
		}
		_, _, err := hj.Hijack()
		if err != nil {
			t.Fatalf("unexpected hijack error: %v", err)
		}
	}))

	handler.ServeHTTP(hr, httptest.NewRequest(http.MethodGet, "/ws/events/1", nil))
	if !hr.hijacked {
		t.Fatal("expected underlying Hijack to be called")
	}
}
