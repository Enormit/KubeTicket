package waitingroom

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Enormit/kubeticket/services/pkg/middleware"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
)

type stubVerifier struct{ err error }

func (s stubVerifier) Verify(_ context.Context, _, _ string) error { return s.err }

func TestWaitingRoomFairQueueAndBoundToken(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	svc := NewService(rdb, stubVerifier{}, "wr-secret", 2)
	ctx := context.Background()

	// Disable immediate admission on first join by pre-filling active capacity for first check,
	// or test Join -> AdmitBatch -> ValidateUserPass.
	// Fill active capacity to test queued ranks deterministically:
	svc.maxActiveUsers = 2

	// Admit 2 users first
	s1, err := svc.Join(ctx, "user-1", "ev-1", "", "127.0.0.1")
	if err != nil || !s1.Admitted || s1.AdmissionToken == "" {
		t.Fatalf("expected user-1 admitted immediately, got %+v err=%v", s1, err)
	}
	s2, err := svc.Join(ctx, "user-2", "ev-1", "", "127.0.0.1")
	if err != nil || !s2.Admitted {
		t.Fatalf("expected user-2 admitted, got %+v err=%v", s2, err)
	}

	// Capacity full (maxActiveUsers=2): user-3 and user-4 must queue
	s3, err := svc.Join(ctx, "user-3", "ev-1", "", "127.0.0.1")
	if err != nil || s3.Admitted || s3.Position != 1 {
		t.Fatalf("expected user-3 queued at position 1, got %+v err=%v", s3, err)
	}
	s4, err := svc.Join(ctx, "user-4", "ev-1", "", "127.0.0.1")
	if err != nil || s4.Admitted || s4.Position != 2 {
		t.Fatalf("expected user-4 queued at position 2, got %+v err=%v", s4, err)
	}

	// Re-joining by user-4 must NOT change their position (ZADD NX)
	s4Again, err := svc.Join(ctx, "user-4", "ev-1", "", "127.0.0.1")
	if err != nil || s4Again.Position != 2 {
		t.Fatalf("expected user-4 position to remain 2, got %+v err=%v", s4Again, err)
	}

	// Token binding check: user-1's token cannot be used by user-2 or on ev-2
	if err := svc.ValidateUserPass(ctx, s1.AdmissionToken, "user-1", "ev-1"); err != nil {
		t.Fatalf("expected valid pass for user-1/ev-1, got %v", err)
	}
	if err := svc.ValidateUserPass(ctx, s1.AdmissionToken, "user-2", "ev-1"); err == nil {
		t.Fatalf("expected rejection when token replayed by user-2")
	}
	if err := svc.ValidateUserPass(ctx, s1.AdmissionToken, "user-1", "ev-2"); err == nil {
		t.Fatalf("expected rejection when token replayed for ev-2")
	}
}

func TestWaitingRoomHTTPHandlerRejectsBot(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	svc := NewService(rdb, stubVerifier{err: ErrTurnstileFailed}, "wr-secret", 10)
	h := NewHandler(svc)

	r := chi.NewRouter()
	r.Use(middleware.JWTAuth("jwt-secret"))
	r.Post("/join", h.Join)

	tok, _ := middleware.GenerateToken("jwt-secret", 3600e9, "u-bot", "bot@test.local", "USER")
	req := httptest.NewRequest(http.MethodPost, "/join", bytes.NewBufferString(`{"event_id":"ev-1"}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for failed Turnstile, got %d", rec.Code)
	}
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["error"] != "anti_bot_failed" {
		t.Fatalf("unexpected error body: %v", body)
	}
}
