package repo

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Enormit/kubeticket/services/pkg/models"
	"github.com/jackc/pgx/v5"
)

var (
	ErrAlreadyCheckedIn = errors.New("ticket already checked in")
	ErrTicketNotPaid    = errors.New("ticket is not in PAID status")
	ErrEventMismatch    = errors.New("ticket does not belong to this event")
)

// ZoneInput defines a pricing zone when creating an event.
type ZoneInput struct {
	Code       string         `json:"code"`
	Name       string         `json:"name"`
	PriceCents int64          `json:"price_cents"`
	Currency   string         `json:"currency"`
	Color      string         `json:"color"`
	SVGBounds  map[string]any `json:"svg_bounds"`
}

// CreateEventInput holds validated event creation fields.
type CreateEventInput struct {
	Slug             string      `json:"slug"`
	Title            string      `json:"title"`
	Description      string      `json:"description"`
	Venue            string      `json:"venue"`
	StartsAt         time.Time   `json:"starts_at"`
	SaleStartsAt     time.Time   `json:"sale_starts_at"`
	MaxSeatsPerOrder int         `json:"max_seats_per_order"`
	Status           string      `json:"status"`
	Zones            []ZoneInput `json:"zones"`
}

// CheckinResult captures atomic gate check-in output.
type CheckinResult struct {
	CheckinID   string    `json:"checkin_id"`
	TicketID    string    `json:"ticket_id"`
	EventID     string    `json:"event_id"`
	Gate        string    `json:"gate"`
	CheckedInAt time.Time `json:"checked_in_at"`
}

// TicketRepository manages user orders and tickets.
type TicketRepository interface {
	ListUserOrders(ctx context.Context, userID string) ([]models.Order, error)
	ListUserTickets(ctx context.Context, userID string) ([]models.Ticket, error)
	GetTicketByID(ctx context.Context, ticketID string) (*models.Ticket, string, error)
}

// AdminRepository manages admin event creation and gate check-ins.
type AdminRepository interface {
	CreateEvent(ctx context.Context, in CreateEventInput) (*models.Event, []models.Zone, error)
	CheckinTicketAtomic(ctx context.Context, ticketID, eventID, adminUserID, gate string) (*CheckinResult, error)
}

func (r *PostgresRepo) ListUserOrders(ctx context.Context, userID string) ([]models.Order, error) {
	const ordersQ = `
		SELECT id::text, user_id::text, event_id::text, status, total_cents, currency,
		       payment_provider, expires_at, completed_at, created_at
		FROM orders
		WHERE user_id = $1::uuid
		ORDER BY created_at DESC`

	rows, err := r.pool.Query(ctx, ordersQ, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	orders := make([]models.Order, 0)
	orderIndex := make(map[string]int)
	for rows.Next() {
		var o models.Order
		if err := rows.Scan(
			&o.ID, &o.UserID, &o.EventID, &o.Status, &o.TotalCents, &o.Currency,
			&o.PaymentProvider, &o.ExpiresAt, &o.CompletedAt, &o.CreatedAt,
		); err != nil {
			return nil, err
		}
		orderIndex[o.ID] = len(orders)
		orders = append(orders, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(orders) == 0 {
		return orders, nil
	}

	const itemsQ = `
		SELECT oi.order_id::text, oi.seat_id::text, s.label, oi.zone_id::text, oi.unit_price_cents
		FROM order_items oi
		JOIN orders o ON o.id = oi.order_id
		JOIN seats s ON s.id = oi.seat_id
		WHERE o.user_id = $1::uuid`

	iRows, err := r.pool.Query(ctx, itemsQ, userID)
	if err != nil {
		return nil, err
	}
	defer iRows.Close()

	for iRows.Next() {
		var orderID string
		var item models.OrderItem
		if err := iRows.Scan(&orderID, &item.SeatID, &item.SeatLabel, &item.ZoneID, &item.UnitPriceCents); err != nil {
			return nil, err
		}
		if idx, ok := orderIndex[orderID]; ok {
			orders[idx].Items = append(orders[idx].Items, item)
		}
	}
	return orders, iRows.Err()
}

func (r *PostgresRepo) ListUserTickets(ctx context.Context, userID string) ([]models.Ticket, error) {
	const q = `
		SELECT t.id::text, t.order_id::text, t.event_id::text, e.title,
		       t.seat_id::text, s.label, t.ticket_code, t.status, t.created_at
		FROM tickets t
		JOIN events e ON e.id = t.event_id
		JOIN seats s ON s.id = t.seat_id
		WHERE t.user_id = $1::uuid
		ORDER BY t.created_at DESC`

	rows, err := r.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tickets := make([]models.Ticket, 0)
	for rows.Next() {
		var t models.Ticket
		if err := rows.Scan(
			&t.ID, &t.OrderID, &t.EventID, &t.EventTitle,
			&t.SeatID, &t.SeatLabel, &t.TicketCode, &t.Status, &t.CreatedAt,
		); err != nil {
			return nil, err
		}
		tickets = append(tickets, t)
	}
	return tickets, rows.Err()
}

func (r *PostgresRepo) GetTicketByID(ctx context.Context, ticketID string) (*models.Ticket, string, error) {
	const q = `
		SELECT t.id::text, t.order_id::text, t.user_id::text, t.event_id::text, e.title,
		       t.seat_id::text, s.label, t.ticket_code, t.status, t.created_at
		FROM tickets t
		JOIN events e ON e.id = t.event_id
		JOIN seats s ON s.id = t.seat_id
		WHERE t.id = $1::uuid`

	var t models.Ticket
	var ownerID string
	err := r.pool.QueryRow(ctx, q, ticketID).Scan(
		&t.ID, &t.OrderID, &ownerID, &t.EventID, &t.EventTitle,
		&t.SeatID, &t.SeatLabel, &t.TicketCode, &t.Status, &t.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	return &t, ownerID, nil
}

func (r *PostgresRepo) CreateEvent(ctx context.Context, in CreateEventInput) (*models.Event, []models.Zone, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if in.MaxSeatsPerOrder <= 0 {
		in.MaxSeatsPerOrder = 4
	}
	if in.Status == "" {
		in.Status = "UPCOMING"
	}

	const evQ = `
		INSERT INTO events (slug, title, description, venue, starts_at, sale_starts_at, max_seats_per_order, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id::text, slug, title, description, venue, starts_at, sale_starts_at, sale_ends_at, max_seats_per_order, status`

	var ev models.Event
	err = tx.QueryRow(ctx, evQ,
		in.Slug, in.Title, in.Description, in.Venue, in.StartsAt, in.SaleStartsAt, in.MaxSeatsPerOrder, in.Status,
	).Scan(
		&ev.ID, &ev.Slug, &ev.Title, &ev.Description, &ev.Venue,
		&ev.StartsAt, &ev.SaleStartsAt, &ev.SaleEndsAt, &ev.MaxSeatsPerOrder, &ev.Status,
	)
	if err != nil {
		return nil, nil, err
	}

	zones := make([]models.Zone, 0, len(in.Zones))
	const zoneQ = `
		INSERT INTO zones (event_id, code, name, price_cents, currency, color, svg_bounds)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7)
		RETURNING id::text, event_id::text, code, name, price_cents, currency, color`

	for _, zIn := range in.Zones {
		currency := zIn.Currency
		if currency == "" {
			currency = "USD"
		}
		color := zIn.Color
		if color == "" {
			color = "#3B82F6"
		}
		boundsBytes, _ := json.Marshal(zIn.SVGBounds)
		if len(boundsBytes) == 0 || string(boundsBytes) == "null" {
			boundsBytes = []byte("{}")
		}
		var z models.Zone
		if err := tx.QueryRow(ctx, zoneQ,
			ev.ID, zIn.Code, zIn.Name, zIn.PriceCents, currency, color, boundsBytes,
		).Scan(&z.ID, &z.EventID, &z.Code, &z.Name, &z.PriceCents, &z.Currency, &z.Color); err != nil {
			return nil, nil, err
		}
		z.SVGBounds = zIn.SVGBounds
		zones = append(zones, z)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}
	return &ev, zones, nil
}

func (r *PostgresRepo) CheckinTicketAtomic(ctx context.Context, ticketID, eventID, adminUserID, gate string) (*CheckinResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var status, ticketEventID string
	err = tx.QueryRow(ctx, `
		SELECT status, event_id::text
		FROM tickets
		WHERE id = $1::uuid
		FOR UPDATE`, ticketID).Scan(&status, &ticketEventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if eventID != "" && ticketEventID != eventID {
		return nil, ErrEventMismatch
	}
	if status == models.TicketCheckedIn {
		return nil, ErrAlreadyCheckedIn
	}
	if status != models.TicketPaid {
		return nil, ErrTicketNotPaid
	}

	if gate == "" {
		gate = "MAIN"
	}

	var res CheckinResult
	err = tx.QueryRow(ctx, `
		INSERT INTO checkins (ticket_id, event_id, scanned_by, gate)
		VALUES ($1::uuid, $2::uuid, NULLIF($3, '')::uuid, $4)
		ON CONFLICT (ticket_id) DO NOTHING
		RETURNING id::text, checked_in_at`,
		ticketID, ticketEventID, adminUserID, gate,
	).Scan(&res.CheckinID, &res.CheckedInAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrAlreadyCheckedIn
	}
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE tickets
		SET status = 'CHECKED_IN', updated_at = NOW()
		WHERE id = $1::uuid`, ticketID); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	res.TicketID = ticketID
	res.EventID = ticketEventID
	res.Gate = gate
	return &res, nil
}
