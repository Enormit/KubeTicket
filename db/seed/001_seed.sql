-- KubeTicket Seed Data (Idempotent)

BEGIN;

-- 1. Seed Users (bcrypt hashes generated via pgcrypto, compatible with Go bcrypt)
INSERT INTO users (id, email, password_hash, full_name, role)
VALUES
    (
        '00000000-0000-0000-0000-000000000001',
        'admin@kubeticket.local',
        crypt('Admin@123', gen_salt('bf', 10)),
        'KubeTicket Gate Admin',
        'ADMIN'
    ),
    (
        '00000000-0000-0000-0000-000000000002',
        'alice@kubeticket.local',
        crypt('User@123', gen_salt('bf', 10)),
        'Alice Nguyen',
        'USER'
    ),
    (
        '00000000-0000-0000-0000-000000000003',
        'bob@kubeticket.local',
        crypt('User@123', gen_salt('bf', 10)),
        'Bob Tran',
        'USER'
    )
ON CONFLICT (email) DO NOTHING;

-- 2. Seed Events
INSERT INTO events (id, slug, title, description, venue, starts_at, sale_starts_at, sale_ends_at, max_seats_per_order, status)
VALUES
    (
        '11111111-1111-1111-1111-111111111101',
        'kubefest-2026-world-tour',
        'KubeFest 2026: Cloud Native World Tour',
        'Live stadium concert featuring synchronized light shows, multi-tier seating, and instant QR e-tickets.',
        'Saigon Dome Arena',
        NOW() + INTERVAL '30 days',
        NOW() - INTERVAL '1 hour',
        NOW() + INTERVAL '29 days',
        4,
        'ON_SALE'
    ),
    (
        '11111111-1111-1111-1111-111111111102',
        'flashsale-championship-finals',
        'ASEAN Esports Championship Finals',
        'High-demand flash sale championship match with courtside VIP and upper bowl seating.',
        'Phu Tho Indoor Stadium',
        NOW() + INTERVAL '14 days',
        NOW() - INTERVAL '10 minutes',
        NOW() + INTERVAL '13 days',
        4,
        'ON_SALE'
    ),
    (
        '11111111-1111-1111-1111-111111111103',
        'upcoming-synth-night',
        'Midnight Synth Symphony',
        'Upcoming electronic orchestral night opening soon via Virtual Waiting Room.',
        'Ho Chi Minh City Opera Hall',
        NOW() + INTERVAL '45 days',
        NOW() + INTERVAL '2 days',
        NOW() + INTERVAL '44 days',
        4,
        'UPCOMING'
    )
ON CONFLICT (slug) DO NOTHING;

-- 3. Seed Zones (with SVG overview bounds)
INSERT INTO zones (id, event_id, code, name, price_cents, currency, color, svg_bounds)
VALUES
    -- Event 1 Zones (600 seats total)
    (
        '22222222-2222-2222-2222-222222220101',
        '11111111-1111-1111-1111-111111111101',
        'VIP',
        'VIP Front Stage',
        25000,
        'USD',
        '#F59E0B',
        '{"x": 150, "y": 80, "width": 500, "height": 130}'::jsonb
    ),
    (
        '22222222-2222-2222-2222-222222220102',
        '11111111-1111-1111-1111-111111111101',
        'CAT1',
        'Category 1 Mid Bowl',
        15000,
        'USD',
        '#3B82F6',
        '{"x": 100, "y": 230, "width": 600, "height": 180}'::jsonb
    ),
    (
        '22222222-2222-2222-2222-222222220103',
        '11111111-1111-1111-1111-111111111101',
        'GA',
        'General Admission Upper',
        8000,
        'USD',
        '#10B981',
        '{"x": 60, "y": 430, "width": 680, "height": 220}'::jsonb
    ),
    -- Event 2 Zones (120 seats total)
    (
        '22222222-2222-2222-2222-222222220201',
        '11111111-1111-1111-1111-111111111102',
        'COURTSIDE',
        'Courtside Floor',
        18000,
        'USD',
        '#EC4899',
        '{"x": 150, "y": 90, "width": 500, "height": 120}'::jsonb
    ),
    (
        '22222222-2222-2222-2222-222222220202',
        '11111111-1111-1111-1111-111111111102',
        'UPPER',
        'Upper Tribune',
        7500,
        'USD',
        '#6366F1',
        '{"x": 100, "y": 240, "width": 600, "height": 200}'::jsonb
    ),
    -- Event 3 Zones (60 seats total)
    (
        '22222222-2222-2222-2222-222222220301',
        '11111111-1111-1111-1111-111111111103',
        'ORCH',
        'Orchestra Hall',
        12000,
        'USD',
        '#8B5CF6',
        '{"x": 140, "y": 100, "width": 520, "height": 220}'::jsonb
    )
ON CONFLICT (event_id, code) DO NOTHING;

-- 4. Seed Coordinate-Mapped Seats (Data-driven CTE)
WITH seat_specs (event_id, zone_id, prefix, num_rows, seats_per_row, origin_x, origin_y, step_x, step_y) AS (
    VALUES
        ('11111111-1111-1111-1111-111111111101'::uuid, '22222222-2222-2222-2222-222222220101'::uuid, 'VIP-',  5, 20, 40.0, 40.0, 32.0, 34.0),
        ('11111111-1111-1111-1111-111111111101'::uuid, '22222222-2222-2222-2222-222222220102'::uuid, 'CAT1-', 8, 25, 30.0, 40.0, 28.0, 32.0),
        ('11111111-1111-1111-1111-111111111101'::uuid, '22222222-2222-2222-2222-222222220103'::uuid, 'GA-',  10, 30, 24.0, 36.0, 25.0, 30.0),
        ('11111111-1111-1111-1111-111111111102'::uuid, '22222222-2222-2222-2222-222222220201'::uuid, 'CS-',   2, 20, 40.0, 45.0, 32.0, 36.0),
        ('11111111-1111-1111-1111-111111111102'::uuid, '22222222-2222-2222-2222-222222220202'::uuid, 'UP-',   4, 20, 40.0, 45.0, 32.0, 36.0),
        ('11111111-1111-1111-1111-111111111103'::uuid, '22222222-2222-2222-2222-222222220301'::uuid, 'ORCH-', 4, 15, 50.0, 45.0, 36.0, 38.0)
)
INSERT INTO seats (event_id, zone_id, row_label, seat_number, label, pos_x, pos_y)
SELECT
    sp.event_id,
    sp.zone_id,
    CHR(64 + r),
    s,
    sp.prefix || CHR(64 + r) || s::text,
    sp.origin_x + (s - 1) * sp.step_x,
    sp.origin_y + (r - 1) * sp.step_y
FROM seat_specs sp
CROSS JOIN LATERAL generate_series(1, sp.num_rows) AS r
CROSS JOIN LATERAL generate_series(1, sp.seats_per_row) AS s
ON CONFLICT (event_id, label) DO NOTHING;

COMMIT;
