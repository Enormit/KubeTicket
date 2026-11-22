package service

import (
	"context"
	"time"

	"github.com/Enormit/kubeticket/services/gateway-api/internal/repo"
	"github.com/Enormit/kubeticket/services/pkg/models"
	"github.com/redis/go-redis/v9"
)

const redisMGetChunkSize = 500

// EventService serves event listings and merged DB + Redis seat snapshots.
type EventService struct {
	repo repo.EventRepository
	rdb  redis.UniversalClient
}

// NewEventService constructs an EventService.
func NewEventService(r repo.EventRepository, rdb redis.UniversalClient) *EventService {
	return &EventService{repo: r, rdb: rdb}
}

// ListEvents returns all public events.
func (s *EventService) ListEvents(ctx context.Context) ([]models.Event, error) {
	return s.repo.ListEvents(ctx)
}

// GetEventWithZones returns event metadata and its pricing zones.
func (s *EventService) GetEventWithZones(ctx context.Context, idOrSlug string) (*models.Event, []models.Zone, error) {
	return s.repo.GetEventWithZones(ctx, idOrSlug)
}

// GetSeatSnapshot combines PostgreSQL seat records with live Redis lock states and sequence number.
func (s *EventService) GetSeatSnapshot(ctx context.Context, idOrSlug string) (*models.SeatSnapshotResponse, error) {
	ev, zones, err := s.repo.GetEventWithZones(ctx, idOrSlug)
	if err != nil {
		return nil, err
	}

	seats, err := s.repo.ListEventSeats(ctx, ev.ID)
	if err != nil {
		return nil, err
	}

	var seq int64
	if s.rdb != nil {
		if v, err := s.rdb.Get(ctx, models.RedisSeatSeqKey(ev.ID)).Int64(); err == nil {
			seq = v
		}
		s.overlayRedisLocks(ctx, ev.ID, seats)
	}

	return &models.SeatSnapshotResponse{
		EventID:   ev.ID,
		Seq:       seq,
		ServerNow: time.Now().UTC(),
		Zones:     zones,
		Seats:     seats,
	}, nil
}

func (s *EventService) overlayRedisLocks(ctx context.Context, eventID string, seats []models.SeatState) {
	if len(seats) == 0 {
		return
	}

	nowMs := time.Now().UnixMilli()
	for start := 0; start < len(seats); start += redisMGetChunkSize {
		end := min(start+redisMGetChunkSize, len(seats))
		keys := make([]string, end-start)
		for i := start; i < end; i++ {
			keys[i-start] = models.RedisSeatLockKey(eventID, seats[i].ID)
		}

		vals, err := s.rdb.MGet(ctx, keys...).Result()
		if err != nil {
			return
		}

		for i, raw := range vals {
			str, ok := raw.(string)
			if !ok || str == "" {
				continue
			}
			rec, ok := models.ParseSeatLockRecord(str)
			if !ok {
				continue
			}
			idx := start + i
			// Sold persisted in DB or Redis always wins.
			if seats[idx].Status == models.SeatSold || rec.Status == models.SeatSold {
				seats[idx].Status = models.SeatSold
				seats[idx].ExpiresAt = nil
				continue
			}
			if rec.ExpiresUnixMs > 0 && rec.ExpiresUnixMs <= nowMs {
				continue
			}
			if rec.Status == models.SeatHeld || rec.Status == models.SeatCheckout {
				seats[idx].Status = rec.Status
				if rec.ExpiresUnixMs > 0 {
					exp := time.UnixMilli(rec.ExpiresUnixMs).UTC()
					seats[idx].ExpiresAt = &exp
				}
			}
		}
	}
}
