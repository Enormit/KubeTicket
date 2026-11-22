package handler

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/Enormit/kubeticket/services/pkg/config"
	"github.com/Enormit/kubeticket/services/pkg/database"
	"github.com/Enormit/kubeticket/services/pkg/middleware"
	"github.com/Enormit/kubeticket/services/pkg/observability"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/trace"
)

// RouterDeps aggregates dependencies required to mount gateway-api HTTP routes.
type RouterDeps struct {
	Config       *config.Config
	Logger       *slog.Logger
	Tracer       trace.Tracer
	Metrics      *observability.Metrics
	Pool         *pgxpool.Pool
	Redis        redis.UniversalClient
	AuthHandler  *AuthHandler
	EventHandler *EventHandler
}

// NewRouter constructs the chi HTTP router with observability, CORS, auth, and route handlers.
func NewRouter(d RouterDeps) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.Recover(d.Logger))
	if d.Tracer != nil {
		r.Use(observability.HTTPTracingMiddleware(d.Tracer))
	}
	r.Use(middleware.CORS(d.Config.AllowedOrigins))
	if d.Metrics != nil {
		r.Use(d.Metrics.HTTPMetricsMiddleware(func(req *http.Request) string {
			if rc := chi.RouteContext(req.Context()); rc != nil {
				if pat := rc.RoutePattern(); pat != "" {
					return pat
				}
			}
			return req.URL.Path
		}))
	}

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		middleware.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := database.PingPostgres(r.Context(), d.Pool); err != nil {
			middleware.WriteError(w, http.StatusServiceUnavailable, "db_unavailable", err.Error())
			return
		}
		if err := database.PingRedis(r.Context(), d.Redis); err != nil {
			middleware.WriteError(w, http.StatusServiceUnavailable, "redis_unavailable", err.Error())
			return
		}
		middleware.WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	if d.Metrics != nil {
		r.Handle(d.Config.PrometheusMetricsPath, d.Metrics.Handler())
	}

	r.Route("/api/v1", func(api chi.Router) {
		api.Route("/auth", func(auth chi.Router) {
			auth.With(middleware.RateLimit(d.Redis, "auth", 30, time.Minute)).Post("/register", d.AuthHandler.Register)
			auth.With(middleware.RateLimit(d.Redis, "auth", 30, time.Minute)).Post("/login", d.AuthHandler.Login)
			auth.Post("/logout", d.AuthHandler.Logout)
			auth.With(middleware.JWTAuth(d.Config.JWTSecret)).Get("/me", d.AuthHandler.Me)
		})

		api.Route("/events", func(ev chi.Router) {
			ev.Get("/", d.EventHandler.ListEvents)
			ev.Get("/{id}", d.EventHandler.GetEvent)
			ev.Get("/{id}/seats/snapshot", d.EventHandler.GetSeatSnapshot)
		})
	})

	return r
}
