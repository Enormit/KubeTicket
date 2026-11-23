package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/Enormit/kubeticket/services/order-worker/internal/consumer"
	"github.com/Enormit/kubeticket/services/order-worker/internal/order"
	"github.com/Enormit/kubeticket/services/order-worker/internal/outbox"
	"github.com/Enormit/kubeticket/services/order-worker/internal/payment"
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

	logger := observability.NewLogger("order-worker", cfg.LogLevel)
	tp, tracer := observability.InitTracer("order-worker")
	defer func() { _ = tp.Shutdown(context.Background()) }()

	metrics := observability.NewMetrics("order-worker")
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

	var queueConsumer consumer.Consumer
	if cfg.SQSOrderQueueURL != "" {
		sqsCons, err := consumer.NewSQSFIFOConsumer(ctx, cfg.AWSRegion, cfg.SQSOrderQueueURL, logger, metrics)
		if err != nil {
			logger.Error("failed to init SQS FIFO consumer", slog.Any("error", err))
			return
		}
		queueConsumer = sqsCons
		logger.Info("order consumer configured", slog.String("backend", "sqs-fifo"))
	} else {
		queueConsumer = consumer.NewRedisStreamConsumer(rdb, logger, metrics)
		logger.Info("order consumer configured", slog.String("backend", "redis-streams"))
	}

	orderRepo := order.NewPostgresRepository(pool)
	processor := order.NewProcessor(orderRepo, logger, cfg.CheckoutHoldTTL)

	dispatcher := outbox.NewDispatcher(pool, rdb, logger)
	stripeProvider := payment.NewStripeProvider(cfg.StripeSecretKey, cfg.StripeWebhookSecret, cfg.AppEnv, cfg.PaymentWebhookTolerance)
	paySvc := payment.NewService(pool, stripeProvider, logger, dispatcher.Notify)
	payHandler := payment.NewHandler(paySvc, stripeProvider)

	go func() {
		if err := queueConsumer.Run(ctx, processor.HandleOrderMessage); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("queue consumer exited", slog.Any("error", err))
			stop()
		}
	}()
	go dispatcher.Run(ctx, time.Second, 5*time.Second)

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
		if err := database.PingPostgres(r.Context(), pool); err != nil {
			middleware.WriteError(w, http.StatusServiceUnavailable, "db_unavailable", err.Error())
			return
		}
		if err := database.PingRedis(r.Context(), rdb); err != nil {
			middleware.WriteError(w, http.StatusServiceUnavailable, "redis_unavailable", err.Error())
			return
		}
		middleware.WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	r.Handle(cfg.PrometheusMetricsPath, metrics.Handler())

	r.Post("/api/v1/webhooks/payment", payHandler.HandleWebhook)
	if cfg.AppEnv != "production" {
		r.With(middleware.JWTAuth(cfg.JWTSecret)).Post("/api/v1/payments/sandbox/complete", payHandler.HandleSandboxComplete)
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		logger.Info("order-worker listening", slog.String("port", cfg.Port), slog.String("env", cfg.AppEnv))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server error", slog.Any("error", err))
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	logger.Info("order-worker stopped gracefully")
}
