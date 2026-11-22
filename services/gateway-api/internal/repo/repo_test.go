package repo

import (
	"context"
	"os"
	"testing"

	"github.com/Enormit/kubeticket/services/pkg/config"
	"github.com/Enormit/kubeticket/services/pkg/database"
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
