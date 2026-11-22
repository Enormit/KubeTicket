package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Enormit/kubeticket/services/gateway-api/internal/repo"
	"github.com/Enormit/kubeticket/services/gateway-api/internal/service"
	"github.com/Enormit/kubeticket/services/pkg/middleware"
)

// AdminHandler handles admin event management and atomic QR check-in.
type AdminHandler struct {
	svc *service.AdminService
}

// NewAdminHandler constructs an AdminHandler.
func NewAdminHandler(svc *service.AdminService) *AdminHandler {
	return &AdminHandler{svc: svc}
}

func (h *AdminHandler) CreateEvent(w http.ResponseWriter, r *http.Request) {
	var in repo.CreateEventInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_json", "malformed JSON request")
		return
	}
	ev, zones, err := h.svc.CreateEvent(r.Context(), in)
	if err != nil {
		if errors.Is(err, service.ErrInvalidInput) {
			middleware.WriteError(w, http.StatusBadRequest, "invalid_input", "slug, title, and venue are required")
			return
		}
		middleware.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to create event")
		return
	}
	middleware.WriteJSON(w, http.StatusCreated, map[string]any{
		"event": ev,
		"zones": zones,
	})
}

func (h *AdminHandler) Checkin(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}

	var in service.CheckinInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_json", "malformed JSON request")
		return
	}

	res, err := h.svc.Checkin(r.Context(), user.ID, in)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidQR):
			middleware.WriteError(w, http.StatusBadRequest, "invalid_qr", "QR signature verification failed")
		case errors.Is(err, service.ErrExpiredQR):
			middleware.WriteError(w, http.StatusUnauthorized, "expired_qr", "QR token has expired")
		case errors.Is(err, repo.ErrNotFound):
			middleware.WriteError(w, http.StatusNotFound, "not_found", "ticket not found")
		case errors.Is(err, repo.ErrEventMismatch):
			middleware.WriteError(w, http.StatusBadRequest, "event_mismatch", "ticket belongs to another event")
		case errors.Is(err, repo.ErrAlreadyCheckedIn):
			middleware.WriteError(w, http.StatusConflict, "already_checked_in", "ticket has already been checked in")
		case errors.Is(err, repo.ErrTicketNotPaid):
			middleware.WriteError(w, http.StatusConflict, "ticket_not_paid", "ticket is not valid for entry")
		default:
			middleware.WriteError(w, http.StatusInternalServerError, "internal_error", "check-in failed")
		}
		return
	}

	middleware.WriteJSON(w, http.StatusOK, map[string]any{
		"status":  "checked_in",
		"checkin": res,
	})
}
