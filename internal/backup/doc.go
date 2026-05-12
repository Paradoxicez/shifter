// Package backup implements Shifter's operator backup surface (OPS-02, OPS-03).
//
// # Tarball Layout
//
// Every backup produces a single tar.gz file named:
//
//	shifter-backup-{install_slug}-{YYYYMMDD-HHMM}-{schema_version}.tar.gz
//
// Contents:
//
//	db/shifter.dump         — pg_dump --format=custom of the Shifter database.
//	db/chirpstack.dump      — pg_dump of the ChirpStack database (bundled mode only).
//	floor-plans/            — verbatim copy of the operator's floor-plan image directory.
//	manifest.json           — metadata + per-file sha256 checksums (last entry in tarball).
//
// # Bundled vs. External Mode
//
// The Runner reads install_state.chirpstack_mode at backup time:
//
//   - "bundled": both Shifter and ChirpStack share the same Postgres instance.
//     Both databases are dumped into the tarball.  One-tarball recovery.
//   - "external": only the Shifter database is in scope.  The external
//     ChirpStack operator is responsible for their own backup.
//
// # TimescaleDB Compatibility
//
// The backup uses plain pg_dump --format=custom.  The timescaledb-backup helper
// tool was archived by TigerData in 2022; their official recommendation is to use
// pg_dump + timescaledb_pre_restore() / timescaledb_post_restore() hooks on the
// restore side (D-40, RESEARCH §Decision A).
//
// NEVER pass --jobs / -j to pg_dump or pg_restore against a TimescaleDB database.
// Parallel restore breaks the TimescaleDB catalog ordering.  The Runner enforces
// this by hardcoding the args slice; TestRunner_PgDumpFlags verifies no -j appears.
//
// # Audit-in-Transaction Pattern
//
// Every backup writes:
//   - One backup_run row (status='running') before pg_dump starts.
//   - An audit_log row with action='backup.start'.
//   - On success: UPDATE backup_run status='completed' + audit 'backup.complete'.
//   - On failure: UPDATE backup_run status='failed' + audit 'backup.failed'.
//
// The start row is committed immediately (so external observers see in-progress
// state); the completion update uses a separate transaction so the row is always
// visible regardless of whether the backup process was interrupted.
package backup

// DefaultBackupDir is the default filesystem destination for backup tarballs,
// configurable via the SHIFTER_BACKUP_DIR environment variable (OPS-03 / D-42).
const DefaultBackupDir = "/var/lib/shifter/backups"
