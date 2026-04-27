---
phase: 01-foundation
plan: 03
type: execute
wave: 3
depends_on: [01, 02]
files_modified:
  - go.mod
  - go.sum
  - internal/db/pool.go
  - internal/db/migrations.go
  - internal/db/migrations/0001_init.up.sql
  - internal/db/migrations/0001_init.down.sql
  - internal/db/migrations/0002_users.up.sql
  - internal/db/migrations/0002_users.down.sql
  - internal/db/migrations/0003_sessions.up.sql
  - internal/db/migrations/0003_sessions.down.sql
  - internal/db/migrations/0004_install_state.up.sql
  - internal/db/migrations/0004_install_state.down.sql
  - internal/db/migrations/0005_install_identity.up.sql
  - internal/db/migrations/0005_install_identity.down.sql
  - internal/db/migrations/0006_chirpstack_connection.up.sql
  - internal/db/migrations/0006_chirpstack_connection.down.sql
  - internal/db/queries/users.sql
  - internal/db/queries/install_state.sql
  - internal/db/queries/install_identity.sql
  - internal/db/queries/chirpstack_connection.sql
  - sqlc.yaml
  - internal/db/migrations_test.go
autonomous: true
requirements: []
must_haves:
  truths:
    - "RunMigrations applied to a fresh testcontainer Postgres+TimescaleDB produces all 6 schema versions"
    - "Re-running RunMigrations against an up-to-date schema is a no-op (idempotent)"
    - "Migrations are embedded into the binary via go:embed (D-13, D-16) — no external SQL files at runtime"
    - "sqlc generate produces typed Go code for users, install_state, install_identity, chirpstack_connection"
    - "TimescaleDB extension is created in 0001_init"
  artifacts:
    - path: "internal/db/pool.go"
      provides: "pgxpool.New(ctx, dsn) wrapper with sane defaults (MaxConns from config)"
      contains: "pgxpool.New"
    - path: "internal/db/migrations.go"
      provides: "go:embed of migrations/*.sql + migrate.New + Up()"
      contains: "//go:embed migrations/*.sql"
    - path: "internal/db/migrations/0001_init.up.sql"
      provides: "CREATE EXTENSION timescaledb; CREATE EXTENSION pgcrypto"
      contains: "CREATE EXTENSION"
    - path: "internal/db/migrations/0002_users.up.sql"
      provides: "user table with id (uuid), email (unique), name, password_hash, role enum"
      contains: "CREATE TABLE"
    - path: "internal/db/migrations/0003_sessions.up.sql"
      provides: "alexedwards/scs/pgxstore canonical schema (token, data, expiry)"
      contains: "sessions"
    - path: "internal/db/migrations/0004_install_state.up.sql"
      provides: "Singleton install_state table with step1..4 JSONB drafts"
      contains: "CHECK (id = 1)"
    - path: "internal/db/migrations/0005_install_identity.up.sql"
      provides: "install_identity table with display_name, logo_path, address, timezone, units"
      contains: "install_identity"
    - path: "internal/db/migrations/0006_chirpstack_connection.up.sql"
      provides: "chirpstack_connection table with mode, grpc_url, api_token_ref, mqtt_url, default_region (name + common_name + sub_band)"
      contains: "chirpstack_connection"
    - path: "sqlc.yaml"
      provides: "sqlc 2.x config: pgx/v5 engine, generates internal/db/sqlc"
      contains: "engine: postgresql"
  key_links:
    - from: "internal/db/migrations.go"
      to: "internal/db/migrations/*.sql"
      via: "go:embed"
      pattern: "//go:embed migrations"
    - from: "internal/db/migrations.go"
      to: "pgxpool.Pool"
      via: "migrate.NewWithInstance + pgx/v5 driver"
      pattern: "pgx/v5"
---

<objective>
Implement Shifter's database layer: pgxpool connection management, golang-migrate-as-library invocation with embedded SQL files, six initial migrations covering all Phase 1 schema (TimescaleDB extension, users, sessions, install_state, install_identity, chirpstack_connection), and sqlc configuration so `sqlc generate` produces typed Go for every Phase 1 query.

Purpose: D-13 (auto-migrate on serve), D-16 (golang-migrate as library), D-17 (integer-prefix migrations), and the entire schema needed by Plans 07-17. Wave 2 unblocks auth (Plan 07-11), install (Plan 14-16), and ChirpStack settings (Plan 12, 13, 17).

Output: `go test ./internal/db -run TestRunMigrations_Clean` passes against a testcontainer Postgres. The schema is the single source of truth for every later plan.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/01-foundation/01-CONTEXT.md
@.planning/phases/01-foundation/01-RESEARCH.md
@01-02-test-harness-PLAN.md

<interfaces>
RESEARCH §Pattern 7 (lines 612-666) — verbatim implementation of RunMigrations:
```go
//go:embed migrations/*.sql
var migrationsFS embed.FS

func RunMigrations(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) error {
    src, err := iofs.New(migrationsFS, "migrations")
    if err != nil { return err }
    conn, err := pool.Acquire(ctx)
    if err != nil { return err }
    defer conn.Release()
    drv, err := pgxdb.WithInstance(conn.Conn(), &pgxdb.Config{})
    if err != nil { return err }
    m, err := migrate.NewWithInstance("iofs", src, "pgx", drv)
    if err != nil { return err }
    if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
        return fmt.Errorf("migrate up: %w", err)
    }
    return nil
}
```

RESEARCH §Pattern 3 (lines 412-432) — verbatim install_state schema:
```sql
CREATE TABLE install_state (
    id            INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    started_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at  TIMESTAMPTZ,
    current_step  SMALLINT NOT NULL DEFAULT 1 CHECK (current_step BETWEEN 1 AND 5),
    step1_admin       JSONB,
    step2_chirpstack  JSONB,
    step3_region      JSONB,
    step4_identity    JSONB,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

RESEARCH §Pattern 6 (lines 575-582) — verbatim sessions schema (alexedwards/scs/pgxstore):
```sql
CREATE TABLE sessions (
    token  TEXT        PRIMARY KEY,
    data   BYTEA       NOT NULL,
    expiry TIMESTAMPTZ NOT NULL
);
CREATE INDEX sessions_expiry_idx ON sessions (expiry);
```
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Migration files (6 up/down pairs) + pool helper + RunMigrations + sqlc config</name>
  <files>go.mod, go.sum, internal/db/pool.go, internal/db/migrations.go, internal/db/migrations/0001_init.up.sql, internal/db/migrations/0001_init.down.sql, internal/db/migrations/0002_users.up.sql, internal/db/migrations/0002_users.down.sql, internal/db/migrations/0003_sessions.up.sql, internal/db/migrations/0003_sessions.down.sql, internal/db/migrations/0004_install_state.up.sql, internal/db/migrations/0004_install_state.down.sql, internal/db/migrations/0005_install_identity.up.sql, internal/db/migrations/0005_install_identity.down.sql, internal/db/migrations/0006_chirpstack_connection.up.sql, internal/db/migrations/0006_chirpstack_connection.down.sql, sqlc.yaml, internal/db/migrations_test.go</files>
  <read_first>
    - .planning/phases/01-foundation/01-RESEARCH.md §"Pattern 3: Reentrant Install Wizard" (lines 410-466)
    - .planning/phases/01-foundation/01-RESEARCH.md §"Pattern 6: SCS sessions with pgxstore" (lines 570-610)
    - .planning/phases/01-foundation/01-RESEARCH.md §"Pattern 7: golang-migrate as embedded library" (lines 612-666)
    - .planning/phases/01-foundation/01-CONTEXT.md §"Migrations" (D-16, D-17)
    - 01-02-test-harness-PLAN.md (testsupport.StartPostgres signature)
  </read_first>
  <behavior>
    - TestRunMigrations_Clean: starts a fresh testcontainer Postgres, calls RunMigrations, asserts all 6 schema_migrations versions exist and `users`, `sessions`, `install_state`, `install_identity`, `chirpstack_connection` tables exist; asserts TimescaleDB extension present.
    - TestRunMigrations_Idempotent: runs RunMigrations twice in sequence, second call returns nil (no error, no panic).
    - TestRunMigrations_DirtyState: forces a dirty version, asserts RunMigrations returns an error mentioning "dirty".
  </behavior>
  <action>
1. Install Go deps:
   ```bash
   go get github.com/jackc/pgx/v5@latest
   go get github.com/jackc/pgx/v5/pgxpool@latest
   go get github.com/golang-migrate/migrate/v4@latest
   go get github.com/golang-migrate/migrate/v4/source/iofs@latest
   go get github.com/golang-migrate/migrate/v4/database/pgx/v5@latest
   ```

2. Create `internal/db/pool.go`:
   ```go
   package db

   import (
       "context"
       "fmt"

       "github.com/jackc/pgx/v5/pgxpool"
   )

   // NewPool creates a *pgxpool.Pool with sane defaults.
   // The DSN must be a Postgres connection string.
   // MaxConns is set from the optional config; 0 means pgx default (greatest of 4 or num CPUs).
   func NewPool(ctx context.Context, dsn string, maxConns int32) (*pgxpool.Pool, error) {
       cfg, err := pgxpool.ParseConfig(dsn)
       if err != nil {
           return nil, fmt.Errorf("parse dsn: %w", err)
       }
       if maxConns > 0 {
           cfg.MaxConns = maxConns
       }
       pool, err := pgxpool.NewWithConfig(ctx, cfg)
       if err != nil {
           return nil, fmt.Errorf("new pool: %w", err)
       }
       if err := pool.Ping(ctx); err != nil {
           pool.Close()
           return nil, fmt.Errorf("ping: %w", err)
       }
       return pool, nil
   }
   ```

3. Create `internal/db/migrations.go` — VERBATIM from RESEARCH §Pattern 7 with module path inserted:
   ```go
   package db

   import (
       "context"
       "embed"
       "errors"
       "fmt"
       "log/slog"

       "github.com/golang-migrate/migrate/v4"
       pgxdb "github.com/golang-migrate/migrate/v4/database/pgx/v5"
       "github.com/golang-migrate/migrate/v4/source/iofs"
       "github.com/jackc/pgx/v5/pgxpool"
   )

   //go:embed migrations/*.sql
   var migrationsFS embed.FS

   // RunMigrations applies all pending up migrations.
   // Idempotent: returns nil with no work if schema is current.
   // Returns a descriptive error if the schema is dirty (operator must run `shifter migrate force`).
   func RunMigrations(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) error {
       src, err := iofs.New(migrationsFS, "migrations")
       if err != nil {
           return fmt.Errorf("iofs: %w", err)
       }
       conn, err := pool.Acquire(ctx)
       if err != nil {
           return fmt.Errorf("acquire conn: %w", err)
       }
       defer conn.Release()

       drv, err := pgxdb.WithInstance(conn.Conn(), &pgxdb.Config{})
       if err != nil {
           return fmt.Errorf("pgx driver: %w", err)
       }
       m, err := migrate.NewWithInstance("iofs", src, "pgx", drv)
       if err != nil {
           return fmt.Errorf("migrate.NewWithInstance: %w", err)
       }

       versionBefore, dirtyBefore, _ := m.Version()
       if dirtyBefore {
           return fmt.Errorf("schema is dirty at version %d — run `shifter migrate force <prev>` and re-run", versionBefore)
       }

       if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
           return fmt.Errorf("migrate up: %w", err)
       }

       versionAfter, _, _ := m.Version()
       log.Info("migrations applied", "from", versionBefore, "to", versionAfter)
       return nil
   }

   // ForceVersion is the recovery escape hatch for dirty migrations.
   func ForceVersion(ctx context.Context, pool *pgxpool.Pool, version int) error {
       conn, err := pool.Acquire(ctx)
       if err != nil { return err }
       defer conn.Release()
       drv, err := pgxdb.WithInstance(conn.Conn(), &pgxdb.Config{})
       if err != nil { return err }
       src, err := iofs.New(migrationsFS, "migrations")
       if err != nil { return err }
       m, err := migrate.NewWithInstance("iofs", src, "pgx", drv)
       if err != nil { return err }
       return m.Force(version)
   }
   ```

4. Migrations — write each file verbatim:

   `internal/db/migrations/0001_init.up.sql`:
   ```sql
   CREATE EXTENSION IF NOT EXISTS pgcrypto;
   CREATE EXTENSION IF NOT EXISTS timescaledb;
   ```
   `0001_init.down.sql`:
   ```sql
   -- Extensions are not auto-dropped (TimescaleDB has destructive side-effects).
   -- Manual cleanup if needed: DROP EXTENSION timescaledb CASCADE; DROP EXTENSION pgcrypto;
   SELECT 1;
   ```

   `0002_users.up.sql`:
   ```sql
   CREATE TYPE user_role AS ENUM ('admin', 'viewer');

   CREATE TABLE "user" (
       id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
       email           CITEXT, -- created via separate type fallback below
       name            TEXT NOT NULL,
       password_hash   TEXT NOT NULL,
       role            user_role NOT NULL,
       must_change_password BOOLEAN NOT NULL DEFAULT FALSE,
       disabled_at     TIMESTAMPTZ,
       created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
       updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
   );

   -- Use lower(email) unique index since CITEXT requires the citext extension.
   -- Avoid extra extension dep — just enforce lowercase via index + check.
   ALTER TABLE "user" ALTER COLUMN email TYPE TEXT;
   ALTER TABLE "user" ADD CONSTRAINT user_email_lowercase CHECK (email = lower(email));
   ALTER TABLE "user" ADD CONSTRAINT user_email_not_empty CHECK (length(email) > 0);
   CREATE UNIQUE INDEX user_email_unique ON "user" (email);

   CREATE OR REPLACE FUNCTION touch_updated_at() RETURNS trigger AS $$
   BEGIN NEW.updated_at = now(); RETURN NEW; END $$ LANGUAGE plpgsql;

   CREATE TRIGGER user_touch BEFORE UPDATE ON "user"
       FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
   ```
   `0002_users.down.sql`:
   ```sql
   DROP TABLE IF EXISTS "user";
   DROP FUNCTION IF EXISTS touch_updated_at();
   DROP TYPE IF EXISTS user_role;
   ```

   `0003_sessions.up.sql` (alexedwards/scs/pgxstore canonical schema):
   ```sql
   CREATE TABLE sessions (
       token   TEXT        PRIMARY KEY,
       data    BYTEA       NOT NULL,
       expiry  TIMESTAMPTZ NOT NULL
   );
   CREATE INDEX sessions_expiry_idx ON sessions (expiry);
   ```
   `0003_sessions.down.sql`:
   ```sql
   DROP TABLE IF EXISTS sessions;
   ```

   `0004_install_state.up.sql`:
   ```sql
   CREATE TABLE install_state (
       id              INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
       started_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
       completed_at    TIMESTAMPTZ,
       current_step    SMALLINT NOT NULL DEFAULT 1 CHECK (current_step BETWEEN 1 AND 5),
       step1_admin       JSONB,
       step2_chirpstack  JSONB,
       step3_region      JSONB,
       step4_identity    JSONB,
       updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
   );

   CREATE TRIGGER install_state_touch BEFORE UPDATE ON install_state
       FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
   ```
   `0004_install_state.down.sql`:
   ```sql
   DROP TABLE IF EXISTS install_state;
   ```

   `0005_install_identity.up.sql`:
   ```sql
   CREATE TYPE units_system AS ENUM ('metric', 'imperial');

   CREATE TABLE install_identity (
       id              INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
       display_name    TEXT NOT NULL,
       logo_path       TEXT,
       address         TEXT,
       timezone        TEXT NOT NULL DEFAULT 'UTC',
       units           units_system NOT NULL DEFAULT 'metric',
       created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
       updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
   );

   CREATE TRIGGER install_identity_touch BEFORE UPDATE ON install_identity
       FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
   ```
   `0005_install_identity.down.sql`:
   ```sql
   DROP TABLE IF EXISTS install_identity;
   DROP TYPE IF EXISTS units_system;
   ```

   `0006_chirpstack_connection.up.sql`:
   ```sql
   CREATE TYPE chirpstack_mode AS ENUM ('bundled', 'external');

   CREATE TABLE chirpstack_connection (
       id                  INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
       mode                chirpstack_mode NOT NULL,
       grpc_url            TEXT NOT NULL,
       api_token_ref       TEXT NOT NULL, -- path to file under /run/secrets/
       mqtt_url            TEXT NOT NULL,
       mqtt_user           TEXT,
       mqtt_password_ref   TEXT,
       region_name         TEXT NOT NULL,           -- ChirpStack `name` (e.g. "as923_2")
       region_common_name  TEXT NOT NULL,           -- ChirpStack `common_name` (e.g. "AS923_2")
       created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
       updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
   );

   CREATE TRIGGER chirpstack_connection_touch BEFORE UPDATE ON chirpstack_connection
       FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
   ```
   `0006_chirpstack_connection.down.sql`:
   ```sql
   DROP TABLE IF EXISTS chirpstack_connection;
   DROP TYPE IF EXISTS chirpstack_mode;
   ```

5. Create `sqlc.yaml`:
   ```yaml
   version: "2"
   sql:
     - schema: "internal/db/migrations"
       queries: "internal/db/queries"
       engine: "postgresql"
       gen:
         go:
           package: "sqlc"
           out: "internal/db/sqlc"
           sql_package: "pgx/v5"
           emit_pointers_for_null_types: true
           emit_json_tags: false
           emit_db_tags: false
           emit_prepared_queries: false
           emit_interface: true
   ```

6. Create stub query files (Plans 09, 14, 15, 17 will populate):

   `internal/db/queries/users.sql`:
   ```sql
   -- name: GetUserByEmail :one
   SELECT * FROM "user" WHERE email = $1 AND disabled_at IS NULL;

   -- name: AdminExists :one
   SELECT EXISTS(SELECT 1 FROM "user" WHERE role = 'admin' AND disabled_at IS NULL) AS exists;

   -- name: InsertAdminUser :one
   INSERT INTO "user" (email, name, password_hash, role, must_change_password)
   VALUES ($1, $2, $3, 'admin', FALSE)
   RETURNING id, email, name, role, created_at;

   -- name: UpdateUserPassword :exec
   UPDATE "user" SET password_hash = $2 WHERE id = $1;
   ```

   `internal/db/queries/install_state.sql`:
   ```sql
   -- name: GetOrCreateInstallState :one
   INSERT INTO install_state (id) VALUES (1)
   ON CONFLICT (id) DO UPDATE SET id = 1
   RETURNING *;

   -- name: UpdateStep1 :exec
   UPDATE install_state SET step1_admin = $1, current_step = GREATEST(current_step, 2) WHERE id = 1;

   -- name: UpdateStep2 :exec
   UPDATE install_state SET step2_chirpstack = $1, current_step = GREATEST(current_step, 3) WHERE id = 1;

   -- name: UpdateStep3 :exec
   UPDATE install_state SET step3_region = $1, current_step = GREATEST(current_step, 4) WHERE id = 1;

   -- name: UpdateStep4 :exec
   UPDATE install_state SET step4_identity = $1, current_step = GREATEST(current_step, 5) WHERE id = 1;

   -- name: DeleteInstallState :exec
   DELETE FROM install_state WHERE id = 1;
   ```

   `internal/db/queries/install_identity.sql`:
   ```sql
   -- name: GetInstallIdentity :one
   SELECT * FROM install_identity WHERE id = 1;

   -- name: UpsertInstallIdentity :one
   INSERT INTO install_identity (id, display_name, logo_path, address, timezone, units)
   VALUES (1, $1, $2, $3, $4, $5)
   ON CONFLICT (id) DO UPDATE SET
       display_name = EXCLUDED.display_name,
       logo_path    = EXCLUDED.logo_path,
       address      = EXCLUDED.address,
       timezone     = EXCLUDED.timezone,
       units        = EXCLUDED.units
   RETURNING *;
   ```

   `internal/db/queries/chirpstack_connection.sql`:
   ```sql
   -- name: GetChirpStackConnection :one
   SELECT * FROM chirpstack_connection WHERE id = 1;

   -- name: UpsertChirpStackConnection :one
   INSERT INTO chirpstack_connection (id, mode, grpc_url, api_token_ref, mqtt_url, mqtt_user, mqtt_password_ref, region_name, region_common_name)
   VALUES (1, $1, $2, $3, $4, $5, $6, $7, $8)
   ON CONFLICT (id) DO UPDATE SET
       mode = EXCLUDED.mode,
       grpc_url = EXCLUDED.grpc_url,
       api_token_ref = EXCLUDED.api_token_ref,
       mqtt_url = EXCLUDED.mqtt_url,
       mqtt_user = EXCLUDED.mqtt_user,
       mqtt_password_ref = EXCLUDED.mqtt_password_ref,
       region_name = EXCLUDED.region_name,
       region_common_name = EXCLUDED.region_common_name
   RETURNING *;
   ```

7. Run `sqlc generate` (after `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest` from `just bootstrap`). The generated code lives in `internal/db/sqlc/`.

8. Replace `internal/db/migrations_test.go` skip-stubs with real tests:
   ```go
   package db

   import (
       "context"
       "log/slog"
       "os"
       "testing"

       "github.com/shifter-io/shifter/internal/testsupport"
       "github.com/stretchr/testify/require"
   )

   func TestRunMigrations_Clean(t *testing.T) {
       pool := testsupport.StartPostgres(t)
       log := slog.New(slog.NewTextHandler(os.Stderr, nil))
       err := RunMigrations(context.Background(), pool, log)
       require.NoError(t, err)

       // Verify each table exists
       for _, table := range []string{"user", "sessions", "install_state", "install_identity", "chirpstack_connection"} {
           var exists bool
           err := pool.QueryRow(context.Background(),
               "SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = $1)", table,
           ).Scan(&exists)
           require.NoError(t, err)
           require.True(t, exists, "table %s should exist", table)
       }

       // Verify TimescaleDB extension
       var extExists bool
       err = pool.QueryRow(context.Background(),
           "SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'timescaledb')",
       ).Scan(&extExists)
       require.NoError(t, err)
       require.True(t, extExists, "timescaledb extension should be created")
   }

   func TestRunMigrations_Idempotent(t *testing.T) {
       pool := testsupport.StartPostgres(t)
       log := slog.New(slog.NewTextHandler(os.Stderr, nil))
       require.NoError(t, RunMigrations(context.Background(), pool, log))
       require.NoError(t, RunMigrations(context.Background(), pool, log)) // second run = no-op
   }

   func TestRunMigrations_DirtyState(t *testing.T) {
       pool := testsupport.StartPostgres(t)
       log := slog.New(slog.NewTextHandler(os.Stderr, nil))
       require.NoError(t, RunMigrations(context.Background(), pool, log))

       // Force the schema dirty
       _, err := pool.Exec(context.Background(),
           "UPDATE schema_migrations SET dirty = TRUE")
       require.NoError(t, err)

       err = RunMigrations(context.Background(), pool, log)
       require.Error(t, err)
       require.Contains(t, err.Error(), "dirty")
   }
   ```
  </action>
  <verify>
    <automated>go test ./internal/db -run 'TestRunMigrations_(Clean|Idempotent|DirtyState)' -race -count=1</automated>
  </verify>
  <acceptance_criteria>
    - Files `internal/db/migrations/0001_init.up.sql` through `0006_chirpstack_connection.up.sql` exist (6 up files + 6 down files = 12 SQL files)
    - File `internal/db/migrations/0001_init.up.sql` contains both `CREATE EXTENSION IF NOT EXISTS pgcrypto;` and `CREATE EXTENSION IF NOT EXISTS timescaledb;`
    - File `internal/db/migrations/0002_users.up.sql` contains `CREATE TYPE user_role AS ENUM ('admin', 'viewer');` and `CREATE TABLE "user"`
    - File `internal/db/migrations/0003_sessions.up.sql` contains the verbatim alexedwards/scs schema (`token TEXT PRIMARY KEY, data BYTEA NOT NULL, expiry TIMESTAMPTZ NOT NULL`)
    - File `internal/db/migrations/0004_install_state.up.sql` contains `CHECK (id = 1)` and four JSONB columns: `step1_admin`, `step2_chirpstack`, `step3_region`, `step4_identity`
    - File `internal/db/migrations/0006_chirpstack_connection.up.sql` contains `region_name` AND `region_common_name` columns
    - File `internal/db/migrations.go` contains `//go:embed migrations/*.sql` directive (line-anchored, no leading whitespace before `//go:embed`)
    - File `sqlc.yaml` has `engine: "postgresql"` and `sql_package: "pgx/v5"`
    - Command `go test ./internal/db -run TestRunMigrations_Clean -race` exits 0 (per VALIDATION.md)
    - Command `go test ./internal/db -run TestRunMigrations_Idempotent -race` exits 0
    - Command `go test ./internal/db -run TestRunMigrations_DirtyState -race` exits 0
    - Command `sqlc generate` exits 0 and produces files under `internal/db/sqlc/`
  </acceptance_criteria>
  <done>
    Schema migrations run cleanly on a fresh DB, idempotent on re-run, surface dirty state. sqlc generates typed Go for every Phase 1 query. Plan 04+ depend on this layer being in place.
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| migration source → DB | Embedded SQL files run with elevated DDL privileges |
| application → DB | All later queries flow through pgxpool (parameterized via sqlc) |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-03-01 | Tampering (SQL injection) | future query layer | mitigate | sqlc generates parameterized queries; never string interpolation. ASVS V5. |
| T-03-02 | Tampering | dirty migration state on partial failure | mitigate | RunMigrations errors out and tells operator how to recover (`shifter migrate force`); D-13 requires error-on-dirty. ASVS V11. |
| T-03-03 | Information Disclosure | password hashes / install secrets in DB backups | accept | Backup encryption is operator's responsibility per Phase 6 OPS-02; password hashes are Argon2id (one-way). ASVS V8. |
| T-03-04 | Spoofing | first-run race (two operators starting wizard simultaneously) | mitigate | `install_state` is singleton (`CHECK (id=1)`); commit is `Serializable` isolation in Plan 15. ASVS V11. |
| T-03-05 | Tampering | SQL `email` field bypasses lowercase invariant | mitigate | `CHECK (email = lower(email))` constraint in 0002_users; application code calls `lower()` before insert (Plan 09, 15). |
</threat_model>

<verification>
- All 6 migration up/down pairs exist under `internal/db/migrations/`
- `internal/db/migrations.go` embeds them via `//go:embed`
- `internal/db/pool.go` exports `NewPool(ctx, dsn, maxConns) (*pgxpool.Pool, error)`
- `sqlc.yaml` configured for pgx/v5
- 3 migration tests pass (TestRunMigrations_Clean, _Idempotent, _DirtyState)
</verification>

<success_criteria>
- A fresh testcontainer Postgres+TimescaleDB has all 5 Phase 1 tables after RunMigrations
- TimescaleDB extension is created in 0001_init (Phase 2 will need it for the hypertable)
- Re-running RunMigrations is a no-op (D-13 idempotency)
- Dirty schema produces a clear error message instructing the operator
- sqlc generates Go types for every Phase 1 query
</success_criteria>

<output>
After completion, create `.planning/phases/01-foundation/01-03-SUMMARY.md` documenting:
- Migration version range (1-6) and what each adds
- pgxpool helper signature
- sqlc-generated package import path (`github.com/shifter-io/shifter/internal/db/sqlc`)
- Conventions to inherit (singleton `CHECK (id=1)` pattern, `touch_updated_at()` trigger)
- Anything Phase 2 needs to know to add the hypertable
</output>
