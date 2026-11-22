package models

import (
	"fmt"
	"time"
)

const (
	SeatAvailable = "AVAILABLE"
	SeatHeld      = "HELD"
	SeatCheckout  = "CHECKOUT"
	SeatSold      = "SOLD"
	SeatBlocked   = "BLOCKED"

	OrderPending         = "PENDING"
	OrderCompleted       = "COMPLETED"
	OrderExpired         = "EXPIRED"
	OrderCancelled       = "CANCELLED"
	OrderRefundedExpired = "REFUNDED_EXPIRED"

	TicketPending   = "PENDING"
	TicketPaid      = "PAID"
	TicketCheckedIn = "CHECKED_IN"
	TicketCancelled = "CANCELLED"
	TicketRefunded  = "REFUNDED"
)

// User represents an application account.
type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	FullName  string    `json:"full_name"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// Event represents a ticketed concert or sports event.
type Event struct {
	ID               string     `json:"id"`
	Slug             string     `json:"slug"`
	Title            string     `json:"title"`
	Description      string     `json:"description"`
	Venue            string     `json:"venue"`
	StartsAt         time.Time  `json:"starts_at"`
	SaleStartsAt     time.Time  `json:"sale_starts_at"`
	SaleEndsAt       *time.Time `json:"sale_ends_at,omitempty"`
	MaxSeatsPerOrder int        `json:"max_seats_per_order"`
	Status           string     `json:"status"`
}

// Zone represents a venue pricing section.
type Zone struct {
	ID         string         `json:"id"`
	EventID    string         `json:"event_id"`
	Code       string         `json:"code"`
	Name       string         `json:"name"`
	PriceCents int64          `json:"price_cents"`
	Currency   string         `json:"currency"`
	Color      string         `json:"color"`
	SVGBounds  map[string]any `json:"svg_bounds"`
}

// SeatState represents a seat plus its real-time availability overlay.
type SeatState struct {
	ID         string     `json:"id"`
	ZoneID     string     `json:"zone_id"`
	RowLabel   string     `json:"row_label"`
	SeatNumber int        `json:"seat_number"`
	Label      string     `json:"label"`
	PosX       float64    `json:"pos_x"`
	PosY       float64    `json:"pos_y"`
	Status     string     `json:"status"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

// SeatSnapshotResponse returns the merged DB + Redis seat map with sequence and server clock.
type SeatSnapshotResponse struct {
	EventID   string      `json:"event_id"`
	Seq       int64       `json:"seq"`
	ServerNow time.Time   `json:"server_now"`
	Zones     []Zone      `json:"zones"`
	Seats     []SeatState `json:"seats"`
}

// Order represents a ticket purchase order.
type Order struct {
	ID              string      `json:"id"`
	UserID          string      `json:"user_id"`
	EventID         string      `json:"event_id"`
	Status          string      `json:"status"`
	TotalCents      int64       `json:"total_cents"`
	Currency        string      `json:"currency"`
	PaymentProvider string      `json:"payment_provider"`
	ExpiresAt       time.Time   `json:"expires_at"`
	CompletedAt     *time.Time  `json:"completed_at,omitempty"`
	CreatedAt       time.Time   `json:"created_at"`
	Items           []OrderItem `json:"items,omitempty"`
}

// OrderItem links an order to a specific seat.
type OrderItem struct {
	SeatID         string `json:"seat_id"`
	SeatLabel      string `json:"seat_label,omitempty"`
	ZoneID         string `json:"zone_id"`
	UnitPriceCents int64  `json:"unit_price_cents"`
}

// Ticket represents an issued e-ticket.
type Ticket struct {
	ID         string    `json:"id"`
	OrderID    string    `json:"order_id"`
	EventID    string    `json:"event_id"`
	EventTitle string    `json:"event_title,omitempty"`
	SeatID     string    `json:"seat_id"`
	SeatLabel  string    `json:"seat_label,omitempty"`
	TicketCode string    `json:"ticket_code"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

// QRPayload is the HMAC-SHA256 signed e-ticket token.
type QRPayload struct {
	TicketID string `json:"ticket_id"`
	EventID  string `json:"event_id"`
	Exp      int64  `json:"exp"`
	Nonce    string `json:"nonce"`
	Sig      string `json:"sig"`
}

// Centralized Redis key & channel helpers shared across all services.

func RedisSeatLockKey(eventID, seatID string) string {
	return fmt.Sprintf("seat:lock:%s:%s", eventID, seatID)
}

func RedisSeatLockPattern(eventID string) string {
	return fmt.Sprintf("seat:lock:%s:*", eventID)
}

func RedisHoldZSetKey(eventID string) string {
	return fmt.Sprintf("seat:holds_zset:%s", eventID)
}

func RedisActiveHoldEventsSetKey() string {
	return "seat:active_events"
}

func RedisSeatSeqKey(eventID string) string {
	return fmt.Sprintf("seat:seq:%s", eventID)
}

func RedisSeatPubSubChannel(eventID string) string {
	return fmt.Sprintf("seat:updates:%s", eventID)
}

func RedisWaitingQueueKey(eventID string) string {
	return fmt.Sprintf("wr:queue:%s", eventID)
}

func RedisWaitingTokenHashKey(eventID string) string {
	return fmt.Sprintf("wr:tokens:%s", eventID)
}

func RedisWaitingActiveCheckoutKey(eventID string) string {
	return fmt.Sprintf("wr:active:%s", eventID)
}

func RedisOrderStreamKey() string {
	return "orders:stream"
}
