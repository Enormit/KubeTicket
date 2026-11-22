package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Enormit/kubeticket/services/gateway-api/internal/repo"
	"github.com/Enormit/kubeticket/services/pkg/models"
)

var (
	ErrForbidden        = errors.New("forbidden")
	ErrInvalidQR        = errors.New("invalid QR signature")
	ErrExpiredQR        = errors.New("expired QR token")
)

// TicketService handles user orders, tickets, and HMAC-SHA256 QR signing/verification.
type TicketService struct {
	repo       repo.TicketRepository
	hmacSecret []byte
	qrTTL      time.Duration
}

// NewTicketService constructs a TicketService.
func NewTicketService(r repo.TicketRepository, hmacSecret string, qrTTL time.Duration) *TicketService {
	if qrTTL <= 0 {
		qrTTL = 24 * time.Hour
	}
	return &TicketService{
		repo:       r,
		hmacSecret: []byte(hmacSecret),
		qrTTL:      qrTTL,
	}
}

func (s *TicketService) ListUserOrders(ctx context.Context, userID string) ([]models.Order, error) {
	return s.repo.ListUserOrders(ctx, userID)
}

func (s *TicketService) ListUserTickets(ctx context.Context, userID string) ([]models.Ticket, error) {
	return s.repo.ListUserTickets(ctx, userID)
}

// GenerateTicketQR creates a signed HMAC-SHA256 QR payload and compact token for the ticket owner.
func (s *TicketService) GenerateTicketQR(ctx context.Context, userID, ticketID string) (*models.QRPayload, string, error) {
	t, ownerID, err := s.repo.GetTicketByID(ctx, ticketID)
	if err != nil {
		return nil, "", err
	}
	if ownerID != userID {
		return nil, "", ErrForbidden
	}
	if t.Status != models.TicketPaid && t.Status != models.TicketCheckedIn {
		return nil, "", repo.ErrTicketNotPaid
	}

	nonceBytes := make([]byte, 12)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, "", err
	}

	payload := models.QRPayload{
		TicketID: t.ID,
		EventID:  t.EventID,
		Exp:      time.Now().Add(s.qrTTL).Unix(),
		Nonce:    hex.EncodeToString(nonceBytes),
	}
	payload.Sig = s.SignQR(payload)

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	return &payload, token, nil
}

// SignQR computes the canonical HMAC-SHA256 hex digest for a QR payload.
func (s *TicketService) SignQR(p models.QRPayload) string {
	mac := hmac.New(sha256.New, s.hmacSecret)
	msg := fmt.Sprintf("%s|%s|%d|%s", p.TicketID, p.EventID, p.Exp, p.Nonce)
	mac.Write([]byte(msg))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyQR validates signature integrity (constant-time) and token expiration.
func (s *TicketService) VerifyQR(p models.QRPayload) error {
	if p.TicketID == "" || p.EventID == "" || p.Nonce == "" || p.Sig == "" {
		return ErrInvalidQR
	}
	expected := s.SignQR(p)
	providedBytes, err := hex.DecodeString(p.Sig)
	if err != nil {
		return ErrInvalidQR
	}
	expectedBytes, _ := hex.DecodeString(expected)
	if !hmac.Equal(providedBytes, expectedBytes) {
		return ErrInvalidQR
	}
	if time.Now().Unix() > p.Exp {
		return ErrExpiredQR
	}
	return nil
}

// ParseQRToken decodes a base64url QR token into QRPayload and verifies it.
func (s *TicketService) ParseQRToken(token string) (*models.QRPayload, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, ErrInvalidQR
	}
	var p models.QRPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, ErrInvalidQR
	}
	if err := s.VerifyQR(p); err != nil {
		return nil, err
	}
	return &p, nil
}
