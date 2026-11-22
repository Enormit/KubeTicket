package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Enormit/kubeticket/services/pkg/config"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPostgresPool builds a pgxpool tuned for Neon PgBouncer or direct PostgreSQL.
func NewPostgresPool(ctx context.Context, cfg *config.Config) (*pgxpool.Pool, error) {
	if cfg == nil || cfg.DatabaseURL == "" {
		return nil, errors.New("database URL is required")
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}

	poolCfg.MaxConns = cfg.DBMaxConns
	poolCfg.MinConns = cfg.DBMinConns
	poolCfg.MaxConnLifetime = 30 * time.Minute
	poolCfg.MaxConnIdleTime = 5 * time.Minute
	poolCfg.HealthCheckPeriod = 30 * time.Second

	// Neon's PgBouncer runs in transaction pooling mode; SimpleProtocol avoids prepared-statement errors.
	if cfg.DBUseSimpleProtocol {
		poolCfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pgx pool: %w", err)
	}

	if err := PingPostgres(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}

// PingPostgres verifies PostgreSQL connectivity within a bounded timeout.
func PingPostgres(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return errors.New("postgres pool is nil")
	}
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var one int
	if err := pool.QueryRow(pingCtx, "SELECT 1").Scan(&one); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}
	return nil
}
