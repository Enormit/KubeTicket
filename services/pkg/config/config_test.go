package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadDefaultsAndSimpleProtocolDetection(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("NEON_DATABASE_POOLED_URL", "postgresql://u:p@ep-test-pooler.ap-southeast-1.aws.neon.tech/db?sslmode=require")
	t.Setenv("LOCAL_DATABASE_URL", "postgresql://kubeticket:kubeticket@localhost:5432/kubeticket?sslmode=disable")
	t.Setenv("SEAT_HOLD_TTL_SECONDS", "300")
	t.Setenv("CHECKOUT_HOLD_TTL_SECONDS", "900")

	cfg, err := Load("/nonexistent/.env")
	if err != nil {
		t.Fatalf("unexpected Load error: %v", err)
	}

	if !cfg.DBUseSimpleProtocol {
		t.Fatalf("expected DBUseSimpleProtocol=true for pooler URL")
	}
	if cfg.SeatHoldTTL != 300*time.Second || cfg.CheckoutHoldTTL != 900*time.Second {
		t.Fatalf("unexpected TTLs: hold=%s checkout=%s", cfg.SeatHoldTTL, cfg.CheckoutHoldTTL)
	}
}

func TestLoadEnvFileRespectsExistingEnv(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	if err := os.WriteFile(envFile, []byte("PORT=9090\nREDIS_URL=redis://example:6379/1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PORT", "8181")
	t.Setenv("REDIS_URL", "")

	cfg, err := Load(envFile)
	if err != nil {
		t.Fatalf("unexpected Load error: %v", err)
	}
	if cfg.Port != "8181" {
		t.Fatalf("expected existing env PORT=8181 to take precedence, got %s", cfg.Port)
	}
	if cfg.RedisURL != "redis://example:6379/1" {
		t.Fatalf("expected REDIS_URL from file, got %s", cfg.RedisURL)
	}
}

func TestValidateRejectsInvalidProductionSecrets(t *testing.T) {
	cfg := &Config{
		AppEnv:             "production",
		Port:               "8080",
		DBMaxConns:         10,
		DBMinConns:         2,
		SeatHoldTTL:        300 * time.Second,
		CheckoutHoldTTL:    900 * time.Second,
		JWTSecret:          "change-me-to-a-strong-random-secret",
		TicketQRHMACSecret: "real-hmac-secret",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected validation error for placeholder JWT_SECRET in production")
	}
}
