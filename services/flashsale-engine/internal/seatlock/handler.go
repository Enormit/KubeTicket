package seatlock

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Enormit/kubeticket/services/flashsale-engine/internal/waitingroom"
	"github.com/Enormit/kubeticket/services/pkg/middleware"
	"github.com/go-chi/chi/v5"
)

// Handler exposes HTTP endpoints for seat locking, release, and checkout submission.
type Handler struct {
	svc    *Service
	wrSvc  *waitingroom.Service
}

// NewHandler creates a seatlock HTTP handler.
func NewHandler(svc *Service, wrSvc *waitingroom.Service) *Handler {
	return &Handler{svc: svc, wrSvc: wrSvc}
}

type holdRequest struct {
	EventID string   `json:"event_id"`
	SeatIDs []string `json:"seat_ids"`
}

type checkoutRequest struct {
	EventID         string   `json:"event_id"`
	HoldToken       string   `json:"hold_token"`
	SeatIDs         []string `json:"seat_ids"`
	PaymentProvider string   `json:"payment_provider"`
}

func (h *Handler) AcquireHolds(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}

	var req holdRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.EventID == "" {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_request", "event_id and seat_ids are required")
		return
	}

	if h.wrSvc != nil {
		admTok := r.Header.Get("X-Admission-Token")
		if err := h.wrSvc.ValidateUserPass(r.Context(), admTok, user.ID, req.EventID); err != nil {
			middleware.WriteError(w, http.StatusForbidden, "admission_required", "valid X-Admission-Token from waiting room is required")
			return
		}
	}

	res, err := h.svc.AcquireHolds(r.Context(), user.ID, req.EventID, req.SeatIDs)
	if err != nil {
		switch {
		case errors.Is(err, ErrEmptySeatList), errors.Is(err, ErrTooManySeats):
			middleware.WriteError(w, http.StatusBadRequest, "invalid_seat_count", err.Error())
		case errors.Is(err, ErrSeatConflict):
			middleware.WriteError(w, http.StatusConflict, "seat_conflict", err.Error())
		default:
			middleware.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to acquire seat lock")
		}
		return
	}
	middleware.WriteJSON(w, http.StatusOK, res)
}

func (h *Handler) ReleaseHold(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}

	seatID := chi.URLParam(r, "seat_id")
	eventID := r.URL.Query().Get("event_id")
	if seatID == "" || eventID == "" {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_request", "seat_id and event_id are required")
		return
	}

	if err := h.svc.ReleaseHold(r.Context(), user.ID, eventID, seatID); err != nil {
		switch {
		case errors.Is(err, ErrHoldForbidden):
			middleware.WriteError(w, http.StatusForbidden, "forbidden", err.Error())
		case errors.Is(err, ErrHoldExpired):
			middleware.WriteError(w, http.StatusNotFound, "hold_not_found", err.Error())
		default:
			middleware.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to release hold")
		}
		return
	}
	middleware.WriteJSON(w, http.StatusOK, map[string]string{"status": "released", "seat_id": seatID})
}

func (h *Handler) SubmitCheckout(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}

	var req checkoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.EventID == "" {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_request", "event_id and seat_ids are required")
		return
	}

	if h.wrSvc != nil {
		admTok := r.Header.Get("X-Admission-Token")
		if err := h.wrSvc.ValidateUserPass(r.Context(), admTok, user.ID, req.EventID); err != nil {
			middleware.WriteError(w, http.StatusForbidden, "admission_required", "valid X-Admission-Token from waiting room is required")
			return
		}
	}

	res, err := h.svc.SubmitCheckout(r.Context(), user.ID, req.EventID, req.HoldToken, req.PaymentProvider, req.SeatIDs)
	if err != nil {
		switch {
		case errors.Is(err, ErrEmptySeatList), errors.Is(err, ErrTooManySeats):
			middleware.WriteError(w, http.StatusBadRequest, "invalid_seat_count", err.Error())
		case errors.Is(err, ErrHoldExpired):
			middleware.WriteError(w, http.StatusConflict, "hold_expired", err.Error())
		default:
			middleware.WriteError(w, http.StatusInternalServerError, "internal_error", "checkout submission failed")
		}
		return
	}
	middleware.WriteJSON(w, http.StatusAccepted, res)
}
