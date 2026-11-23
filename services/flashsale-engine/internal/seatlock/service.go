package seatlock

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/Enormit/kubeticket/services/pkg/database"
	"github.com/Enormit/kubeticket/services/pkg/models"
	"github.com/Enormit/kubeticket/services/pkg/observability"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

//go:embed lua/acquire_holds.lua
var acquireHoldsLua string

//go:embed lua/release_hold.lua
var releaseHoldLua string

//go:embed lua/promote_checkout.lua
var promoteCheckoutLua string

//go:embed lua/expire_seat.lua
var expireSeatLua string

var (
	ErrSeatConflict    = errors.New("one or more seats are no longer available")
	ErrHoldForbidden   = errors.New("seat is not held by requesting user")
	ErrHoldExpired     = errors.New("seat hold expired or missing")
	ErrTooManySeats    = errors.New("cannot hold more than 4 seats per request")
	ErrEmptySeatList   = errors.New("at least 1 seat_id is required")
)

// HoldResult returns lock details and synchronized expiration timestamp.
type HoldResult struct {
	HoldToken string    `json:"hold_token"`
	EventID   string    `json:"event_id"`
	SeatIDs   []string  `json:"seat_ids"`
	Seq       int64     `json:"seq"`
	ServerNow time.Time `json:"server_now"`
	ExpiresAt time.Time `json:"expires_at"`
}

// CheckoutResult returns the created async order reference.
type CheckoutResult struct {
	OrderID   string    `json:"order_id"`
	EventID   string    `json:"event_id"`
	SeatIDs   []string  `json:"seat_ids"`
	Status    string    `json:"status"`
	Seq       int64     `json:"seq"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Service orchestrates Lua seat locks, ZSET expiry sweeps, DB hydration, and order queuing.
type Service struct {
	rdb             redis.UniversalClient
	scripts         *database.ScriptRegistry
	publisher       OrderPublisher
	metrics         *observability.Metrics
	seatHoldTTL     time.Duration
	checkoutHoldTTL time.Duration
	instanceID      string
	hydrated        atomic.Bool
}

// NewService constructs a seatlock Service.
func NewService(
	rdb redis.UniversalClient,
	publisher OrderPublisher,
	metrics *observability.Metrics,
	seatHoldTTL, checkoutHoldTTL time.Duration,
) *Service {
	if seatHoldTTL <= 0 {
		seatHoldTTL = 300 * time.Second
	}
	if checkoutHoldTTL < seatHoldTTL {
		checkoutHoldTTL = 900 * time.Second
	}
	scripts := database.NewScriptRegistry(map[string]string{
		"acquire": acquireHoldsLua,
		"release": releaseHoldLua,
		"promote": promoteCheckoutLua,
		"expire":  expireSeatLua,
	})
	return &Service{
		rdb:             rdb,
		scripts:         scripts,
		publisher:       publisher,
		metrics:         metrics,
		seatHoldTTL:     seatHoldTTL,
		checkoutHoldTTL: checkoutHoldTTL,
		instanceID:      uuid.NewString(),
	}
}

// IsHydrated reports whether cold-start DB->Redis synchronization has completed.
func (s *Service) IsHydrated() bool {
	return s.hydrated.Load()
}

// MarkHydrated sets hydration state directly (useful in unit tests without Postgres).
func (s *Service) MarkHydrated(ready bool) {
	s.hydrated.Store(ready)
}

// HydrateFromPostgres loads active PENDING/PAID/CHECKED_IN seats into Redis before readiness passes.
func (s *Service) HydrateFromPostgres(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	if pool == nil {
		return 0, errors.New("postgres pool is nil")
	}

	const q = `
		SELECT t.event_id::text, t.seat_id::text, t.user_id::text, t.order_id::text, t.status,
		       COALESCE((EXTRACT(EPOCH FROM o.expires_at) * 1000)::bigint, 0)
		FROM tickets t
		JOIN orders o ON o.id = t.order_id
		WHERE t.status IN ('PENDING', 'PAID', 'CHECKED_IN')
		UNION ALL
		SELECT s.event_id::text, s.id::text, '', '', 'SOLD', 0
		FROM seats s
		WHERE s.status = 'SOLD'`

	rows, err := pool.Query(ctx, q)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	nowMs := time.Now().UnixMilli()
	restored := 0
	for rows.Next() {
		var eventID, seatID, userID, orderID, status string
		var expMs int64
		if err := rows.Scan(&eventID, &seatID, &userID, &orderID, &status, &expMs); err != nil {
			return restored, err
		}

		key := models.RedisSeatLockKey(eventID, seatID)
		if status == models.TicketPaid || status == models.TicketCheckedIn || status == models.SeatSold {
			val := models.SeatLockRecord{
				Status:        models.SeatSold,
				UserID:        userID,
				RefID:         orderID,
				ExpiresUnixMs: 0,
			}.Encode()
			_ = s.rdb.Set(ctx, key, val, 0).Err()
			restored++
		} else if status == models.TicketPending && expMs > nowMs {
			ttl := time.Duration(expMs-nowMs) * time.Millisecond
			val := models.SeatLockRecord{
				Status:        models.SeatCheckout,
				UserID:        userID,
				RefID:         orderID,
				ExpiresUnixMs: expMs,
			}.Encode()
			_ = s.rdb.SetNX(ctx, key, val, ttl).Err()
			restored++
		}
	}
	if err := rows.Err(); err != nil {
		return restored, err
	}

	s.hydrated.Store(true)
	return restored, nil
}

// AcquireHolds atomically locks up to 4 seats for userID and broadcasts HELD deltas.
func (s *Service) AcquireHolds(ctx context.Context, userID, eventID string, seatIDs []string) (*HoldResult, error) {
	seatIDs = dedupeNonEmpty(seatIDs)
	if len(seatIDs) == 0 {
		return nil, ErrEmptySeatList
	}
	if len(seatIDs) > 4 {
		return nil, ErrTooManySeats
	}

	now := time.Now().UTC()
	exp := now.Add(s.seatHoldTTL)
	expMs := exp.UnixMilli()
	holdToken := uuid.NewString()

	lockVal := models.SeatLockRecord{
		Status:        models.SeatHeld,
		UserID:        userID,
		RefID:         holdToken,
		ExpiresUnixMs: expMs,
	}.Encode()

	keys := make([]string, 0, len(seatIDs)+3)
	for _, sid := range seatIDs {
		keys = append(keys, models.RedisSeatLockKey(eventID, sid))
	}
	keys = append(keys,
		models.RedisHoldZSetKey(eventID),
		models.RedisActiveHoldEventsSetKey(),
		models.RedisSeatSeqKey(eventID),
	)

	args := make([]any, 0, 4+len(seatIDs))
	args = append(args, lockVal, int(s.seatHoldTTL.Seconds()), expMs, eventID)
	for _, sid := range seatIDs {
		args = append(args, sid)
	}

	res, err := s.scripts.Run(ctx, s.rdb, "acquire", keys, args...).Slice()
	if err != nil {
		return nil, err
	}

	okCode := toInt64(res[0])
	if okCode != 1 {
		if s.metrics != nil {
			s.metrics.SeatLockConflicts.WithLabelValues(eventID).Inc()
		}
		return nil, fmt.Errorf("%w: seat %v", ErrSeatConflict, res[1])
	}

	seq := toInt64(res[1])
	s.publishDeltas(ctx, eventID, seq, seatIDs, models.SeatHeld, &exp)

	return &HoldResult{
		HoldToken: holdToken,
		EventID:   eventID,
		SeatIDs:   seatIDs,
		Seq:       seq,
		ServerNow: now,
		ExpiresAt: exp,
	}, nil
}

// ReleaseHold unlocks a HELD seat if owned by userID and publishes AVAILABLE.
func (s *Service) ReleaseHold(ctx context.Context, userID, eventID, seatID string) error {
	keys := []string{
		models.RedisSeatLockKey(eventID, seatID),
		models.RedisHoldZSetKey(eventID),
		models.RedisSeatSeqKey(eventID),
	}
	res, err := s.scripts.Run(ctx, s.rdb, "release", keys, userID, seatID).Slice()
	if err != nil {
		return err
	}

	code := toInt64(res[0])
	switch code {
	case -1:
		return ErrHoldForbidden
	case 0:
		return ErrHoldExpired
	default:
		seq := toInt64(res[1])
		s.publishDeltas(ctx, eventID, seq, []string{seatID}, models.SeatAvailable, nil)
		return nil
	}
}

// SubmitCheckout promotes HELD seats to CHECKOUT TTL and enqueues the order asynchronously.
func (s *Service) SubmitCheckout(ctx context.Context, userID, eventID, holdToken, paymentProvider string, seatIDs []string) (*CheckoutResult, error) {
	seatIDs = dedupeNonEmpty(seatIDs)
	if len(seatIDs) == 0 {
		return nil, ErrEmptySeatList
	}
	if len(seatIDs) > 4 {
		return nil, ErrTooManySeats
	}
	if paymentProvider == "" {
		paymentProvider = "stripe"
	}

	orderID := uuid.NewString()
	now := time.Now().UTC()
	exp := now.Add(s.checkoutHoldTTL)
	expMs := exp.UnixMilli()

	checkoutVal := models.SeatLockRecord{
		Status:        models.SeatCheckout,
		UserID:        userID,
		RefID:         orderID,
		ExpiresUnixMs: expMs,
	}.Encode()

	keys := make([]string, 0, len(seatIDs)+2)
	for _, sid := range seatIDs {
		keys = append(keys, models.RedisSeatLockKey(eventID, sid))
	}
	keys = append(keys,
		models.RedisHoldZSetKey(eventID),
		models.RedisSeatSeqKey(eventID),
	)

	args := make([]any, 0, 4+len(seatIDs))
	args = append(args, userID, checkoutVal, int(s.checkoutHoldTTL.Seconds()), expMs)
	for _, sid := range seatIDs {
		args = append(args, sid)
	}

	res, err := s.scripts.Run(ctx, s.rdb, "promote", keys, args...).Slice()
	if err != nil {
		return nil, err
	}
	if toInt64(res[0]) != 1 {
		return nil, fmt.Errorf("%w: seat %v", ErrHoldExpired, res[1])
	}
	seq := toInt64(res[1])

	msg := models.OrderQueueMessage{
		OrderID:         orderID,
		UserID:          userID,
		EventID:         eventID,
		HoldToken:       holdToken,
		SeatIDs:         seatIDs,
		PaymentProvider: paymentProvider,
		RequestedAt:     now,
	}
	if s.publisher != nil {
		if err := s.publisher.PublishOrder(ctx, msg); err != nil {
			return nil, fmt.Errorf("enqueue order: %w", err)
		}
	}

	s.publishDeltas(ctx, eventID, seq, seatIDs, models.SeatCheckout, &exp)

	return &CheckoutResult{
		OrderID:   orderID,
		EventID:   eventID,
		SeatIDs:   seatIDs,
		Status:    models.OrderPending,
		Seq:       seq,
		ExpiresAt: exp,
	}, nil
}

// SweepExpiredHolds cleans expired ZSET seat locks under a distributed leader lock and broadcasts AVAILABLE.
func (s *Service) SweepExpiredHolds(ctx context.Context) (int, error) {
	events, err := s.rdb.SMembers(ctx, models.RedisActiveHoldEventsSetKey()).Result()
	if err != nil || len(events) == 0 {
		return 0, err
	}

	nowMs := time.Now().UnixMilli()
	releasedTotal := 0

	for _, evID := range events {
		leaderKey := fmt.Sprintf("lock:sweeper:%s", evID)
		acquired, err := s.rdb.SetNX(ctx, leaderKey, s.instanceID, 1500*time.Millisecond).Result()
		if err != nil || !acquired {
			continue
		}

		zKey := models.RedisHoldZSetKey(evID)
		expiredSeatIDs, err := s.rdb.ZRangeByScore(ctx, zKey, &redis.ZRangeBy{
			Min:   "-inf",
			Max:   strconv.FormatInt(nowMs, 10),
			Count: 200,
		}).Result()
		if err != nil {
			continue
		}

		for _, seatID := range expiredSeatIDs {
			keys := []string{
				models.RedisSeatLockKey(evID, seatID),
				zKey,
				models.RedisSeatSeqKey(evID),
			}
			res, err := s.scripts.Run(ctx, s.rdb, "expire", keys, nowMs, seatID).Slice()
			if err != nil || len(res) < 2 {
				continue
			}
			if toInt64(res[0]) == 1 {
				seq := toInt64(res[1])
				s.publishDeltas(ctx, evID, seq, []string{seatID}, models.SeatAvailable, nil)
				releasedTotal++
			}
		}
	}

	return releasedTotal, nil
}

// RunSweeperLoop runs SweepExpiredHolds on a fixed interval until ctx cancels.
func (s *Service) RunSweeperLoop(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = s.SweepExpiredHolds(ctx)
		}
	}
}

func (s *Service) publishDeltas(ctx context.Context, eventID string, seq int64, seatIDs []string, status string, exp *time.Time) {
	deltas := make([]models.SeatDelta, 0, len(seatIDs))
	for _, sid := range seatIDs {
		deltas = append(deltas, models.SeatDelta{
			SeatID:    sid,
			Status:    status,
			ExpiresAt: exp,
		})
	}
	env := models.WebSocketEnvelope{
		Type:      "DELTA",
		EventID:   eventID,
		Seq:       seq,
		ServerNow: time.Now().UTC(),
		Seats:     deltas,
	}
	if raw, err := json.Marshal(env); err == nil {
		_ = s.rdb.Publish(ctx, models.RedisSeatPubSubChannel(eventID), raw).Err()
	}
}

func dedupeNonEmpty(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		if _, exists := seen[item]; !exists {
			seen[item] = struct{}{}
			out = append(out, item)
		}
	}
	return out
}

func toInt64(v any) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case string:
		n, _ := strconv.ParseInt(t, 10, 64)
		return n
	default:
		return 0
	}
}
