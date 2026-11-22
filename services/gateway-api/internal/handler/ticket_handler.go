package handler

import (
	"errors"
	"net/http"

	"github.com/Enormit/kubeticket/services/gateway-api/internal/repo"
	"github.com/Enormit/kubeticket/services/gateway-api/internal/service"
	"github.com/Enormit/kubeticket/services/pkg/middleware"
	"github.com/go-chi/chi/v5"
)

// TicketHandler serves authenticated order history, tickets, and QR token issuance.
type TicketHandler struct {
	svc *service.TicketService
}

// NewTicketHandler constructs a TicketHandler.
func NewTicketHandler(svc *service.TicketService) *TicketHandler {
	return &TicketHandler{svc: svc}
}

func (h *TicketHandler) ListOrders(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	orders, err := h.svc.ListUserOrders(r.Context(), user.ID)
	if err != nil {
		middleware.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to load orders")
		return
	}
	middleware.WriteJSON(w, http.StatusOK, map[string]any{"orders": orders})
}

func (h *TicketHandler) ListTickets(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	tickets, err := h.svc.ListUserTickets(r.Context(), user.ID)
	if err != nil {
		middleware.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to load tickets")
		return
	}
	middleware.WriteJSON(w, http.StatusOK, map[string]any{"tickets": tickets})
}

func (h *TicketHandler) GetTicketQR(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	ticketID := chi.URLParam(r, "id")
	payload, token, err := h.svc.GenerateTicketQR(r.Context(), user.ID, ticketID)
	if err != nil {
		switch {
		case errors.Is(err, repo.ErrNotFound):
			middleware.WriteError(w, http.StatusNotFound, "not_found", "ticket not found")
		case errors.Is(err, service.ErrForbidden):
			middleware.WriteError(w, http.StatusForbidden, "forbidden", "you do not own this ticket")
		case errors.Is(err, repo.ErrTicketNotPaid):
			middleware.WriteError(w, http.StatusConflict, "ticket_not_paid", "ticket has not completed payment")
		default:
			middleware.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to generate QR")
		}
		return
	}
	middleware.WriteJSON(w, http.StatusOK, map[string]any{
		"qr_token": token,
		"payload":  payload,
	})
}
