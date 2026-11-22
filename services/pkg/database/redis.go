package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Enormit/kubeticket/services/pkg/config"
	"github.com/redis/go-redis/v9"
)

// NewRedisClient initializes a pooled go-redis client and verifies connectivity.
func NewRedisClient(ctx context.Context, cfg *config.Config) (*redis.Client, error) {
	if cfg == nil || cfg.RedisURL == "" {
		return nil, errors.New("redis URL is required")
	}

	opts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis URL: %w", err)
	}

	opts.PoolSize = 50
	opts.MinIdleConns = 5
	opts.DialTimeout = 3 * time.Second
	opts.ReadTimeout = 2 * time.Second
	opts.WriteTimeout = 2 * time.Second

	client := redis.NewClient(opts)
	if err := PingRedis(ctx, client); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

// PingRedis checks Redis responsiveness within 2 seconds.
func PingRedis(ctx context.Context, client redis.UniversalClient) error {
	if client == nil {
		return errors.New("redis client is nil")
	}
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if err := client.Ping(pingCtx).Err(); err != nil {
		return fmt.Errorf("ping redis: %w", err)
	}
	return nil
}

// ScriptRegistry caches compiled go-redis Lua scripts by logical name.
type ScriptRegistry struct {
	scripts map[string]*redis.Script
}

// NewScriptRegistry compiles a map of named Lua scripts for EVALSHA execution.
func NewScriptRegistry(sources map[string]string) *ScriptRegistry {
	compiled := make(map[string]*redis.Script, len(sources))
	for name, src := range sources {
		compiled[name] = redis.NewScript(src)
	}
	return &ScriptRegistry{scripts: compiled}
}

// Run executes a registered Lua script by name.
func (r *ScriptRegistry) Run(ctx context.Context, client redis.Scripter, name string, keys []string, args ...any) *redis.Cmd {
	script, ok := r.scripts[name]
	if !ok {
		cmd := redis.NewCmd(ctx)
		cmd.SetErr(fmt.Errorf("lua script %q not registered", name))
		return cmd
	}
	return script.Run(ctx, client, keys, args...)
}
