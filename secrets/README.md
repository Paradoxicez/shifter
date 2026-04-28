# Secrets

This directory holds file-mounted secrets per **D-06** (no `.env` for secrets).
The install kit (`install/bundled/install.sh`) pre-populates these files. Do
not commit real secret values — only the `.gitkeep` placeholder is tracked.

## Required files

| File                          | Purpose                                                                |
| ----------------------------- | ---------------------------------------------------------------------- |
| `postgres_password.txt`       | Postgres + TimescaleDB password (`POSTGRES_PASSWORD_FILE`)             |
| `chirpstack_api_token.txt`    | ChirpStack v4 API token; set after first ChirpStack boot              |
| `session_signing_key.txt`     | 32+ byte random; SCS session HMAC key (Plan 08, T-04-03 hard-required) |
| `mqtt_password.txt`           | Optional; only used when Mosquitto is configured with auth             |

## Generation

```bash
openssl rand -base64 24 > secrets/postgres_password.txt
openssl rand -hex 32     > secrets/session_signing_key.txt
echo "set-after-chirpstack-boots" > secrets/chirpstack_api_token.txt
echo ""                  > secrets/mqtt_password.txt
chmod 0600 secrets/*.txt
```

## Permissions

```bash
chmod 0600 secrets/*.txt
```

The Compose `secrets:` block remounts these files into the container at
`/run/secrets/<name>` with mode `0400` owned by the container user. Shifter
reads them via `SHIFTER_<NAME>_FILE` env vars (Plan 04 `ReadSecret` /
`ReadSecretOrEmpty`); the raw secret never enters `config.yaml` or any
env-var.

## CRLF caveat (PITFALL #8)

Shifter's `ReadSecret` strips trailing `\r\n` so a Windows-edited file does
not silently corrupt the value. Still, prefer Unix line-endings to avoid
operator confusion when copy-pasting.
