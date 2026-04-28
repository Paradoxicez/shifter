package logging

import (
	"log/slog"
	"os"
	"strings"
)

// New returns a slog.Logger emitting JSON to stderr at the given level.
//
// Plan 04 (config-secrets) replaces this with the canonical D-24 / D-25
// implementation (one-line JSON, structured fields, request-id propagation).
// Today this minimal version exists so Plan 05's CLI subcommands compile and
// have a usable logger for db.RunMigrations / serve startup logs.
//
// Accepted levels: "debug" | "info" (D-25). Anything else falls back to info.
func New(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}
