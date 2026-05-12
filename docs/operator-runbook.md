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

## Backup & Restore

### Backup (manual)

Bundled mode:

```sh
docker compose -f compose/bundled.yml exec shifter \
  shifter backup --to /var/lib/shifter/backups
```

External mode:

```sh
docker compose -f compose/external.yml exec shifter \
  shifter backup --to /var/lib/shifter/backups
```

Output: `/var/lib/shifter/backups/shifter-backup-<slug>-<YYYYMMDD-HHMM>-<schema>.tar.gz`
plus a `backup_run` row in the Shifter DB and two audit rows (`backup.start` + `backup.complete`).

### Backup (scheduled, bundled mode)

The bundled compose ships a `backup-cron` sidecar (mcuadros/ofelia:v0.3.22) that runs
`shifter backup` daily at 02:00 install timezone. To change the schedule, edit
`compose/bundled.yml` and update the `ofelia.job-exec.shifter-backup.schedule` label,
then run `docker compose up -d backup-cron`.

### Backup (scheduled, external mode)

External mode does **not** include the cron sidecar. Set up your preferred scheduler
(system cron, k8s CronJob, etc.) to run:

```sh
docker compose -f compose/external.yml exec -T shifter \
  shifter backup --to /var/lib/shifter/backups --trigger=cron
```

### Restore (manual — Shifter must be stopped first)

**Shifter must be stopped before running restore.** The restore command acquires a
Postgres advisory lock and refuses to proceed if Shifter is still serving.

```sh
# 1. Stop Shifter.
docker compose -f compose/bundled.yml stop shifter

# 2. Restore from tarball.
docker compose -f compose/bundled.yml run --rm shifter \
  shifter restore --from /var/lib/shifter/backups/<file>.tar.gz

# 3. Start Shifter (will apply any pending migrations on boot).
docker compose -f compose/bundled.yml start shifter
```

For external-mode installs, substitute `compose/external.yml`.

**Restore safety properties:**

- Refuses to run if the Shifter HTTP server is still up (PG advisory lock check).
- Verifies per-file sha256 sums against `manifest.json` before pg_restore runs.
- Wraps `pg_restore` in `SELECT timescaledb_pre_restore()` and
  `SELECT timescaledb_post_restore()` — required for TimescaleDB CAGG state.
- Never passes `-j`/`--jobs` to `pg_restore` (breaks TimescaleDB catalog ordering).
- Restores the Shifter DB first; in bundled mode, ChirpStack DB second.
- Rsyncs the floor-plans volume from the tarball.
- Writes a `backup.restore` audit row on success.

**Optional chain-of-custody verification:**

The `shifter backup` command prints the outer tarball sha256 at completion. Pass it
to `--expected-sha256` to verify the tarball has not been tampered with:

```sh
docker compose -f compose/bundled.yml run --rm shifter \
  shifter restore \
    --from /var/lib/shifter/backups/<file>.tar.gz \
    --expected-sha256 <sha256-from-backup-output>
```

**Cross-version restore** (e.g., a v0.5.0 backup restored into a v0.6.0 install) is
**not supported in v1**. The manifest's `db_schema_version` field is the forward-compat
hook for v1.1. After any version upgrade, always take a fresh backup before deploying
the new version; then if rollback is needed, restore that latest backup into the
previous version.

### CI gate

`.github/workflows/backup-restore-roundtrip.yml` runs the full round-trip
(seed → backup → drop schema → restore → smoke) on every PR touching
`internal/backup/**` or `internal/db/migrations/**`, and on every push to main.
The workflow fails CI if backup or restore produces invalid output.

## Compose conventions

All Shifter Compose files follow these conventions (enforced by `go test ./internal/compose/...`):

- **Pinned image tags only.** No `:latest` anywhere. Every service has a specific version
  (e.g., `timescale/timescaledb:2.26.0-pg16`, `mcuadros/ofelia:v0.3.22`).
- **json-file logging caps.** Every service block contains `logging: *json-logging`
  (the YAML anchor defined at the top of each compose file). This applies the
  `json-file` driver with `max-size: 10m` + `max-file: 3` to prevent unbounded
  disk usage (OPS-05).
- **Secrets via `secrets:` declarations.** Credentials are read from
  `/run/secrets/<name>` files (mounted by Compose from `../secrets/*.txt`), NOT
  from environment variables. Never use `${SHIFTER_DB_PASSWORD}` in compose files
  — use `SHIFTER_DB_PASSWORD_FILE: /run/secrets/postgres_password` instead (D-06).
- **No exposed internal services.** Postgres, Mosquitto, Redis, ChirpStack do NOT
  bind host ports. Only Caddy (80/443) and the ChirpStack gateway bridge (1700/udp)
  bind externally (T-20-03).
- **Backups volume.** Both compose flavors mount `/var/lib/shifter/backups` via a
  named volume `backups`. Bundled mode adds the `mcuadros/ofelia` cron sidecar
  (read-only Docker socket); external mode operators use their own scheduler.

When editing a compose file, run `go test ./internal/compose/...` to confirm
conventions hold before committing.

## Upgrading Shifter

Operator-driven five-step procedure per Shifter release (no in-app upgrade in v1):

1. **Take a backup** (per the Backup & Restore section above):
   ```sh
   docker compose -f compose/bundled.yml exec shifter \
     shifter backup --to /var/lib/shifter/backups
   ```
   Confirm the tarball is written and the `backup_run` row shows `status='completed'`
   via `GET /api/backup/last` or the Settings → Backup card.

2. **Bump the image tag.** Edit `compose/bundled.yml` (or `compose/external.yml`)
   and change `image: shifter:0.X.Y` to the new release tag. **Never use `:latest`** —
   the compose conventions test will fail and block the PR.

3. **Pull and restart Shifter only.** Other services (Postgres, ChirpStack, etc.)
   stay running during the upgrade:
   ```sh
   docker compose -f compose/bundled.yml pull shifter
   docker compose -f compose/bundled.yml up -d shifter
   ```
   Expected downtime: ≤ 5 seconds. Uplinks queued in MQTT during the restart are
   processed on resume (MQTT broker stays up throughout).

4. **Verify the upgrade.** Check `/health` (public) and `/health/detailed` (admin):
   ```sh
   curl https://shifter.example.com/health
   # expected: {"status":"ok","version":"0.X.Y","uptime_seconds":...}
   ```
   Sign in to the UI and confirm the `/health/detailed` response reports:
   - `alert_workers[*].degraded = false` for every worker
   - `last_backup.age_seconds < backup_warn_threshold_hours * 3600`
   - `checks.db = true`

   If any signal is red, follow the rollback procedure (step 5).

5. **Rollback procedure** (if step 4 fails or any blocker surfaces within 24 h):
   ```sh
   docker compose -f compose/bundled.yml stop shifter
   docker compose -f compose/bundled.yml run --rm shifter \
     shifter restore --from /var/lib/shifter/backups/<pre-upgrade>.tar.gz
   # Edit compose file: revert image tag to the previous version
   docker compose -f compose/bundled.yml up -d shifter
   ```

### Per-release migration notes

Each Shifter release appends migration notes here when schema changes require
operator awareness. Phase 6 (v0.6.0) adds migrations 0037–0048 (audit vocabulary,
alert engine substrate, retention extensions, backup history, backup thresholds,
alert retention prune vocabulary). All are additive — no data migration risk.
