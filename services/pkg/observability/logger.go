package observability

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

// NewLogger builds a structured JSON logger scoped to a microservice.
func NewLogger(serviceName, level string) *slog.Logger {
	var slogLevel slog.Level
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		slogLevel = slog.LevelDebug
	case "warn", "warning":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slogLevel,
	})
	return slog.New(handler).With(slog.String("service", serviceName))
}

// WithContext attaches the active OpenTelemetry trace_id and span_id if present.
func WithContext(ctx context.Context, logger *slog.Logger) *slog.Logger {
	if logger == nil {
		logger = slog.Default()
	}
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return logger
	}
	return logger.With(
		slog.String("trace_id", sc.TraceID().String()),
		slog.String("span_id", sc.SpanID().String()),
	)
}
