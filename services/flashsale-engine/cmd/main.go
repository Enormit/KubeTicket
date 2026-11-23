package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/Enormit/kubeticket/services/flashsale-engine/internal/seatlock"
	"github.com/Enormit/kubeticket/services/flashsale-engine/internal/waitingroom"
	"github.com/Enormit/kubeticket/services/flashsale-engine/internal/ws"
	"github.com/Enormit/kubeticket/services/pkg/config"
	"github.com/Enormit/kubeticket/services/pkg/database"
	"github.com/Enormit/kubeticket/services/pkg/middleware"
	"github.com/Enormit/kubeticket/services/pkg/observability"
	"github.com/go-chi/chi/v5"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	logger := observability.NewLogger("flashsale-engine", cfg.LogLevel)
	tp, tracer := observability.InitTracer("flashsale-engine")
	defer func() { _ = tp.Shutdown(context.Background()) }()

	metrics := observability.NewMetrics("flashsale-engine")
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

	var publisher seatlock.OrderPublisher
	if cfg.SQSOrderQueueURL != "" {
		sqsPub, err := seatlock.NewSQSFIFOPublisher(ctx, cfg.AWSRegion, cfg.SQSOrderQueueURL)
		if err != nil {
			logger.Error("failed to init SQS FIFO publisher", slog.Any("error", err))
			return
		}
		publisher = sqsPub
		logger.Info("order publisher configured", slog.String("backend", "sqs-fifo"))
	} else {
		publisher = seatlock.NewRedisStreamPublisher(rdb)
		logger.Info("order publisher configured", slog.String("backend", "redis-streams"))
	}

	wrSvc := waitingroom.NewService(rdb, waitingroom.NewTurnstileVerifier(cfg.TurnstileSecretKey), cfg.JWTSecret, cfg.QueueAdmissionRate)
	wrHandler := waitingroom.NewHandler(wrSvc)

	seatSvc := seatlock.NewService(rdb, publisher, metrics, cfg.SeatHoldTTL, cfg.CheckoutHoldTTL)
	restored, err := seatSvc.HydrateFromPostgres(ctx, pool)
	if err != nil {
		logger.Error("cold-start redis hydration failed", slog.Any("error", err))
		return
	}
	logger.Info("cold-start redis hydration completed", slog.Int("restored_seats", restored))

	seatHandler := seatlock.NewHandler(seatSvc, wrSvc)
	hub := ws.NewHub(rdb, logger, metrics, cfg.AllowedOrigins)

	go wrSvc.RunAdmissionLoop(ctx, time.Second)
	go seatSvc.RunSweeperLoop(ctx, time.Second)
	go hub.Run(ctx)

	r := chi.NewRouter()
	r.Use(middleware.Recover(logger))
	r.Use(observability.HTTPTracingMiddleware(tracer))
	r.Use(middleware.CORS(cfg.AllowedOrigins))
	r.Use(metrics.HTTPMetricsMiddleware(func(req *http.Request) string {
		if rc := chi.RouteContext(req.Context()); rc != nil {
			if pat := rc.RoutePattern(); pat != "" {
				return pat
			}
		}
		return req.URL.Path
	}))

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		middleware.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if !seatSvc.IsHydrated() {
			middleware.WriteError(w, http.StatusServiceUnavailable, "not_hydrated", "waiting for cold-start hydration")
			return
		}
		if err := database.PingRedis(r.Context(), rdb); err != nil {
			middleware.WriteError(w, http.StatusServiceUnavailable, "redis_unavailable", err.Error())
			return
		}
		middleware.WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	r.Handle(cfg.PrometheusMetricsPath, metrics.Handler())
	r.Get("/ws/events/{id}", hub.ServeWS)

	r.Route("/api/v1", func(api chi.Router) {
		api.Use(middleware.JWTAuth(cfg.JWTSecret))

		api.With(middleware.RateLimit(rdb, "wr-join", 20, time.Minute)).Post("/waiting-room/join", wrHandler.Join)
		api.Get("/waiting-room/status", wrHandler.Status)

		api.With(
			middleware.RateLimit(rdb, "holds", 40, time.Minute),
			middleware.Idempotency(rdb, 10*time.Minute, true),
		).Post("/holds", seatHandler.AcquireHolds)

		api.Delete("/holds/{seat_id}", seatHandler.ReleaseHold)

		api.With(
			middleware.RateLimit(rdb, "checkout", 15, time.Minute),
			middleware.Idempotency(rdb, 30*time.Minute, true),
		).Post("/checkout/submit", seatHandler.SubmitCheckout)
	})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		logger.Info("flashsale-engine listening", slog.String("port", cfg.Port), slog.String("env", cfg.AppEnv))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server error", slog.Any("error", err))
			stop()
		}
	}()

	<-ctx.Done()
	hub.Shutdown()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	logger.Info("flashsale-engine stopped gracefully")
}
