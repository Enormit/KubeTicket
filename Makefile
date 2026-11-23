SHELL := /usr/bin/env bash
export PATH := $(HOME)/.local/go/bin:$(PATH)

LOCAL_DB_URL ?= postgresql://kubeticket:kubeticket@127.0.0.1:5432/kubeticket?sslmode=disable
PSQL_EXEC := docker compose exec -T postgres psql "$(LOCAL_DB_URL)" -v ON_ERROR_STOP=1

.PHONY: help up up-infra up-obs up-full down ps logs migrate-up migrate-down seed test test-race lint

help: ## Show available Make targets
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

up-infra: ## Start local Postgres and Redis containers only
	docker compose up -d --wait postgres redis

up: ## Build and start backend stack (DB, Redis, 3 Go services, Nginx edge-gateway)
	docker compose up -d --build --wait

up-obs: ## Start backend stack + Prometheus and Grafana
	docker compose --profile observability up -d --build --wait

up-full: ## Start full stack including Frontend, Prometheus, and Grafana
	docker compose --profile full up -d --build --wait

down: ## Stop all local containers across all profiles
	docker compose --profile full --profile observability down

ps: ## Show container status
	docker compose --profile full --profile observability ps

logs: ## Tail container logs
	docker compose logs -f

migrate-up: ## Apply up SQL migrations to local Postgres
	@for f in $$(ls -1 db/migrations/*.up.sql 2>/dev/null | sort); do \
		echo "Applying $$f..."; \
		$(PSQL_EXEC) < "$$f"; \
	done

migrate-down: ## Apply down SQL migrations in reverse order
	@for f in $$(ls -1 db/migrations/*.down.sql 2>/dev/null | sort -r); do \
		echo "Reverting $$f..."; \
		$(PSQL_EXEC) < "$$f"; \
	done

seed: ## Load SQL seed files into local Postgres
	@for f in $$(ls -1 db/seed/*.sql 2>/dev/null | sort); do \
		echo "Seeding $$f..."; \
		$(PSQL_EXEC) < "$$f"; \
	done

test: ## Run Go and Frontend unit tests
	@if [ -f services/go.work ]; then cd services && go test ./pkg/... ./gateway-api/... ./flashsale-engine/... ./order-worker/...; fi
	@if [ -f frontend/package.json ]; then cd frontend && npm test --if-present; fi

test-race: ## Run Go race detector tests across all Go workspace modules
	@if [ -f services/go.work ]; then cd services && RUN_INTEGRATION=1 go test -race ./pkg/... ./gateway-api/... ./flashsale-engine/... ./order-worker/...; fi

lint: ## Run Go vet and Frontend lint/typecheck
	@if [ -f services/go.work ]; then cd services && go vet ./pkg/... ./gateway-api/... ./flashsale-engine/... ./order-worker/...; fi
	@if [ -f frontend/package.json ]; then cd frontend && npm run lint && npx tsc --noEmit; fi
