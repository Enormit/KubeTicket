package consumer

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Enormit/kubeticket/services/pkg/models"
	"github.com/Enormit/kubeticket/services/pkg/observability"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/trace"
)

func TestRedisStreamConsumerProcessesAndAcks(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	tp, tracer := observability.InitTracer("consumer-test")
	defer func() { _ = tp.Shutdown(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	txCtx, span := tracer.Start(ctx, "checkout")
	traceAttrs := observability.InjectTraceContext(txCtx)
	span.End()

	msg := models.OrderQueueMessage{
		OrderID:     "ord-1",
		UserID:      "u-1",
		EventID:     "ev-1",
		HoldToken:   "hold-1",
		SeatIDs:     []string{"s-1"},
		TraceParent: traceAttrs["traceparent"],
		RequestedAt: time.Now().UTC(),
	}
	raw, _ := json.Marshal(msg)
	_ = rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: models.RedisOrderStreamKey(),
		Values: map[string]any{"payload": string(raw)},
	}).Err()

	c := NewRedisStreamConsumer(rdb, nil, observability.NewMetrics("worker-test"))
	gotTrace := make(chan string, 1)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = c.Run(ctx, func(mCtx context.Context, m models.OrderQueueMessage) error {
			sc := trace.SpanContextFromContext(mCtx)
			gotTrace <- sc.TraceID().String()
			cancel()
			return nil
		})
	}()

	select {
	case tid := <-gotTrace:
		if tid != span.SpanContext().TraceID().String() {
			t.Fatalf("traceparent not propagated: want %s got %s", span.SpanContext().TraceID().String(), tid)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for RedisStreamConsumer")
	}
	<-done

	pending, err := rdb.XPending(context.Background(), models.RedisOrderStreamKey(), c.group).Result()
	if err != nil || pending.Count != 0 {
		t.Fatalf("expected 0 pending messages after XAck, got %+v err=%v", pending, err)
	}
}
