package service

import (
	"context"
	"strings"

	"github.com/Enormit/kubeticket/services/gateway-api/internal/repo"
	"github.com/Enormit/kubeticket/services/pkg/models"
)

// CheckinInput accepts either an encoded qr_token string or an explicit QRPayload.
type CheckinInput struct {
	QRToken string            `json:"qr_token"`
	Payload *models.QRPayload `json:"payload"`
	Gate    string            `json:"gate"`
}

// AdminService handles event management and gate ticket verification/check-in.
type AdminService struct {
	repo      repo.AdminRepository
	ticketSvc *TicketService
}

// NewAdminService constructs an AdminService.
func NewAdminService(r repo.AdminRepository, ticketSvc *TicketService) *AdminService {
	return &AdminService{repo: r, ticketSvc: ticketSvc}
}

func (s *AdminService) CreateEvent(ctx context.Context, in repo.CreateEventInput) (*models.Event, []models.Zone, error) {
	if strings.TrimSpace(in.Slug) == "" || strings.TrimSpace(in.Title) == "" || strings.TrimSpace(in.Venue) == "" {
		return nil, nil, ErrInvalidInput
	}
	return s.repo.CreateEvent(ctx, in)
}

func (s *AdminService) Checkin(ctx context.Context, adminUserID string, in CheckinInput) (*repo.CheckinResult, error) {
	var payload *models.QRPayload
	if strings.TrimSpace(in.QRToken) != "" {
		parsed, err := s.ticketSvc.ParseQRToken(strings.TrimSpace(in.QRToken))
		if err != nil {
			return nil, err
		}
		payload = parsed
	} else if in.Payload != nil {
		if err := s.ticketSvc.VerifyQR(*in.Payload); err != nil {
			return nil, err
		}
		payload = in.Payload
	} else {
		return nil, ErrInvalidQR
	}

	return s.repo.CheckinTicketAtomic(ctx, payload.TicketID, payload.EventID, adminUserID, in.Gate)
}
