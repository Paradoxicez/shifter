---
phase: 01-foundation
plan: 22
type: execute
wave: 15
depends_on: [20]
files_modified:
  - Caddyfile
  - install/bundled/install.sh
  - install/external/install.sh
autonomous: true
requirements:
  - OPS-01
must_haves:
  truths:
    - "Caddyfile uses {$VAR} env interpolation (D-21)"
    - "Caddyfile global block has email + tls block injected via $CADDY_GLOBAL_TLS_BLOCK"
    - "Site block has security headers: HSTS (max-age=31536000; includeSubDomains), X-Content-Type-Options nosniff, Referrer-Policy strict-origin-when-cross-origin, X-Frame-Options DENY, CSP locked"
    - "TLS block injected via $CADDY_TLS_BLOCK (acme=empty, byo=tls cert key, internal=tls internal)"
    - "/sse path has flush_interval -1 + read_buffer 0 + response_header_timeout 0 (PITFALL #7 prevention)"
    - "All other traffic forwards to shifter:8080"
    - "install.sh sets CADDY_TLS_BLOCK based on SHIFTER_TLS_MODE"
  artifacts:
    - path: "Caddyfile"
      provides: "Caddy 2 reverse proxy with env-driven TLS modes (D-20, D-21, D-22)"
      contains: "tls"
  key_links:
    - from: "Caddyfile"
      to: "shifter:8080"
      via: "reverse_proxy with SSE-aware /sse path matcher"
      pattern: "reverse_proxy"
---

<objective>
Implement the Caddyfile per RESEARCH §Pattern 11: env-driven TLS modes (`acme` | `byo` | `internal`), security headers (HSTS, CSP, frame deny), SSE-aware proxy block (`flush_interval -1`), and reverse proxy to `shifter:8080`. Both compose flavors mount this single Caddyfile.

Purpose: D-20 (Caddy bundled in both flavors), D-21 (3 TLS modes), D-22 (no plain HTTP in production). PITFALL #7 (SSE buffering) addressed even though Phase 1 has no SSE endpoint yet.

Output: A Caddyfile at the repo root that the bundled+external compose files both mount; smoke tests pass through it.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/01-foundation/01-RESEARCH.md
@.planning/phases/01-foundation/01-CONTEXT.md
@01-20-compose-bundled-PLAN.md
@01-21-compose-external-PLAN.md

<interfaces>
RESEARCH §Pattern 11 (lines 851-923) — verbatim Caddyfile.
RESEARCH §Pattern 11 install kit table (lines 916-922) — three preset env values for `CADDY_TLS_BLOCK`.

Both compose files mount `Caddyfile` from repo root at `/etc/caddy/Caddyfile:ro`. install scripts (Plans 20, 21) export `CADDY_TLS_BLOCK` based on `SHIFTER_TLS_MODE`.

Phase 1 has no `/sse` endpoint (Phase 4 introduces SSE). The SSE block is included now to prevent PITFALL #7 retrofitting.
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 1: Caddyfile with env-driven TLS + SSE-aware proxy + security headers</name>
  <files>Caddyfile, install/bundled/install.sh, install/external/install.sh</files>
  <read_first>
    - .planning/phases/01-foundation/01-RESEARCH.md §"Pattern 11: Caddyfile with config-driven TLS" (lines 851-923)
    - .planning/phases/01-foundation/01-RESEARCH.md §"Pitfall 7: Vite proxy buffers SSE responses" (lines 1349-1353)
    - .planning/phases/01-foundation/01-CONTEXT.md (D-20, D-21, D-22)
  </read_first>
  <action>
1. Create `Caddyfile` at repo root — VERBATIM from RESEARCH §Pattern 11 with explicit comments:
   ```caddy
   # Shifter — Caddy 2 reverse proxy (D-20).
   #
   # TLS modes (D-21): acme | byo | internal
   #   acme    — Let's Encrypt for SHIFTER_DOMAIN; ZeroSSL fallback when LE rate-limits.
   #   byo     — operator mounts /etc/caddy/cert.pem + /etc/caddy/key.pem
   #   internal— Caddy local CA (self-signed); LAN-only deployments
   #
   # The CADDY_TLS_BLOCK env var is templated by the install script before `caddy run`:
   #   acme:     CADDY_TLS_BLOCK=""
   #   byo:      CADDY_TLS_BLOCK="tls /etc/caddy/cert.pem /etc/caddy/key.pem"
   #   internal: CADDY_TLS_BLOCK="tls internal"
   #
   # PITFALL #7 prevention: /sse paths are forwarded with flush_interval -1
   # so SSE responses stream to the browser without buffering.
   # (Phase 1 has no /sse endpoint — block included now for Phase 4 readiness.)

   {
       email {$SHIFTER_TLS_EMAIL}
       {$CADDY_GLOBAL_TLS_BLOCK}
   }

   {$SHIFTER_DOMAIN:localhost} {
       encode zstd gzip

       # Security headers (D-22 + ASVS V9/V14)
       header {
           Strict-Transport-Security "max-age=31536000; includeSubDomains"
           X-Content-Type-Options "nosniff"
           Referrer-Policy "strict-origin-when-cross-origin"
           X-Frame-Options "DENY"
           # CSP: SPA assets only; no inline scripts (Vite production build emits none).
           Content-Security-Policy "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; connect-src 'self'; font-src 'self'"
           # Disable referrer leak from server side
           -Server
       }

       {$CADDY_TLS_BLOCK}

       # SSE — must NOT be buffered (PITFALL #7).
       @sse path /sse /sse/*
       handle @sse {
           reverse_proxy shifter:8080 {
               flush_interval -1
               transport http {
                   read_buffer 0
                   response_header_timeout 0
               }
           }
       }

       # Health endpoint (public, no auth) — explicit handler to keep it fast.
       handle /health {
           reverse_proxy shifter:8080
       }

       # /api/*, /assets/*, /, etc.
       reverse_proxy shifter:8080
   }
   ```

2. Update `install/bundled/install.sh` to set `CADDY_TLS_BLOCK` based on `SHIFTER_TLS_MODE`. Insert before `(cd compose && docker compose -f bundled.yml up -d)`:
   ```bash
   case "${SHIFTER_TLS_MODE:-internal}" in
     acme)     export CADDY_TLS_BLOCK="" ;;
     byo)      export CADDY_TLS_BLOCK="tls /etc/caddy/cert.pem /etc/caddy/key.pem" ;;
     internal) export CADDY_TLS_BLOCK="tls internal" ;;
     *)        echo "ERROR: SHIFTER_TLS_MODE invalid"; exit 2 ;;
   esac
   ```
   And accept `SHIFTER_DOMAIN` + `SHIFTER_TLS_MODE` from arg or env (default `localhost` + `internal`).

3. The external installer (Plan 21) already has the same case statement; verify it's present.

4. Smoke-test: re-run `just compose-smoke-bundled` and `just compose-smoke-external` with the Caddyfile mounted. They should still exit 0 because both polls go to `http://localhost:8080/health` directly (bypassing Caddy via the shifter container's exposed port… BUT the Plan 20 compose doesn't expose 8080 — Caddy fronts on 80/443).

   **Adjustment**: For smoke testing, expose port 8080 ONLY in compose files OR poll through Caddy. The smoke recipe currently polls `localhost:8080`. Since Caddy listens on 80/443 and the bundled stack exposes those, update the smoke recipe to poll Caddy instead (with `--insecure` since `tls internal`):
   ```bash
   # Replace:
   #   curl -fs http://localhost:8080/health
   # With:
   #   curl -fsk https://localhost/health
   ```
   Update the Justfile recipes from Plans 20+21 accordingly. The shifter container itself exposes port 8080 internally on the network only; the compose smoke now exercises the full path through Caddy.
  </action>
  <verify>
    <automated>just _compose-build-image && just compose-smoke-bundled && just compose-smoke-external</automated>
  </verify>
  <acceptance_criteria>
    - File `Caddyfile` at repo root exists
    - File contains exactly the security headers: `Strict-Transport-Security`, `X-Content-Type-Options`, `Referrer-Policy`, `X-Frame-Options`, `Content-Security-Policy` (5 headers — grep verifiable)
    - File contains `{$CADDY_TLS_BLOCK}` directive (D-21 env-templating)
    - File contains `@sse path /sse /sse/*` matcher with `flush_interval -1` (PITFALL #7)
    - File contains `reverse_proxy shifter:8080` (forwards to the shifter container)
    - File `install/bundled/install.sh` contains a `case "${SHIFTER_TLS_MODE:-internal}"` block setting `CADDY_TLS_BLOCK`
    - File `install/external/install.sh` already contains the same case block (from Plan 21)
    - Command `just compose-smoke-bundled` exits 0 with HTTPS-through-Caddy `/health` poll
    - Command `just compose-smoke-external` exits 0 with HTTPS-through-Caddy `/health` poll
  </acceptance_criteria>
  <done>
    Caddy reverse proxy active in both flavors with proper TLS, security headers, and SSE-readiness. The Phase 1 stack is now fully operational: install → login → settings → test connection.
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| public internet → Caddy | TLS termination; security headers applied here |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-22-01 | Tampering (TLS downgrade) | mixed-content / plain HTTP | mitigate | D-22: production rejects `tls.mode: none`; Caddy enforces redirect from :80 → :443 by default. HSTS header. ASVS V9. |
| T-22-02 | Tampering (clickjacking) | embedded in iframe | mitigate | `X-Frame-Options: DENY`. ASVS V14. |
| T-22-03 | Tampering (XSS) | inline script execution | mitigate | CSP `default-src 'self'; script-src 'self'` (no `'unsafe-inline'` for scripts). ASVS V14. |
| T-22-04 | Information Disclosure | MIME sniffing leaks content type | mitigate | `X-Content-Type-Options: nosniff`. ASVS V14. |
| T-22-05 | Information Disclosure | Server header reveals Caddy + version | mitigate | `-Server` header strip in Caddyfile. ASVS V14. |
| T-22-06 | Tampering (SSE buffering) | proxy buffers stream → broken realtime | mitigate | `flush_interval -1` on /sse handle (PITFALL #7). |
| T-22-07 | Spoofing | self-signed cert in `internal` mode | accept | Documented one-time browser warning; LAN-only deployments. ASVS V9. |
</threat_model>

<verification>
- Caddyfile contains 5 security headers + CSP
- 3 TLS modes via $CADDY_TLS_BLOCK env templating
- /sse handler with flush_interval -1
- Both install scripts set CADDY_TLS_BLOCK correctly
- Both compose smoke tests still pass
</verification>

<success_criteria>
- D-20 enforced (Caddy bundled in both flavors)
- D-21 enforced (3 TLS modes via env)
- D-22 enforced (HSTS + production HTTPS only)
- PITFALL #7 prevented (SSE flush_interval ready for Phase 4)
- Smoke tests verify the full path through Caddy
</success_criteria>

<output>
After completion, create `.planning/phases/01-foundation/01-22-SUMMARY.md` documenting:
- TLS mode env table
- Security header set
- /sse SSE-aware block
- Caddyfile mount path
</output>
