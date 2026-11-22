package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/Enormit/kubeticket/services/gateway-api/internal/handler"
	"github.com/Enormit/kubeticket/services/gateway-api/internal/repo"
	"github.com/Enormit/kubeticket/services/gateway-api/internal/service"
	"github.com/Enormit/kubeticket/services/pkg/config"
	"github.com/Enormit/kubeticket/services/pkg/database"
	"github.com/Enormit/kubeticket/services/pkg/observability"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	logger := observability.NewLogger("gateway-api", cfg.LogLevel)
	tp, tracer := observability.InitTracer("gateway-api")
	defer func() { _ = tp.Shutdown(context.Background()) }()

	metrics := observability.NewMetrics("gateway-api")
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPostgresPool(ctx, cfg)
	if err != nil {
		logger.Error("failed to connect postgres", slog.Any("error", err))
		return
	}
	defer pool.Close()

	rdb, err := database.NewRedisClient(ctx, cfg)
	if err != nil {
		logger.Error("failed to connect redis", slog.Any("error", err))
		return
	}
	defer rdb.Close()

	pgRepo := repo.NewPostgresRepo(pool)
	authSvc := service.NewAuthService(pgRepo, cfg.JWTSecret, cfg.JWTExpiresIn)
	eventSvc := service.NewEventService(pgRepo, rdb)
	ticketSvc := service.NewTicketService(pgRepo, cfg.TicketQRHMACSecret, 24*time.Hour)
	adminSvc := service.NewAdminService(pgRepo, ticketSvc)

	router := handler.NewRouter(handler.RouterDeps{
		Config:        cfg,
		Logger:        logger,
		Tracer:        tracer,
		Metrics:       metrics,
		Pool:          pool,
		Redis:         rdb,
		AuthHandler:   handler.NewAuthHandler(authSvc, cfg.JWTExpiresIn, cfg.CookieSecure),
		EventHandler:  handler.NewEventHandler(eventSvc),
		TicketHandler: handler.NewTicketHandler(ticketSvc),
		AdminHandler:  handler.NewAdminHandler(adminSvc),
	})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		logger.Info("gateway-api listening", slog.String("port", cfg.Port), slog.String("env", cfg.AppEnv))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server error", slog.Any("error", err))
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	logger.Info("gateway-api stopped gracefully")
}
