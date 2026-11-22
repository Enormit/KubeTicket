package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const AuthCookieName = "kubeticket_token"

type contextKey string

const authUserContextKey contextKey = "auth_user"

// AuthUser represents the authenticated principal extracted from JWT.
type AuthUser struct {
	ID    string
	Email string
	Role  string
}

// Claims represents standard KubeTicket JWT access token claims.
type Claims struct {
	Email string `json:"email"`
	Role  string `json:"role"`
	jwt.RegisteredClaims
}

// AdmissionClaims binds a Waiting Room pass to a single user, event, and token ID (jti).
type AdmissionClaims struct {
	EventID string `json:"event_id"`
	jwt.RegisteredClaims
}

// GenerateToken signs an HMAC-SHA256 JWT for user authentication.
func GenerateToken(secret string, ttl time.Duration, userID, email, role string) (string, error) {
	now := time.Now()
	claims := Claims{
		Email: email,
		Role:  role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ParseToken validates and decodes a user authentication JWT.
func ParseToken(secret, rawToken string) (*AuthUser, error) {
	token, err := jwt.ParseWithClaims(rawToken, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid || claims.Subject == "" {
		return nil, errors.New("invalid token claims")
	}
	return &AuthUser{
		ID:    claims.Subject,
		Email: claims.Email,
		Role:  claims.Role,
	}, nil
}

// IssueAdmissionToken mints a short-lived Waiting Room pass bound to userID + eventID + jti.
func IssueAdmissionToken(secret string, ttl time.Duration, userID, eventID, jti string) (string, error) {
	now := time.Now()
	claims := AdmissionClaims{
		EventID: eventID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

// VerifyAdmissionToken validates a Waiting Room pass against the expected userID and eventID.
func VerifyAdmissionToken(secret, rawToken, expectedUserID, expectedEventID string) (string, error) {
	token, err := jwt.ParseWithClaims(rawToken, &AdmissionClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return "", err
	}
	claims, ok := token.Claims.(*AdmissionClaims)
	if !ok || !token.Valid {
		return "", errors.New("invalid admission token")
	}
	if claims.Subject != expectedUserID || claims.EventID != expectedEventID || claims.ID == "" {
		return "", errors.New("admission token binding mismatch")
	}
	return claims.ID, nil
}

// SetAuthCookie writes an HttpOnly SameSite=Lax JWT cookie.
func SetAuthCookie(w http.ResponseWriter, token string, ttl time.Duration, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     AuthCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearAuthCookie expires the authentication cookie immediately.
func ClearAuthCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     AuthCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// JWTAuth authenticates requests via HttpOnly cookie first, then Authorization Bearer header.
func JWTAuth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := extractRawToken(r)
			if raw == "" {
				WriteError(w, http.StatusUnauthorized, "unauthorized", "missing authentication token")
				return
			}
			user, err := ParseToken(secret, raw)
			if err != nil {
				WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid or expired token")
				return
			}
			ctx := context.WithValue(r.Context(), authUserContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole enforces an authenticated user role (e.g. ADMIN).
func RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := UserFromContext(r.Context())
			if !ok {
				WriteError(w, http.StatusUnauthorized, "unauthorized", "missing user context")
				return
			}
			if !strings.EqualFold(user.Role, role) {
				WriteError(w, http.StatusForbidden, "forbidden", "insufficient permissions")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// UserFromContext retrieves the authenticated user attached by JWTAuth.
func UserFromContext(ctx context.Context) (*AuthUser, bool) {
	u, ok := ctx.Value(authUserContextKey).(*AuthUser)
	return u, ok && u != nil
}

func extractRawToken(r *http.Request) string {
	if c, err := r.Cookie(AuthCookieName); err == nil && strings.TrimSpace(c.Value) != "" {
		return strings.TrimSpace(c.Value)
	}
	authz := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(authz), "bearer ") {
		return strings.TrimSpace(authz[7:])
	}
	return ""
}
