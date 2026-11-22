package repo

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/Enormit/kubeticket/services/pkg/config"
	"github.com/Enormit/kubeticket/services/pkg/database"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func TestPostgresRepoWithSeededDB(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION") == "" {
		t.Skip("set RUN_INTEGRATION=1 to run against seeded local postgres")
	}
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

	r := NewPostgresRepo(pool)

	u, hash, err := r.GetUserByEmail(context.Background(), "alice@kubeticket.local")
	if err != nil {
		t.Fatalf("GetUserByEmail failed: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("User@123")); err != nil {
		t.Fatalf("seeded pgcrypto password hash incompatible with Go bcrypt: %v", err)
	}
	if u.Role != "USER" {
		t.Fatalf("expected USER role, got %s", u.Role)
	}

	ev, zones, err := r.GetEventWithZones(context.Background(), "kubefest-2026-world-tour")
	if err != nil {
		t.Fatalf("GetEventWithZones failed: %v", err)
	}
	if len(zones) != 3 {
		t.Fatalf("expected 3 zones, got %d", len(zones))
	}

	seats, err := r.ListEventSeats(context.Background(), ev.ID)
	if err != nil {
		t.Fatalf("ListEventSeats failed: %v", err)
	}
	if len(seats) != 600 {
		t.Fatalf("expected 600 seats for kubefest, got %d", len(seats))
	}
}

func TestPostgresAtomicCheckinConcurrency(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION") == "" {
		t.Skip("set RUN_INTEGRATION=1 to run against local postgres")
	}
	cfg := &config.Config{
		DatabaseURL:         "postgresql://kubeticket:kubeticket@127.0.0.1:5432/kubeticket?sslmode=disable",
		DBMaxConns:          10,
		DBMinConns:          2,
		DBUseSimpleProtocol: true,
	}
	ctx := context.Background()
	pool, err := database.NewPostgresPool(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	// Insert temporary PAID ticket for seeded Alice + Event 1 last seat, clean up afterwards.
	orderID := uuid.NewString()
	ticketID := uuid.NewString()
	const buyerID = "00000000-0000-0000-0000-000000000002"
	const adminID = "00000000-0000-0000-0000-000000000001"
	const eventID = "11111111-1111-1111-1111-111111111101"

	var seatID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM seats WHERE event_id = $1::uuid LIMIT 1`, eventID).Scan(&seatID); err != nil {
		t.Fatal(err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO orders (id, user_id, event_id, hold_token, status, total_cents, expires_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, 'test-hold', 'COMPLETED', 1000, NOW() + INTERVAL '1 hour')`,
		orderID, buyerID, eventID); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM checkins WHERE ticket_id = $1::uuid`, ticketID)
		_, _ = pool.Exec(ctx, `DELETE FROM tickets WHERE id = $1::uuid`, ticketID)
		_, _ = pool.Exec(ctx, `DELETE FROM orders WHERE id = $1::uuid`, orderID)
	}()

	if _, err := pool.Exec(ctx, `
		INSERT INTO tickets (id, order_id, user_id, event_id, seat_id, ticket_code, status)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5::uuid, $6, 'PAID')`,
		ticketID, orderID, buyerID, eventID, seatID, "TEST-QR-"+ticketID[:8]); err != nil {
		t.Fatal(err)
	}

	r := NewPostgresRepo(pool)
	const scanners = 5
	errs := make([]error, scanners)
	var wg sync.WaitGroup
	for i := 0; i < scanners; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = r.CheckinTicketAtomic(ctx, ticketID, eventID, adminID, "GATE-A")
		}(i)
	}
	wg.Wait()

	success, dup := 0, 0
	for _, e := range errs {
		if e == nil {
			success++
		} else if errors.Is(e, ErrAlreadyCheckedIn) {
			dup++
		} else {
			t.Fatalf("unexpected checkin error: %v", e)
		}
	}
	if success != 1 || dup != scanners-1 {
		t.Fatalf("expected 1 success and %d ErrAlreadyCheckedIn, got success=%d dup=%d", scanners-1, success, dup)
	}
}
