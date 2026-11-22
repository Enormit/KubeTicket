SHELL := /usr/bin/env bash
export PATH := $(HOME)/.local/go/bin:$(PATH)

LOCAL_DB_URL ?= postgresql://kubeticket:kubeticket@127.0.0.1:5432/kubeticket?sslmode=disable
PSQL_EXEC := docker compose exec -T postgres psql "$(LOCAL_DB_URL)" -v ON_ERROR_STOP=1

.PHONY: help up up-all down ps logs migrate-up migrate-down seed test test-race lint

help: ## Show available Make targets
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

up: ## Start local Postgres and Redis containers
	docker compose up -d --wait postgres redis

up-all: ## Start local Postgres, Redis, Prometheus, and Grafana
	docker compose --profile observability up -d --wait

down: ## Stop all local containers
	docker compose --profile observability down

ps: ## Show container status
	docker compose --profile observability ps

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

test: ## Run Go and Frontend unit tests (when initialized)
	@if [ -f services/go.work ]; then cd services && go test ./...; fi
	@if [ -f frontend/package.json ]; then cd frontend && npm test --if-present; fi

test-race: ## Run Go race detector tests across workspace
	@if [ -f services/go.work ]; then cd services && go test -race ./...; fi

lint: ## Run Go vet and Frontend lint/typecheck
	@if [ -f services/go.work ]; then cd services && go vet ./...; fi
	@if [ -f frontend/package.json ]; then cd frontend && npm run lint && npx tsc --noEmit; fi
