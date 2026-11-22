package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestJWTCookieAndRoleMiddleware(t *testing.T) {
	secret := "test-secret"
	tok, err := GenerateToken(secret, time.Hour, "user-123", "alice@kubeticket.local", "ADMIN")
	if err != nil {
		t.Fatal(err)
	}

	protected := JWTAuth(secret)(RequireRole("ADMIN")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := UserFromContext(r.Context())
		if !ok || u.ID != "user-123" {
			t.Fatalf("missing expected user in context")
		}
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.AddCookie(&http.Cookie{Name: AuthCookieName, Value: tok})
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with admin cookie, got %d", rec.Code)
	}

	userTok, _ := GenerateToken(secret, time.Hour, "user-456", "bob@kubeticket.local", "USER")
	reqUser := httptest.NewRequest(http.MethodGet, "/admin", nil)
	reqUser.Header.Set("Authorization", "Bearer "+userTok)
	recUser := httptest.NewRecorder()
	protected.ServeHTTP(recUser, reqUser)
	if recUser.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for USER role, got %d", recUser.Code)
	}
}

func TestAdmissionTokenBinding(t *testing.T) {
	secret := "wr-secret"
	tok, err := IssueAdmissionToken(secret, 5*time.Minute, "user-1", "event-1", "jti-abc")
	if err != nil {
		t.Fatal(err)
	}

	jti, err := VerifyAdmissionToken(secret, tok, "user-1", "event-1")
	if err != nil || jti != "jti-abc" {
		t.Fatalf("expected valid jti, got jti=%q err=%v", jti, err)
	}
	if _, err := VerifyAdmissionToken(secret, tok, "user-2", "event-1"); err == nil {
		t.Fatalf("expected user binding rejection")
	}
	if _, err := VerifyAdmissionToken(secret, tok, "user-1", "event-2"); err == nil {
		t.Fatalf("expected event binding rejection")
	}
}

func TestRateLimitAndIdempotencyMiddleware(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	calls := 0
	h := RateLimit(rdb, "holds", 3, time.Minute)(
		Idempotency(rdb, time.Minute, true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			WriteJSON(w, http.StatusCreated, map[string]int{"call": calls})
		})),
	)

	// 1st request succeeds
	req1 := httptest.NewRequest(http.MethodPost, "/holds", bytes.NewBufferString(`{"seat":"A1"}`))
	req1.Header.Set("Idempotency-Key", "k1")
	rec1 := httptest.NewRecorder()
	h.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusCreated || calls != 1 {
		t.Fatalf("expected 201 and calls=1, got %d calls=%d", rec1.Code, calls)
	}

	// Replay with same key returns cached body without executing handler again
	req2 := httptest.NewRequest(http.MethodPost, "/holds", bytes.NewBufferString(`{"seat":"A1"}`))
	req2.Header.Set("Idempotency-Key", "k1")
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusCreated || calls != 1 || rec2.Header().Get("X-Idempotent-Replay") != "true" {
		t.Fatalf("expected cached replay, got code=%d calls=%d header=%s", rec2.Code, calls, rec2.Header().Get("X-Idempotent-Replay"))
	}

	// Mismatched payload with same key returns 409
	req3 := httptest.NewRequest(http.MethodPost, "/holds", bytes.NewBufferString(`{"seat":"A2"}`))
	req3.Header.Set("Idempotency-Key", "k1")
	rec3 := httptest.NewRecorder()
	h.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusConflict {
		t.Fatalf("expected 409 mismatch, got %d", rec3.Code)
	}

	// 4th request exceeds rate limit (limit=3)
	req4 := httptest.NewRequest(http.MethodPost, "/holds", bytes.NewBufferString(`{"seat":"A3"}`))
	req4.Header.Set("Idempotency-Key", "k2")
	rec4 := httptest.NewRecorder()
	h.ServeHTTP(rec4, req4)
	if rec4.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 rate limited, got %d", rec4.Code)
	}
}
