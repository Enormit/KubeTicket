package handler

import (
	"errors"
	"net/http"

	"github.com/Enormit/kubeticket/services/gateway-api/internal/repo"
	"github.com/Enormit/kubeticket/services/gateway-api/internal/service"
	"github.com/Enormit/kubeticket/services/pkg/middleware"
	"github.com/go-chi/chi/v5"
)

// EventHandler serves public event listings and real-time seat snapshots.
type EventHandler struct {
	svc *service.EventService
}

// NewEventHandler creates an EventHandler.
func NewEventHandler(svc *service.EventService) *EventHandler {
	return &EventHandler{svc: svc}
}

func (h *EventHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	events, err := h.svc.ListEvents(r.Context())
	if err != nil {
		middleware.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to load events")
		return
	}
	middleware.WriteJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (h *EventHandler) GetEvent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ev, zones, err := h.svc.GetEventWithZones(r.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			middleware.WriteError(w, http.StatusNotFound, "not_found", "event not found")
			return
		}
		middleware.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to load event")
		return
	}
	middleware.WriteJSON(w, http.StatusOK, map[string]any{
		"event": ev,
		"zones": zones,
	})
}

func (h *EventHandler) GetSeatSnapshot(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	snap, err := h.svc.GetSeatSnapshot(r.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			middleware.WriteError(w, http.StatusNotFound, "not_found", "event not found")
			return
		}
		middleware.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to build seat snapshot")
		return
	}
	middleware.WriteJSON(w, http.StatusOK, snap)
}
