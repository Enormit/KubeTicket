# KubeTicket

Cloud-native **Flash Sale Event Ticketing** system built with **Golang microservices**, **Next.js (TypeScript)**, **Redis**, **PostgreSQL (Neon / PgBouncer)**, **Amazon SQS FIFO**, and **Kubernetes GitOps (ArgoCD + Prometheus/Grafana)**.

## Core Architecture Guarantees

1. **Zero Overselling (Defense-in-Depth):**
   - Fast-path atomic seat locking via **Redis Lua Scripts** (`HELD` 300s → `CHECKOUT` 900s) and `ZSET` expiration sweeper.
   - Final ACID backstop in PostgreSQL via a partial unique index on active tickets (`PENDING`, `PAID`, `CHECKED_IN`).
2. **Traffic Spike Protection:**
   - **Virtual Waiting Room** (`ZADD NX` per user, adaptive admission rate, bound admission token) protects seat selection and DB pools.
   - Asynchronous order ingestion via **Amazon SQS FIFO** (or Redis Streams locally) processed by **KEDA**-autoscaled workers.
3. **Real-Time Seat Map & Resilient UX:**
   - **WebSocket Hub** fans out batched delta updates over **Redis Pub/Sub** with snapshot `seq` recovery and server-synced countdown timers.
4. **Secure E-Tickets & Gate Check-In:**
   - `HMAC-SHA256` signed QR payloads and atomic `ON CONFLICT DO NOTHING` gate check-in to prevent duplicate scans.

## Monorepo Layout

```text
kubeticket/
├── db/             # PostgreSQL migrations and seed data
├── services/       # Go workspace: pkg, gateway-api, flashsale-engine, order-worker
├── frontend/       # Next.js (standalone) + TailwindCSS + Zustand web app
├── deploy/         # Helm charts, ArgoCD manifests, Prometheus/Grafana monitoring
├── infra/          # Terraform IaC (modular blueprint)
└── tests/          # End-to-end concurrency & booking flow tests
```

## Quickstart (Local Development)

1. Copy environment variables:
   ```bash
   cp .env.example .env
   ```
2. Start local infrastructure (`postgres`, `redis`, `prometheus`, `grafana`):
   ```bash
   make up
   ```
3. Run database migrations and seed data:
   ```bash
   make migrate-up
   make seed
   ```
4. Check available automation targets:
   ```bash
   make help
   ```
