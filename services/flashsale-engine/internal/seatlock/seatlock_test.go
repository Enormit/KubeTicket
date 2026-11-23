package seatlock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Enormit/kubeticket/services/pkg/config"
	"github.com/Enormit/kubeticket/services/pkg/database"
	"github.com/Enormit/kubeticket/services/pkg/models"
	"github.com/Enormit/kubeticket/services/pkg/observability"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestConcurrentLuaSeatLockZeroOverselling(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	metrics := observability.NewMetrics("flashsale-test")
	svc := NewService(rdb, NewRedisStreamPublisher(rdb), metrics, 300*time.Second, 900*time.Second)

	const goroutines = 50
	var wg sync.WaitGroup
	errs := make([]error, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			userID := fmt.Sprintf("user-%d", idx)
			_, errs[idx] = svc.AcquireHolds(context.Background(), userID, "ev-1", []string{"seat-hot"})
		}(i)
	}
	wg.Wait()

	success, conflicts := 0, 0
	for _, err := range errs {
		if err == nil {
			success++
		} else if errors.Is(err, ErrSeatConflict) {
			conflicts++
		} else {
			t.Fatalf("unexpected lock error: %v", err)
		}
	}
	if success != 1 || conflicts != goroutines-1 {
		t.Fatalf("expected 1 success and %d conflicts, got success=%d conflicts=%d", goroutines-1, success, conflicts)
	}
}

func TestMultiSeatAtomicityAndCheckoutPromotion(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	svc := NewService(rdb, NewRedisStreamPublisher(rdb), nil, 300*time.Second, 900*time.Second)
	ctx := context.Background()

	// user-1 holds seat-2
	h1, err := svc.AcquireHolds(ctx, "user-1", "ev-1", []string{"seat-2"})
	if err != nil {
		t.Fatal(err)
	}

	// user-2 tries [seat-1, seat-2]: must fail and NOT lock seat-1
	if _, err := svc.AcquireHolds(ctx, "user-2", "ev-1", []string{"seat-1", "seat-2"}); !errors.Is(err, ErrSeatConflict) {
		t.Fatalf("expected ErrSeatConflict, got %v", err)
	}
	if exists, _ := rdb.Exists(ctx, models.RedisSeatLockKey("ev-1", "seat-1")).Result(); exists != 0 {
		t.Fatalf("expected seat-1 to remain unlocked after atomic multi-seat conflict")
	}

	// user-2 locks seat-1 and promotes to checkout -> order appears in Redis stream
	h2, err := svc.AcquireHolds(ctx, "user-2", "ev-1", []string{"seat-1"})
	if err != nil {
		t.Fatal(err)
	}

	// user-1 cannot checkout user-2's seat
	if _, err := svc.SubmitCheckout(ctx, "user-1", "ev-1", h1.HoldToken, "stripe", []string{"seat-1"}); !errors.Is(err, ErrHoldExpired) {
		t.Fatalf("expected checkout rejection for non-owner, got %v", err)
	}

	co, err := svc.SubmitCheckout(ctx, "user-2", "ev-1", h2.HoldToken, "stripe", []string{"seat-1"})
	if err != nil || co.OrderID == "" {
		t.Fatalf("expected checkout accepted, got %+v err=%v", co, err)
	}

	streamLen, err := rdb.XLen(ctx, models.RedisOrderStreamKey()).Result()
	if err != nil || streamLen != 1 {
		t.Fatalf("expected 1 message in order stream, got len=%d err=%v", streamLen, err)
	}
}

func TestZSETSweeperPublishesReleasedDelta(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	svc := NewService(rdb, nil, nil, 2*time.Second, 10*time.Second)
	ctx := context.Background()

	sub := rdb.Subscribe(ctx, models.RedisSeatPubSubChannel("ev-1"))
	defer sub.Close()
	if _, err := sub.Receive(ctx); err != nil {
		t.Fatal(err)
	}

	// Insert an already-expired hold into Redis + ZSET
	pastExp := time.Now().Add(-5 * time.Second).UnixMilli()
	_ = rdb.Set(ctx, models.RedisSeatLockKey("ev-1", "seat-exp"), models.SeatLockRecord{
		Status:        models.SeatHeld,
		UserID:        "u-1",
		RefID:         "h-1",
		ExpiresUnixMs: pastExp,
	}.Encode(), time.Minute).Err()
	_ = rdb.ZAdd(ctx, models.RedisHoldZSetKey("ev-1"), redis.Z{Score: float64(pastExp), Member: "seat-exp"}).Err()
	_ = rdb.SAdd(ctx, models.RedisActiveHoldEventsSetKey(), "ev-1").Err()

	released, err := svc.SweepExpiredHolds(ctx)
	if err != nil || released != 1 {
		t.Fatalf("expected 1 seat swept, got released=%d err=%v", released, err)
	}

	msg, err := sub.ReceiveMessage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var env models.WebSocketEnvelope
	if err := json.Unmarshal([]byte(msg.Payload), &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Seats) != 1 || env.Seats[0].SeatID != "seat-exp" || env.Seats[0].Status != models.SeatAvailable {
		t.Fatalf("expected AVAILABLE delta for seat-exp, got %+v", env)
	}
}

func TestHydrateFromPostgresIntegration(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION") == "" {
		t.Skip("set RUN_INTEGRATION=1 to run against local postgres")
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
	pool, err := database.NewPostgresPool(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	svc := NewService(rdb, nil, nil, 300*time.Second, 900*time.Second)
	if _, err := svc.HydrateFromPostgres(context.Background(), pool); err != nil {
		t.Fatalf("HydrateFromPostgres failed: %v", err)
	}
	if !svc.IsHydrated() {
		t.Fatalf("expected IsHydrated() == true")
	}
}

func TestSQSMessageGroupIDSharding(t *testing.T) {
	eventID := "11111111-1111-1111-1111-111111111101"

	// Empty OrderID falls back to EventID.
	if got := SQSMessageGroupID(models.OrderQueueMessage{EventID: eventID}); got != eventID {
		t.Fatalf("expected fallback %q, got %q", eventID, got)
	}

	// Same OrderID is deterministic across retries.
	m1 := models.OrderQueueMessage{EventID: eventID, OrderID: "ord-fixed-123"}
	if a, b := SQSMessageGroupID(m1), SQSMessageGroupID(m1); a != b || a == eventID {
		t.Fatalf("expected stable sharded group id, got a=%q b=%q", a, b)
	}

	// Distinct orders for the same event spread across multiple FIFO groups for parallel worker consumption.
	groups := make(map[string]struct{})
	for i := 0; i < 40; i++ {
		gid := SQSMessageGroupID(models.OrderQueueMessage{
			EventID: eventID,
			OrderID: fmt.Sprintf("order-%d", i),
		})
		groups[gid] = struct{}{}
	}
	if len(groups) < 10 {
		t.Fatalf("expected orders to fan out across >=10 FIFO shards, got %d", len(groups))
	}
}
