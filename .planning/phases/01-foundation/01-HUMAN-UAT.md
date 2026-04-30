---
status: partial
phase: 01-foundation
source: [01-VERIFICATION.md]
started: 2026-04-30T16:02:33Z
updated: 2026-04-30T16:02:33Z
---

## Current Test

[awaiting human testing]

## Tests

### 1. Bundled compose live install
expected: Run `./install/bundled/install.sh shifter.example.com` on a host with a working Docker daemon. Script generates secrets, builds the shifter:0.1.0 image via the multi-stage Dockerfile, brings up postgres/mosquitto/redis/chirpstack/chirpstack-gateway-bridge/chirpstack-rest-api/shifter/caddy via compose/bundled.yml, polls https://localhost/health (Caddy → shifter:8080) until 200, and prints "Visit https://shifter.example.com/install".
result: [pending]

### 2. External compose live install
expected: Run `./install/external/install.sh` after populating install/external/.env (SHIFTER_DOMAIN, SHIFTER_CHIRPSTACK_GRPC_URL, SHIFTER_MQTT_URL) and secrets/chirpstack_api_token.txt. Script validates env, refuses to fabricate the ChirpStack API token, builds image, starts postgres/shifter/caddy via compose/external.yml, polls https://localhost/health until 200.
result: [pending]

### 3. Wizard end-to-end against live ChirpStack v4
expected: After bundled install, visit https://<domain>/install; complete all 5 wizard steps (admin user → ChirpStack mode + creds against a real v4 server → AS923-2 region → install identity → review/finish). Wizard advances through all 5 steps; Step 2 calls real ChirpStack v4 via gRPC and ProbeVersion succeeds; finish atomically inserts admin row + install_identity + chirpstack_connection in one Serializable txn; redirected to /login; signing in as the new admin reaches /settings.
result: [pending]

### 4. ChirpStack v3 refusal banner
expected: Step 2 against a v3 ChirpStack server returns 422 v3_detected; UI renders the destructive Alert with copy "Shifter doesn't support ChirpStack v3".
result: [pending]

### 5. Login rate-limit at 6th failed attempt
expected: 5 failed POST /api/auth/login → 401 bad_credentials; 6th → 429 rate_limited with Retry-After header.
result: [pending]

### 6. Modal-first dialog convention
expected: Every CRUD action in Phase 1 (admin password change, settings ChirpStack edit) opens a Dialog (or Sheet on mobile via ResponsiveDialog) — no full-page edit forms. Visual confirmation that the modal-first feel is correct.
result: [pending]

## Summary

total: 6
passed: 0
issues: 0
pending: 6
skipped: 0
blocked: 0

## Gaps
