-- Enable pgcrypto for gen_random_uuid() just in case it is needed (pre-v13 PG compatibility)
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE regions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL UNIQUE,      -- e.g. "india", "us-east"
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE clusters (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL UNIQUE,
    region_id   UUID NOT NULL REFERENCES regions(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE nodes (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cluster_id      UUID NOT NULL REFERENCES clusters(id),
    hostname        TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'unknown',  -- enum: healthy | unhealthy | draining | dead | unknown
    cpu_pct         REAL,
    memory_pct      REAL,
    disk_pct        REAL,
    last_heartbeat  TIMESTAMPTZ,
    registered_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (cluster_id, hostname)
);

CREATE TABLE heartbeats (
    id          BIGSERIAL PRIMARY KEY,
    node_id     UUID NOT NULL REFERENCES nodes(id),
    cpu_pct     REAL NOT NULL,
    memory_pct  REAL NOT NULL,
    disk_pct    REAL NOT NULL,
    healthy     BOOLEAN NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE databases (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    node_id     UUID REFERENCES nodes(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE replicas (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    database_id  UUID NOT NULL REFERENCES databases(id),
    node_id      UUID NOT NULL REFERENCES nodes(id),
    role         TEXT NOT NULL DEFAULT 'follower'  -- leader | follower
);
