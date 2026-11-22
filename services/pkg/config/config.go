package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds shared runtime configuration for all KubeTicket Go services.
type Config struct {
	AppName string
	AppEnv  string
	LogLevel string
	Port    string

	DatabaseURL         string
	DirectDatabaseURL   string
	DBMaxConns          int32
	DBMinConns          int32
	DBUseSimpleProtocol bool

	RedisURL           string
	RedisMode          string
	SeatHoldTTL        time.Duration
	CheckoutHoldTTL    time.Duration
	QueueAdmissionRate int

	JWTSecret          string
	JWTExpiresIn       time.Duration
	CookieSecure       bool
	TicketQRHMACSecret string
	TurnstileSecretKey string

	AWSRegion               string
	SQSOrderQueueURL        string
	PaymentProvider         string
	StripeSecretKey         string
	StripeWebhookSecret     string
	PaymentWebhookTolerance time.Duration

	PrometheusMetricsPath string
	AllowedOrigins        []string
}

// Load reads environment variables (optionally hydrating unset keys from .env files)
// and validates core configuration invariants.
func Load(envPaths ...string) (*Config, error) {
	if len(envPaths) == 0 {
		envPaths = []string{".env", "../.env", "../../.env"}
	}
	for _, p := range envPaths {
		_ = loadEnvIfExists(p)
	}

	appEnv := getEnv("APP_ENV", "development")
	dbURL := firstNonEmpty(
		os.Getenv("DATABASE_URL"),
		os.Getenv("NEON_DATABASE_POOLED_URL"),
		os.Getenv("LOCAL_DATABASE_URL"),
	)
	directDBURL := firstNonEmpty(
		os.Getenv("NEON_DATABASE_DIRECT_URL"),
		dbURL,
	)

	simpleProtoDefault := strings.Contains(dbURL, "pooler") || strings.Contains(dbURL, "neon.tech")

	cfg := &Config{
		AppName:                 getEnv("APP_NAME", "kubeticket"),
		AppEnv:                  appEnv,
		LogLevel:                getEnv("LOG_LEVEL", "info"),
		Port:                    getEnv("PORT", "8080"),
		DatabaseURL:             dbURL,
		DirectDatabaseURL:       directDBURL,
		DBMaxConns:              int32(getEnvInt("DB_MAX_CONNS", 10)),
		DBMinConns:              int32(getEnvInt("DB_MIN_CONNS", 2)),
		DBUseSimpleProtocol:     getEnvBool("DB_SIMPLE_PROTOCOL", simpleProtoDefault),
		RedisURL:                getEnv("REDIS_URL", "redis://localhost:6379/0"),
		RedisMode:               getEnv("REDIS_MODE", "standalone"),
		SeatHoldTTL:             time.Duration(getEnvInt("SEAT_HOLD_TTL_SECONDS", 300)) * time.Second,
		CheckoutHoldTTL:         time.Duration(getEnvInt("CHECKOUT_HOLD_TTL_SECONDS", 900)) * time.Second,
		QueueAdmissionRate:      getEnvInt("QUEUE_ADMISSION_RATE", 50),
		JWTSecret:               getEnv("JWT_SECRET", "dev-insecure-jwt-secret"),
		JWTExpiresIn:            getEnvDuration("JWT_EXPIRES_IN", 24*time.Hour),
		CookieSecure:            getEnvBool("COOKIE_SECURE", appEnv == "production"),
		TicketQRHMACSecret:      getEnv("TICKET_QR_HMAC_SECRET", "dev-insecure-hmac-secret"),
		TurnstileSecretKey:      os.Getenv("TURNSTILE_SECRET_KEY"),
		AWSRegion:               getEnv("AWS_REGION", "ap-southeast-1"),
		SQSOrderQueueURL:        os.Getenv("SQS_ORDER_QUEUE_URL"),
		PaymentProvider:         getEnv("PAYMENT_PROVIDER", "stripe"),
		StripeSecretKey:         os.Getenv("STRIPE_SECRET_KEY"),
		StripeWebhookSecret:     os.Getenv("STRIPE_WEBHOOK_SECRET"),
		PaymentWebhookTolerance: time.Duration(getEnvInt("PAYMENT_WEBHOOK_TOLERANCE_SECONDS", 300)) * time.Second,
		PrometheusMetricsPath:   getEnv("PROMETHEUS_METRICS_PATH", "/metrics"),
		AllowedOrigins:          splitCSV(getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000")),
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate checks required settings and bounds.
func (c *Config) Validate() error {
	if c.Port == "" {
		return errors.New("PORT must not be empty")
	}
	if c.SeatHoldTTL <= 0 {
		return fmt.Errorf("SEAT_HOLD_TTL_SECONDS must be positive, got %s", c.SeatHoldTTL)
	}
	if c.CheckoutHoldTTL < c.SeatHoldTTL {
		return fmt.Errorf("CHECKOUT_HOLD_TTL_SECONDS (%s) must be >= SEAT_HOLD_TTL_SECONDS (%s)", c.CheckoutHoldTTL, c.SeatHoldTTL)
	}
	if c.DBMaxConns <= 0 || c.DBMinConns < 0 || c.DBMinConns > c.DBMaxConns {
		return fmt.Errorf("invalid DB pool bounds: min=%d max=%d", c.DBMinConns, c.DBMaxConns)
	}
	if c.AppEnv == "production" {
		if strings.Contains(c.JWTSecret, "change-me") || strings.Contains(c.JWTSecret, "dev-insecure") {
			return errors.New("JWT_SECRET must be set to a strong secret in production")
		}
		if strings.Contains(c.TicketQRHMACSecret, "change-me") || strings.Contains(c.TicketQRHMACSecret, "dev-insecure") {
			return errors.New("TICKET_QR_HMAC_SECRET must be set to a strong secret in production")
		}
	}
	return nil
}

func loadEnvIfExists(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if k != "" && os.Getenv(k) == "" {
			_ = os.Setenv(k, v)
		}
	}
	return scanner.Err()
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return parsed
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
