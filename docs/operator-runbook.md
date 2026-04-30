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
