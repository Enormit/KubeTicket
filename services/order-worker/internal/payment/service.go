package payment

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/Enormit/kubeticket/services/pkg/models"
	"github.com/Enormit/kubeticket/services/pkg/observability"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrOrderNotFound = errors.New("order not found")

// SettlementResult summarizes the outcome of processing a payment webhook.
type SettlementResult struct {
	Status  string `json:"status"` // completed, duplicate_ignored, refunded_expired
	OrderID string `json:"order_id"`
}

// Service handles ACID payment settlement, webhook deduplication, and late-webhook refunds.
type Service struct {
	pool     *pgxpool.Pool
	provider Provider
	logger   *slog.Logger
	onOutbox func()
}

// NewService constructs a payment settlement Service.
func NewService(pool *pgxpool.Pool, provider Provider, logger *slog.Logger, onOutbox func()) *Service {
	return &Service{
		pool:     pool,
		provider: provider,
		logger:   logger,
		onOutbox: onOutbox,
	}
}

// ProcessWebhook verifies, deduplicates, and settles a payment webhook transactionally.
func (s *Service) ProcessWebhook(ctx context.Context, ev *WebhookEvent) (*SettlementResult, error) {
	if ev.OrderID == "" {
		return nil, ErrOrderNotFound
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// 1. Webhook deduplication guard
	var insertedID string
	err = tx.QueryRow(ctx, `
		INSERT INTO webhook_events (provider, provider_event_id, event_type)
		VALUES ($1, $2, $3)
		ON CONFLICT (provider, provider_event_id) DO NOTHING
		RETURNING provider_event_id`,
		ev.Provider, ev.ProviderEventID, ev.EventType,
	).Scan(&insertedID)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Commit(ctx)
		return &SettlementResult{Status: "duplicate_ignored", OrderID: ev.OrderID}, nil
	}
	if err != nil {
		return nil, err
	}

	// 2. Lock order row
	var orderStatus, userID, eventID, currency string
	var totalCents int64
	err = tx.QueryRow(ctx, `
		SELECT status, user_id::text, event_id::text, total_cents, currency
		FROM orders
		WHERE id = $1::uuid
		FOR UPDATE`, ev.OrderID).Scan(&orderStatus, &userID, &eventID, &totalCents, &currency)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrOrderNotFound
	}
	if err != nil {
		return nil, err
	}

	if orderStatus == models.OrderCompleted || orderStatus == models.OrderRefundedExpired {
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return &SettlementResult{Status: "duplicate_ignored", OrderID: ev.OrderID}, nil
	}

	rawJSON := jsonOrEmpty(ev.RawPayload)

	// 3. Late Webhook Compensation when order already expired/cancelled
	if orderStatus == models.OrderExpired || orderStatus == models.OrderCancelled {
		if _, err := tx.Exec(ctx, `
			INSERT INTO payments (order_id, provider, provider_payment_id, amount_cents, currency, status, raw_payload)
			VALUES ($1::uuid, $2, $3, $4, $5, 'REFUNDED', $6::jsonb)
			ON CONFLICT (provider, provider_payment_id) DO NOTHING`,
			ev.OrderID, ev.Provider, ev.PaymentID, totalCents, currency, rawJSON,
		); err != nil {
			return nil, err
		}

		if _, err := tx.Exec(ctx, `
			UPDATE orders
			SET status = 'REFUNDED_EXPIRED', payment_intent_id = $2, updated_at = NOW()
			WHERE id = $1::uuid`, ev.OrderID, ev.PaymentID); err != nil {
			return nil, err
		}

		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}

		if s.provider != nil {
			if refErr := s.provider.RefundPayment(ctx, ev.PaymentID, ev.AmountCents, "order_expired_before_payment"); refErr != nil && s.logger != nil {
				observability.WithContext(ctx, s.logger).Error("late refund API call failed", slog.String("order_id", ev.OrderID), slog.Any("error", refErr))
			}
		}
		return &SettlementResult{Status: "refunded_expired", OrderID: ev.OrderID}, nil
	}

	// 4. Normal PENDING -> COMPLETED settlement + Outbox event
	if _, err := tx.Exec(ctx, `
		UPDATE orders
		SET status = 'COMPLETED', payment_intent_id = $2, completed_at = NOW(), updated_at = NOW()
		WHERE id = $1::uuid`, ev.OrderID, ev.PaymentID); err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE tickets
		SET status = 'PAID', updated_at = NOW()
		WHERE order_id = $1::uuid AND status = 'PENDING'`, ev.OrderID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, `
		UPDATE seats
		SET status = 'SOLD', updated_at = NOW()
		FROM order_items oi
		WHERE oi.order_id = $1::uuid AND seats.id = oi.seat_id
		RETURNING seats.id::text`, ev.OrderID)
	if err != nil {
		return nil, err
	}
	var seatIDs []string
	for rows.Next() {
		var sid string
		if err := rows.Scan(&sid); err != nil {
			rows.Close()
			return nil, err
		}
		seatIDs = append(seatIDs, sid)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO payments (order_id, provider, provider_payment_id, amount_cents, currency, status, raw_payload)
		VALUES ($1::uuid, $2, $3, $4, $5, 'SUCCEEDED', $6::jsonb)
		ON CONFLICT (provider, provider_payment_id) DO NOTHING`,
		ev.OrderID, ev.Provider, ev.PaymentID, totalCents, currency, rawJSON,
	); err != nil {
		return nil, err
	}

	outboxPayload, _ := json.Marshal(models.OutboxSeatSoldPayload{
		OrderID:     ev.OrderID,
		UserID:      userID,
		EventID:     eventID,
		SeatIDs:     seatIDs,
		CompletedAt: time.Now().UTC(),
	})

	if _, err := tx.Exec(ctx, `
		INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload)
		VALUES ('order', $1::uuid, 'ORDER_COMPLETED', $2::jsonb)`,
		ev.OrderID, string(outboxPayload)); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	if s.onOutbox != nil {
		s.onOutbox()
	}
	return &SettlementResult{Status: "completed", OrderID: ev.OrderID}, nil
}

func jsonOrEmpty(raw []byte) string {
	if len(raw) == 0 || !json.Valid(raw) {
		return "{}"
	}
	return string(raw)
}
