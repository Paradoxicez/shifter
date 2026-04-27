---
phase: 01-foundation
plan: 24
type: execute
wave: 16
depends_on: [05, 18, 20, 21, 22, 23]
files_modified:
  - README.md
  - docs/install.md
  - docs/operator-runbook.md
  - .planning/REQUIREMENTS.md
autonomous: true
requirements:
  - INST-06
must_haves:
  truths:
    - "README.md has Quick Start with `just bootstrap && just dev` for development"
    - "README.md has Production Install section pointing to install/bundled/install.sh and install/external/install.sh"
    - "README.md has compose-flavor table (bundled vs external)"
    - "README.md links to docs/install.md for full operator install guide"
    - "docs/install.md walks through both flavors end-to-end"
    - "docs/operator-runbook.md documents the recovery commands: shifter migrate force, shifter create-admin --reset, dirty-state recovery"
    - "REQUIREMENTS.md INST-06 wording updated to reflect D-19 split: /health public, /health/detailed admin"
  artifacts:
    - path: "README.md"
      provides: "Project overview, quick start, install pointers"
      contains: "Quick Start"
    - path: "docs/install.md"
      provides: "Full bundled + external install walkthrough"
      contains: "bundled"
    - path: "docs/operator-runbook.md"
      provides: "Recovery procedures (dirty migrations, locked-out admin)"
      contains: "shifter migrate force"
  key_links:
    - from: "README.md"
      to: "docs/install.md, docs/operator-runbook.md, install/bundled/install.sh, install/external/install.sh"
      via: "links in Quick Start + Production Install sections"
      pattern: "docs/install"
---

<objective>
Write the operator-facing README + install documentation + recovery runbook + update REQUIREMENTS.md INST-06 wording per D-19 reframing. This plan closes Phase 1: every artifact previous plans created has a documentation entry pointing operators at it.

Purpose: Plan 18's CLI surface, Plans 20/21 compose flavors, Plan 22's TLS modes, and Plans 09/15 wizard flow all need operator-facing docs. Without this plan, Phase 1 ships without a "how to use it" page.

Output: `README.md`, `docs/install.md`, `docs/operator-runbook.md` exist; `REQUIREMENTS.md` INST-06 reflects D-19; the README's `just bootstrap && just dev` flow is verified end-to-end.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/REQUIREMENTS.md
@.planning/phases/01-foundation/01-CONTEXT.md
@01-05-cobra-cli-PLAN.md
@01-20-compose-bundled-PLAN.md
@01-21-compose-external-PLAN.md
@01-22-caddyfile-PLAN.md

<interfaces>
CONTEXT.md D-19 — INST-06 reframing: "/health is public minimal {status,version,uptime_seconds}; /health/detailed (admin-required) returns DB+CS+MQTT+disk+last-uplink-age. INST-06 wording must be updated during Phase 1 implementation to reflect this split."

REQUIREMENTS.md current INST-06 wording (line 26):
> System exposes a `/health` endpoint reporting database, ChirpStack, MQTT, disk, last-uplink-age, and the running Shifter version

New wording (per D-19): split into `/health` (public summary) + `/health/detailed` (admin, full payload — Phase 6 expands).
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 1: README.md operator-facing overview</name>
  <files>README.md</files>
  <read_first>
    - .planning/PROJECT.md (vision, constraints — to set tone)
    - .planning/phases/01-foundation/01-CONTEXT.md (whole)
    - 01-01-repo-scaffold-PLAN.md (Justfile recipe set)
    - 01-20-compose-bundled-PLAN.md, 01-21-compose-external-PLAN.md (install scripts)
  </read_first>
  <action>
1. Replace `README.md` (Plan 01 created a stub; this is the canonical version):
   ```markdown
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
   ```
  </action>
  <verify>
    <automated>test -f README.md && grep -q 'Quick Start' README.md && grep -q 'just bootstrap' README.md && grep -q 'install/bundled/install.sh' README.md && grep -q 'install/external/install.sh' README.md && grep -q 'shifter migrate force' README.md</automated>
  </verify>
  <acceptance_criteria>
    - File `README.md` exists at repo root
    - File contains `## Quick Start` heading
    - File mentions `just bootstrap` and `just dev`
    - File mentions both `install/bundled/install.sh` and `install/external/install.sh`
    - File contains the bundled vs external comparison table
    - File mentions all 6 CLI subcommands (serve, migrate, version, create-admin, config-check, healthcheck)
    - File documents `/health` and `/health/detailed` per D-19
    - File mentions Compose secrets and `.env` is forbidden for secrets (D-06)
    - File links to `docs/install.md` and `docs/operator-runbook.md`
  </acceptance_criteria>
  <done>
    README operator-grade. Production users have one entry point.
  </done>
</task>

<task type="auto">
  <name>Task 2: docs/install.md + docs/operator-runbook.md</name>
  <files>docs/install.md, docs/operator-runbook.md</files>
  <read_first>
    - 01-20-compose-bundled-PLAN.md, 01-21-compose-external-PLAN.md (install.sh contents)
    - 01-22-caddyfile-PLAN.md (TLS modes)
    - 01-05-cobra-cli-PLAN.md (CLI surface)
    - 01-09-login-ratelimit-PLAN.md (create-admin)
  </read_first>
  <action>
1. Create `docs/install.md`:
   ```markdown
   # Shifter — Install Guide

   This guide walks through both compose flavors end-to-end.

   ## Prerequisites

   - Linux host with Docker + Docker Compose v2 (`docker compose version` ≥ 2.20).
   - DNS pointing to the host (for `acme` TLS mode) — or LAN-only access (`internal` TLS mode).
   - Open ports 80 (HTTP-01 ACME challenge + redirect) and 443 (HTTPS).
   - For external flavor: an existing ChirpStack v4 server reachable from the host + an existing MQTT broker.

   ## Choosing a flavor

   | Flavor | When to choose |
   |--------|---------------|
   | **Bundled** | Customer has no LoRaWAN stack; we provision everything |
   | **External** | Customer already runs ChirpStack v4 (and we won't touch it) |

   ## TLS modes

   Set `SHIFTER_TLS_MODE` to one of:

   | Mode | What happens |
   |------|-------------|
   | `acme` (default) | Caddy fetches a real certificate from Let's Encrypt for `SHIFTER_DOMAIN`. Set `SHIFTER_TLS_EMAIL` for ACME contact. ZeroSSL is the documented fallback. |
   | `byo` | Operator mounts `secrets/cert.pem` and `secrets/key.pem`. Used for internal CAs and air-gapped LANs. |
   | `internal` | Caddy issues a self-signed cert via its local CA. One-time browser warning. LAN-only. |

   Plain HTTP is **not** allowed in production (D-22).

   ## Bundled flavor — step-by-step

   1. Clone or extract the Shifter repo on the host.
   2. (Optional) Pre-populate `secrets/chirpstack_api_token.txt` if you already know what token to use; otherwise the wizard captures it.
   3. Run:
      ```sh
      ./install/bundled/install.sh shifter.example.com
      ```
   4. Wait for `==> Shifter is up: visit https://shifter.example.com/install` to print.
   5. Visit `https://shifter.example.com/install` — the 5-step wizard captures admin user, ChirpStack mode + creds, region (Thailand AS923-2 default), install identity, and finishes by writing the canonical tables.

   ## External flavor — step-by-step

   1. `cp install/external/.env.example install/external/.env`
   2. Edit `install/external/.env`:
      - `SHIFTER_DOMAIN` — the hostname operators visit
      - `SHIFTER_CHIRPSTACK_GRPC_URL` — e.g. `chirpstack.acme.local:8080`
      - `SHIFTER_MQTT_URL` — e.g. `tcp://mqtt.acme.local:1883`
      - `SHIFTER_TLS_MODE` (acme / byo / internal)
      - `SHIFTER_TLS_EMAIL` (for acme)
   3. Place the ChirpStack API token in `secrets/chirpstack_api_token.txt`:
      ```sh
      printf 'YOUR_CHIRPSTACK_API_TOKEN' > secrets/chirpstack_api_token.txt
      chmod 0600 secrets/chirpstack_api_token.txt
      ```
   4. (If MQTT broker has auth) place password in `secrets/mqtt_password.txt`.
   5. Run:
      ```sh
      ./install/external/install.sh
      ```
   6. Visit the wizard at `https://<your domain>/install`.

   ## What the wizard captures

   1. **Admin user** — email + name + password. Argon2id-hashed before persistence.
   2. **ChirpStack** — mode (bundled/external), gRPC URL, API token, MQTT URL, optional MQTT user/password. The wizard probes the gRPC endpoint and **refuses ChirpStack v3** (INST-05).
   3. **Region** — AS923-2 (Thailand) is the default; pick a different region as needed.
   4. **Install identity** — display name, address, timezone, units (metric/imperial). Used in topbar and reports.
   5. **Review & finish** — atomic commit of all four steps in one Postgres transaction.

   ## After install

   - Visit `https://<your domain>/login` — sign in as the admin you just created.
   - Settings → ChirpStack connection → Test connection: confirms gRPC + MQTT are reachable.
   - The sidebar in Phase 1 shows only **Settings**. Provisioning, dashboards, reports etc. arrive in Phases 2-5.

   ## Updating credentials post-install (SETT-03)

   Settings → ChirpStack connection → **Edit connection**. Saving re-runs the v3 probe automatically (no risk of saving a broken config).

   ## Health probes for monitoring

   - `GET /health` (public): `{status, version, uptime_seconds}`. Use this for Docker / load-balancer probes.
   - `GET /health/detailed` (admin-required): currently `{status, checks: { db: bool }, version, uptime_seconds}`. Phase 6 expands with ChirpStack/MQTT/disk/last-uplink-age.
   ```

2. Create `docs/operator-runbook.md`:
   ```markdown
   # Shifter — Operator Runbook

   Recovery procedures for the cases that come up in production.

   ## I'm locked out — no admin can log in

   Use the recovery escape hatch (D-14):

   ```sh
   docker compose -f compose/bundled.yml exec shifter \
     /usr/local/bin/shifter create-admin --reset \
       --email admin@example.com --password 'NEW_STRONG_PASSWORD'
   ```

   - `--reset` updates an existing admin's password
   - Without `--reset`, the command refuses to overwrite an existing admin

   The new password is Argon2id-hashed before write. The admin user's `must_change_password` flag is **not** set (D-09).

   ## Database is dirty after a partial migration

   golang-migrate marks the schema "dirty" if a migration aborts mid-run. `shifter serve` refuses to start in this state.

   1. Identify the last clean version:
      ```sh
      docker compose -f compose/bundled.yml exec postgres \
        psql -U shifter -d shifter -c 'SELECT * FROM schema_migrations'
      ```
   2. Force the schema clean:
      ```sh
      docker compose -f compose/bundled.yml exec shifter \
        /usr/local/bin/shifter migrate force <PREVIOUS_CLEAN_VERSION>
      ```
   3. Restart Shifter — it will re-run the failed migration. If the underlying issue (e.g., a constraint violation) wasn't fixed, the migration will dirty again and you'll need to investigate.

   **Don't bypass dirty state lightly.** It typically signals real schema corruption that needs a code fix, not a force.

   ## ChirpStack v3 detected at boot

   `shifter serve` prints:
   ```
   error: INST-05: refusing to start — ChirpStack v3 detected at <url>
   ```

   This is intentional. Shifter requires ChirpStack v4. Upgrade ChirpStack before restarting Shifter.

   ## /health/detailed shows degraded

   Currently only the DB check is implemented (Phase 1 minimum per D-19).

   - `checks.db: false` → Postgres is unreachable. Inspect: `docker compose logs postgres`.

   Phase 6 will add ChirpStack/MQTT/disk/last-uplink-age checks.

   ## Updating ChirpStack credentials

   Settings → ChirpStack connection → **Edit connection** → Save and test. Save is rejected with a clear error if gRPC or MQTT becomes unreachable.

   For non-UI updates (e.g., the operator UI itself is down), edit the `chirpstack_connection` row directly:
   ```sql
   UPDATE chirpstack_connection SET grpc_url = $1, mqtt_url = $2 WHERE id = 1;
   ```
   …and update `secrets/chirpstack_api_token.txt`. Restart Shifter to reload secrets.

   ## Logs

   All container logs use the `json-file` driver with `max-size: 10m` + `max-file: 3` (PITFALL §15 mitigation; OPS-05 forward-look).

   View structured logs:
   ```sh
   docker compose -f compose/bundled.yml logs -f shifter
   ```

   ## Backups (Phase 6)

   Phase 1 does not ship backup automation. Documented in [Phase 6 plan](../.planning/ROADMAP.md). For now, use `pg_dump` on the postgres container.
   ```
  </action>
  <verify>
    <automated>test -f docs/install.md && test -f docs/operator-runbook.md && grep -q 'Bundled flavor' docs/install.md && grep -q 'Bundled' docs/install.md && grep -q 'External' docs/install.md && grep -q 'shifter migrate force' docs/operator-runbook.md && grep -q 'shifter create-admin --reset' docs/operator-runbook.md    <automated>test -f docs/install.md && test -f docs/operator-runbook.md && grep -q 'Bundled flavor' docs/install.md && grep -q 'Bundled' docs/install.md && grep -q 'External' docs/install.md && grep -q 'shifter migrate force' docs/operator-runbook.md && grep -q 'shifter create-admin --reset' docs/operator-runbook.md</automated>
  </verify>
  <acceptance_criteria>
    - File `docs/install.md` exists and contains a `## Bundled flavor` section
    - File `docs/install.md` contains a `## External flavor` section
    - File `docs/install.md` documents all 3 TLS modes (`acme`, `byo`, `internal`)
    - File `docs/install.md` documents the wizard's 5 steps including AS923-2 default and v3 refusal
    - File `docs/install.md` documents both `/health` and `/health/detailed` per D-19
    - File `docs/operator-runbook.md` exists and contains `shifter migrate force <N>` recovery procedure
    - File `docs/operator-runbook.md` contains `shifter create-admin --reset` lockout-recovery procedure
    - File `docs/operator-runbook.md` documents the v3 boot refusal error
  </acceptance_criteria>
  <done>
    Operator install + runbook docs ready. Combined with README.md, this is the complete operator-facing surface for Phase 1.
  </done>
</task>

<task type="auto">
  <name>Task 3: Update REQUIREMENTS.md INST-06 wording per D-19 reframing</name>
  <files>.planning/REQUIREMENTS.md</files>
  <read_first>
    - .planning/phases/01-foundation/01-CONTEXT.md (D-19 — REQUIREMENTS.md update directive)
    - .planning/REQUIREMENTS.md (current INST-06 wording at line 26)
  </read_first>
  <action>
1. Read the current INST-06 entry in `.planning/REQUIREMENTS.md` (around line 26):
   ```
   - [ ] **INST-06**: System exposes a `/health` endpoint reporting database, ChirpStack, MQTT, disk, last-uplink-age, and the running Shifter version
   ```

2. Replace it with the D-19-reframed wording:
   ```
   - [ ] **INST-06**: System exposes `/health` (public, no auth — `{status, version, uptime_seconds}` minimum) and `/health/detailed` (admin-required — DB connection state in Phase 1; Phase 6 expands with ChirpStack, MQTT, disk, and last-uplink-age). Reframed by Phase 1 D-18/D-19; the original single-endpoint-with-everything wording is replaced by this split.
   ```

3. Confirm the change by reading the file back at that line range.
  </action>
  <verify>
    <automated>grep -A1 'INST-06' .planning/REQUIREMENTS.md | head -3 | grep -q '/health/detailed'</automated>
  </verify>
  <acceptance_criteria>
    - File `.planning/REQUIREMENTS.md` line for INST-06 contains both `/health` AND `/health/detailed` (split per D-19)
    - The new wording references "admin-required" for `/health/detailed`
    - The new wording references "Phase 6" for the full payload expansion
    - `grep -c 'INST-06' .planning/REQUIREMENTS.md` returns at least 2 (definition line + traceability table)
  </acceptance_criteria>
  <done>
    REQUIREMENTS.md aligned with D-19. The phase-1 documentation surface is now consistent end-to-end.
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| operator → docs | Trusted reading; no security boundary crossed |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-24-01 | Information Disclosure | docs publish secrets handling steps | mitigate | Docs reference `secrets/` files but never include real values; install scripts generate dummy ones for smoke and require operator-provided real secrets for prod. ASVS V8. |
| T-24-02 | Tampering | runbook recommends `migrate force` carelessly | mitigate | Runbook explicitly says "Don't bypass dirty state lightly" and recommends investigating root cause first. |
</threat_model>

<verification>
- README.md operator-facing with quick start + install table + CLI table
- docs/install.md: bundled + external walkthroughs + TLS modes + post-install
- docs/operator-runbook.md: dirty-state recovery, create-admin --reset, v3 refusal
- REQUIREMENTS.md INST-06 reflects D-19 split
</verification>

<success_criteria>
- INST-06 wording aligned with D-19 reframing
- Operators have a single README.md entry point
- Both compose flavors documented end-to-end
- All recovery commands documented
- Phase 1 ships with complete operator-facing artifacts
</success_criteria>

<output>
After completion, create `.planning/phases/01-foundation/01-24-SUMMARY.md` documenting:
- Doc structure (README + docs/install + docs/runbook)
- INST-06 wording change
- Where to add docs in Phase 2+ (the canonical doc/ tree pattern)
</output>
