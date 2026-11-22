package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Enormit/kubeticket/services/gateway-api/internal/repo"
	"github.com/Enormit/kubeticket/services/gateway-api/internal/service"
	"github.com/Enormit/kubeticket/services/pkg/config"
	"github.com/Enormit/kubeticket/services/pkg/middleware"
	"github.com/Enormit/kubeticket/services/pkg/models"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

type fakeAuthRepo struct {
	users map[string]struct {
		u    models.User
		hash string
	}
}

func (f *fakeAuthRepo) CreateUser(_ context.Context, email, passwordHash, fullName string) (*models.User, error) {
	if _, exists := f.users[email]; exists {
		return nil, repo.ErrEmailExists
	}
	u := models.User{ID: "u-new", Email: email, FullName: fullName, Role: "USER", CreatedAt: time.Now()}
	f.users[email] = struct {
		u    models.User
		hash string
	}{u: u, hash: passwordHash}
	return &u, nil
}

func (f *fakeAuthRepo) GetUserByEmail(_ context.Context, email string) (*models.User, string, error) {
	rec, ok := f.users[email]
	if !ok {
		return nil, "", repo.ErrNotFound
	}
	return &rec.u, rec.hash, nil
}

func (f *fakeAuthRepo) GetUserByID(_ context.Context, id string) (*models.User, error) {
	for _, rec := range f.users {
		if rec.u.ID == id {
			u := rec.u
			return &u, nil
		}
	}
	return nil, repo.ErrNotFound
}

type fakeEventRepo struct {
	ev    models.Event
	zones []models.Zone
	seats []models.SeatState
}

func (f *fakeEventRepo) ListEvents(_ context.Context) ([]models.Event, error) {
	return []models.Event{f.ev}, nil
}

func (f *fakeEventRepo) GetEventWithZones(_ context.Context, idOrSlug string) (*models.Event, []models.Zone, error) {
	if idOrSlug != f.ev.ID && idOrSlug != f.ev.Slug {
		return nil, nil, repo.ErrNotFound
	}
	return &f.ev, f.zones, nil
}

func (f *fakeEventRepo) ListEventSeats(_ context.Context, _ string) ([]models.SeatState, error) {
	out := make([]models.SeatState, len(f.seats))
	copy(out, f.seats)
	return out, nil
}

func TestAuthLifecycleAndSeatSnapshotOverlay(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	hash, _ := bcrypt.GenerateFromPassword([]byte("Secret123"), bcrypt.MinCost)
	authRepo := &fakeAuthRepo{
		users: map[string]struct {
			u    models.User
			hash string
		}{
			"alice@kubeticket.local": {
				u:    models.User{ID: "user-1", Email: "alice@kubeticket.local", FullName: "Alice", Role: "USER"},
				hash: string(hash),
			},
		},
	}

	evID := "11111111-1111-1111-1111-111111111101"
	eventRepo := &fakeEventRepo{
		ev: models.Event{ID: evID, Slug: "kubefest", Title: "KubeFest", Status: "ON_SALE"},
		zones: []models.Zone{
			{ID: "z1", EventID: evID, Code: "VIP", Name: "VIP", PriceCents: 25000},
		},
		seats: []models.SeatState{
			{ID: "seat-1", ZoneID: "z1", Label: "VIP-A1", Status: models.SeatAvailable},
			{ID: "seat-2", ZoneID: "z1", Label: "VIP-A2", Status: models.SeatAvailable},
		},
	}

	expMs := time.Now().Add(4 * time.Minute).UnixMilli()
	_ = rdb.Set(context.Background(), models.RedisSeatLockKey(evID, "seat-1"), models.SeatLockRecord{
		Status:        models.SeatHeld,
		UserID:        "user-2",
		RefID:         "hold-1",
		ExpiresUnixMs: expMs,
	}.Encode(), 5*time.Minute).Err()
	_ = rdb.Set(context.Background(), models.RedisSeatSeqKey(evID), 42, 0).Err()

	cfg := &config.Config{
		JWTSecret:             "test-secret",
		JWTExpiresIn:          time.Hour,
		AllowedOrigins:        []string{"http://localhost:3000"},
		PrometheusMetricsPath: "/metrics",
	}

	r := NewRouter(RouterDeps{
		Config:       cfg,
		Redis:        rdb,
		AuthHandler:  NewAuthHandler(service.NewAuthService(authRepo, cfg.JWTSecret, cfg.JWTExpiresIn), cfg.JWTExpiresIn, false),
		EventHandler: NewEventHandler(service.NewEventService(eventRepo, rdb)),
	})

	// 1. Login sets HttpOnly cookie and /me works with it
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"alice@kubeticket.local","password":"Secret123"}`))
	loginRec := httptest.NewRecorder()
	r.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected 200 login, got %d: %s", loginRec.Code, loginRec.Body.String())
	}
	cookies := loginRec.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != middleware.AuthCookieName || !cookies[0].HttpOnly {
		t.Fatalf("expected HttpOnly auth cookie, got %+v", cookies)
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meReq.AddCookie(cookies[0])
	meRec := httptest.NewRecorder()
	r.ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusOK {
		t.Fatalf("expected 200 /me, got %d", meRec.Code)
	}

	// 2. Seat snapshot overlays HELD lock from Redis and returns seq=42
	snapReq := httptest.NewRequest(http.MethodGet, "/api/v1/events/kubefest/seats/snapshot", nil)
	snapRec := httptest.NewRecorder()
	r.ServeHTTP(snapRec, snapReq)
	if snapRec.Code != http.StatusOK {
		t.Fatalf("expected 200 snapshot, got %d", snapRec.Code)
	}
	var snap models.SeatSnapshotResponse
	if err := json.Unmarshal(snapRec.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	if snap.Seq != 42 || len(snap.Seats) != 2 || snap.Seats[0].Status != models.SeatHeld || snap.Seats[1].Status != models.SeatAvailable {
		t.Fatalf("unexpected snapshot: %+v", snap)
	}
}
