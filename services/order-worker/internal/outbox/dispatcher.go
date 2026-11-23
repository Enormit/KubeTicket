package outbox

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/Enormit/kubeticket/services/pkg/models"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Dispatcher publishes committed DB outbox events to Redis/WebSocket and expires stale PENDING orders.
type Dispatcher struct {
	pool   *pgxpool.Pool
	rdb    redis.UniversalClient
	logger *slog.Logger
	wakeCh chan struct{}
}

// NewDispatcher creates an outbox & expiry reconciler Dispatcher.
func NewDispatcher(pool *pgxpool.Pool, rdb redis.UniversalClient, logger *slog.Logger) *Dispatcher {
	return &Dispatcher{
		pool:   pool,
		rdb:    rdb,
		logger: logger,
		wakeCh: make(chan struct{}, 16),
	}
}

// Notify triggers an immediate outbox dispatch pass.
func (d *Dispatcher) Notify() {
	select {
	case d.wakeCh <- struct{}{}:
	default:
	}
}

// Run loops outbox dispatching and expired-order cleanup until ctx cancels.
func (d *Dispatcher) Run(ctx context.Context, outboxInterval, expiryInterval time.Duration) {
	if outboxInterval <= 0 {
		outboxInterval = time.Second
	}
	if expiryInterval <= 0 {
		expiryInterval = 5 * time.Second
	}

	outboxTicker := time.NewTicker(outboxInterval)
	expiryTicker := time.NewTicker(expiryInterval)
	defer outboxTicker.Stop()
	defer expiryTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-d.wakeCh:
			_, _ = d.DispatchPending(ctx)
		case <-outboxTicker.C:
			_, _ = d.DispatchPending(ctx)
		case <-expiryTicker.C:
			_, _ = d.ExpirePendingOrders(ctx)
		}
	}
}

// DispatchPending reads unpublished outbox rows using SKIP LOCKED, pushes SOLD & ORDER_CONFIRMED to Redis, and marks published.
func (d *Dispatcher) DispatchPending(ctx context.Context) (int, error) {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		SELECT id, event_type, payload
		FROM outbox_events
		WHERE published_at IS NULL
		ORDER BY id ASC
		LIMIT 50
		FOR UPDATE SKIP LOCKED`)
	if err != nil {
		return 0, err
	}

	type rowItem struct {
		id        int64
		eventType string
		raw       []byte
	}
	var items []rowItem
	for rows.Next() {
		var it rowItem
		if err := rows.Scan(&it.id, &it.eventType, &it.raw); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	if len(items) == 0 {
		return 0, tx.Commit(ctx)
	}

	dispatched := 0
	for _, it := range items {
		if it.eventType == "ORDER_COMPLETED" {
			var p models.OutboxSeatSoldPayload
			if err := json.Unmarshal(it.raw, &p); err == nil {
				if err := d.publishSoldToRedis(ctx, p); err != nil {
					if d.logger != nil {
						d.logger.Error("outbox redis publish failed", slog.Int64("outbox_id", it.id), slog.Any("error", err))
					}
					continue
				}
			}
		}

		if _, err := tx.Exec(ctx, `UPDATE outbox_events SET published_at = NOW() WHERE id = $1`, it.id); err != nil {
			return dispatched, err
		}
		dispatched++
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return dispatched, nil
}

func (d *Dispatcher) publishSoldToRedis(ctx context.Context, p models.OutboxSeatSoldPayload) error {
	if d.rdb == nil {
		return nil
	}
	deltas := make([]models.SeatDelta, 0, len(p.SeatIDs))
	zKey := models.RedisHoldZSetKey(p.EventID)
	for _, sid := range p.SeatIDs {
		val := models.SeatLockRecord{
			Status:        models.SeatSold,
			UserID:        p.UserID,
			RefID:         p.OrderID,
			ExpiresUnixMs: 0,
		}.Encode()
		if err := d.rdb.Set(ctx, models.RedisSeatLockKey(p.EventID, sid), val, 0).Err(); err != nil {
			return err
		}
		_ = d.rdb.ZRem(ctx, zKey, sid).Err()
		deltas = append(deltas, models.SeatDelta{
			SeatID: sid,
			Status: models.SeatSold,
		})
	}

	seq, _ := d.rdb.Incr(ctx, models.RedisSeatSeqKey(p.EventID)).Result()
	now := time.Now().UTC()

	deltaFrame, _ := json.Marshal(models.WebSocketEnvelope{
		Type:      "DELTA",
		EventID:   p.EventID,
		Seq:       seq,
		ServerNow: now,
		Seats:     deltas,
	})
	if err := d.rdb.Publish(ctx, models.RedisSeatPubSubChannel(p.EventID), deltaFrame).Err(); err != nil {
		return err
	}

	confirmedFrame, _ := json.Marshal(models.WebSocketEnvelope{
		Type:      "ORDER_CONFIRMED",
		EventID:   p.EventID,
		OrderID:   p.OrderID,
		Seq:       seq,
		ServerNow: now,
		Seats:     deltas,
	})
	return d.rdb.Publish(ctx, models.RedisSeatPubSubChannel(p.EventID), confirmedFrame).Err()
}

// ExpirePendingOrders transitions overdue PENDING orders to EXPIRED, cancels PENDING tickets, and frees locks.
func (d *Dispatcher) ExpirePendingOrders(ctx context.Context) (int, error) {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		SELECT id::text, event_id::text
		FROM orders
		WHERE status = 'PENDING' AND expires_at <= NOW()
		ORDER BY expires_at ASC
		LIMIT 50
		FOR UPDATE SKIP LOCKED`)
	if err != nil {
		return 0, err
	}
	type expOrder struct {
		orderID string
		eventID string
	}
	var expired []expOrder
	for rows.Next() {
		var o expOrder
		if err := rows.Scan(&o.orderID, &o.eventID); err != nil {
			rows.Close()
			return 0, err
		}
		expired = append(expired, o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(expired) == 0 {
		return 0, tx.Commit(ctx)
	}

	releasedSeats := make(map[string][]string)
	for _, o := range expired {
		if _, err := tx.Exec(ctx, `UPDATE orders SET status = 'EXPIRED', updated_at = NOW() WHERE id = $1::uuid`, o.orderID); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(ctx, `UPDATE tickets SET status = 'CANCELLED', updated_at = NOW() WHERE order_id = $1::uuid AND status = 'PENDING'`, o.orderID); err != nil {
			return 0, err
		}
		sRows, err := tx.Query(ctx, `SELECT seat_id::text FROM order_items WHERE order_id = $1::uuid`, o.orderID)
		if err != nil {
			return 0, err
		}
		for sRows.Next() {
			var sid string
			if err := sRows.Scan(&sid); err != nil {
				sRows.Close()
				return 0, err
			}
			releasedSeats[o.eventID] = append(releasedSeats[o.eventID], sid)
		}
		sRows.Close()
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}

	if d.rdb != nil {
		for evID, sids := range releasedSeats {
			var deltas []models.SeatDelta
			for _, sid := range sids {
				k := models.RedisSeatLockKey(evID, sid)
				if cur, err := d.rdb.Get(ctx, k).Result(); err == nil && strings.HasPrefix(cur, "CHECKOUT|") {
					_ = d.rdb.Del(ctx, k).Err()
					_ = d.rdb.ZRem(ctx, models.RedisHoldZSetKey(evID), sid).Err()
					deltas = append(deltas, models.SeatDelta{SeatID: sid, Status: models.SeatAvailable})
				}
			}
			if len(deltas) > 0 {
				seq, _ := d.rdb.Incr(ctx, models.RedisSeatSeqKey(evID)).Result()
				frame, _ := json.Marshal(models.WebSocketEnvelope{
					Type:      "DELTA",
					EventID:   evID,
					Seq:       seq,
					ServerNow: time.Now().UTC(),
					Seats:     deltas,
				})
				_ = d.rdb.Publish(ctx, models.RedisSeatPubSubChannel(evID), frame).Err()
			}
		}
	}

	return len(expired), nil
}
