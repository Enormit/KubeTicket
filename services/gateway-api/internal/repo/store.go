package repo

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Enormit/kubeticket/services/pkg/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound    = errors.New("resource not found")
	ErrEmailExists = errors.New("email already registered")
)

// AuthRepository handles user account persistence.
type AuthRepository interface {
	CreateUser(ctx context.Context, email, passwordHash, fullName string) (*models.User, error)
	GetUserByEmail(ctx context.Context, email string) (*models.User, string, error)
	GetUserByID(ctx context.Context, id string) (*models.User, error)
}

// EventRepository handles public event, zone, and seat queries.
type EventRepository interface {
	ListEvents(ctx context.Context) ([]models.Event, error)
	GetEventWithZones(ctx context.Context, idOrSlug string) (*models.Event, []models.Zone, error)
	ListEventSeats(ctx context.Context, eventID string) ([]models.SeatState, error)
}

// PostgresRepo implements gateway-api database repositories with pgxpool.
type PostgresRepo struct {
	pool *pgxpool.Pool
}

// NewPostgresRepo creates a Postgres-backed repository.
func NewPostgresRepo(pool *pgxpool.Pool) *PostgresRepo {
	return &PostgresRepo{pool: pool}
}

func (r *PostgresRepo) CreateUser(ctx context.Context, email, passwordHash, fullName string) (*models.User, error) {
	const q = `
		INSERT INTO users (email, password_hash, full_name, role)
		VALUES ($1, $2, $3, 'USER')
		RETURNING id::text, email, full_name, role, created_at`

	var u models.User
	err := r.pool.QueryRow(ctx, q, email, passwordHash, fullName).Scan(
		&u.ID, &u.Email, &u.FullName, &u.Role, &u.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrEmailExists
		}
		return nil, err
	}
	return &u, nil
}

func (r *PostgresRepo) GetUserByEmail(ctx context.Context, email string) (*models.User, string, error) {
	const q = `
		SELECT id::text, email, password_hash, full_name, role, created_at
		FROM users
		WHERE email = $1`

	var u models.User
	var hash string
	err := r.pool.QueryRow(ctx, q, email).Scan(
		&u.ID, &u.Email, &hash, &u.FullName, &u.Role, &u.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	return &u, hash, nil
}

func (r *PostgresRepo) GetUserByID(ctx context.Context, id string) (*models.User, error) {
	const q = `
		SELECT id::text, email, full_name, role, created_at
		FROM users
		WHERE id = $1::uuid`

	var u models.User
	err := r.pool.QueryRow(ctx, q, id).Scan(
		&u.ID, &u.Email, &u.FullName, &u.Role, &u.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *PostgresRepo) ListEvents(ctx context.Context) ([]models.Event, error) {
	const q = `
		SELECT id::text, slug, title, description, venue, starts_at, sale_starts_at, sale_ends_at, max_seats_per_order, status
		FROM events
		WHERE status <> 'DRAFT'
		ORDER BY starts_at ASC`

	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]models.Event, 0)
	for rows.Next() {
		var ev models.Event
		if err := rows.Scan(
			&ev.ID, &ev.Slug, &ev.Title, &ev.Description, &ev.Venue,
			&ev.StartsAt, &ev.SaleStartsAt, &ev.SaleEndsAt, &ev.MaxSeatsPerOrder, &ev.Status,
		); err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}

func (r *PostgresRepo) GetEventWithZones(ctx context.Context, idOrSlug string) (*models.Event, []models.Zone, error) {
	const eventQ = `
		SELECT id::text, slug, title, description, venue, starts_at, sale_starts_at, sale_ends_at, max_seats_per_order, status
		FROM events
		WHERE id::text = $1 OR slug = $1`

	var ev models.Event
	err := r.pool.QueryRow(ctx, eventQ, idOrSlug).Scan(
		&ev.ID, &ev.Slug, &ev.Title, &ev.Description, &ev.Venue,
		&ev.StartsAt, &ev.SaleStartsAt, &ev.SaleEndsAt, &ev.MaxSeatsPerOrder, &ev.Status,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}

	const zonesQ = `
		SELECT id::text, event_id::text, code, name, price_cents, currency, color, svg_bounds
		FROM zones
		WHERE event_id = $1::uuid
		ORDER BY price_cents DESC`

	rows, err := r.pool.Query(ctx, zonesQ, ev.ID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	zones := make([]models.Zone, 0)
	for rows.Next() {
		var z models.Zone
		var rawBounds []byte
		if err := rows.Scan(&z.ID, &z.EventID, &z.Code, &z.Name, &z.PriceCents, &z.Currency, &z.Color, &rawBounds); err != nil {
			return nil, nil, err
		}
		if len(rawBounds) > 0 {
			_ = json.Unmarshal(rawBounds, &z.SVGBounds)
		}
		zones = append(zones, z)
	}
	return &ev, zones, rows.Err()
}

func (r *PostgresRepo) ListEventSeats(ctx context.Context, eventID string) ([]models.SeatState, error) {
	const q = `
		SELECT id::text, zone_id::text, row_label, seat_number, label, pos_x, pos_y, status
		FROM seats
		WHERE event_id = $1::uuid
		ORDER BY zone_id, row_label, seat_number`

	rows, err := r.pool.Query(ctx, q, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seats := make([]models.SeatState, 0)
	for rows.Next() {
		var s models.SeatState
		if err := rows.Scan(&s.ID, &s.ZoneID, &s.RowLabel, &s.SeatNumber, &s.Label, &s.PosX, &s.PosY, &s.Status); err != nil {
			return nil, err
		}
		seats = append(seats, s)
	}
	return seats, rows.Err()
}
