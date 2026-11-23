package payment

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Enormit/kubeticket/services/pkg/middleware"
	"github.com/google/uuid"
)

// Handler exposes payment webhook and local sandbox completion endpoints.
type Handler struct {
	svc      *Service
	provider Provider
}

// NewHandler creates a payment Handler.
func NewHandler(svc *Service, provider Provider) *Handler {
	return &Handler{svc: svc, provider: provider}
}

// HandleWebhook reads raw bytes, verifies HMAC signature, and settles the order.
func (h *Handler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	rawBody, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_body", "failed to read webhook body")
		return
	}

	ev, err := h.provider.VerifyAndParseWebhook(r.Header, rawBody)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidSignature), errors.Is(err, ErrWebhookTimestamp):
			middleware.WriteError(w, http.StatusUnauthorized, "invalid_signature", err.Error())
		case errors.Is(err, ErrUnsupportedEvent):
			middleware.WriteJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		default:
			middleware.WriteError(w, http.StatusBadRequest, "invalid_webhook", err.Error())
		}
		return
	}

	res, err := h.svc.ProcessWebhook(r.Context(), ev)
	if err != nil {
		if errors.Is(err, ErrOrderNotFound) {
			middleware.WriteError(w, http.StatusNotFound, "order_not_found", "order does not exist")
			return
		}
		middleware.WriteError(w, http.StatusInternalServerError, "settlement_failed", "failed to settle payment webhook")
		return
	}
	middleware.WriteJSON(w, http.StatusOK, res)
}

// HandleSandboxComplete lets local dev/E2E workflows simulate a signed Stripe payment webhook.
func (h *Handler) HandleSandboxComplete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OrderID     string `json:"order_id"`
		AmountCents int64  `json:"amount_cents"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.OrderID == "" {
		middleware.WriteError(w, http.StatusBadRequest, "invalid_request", "order_id is required")
		return
	}

	ev := &WebhookEvent{
		Provider:        "stripe",
		ProviderEventID: "evt_sandbox_" + uuid.NewString(),
		EventType:       "checkout.session.completed",
		OrderID:         req.OrderID,
		PaymentID:       "pi_sandbox_" + uuid.NewString()[:12],
		AmountCents:     req.AmountCents,
		Currency:        "USD",
		RawPayload:      []byte(fmt.Sprintf(`{"sandbox":true,"order_id":%q,"ts":%d}`, req.OrderID, time.Now().Unix())),
	}

	res, err := h.svc.ProcessWebhook(r.Context(), ev)
	if err != nil {
		if errors.Is(err, ErrOrderNotFound) {
			middleware.WriteError(w, http.StatusNotFound, "order_not_found", "order does not exist yet")
			return
		}
		middleware.WriteError(w, http.StatusInternalServerError, "settlement_failed", err.Error())
		return
	}
	middleware.WriteJSON(w, http.StatusOK, res)
}
