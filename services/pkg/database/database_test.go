package database

import (
	"context"
	"os"
	"testing"

	"github.com/Enormit/kubeticket/services/pkg/config"
)

func TestNewPostgresPoolRejectsMissingURL(t *testing.T) {
	if _, err := NewPostgresPool(context.Background(), &config.Config{}); err == nil {
		t.Fatalf("expected error for empty DatabaseURL")
	}
}

func TestNewRedisClientRejectsMissingURL(t *testing.T) {
	if _, err := NewRedisClient(context.Background(), &config.Config{}); err == nil {
		t.Fatalf("expected error for empty RedisURL")
	}
}

func TestScriptRegistryMissingScript(t *testing.T) {
	reg := NewScriptRegistry(map[string]string{"ok": "return 1"})
	if err := reg.Run(context.Background(), nil, "missing", nil).Err(); err == nil {
		t.Fatalf("expected error for unregistered script")
	}
}

func TestLocalDockerConnectivity(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION") == "" {
		t.Skip("set RUN_INTEGRATION=1 to test against local docker postgres/redis")
	}
	cfg := &config.Config{
		DatabaseURL:         "postgresql://kubeticket:kubeticket@127.0.0.1:5432/kubeticket?sslmode=disable",
		DBMaxConns:          5,
		DBMinConns:          1,
		DBUseSimpleProtocol: true,
		RedisURL:            "redis://127.0.0.1:6379/0",
	}

	ctx := context.Background()
	pool, err := NewPostgresPool(ctx, cfg)
	if err != nil {
		t.Fatalf("postgres connect failed: %v", err)
	}
	defer pool.Close()

	rdb, err := NewRedisClient(ctx, cfg)
	if err != nil {
		t.Fatalf("redis connect failed: %v", err)
	}
	defer rdb.Close()
}
