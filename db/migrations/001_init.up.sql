-- KubeTicket Core ACID Schema (PostgreSQL 16 / Neon Serverless Compatible)

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- 1. Users
CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    full_name     TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'USER' CHECK (role IN ('USER', 'ADMIN')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 2. Events
CREATE TABLE events (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug                TEXT NOT NULL UNIQUE,
    title               TEXT NOT NULL,
    description         TEXT NOT NULL DEFAULT '',
    venue               TEXT NOT NULL,
    starts_at           TIMESTAMPTZ NOT NULL,
    sale_starts_at      TIMESTAMPTZ NOT NULL,
    sale_ends_at        TIMESTAMPTZ,
    max_seats_per_order INT NOT NULL DEFAULT 4 CHECK (max_seats_per_order BETWEEN 1 AND 10),
    status              TEXT NOT NULL DEFAULT 'DRAFT'
                        CHECK (status IN ('DRAFT', 'UPCOMING', 'ON_SALE', 'SOLD_OUT', 'ENDED')),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 3. Zones (2-tier venue sections)
CREATE TABLE zones (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id    UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    code        TEXT NOT NULL,
    name        TEXT NOT NULL,
    price_cents BIGINT NOT NULL CHECK (price_cents >= 0),
    currency    TEXT NOT NULL DEFAULT 'USD',
    color       TEXT NOT NULL DEFAULT '#3B82F6',
    svg_bounds  JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (event_id, code)
);

-- 4. Seats (coordinate-mapped for SVG/Canvas)
CREATE TABLE seats (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id    UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    zone_id     UUID NOT NULL REFERENCES zones(id) ON DELETE CASCADE,
    row_label   TEXT NOT NULL,
    seat_number INT NOT NULL,
    label       TEXT NOT NULL,
    pos_x       DOUBLE PRECISION NOT NULL,
    pos_y       DOUBLE PRECISION NOT NULL,
    status      TEXT NOT NULL DEFAULT 'AVAILABLE'
                CHECK (status IN ('AVAILABLE', 'SOLD', 'BLOCKED')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (event_id, label)
);

CREATE INDEX idx_seats_event_zone ON seats (event_id, zone_id);

-- 5. Orders
CREATE TABLE orders (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           UUID NOT NULL REFERENCES users(id),
    event_id          UUID NOT NULL REFERENCES events(id),
    hold_token        TEXT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'PENDING'
                      CHECK (status IN ('PENDING', 'COMPLETED', 'EXPIRED', 'CANCELLED', 'REFUNDED_EXPIRED')),
    total_cents       BIGINT NOT NULL CHECK (total_cents >= 0),
    currency          TEXT NOT NULL DEFAULT 'USD',
    payment_provider  TEXT NOT NULL DEFAULT 'stripe',
    payment_intent_id TEXT,
    expires_at        TIMESTAMPTZ NOT NULL,
    completed_at      TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_orders_user_created ON orders (user_id, created_at DESC);
CREATE INDEX idx_orders_pending_expires ON orders (expires_at) WHERE status = 'PENDING';

-- 6. Order Items
CREATE TABLE order_items (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id         UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    seat_id          UUID NOT NULL REFERENCES seats(id),
    zone_id          UUID NOT NULL REFERENCES zones(id),
    unit_price_cents BIGINT NOT NULL CHECK (unit_price_cents >= 0),
    UNIQUE (order_id, seat_id)
);

-- 7. Tickets (Final ACID Zero-Overselling Backstop)
CREATE TABLE tickets (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id    UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id),
    event_id    UUID NOT NULL REFERENCES events(id),
    seat_id     UUID NOT NULL REFERENCES seats(id),
    ticket_code TEXT NOT NULL UNIQUE,
    status      TEXT NOT NULL DEFAULT 'PENDING'
                CHECK (status IN ('PENDING', 'PAID', 'CHECKED_IN', 'CANCELLED', 'REFUNDED')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX ux_tickets_active_event_seat
    ON tickets (event_id, seat_id)
    WHERE status IN ('PENDING', 'PAID', 'CHECKED_IN');

CREATE INDEX idx_tickets_user ON tickets (user_id, created_at DESC);

-- 8. Payments
CREATE TABLE payments (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id            UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    provider            TEXT NOT NULL,
    provider_payment_id TEXT NOT NULL,
    amount_cents        BIGINT NOT NULL CHECK (amount_cents >= 0),
    currency            TEXT NOT NULL DEFAULT 'USD',
    status              TEXT NOT NULL CHECK (status IN ('INITIATED', 'SUCCEEDED', 'FAILED', 'REFUNDED')),
    raw_payload         JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (provider, provider_payment_id)
);

-- 9. Check-ins (Atomic Gate Deduplication)
CREATE TABLE checkins (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_id     UUID NOT NULL REFERENCES tickets(id),
    event_id      UUID NOT NULL REFERENCES events(id),
    scanned_by    UUID REFERENCES users(id),
    gate          TEXT NOT NULL DEFAULT 'MAIN',
    checked_in_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (ticket_id)
);

-- 10. Idempotency Keys
CREATE TABLE idempotency_keys (
    key           TEXT NOT NULL,
    user_id       UUID NOT NULL,
    endpoint      TEXT NOT NULL,
    request_hash  TEXT NOT NULL,
    status_code   INT,
    response_body JSONB,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at    TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, endpoint, key)
);

CREATE INDEX idx_idempotency_expires ON idempotency_keys (expires_at);

-- 11. Webhook Event Deduplication
CREATE TABLE webhook_events (
    provider          TEXT NOT NULL,
    provider_event_id TEXT NOT NULL,
    event_type        TEXT NOT NULL,
    processed_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (provider, provider_event_id)
);

-- 12. Transactional Outbox (Reliable Redis SOLD Fan-out)
CREATE TABLE outbox_events (
    id             BIGSERIAL PRIMARY KEY,
    aggregate_type TEXT NOT NULL,
    aggregate_id   UUID NOT NULL,
    event_type     TEXT NOT NULL,
    payload        JSONB NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at   TIMESTAMPTZ
);

CREATE INDEX idx_outbox_unpublished ON outbox_events (id) WHERE published_at IS NULL;
