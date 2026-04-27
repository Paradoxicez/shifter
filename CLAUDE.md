<!-- GSD:project-start source:PROJECT.md -->
## Project

**Shifter**

Shifter is a self-hosted water and electricity monitoring platform that wraps ChirpStack as its LoRaWAN backend. It gives each customer one app to manage gateways, sites, devices, and meters — replacing the need to ever touch ChirpStack directly — and presents real-time and historical consumption through a modern, minimal dashboard. Each install serves a single customer (single-tenant per install) and is deployed by us on the customer's infrastructure.

**Core Value:** The operator runs their entire LoRaWAN water/electricity monitoring operation — provisioning, placement, monitoring, reporting — from Shifter alone, and meter swaps never break historical continuity.

### Constraints

- **Tech stack — backend ↔ ChirpStack:** Must communicate over gRPC (ChirpStack's recommended integration surface)
- **Tech stack — time-series:** TimescaleDB (Postgres extension) for telemetry — single database to deploy and back up, native daily/monthly/yearly continuous aggregates
- **Tech stack — frontend:** shadcn/ui plus the shadcn ecosystem (charts, layout/blocks); blue/navy palette; modern minimal
- **Tech stack — maps:** OpenStreetMap via Leaflet or MapLibre (no paid API keys at install time)
- **Deployment:** Self-hosted per customer; install must be a single, scripted operation we run for each customer; ChirpStack must be optionally bundleable (Docker Compose) so a customer with no existing LoRaWAN stack can be onboarded end-to-end
- **Interaction model:** All CRUD via dialogs; multi-step external flows (e.g. ChirpStack device activation) condensed into one user action or, at most, a stepped dialog
- **Tenancy:** Single-tenant per install — no cross-customer data isolation logic needed
- **Auth:** Local accounts only (email + password); no SMTP dependency in v1
<!-- GSD:project-end -->

<!-- GSD:stack-start source:research/STACK.md -->
## Technology Stack

## Primary Recommendation: Backend Language
### **Go (Golang) — recommended**
- **Node.js/TypeScript** — The shared-types-with-frontend pull is real, but `@chirpstack/chirpstack-api` exists and is functional, not best-of-breed. Node loses single-binary, gains a runtime dependency tree, and has weaker production stories for long-running MQTT consumers (event-loop starvation under high-throughput uplink bursts is a real concern). If the team had no Go experience and strong TS, Node would be defensible; for a greenfield self-hosted IoT backend, Go is the better default.
- **Python (FastAPI)** — Fast to write, but: (a) the Python ChirpStack SDK is the same generated stubs as Go, no advantage; (b) deploying Python self-hosted means picking poison between Docker images (50–80MB), system Python (version drift across customer machines), or PyInstaller (slow, fragile). FastAPI's strengths (Pydantic, OpenAPI, async I/O) don't outweigh deploy friction for this product.
- **Rust** — ChirpStack itself is Rust and the team could even contribute upstream. But Rust's compile times and learning curve cost more velocity than Go's verbosity, and the gRPC + MQTT + TimescaleDB Go ecosystem is more mature/documented today.
## Recommended Stack
### Core Technologies
| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| **Go** | 1.24+ (Go 1.26 GA Feb 2026 brings Green Tea GC) | Backend language | gRPC native, single-binary deploy, mature MQTT/Postgres/scheduling ecosystem |
| **PostgreSQL** | 16 or 17 | OLTP store (users, sites, devices, metering points) | Required by TimescaleDB; mature, single DB to back up |
| **TimescaleDB** | 2.26.0 (March 2026) | Time-series telemetry, continuous aggregates for daily/monthly/yearly | Postgres extension — one DB to deploy. 2.26 dropped trigger-based CAGG invalidation (10–20% faster ingest). Native time_bucket aggregation maps directly to Shifter's reporting requirements |
| **ChirpStack** | v4 (latest) | LoRaWAN Network Server | The product wraps it; non-negotiable per PROJECT.md |
| **MQTT broker** | Mosquitto 2.x or NanoMQ | ChirpStack → Shifter event bus | ChirpStack publishes uplink events over MQTT (also Redis Streams, AMQP, Kafka, HTTP). MQTT is the simplest of those for a self-hosted bundle |
| **React** | 19.x | Frontend framework | shadcn/ui + react-leaflet v5 both require React 19 peer dep |
| **Vite** | 7.x | Frontend build tool | The shadcn/ui official guide treats Vite + React + TS as the canonical SPA path; faster cold starts and HMR than Next.js Turbopack |
| **TypeScript** | 5.6+ | Frontend type safety | Standard for any non-trivial React app in 2026 |
| **Tailwind CSS** | 4.x | Styling primitives | shadcn/ui v4-era is built on Tailwind v4 |
| **Docker / Docker Compose** | latest | Deployment unit | The PROJECT.md install model is "one scripted operation we run for each customer" — Compose is the right shape |
### Backend Supporting Libraries (Go)
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/chirpstack/chirpstack/api/go/v4` | v4.17.0+ | Official ChirpStack gRPC client stubs | All gateway/device/application/tenant CRUD against ChirpStack |
| `google.golang.org/grpc` | v1.66+ | gRPC transport for the ChirpStack client | Required by chirpstack-api |
| `github.com/eclipse/paho.mqtt.golang` | v1.5+ | MQTT client | Subscribe to `application/+/device/+/event/up` to ingest uplinks; ChirpStack also uses it under the hood |
| `github.com/jackc/pgx/v5` | v5.7+ | Postgres driver + connection pooling | The production-grade Postgres driver for Go; used by sqlc-generated code; pq is in maintenance mode |
| `github.com/sqlc-dev/sqlc` | v1.31+ | SQL → typed Go code generator | Write SQL, get typed Go. TimescaleDB-specific functions (time_bucket, locf, etc.) work natively because sqlc never abstracts SQL away. *Do not use GORM* |
| `github.com/go-chi/chi/v5` | v5.1+ | HTTP router for the REST API the SPA talks to | Lightweight, idiomatic, plays nicely with stdlib `net/http` and Go 1.22+ pattern routing. Gin is fine if the team prefers it, but Chi composes better with stdlib middleware |
| `github.com/golang-migrate/migrate/v4` | v4.18+ | Database migrations | Standard Go migration tool; SQL-file based; ships as a CLI for use in install scripts |
| `golang.org/x/crypto/argon2` | latest | Password hashing | Argon2id is the OWASP-recommended hash. `golang.org/x/crypto/bcrypt` is also fine; pick one and don't mix |
| `github.com/alexedwards/scs/v2` | v2.8+ | Server-side session manager | Cookie-based sessions backed by Postgres; no JWT complexity, easy revocation, fits "local-only auth, no SSO" |
| `github.com/riverqueue/river` | v0.13+ | Background job queue | Postgres-backed (no Redis dependency!) job queue with cron support. Perfect for daily/monthly/yearly aggregate refreshes and alert evaluation. Asynq is a strong alternative but adds Redis to the install footprint |
| `github.com/xuri/excelize/v2` | v2.10+ | XLSX generation | Pure-Go, actively maintained (2.10.x released Feb 2026), used by 2,400+ projects. The Go XLSX library |
| `github.com/johnfercher/maroto/v2` | v2.x | PDF generation | Bootstrap-grid-style API, fast enough for monthly/yearly reports. *Avoid `gofpdf`* — archived since 2021 |
| `encoding/csv` (stdlib) | — | CSV import / export | Stdlib is faster than third-party (no reflection); use `csvutil` only if struct-tag mapping ergonomics matter more than throughput |
| `github.com/spf13/viper` + `github.com/spf13/cobra` | latest | Config + CLI | Standard Go config-file + env-var + flags layering; Cobra for `shifter migrate`, `shifter serve`, `shifter create-admin` subcommands |
| `log/slog` (stdlib) | — | Structured logging | Stdlib since Go 1.21; no third-party dependency needed |
### Frontend Supporting Libraries
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `shadcn/ui` | latest CLI | Component library | Required by PROJECT.md. Use the CLI workflow (`npx shadcn add ...`) — components are copied into the repo, not installed as a dep |
| `tailwindcss` | 4.x | Styling | Required by shadcn/ui |
| `lucide-react` | latest | Icon set | shadcn's default icon library |
| `recharts` | v3.x | Charts (used internally by shadcn/ui chart components) | shadcn's chart blocks are Recharts wrappers; sufficient for Shifter's consumption time-series; for >100K-point views consider downsampling at the Postgres layer (TimescaleDB `time_bucket_gapfill`) before the chart sees the data |
| `@tanstack/react-query` | v5.x | Server state / data fetching | The dashboard standard. SWR is lighter (4KB vs 13KB) but TanStack Query's mutation/cache primitives matter once you have create/edit/delete dialogs everywhere — and Shifter is dialog-heavy by design |
| `react-leaflet` | 5.0.0 | OSM map view of sites/devices | Lightweight (Leaflet ~42KB vs MapLibre ~290KB), v5 supports React 19. **Use this, not MapLibre/react-map-gl** — Shifter's map use case is "show site pins on OSM tiles," which is exactly Leaflet's sweet spot. MapLibre is overkill (vector tiles, WebGL, 3D) for what amounts to a marker layer |
| `leaflet` | 1.9.x | Underlying Leaflet library | Peer of react-leaflet |
| `react-image-pin` | latest | Floor-plan pin drag/drop on uploaded images | Built specifically for "drop pins on a static image with pixel coordinates" — exactly the floor-plan device-placement requirement. Alternatives: `react-image-annotation` (more general but heavier), or a custom `dnd-kit`-based component if the canned library lacks needed control |
| `react-router-dom` | v7.x | Client-side routing | Standard for Vite SPAs; no server needed |
| `zod` | v3.23+ | Runtime validation for forms and API responses | Pairs with `react-hook-form` for shadcn form components |
| `react-hook-form` | v7.53+ | Form state | shadcn's official form recipe |
| `date-fns` | v4.x | Date math | Lightweight, tree-shakeable; use over moment/dayjs |
| `sonner` | latest | Toasts | shadcn's official toast component |
### Realtime Push to UI
- Data flow is one-way (server → browser). SSE is the boring, correct tool.
- Native browser API (`EventSource`), automatic reconnection, no extra client lib.
- Plays nicely with HTTP/2, plain HTTP, reverse proxies (nginx, Caddy) without WebSocket-specific config.
- No MQTT-over-WS exposing the broker (and therefore *all* customer telemetry topics) directly to the browser. Exposing the MQTT broker also leaks the integration architecture into the auth surface — bad fit.
- WebSocket is the right pick later if/when there's bidirectional needs (e.g., live device control). For v1, SSE is simpler.
### Vendor-Agnostic Measurement Model (TimescaleDB)
- The handful of fields the UI *always* renders (cumulative_value, battery, rssi) are first-class columns — fast queries, indexable, type-safe.
- Anything new a vendor sends lands in `raw` (JSONB) and is immediately surfaced in the per-meter "advanced" view via `jsonb_each`.
- Adding a new vendor doesn't require a schema migration. Promoting a new field from `raw` to a column is a one-time backfill.
- TigerData (TimescaleDB's vendor) explicitly recommends this pattern for IoT multi-vendor schemas.
- Continuous aggregates roll up the canonical columns. The `raw` JSONB doesn't get aggregated (which is fine — nobody wants to roll up "10 vendor-specific debug fields by month").
- *Pure narrow EAV* (`metric_name TEXT, value DOUBLE`): row explosion, awful query ergonomics, breaks continuous-aggregates patterns.
- *Pure wide* (one column per vendor parameter): every vendor onboarding is a migration; sparse storage is wasteful.
- *Per-vendor table*: breaks "metering point persists, physical device swaps" — would require UNION queries or a view layer for every cross-vendor report.
### Deployment
- `docker-compose.bundled.yml` — Postgres+TimescaleDB, Mosquitto, ChirpStack, Shifter. For greenfield customers.
- `docker-compose.external.yml` — Postgres+TimescaleDB, Mosquitto, Shifter only. Customer points Shifter at their existing ChirpStack (gRPC URL + API key + MQTT URL via env vars).
### Development Tools
| Tool | Purpose | Notes |
|------|---------|-------|
| `golangci-lint` | Lint | The de facto Go meta-linter; pin a version in CI |
| `gofumpt` | Stricter formatter than `gofmt` | Optional but nice |
| `air` (cosmtrek/air) | Live-reload Go binary in dev | One-line config; rebuild on file change |
| `mockgen` (uber-go/mock) | Interface mocks for tests | Vendor-mock the ChirpStack gRPC client and MQTT client at the interface boundary |
| `pnpm` | Frontend package manager | Faster than npm, deterministic, monorepo-friendly if Shifter ever splits |
| `biome` or `eslint+prettier` | TS/JS lint+format | Biome is faster and merging the toolchain; eslint+prettier still safe default |
| `playwright` | E2E tests | Better Vite/SPA story than Cypress in 2026 |
## Installation
### Backend (Go) starter dependencies
# ChirpStack + gRPC
# MQTT
# Database
# Web + auth
# Jobs / scheduling
# Reporting
# CLI / config
# Dev tools
### Frontend (Vite + React + TS + shadcn) bootstrap
# follow shadcn/ui Vite install guide
# Data + forms
# Routing
# Maps + floor plans
# Charts (auto-installed by shadcn chart blocks)
# Misc
# Dev
## Alternatives Considered
| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|-------------------------|
| **Go** | Node.js / TypeScript | Team has zero Go experience and strong TS; willing to accept node_modules/deploy weight |
| **Go** | Python (FastAPI) | Team is heavily Python and the deploy is *not* per-customer self-hosted (hosted SaaS, single deploy) |
| **Vite SPA** | Next.js 15 (static export) | If a public, SEO-indexed portion of the product appears later |
| **Vite SPA** | Next.js 15 (server) | If you need RSC streaming for very large dashboards or want server-side data hydration. Shifter does not |
| **react-leaflet (Leaflet)** | react-map-gl + MapLibre GL JS | If you need vector tiles, custom map styling, 3D extrusion, or are rendering >5K markers on a single map |
| **sqlc + pgx** | GORM | Team prefers active-record / model-first; willing to lose 30–50% query performance and SQL clarity |
| **sqlc + pgx** | Bun (uptrace/bun) | Want a query builder middle ground (more Go-idiomatic than raw SQL, more flexible than GORM) |
| **River (Postgres jobs)** | Asynq (Redis jobs) | Already have Redis in the stack; want Asynq's web UI; need very high throughput |
| **River** | gocron | Only need cron-style scheduling, no durable retries needed (worse fit for alert evaluation, fine for "refresh CAGG nightly") |
| **chi** | Gin / Echo | Team is more familiar with one of those; both are stable enough |
| **SCS sessions** | JWT (golang-jwt/jwt) | Stateless auth required (e.g., multi-instance behind LB without shared session store). Shifter is single-tenant single-install; sessions in Postgres are simpler |
| **SSE** | WebSocket | Need bidirectional realtime later (live device control, collaborative cursor, etc.) |
| **Mosquitto** | NanoMQ / EMQX | Need MQTT v5 features, MQTT over QUIC, or higher throughput. For Shifter's per-customer install, Mosquitto's footprint is unbeatable |
| **excelize** | tealeg/xlsx | Excelize is the better-maintained answer in 2026 — only use tealeg/xlsx for legacy reasons |
| **maroto (PDF)** | chromedp + headless Chrome | If you need pixel-perfect HTML→PDF and don't mind a 300MB binary for the Chromium dep. For invoice/report-style PDFs, Maroto is dramatically lighter |
## What NOT to Use
| Avoid | Why | Use Instead |
|-------|-----|-------------|
| **GORM** | 30–50% slower than pgx; SQL abstraction breaks down on TimescaleDB-specific queries (time_bucket, continuous aggregates, hyperfunctions); generated SQL is opaque when you need to optimize | sqlc + pgx |
| **lib/pq (`github.com/lib/pq`)** | In maintenance mode since 2021; ecosystem has moved to pgx | jackc/pgx/v5 |
| **gofpdf** | Archived 2021, no maintenance, poor unicode/CJK | maroto/v2 (built on gofpdf but actively maintained), or `gpdf` for newer projects |
| **MapLibre GL JS / react-map-gl** for this project | 290KB gzipped vs 42KB Leaflet; vector-tile/WebGL machinery is unused for "site pins on OSM" | react-leaflet v5 |
| **Mapbox GL JS** | Requires API key and account; PROJECT.md explicitly excludes paid map APIs | OSM via Leaflet |
| **Google Maps** | Same — paid API, excluded by PROJECT.md | OSM via Leaflet |
| **Next.js for the dashboard** | All the SSR/RSC machinery is dead weight behind a login wall; deploys a Node server you didn't need | Vite + React SPA, served by the Go binary |
| **MQTT-over-WebSocket exposed to the browser** | Leaks broker auth and entire telemetry topic tree into the browser security boundary; no per-user filtering | Backend MQTT subscriber + per-user SSE fan-out |
| **JWT in localStorage** | XSS-exfiltratable; rotation is hard; Shifter has no SSO/multi-service requirement | Server-side sessions in Postgres (SCS), httpOnly cookie |
| **bcrypt + argon2 mixed** | Fine to migrate one→other, but don't have *both* live. Pick one | Argon2id (golang.org/x/crypto/argon2) |
| **GORM / Bun migrations** | ORM-coupled migrations make schema-only DBA review harder | golang-migrate (plain SQL files) |
| **Kafka or RabbitMQ for ChirpStack→Shifter events** | ChirpStack supports them, but they double the install footprint for one customer | MQTT (Mosquitto) |
| **Redis** *unless needed* | Adds an install dependency. River uses Postgres for jobs; SCS uses Postgres for sessions. Only add Redis if Asynq is preferred or rate-limit/cache requirements appear | Postgres-backed everything until Redis is justified |
## Stack Patterns by Variant
- Use `docker-compose.external.yml` template
- Configure `CHIRPSTACK_GRPC_URL`, `CHIRPSTACK_API_KEY`, `MQTT_URL` via environment
- Shifter does not bundle Mosquitto in this mode (uses customer's existing broker)
- Use `docker-compose.bundled.yml`
- Bundles Postgres+TimescaleDB, Mosquitto, ChirpStack v4, Shifter
- Single `docker compose up -d`; ports for Mosquitto (1883) and ChirpStack (8080 admin) are *not* exposed externally — only the Shifter HTTP port
- Drive UI from a `customer.capabilities = {water: true|false, electricity: true|false}` config (set at install time, derived from device profiles after first uplinks)
- Hide nav and dashboard tiles for absent capabilities — single codebase, no separate builds
- Add a Postgres read replica for the report endpoints
- Move TimescaleDB compression policies to >7 days old (massive disk savings on raw measurement rows)
- Consider Redis for SSE fan-out if connection count > ~1K (single Go process handles low-thousands fine)
## Version Compatibility
| Package A | Compatible With | Notes |
|-----------|-----------------|-------|
| Go 1.24+ | excelize v2.10+ | excelize 2.10 requires Go 1.24 |
| TimescaleDB 2.26 | PostgreSQL 14, 15, 16, 17 | Pin to PG 16 or 17 for new installs |
| react-leaflet 5.0 | React 19+ | Hard peer dep — do not pair with React 18 |
| shadcn/ui current | Tailwind CSS 4 + React 19 | Tailwind 3 path is legacy |
| chirpstack-api/go v4 | google.golang.org/grpc v1.66+ | Track ChirpStack server version; mismatched protobuf schema = silent field drops |
| sqlc 1.31+ | pgx/v5 | sqlc 1.18+ added pgx/v5 driver support |
| River v0.13+ | pgx/v5 | Use `riverdriver/riverpgxv5` |
| Vite 7 | React 19 + TypeScript 5.6+ | Standard 2026 SPA combo |
## Sources
- https://www.chirpstack.io/docs/chirpstack/api/grpc.html — gRPC SDK languages (Go, Python, JS, Rust)
- https://www.chirpstack.io/docs/chirpstack/integrations/mqtt.html — MQTT integration is canonical event delivery; topic structure `application/+/device/+/event/up`
- https://pkg.go.dev/github.com/chirpstack/chirpstack/api/go/v4 — current v4.17.0 (March 2026)
- https://www.chirpstack.io/docs/chirpstack/api/go-examples.html — official Go usage pattern
- https://deepwiki.com/chirpstack/chirpstack/6-integrations-and-extensions — full list of integration backends (HTTP, MQTT, AMQP, Kafka, Redis Streams, PostgreSQL)
- https://github.com/timescale/timescaledb/releases — 2.26.0 (March 24, 2026); 2.25.0 (Jan 29, 2026)
- https://docs.timescale.com/about/latest/changelog/ — CAGG invalidation refactor 10–20% faster DML
- https://www.tigerdata.com/learn/designing-your-database-schema-wide-vs-narrow-postgres-tables — wide+JSONB hybrid pattern for IoT
- https://www.tigerdata.com/learn/best-practices-time-series-data-modeling-single-or-multiple-partitioned-tables-aka-hypertables — single-hypertable guidance
- https://www.index.dev/skill-vs-skill/backend-go-vs-nodejs-vs-python-fastapi — Go vs Node vs Python tradeoffs
- https://dev.to/_d7eb1c1703182e3ce1782/python-vs-go-for-backend-development-in-2026-an-honest-comparison-3pno — single-binary, deploy size data
- https://encore.dev/articles/best-go-backend-frameworks — Chi/Gin/Echo positioning
- https://pkg.go.dev/github.com/jackc/pgx/v5 — pgx v5
- https://docs.sqlc.dev/en/latest/guides/using-go-and-pgx.html — sqlc + pgx integration (sqlc 1.31)
- https://github.com/eclipse-paho/paho.mqtt.golang — paho.mqtt.golang
- https://github.com/riverqueue/river — Postgres-backed job queue
- https://github.com/xuri/excelize — excelize 2.10 (Feb 2026)
- https://github.com/golang-migrate/migrate — migrations
- https://github.com/alexedwards/scs — session manager (recommended in 2026 Go auth guides)
- https://workos.com/blog/go-authentication-guide — Go auth patterns 2026
- https://ui.shadcn.com/docs/installation/vite — official Vite install path
- https://www.npmjs.com/package/react-leaflet — react-leaflet 5.0.0 (React 19 peer)
- https://maplibre.org/maplibre-gl-js/docs/guides/leaflet-migration-guide/ — Leaflet vs MapLibre tradeoffs
- https://designrevision.com/blog/vite-vs-nextjs — Vite-for-dashboards consensus 2026
- https://github.com/art29/react-image-pin — image pin drag-drop library
- https://websocket.org/comparisons/ — SSE vs WebSocket vs MQTT comparison
- https://dev.to/young_gao/server-sent-events-the-underrated-alternative-to-websockets-for-real-time-notifications-1i1f — SSE for one-way push
- https://docraptor.com/go-pdf-generation — gofpdf archived, maroto active
- https://blog.logrocket.com/go-long-generating-pdfs-golang-maroto/ — maroto v2 patterns
- https://pkg.go.dev/encoding/csv — stdlib CSV is faster than gocsv
<!-- GSD:stack-end -->

<!-- GSD:conventions-start source:CONVENTIONS.md -->
## Conventions

Conventions not yet established. Will populate as patterns emerge during development.
<!-- GSD:conventions-end -->

<!-- GSD:architecture-start source:ARCHITECTURE.md -->
## Architecture

Architecture not yet mapped. Follow existing patterns found in the codebase.
<!-- GSD:architecture-end -->

<!-- GSD:skills-start source:skills/ -->
## Project Skills

No project skills found. Add skills to any of: `.claude/skills/`, `.agents/skills/`, `.cursor/skills/`, or `.github/skills/` with a `SKILL.md` index file.
<!-- GSD:skills-end -->

<!-- GSD:workflow-start source:GSD defaults -->
## GSD Workflow Enforcement

Before using Edit, Write, or other file-changing tools, start work through a GSD command so planning artifacts and execution context stay in sync.

Use these entry points:
- `/gsd-quick` for small fixes, doc updates, and ad-hoc tasks
- `/gsd-debug` for investigation and bug fixing
- `/gsd-execute-phase` for planned phase work

Do not make direct repo edits outside a GSD workflow unless the user explicitly asks to bypass it.
<!-- GSD:workflow-end -->



<!-- GSD:profile-start -->
## Developer Profile

> Profile not yet configured. Run `/gsd-profile-user` to generate your developer profile.
> This section is managed by `generate-claude-profile` -- do not edit manually.
<!-- GSD:profile-end -->
