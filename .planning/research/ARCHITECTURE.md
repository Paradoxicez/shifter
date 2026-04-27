# Architecture Research

**Domain:** Self-hosted LoRaWAN utility (water/electricity) monitoring platform wrapping ChirpStack v4
**Researched:** 2026-04-27
**Confidence:** HIGH (architecture patterns and ChirpStack integration surface), MEDIUM (specific schema choices — multiple valid patterns exist)

## Standard Architecture

The dominant pattern in 2026 for ChirpStack-wrapping utility platforms is a **two-path architecture** with a clear seam between the ingestion path (asynchronous, MQTT-driven, write-heavy) and the API path (synchronous, gRPC + HTTP, read-heavy). They share one Postgres/TimescaleDB instance but otherwise stay decoupled.

### System Overview

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                          PHYSICAL / FIELD LAYER                               │
│   ┌──────────┐    LoRa    ┌──────────┐   UDP/MQTT   ┌────────────────────┐   │
│   │  Meter   │ ─────────► │ Gateway  │ ───────────► │ ChirpStack Gateway │   │
│   │ (vendor) │            │ (Semtech)│              │      Bridge        │   │
│   └──────────┘            └──────────┘              └─────────┬──────────┘   │
└─────────────────────────────────────────────────────────────────┬─────────────┘
                                                                  │ MQTT
┌─────────────────────────────────────────────────────────────────▼─────────────┐
│                      CHIRPSTACK LAYER (bundled or external)                   │
│   ┌────────────────┐  ┌──────────────────┐  ┌──────────────────────────┐      │
│   │  ChirpStack    │  │  PostgreSQL      │  │      Redis (sessions,     │     │
│   │  (NS + AS + UI)│◄─┤  (CS metadata)   │  │       device queue)       │     │
│   └───────┬────────┘  └──────────────────┘  └──────────────────────────┘      │
│           │ publishes JSON events on:                                         │
│           │  application/{APP_ID}/device/{DEV_EUI}/event/{up|join|status|…}   │
│           │ AND exposes gRPC API for management (devices, gateways, profiles) │
└───────────┼──────────────────────────────────────────────────────────────────┘
            │ MQTT (events)                              │ gRPC (mgmt)
┌───────────▼──────────────────────────────────────────────▼────────────────────┐
│                              SHIFTER BACKEND                                  │
│  ┌────────────────────────┐         ┌──────────────────────────────────────┐  │
│  │   INGESTION PATH       │         │             API PATH                  │ │
│  │  (MQTT subscriber)     │         │   (gRPC client + HTTP/WS server)      │ │
│  │                        │         │                                       │ │
│  │  ┌──────────────────┐  │         │  ┌────────────────────────────────┐  │ │
│  │  │ MQTT consumer    │  │         │  │ HTTP API (CRUD: sites, points, │  │ │
│  │  │ (per app topic)  │  │         │  │ devices, users, alerts, plans) │  │ │
│  │  └────────┬─────────┘  │         │  └──────────────┬─────────────────┘  │ │
│  │           │            │         │                 │                     │ │
│  │  ┌────────▼─────────┐  │         │  ┌──────────────▼─────────────────┐  │ │
│  │  │ Codec/normalizer │  │         │  │ ChirpStack gRPC adapter        │  │ │
│  │  │ (canonical model)│  │         │  │ (DeviceService, GatewayService, │  │ │
│  │  └────────┬─────────┘  │         │  │ DeviceProfileService, …)       │  │ │
│  │           │            │         │  └────────────────────────────────┘  │ │
│  │  ┌────────▼─────────┐  │         │                                       │ │
│  │  │ Metering-point   │  │         │  ┌────────────────────────────────┐  │ │
│  │  │ resolver (DEV_EUI│  │         │  │ Read API (latest, history,     │  │ │
│  │  │ → active point)  │  │         │  │  daily/monthly/yearly aggs)    │  │ │
│  │  └────────┬─────────┘  │         │  └──────────────┬─────────────────┘  │ │
│  │           │            │         │                 │                     │ │
│  │  ┌────────▼─────────┐  │         │  ┌──────────────▼─────────────────┐  │ │
│  │  │ Hypertable INSERT│  │         │  │ WS/SSE hub (subscribes to      │  │ │
│  │  │ + LISTEN/NOTIFY  │──┼────────►│  │ pg_notify, fans out to clients)│  │ │
│  │  └──────────────────┘  │         │  └────────────────────────────────┘  │ │
│  │                        │         │                                       │ │
│  │  ┌──────────────────┐  │         │  ┌────────────────────────────────┐  │ │
│  │  │ Background jobs  │  │         │  │ Static / blob serving          │  │ │
│  │  │ (alerts, offline │  │         │  │ (floor-plan images)            │  │ │
│  │  │ watchdog, rollups│  │         │  └────────────────────────────────┘  │ │
│  │  └──────────────────┘  │         │                                       │ │
│  └────────────────────────┘         └──────────────────────────────────────┘  │
└────────────────────────────────────┬──────────────────────────────────────────┘
                                     │
┌────────────────────────────────────▼──────────────────────────────────────────┐
│                       STORAGE LAYER (single Postgres)                          │
│  ┌────────────────────────────────────────────────────────────────────────┐   │
│  │  PostgreSQL + TimescaleDB                                              │   │
│  │  ┌──────────────┐  ┌──────────────────┐  ┌──────────────────────────┐  │   │
│  │  │ Relational   │  │  Hypertable:     │  │ Continuous aggregates:    │  │   │
│  │  │ tables:      │  │  measurements    │  │  hourly / daily / monthly │  │   │
│  │  │ users, sites,│  │  (time, mp_id,   │  │  / yearly per metering pt │  │   │
│  │  │ floor_plans, │  │   reading,       │  └──────────────────────────┘  │   │
│  │  │ metering_pts,│  │   flow_rate,     │  ┌──────────────────────────┐  │   │
│  │  │ devices,     │  │   raw JSONB,…)   │  │ Files (floor-plan images)│  │   │
│  │  │ alerts,…     │  └──────────────────┘  │ on filesystem volume     │  │   │
│  │  └──────────────┘                        └──────────────────────────┘  │   │
│  └────────────────────────────────────────────────────────────────────────┘   │
└────────────────────────────────────────────────────────────────────────────────┘
                                     ▲
                                     │ HTTPS (single port via reverse proxy)
                                     │
┌────────────────────────────────────┴──────────────────────────────────────────┐
│   FRONTEND (shadcn/ui SPA — Next.js or Vite+React)                            │
│   - REST/HTTP for CRUD + history queries                                      │
│   - WebSocket (or SSE) subscription for live updates                          │
└────────────────────────────────────────────────────────────────────────────────┘
```

### Component Responsibilities

| Component | Responsibility | Typical Implementation |
|-----------|----------------|------------------------|
| **ChirpStack** | LoRaWAN NS + AS: gateway management, device session/MIC, ADR, join procedure, codec execution, event publishing | Bundled v4 image (`chirpstack/chirpstack:4`) — never modified |
| **Mosquitto** | MQTT broker carrying CS↔Gateway and CS↔App events | Bundled (`eclipse-mosquitto:2`) |
| **Ingestion service** (Shifter) | Subscribe to `application/+/device/+/event/up`, normalize, persist | One process inside the Shifter backend; long-running MQTT loop |
| **Codec/normalizer** | Map ChirpStack `object` (decoded by CS codec) into canonical measurement schema | Pure functions per device-profile family |
| **Metering-point resolver** | Translate `DEV_EUI` (at event time) → active `metering_point_id`; apply offset; reject orphan events | DB lookup against `metering_point_assignment` table |
| **API service** (Shifter) | HTTP/JSON for CRUD, history queries, auth; WS/SSE for push | Same backend binary, different module |
| **ChirpStack gRPC adapter** | Wrap CS gRPC API for device/gateway/profile CRUD, hide multi-step flows | One client per CS API service; auto-reconnect |
| **Realtime hub** | Push fresh measurements to connected browsers | `LISTEN pg_notify` channel, fan out via WS/SSE |
| **Background workers** | Alert evaluation, offline watchdog, daily/monthly/yearly rollups (if not pure CAGGs), report generation | Cron-like scheduled goroutines/tokio tasks; or `pg_cron` |
| **TimescaleDB** | Telemetry hypertable + continuous aggregates + relational metadata, all in one DB | Postgres 16 + Timescale 2.x extension |
| **Reverse proxy** | TLS termination, single 80/443 ingress, route `/api`, `/ws`, `/files`, `/` (SPA), and `/chirpstack` (admin override only) | Caddy (auto-HTTPS, simplest) |

## Recommended Project Structure

This is layout-agnostic to backend language (Go/Rust/Node — to be decided in STACK.md). The structure expresses the boundaries.

```
shifter/
├── docker-compose.yml              # bundled mode (CS + Mosquitto + Postgres + Shifter)
├── docker-compose.external.yml     # external mode (Postgres + Shifter only)
├── Caddyfile                       # reverse proxy config
├── .env.example                    # CS_GRPC_URL, CS_API_KEY, MQTT_URL, …
│
├── backend/
│   ├── cmd/
│   │   └── shifter/                # single binary entrypoint (API + ingestion + jobs)
│   ├── internal/
│   │   ├── ingestion/              # MQTT subscriber, normalization, insert
│   │   │   ├── mqtt.go             # connect, subscribe, dispatch
│   │   │   ├── normalize.go        # canonical model mapping
│   │   │   └── resolver.go         # DEV_EUI → metering point
│   │   ├── api/                    # HTTP handlers
│   │   │   ├── sites.go
│   │   │   ├── metering_points.go
│   │   │   ├── devices.go          # delegates to chirpstack/
│   │   │   ├── measurements.go     # latest + history reads
│   │   │   └── auth.go
│   │   ├── chirpstack/             # gRPC adapter — *only* layer that imports CS protos
│   │   │   ├── client.go
│   │   │   ├── devices.go
│   │   │   ├── gateways.go
│   │   │   └── profiles.go
│   │   ├── realtime/               # WS/SSE hub + pg_notify listener
│   │   ├── jobs/                   # alert eval, offline watchdog, rollup refresh
│   │   ├── domain/                 # core types: MeteringPoint, Measurement, Site, …
│   │   ├── store/                  # SQL queries, migrations
│   │   └── files/                  # floor-plan upload/serve abstraction
│   └── migrations/                 # SQL incl. hypertable + CAGG definitions
│
├── frontend/
│   ├── app/                        # Next.js app router OR Vite src/
│   ├── components/                 # shadcn/ui components
│   ├── lib/
│   │   ├── api.ts                  # typed REST client
│   │   └── ws.ts                   # WS subscription helper
│   └── pages or routes/
│
└── .planning/                      # GSD artifacts (this file lives here)
```

### Structure Rationale

- **Single binary, multiple modules:** The ingestion path, API path, and jobs all run in one process. Self-hosted single-tenant installs do not need separate microservices — extra processes mean extra failure modes and more Docker complexity. Threads/goroutines/tokio tasks separate concerns within one binary.
- **`chirpstack/` is the only ChirpStack-aware module:** Every other module talks to `domain/` types. Replacing ChirpStack later (or supporting an LNS variant) is an isolated change.
- **`ingestion/` and `api/` never call each other directly:** They share `domain/` and `store/`. The seam is the database. This keeps a slow API request from blocking ingestion, and a burst of uplinks from blocking the UI.
- **Migrations include CAGG and hypertable creation:** Time-series schema is application code, not ops magic.

## Architectural Patterns

### Pattern 1: Two-Path Architecture (Ingestion vs API)

**What:** Split the backend into a write-heavy MQTT-driven ingestion path and a read-heavy HTTP/WS API path. They share storage but not control flow.

**When to use:** Always for IoT platforms — uplink rate is independent of user activity, and a slow API request must never block a sensor write.

**Trade-offs:** Slightly more module-level code, but dramatically clearer reasoning. The pattern is essentially CQRS-lite: the write side normalizes and persists; the read side queries and pushes.

**Sketch:**
```go
// ingestion path: long-running, never blocks on UI
mqtt.Subscribe("application/+/device/+/event/up", func(msg Message) {
    evt := decodeUplink(msg)
    canonical := normalize(evt)                 // vendor → canonical
    mp := resolver.Resolve(evt.DevEUI, evt.Time) // device → metering point
    if mp == nil { metrics.Orphan.Inc(); return }
    store.InsertMeasurement(mp.ID, canonical)   // hypertable insert
    // pg trigger emits NOTIFY 'measurements' with payload
})

// api path: serves user requests; subscribes to NOTIFY for push
listener := db.Listen("measurements")
for n := range listener.Channel() {
    realtimeHub.Broadcast(n.Payload)
}
```

### Pattern 2: Metering-Point Abstraction (the lifecycle anchor)

**What:** A `metering_point` is an addressable location/role in the customer's site (e.g., "Building A, 3rd floor cold-water main"). Devices are *attached* to metering points for windows of time. The history series lives on the metering point, not the device.

**When to use:** Any utility-monitoring platform where physical meters get replaced, swapped between locations, or upgraded. This is non-negotiable for water/electricity.

**Trade-offs:** Adds one indirection layer at write time and read time. The benefit is enormous: a meter swap is a row insert, not a data migration.

**Schema sketch:**
```sql
CREATE TABLE metering_point (
    id              UUID PRIMARY KEY,
    site_id         UUID NOT NULL REFERENCES site(id),
    name            TEXT NOT NULL,             -- "Building A 3F cold water"
    utility         TEXT NOT NULL,             -- 'water' | 'electricity'
    floor_x         INT,                       -- pixel coords on floor plan
    floor_y         INT,
    floor_plan_id   UUID REFERENCES floor_plan(id)
);

CREATE TABLE device (
    dev_eui         BYTEA PRIMARY KEY,         -- LoRaWAN identity
    vendor          TEXT NOT NULL,
    model           TEXT NOT NULL,
    cs_device_id    TEXT,                      -- ChirpStack reference
    cs_app_id       TEXT
);

-- The crucial table: time-windowed assignment
CREATE TABLE metering_point_assignment (
    id                  UUID PRIMARY KEY,
    metering_point_id   UUID NOT NULL REFERENCES metering_point(id),
    dev_eui             BYTEA NOT NULL REFERENCES device(dev_eui),
    valid_from          TIMESTAMPTZ NOT NULL,
    valid_to            TIMESTAMPTZ,            -- NULL = currently active
    reading_offset      NUMERIC NOT NULL DEFAULT 0,  -- carried from previous meter
    UNIQUE (metering_point_id, valid_from)
);

CREATE INDEX ON metering_point_assignment (dev_eui, valid_from DESC);
```

**Resolution at ingest:**
```sql
SELECT metering_point_id, reading_offset
FROM metering_point_assignment
WHERE dev_eui = $1
  AND valid_from <= $2
  AND (valid_to IS NULL OR valid_to > $2)
LIMIT 1;
```

**Meter swap procedure (single API call):**
1. Read last reading R from outgoing device.
2. Close current assignment with `valid_to = now()`.
3. Insert new assignment with `valid_from = now()`, `reading_offset = previous_offset + R - new_meter_initial`.
4. UI continues to show a continuous cumulative line.

### Pattern 3: Canonical Measurement Model (vendor-agnostic schema)

**What:** ChirpStack's JS codec turns vendor bytes into a JSON `object`. Shifter's normalizer maps that vendor-shaped object onto a fixed canonical record with optional fields plus a JSONB `extra` for everything the UI doesn't natively render.

**When to use:** Any time the device fleet contains more than one vendor or model. (For Shifter, this is day one.)

**Trade-offs:** A canonical schema requires upfront thought about which fields are first-class. The payoff is that dashboards, reports, and alerts query a single shape regardless of vendor — and "advanced view" can iterate JSONB to display anything extra without code changes.

**Schema sketch:**
```sql
CREATE TABLE measurement (
    time            TIMESTAMPTZ NOT NULL,
    metering_point_id UUID NOT NULL REFERENCES metering_point(id),
    device_id       BYTEA NOT NULL,                -- DEV_EUI for traceability

    -- Canonical first-class fields (NULLable; populated when present)
    cumulative      NUMERIC,                       -- e.g. m³ water, kWh electricity
    flow_rate       NUMERIC,                       -- e.g. m³/h, current power
    voltage         NUMERIC,
    current_amps    NUMERIC,
    battery_pct     SMALLINT,
    rssi            SMALLINT,
    snr             REAL,

    -- Everything else the device emitted (vendor-shaped)
    extra           JSONB NOT NULL DEFAULT '{}',

    -- Provenance
    raw             JSONB,                         -- the original CS event (debug / replay)

    PRIMARY KEY (metering_point_id, time)
);
SELECT create_hypertable('measurement', 'time', chunk_time_interval => INTERVAL '7 days');
```

**Normalizer pseudo-code:**
```go
func Normalize(evt ChirpStackUplink, profile DeviceProfile) Measurement {
    obj := evt.Object
    m := Measurement{Time: evt.Time, DeviceID: evt.DevEUI}

    // Family-specific mappings (vendor + model)
    switch profile.Family {
    case "elsys-water":
        m.Cumulative = obj.Get("water_cumulative_m3")
        m.FlowRate   = obj.Get("flow_lpm").Mul(0.06) // L/min → m³/h
        m.Battery    = obj.Get("battery_pct")
    case "milesight-em500":
        m.Cumulative = obj.Get("counter")
        // ...
    }

    // Always preserve everything else for the advanced view
    m.Extra = obj.WithoutKeys("water_cumulative_m3", "flow_lpm", "battery_pct")
    m.Raw   = evt.OriginalJSON
    return m
}
```

The mapping table (vendor+model → canonical fields) lives in code, not config — it ships with each release as Shifter learns new device families. Customers never edit it.

### Pattern 4: Continuous Aggregates over Application Rollups

**What:** Use TimescaleDB continuous aggregates (CAGGs) for hourly/daily/monthly/yearly rollups, refreshed incrementally by Timescale itself. Application code only computes things CAGGs cannot (alert evaluation, derived series).

**When to use:** Default choice for time-bucketed reports in TimescaleDB. Only fall back to application-side rollups if the aggregation requires logic SQL cannot express cleanly.

**Trade-offs:** CAGGs are tied to one base table — schema changes propagate carefully. They are dramatically cheaper than recomputing on every dashboard load.

**Sketch:**
```sql
CREATE MATERIALIZED VIEW measurement_daily
WITH (timescaledb.continuous) AS
SELECT
    metering_point_id,
    time_bucket('1 day', time) AS day,
    first(cumulative, time) AS day_start_cumulative,
    last(cumulative, time)  AS day_end_cumulative,
    last(cumulative, time) - first(cumulative, time) AS day_consumption,
    avg(flow_rate) AS avg_flow_rate,
    max(flow_rate) AS peak_flow_rate
FROM measurement
GROUP BY metering_point_id, day;

SELECT add_continuous_aggregate_policy('measurement_daily',
    start_offset => INTERVAL '30 days',
    end_offset   => INTERVAL '1 hour',
    schedule_interval => INTERVAL '1 hour');
```

`measurement_monthly` and `measurement_yearly` can be CAGGs over `measurement_daily` (hierarchical CAGGs, supported since Timescale 2.x).

### Pattern 5: Realtime Push via Postgres LISTEN/NOTIFY

**What:** A Postgres trigger on `measurement` INSERT publishes a small JSON payload via `pg_notify('measurements', …)`. The backend's realtime hub `LISTEN`s and fans out to subscribed WebSocket/SSE clients filtered by `metering_point_id`.

**When to use:** Single-tenant, single-installation deployments with at most a few hundred concurrent dashboard sessions. Avoids adding Redis pub/sub or a message broker to the deployment surface.

**Trade-offs:** Notifications are best-effort and bounded in size (~8KB). At thousands of concurrent listeners or extremely high event rates, switch to Redis pub/sub. For Shifter's single-tenant scope, LISTEN/NOTIFY is the right call.

**Sketch:**
```sql
CREATE OR REPLACE FUNCTION notify_measurement() RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify(
        'measurements',
        json_build_object(
            'mp', NEW.metering_point_id,
            't',  NEW.time,
            'cum', NEW.cumulative,
            'fr', NEW.flow_rate
        )::text
    );
    RETURN NEW;
END $$ LANGUAGE plpgsql;

CREATE TRIGGER measurement_notify AFTER INSERT ON measurement
    FOR EACH ROW EXECUTE FUNCTION notify_measurement();
```

The realtime hub does **not** subscribe to MQTT directly. Decoupling realtime from ingestion via the database guarantees the dashboard only ever shows data that has been successfully persisted — no "appears live, then disappears on reload" bugs.

### Pattern 6: ChirpStack as a Configurable External or Bundled Backend

**What:** Treat ChirpStack as a remote dependency reached via two URLs: a gRPC URL (for management) and an MQTT URL (for events). Bundled mode is just `docker-compose` providing those URLs locally; external mode reads them from the env. The Shifter backend code is identical in both modes.

**When to use:** Always. This is the entire deployment-flexibility story.

**Trade-offs:** Forces clean configuration discipline (no hardcoded localhost). Returns enormous deployment flexibility.

## Data Flow

### Ingestion Flow (uplink, write path)

```
Meter sends LoRa frame
    ↓ (LoRa air interface, AES)
Gateway receives, packetizes
    ↓ (UDP Semtech protocol)
ChirpStack Gateway Bridge converts UDP → MQTT
    ↓ (MQTT to internal CS topics)
ChirpStack Network Server: dedup, MIC check, ADR, decrypt
    ↓
ChirpStack Application Server: run JS codec → produces `object`
    ↓
ChirpStack publishes JSON event:
    application/{APP_ID}/device/{DEV_EUI}/event/up
    ↓ (MQTT)
Shifter ingestion service:
    1. Decode JSON event
    2. Look up device profile family (cached)
    3. Normalize `object` → canonical Measurement
    4. Resolve DEV_EUI → active metering_point_id (cached, TTL 60s)
    5. Apply reading_offset to cumulative
    6. INSERT INTO measurement (...)
        ↓
        Postgres trigger fires pg_notify('measurements', payload)
            ↓
            Realtime hub receives notification
                ↓ (WebSocket frame)
                Browser dashboard updates the chart in place
    7. Background alert evaluator (separate goroutine, debounced) checks
       new measurement against active threshold/anomaly rules
```

### API Flow (user action, read/write CRUD)

```
User in browser clicks "Add device" in dialog
    ↓ (HTTPS, JSON)
Caddy reverse proxy → Shifter API
    ↓
HTTP handler: validate, check role
    ↓
Domain operation (e.g., "register meter for metering point X"):
    1. Insert metering_point_assignment row (DB transaction begins)
    2. Call ChirpStack gRPC adapter:
       - DeviceService.Create(devEUI, profileID, appID)
       - DeviceService.CreateKeys(appKey)
    3. Commit DB transaction
    4. Return 201 with full device record
    ↓
Browser closes dialog, refetches device list
```

For read paths (latest, history, daily/monthly/yearly):
```
GET /api/metering-points/{id}/series?from=...&to=...&bucket=daily
    ↓
Handler picks the right relation:
    bucket=raw     → SELECT FROM measurement
    bucket=hourly  → SELECT FROM measurement_hourly  (CAGG)
    bucket=daily   → SELECT FROM measurement_daily   (CAGG)
    bucket=monthly → SELECT FROM measurement_monthly (CAGG)
    bucket=yearly  → SELECT FROM measurement_yearly  (CAGG)
    ↓
Stream JSON rows back; frontend feeds into shadcn Recharts
```

### State / Realtime Flow

```
Browser establishes WebSocket: /ws?token=...
    ↓
WS hub authenticates, registers session with subscription filter
    (set of metering_point_ids visible on the current screen)
    ↓
For each pg_notify('measurements', payload):
    Hub iterates active sessions; pushes payload only to sessions
    whose filter contains the mp_id
```

### Key Data Flows Summary

1. **Uplink ingestion:** ChirpStack MQTT → Shifter normalize → metering point resolve → hypertable insert. Asynchronous, never touches HTTP.
2. **Realtime push:** DB trigger → pg_notify → WS hub → browser. Decoupled from MQTT; dashboard only sees persisted data.
3. **CRUD:** HTTP handler → domain operation → ChirpStack gRPC + Postgres in one logical transaction.
4. **History reads:** HTTP handler → CAGG query → JSON. Bucket size selects the right CAGG; raw table only for short windows.
5. **Background:** Scheduled jobs query the DB for offline devices, alert conditions, and report generation.

## Deployment Topology

### Bundled Mode (turnkey customer with no existing LNS)

```yaml
# docker-compose.yml
services:
  caddy:                  # 80, 443 → routes /api, /ws, /, /chirpstack-admin
    image: caddy:2
    ports: ["80:80", "443:443"]
    volumes: [./Caddyfile:/etc/caddy/Caddyfile, caddy_data:/data]

  shifter:                # the Shifter backend (single binary)
    build: ./backend
    environment:
      DATABASE_URL: postgres://shifter:***@db:5432/shifter
      CS_GRPC_URL:  chirpstack:8080
      CS_API_KEY:   ${CS_API_KEY}
      MQTT_URL:     tcp://mosquitto:1883
      FILES_DIR:    /var/lib/shifter/files
    volumes: [shifter_files:/var/lib/shifter/files]

  shifter-frontend:       # if Next.js, otherwise served as static by caddy
    build: ./frontend

  db:                     # ONE Postgres instance — shared by CS and Shifter
    image: timescale/timescaledb:latest-pg16
    environment: { POSTGRES_PASSWORD: *** }
    volumes: [db_data:/var/lib/postgresql/data]

  redis:                  # required by ChirpStack
    image: redis:7

  mosquitto:              # MQTT broker (CS↔Gateway and CS↔Shifter)
    image: eclipse-mosquitto:2
    volumes: [./mosquitto.conf:/mosquitto/config/mosquitto.conf]

  chirpstack-gateway-bridge:
    image: chirpstack/chirpstack-gateway-bridge:4
    ports: ["1700:1700/udp"]   # Semtech UDP from gateways

  chirpstack:             # the LoRaWAN NS + AS + admin UI
    image: chirpstack/chirpstack:4
    volumes: [./chirpstack-config:/etc/chirpstack]
    depends_on: [db, redis, mosquitto]

volumes: { db_data:, shifter_files:, caddy_data: }
```

Notes:
- **One Postgres instance** is shared between ChirpStack and Shifter (separate databases inside it: `chirpstack`, `shifter`). This halves the operational surface.
- **Mosquitto** is the shared MQTT broker.
- **`shifter_files`** is a Docker named volume holding floor-plan images. Filesystem is the right answer here — see Pattern 7 below.
- **ChirpStack admin UI** is exposed only as an admin escape hatch behind auth, not the daily UX.

### External Mode (customer already runs ChirpStack)

```yaml
# docker-compose.external.yml
services:
  caddy:
    image: caddy:2
    ports: ["80:80", "443:443"]

  shifter:
    build: ./backend
    environment:
      DATABASE_URL: postgres://shifter:***@db:5432/shifter
      CS_GRPC_URL:  ${CS_GRPC_URL}     # external, e.g. cs.customer.local:8080
      CS_API_KEY:   ${CS_API_KEY}
      MQTT_URL:     ${MQTT_URL}        # external broker
      MQTT_USER:    ${MQTT_USER}
      MQTT_PASS:    ${MQTT_PASS}
      FILES_DIR:    /var/lib/shifter/files
    volumes: [shifter_files:/var/lib/shifter/files]

  shifter-frontend:
    build: ./frontend

  db:
    image: timescale/timescaledb:latest-pg16

volumes: { db_data:, shifter_files:, caddy_data: }
```

The Shifter backend is **bit-identical** between the two modes. Only the env vars change. Customers who run ChirpStack on Kubernetes, on a separate VM, or in a managed setting plug those URLs in.

### Pattern 7: File Storage on the Filesystem (with S3 escape hatch)

**What:** Floor-plan images go on a local Docker volume (`/var/lib/shifter/files`) referenced by row in `floor_plan.path`. Use a thin `files` package that abstracts the storage so a future `S3_BUCKET=...` env var swaps in MinIO/S3 without touching call sites.

**When to use:** Default for self-hosted single-tenant. The volume is part of the same backup as the DB. No extra service.

**Trade-offs:** No multi-instance scaling — but Shifter is single-instance per install, so this is fine. The S3-compatible escape hatch costs little to leave open.

**Why not always MinIO:** Adds a service, requires bucket policy + credentials, and floor-plans are dozens of images per install, not millions of objects. Filesystem wins on simplicity for v1.

## Build Order Implications

Because everything else depends on a working ingestion path and a stable measurement schema, phases must be ordered:

1. **Phase 1 — Skeleton + ChirpStack adapter (foundation).**
   Single Postgres with Timescale, Caddy reverse proxy, Shifter binary that can:
   - Connect to ChirpStack gRPC and list devices/gateways.
   - Subscribe to MQTT and log events to stdout (no DB write yet).
   - Local auth + a single hardcoded admin user.

2. **Phase 2 — Metering-point + measurement model (the lifecycle anchor).**
   - `metering_point`, `device`, `metering_point_assignment`, `measurement` hypertable.
   - Ingestion service does normalize → resolve → insert end-to-end.
   - One device family supported by the normalizer (pick the most common vendor first).
   - REST endpoints for sites/metering-points/devices CRUD.
   - **Meter-swap flow** (the core differentiator) implemented and tested.

3. **Phase 3 — Realtime + dashboard.**
   - LISTEN/NOTIFY trigger, WS hub, frontend subscription.
   - Per-meter detail view (normal + advanced JSONB-driven).
   - shadcn-based site list, device list, dashboard.

4. **Phase 4 — Aggregates + reports.**
   - Continuous aggregates: hourly/daily/monthly/yearly.
   - Reports endpoints + CSV/Excel/PDF export.
   - Map view via Leaflet/MapLibre.
   - Floor-plan upload + pixel-coordinate device placement.

5. **Phase 5 — Alerts + operational hardening.**
   - Threshold, anomaly, offline-watchdog alerts.
   - Bulk import (CSV).
   - User management UI.
   - Bundled vs external deployment validation.

6. **Phase 6 — Multi-vendor breadth.**
   - Add normalizer mappings for additional device families.
   - Refine "advanced view" rendering of `extra` JSONB.

The metering-point abstraction (Phase 2) and canonical measurement model (Phase 2) are non-negotiable foundations. Building dashboards (Phase 3) on a device-keyed schema is the single most expensive mistake possible — it would force a data migration the first time a meter is replaced.

## Scaling Considerations

Shifter is single-tenant per install, so "scale" here means "growth within one customer."

| Scale | Architecture Adjustments |
|-------|--------------------------|
| Up to 10 gateways, 500 meters, 1k uplinks/hour | Default monolith. Single Postgres, single Shifter process. No tuning needed. |
| 10–50 gateways, 5k meters, 50k uplinks/hour | Add Postgres connection pooling (PgBouncer if not built in). Tune Mosquitto QoS. Confirm CAGG refresh policy frequency vs query latency. |
| 50+ gateways, 50k+ meters, > 1M uplinks/hour | Move ChirpStack to external mode on dedicated host. Consider running the ingestion module as a separate process from the API module (still same codebase). Switch realtime fanout from LISTEN/NOTIFY to Redis pub/sub. Enable Timescale compression on chunks > 30 days old. |

### Scaling Priorities

1. **First bottleneck:** PostgreSQL write throughput on the `measurement` hypertable, typically when gateway count grows past ~50. Mitigation: Timescale compression on old chunks, and batched inserts in the ingestion service (group N events for ≤200 ms before flushing).
2. **Second bottleneck:** WebSocket fan-out to a large number of dashboard sessions. Mitigation: switch the hub to Redis pub/sub, keep LISTEN/NOTIFY only as the producer signal.
3. **Third bottleneck:** Report generation (PDF/Excel) blocking the API. Mitigation: move to a background job + downloadable artifact pattern, signaled to the user via WS.

## Anti-Patterns

### Anti-Pattern 1: Coupling realtime push directly to the MQTT subscriber

**What people do:** Subscribe MQTT events and immediately fan out to WebSocket sessions, before the DB write.

**Why it's wrong:** The dashboard "sees" data that hasn't been persisted yet — sometimes ever (if a normalizer or DB error rejects it). Reload shows different data than the live update did. Worse, every WS session must filter every event, scaling poorly.

**Do this instead:** Persist first, push second. Use `pg_notify` from the DB trigger as the signal. Realtime is a side-effect of a successful write.

### Anti-Pattern 2: Storing the device identifier in the measurement table as the primary correlation key

**What people do:** `measurement(time, dev_eui, …)` because that's what the uplink event contains.

**Why it's wrong:** When the meter is replaced, every chart query must `UNION` across old and new dev_euis with offset arithmetic in user code. Reports break. History is fragile.

**Do this instead:** Resolve to `metering_point_id` at ingest time, store that as the correlation key. `dev_eui` stays in the row for traceability but is not the query key.

### Anti-Pattern 3: Re-implementing ChirpStack features in Shifter

**What people do:** Build their own device-profile registry, codec runtime, gateway management, or join-procedure handling because "we want full control."

**Why it's wrong:** ChirpStack is a mature, battle-tested LoRaWAN NS/AS. Reinventing it doubles the codebase and triples the bug surface. The "wrap, don't replace" mandate from the project brief exists for this reason.

**Do this instead:** Use ChirpStack's gRPC for management; collapse the multi-step UX flows in Shifter. The user never sees ChirpStack, but the wire format and protocol stack are ChirpStack's.

### Anti-Pattern 4: Per-vendor measurement tables or per-vendor backend modules

**What people do:** A `water_meter_measurement` and `electricity_meter_measurement` table — or worse, a table per vendor.

**Why it's wrong:** Adding a new vendor becomes a schema migration. Joining across vendors for "all sites" reports becomes painful. Aggregate queries multiply.

**Do this instead:** One canonical `measurement` table with optional first-class fields plus `extra JSONB`. The normalizer is the only vendor-aware code; below it, everything is canonical.

### Anti-Pattern 5: Application-side daily/monthly/yearly rollups in cron jobs

**What people do:** A nightly job that scans the day's measurements and writes a `daily_consumption` row.

**Why it's wrong:** Brittle, slow, and re-runs hurt. Late-arriving data invalidates the row. Operators forget the cron exists.

**Do this instead:** Continuous aggregates with refresh policies. Timescale handles late data, incremental refresh, and exposes the result as a queryable relation.

### Anti-Pattern 6: One `docker-compose.yml` that hardcodes "bundled"

**What people do:** Customers get the bundled stack and then have to fork the compose file to remove ChirpStack/Mosquitto for external mode.

**Why it's wrong:** Diverging compose files per customer = unmaintainable.

**Do this instead:** Two compose files (`docker-compose.yml`, `docker-compose.external.yml`) that share `.env` shape. Backend reads URLs from env; everything else is a Docker concern.

## Integration Points

### External Services

| Service | Integration Pattern | Notes |
|---------|---------------------|-------|
| ChirpStack (management) | gRPC client over HTTP/2, API-key auth | One persistent connection per CS service stub; auto-reconnect. Wrap to expose domain-shaped methods. |
| ChirpStack (events) | MQTT subscriber on `application/+/device/+/event/+` | JSON encoding (set `json=true` in CS config). Use `+` wildcards to avoid per-app subscription churn. |
| OpenStreetMap tiles | HTTPS GET from browser | No backend involvement; respect tile usage policy or self-host tiles for high-volume installs. |
| Reverse proxy (Caddy) | Edge process, TLS termination | Auto-HTTPS via Let's Encrypt for customers with public DNS; self-signed/internal CA for air-gapped. |

### Internal Boundaries

| Boundary | Communication | Notes |
|----------|---------------|-------|
| Ingestion ↔ API | Shared DB only | Never direct calls. The DB is the seam. |
| API ↔ ChirpStack adapter | In-process function calls | Adapter is the only module that knows ChirpStack proto types. |
| Realtime hub ↔ ingestion | Postgres LISTEN/NOTIFY | One channel `measurements`. Hub never opens MQTT. |
| Frontend ↔ backend | HTTPS + WS | REST for CRUD, WS for live measurement push, simple bearer-token auth. |
| Background jobs ↔ everything else | Read DB, may call ChirpStack adapter for offline-watchdog gateway pings | No event bus; cron-style scheduling inside the binary. |

## Sources

- [ChirpStack v4 architecture](https://www.chirpstack.io/docs/architecture.html)
- [ChirpStack MQTT integration docs](https://www.chirpstack.io/docs/chirpstack/integrations/mqtt.html) — topic structure, JSON event schema, decoded `object` field
- [ChirpStack v4 breaking changes](https://www.chirpstack.io/docs/v4-breaking-changes.html) — gRPC-first API surface, no embedded REST
- [ChirpStack device profiles + JS codec](https://www.chirpstack.io/docs/chirpstack/use/device-profiles.html) — QuickJS-based JS codecs producing decoded `object`
- [ChirpStack Docker reference compose](https://github.com/chirpstack/chirpstack-docker) — canonical bundled stack (CS + Mosquitto + Postgres + Redis + gateway-bridge)
- [ChirpStack v4 configuration](https://www.chirpstack.io/docs/chirpstack/configuration.html) — `[integration.mqtt]` section, JSON vs Protobuf encoding
- [ChirpStack MQTT Integration deep-dive (DeepWiki)](https://deepwiki.com/chirpstack/chirpstack/6.1-mqtt-integration)
- [TimescaleDB continuous aggregates](https://docs.timescale.com/getting-started/latest/aggregation/) — incremental refresh, hierarchical CAGGs
- [Tiger Data: building IoT pipelines on Postgres + Timescale](https://www.tigerdata.com/blog/how-to-build-an-iot-pipeline-for-real-time-analytics-in-postgresql)
- [SQL patterns for IoT in TimescaleDB](https://dev.to/tigerdata/sql-patterns-for-optimizing-iot-queries-in-timescaledb-kp9)
- [pg_eventserv — Postgres LISTEN/NOTIFY → WebSocket bridge](https://www.crunchydata.com/blog/real-time-database-events-with-pg_eventserv)
- [Postgres LISTEN/NOTIFY for realtime](https://www.pedroalonso.net/blog/postgres-listen-notify-real-time/)
- [sqlx PgListener (Rust) reference](https://docs.rs/sqlx/latest/sqlx/postgres/struct.PgListener.html)
- [Caddy as a Docker reverse proxy](https://github.com/lucaslorentz/caddy-docker-proxy) — TLS termination + single-port ingress
- [Smart-meter MDMS architecture overview](https://www.sciencedirect.com/topics/engineering/meter-data-management-system) — historical-continuity rationale for the metering-point abstraction

---
*Architecture research for: self-hosted LoRaWAN water/electricity monitoring platform wrapping ChirpStack v4*
*Researched: 2026-04-27*
