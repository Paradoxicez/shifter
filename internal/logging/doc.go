// Package logging — slog handler bootstrap (D-24, D-25).
//
// Plan 04 (config-secrets) replaces this stub with a JSON handler bound to the
// config-driven log level. Plan 05 only needs the New(level) constructor today
// so the CLI subcommands can pass a *slog.Logger into db.RunMigrations and
// other helpers.
package logging
