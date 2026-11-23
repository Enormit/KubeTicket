package waitingroom

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Enormit/kubeticket/services/pkg/middleware"
	"github.com/Enormit/kubeticket/services/pkg/models"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

var (
	ErrTurnstileFailed   = errors.New("human verification failed")
	ErrNotInWaitingRoom  = errors.New("user has not joined waiting room")
	ErrAdmissionRequired = errors.New("valid waiting room admission token required")
)

// TurnstileVerifier verifies anti-bot tokens (Cloudflare Turnstile / hCaptcha).
type TurnstileVerifier interface {
	Verify(ctx context.Context, token, remoteIP string) error
}

// HTTPTurnstileVerifier calls Cloudflare Turnstile's siteverify API when secretKey is set.
type HTTPTurnstileVerifier struct {
	secretKey  string
	verifyURL  string
	httpClient *http.Client
}

// NewTurnstileVerifier creates a Turnstile verifier (no-op when secretKey is empty for local dev).
func NewTurnstileVerifier(secretKey string) *HTTPTurnstileVerifier {
	return &HTTPTurnstileVerifier{
		secretKey:  strings.TrimSpace(secretKey),
		verifyURL:  "https://challenges.cloudflare.com/turnstile/v0/siteverify",
		httpClient: &http.Client{Timeout: 4 * time.Second},
	}
}

func (v *HTTPTurnstileVerifier) Verify(ctx context.Context, token, remoteIP string) error {
	if v.secretKey == "" {
		return nil
	}
	if strings.TrimSpace(token) == "" {
		return ErrTurnstileFailed
	}

	form := url.Values{
		"secret":   {v.secretKey},
		"response": {token},
	}
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.verifyURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrTurnstileFailed, err)
	}
	defer resp.Body.Close()

	var result struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || !result.Success {
		return ErrTurnstileFailed
	}
	return nil
}

// QueueStatus represents a user's live waiting room state.
type QueueStatus struct {
	EventID          string     `json:"event_id"`
	Position         int64      `json:"position"`
	AheadCount       int64      `json:"ahead_count"`
	EstimatedWaitSec int64      `json:"estimated_wait_sec"`
	Admitted         bool       `json:"admitted"`
	AdmissionToken   string     `json:"admission_token,omitempty"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
}

type storedToken struct {
	Token     string    `json:"token"`
	JTI       string    `json:"jti"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Service manages fair FIFO queueing, anti-bot checks, and bound admission tokens.
type Service struct {
	rdb              redis.UniversalClient
	verifier         TurnstileVerifier
	tokenSecret      string
	admissionRate    int
	maxActiveUsers   int64
	tokenTTL         time.Duration
}

// NewService creates a waiting room service.
func NewService(rdb redis.UniversalClient, verifier TurnstileVerifier, tokenSecret string, admissionRate int) *Service {
	if admissionRate <= 0 {
		admissionRate = 50
	}
	return &Service{
		rdb:            rdb,
		verifier:       verifier,
		tokenSecret:    tokenSecret,
		admissionRate:  admissionRate,
		maxActiveUsers: int64(admissionRate * 10),
		tokenTTL:       10 * time.Minute,
	}
}

// Join places userID into the event's waiting queue once (ZADD NX) or returns existing admission.
func (s *Service) Join(ctx context.Context, userID, eventID, turnstileToken, remoteIP string) (*QueueStatus, error) {
	if s.verifier != nil {
		if err := s.verifier.Verify(ctx, turnstileToken, remoteIP); err != nil {
			return nil, err
		}
	}

	if existing, ok := s.getValidAdmittedToken(ctx, userID, eventID); ok {
		return &QueueStatus{
			EventID:        eventID,
			Position:       0,
			AheadCount:     0,
			Admitted:       true,
			AdmissionToken: existing.Token,
			ExpiresAt:      &existing.ExpiresAt,
		}, nil
	}

	qKey := models.RedisWaitingQueueKey(eventID)
	score := float64(time.Now().UnixNano())
	_ = s.rdb.ZAddArgs(ctx, qKey, redis.ZAddArgs{
		NX:      true,
		Members: []redis.Z{{Score: score, Member: userID}},
	}).Err()
	_ = s.rdb.SAdd(ctx, "wr:events", eventID).Err()

	return s.Status(ctx, userID, eventID)
}

// Status returns queue position or promotes the user if admission capacity allows.
func (s *Service) Status(ctx context.Context, userID, eventID string) (*QueueStatus, error) {
	if existing, ok := s.getValidAdmittedToken(ctx, userID, eventID); ok {
		return &QueueStatus{
			EventID:        eventID,
			Position:       0,
			AheadCount:     0,
			Admitted:       true,
			AdmissionToken: existing.Token,
			ExpiresAt:      &existing.ExpiresAt,
		}, nil
	}

	qKey := models.RedisWaitingQueueKey(eventID)
	rank, err := s.rdb.ZRank(ctx, qKey, userID).Result()
	if errors.Is(err, redis.Nil) {
		return nil, ErrNotInWaitingRoom
	}
	if err != nil {
		return nil, err
	}

	// Opportunistically admit front of queue if capacity allows.
	if rank < int64(s.admissionRate) {
		if _, err := s.AdmitBatch(ctx, eventID); err == nil {
			if existing, ok := s.getValidAdmittedToken(ctx, userID, eventID); ok {
				return &QueueStatus{
					EventID:        eventID,
					Position:       0,
					AheadCount:     0,
					Admitted:       true,
					AdmissionToken: existing.Token,
					ExpiresAt:      &existing.ExpiresAt,
				}, nil
			}
		}
	}

	// Refresh rank after admission batch
	if newRank, err := s.rdb.ZRank(ctx, qKey, userID).Result(); err == nil {
		rank = newRank
	}

	waitSec := max(int64(1), (rank+1)/int64(s.admissionRate))
	return &QueueStatus{
		EventID:          eventID,
		Position:         rank + 1,
		AheadCount:       rank,
		EstimatedWaitSec: waitSec,
		Admitted:         false,
	}, nil
}

// AdmitBatch pops eligible users from the front of the waiting queue based on active capacity.
func (s *Service) AdmitBatch(ctx context.Context, eventID string) (int, error) {
	activeKey := models.RedisWaitingActiveCheckoutKey(eventID)
	nowMs := float64(time.Now().UnixMilli())
	_ = s.rdb.ZRemRangeByScore(ctx, activeKey, "-inf", fmt.Sprintf("%f", nowMs)).Err()

	activeCount, err := s.rdb.ZCard(ctx, activeKey).Result()
	if err != nil {
		return 0, err
	}
	slots := s.maxActiveUsers - activeCount
	if slots <= 0 {
		return 0, nil
	}
	batchSize := min(int64(s.admissionRate), slots)

	qKey := models.RedisWaitingQueueKey(eventID)
	popped, err := s.rdb.ZPopMin(ctx, qKey, batchSize).Result()
	if err != nil || len(popped) == 0 {
		return 0, err
	}

	tKey := models.RedisWaitingTokenHashKey(eventID)
	admitted := 0
	for _, z := range popped {
		uid, ok := z.Member.(string)
		if !ok || uid == "" {
			continue
		}
		jti := uuid.NewString()
		exp := time.Now().Add(s.tokenTTL).UTC()
		tok, err := middleware.IssueAdmissionToken(s.tokenSecret, s.tokenTTL, uid, eventID, jti)
		if err != nil {
			continue
		}
		encoded, _ := json.Marshal(storedToken{Token: tok, JTI: jti, ExpiresAt: exp})
		pipe := s.rdb.TxPipeline()
		pipe.HSet(ctx, tKey, uid, encoded)
		pipe.ZAdd(ctx, activeKey, redis.Z{Score: float64(exp.UnixMilli()), Member: uid})
		if _, err := pipe.Exec(ctx); err == nil {
			admitted++
		}
	}
	return admitted, nil
}

// RunAdmissionLoop periodically admits queued users across active events.
func (s *Service) RunAdmissionLoop(ctx context.Context, interval time.Duration) {
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
			events, err := s.rdb.SMembers(ctx, "wr:events").Result()
			if err != nil {
				continue
			}
			for _, evID := range events {
				_, _ = s.AdmitBatch(ctx, evID)
			}
		}
	}
}

// ValidateUserPass verifies the X-Admission-Token header against userID, eventID, and Redis permit state.
func (s *Service) ValidateUserPass(ctx context.Context, rawToken, userID, eventID string) error {
	jti, err := middleware.VerifyAdmissionToken(s.tokenSecret, rawToken, userID, eventID)
	if err != nil {
		return ErrAdmissionRequired
	}
	stored, ok := s.getValidAdmittedToken(ctx, userID, eventID)
	if !ok || stored.JTI != jti {
		return ErrAdmissionRequired
	}
	return nil
}

func (s *Service) getValidAdmittedToken(ctx context.Context, userID, eventID string) (storedToken, bool) {
	raw, err := s.rdb.HGet(ctx, models.RedisWaitingTokenHashKey(eventID), userID).Bytes()
	if err != nil {
		return storedToken{}, false
	}
	var st storedToken
	if err := json.Unmarshal(raw, &st); err != nil {
		return storedToken{}, false
	}
	if time.Now().After(st.ExpiresAt) {
		_ = s.rdb.HDel(ctx, models.RedisWaitingTokenHashKey(eventID), userID).Err()
		return storedToken{}, false
	}
	return st, true
}
