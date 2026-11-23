package order

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Enormit/kubeticket/services/order-worker/internal/consumer"
	"github.com/Enormit/kubeticket/services/pkg/models"
	"github.com/Enormit/kubeticket/services/pkg/observability"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrSeatAlreadyTaken = errors.New("seat already claimed by another active order")
	ErrSeatUnavailable  = errors.New("one or more seats not found or blocked")
)

// Repository persists orders atomically in PostgreSQL.
type Repository interface {
	CreatePendingOrder(ctx context.Context, msg models.OrderQueueMessage, checkoutTTL time.Duration) (bool, error)
}

// PostgresRepository implements Repository using pgxpool.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository creates a PostgresRepository.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) CreatePendingOrder(ctx context.Context, msg models.OrderQueueMessage, checkoutTTL time.Duration) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// 1. Idempotency check for at-least-once SQS/Stream deliveries
	var existingID string
	err = tx.QueryRow(ctx, `SELECT id::text FROM orders WHERE id = $1::uuid`, msg.OrderID).Scan(&existingID)
	if err == nil {
		return false, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}

	// 2. Lock seat rows and load authoritative zone prices
	type seatMeta struct {
		zoneID     string
		priceCents int64
		currency   string
		status     string
	}
	const seatsQ = `
		SELECT s.id::text, s.zone_id::text, z.price_cents, z.currency, s.status
		FROM seats s
		JOIN zones z ON z.id = s.zone_id
		WHERE s.event_id = $1::uuid AND s.id = ANY($2::uuid[])
		FOR UPDATE OF s`

	rows, err := tx.Query(ctx, seatsQ, msg.EventID, msg.SeatIDs)
	if err != nil {
		return false, err
	}
	seatMap := make(map[string]seatMeta, len(msg.SeatIDs))
	for rows.Next() {
		var seatID string
		var meta seatMeta
		if err := rows.Scan(&seatID, &meta.zoneID, &meta.priceCents, &meta.currency, &meta.status); err != nil {
			rows.Close()
			return false, err
		}
		seatMap[seatID] = meta
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return false, err
	}
	rows.Close()

	if len(seatMap) != len(msg.SeatIDs) {
		return false, ErrSeatUnavailable
	}

	var totalCents int64
	currency := "USD"
	for _, sid := range msg.SeatIDs {
		meta := seatMap[sid]
		if meta.status != models.SeatAvailable {
			return false, ErrSeatAlreadyTaken
		}
		totalCents += meta.priceCents
		currency = meta.currency
	}

	provider := msg.PaymentProvider
	if provider == "" {
		provider = "stripe"
	}
	expiresAt := time.Now().UTC().Add(checkoutTTL)

	// 3. Create PENDING order
	if _, err := tx.Exec(ctx, `
		INSERT INTO orders (id, user_id, event_id, hold_token, status, total_cents, currency, payment_provider, expires_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, 'PENDING', $5, $6, $7, $8)`,
		msg.OrderID, msg.UserID, msg.EventID, msg.HoldToken, totalCents, currency, provider, expiresAt,
	); err != nil {
		return false, err
	}

	// 4. Create order_items and PENDING tickets (guarded by ux_tickets_active_event_seat)
	for _, sid := range msg.SeatIDs {
		meta := seatMap[sid]
		if _, err := tx.Exec(ctx, `
			INSERT INTO order_items (order_id, seat_id, zone_id, unit_price_cents)
			VALUES ($1::uuid, $2::uuid, $3::uuid, $4)`,
			msg.OrderID, sid, meta.zoneID, meta.priceCents,
		); err != nil {
			return false, err
		}

		ticketCode := "KT-" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", "")[:10])
		if _, err := tx.Exec(ctx, `
			INSERT INTO tickets (order_id, user_id, event_id, seat_id, ticket_code, status)
			VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, 'PENDING')`,
			msg.OrderID, msg.UserID, msg.EventID, sid, ticketCode,
		); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return false, ErrSeatAlreadyTaken
			}
			return false, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

// Processor processes queued checkout messages into ACID database orders.
type Processor struct {
	repo        Repository
	logger      *slog.Logger
	checkoutTTL time.Duration
}

// NewProcessor creates an order Processor.
func NewProcessor(repo Repository, logger *slog.Logger, checkoutTTL time.Duration) *Processor {
	if checkoutTTL <= 0 {
		checkoutTTL = 15 * time.Minute
	}
	return &Processor{repo: repo, logger: logger, checkoutTTL: checkoutTTL}
}

func (p *Processor) HandleOrderMessage(ctx context.Context, msg models.OrderQueueMessage) error {
	if msg.OrderID == "" || msg.UserID == "" || msg.EventID == "" || len(msg.SeatIDs) == 0 {
		return fmt.Errorf("%w: missing required fields in order message", consumer.ErrPermanent)
	}

	created, err := p.repo.CreatePendingOrder(ctx, msg, p.checkoutTTL)
	if err != nil {
		if errors.Is(err, ErrSeatAlreadyTaken) || errors.Is(err, ErrSeatUnavailable) {
			return fmt.Errorf("%w: %v", consumer.ErrPermanent, err)
		}
		return err
	}

	if p.logger != nil {
		observability.WithContext(ctx, p.logger).Info(
			"order processed into PENDING",
			slog.String("order_id", msg.OrderID),
			slog.Bool("created", created),
			slog.Int("seat_count", len(msg.SeatIDs)),
		)
	}
	return nil
}
