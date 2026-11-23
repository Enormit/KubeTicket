package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/Enormit/kubeticket/services/pkg/models"
	"github.com/Enormit/kubeticket/services/pkg/observability"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

const (
	defaultSendBufSize   = 64
	defaultFlushInterval = 75 * time.Millisecond
	writeWait            = 5 * time.Second
	pongWait             = 60 * time.Second
	pingPeriod           = 25 * time.Second
)

type client struct {
	hub     *Hub
	eventID string
	conn    *websocket.Conn
	send    chan []byte
	once    sync.Once
}

func (c *client) close() {
	c.once.Do(func() {
		close(c.send)
	})
}

type room struct {
	clients map[*client]struct{}
	pending map[string]models.SeatDelta
	maxSeq  int64
}

// Hub fans out Redis Pub/Sub seat updates to WebSocket rooms with 75ms delta batching
// and bounded slow-consumer eviction.
type Hub struct {
	rdb           redis.UniversalClient
	logger        *slog.Logger
	metrics       *observability.Metrics
	allowedOrigin []string
	upgrader      websocket.Upgrader
	flushInterval time.Duration
	sendBufSize   int

	mu    sync.Mutex
	rooms map[string]*room
}

// NewHub constructs a WebSocket Hub.
func NewHub(rdb redis.UniversalClient, logger *slog.Logger, metrics *observability.Metrics, allowedOrigins []string) *Hub {
	h := &Hub{
		rdb:           rdb,
		logger:        logger,
		metrics:       metrics,
		allowedOrigin: allowedOrigins,
		flushInterval: defaultFlushInterval,
		sendBufSize:   defaultSendBufSize,
		rooms:         make(map[string]*room),
	}
	h.upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 4096,
		CheckOrigin:     h.checkOrigin,
	}
	return h
}

func (h *Hub) checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" || len(h.allowedOrigin) == 0 {
		return true
	}
	return slices.Contains(h.allowedOrigin, "*") || slices.Contains(h.allowedOrigin, origin)
}

// Run subscribes to Redis Pub/Sub seat update channels and flushes batched deltas until ctx closes.
func (h *Hub) Run(ctx context.Context) {
	pubsub := h.rdb.PSubscribe(ctx, "seat:updates:*")
	defer func() { _ = pubsub.Close() }()

	ch := pubsub.Channel()
	ticker := time.NewTicker(h.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			h.Shutdown()
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			h.handlePubSubMessage(msg.Payload)
		case <-ticker.C:
			h.flushAllRooms()
		}
	}
}

func (h *Hub) handlePubSubMessage(payload string) {
	var env models.WebSocketEnvelope
	if err := json.Unmarshal([]byte(payload), &env); err != nil || env.EventID == "" {
		return
	}

	// Non-DELTA envelopes (e.g. ORDER_CONFIRMED) bypass delta coalescing and deliver immediately.
	if env.Type != "DELTA" {
		raw, err := json.Marshal(env)
		if err == nil {
			h.broadcastRaw(env.EventID, raw)
		}
		return
	}

	h.mu.Lock()
	rm, exists := h.rooms[env.EventID]
	if !exists || len(rm.clients) == 0 {
		h.mu.Unlock()
		return
	}
	if rm.pending == nil {
		rm.pending = make(map[string]models.SeatDelta)
	}
	for _, d := range env.Seats {
		rm.pending[d.SeatID] = d
	}
	if env.Seq > rm.maxSeq {
		rm.maxSeq = env.Seq
	}
	h.mu.Unlock()
}

func (h *Hub) flushAllRooms() {
	type outbound struct {
		eventID string
		clients []*client
		data    []byte
	}
	var toSend []outbound

	h.mu.Lock()
	now := time.Now().UTC()
	for evID, rm := range h.rooms {
		if len(rm.pending) == 0 || len(rm.clients) == 0 {
			rm.pending = nil
			continue
		}
		seats := make([]models.SeatDelta, 0, len(rm.pending))
		for _, d := range rm.pending {
			seats = append(seats, d)
		}
		env := models.WebSocketEnvelope{
			Type:      "DELTA",
			EventID:   evID,
			Seq:       rm.maxSeq,
			ServerNow: now,
			Seats:     seats,
		}
		raw, err := json.Marshal(env)
		rm.pending = nil
		if err != nil {
			continue
		}
		clients := make([]*client, 0, len(rm.clients))
		for c := range rm.clients {
			clients = append(clients, c)
		}
		toSend = append(toSend, outbound{eventID: evID, clients: clients, data: raw})
	}
	h.mu.Unlock()

	for _, item := range toSend {
		h.sendToClients(item.eventID, item.clients, item.data)
	}
}

func (h *Hub) broadcastRaw(eventID string, raw []byte) {
	h.mu.Lock()
	rm, exists := h.rooms[eventID]
	if !exists || len(rm.clients) == 0 {
		h.mu.Unlock()
		return
	}
	clients := make([]*client, 0, len(rm.clients))
	for c := range rm.clients {
		clients = append(clients, c)
	}
	h.mu.Unlock()

	h.sendToClients(eventID, clients, raw)
}

func (h *Hub) sendToClients(eventID string, clients []*client, data []byte) {
	var slow []*client
	for _, c := range clients {
		select {
		case c.send <- data:
		default:
			// Bounded channel full -> evict slow consumer rather than blocking room broadcast.
			slow = append(slow, c)
		}
	}
	for _, c := range slow {
		h.unregister(c)
		_ = c.conn.Close()
		if h.logger != nil {
			h.logger.Warn("evicted slow websocket consumer", slog.String("event_id", eventID))
		}
	}
}

// ServeWS upgrades HTTP requests to WebSocket on /ws/events/{id} and emits initial SYNC frame.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "id")
	if eventID == "" {
		eventID = r.URL.Query().Get("event_id")
	}
	if eventID == "" {
		http.Error(w, "event_id required", http.StatusBadRequest)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	var seq int64
	if h.rdb != nil {
		if v, err := h.rdb.Get(r.Context(), models.RedisSeatSeqKey(eventID)).Int64(); err == nil {
			seq = v
		}
	}

	c := &client{
		hub:     h,
		eventID: eventID,
		conn:    conn,
		send:    make(chan []byte, h.sendBufSize),
	}
	h.register(c)

	syncFrame, _ := json.Marshal(models.WebSocketEnvelope{
		Type:      "SYNC",
		EventID:   eventID,
		Seq:       seq,
		ServerNow: time.Now().UTC(),
	})
	c.send <- syncFrame

	go c.writePump()
	c.readPump()
}

func (h *Hub) register(c *client) {
	h.mu.Lock()
	rm, exists := h.rooms[c.eventID]
	if !exists {
		rm = &room{clients: make(map[*client]struct{})}
		h.rooms[c.eventID] = rm
	}
	rm.clients[c] = struct{}{}
	h.mu.Unlock()

	if h.metrics != nil {
		h.metrics.WSActiveConnections.WithLabelValues(c.eventID).Inc()
	}
}

func (h *Hub) unregister(c *client) {
	h.mu.Lock()
	rm, exists := h.rooms[c.eventID]
	removed := false
	if exists {
		if _, ok := rm.clients[c]; ok {
			delete(rm.clients, c)
			removed = true
		}
		if len(rm.clients) == 0 {
			delete(h.rooms, c.eventID)
		}
	}
	h.mu.Unlock()

	if removed {
		c.close()
		if h.metrics != nil {
			h.metrics.WSActiveConnections.WithLabelValues(c.eventID).Dec()
		}
	}
}

// Shutdown gracefully sends CloseGoingAway to all active WebSocket clients.
func (h *Hub) Shutdown() {
	h.mu.Lock()
	var all []*client
	for _, rm := range h.rooms {
		for c := range rm.clients {
			all = append(all, c)
		}
	}
	h.rooms = make(map[string]*room)
	h.mu.Unlock()

	deadline := time.Now().Add(time.Second)
	for _, c := range all {
		_ = c.conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseGoingAway, "server shutting down"),
			deadline,
		)
		c.close()
		_ = c.conn.Close()
	}
}

func (c *client) readPump() {
	defer func() {
		c.hub.unregister(c)
		_ = c.conn.Close()
	}()
	c.conn.SetReadLimit(1024)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		var req struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(msg, &req) == nil && req.Type == "PING" {
			pong, _ := json.Marshal(models.WebSocketEnvelope{
				Type:      "PONG",
				EventID:   c.eventID,
				ServerNow: time.Now().UTC(),
			})
			select {
			case c.send <- pong:
			default:
			}
		}
	}
}

func (c *client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
