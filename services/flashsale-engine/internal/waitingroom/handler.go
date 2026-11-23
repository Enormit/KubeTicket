package waitingroom

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Enormit/kubeticket/services/pkg/middleware"
)

// Handler exposes HTTP endpoints for the Virtual Waiting Room.
type Handler struct {
	svc *Service
}

// NewHandler creates a waiting room HTTP handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

type joinRequest struct {
	EventID        string `json:"event_id"`
	TurnstileToken string `json:"turnstile_token"`
}

func (h *Handler) Join(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}

	var req joinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.EventID == "" {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_request", "event_id is required")
		return
	}

	status, err := h.svc.Join(r.Context(), user.ID, req.EventID, req.TurnstileToken, r.RemoteAddr)
	if err != nil {
		if errors.Is(err, ErrTurnstileFailed) {
			middleware.WriteError(w, http.StatusForbidden, "anti_bot_failed", "anti-bot verification failed")
			return
		}
		middleware.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to join waiting room")
		return
	}
	middleware.WriteJSON(w, http.StatusOK, status)
}

func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}

	eventID := r.URL.Query().Get("event_id")
	if eventID == "" {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_request", "event_id query param is required")
		return
	}

	status, err := h.svc.Status(r.Context(), user.ID, eventID)
	if err != nil {
		if errors.Is(err, ErrNotInWaitingRoom) {
			middleware.WriteError(w, http.StatusNotFound, "not_in_queue", "user has not joined waiting room for this event")
			return
		}
		middleware.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to fetch waiting room status")
		return
	}
	middleware.WriteJSON(w, http.StatusOK, status)
}
