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
