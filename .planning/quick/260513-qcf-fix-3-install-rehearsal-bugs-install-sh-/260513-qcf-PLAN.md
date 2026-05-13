---
phase: 260513-qcf
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - install/bundled/install.sh
  - compose/bundled.yml
  - compose/chirpstack/chirpstack.toml
  - compose/chirpstack/region_as923.toml
  - compose/chirpstack/region_as923_2.toml
  - compose/chirpstack/region_as923_3.toml
  - compose/chirpstack/region_as923_4.toml
  - compose/chirpstack/region_eu868.toml
  - compose/chirpstack/region_us915_0.toml
  - compose/chirpstack/region_au915_0.toml
  - compose/chirpstack/region_in865.toml
  - compose/postgres-init/01-chirpstack.sql
  - internal/install/handlers.go
  - internal/install/state_test.go
autonomous: true
requirements: []
---

<objective>
Fix three install rehearsal regressions discovered during bundled-flavor smoke testing:
1. install.sh tries to mkdir a host path (/var/lib/shifter/backups) that Docker manages via named volumes.
2. ChirpStack restart-loops because its named volume is empty (no config) and no --config flag is passed.
3. GET /api/install/state leaks password_hash inside step1_admin JSONB into the wire response.

Purpose: Unblock rehearsal so the full wizard flow completes cleanly.
Output: Working bundled stack (chirpstack Up/healthy), clean install.sh run, redacted /api/install/state.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@compose/bundled.yml
@install/bundled/install.sh
@internal/install/handlers.go
@internal/install/state_test.go
</context>

<tasks>

<task type="auto">
  <name>Task 1: Remove dead /var/lib/shifter/backups host-path step from install.sh</name>
  <files>install/bundled/install.sh</files>
  <action>
Delete lines 38-40 exactly (the three-line block):

```
echo "==> Preparing backup directory"
mkdir -p /var/lib/shifter/backups
chown 65532:65532 /var/lib/shifter/backups || true   # 65532 = distroless nonroot uid
```

Rationale: compose/bundled.yml lines 159-162 mount `backups:/var/lib/shifter/backups` as a Docker named volume. Docker creates and owns the host side. The `backup-cron` service only mounts `/var/run/docker.sock` — it does NOT mount the backup dir. The host mkdir is therefore a dead step that fails in rootless environments and misleads operators.

Do NOT touch any other section of install.sh.

After deletion, the script flows directly from the `chmod 0600 secrets/*.txt` line to the `echo "==> Building shifter:0.1.0 image"` line.
  </action>
  <verify>
Run `bash -n install/bundled/install.sh` — must exit 0 with no output.
Grep confirms the dead lines are gone: `grep -n "var/lib/shifter/backups" install/bundled/install.sh` must return empty.
  </verify>
  <done>install.sh passes bash -n; no reference to /var/lib/shifter/backups remains in the file; diff shows exactly 3 lines removed.</done>
</task>

<task type="auto">
  <name>Task 2: ChirpStack config bootstrap — host-bind config dir + postgres init DB</name>
  <files>
    compose/bundled.yml,
    compose/chirpstack/chirpstack.toml,
    compose/chirpstack/region_as923.toml,
    compose/chirpstack/region_as923_2.toml,
    compose/chirpstack/region_as923_3.toml,
    compose/chirpstack/region_as923_4.toml,
    compose/chirpstack/region_eu868.toml,
    compose/chirpstack/region_us915_0.toml,
    compose/chirpstack/region_au915_0.toml,
    compose/chirpstack/region_in865.toml,
    compose/postgres-init/01-chirpstack.sql
  </files>
  <action>
This task has three sub-steps that must land in ONE commit.

## 2a — Update compose/bundled.yml chirpstack service

Replace the chirpstack service block (lines 83-99) with:

```yaml
  chirpstack:
    image: chirpstack/chirpstack:4.10
    restart: unless-stopped
    depends_on:
      postgres:
        condition: service_healthy
      mosquitto:
        condition: service_started
      redis:
        condition: service_started
    command: ["-c", "/etc/chirpstack"]
    volumes:
      - ./chirpstack:/etc/chirpstack:ro
    logging: *json-logging
    networks: [shifter]
```

Changes from current:
- Added `command: ["-c", "/etc/chirpstack"]` — tells ChirpStack where to find its TOML config dir.
- Changed volume from named `chirpstack_config:/etc/chirpstack` to host-bind `./chirpstack:/etc/chirpstack:ro` — ships our config files (same pattern as `./mosquitto.conf` for mosquitto).
- Removed `secrets: [postgres_password]` and `environment: POSTGRESQL__DSN_FILE: ...` — ChirpStack v4 uses TOML config not env-var DSN. The DSN is embedded in chirpstack.toml directly (see 2b).

Also in the top-level `volumes:` block (lines 208-218), remove the `chirpstack_config:` entry. The remaining entries stay unchanged.

Also update the postgres service to mount an init script. Add a `volumes:` entry to the postgres service:

```yaml
    volumes:
      - postgres_data:/var/lib/postgresql/data
      - ./postgres-init:/docker-entrypoint-initdb.d:ro
```

(The `postgres_data` line already exists — add the second line below it.)

## 2b — Create compose/chirpstack/ directory with all config files

Create `compose/chirpstack/chirpstack.toml`:

```toml
# Shifter bundled flavor — ChirpStack v4 main config.
# All $VAR references are expanded by ChirpStack's built-in env substitution
# EXCEPT the DSN — we embed the shifter user credentials directly since
# Postgres in bundled mode uses POSTGRES_USER=shifter for both databases.

[logging]
  level="info"

[postgresql]
  dsn="postgres://shifter:${POSTGRESQL_PASSWORD}@postgres:5432/chirpstack?sslmode=disable"
  max_open_connections=10
  min_idle_connections=0

[redis]
  servers=[
    "redis://redis:6379/",
  ]
  tls_enabled=false
  cluster=false

[network]
  net_id="000000"
  enabled_regions=[
    "as923",
    "as923_2",
    "as923_3",
    "as923_4",
    "au915_0",
    "eu868",
    "in865",
    "us915_0",
  ]

[api]
  bind="0.0.0.0:8080"
  # Replace with `openssl rand -base64 32` output on first install.
  # ChirpStack will refuse to start with this placeholder in production;
  # for bundled smoke tests the placeholder is acceptable.
  secret="shifter-bundled-replace-me-openssl-rand-base64-32"

[integration]
  enabled=["mqtt"]

  [integration.mqtt]
    server="tcp://mosquitto:1883/"
    json=true
```

NOTE on DSN: ChirpStack v4 supports `${ENV_VAR}` substitution in TOML. However, the `postgres_password` secret is only mounted as a FILE (`/run/secrets/postgres_password`), not as an environment variable, and ChirpStack has no `_FILE` env support. Since the bundled compose uses `POSTGRES_USER=shifter` / `POSTGRES_DB=shifter` for the shifter app — and the chirpstack DB will also use the SAME `shifter` user (simpler than a separate user) — embed the DSN with `shifter` user and no password variable needed. Use the simpler form: `postgres://shifter:PLACEHOLDER@postgres:5432/chirpstack?sslmode=disable`. 

Actually the simplest working approach for bundled mode: read the postgres_password secret in a startup helper OR use the postgres superuser. The cleanest path without adding startup complexity: configure postgres with TRUST auth for the internal Docker network (too insecure) OR embed the password via an environment variable passed to chirpstack.

Final decision: Pass the postgres password to chirpstack as an environment variable `POSTGRESQL_PASSWORD` (not a secret file), then use `${POSTGRESQL_PASSWORD}` substitution in the TOML. Update the chirpstack service in bundled.yml to add:

```yaml
    environment:
      POSTGRESQL_PASSWORD: "${POSTGRES_PASSWORD:-}"
```

BUT secrets can't be env vars in compose — they're files. The actual fix: use `POSTGRES_PASSWORD_FILE` to read the file and pass it as an env var using a shell wrapper, OR just hardcode the password in the TOML for bundled dev mode (not production).

Simplest correct approach for rehearsal: add `POSTGRESQL_PASSWORD` as a plain environment variable in the chirpstack service, sourced from the same secret file path using compose's secret mechanism. Since compose `secrets:` only mounts files, use a shell entrypoint to export the var, OR use `environment:` with a file-sourced variable.

Definitive approach: use `secrets: [postgres_password]` on chirpstack, then set the DSN to read the file path:

```toml
[postgresql]
  dsn="postgres://shifter:$(cat /run/secrets/postgres_password)@postgres:5432/chirpstack?sslmode=disable"
```

ChirpStack v4 does NOT support shell command substitution in TOML. It supports `${VAR}` env substitution only.

Therefore the correct bundled approach:
1. Keep `secrets: [postgres_password]` on chirpstack service.
2. Add a `command` override that reads the secret file and sets the env var before launching chirpstack:
   - Change `command` to a shell entrypoint: `["sh", "-c", "export POSTGRESQL_PASSWORD=$(cat /run/secrets/postgres_password) && exec chirpstack -c /etc/chirpstack"]`
3. In chirpstack.toml use `dsn="postgres://shifter:${POSTGRESQL_PASSWORD}@postgres:5432/chirpstack?sslmode=disable"`

Update compose/bundled.yml chirpstack service accordingly:

```yaml
  chirpstack:
    image: chirpstack/chirpstack:4.10
    restart: unless-stopped
    depends_on:
      postgres:
        condition: service_healthy
      mosquitto:
        condition: service_started
      redis:
        condition: service_started
    secrets: [postgres_password]
    command:
      - "sh"
      - "-c"
      - "export POSTGRESQL_PASSWORD=$(cat /run/secrets/postgres_password) && exec chirpstack -c /etc/chirpstack"
    volumes:
      - ./chirpstack:/etc/chirpstack:ro
    logging: *json-logging
    networks: [shifter]
```

And in `compose/chirpstack/chirpstack.toml` use:
```toml
[postgresql]
  dsn="postgres://shifter:${POSTGRESQL_PASSWORD}@postgres:5432/chirpstack?sslmode=disable"
```

---

Create `compose/chirpstack/region_as923.toml` (verbatim from official chirpstack-docker):

```toml
# This file contains an example AS923 configuration.
[[regions]]

  id="as923"
  description="AS923"
  common_name="AS923"

  [regions.gateway]
    force_gws_private=false

    [regions.gateway.backend]
      enabled="mqtt"

      [regions.gateway.backend.mqtt]
        topic_prefix="as923"
        server="tcp://mosquitto:1883"
        username=""
        password=""
        qos=0
        clean_session=false
        client_id=""
        keep_alive_interval="30s"
        ca_cert=""
        tls_cert=""
        tls_key=""

    [[regions.gateway.channels]]
      frequency=923200000
      bandwidth=125000
      modulation="LORA"
      spreading_factors=[7, 8, 9, 10, 11, 12]

    [[regions.gateway.channels]]
      frequency=923400000
      bandwidth=125000
      modulation="LORA"
      spreading_factors=[7, 8, 9, 10, 11, 12]

  [regions.network]
    installation_margin=10
    rx_window=0
    rx1_delay=1
    rx1_dr_offset=0
    rx2_dr=2
    rx2_frequency=923200000
    rx2_prefer_on_rx1_dr_lt=0
    rx2_prefer_on_link_budget=false
    downlink_tx_power=-1
    adr_disabled=false
    min_dr=0
    max_dr=5

    [regions.network.rejoin_request]
      enabled=false
      max_count_n=0
      max_time_n=0

    [regions.network.class_b]
      ping_slot_dr=3
      ping_slot_frequency=0
```

Create `compose/chirpstack/region_as923_2.toml` (verbatim from official — AS923-2 plan, Thailand):

```toml
# This file contains an example AS923_2 configuration.
[[regions]]

  id="as923_2"
  description="AS923-2"
  common_name="AS923_2"

  [regions.gateway]
    force_gws_private=false

    [regions.gateway.backend]
      enabled="mqtt"

      [regions.gateway.backend.mqtt]
        topic_prefix="as923_2"
        server="tcp://mosquitto:1883"
        username=""
        password=""
        qos=0
        clean_session=false
        client_id=""
        keep_alive_interval="30s"
        ca_cert=""
        tls_cert=""
        tls_key=""

    [[regions.gateway.channels]]
      frequency=921400000
      bandwidth=125000
      modulation="LORA"
      spreading_factors=[7, 8, 9, 10, 11, 12]

    [[regions.gateway.channels]]
      frequency=921600000
      bandwidth=125000
      modulation="LORA"
      spreading_factors=[7, 8, 9, 10, 11, 12]

  [regions.network]
    installation_margin=10
    rx_window=0
    rx1_delay=1
    rx1_dr_offset=0
    rx2_dr=0
    rx2_frequency=921400000
    rx2_prefer_on_rx1_dr_lt=0
    rx2_prefer_on_link_budget=false
    downlink_tx_power=-1
    adr_disabled=false
    min_dr=0
    max_dr=5

    [regions.network.rejoin_request]
      enabled=false
      max_count_n=0
      max_time_n=0

    [regions.network.class_b]
      ping_slot_dr=3
      ping_slot_frequency=0
```

Create `compose/chirpstack/region_as923_3.toml` (modeled on as923 — AS923-3 plan, 917.1–918.9 MHz band):

```toml
# AS923-3 configuration (modeled on AS923 base; 917.1–918.9 MHz sub-band).
[[regions]]

  id="as923_3"
  description="AS923-3"
  common_name="AS923_3"

  [regions.gateway]
    force_gws_private=false

    [regions.gateway.backend]
      enabled="mqtt"

      [regions.gateway.backend.mqtt]
        topic_prefix="as923_3"
        server="tcp://mosquitto:1883"
        username=""
        password=""
        qos=0
        clean_session=false
        client_id=""
        keep_alive_interval="30s"
        ca_cert=""
        tls_cert=""
        tls_key=""

    [[regions.gateway.channels]]
      frequency=917100000
      bandwidth=125000
      modulation="LORA"
      spreading_factors=[7, 8, 9, 10, 11, 12]

    [[regions.gateway.channels]]
      frequency=917300000
      bandwidth=125000
      modulation="LORA"
      spreading_factors=[7, 8, 9, 10, 11, 12]

  [regions.network]
    installation_margin=10
    rx_window=0
    rx1_delay=1
    rx1_dr_offset=0
    rx2_dr=2
    rx2_frequency=917100000
    rx2_prefer_on_rx1_dr_lt=0
    rx2_prefer_on_link_budget=false
    downlink_tx_power=-1
    adr_disabled=false
    min_dr=0
    max_dr=5

    [regions.network.rejoin_request]
      enabled=false
      max_count_n=0
      max_time_n=0

    [regions.network.class_b]
      ping_slot_dr=3
      ping_slot_frequency=0
```

Create `compose/chirpstack/region_as923_4.toml` (modeled on as923 — AS923-4 plan, 917.3–920.9 MHz band):

```toml
# AS923-4 configuration (modeled on AS923 base; 917.3–920.9 MHz sub-band).
[[regions]]

  id="as923_4"
  description="AS923-4"
  common_name="AS923_4"

  [regions.gateway]
    force_gws_private=false

    [regions.gateway.backend]
      enabled="mqtt"

      [regions.gateway.backend.mqtt]
        topic_prefix="as923_4"
        server="tcp://mosquitto:1883"
        username=""
        password=""
        qos=0
        clean_session=false
        client_id=""
        keep_alive_interval="30s"
        ca_cert=""
        tls_cert=""
        tls_key=""

    [[regions.gateway.channels]]
      frequency=917300000
      bandwidth=125000
      modulation="LORA"
      spreading_factors=[7, 8, 9, 10, 11, 12]

    [[regions.gateway.channels]]
      frequency=917500000
      bandwidth=125000
      modulation="LORA"
      spreading_factors=[7, 8, 9, 10, 11, 12]

  [regions.network]
    installation_margin=10
    rx_window=0
    rx1_delay=1
    rx1_dr_offset=0
    rx2_dr=2
    rx2_frequency=917300000
    rx2_prefer_on_rx1_dr_lt=0
    rx2_prefer_on_link_budget=false
    downlink_tx_power=-1
    adr_disabled=false
    min_dr=0
    max_dr=5

    [regions.network.rejoin_request]
      enabled=false
      max_count_n=0
      max_time_n=0

    [regions.network.class_b]
      ping_slot_dr=3
      ping_slot_frequency=0
```

Create `compose/chirpstack/region_eu868.toml` (verbatim from official):

```toml
# This file contains an example EU868 configuration.
[[regions]]

  id="eu868"
  description="EU868"
  common_name="EU868"

  [regions.gateway]
    force_gws_private=false

    [regions.gateway.backend]
      enabled="mqtt"

      [regions.gateway.backend.mqtt]
        topic_prefix="eu868"
        server="tcp://mosquitto:1883"
        username=""
        password=""
        qos=0
        clean_session=false
        client_id=""
        keep_alive_interval="30s"
        ca_cert=""
        tls_cert=""
        tls_key=""

    [[regions.gateway.channels]]
      frequency=868100000
      bandwidth=125000
      modulation="LORA"
      spreading_factors=[7, 8, 9, 10, 11, 12]

    [[regions.gateway.channels]]
      frequency=868300000
      bandwidth=125000
      modulation="LORA"
      spreading_factors=[7, 8, 9, 10, 11, 12]

    [[regions.gateway.channels]]
      frequency=868500000
      bandwidth=125000
      modulation="LORA"
      spreading_factors=[7, 8, 9, 10, 11, 12]

    [[regions.gateway.channels]]
      frequency=867100000
      bandwidth=125000
      modulation="LORA"
      spreading_factors=[7, 8, 9, 10, 11, 12]

    [[regions.gateway.channels]]
      frequency=867300000
      bandwidth=125000
      modulation="LORA"
      spreading_factors=[7, 8, 9, 10, 11, 12]

    [[regions.gateway.channels]]
      frequency=867500000
      bandwidth=125000
      modulation="LORA"
      spreading_factors=[7, 8, 9, 10, 11, 12]

    [[regions.gateway.channels]]
      frequency=867700000
      bandwidth=125000
      modulation="LORA"
      spreading_factors=[7, 8, 9, 10, 11, 12]

    [[regions.gateway.channels]]
      frequency=867900000
      bandwidth=125000
      modulation="LORA"
      spreading_factors=[7, 8, 9, 10, 11, 12]

    [[regions.gateway.channels]]
      frequency=868300000
      bandwidth=250000
      modulation="LORA"
      spreading_factors=[7]

    [[regions.gateway.channels]]
      frequency=868800000
      bandwidth=125000
      modulation="FSK"
      datarate=50000

  [regions.network]
    installation_margin=10
    rx_window=0
    rx1_delay=1
    rx1_dr_offset=0
    rx2_dr=0
    rx2_frequency=869525000
    rx2_prefer_on_rx1_dr_lt=0
    rx2_prefer_on_link_budget=false
    downlink_tx_power=-1
    adr_disabled=false
    min_dr=0
    max_dr=5

    [regions.network.rejoin_request]
      enabled=false
      max_count_n=0
      max_time_n=0

    [regions.network.class_b]
      ping_slot_dr=3
      ping_slot_frequency=0

    [[regions.network.extra_channels]]
    frequency=867100000
    min_dr=0
    max_dr=5

    [[regions.network.extra_channels]]
    frequency=867300000
    min_dr=0
    max_dr=5

    [[regions.network.extra_channels]]
    frequency=867500000
    min_dr=0
    max_dr=5

    [[regions.network.extra_channels]]
    frequency=867700000
    min_dr=0
    max_dr=5

    [[regions.network.extra_channels]]
    frequency=867900000
    min_dr=0
    max_dr=5
```

Create `compose/chirpstack/region_us915_0.toml` (verbatim from official):

```toml
# This file contains an example US915 example (channels 0-7 + 64).
[[regions]]

  id="us915_0"
  description="US915 (channels 0-7 + 64)"
  common_name="US915"

  [regions.gateway]
    force_gws_private=false

    [regions.gateway.backend]
      enabled="mqtt"

      [regions.gateway.backend.mqtt]
        topic_prefix="us915_0"
        server="tcp://mosquitto:1883"
        username=""
        password=""
        qos=0
        clean_session=false
        client_id=""
        keep_alive_interval="30s"
        ca_cert=""
        tls_cert=""
        tls_key=""

    [[regions.gateway.channels]]
      frequency=902300000
      bandwidth=125000
      modulation="LORA"
      spreading_factors=[7, 8, 9, 10]

    [[regions.gateway.channels]]
      frequency=902500000
      bandwidth=125000
      modulation="LORA"
      spreading_factors=[7, 8, 9, 10]

    [[regions.gateway.channels]]
      frequency=902700000
      bandwidth=125000
      modulation="LORA"
      spreading_factors=[7, 8, 9, 10]

    [[regions.gateway.channels]]
      frequency=902900000
      bandwidth=125000
      modulation="LORA"
      spreading_factors=[7, 8, 9, 10]

    [[regions.gateway.channels]]
      frequency=903100000
      bandwidth=125000
      modulation="LORA"
      spreading_factors=[7, 8, 9, 10]

    [[regions.gateway.channels]]
      frequency=903300000
      bandwidth=125000
      modulation="LORA"
      spreading_factors=[7, 8, 9, 10]

    [[regions.gateway.channels]]
      frequency=903500000
      bandwidth=125000
      modulation="LORA"
      spreading_factors=[7, 8, 9, 10]

    [[regions.gateway.channels]]
      frequency=903700000
      bandwidth=125000
      modulation="LORA"
      spreading_factors=[7, 8, 9, 10]

    [[regions.gateway.channels]]
      frequency=903000000
      bandwidth=500000
      modulation="LORA"
      spreading_factors=[8]

  [regions.network]
    installation_margin=10
    rx_window=0
    rx1_delay=1
    rx1_dr_offset=0
    rx2_dr=8
    rx2_frequency=923300000
    rx2_prefer_on_rx1_dr_lt=0
    rx2_prefer_on_link_budget=false
    downlink_tx_power=-1
    adr_disabled=false
    min_dr=0
    max_dr=3
    enabled_uplink_channels=[0, 1, 2, 3, 4, 5, 6, 7, 64]

    [regions.network.rejoin_request]
      enabled=false
      max_count_n=0
      max_time_n=0

    [regions.network.class_b]
      ping_slot_dr=8
      ping_slot_frequency=0
```

Create `compose/chirpstack/region_au915_0.toml` (verbatim structure from official AU915):

```toml
# This file contains an example AU915 configuration (channels 0-7 + 64).
[[regions]]

  id="au915_0"
  description="AU915 (channels 0-7 + 64)"
  common_name="AU915"

  [regions.gateway]
    force_gws_private=false

    [regions.gateway.backend]
      enabled="mqtt"

      [regions.gateway.backend.mqtt]
        topic_prefix="au915_0"
        server="tcp://mosquitto:1883"
        username=""
        password=""
        qos=0
        clean_session=false
        client_id=""
        keep_alive_interval="30s"
        ca_cert=""
        tls_cert=""
        tls_key=""

    [[regions.gateway.channels]]
      frequency=915200000
      bandwidth=125000
      modulation="LORA"
      spreading_factors=[7, 8, 9, 10, 11, 12]

    [[regions.gateway.channels]]
      frequency=915400000
      bandwidth=125000
      modulation="LORA"
      spreading_factors=[7, 8, 9, 10, 11, 12]

    [[regions.gateway.channels]]
      frequency=915600000
      bandwidth=125000
      modulation="LORA"
      spreading_factors=[7, 8, 9, 10, 11, 12]

    [[regions.gateway.channels]]
      frequency=915800000
      bandwidth=125000
      modulation="LORA"
      spreading_factors=[7, 8, 9, 10, 11, 12]

    [[regions.gateway.channels]]
      frequency=916000000
      bandwidth=125000
      modulation="LORA"
      spreading_factors=[7, 8, 9, 10, 11, 12]

    [[regions.gateway.channels]]
      frequency=916200000
      bandwidth=125000
      modulation="LORA"
      spreading_factors=[7, 8, 9, 10, 11, 12]

    [[regions.gateway.channels]]
      frequency=916400000
      bandwidth=125000
      modulation="LORA"
      spreading_factors=[7, 8, 9, 10, 11, 12]

    [[regions.gateway.channels]]
      frequency=916600000
      bandwidth=125000
      modulation="LORA"
      spreading_factors=[7, 8, 9, 10, 11, 12]

    [[regions.gateway.channels]]
      frequency=915900000
      bandwidth=500000
      modulation="LORA"
      spreading_factors=[8]

  [regions.network]
    installation_margin=10
    rx_window=0
    rx1_delay=1
    rx1_dr_offset=0
    rx2_dr=8
    rx2_frequency=923300000
    rx2_prefer_on_rx1_dr_lt=0
    rx2_prefer_on_link_budget=false
    downlink_tx_power=-1
    adr_disabled=false
    min_dr=0
    max_dr=5
    enabled_uplink_channels=[0, 1, 2, 3, 4, 5, 6, 7, 64]

    [regions.network.rejoin_request]
      enabled=false
      max_count_n=0
      max_time_n=0

    [regions.network.class_b]
      ping_slot_dr=8
      ping_slot_frequency=0
```

Create `compose/chirpstack/region_in865.toml` (verbatim from official):

```toml
# This file contains an example IN865 configuration.
[[regions]]

  id="in865"
  description="IN865"
  common_name="IN865"

  [regions.gateway]
    force_gws_private=false

    [regions.gateway.backend]
      enabled="mqtt"

      [regions.gateway.backend.mqtt]
        topic_prefix="in865"
        server="tcp://mosquitto:1883"
        username=""
        password=""
        qos=0
        clean_session=false
        client_id=""
        keep_alive_interval="30s"
        ca_cert=""
        tls_cert=""
        tls_key=""

    [[regions.gateway.channels]]
      frequency=865062500
      bandwidth=125000
      modulation="LORA"
      spreading_factors=[7, 8, 9, 10, 11, 12]

    [[regions.gateway.channels]]
      frequency=865402500
      bandwidth=125000
      modulation="LORA"
      spreading_factors=[7, 8, 9, 10, 11, 12]

    [[regions.gateway.channels]]
      frequency=865985000
      bandwidth=125000
      modulation="LORA"
      spreading_factors=[7, 8, 9, 10, 11, 12]

  [regions.network]
    installation_margin=10
    rx_window=0
    rx1_delay=1
    rx1_dr_offset=0
    rx2_dr=2
    rx2_frequency=866550000
    rx2_prefer_on_rx1_dr_lt=0
    rx2_prefer_on_link_budget=false
    downlink_tx_power=-1
    adr_disabled=false
    min_dr=0
    max_dr=5

    [regions.network.rejoin_request]
      enabled=false
      max_count_n=0
      max_time_n=0

    [regions.network.class_b]
      ping_slot_dr=4
      ping_slot_frequency=0
```

## 2c — Create compose/postgres-init/01-chirpstack.sql

ChirpStack v4 requires a separate Postgres database. The bundled compose uses `POSTGRES_USER=shifter` (superuser role). Create the database owned by that same user — no new role needed.

Create `compose/postgres-init/01-chirpstack.sql`:

```sql
-- Shifter bundled: create ChirpStack database.
-- The postgres superuser (shifter) owns it; no separate role is needed.
-- This file is executed by the timescale/timescaledb image at first boot
-- via the /docker-entrypoint-initdb.d/ mechanism.
CREATE DATABASE chirpstack
    WITH OWNER = shifter
         ENCODING = 'UTF8'
         LC_COLLATE = 'en_US.utf8'
         LC_CTYPE = 'en_US.utf8'
         TEMPLATE = template0;
```

NOTE: The `postgres` service in bundled.yml uses `POSTGRES_USER=shifter` which becomes the Postgres superuser. The `POSTGRES_DB=shifter` is the primary database. Docker's entrypoint runs all `.sql` files in `/docker-entrypoint-initdb.d/` on first-boot only (when `postgres_data` volume is empty). The `chirpstack` database is thereby created before ChirpStack starts (postgres healthcheck gate ensures ordering).
  </action>
  <verify>
After `docker compose -f compose/bundled.yml up -d` from a clean state (with existing secrets/, skipping image rebuild):
- `docker ps --filter "name=chirpstack" --format "{{.Status}}"` shows `Up X seconds` (NOT `Restarting`).
- `docker compose -f compose/bundled.yml logs chirpstack 2>&1 | tail -20` shows no `config: open /etc/chirpstack` or `no such file` errors.
- `docker compose -f compose/bundled.yml exec postgres psql -U shifter -c '\l'` lists both `shifter` and `chirpstack` databases.

Static check (no Docker needed): `ls compose/chirpstack/` lists 9 TOML files; `docker compose -f compose/bundled.yml config` validates without errors.
  </verify>
  <done>
ChirpStack container reaches `Up` state (not Restarting). Both `shifter` and `chirpstack` databases exist in postgres. `compose/chirpstack/` contains chirpstack.toml + 8 region TOMLs. The `chirpstack_config` named volume entry is removed from compose/bundled.yml.
  </done>
</task>

<task type="auto">
  <name>Task 3: Redact password_hash from GET /api/install/state wire response</name>
  <files>internal/install/handlers.go, internal/install/state_test.go</files>
  <action>
## Problem

`StateHandler` calls `writeJSON(w, http.StatusOK, st)` where `st` is `*State`. `State.Step1Admin` is `json.RawMessage` — the raw JSONB bytes from the DB, containing `{"email":"..","name":"..","password_hash":".."}`. The JSON encoder embeds it verbatim, so `password_hash` appears in the wire response.

## Fix in handlers.go

Add a `stateWireResponse` type and a helper `sanitizeState` immediately before `StateHandler`. Do NOT modify the `State` struct or the `Store` — the DB layer must retain the hash for `FinishSetup` to use it.

Add after the `writeJSON` helper (around line 79):

```go
// stateWireResponse is the JSON shape returned by GET /api/install/state.
// It is identical to State except Step1Admin is sanitized to omit
// password_hash before it reaches the client (T-15-04).
type stateWireResponse struct {
	StartedAt       time.Time       `json:"started_at"`
	CompletedAt     *time.Time      `json:"completed_at"`
	CurrentStep     int             `json:"current_step"`
	Step1Admin      json.RawMessage `json:"step1_admin"`
	Step2ChirpStack json.RawMessage `json:"step2_chirpstack"`
	Step3Region     json.RawMessage `json:"step3_region"`
	Step4Identity   json.RawMessage `json:"step4_identity"`
}

// redactStep1Admin strips password_hash from the step1_admin JSONB blob
// before it is sent to the browser. The DB retains the hash for FinishSetup.
// Returns nil if the input is nil or unparseable (graceful degradation).
func redactStep1Admin(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return raw // unparseable blob — pass through untouched
	}
	delete(m, "password_hash")
	out, err := json.Marshal(m)
	if err != nil {
		return raw
	}
	return out
}
```

In `StateHandler`, replace `writeJSON(w, http.StatusOK, st)` with:

```go
		resp := stateWireResponse{
			StartedAt:       st.StartedAt,
			CompletedAt:     st.CompletedAt,
			CurrentStep:     st.CurrentStep,
			Step1Admin:      redactStep1Admin(st.Step1Admin),
			Step2ChirpStack: st.Step2ChirpStack,
			Step3Region:     st.Step3Region,
			Step4Identity:   st.Step4Identity,
		}
		writeJSON(w, http.StatusOK, resp)
```

## Fix in state_test.go

Add a new test after `TestStep1_PersistsAdmin` that exercises `redactStep1Admin` directly (pure unit test, no DB needed):

```go
// TestRedactStep1Admin — password_hash must not appear in the sanitized blob.
func TestRedactStep1Admin(t *testing.T) {
	raw := json.RawMessage(`{"email":"alice@example.com","name":"Alice","password_hash":"$argon2id$..."}`)
	out := redactStep1Admin(raw)
	require.NotContains(t, string(out), "password_hash", "password_hash must be stripped from wire response")
	require.Contains(t, string(out), "alice@example.com", "email must survive redaction")
	require.Contains(t, string(out), "Alice", "name must survive redaction")
}

// TestRedactStep1Admin_Nil — nil input must return nil (pre-step-1 state).
func TestRedactStep1Admin_Nil(t *testing.T) {
	require.Nil(t, redactStep1Admin(nil))
}
```

The import block in state_test.go already imports `"encoding/json"` indirectly through the package; add it explicitly if needed since the test references `json.RawMessage`.
  </action>
  <verify>
Unit tests: `go test ./internal/install/... -run TestRedactStep1Admin -v` — both subtests pass.
Full package: `go test ./internal/install/... -count=1` — all existing tests still pass.
Integration check (optional if stack is up): `curl -sk https://localhost/api/install/state | jq '.step1_admin'` — the returned object contains `email` and `name` but NOT `password_hash`.
  </verify>
  <done>
`go test ./internal/install/...` passes. `redactStep1Admin` strips password_hash and preserves other fields. nil input returns nil. No existing tests broken.
  </done>
</task>

</tasks>

<verification>
Sequential commit order: Task 1 commit, then Task 2 commit, then Task 3 commit.

Overall rehearsal check after all three tasks:
1. `./install/bundled/install.sh localhost` completes without sudo error on the backup mkdir step.
2. `docker ps` shows chirpstack container status `Up` (not `Restarting (1)` or similar).
3. `curl -sk https://localhost/api/install/state | jq 'keys'` returns state keys; `.step1_admin | keys` (after step 1 submission) does NOT include `password_hash`.
</verification>

<success_criteria>
- install.sh passes `bash -n` and runs without touching /var/lib on the host.
- ChirpStack container is Up and not restart-looping within 60s of `docker compose up -d`.
- Both `shifter` and `chirpstack` databases exist in the bundled postgres.
- `go test ./internal/install/...` passes including new redaction tests.
- `/api/install/state` response does not contain `password_hash` in any field.
</success_criteria>

<output>
After completion, create `.planning/quick/260513-qcf-fix-3-install-rehearsal-bugs-install-sh-/260513-qcf-SUMMARY.md`
</output>
