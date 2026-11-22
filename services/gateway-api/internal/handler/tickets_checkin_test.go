package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Enormit/kubeticket/services/gateway-api/internal/repo"
	"github.com/Enormit/kubeticket/services/gateway-api/internal/service"
	"github.com/Enormit/kubeticket/services/pkg/config"
	"github.com/Enormit/kubeticket/services/pkg/middleware"
	"github.com/Enormit/kubeticket/services/pkg/models"
)

type fakeTicketAdminRepo struct {
	mu        sync.Mutex
	tickets   map[string]models.Ticket
	owners    map[string]string
	checkedIn map[string]bool
}

func (f *fakeTicketAdminRepo) ListUserOrders(_ context.Context, _ string) ([]models.Order, error) {
	return nil, nil
}

func (f *fakeTicketAdminRepo) ListUserTickets(_ context.Context, userID string) ([]models.Ticket, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []models.Ticket
	for id, t := range f.tickets {
		if f.owners[id] == userID {
			out = append(out, t)
		}
	}
	return out, nil
}

func (f *fakeTicketAdminRepo) GetTicketByID(_ context.Context, ticketID string) (*models.Ticket, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.tickets[ticketID]
	if !ok {
		return nil, "", repo.ErrNotFound
	}
	return &t, f.owners[ticketID], nil
}

func (f *fakeTicketAdminRepo) CreateEvent(_ context.Context, in repo.CreateEventInput) (*models.Event, []models.Zone, error) {
	return &models.Event{ID: "ev-new", Slug: in.Slug, Title: in.Title, Venue: in.Venue}, nil, nil
}

func (f *fakeTicketAdminRepo) CheckinTicketAtomic(_ context.Context, ticketID, eventID, _ string, gate string) (*repo.CheckinResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.tickets[ticketID]
	if !ok {
		return nil, repo.ErrNotFound
	}
	if t.EventID != eventID {
		return nil, repo.ErrEventMismatch
	}
	if f.checkedIn[ticketID] || t.Status == models.TicketCheckedIn {
		return nil, repo.ErrAlreadyCheckedIn
	}
	if t.Status != models.TicketPaid {
		return nil, repo.ErrTicketNotPaid
	}
	f.checkedIn[ticketID] = true
	t.Status = models.TicketCheckedIn
	f.tickets[ticketID] = t
	return &repo.CheckinResult{
		CheckinID:   "chk-1",
		TicketID:    ticketID,
		EventID:     eventID,
		Gate:        gate,
		CheckedInAt: time.Now().UTC(),
	}, nil
}

func TestTicketQRHMACAndAtomicCheckin(t *testing.T) {
	taRepo := &fakeTicketAdminRepo{
		tickets: map[string]models.Ticket{
			"tkt-1": {ID: "tkt-1", OrderID: "ord-1", EventID: "ev-1", SeatID: "s-1", TicketCode: "KT-001", Status: models.TicketPaid},
		},
		owners:    map[string]string{"tkt-1": "buyer-1"},
		checkedIn: map[string]bool{},
	}

	cfg := &config.Config{
		JWTSecret:             "jwt-secret",
		JWTExpiresIn:          time.Hour,
		TicketQRHMACSecret:    "qr-hmac-secret",
		AllowedOrigins:        []string{"http://localhost:3000"},
		PrometheusMetricsPath: "/metrics",
	}

	ticketSvc := service.NewTicketService(taRepo, cfg.TicketQRHMACSecret, time.Hour)
	adminSvc := service.NewAdminService(taRepo, ticketSvc)

	r := NewRouter(RouterDeps{
		Config:        cfg,
		TicketHandler: NewTicketHandler(ticketSvc),
		AdminHandler:  NewAdminHandler(adminSvc),
	})

	buyerTok, _ := middleware.GenerateToken(cfg.JWTSecret, time.Hour, "buyer-1", "buyer1@test.local", "USER")
	otherTok, _ := middleware.GenerateToken(cfg.JWTSecret, time.Hour, "buyer-2", "buyer2@test.local", "USER")
	adminTok, _ := middleware.GenerateToken(cfg.JWTSecret, time.Hour, "admin-1", "admin@test.local", "ADMIN")

	// 1. Other user cannot fetch buyer-1's QR -> 403
	reqOther := httptest.NewRequest(http.MethodGet, "/api/v1/tickets/tkt-1/qr", nil)
	reqOther.Header.Set("Authorization", "Bearer "+otherTok)
	recOther := httptest.NewRecorder()
	r.ServeHTTP(recOther, reqOther)
	if recOther.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-owner, got %d", recOther.Code)
	}

	// 2. Owner fetches signed QR -> 200
	reqQR := httptest.NewRequest(http.MethodGet, "/api/v1/tickets/tkt-1/qr", nil)
	reqQR.Header.Set("Authorization", "Bearer "+buyerTok)
	recQR := httptest.NewRecorder()
	r.ServeHTTP(recQR, reqQR)
	if recQR.Code != http.StatusOK {
		t.Fatalf("expected 200 QR response, got %d: %s", recQR.Code, recQR.Body.String())
	}
	var qrResp struct {
		QRToken string           `json:"qr_token"`
		Payload models.QRPayload `json:"payload"`
	}
	if err := json.Unmarshal(recQR.Body.Bytes(), &qrResp); err != nil {
		t.Fatal(err)
	}

	// 3. Regular user cannot call /admin/checkin -> 403
	reqNonAdmin := httptest.NewRequest(http.MethodPost, "/api/v1/admin/checkin", bytes.NewBufferString(`{"qr_token":"`+qrResp.QRToken+`"}`))
	reqNonAdmin.Header.Set("Authorization", "Bearer "+buyerTok)
	recNonAdmin := httptest.NewRecorder()
	r.ServeHTTP(recNonAdmin, reqNonAdmin)
	if recNonAdmin.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for USER calling admin checkin, got %d", recNonAdmin.Code)
	}

	// 4. Tampered payload signature -> 400
	tampered := qrResp.Payload
	tampered.Sig = "0000000000000000000000000000000000000000000000000000000000000000"
	tamperedBody, _ := json.Marshal(map[string]any{"payload": tampered, "gate": "A1"})
	reqTamper := httptest.NewRequest(http.MethodPost, "/api/v1/admin/checkin", bytes.NewReader(tamperedBody))
	reqTamper.Header.Set("Authorization", "Bearer "+adminTok)
	recTamper := httptest.NewRecorder()
	r.ServeHTTP(recTamper, reqTamper)
	if recTamper.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for tampered QR, got %d", recTamper.Code)
	}

	// 5. Concurrent check-in attempts by gate scanners -> exactly 1 succeeds (200), 1 gets 409 Conflict
	var wg sync.WaitGroup
	codes := make([]int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			body, _ := json.Marshal(map[string]any{"qr_token": qrResp.QRToken, "gate": "NORTH-1"})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/checkin", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+adminTok)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			codes[idx] = rec.Code
		}(i)
	}
	wg.Wait()

	okCount, conflictCount := 0, 0
	for _, c := range codes {
		if c == http.StatusOK {
			okCount++
		} else if c == http.StatusConflict {
			conflictCount++
		}
	}
	if okCount != 1 || conflictCount != 1 {
		t.Fatalf("expected 1 OK and 1 Conflict on concurrent check-in, got codes=%v", codes)
	}
}
