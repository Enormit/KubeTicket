package observability

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// InitTracer configures a W3C TraceContext propagator and SDK TracerProvider.
func InitTracer(serviceName string) (*sdktrace.TracerProvider, trace.Tracer) {
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	return tp, tp.Tracer(serviceName)
}

// InjectTraceContext serializes the active span context into a flat string map (e.g. SQS/Stream attributes).
func InjectTraceContext(ctx context.Context) map[string]string {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return map[string]string(carrier)
}

// ExtractTraceContext restores a parent span context from a string map carrier.
func ExtractTraceContext(ctx context.Context, attrs map[string]string) context.Context {
	if len(attrs) == 0 {
		return ctx
	}
	return otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(attrs))
}

// HTTPTracingMiddleware extracts incoming W3C trace headers and opens a server span.
func HTTPTracingMiddleware(tracer trace.Tracer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
			ctx, span := tracer.Start(ctx, r.Method+" "+r.URL.Path)
			defer span.End()

			if sc := span.SpanContext(); sc.IsValid() {
				w.Header().Set("X-Trace-Id", sc.TraceID().String())
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
