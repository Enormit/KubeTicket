package models

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// SeatLockRecord is the compact pipe-delimited value stored in Redis under seat:lock:<event>:<seat>.
// Format: "<STATUS>|<USER_ID>|<REF_ID>|<EXPIRES_UNIX_MS>"
type SeatLockRecord struct {
	Status        string
	UserID        string
	RefID         string // hold_token when HELD, order_id when CHECKOUT/SOLD
	ExpiresUnixMs int64
}

// Encode serializes SeatLockRecord for Redis storage (Lua-compatible).
func (r SeatLockRecord) Encode() string {
	return fmt.Sprintf("%s|%s|%s|%d", r.Status, r.UserID, r.RefID, r.ExpiresUnixMs)
}

// ParseSeatLockRecord decodes a pipe-delimited Redis lock value.
func ParseSeatLockRecord(raw string) (SeatLockRecord, bool) {
	parts := strings.Split(raw, "|")
	if len(parts) < 4 {
		return SeatLockRecord{}, false
	}
	expMs, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		expMs = 0
	}
	return SeatLockRecord{
		Status:        parts[0],
		UserID:        parts[1],
		RefID:         parts[2],
		ExpiresUnixMs: expMs,
	}, true
}

// SeatDelta captures a single seat state transition broadcast over WebSocket.
type SeatDelta struct {
	SeatID    string     `json:"seat_id"`
	Status    string     `json:"status"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// WebSocketEnvelope is the canonical JSON frame pushed to WebSocket clients.
type WebSocketEnvelope struct {
	Type      string      `json:"type"` // SYNC, DELTA, ORDER_CONFIRMED, PONG
	EventID   string      `json:"event_id,omitempty"`
	OrderID   string      `json:"order_id,omitempty"`
	Seq       int64       `json:"seq"`
	ServerNow time.Time   `json:"server_now"`
	Seats     []SeatDelta `json:"seats,omitempty"`
}

// OrderQueueMessage is enqueued to SQS FIFO / Redis Streams when a user submits checkout.
type OrderQueueMessage struct {
	OrderID         string    `json:"order_id"`
	UserID          string    `json:"user_id"`
	EventID         string    `json:"event_id"`
	HoldToken       string    `json:"hold_token"`
	SeatIDs         []string  `json:"seat_ids"`
	PaymentProvider string    `json:"payment_provider"`
	TraceParent     string    `json:"traceparent,omitempty"`
	RequestedAt     time.Time `json:"requested_at"`
}

// OutboxSeatSoldPayload is written atomically inside the DB order completion tx.
type OutboxSeatSoldPayload struct {
	OrderID     string    `json:"order_id"`
	UserID      string    `json:"user_id"`
	EventID     string    `json:"event_id"`
	SeatIDs     []string  `json:"seat_ids"`
	CompletedAt time.Time `json:"completed_at"`
}
