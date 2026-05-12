// Package install — atomic FinishSetup transaction (D-10 / RESEARCH §Pattern 3).
//
// FinishSetup commits the four wizard drafts as a single Serializable
// transaction:
//  1. INSERT admin user (rejects if duplicate email)
//  2. UPSERT install_identity (singleton id=1)
//  3. UPSERT chirpstack_connection (singleton id=1)
//  4. INSERT retention_config with D-09 defaults (Phase 5 DATA-13)
//  5. DELETE install_state (drops the wizard draft)
//
// All-or-nothing — if any step fails the txn rolls back and the wizard remains
// reachable. Re-running succeeds because the prior partial state is gone.
// Re-running AFTER success returns ErrAlreadyCompleted (admin row exists).
//
// The retention_config seed is part of the same Serializable transaction as
// admin user creation (D-23 audit/atomicity invariant): either all rows land
// or none do. ON CONFLICT DO NOTHING on the retention_config INSERT preserves
// any operator-set values on the rare path where the row was manually inserted
// before wizard completion.
//
// Concurrency: Serializable isolation collapses two simultaneous finish
// requests to a single committed state — never partial. The pre-check
// (`adminExists`) short-circuits the second caller before opening a txn so the
// happy path is one round-trip.
package install

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrAlreadyCompleted — the install is already finished (an active admin row
// exists). Plan 18 / wizard UI must treat this as a 410 Gone signal: the
// wizard is no longer reachable.
var ErrAlreadyCompleted = errors.New("install: already completed")

// ErrIncompleteWizard — at least one of step1..step4 is missing from
// install_state. Plan 18 / wizard UI must treat this as a 422: the operator
// hasn't finished filling out the wizard.
var ErrIncompleteWizard = errors.New("install: not all wizard steps captured")

// step1Draft mirrors Step1Handler's persisted JSONB shape. All three fields
// are mandatory; password_hash is the Argon2id PHC string produced by
// auth.Hash (raw password is NEVER persisted — D-09).
type step1Draft struct {
	Email        string `json:"email"`
	Name         string `json:"name"`
	PasswordHash string `json:"password_hash"`
}

// step2Draft mirrors Step2Handler's persisted JSONB shape. *_ref fields are
// filesystem paths under SecretsDir, NOT raw values (T-15-02 / D-04).
type step2Draft struct {
	Mode            string `json:"mode"`
	GRPCURL         string `json:"grpc_url"`
	APITokenRef     string `json:"api_token_ref"`
	MQTTURL         string `json:"mqtt_url"`
	MQTTUser        string `json:"mqtt_user,omitempty"`
	MQTTPasswordRef string `json:"mqtt_password_ref,omitempty"`
}

// step3Draft mirrors Step3Handler's persisted JSONB shape. Both fields come
// from the Plan 14 Regions() catalog whitelist, so they are guaranteed to be
// valid LoRaWAN sub-band identifiers.
type step3Draft struct {
	Name       string `json:"name"`
	CommonName string `json:"common_name"`
}

// step4Draft mirrors Step4Handler's persisted JSONB shape. Timezone is a
// validated IANA tz name (Step4Handler runs time.LoadLocation pre-persist);
// units is the install_identity.units enum value.
type step4Draft struct {
	DisplayName string `json:"display_name"`
	Address     string `json:"address,omitempty"`
	Timezone    string `json:"timezone"`
	Units       string `json:"units"`
	LogoPath    string `json:"logo_path,omitempty"`
}

// FinishSetup commits the wizard atomically (RESEARCH §Pattern 3 / D-10).
//
// Order of operations inside the Serializable txn:
//  1. INSERT admin user (rejects if duplicate email)
//  2. UPSERT install_identity (singleton id=1)
//  3. UPSERT chirpstack_connection (singleton id=1)
//  4. DELETE install_state (drops the wizard draft)
//
// Idempotency / re-run semantics:
//   - If admin already exists  → returns ErrAlreadyCompleted (no txn opened).
//   - If any step is missing   → returns ErrIncompleteWizard (no txn opened).
//   - On any txn-internal error → rolls back; wizard remains reachable for retry.
//
// Concurrency: pgx.Serializable + the singleton-row UPSERT pattern collapse
// two concurrent finishes to a single committed state. The deferred Rollback
// is a no-op after a successful Commit (safe by pgx contract).
func FinishSetup(ctx context.Context, deps Deps) error {
	// Pre-check: bail out early if an admin already exists.
	var adminAlready bool
	err := deps.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM "user" WHERE role='admin' AND disabled_at IS NULL)`,
	).Scan(&adminAlready)
	if err != nil {
		return fmt.Errorf("check admin: %w", err)
	}
	if adminAlready {
		return ErrAlreadyCompleted
	}

	// Load drafts from install_state.
	state, err := deps.Store.GetOrCreate(ctx)
	if err != nil {
		return fmt.Errorf("load drafts: %w", err)
	}

	var s1 step1Draft
	var s2 step2Draft
	var s3 step3Draft
	var s4 step4Draft
	if err := unmarshalRequired(state.Step1Admin, &s1, "step1"); err != nil {
		return ErrIncompleteWizard
	}
	if err := unmarshalRequired(state.Step2ChirpStack, &s2, "step2"); err != nil {
		return ErrIncompleteWizard
	}
	if err := unmarshalRequired(state.Step3Region, &s3, "step3"); err != nil {
		return ErrIncompleteWizard
	}
	if err := unmarshalRequired(state.Step4Identity, &s4, "step4"); err != nil {
		return ErrIncompleteWizard
	}

	tx, err := deps.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// 1. Admin user. D-09: must_change_password=FALSE (the operator chose the
	// password during step 1, no force-change UI gate). The user_email_lowercase
	// CHECK is satisfied because Step1Handler lowercased the email pre-persist.
	if _, err := tx.Exec(ctx,
		`INSERT INTO "user" (email, name, password_hash, role, must_change_password)
		   VALUES ($1, $2, $3, 'admin', FALSE)`,
		s1.Email, s1.Name, s1.PasswordHash); err != nil {
		return fmt.Errorf("insert admin: %w", err)
	}

	// 2. Install identity (singleton id=1).
	if _, err := tx.Exec(ctx,
		`INSERT INTO install_identity (id, display_name, logo_path, address, timezone, units)
		   VALUES (1, $1, $2, $3, $4, $5::units_system)
		   ON CONFLICT (id) DO UPDATE SET
		       display_name = EXCLUDED.display_name,
		       logo_path    = EXCLUDED.logo_path,
		       address      = EXCLUDED.address,
		       timezone     = EXCLUDED.timezone,
		       units        = EXCLUDED.units`,
		s4.DisplayName, nullable(s4.LogoPath), nullable(s4.Address), s4.Timezone, s4.Units); err != nil {
		return fmt.Errorf("upsert identity: %w", err)
	}

	// 3. ChirpStack connection (singleton id=1).
	if _, err := tx.Exec(ctx,
		`INSERT INTO chirpstack_connection
		   (id, mode, grpc_url, api_token_ref, mqtt_url, mqtt_user, mqtt_password_ref, region_name, region_common_name)
		   VALUES (1, $1::chirpstack_mode, $2, $3, $4, $5, $6, $7, $8)
		   ON CONFLICT (id) DO UPDATE SET
		       mode               = EXCLUDED.mode,
		       grpc_url           = EXCLUDED.grpc_url,
		       api_token_ref      = EXCLUDED.api_token_ref,
		       mqtt_url           = EXCLUDED.mqtt_url,
		       mqtt_user          = EXCLUDED.mqtt_user,
		       mqtt_password_ref  = EXCLUDED.mqtt_password_ref,
		       region_name        = EXCLUDED.region_name,
		       region_common_name = EXCLUDED.region_common_name`,
		s2.Mode, s2.GRPCURL, s2.APITokenRef, s2.MQTTURL,
		nullable(s2.MQTTUser), nullable(s2.MQTTPasswordRef),
		s3.Name, s3.CommonName); err != nil {
		return fmt.Errorf("upsert chirpstack_connection: %w", err)
	}

	// 4. Seed retention_config with D-09 defaults (Phase 5 DATA-13). NULL yearly_days
	// = forever per D-09. ON CONFLICT DO NOTHING preserves operator changes made
	// before a re-run (which is itself blocked above by ErrAlreadyCompleted, but
	// belt-and-suspenders).
	//
	// Phase 6 (Plan 06-01) — D-13 + D-38: also explicitly UPDATE alerts_days =
	// 365 and audit_log_days = 1825 in the same Serializable tx. Migration 0040
	// gives these columns NOT NULL DEFAULTs already; the explicit UPDATE is
	// belt-and-suspenders against future schema drift and makes the seed
	// values visible in code review next to the Phase 5 values.
	if _, err := tx.Exec(ctx, `
		INSERT INTO retention_config (id, raw_days, hourly_days, daily_days, monthly_days, yearly_days, alerts_days, audit_log_days)
		VALUES (1, 90, 365, 1825, 7300, NULL, 365, 1825)
		ON CONFLICT (id) DO UPDATE SET
		    alerts_days    = EXCLUDED.alerts_days,
		    audit_log_days = EXCLUDED.audit_log_days
	`); err != nil {
		return fmt.Errorf("seed retention_config: %w", err)
	}

	// 5. Drop the install_state draft (D-11: post-finish, GET /state → 410).
	if _, err := tx.Exec(ctx, `DELETE FROM install_state WHERE id = 1`); err != nil {
		return fmt.Errorf("delete install_state: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// unmarshalRequired returns an error when raw is empty/nil OR when the JSON
// payload doesn't decode into dst. Used as the "step is captured" gate for
// FinishSetup's draft loading.
func unmarshalRequired(raw json.RawMessage, dst any, name string) error {
	if len(raw) == 0 {
		return fmt.Errorf("%s: missing", name)
	}
	return json.Unmarshal(raw, dst)
}

// nullable returns nil for an empty string (so pgx writes SQL NULL instead of
// the empty string). install_identity.address / chirpstack_connection.mqtt_user
// are NULLABLE columns — empty-string would silently violate the operator's
// "I left this blank" intent if it round-tripped as a non-NULL value.
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
