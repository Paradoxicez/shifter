# Shifter

**Self-hosted LoRaWAN water and electricity monitoring.** Shifter wraps ChirpStack v4 as its LoRaWAN backend and gives operators a single console to manage gateways, sites, devices, meters, and consumption — without ever touching ChirpStack directly. Single-tenant per install; deployed by us on the customer's infrastructure.

## Why Shifter

- **Wrap, don't replace.** ChirpStack v4 stays the LoRaWAN network server; Shifter speaks customer/site/device language and collapses ChirpStack's multi-step flows into single dialogs.
- **Meter swaps don't break history.** Telemetry is keyed on a stable `metering_point_id`; physical meters can be swapped without losing continuity (Phase 2 delivers this).
- **Two compose flavors.** Bundled (Postgres+TimescaleDB+Mosquitto+ChirpStack+Shifter) for greenfield. External (Postgres+TimescaleDB+Shifter) for customers who already operate ChirpStack.

## Status

Phase 1 (Foundation) is complete: scripted install, local auth, install wizard, ChirpStack v4 integration (gRPC + MQTT), Test Connection, and the modal-first UX shell.

Phase 2 (Domain Model & Canonical Schema) is next — the meter-swap differentiator and the canonical telemetry schema land there.

See [`.planning/ROADMAP.md`](.planning/ROADMAP.md) for the full 7-phase plan.

## Quick Start (development)

Prerequisites: Go 1.24+, Node 22+, pnpm 10+, Docker, `just` (`brew install just` or `cargo install just`).

```sh
just bootstrap     # installs air, sqlc, mockgen; pnpm install in web/
just dev           # runs Air + Vite concurrently (api on :8080, ui on :5173)
```

In a separate terminal, start a dev Postgres + Mosquitto + ChirpStack:
```sh
(cd compose && docker compose -f bundled.yml up -d postgres mosquitto chirpstack chirpstack-gateway-bridge chirpstack-rest-api redis)
```

Visit `http://localhost:5173` — the install wizard will walk you through the rest.

## Production Install

Pick the right compose flavor:

| Flavor | When to use | Services |
|--------|------------|----------|
| **Bundled** | Customer has no existing LoRaWAN stack | Postgres+TimescaleDB, Mosquitto, ChirpStack v4 (+gateway-bridge, rest-api, Redis), Caddy, Shifter |
| **External** | Customer already operates ChirpStack v4 + an MQTT broker | Postgres+TimescaleDB, Caddy, Shifter |

### Bundled flavor

```sh
./install/bundled/install.sh shifter.example.com
# then visit https://shifter.example.com/install
```

### External flavor

```sh
cp install/external/.env.example install/external/.env
# Edit .env: set SHIFTER_DOMAIN, SHIFTER_CHIRPSTACK_GRPC_URL, SHIFTER_MQTT_URL
# Place the ChirpStack API token at secrets/chirpstack_api_token.txt (chmod 0600)
./install/external/install.sh
# then visit https://<your domain>/install
```

See [`docs/install.md`](docs/install.md) for the full walkthrough including TLS modes (`acme` / `byo` / `internal`).

## CLI

The single binary `shifter` exposes:

| Command | What it does |
|---------|-------------|
| `shifter serve` | Run migrations, then start the HTTP listener (D-13) |
| `shifter migrate up` | Apply pending schema migrations |
| `shifter migrate force <N>` | Mark schema clean at version N (dirty-state recovery) |
| `shifter version` | Print binary version + commit + build time |
| `shifter create-admin --email --password [--reset]` | Create or reset admin user (recovery escape hatch) |
| `shifter config-check` | Validate config syntax + ping Postgres + ChirpStack gRPC + MQTT |
| `shifter healthcheck` | Localhost GET /health for use as Docker `HEALTHCHECK` |

## Health Endpoints

- `GET /health` — public, returns `{status, version, uptime_seconds}`
- `GET /health/detailed` — admin-required, returns DB ping result (Phase 6 expands with ChirpStack/MQTT/disk/last-uplink-age)

## Configuration

Config is loaded from `/etc/shifter/config.yaml` (override via `SHIFTER_CONFIG_FILE`), with `SHIFTER_*` env vars overriding individual keys. Secrets are file-mounted via `_FILE`-suffixed env vars (`SHIFTER_DB_PASSWORD_FILE`, `SHIFTER_CHIRPSTACK_API_TOKEN_FILE`, `SHIFTER_SESSION_KEY_FILE`, `SHIFTER_MQTT_PASSWORD_FILE`). See [`config/config.example.yaml`](config/config.example.yaml).

`.env` is forbidden for secrets; only Compose `secrets:` (file-backed) is supported.

## Recovery

See [`docs/operator-runbook.md`](docs/operator-runbook.md) for:

- Recovering from a dirty migration
- Resetting a locked-out admin password
- Updating ChirpStack credentials post-install (Settings → ChirpStack connection → Edit)

## License

TBD — single-customer commercial deploys for now.
