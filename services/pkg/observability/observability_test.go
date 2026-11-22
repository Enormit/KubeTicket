package observability

import (
	"bytes"
	"context"
	"log/slog"
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
