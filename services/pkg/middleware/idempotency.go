package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type cachedIdempotencyRecord struct {
	RequestHash string          `json:"request_hash"`
	Status      int             `json:"status"`
	Body        json.RawMessage `json:"body"`
}

type captureWriter struct {
	http.ResponseWriter
	status int
	buf    bytes.Buffer
}

func (cw *captureWriter) WriteHeader(code int) {
	cw.status = code
	cw.ResponseWriter.WriteHeader(code)
}

func (cw *captureWriter) Write(p []byte) (int, error) {
	cw.buf.Write(p)
	return cw.ResponseWriter.Write(p)
}

// Idempotency deduplicates mutating requests using the Idempotency-Key header in Redis.
// When required is true, requests missing Idempotency-Key fail with 400 Bad Request.
func Idempotency(client redis.UniversalClient, ttl time.Duration, required bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if client == nil || (r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch) {
				next.ServeHTTP(w, r)
				return
			}

			keyHeader := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
			if keyHeader == "" {
				if required {
					WriteError(w, http.StatusBadRequest, "missing_idempotency_key", "Idempotency-Key header is required")
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
			if err != nil {
				WriteError(w, http.StatusBadRequest, "invalid_body", "failed to read request body")
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

			sum := sha256.Sum256(bodyBytes)
			reqHash := hex.EncodeToString(sum[:])

			subject := clientIP(r)
			if u, ok := UserFromContext(r.Context()); ok {
				subject = u.ID
			}
			redisKey := fmt.Sprintf("idem:%s:%s:%s", subject, r.URL.Path, keyHeader)
			lockKey := redisKey + ":lock"

			if raw, err := client.Get(r.Context(), redisKey).Bytes(); err == nil {
				var rec cachedIdempotencyRecord
				if jsonErr := json.Unmarshal(raw, &rec); jsonErr == nil {
					if rec.RequestHash != reqHash {
						WriteError(w, http.StatusConflict, "idempotency_mismatch", "Idempotency-Key reused with different payload")
						return
					}
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					w.Header().Set("X-Idempotent-Replay", "true")
					w.WriteHeader(rec.Status)
					_, _ = w.Write(rec.Body)
					return
				}
			}

			acquired, err := client.SetNX(r.Context(), lockKey, reqHash, 30*time.Second).Result()
			if err == nil && !acquired {
				WriteError(w, http.StatusConflict, "request_in_progress", "identical idempotent request is currently executing")
				return
			}
			defer func() {
				_ = client.Del(r.Context(), lockKey).Err()
			}()

			cw := &captureWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(cw, r)

			if cw.status >= 200 && cw.status < 500 {
				rec := cachedIdempotencyRecord{
					RequestHash: reqHash,
					Status:      cw.status,
					Body:        json.RawMessage(cw.buf.Bytes()),
				}
				if encoded, err := json.Marshal(rec); err == nil {
					_ = client.Set(r.Context(), redisKey, encoded, ttl).Err()
				}
			}
		})
	}
}
