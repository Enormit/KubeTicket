package order

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Enormit/kubeticket/services/order-worker/internal/consumer"
	"github.com/Enormit/kubeticket/services/pkg/config"
	"github.com/Enormit/kubeticket/services/pkg/database"
	"github.com/Enormit/kubeticket/services/pkg/models"
	"github.com/google/uuid"
)

type fakeRepo struct {
	err error
}

func (f *fakeRepo) CreatePendingOrder(_ context.Context, _ models.OrderQueueMessage, _ time.Duration) (bool, error) {
	return f.err == nil, f.err
}

func TestProcessorMapsSeatConflictToPermanent(t *testing.T) {
	p := NewProcessor(&fakeRepo{err: ErrSeatAlreadyTaken}, nil, 15*time.Minute)
	err := p.HandleOrderMessage(context.Background(), models.OrderQueueMessage{
		OrderID: uuid.NewString(), UserID: uuid.NewString(), EventID: uuid.NewString(), SeatIDs: []string{uuid.NewString()},
	})
	if !errors.Is(err, consumer.ErrPermanent) {
		t.Fatalf("expected ErrPermanent, got %v", err)
	}
}

func TestPostgresOrderCreationACIDAndIdempotency(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION") == "" {
		t.Skip("set RUN_INTEGRATION=1 to run against local seeded postgres")
	}
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

	const buyer1 = "00000000-0000-0000-0000-000000000002"
	const buyer2 = "00000000-0000-0000-0000-000000000003"
	const eventID = "11111111-1111-1111-1111-111111111101"

	var seatID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM seats WHERE event_id = $1::uuid AND status = 'AVAILABLE' LIMIT 1`, eventID).Scan(&seatID); err != nil {
		t.Fatal(err)
	}

	ord1 := uuid.NewString()
	ord2 := uuid.NewString()
	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM tickets WHERE order_id IN ($1::uuid, $2::uuid)`, ord1, ord2)
		_, _ = pool.Exec(ctx, `DELETE FROM orders WHERE id IN ($1::uuid, $2::uuid)`, ord1, ord2)
	}()

	r := NewPostgresRepository(pool)

	// 1. First order succeeds
	created, err := r.CreatePendingOrder(ctx, models.OrderQueueMessage{
		OrderID:   ord1,
		UserID:    buyer1,
		EventID:   eventID,
		HoldToken: "hold-1",
		SeatIDs:   []string{seatID},
	}, 15*time.Minute)
	if err != nil || !created {
		t.Fatalf("expected order1 created, got created=%v err=%v", created, err)
	}

	// 2. Duplicate delivery of same OrderID is idempotent
	createdAgain, err := r.CreatePendingOrder(ctx, models.OrderQueueMessage{
		OrderID:   ord1,
		UserID:    buyer1,
		EventID:   eventID,
		HoldToken: "hold-1",
		SeatIDs:   []string{seatID},
	}, 15*time.Minute)
	if err != nil || createdAgain {
		t.Fatalf("expected idempotent no-op, got created=%v err=%v", createdAgain, err)
	}

	// 3. Conflicting order2 for same seat fails on ux_tickets_active_event_seat
	if _, err := r.CreatePendingOrder(ctx, models.OrderQueueMessage{
		OrderID:   ord2,
		UserID:    buyer2,
		EventID:   eventID,
		HoldToken: "hold-2",
		SeatIDs:   []string{seatID},
	}, 15*time.Minute); !errors.Is(err, ErrSeatAlreadyTaken) {
		t.Fatalf("expected ErrSeatAlreadyTaken, got %v", err)
	}
}
