package middleware

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

var rateLimitScript = redis.NewScript(`
local current = redis.call("INCR", KEYS[1])
if current == 1 then
    redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
return current
`)

// RateLimit enforces a Redis-backed fixed-window request quota per user or IP.
func RateLimit(client redis.UniversalClient, prefix string, limit int, window time.Duration) func(http.Handler) http.Handler {
	windowMS := window.Milliseconds()
	if windowMS <= 0 {
		windowMS = 60000
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if client == nil || limit <= 0 {
				next.ServeHTTP(w, r)
				return
			}

			subject := clientIP(r)
			if u, ok := UserFromContext(r.Context()); ok {
				subject = "u:" + u.ID
			}
			key := fmt.Sprintf("rl:%s:%s", prefix, subject)

			count, err := rateLimitScript.Run(r.Context(), client, []string{key}, windowMS).Int()
			if err != nil {
				// Fail open on transient Redis issues rather than dropping legitimate traffic.
				next.ServeHTTP(w, r)
				return
			}

			remaining := max(0, limit-count)
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))

			if count > limit {
				WriteError(w, http.StatusTooManyRequests, "rate_limited", "too many requests, please slow down")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first, _, _ := strings.Cut(xff, ",")
		if trimmed := strings.TrimSpace(first); trimmed != "" {
			return trimmed
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}
