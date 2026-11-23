package outbox

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Enormit/kubeticket/services/order-worker/internal/order"
	"github.com/Enormit/kubeticket/services/order-worker/internal/payment"
	"github.com/Enormit/kubeticket/services/pkg/config"
	"github.com/Enormit/kubeticket/services/pkg/database"
	"github.com/Enormit/kubeticket/services/pkg/models"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type spyProvider struct {
	refundCalls int
}

func (s *spyProvider) Name() string { return "stripe" }
func (s *spyProvider) VerifyAndParseWebhook(_ http.Header, _ []byte) (*payment.WebhookEvent, error) {
	return nil, nil
}
func (s *spyProvider) RefundPayment(_ context.Context, _ string, _ int64, _ string) error {
	s.refundCalls++
	return nil
}

func TestWebhookSettlementOutboxAndLateRefundIntegration(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION") == "" {
		t.Skip("set RUN_INTEGRATION=1 to run against local seeded postgres")
	}

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	cfg := &config.Config{
		DatabaseURL:         "postgresql://kubeticket:kubeticket@127.0.0.1:5432/kubeticket?sslmode=disable",
		DBMaxConns:          5,
		DBMinConns:          1,
		DBUseSimpleProtocol: true,
	}
	ctx := context.Background()
	pool, err := database.NewPostgresPool(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	const buyerID = "00000000-0000-0000-0000-000000000002"
	const eventID = "11111111-1111-1111-1111-111111111102"

	rows, err := pool.Query(ctx, `SELECT id::text FROM seats WHERE event_id = $1::uuid AND status = 'AVAILABLE' LIMIT 2`, eventID)
	if err != nil {
		t.Fatal(err)
	}
	var seatIDs []string
	for rows.Next() {
		var sid string
		_ = rows.Scan(&sid)
		seatIDs = append(seatIDs, sid)
	}
	rows.Close()
	if len(seatIDs) < 2 {
		t.Fatalf("expected 2 available seats")
	}

	ordOK := uuid.NewString()
	ordLate := uuid.NewString()
	evtOK := "evt_ok_" + ordOK[:8]
	evtLate := "evt_late_" + ordLate[:8]

	defer func() {
		_, _ = pool.Exec(ctx, `UPDATE seats SET status = 'AVAILABLE' WHERE id = ANY($1::uuid[])`, seatIDs)
		_, _ = pool.Exec(ctx, `DELETE FROM outbox_events WHERE aggregate_id IN ($1::uuid, $2::uuid)`, ordOK, ordLate)
		_, _ = pool.Exec(ctx, `DELETE FROM webhook_events WHERE provider_event_id IN ($1, $2)`, evtOK, evtLate)
		_, _ = pool.Exec(ctx, `DELETE FROM payments WHERE order_id IN ($1::uuid, $2::uuid)`, ordOK, ordLate)
		_, _ = pool.Exec(ctx, `DELETE FROM tickets WHERE order_id IN ($1::uuid, $2::uuid)`, ordOK, ordLate)
		_, _ = pool.Exec(ctx, `DELETE FROM orders WHERE id IN ($1::uuid, $2::uuid)`, ordOK, ordLate)
	}()

	orderRepo := order.NewPostgresRepository(pool)

	// 1. Create PENDING order #1
	if _, err := orderRepo.CreatePendingOrder(ctx, models.OrderQueueMessage{
		OrderID: ordOK, UserID: buyerID, EventID: eventID, HoldToken: "h1", SeatIDs: []string{seatIDs[0]},
	}, 15*time.Minute); err != nil {
		t.Fatal(err)
	}

	sub := rdb.Subscribe(ctx, models.RedisSeatPubSubChannel(eventID))
	defer sub.Close()
	_, _ = sub.Receive(ctx)

	disp := NewDispatcher(pool, rdb, nil)
	spy := &spyProvider{}
	paySvc := payment.NewService(pool, spy, nil, nil)

	ev1 := &payment.WebhookEvent{
		Provider: "stripe", ProviderEventID: evtOK, EventType: "checkout.session.completed",
		OrderID: ordOK, PaymentID: "pi_" + ordOK[:8], AmountCents: 18000, Currency: "USD", RawPayload: []byte(`{}`),
	}
	res1, err := paySvc.ProcessWebhook(ctx, ev1)
	if err != nil || res1.Status != "completed" {
		t.Fatalf("expected completed, got %+v err=%v", res1, err)
	}

	// Duplicate webhook must be ignored idempotently
	resDup, err := paySvc.ProcessWebhook(ctx, ev1)
	if err != nil || resDup.Status != "duplicate_ignored" {
		t.Fatalf("expected duplicate_ignored, got %+v err=%v", resDup, err)
	}

	// Dispatch outbox -> sets Redis SOLD and emits DELTA + ORDER_CONFIRMED
	dispatched, err := disp.DispatchPending(ctx)
	if err != nil || dispatched < 1 {
		t.Fatalf("expected outbox dispatch >= 1, got %d err=%v", dispatched, err)
	}
	lockVal, _ := rdb.Get(ctx, models.RedisSeatLockKey(eventID, seatIDs[0])).Result()
	if !strings.HasPrefix(lockVal, "SOLD|") {
		t.Fatalf("expected SOLD lock in Redis, got %q", lockVal)
	}

	m1, err := sub.ReceiveMessage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var env models.WebSocketEnvelope
	_ = json.Unmarshal([]byte(m1.Payload), &env)
	if env.Type != "DELTA" && env.Type != "ORDER_CONFIRMED" {
		t.Fatalf("unexpected PubSub frame: %+v", env)
	}

	// 2. Create order #2 and expire it via ExpirePendingOrders, then send late webhook -> REFUNDED_EXPIRED
	if _, err := orderRepo.CreatePendingOrder(ctx, models.OrderQueueMessage{
		OrderID: ordLate, UserID: buyerID, EventID: eventID, HoldToken: "h2", SeatIDs: []string{seatIDs[1]},
	}, 15*time.Minute); err != nil {
		t.Fatal(err)
	}
	_, _ = pool.Exec(ctx, `UPDATE orders SET expires_at = NOW() - INTERVAL '1 minute' WHERE id = $1::uuid`, ordLate)

	expCount, err := disp.ExpirePendingOrders(ctx)
	if err != nil || expCount < 1 {
		t.Fatalf("expected expired order count >= 1, got %d err=%v", expCount, err)
	}

	resLate, err := paySvc.ProcessWebhook(ctx, &payment.WebhookEvent{
		Provider: "stripe", ProviderEventID: evtLate, EventType: "payment_intent.succeeded",
		OrderID: ordLate, PaymentID: "pi_" + ordLate[:8], AmountCents: 18000, Currency: "USD", RawPayload: []byte(`{}`),
	})
	if err != nil || resLate.Status != "refunded_expired" || spy.refundCalls != 1 {
		t.Fatalf("expected refunded_expired & 1 refund call, got %+v refunds=%d err=%v", resLate, spy.refundCalls, err)
	}
}
