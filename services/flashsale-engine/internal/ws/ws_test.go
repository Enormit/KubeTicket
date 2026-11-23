package ws

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Enormit/kubeticket/services/pkg/models"
	"github.com/Enormit/kubeticket/services/pkg/observability"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

func TestWebSocketHubSyncAndBatchedDeltas(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	_ = rdb.Set(context.Background(), models.RedisSeatSeqKey("ev-1"), 7, 0).Err()

	metrics := observability.NewMetrics("flashsale-ws-test")
	hub := NewHub(rdb, nil, metrics, []string{"*"})
	hub.flushInterval = 30 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	r := chi.NewRouter()
	r.Get("/ws/events/{id}", hub.ServeWS)
	srv := httptest.NewServer(r)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/events/ev-1"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	// 1. Expect initial SYNC frame with seq=7
	var syncEnv models.WebSocketEnvelope
	if err := conn.ReadJSON(&syncEnv); err != nil {
		t.Fatal(err)
	}
	if syncEnv.Type != "SYNC" || syncEnv.Seq != 7 {
		t.Fatalf("expected SYNC seq=7, got %+v", syncEnv)
	}

	// Give PubSub goroutine brief time to establish PSubscribe before publishing.
	time.Sleep(40 * time.Millisecond)

	// 2. Publish 2 rapid updates to same seat + 1 update to another seat -> coalesced into 1 DELTA
	p1, _ := json.Marshal(models.WebSocketEnvelope{
		Type: "DELTA", EventID: "ev-1", Seq: 8,
		Seats: []models.SeatDelta{{SeatID: "s1", Status: models.SeatHeld}},
	})
	p2, _ := json.Marshal(models.WebSocketEnvelope{
		Type: "DELTA", EventID: "ev-1", Seq: 9,
		Seats: []models.SeatDelta{
			{SeatID: "s1", Status: models.SeatSold},
			{SeatID: "s2", Status: models.SeatHeld},
		},
	})
	_ = rdb.Publish(ctx, models.RedisSeatPubSubChannel("ev-1"), p1).Err()
	_ = rdb.Publish(ctx, models.RedisSeatPubSubChannel("ev-1"), p2).Err()

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var deltaEnv models.WebSocketEnvelope
	if err := conn.ReadJSON(&deltaEnv); err != nil {
		t.Fatalf("failed reading batched delta: %v", err)
	}
	if deltaEnv.Type != "DELTA" || deltaEnv.Seq != 9 || len(deltaEnv.Seats) != 2 {
		t.Fatalf("unexpected batched delta: %+v", deltaEnv)
	}

	statuses := map[string]string{}
	for _, s := range deltaEnv.Seats {
		statuses[s.SeatID] = s.Status
	}
	if statuses["s1"] != models.SeatSold || statuses["s2"] != models.SeatHeld {
		t.Fatalf("expected coalesced latest statuses, got %v", statuses)
	}
}

func TestWebSocketSlowConsumerEviction(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	hub := NewHub(rdb, nil, nil, []string{"*"})
	hub.sendBufSize = 2

	r := chi.NewRouter()
	r.Get("/ws/events/{id}", hub.ServeWS)
	srv := httptest.NewServer(r)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/events/ev-slow"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Wait for registration & SYNC message
	time.Sleep(30 * time.Millisecond)

	// Flood send channel beyond sendBufSize without reading on client side
	for i := 0; i < 6; i++ {
		hub.broadcastRaw("ev-slow", []byte(`{"type":"ORDER_CONFIRMED"}`))
	}

	// Verify client was evicted from room
	hub.mu.Lock()
	rm := hub.rooms["ev-slow"]
	hub.mu.Unlock()
	if rm != nil && len(rm.clients) > 0 {
		t.Fatalf("expected slow consumer to be evicted")
	}
}
