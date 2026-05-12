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
