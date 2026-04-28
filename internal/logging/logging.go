package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// New returns a slog.Logger writing one JSON line per event to stdout (D-24).
// Level: "info" or "debug" (D-25). Anything else falls back to info.
//
// Stdout is the deliberate destination — the docker `json-file` driver
// captures container stdout and applies the size+rotation caps configured
// in compose. Writing to stderr would split the operator's view of the
// binary's behavior across two streams.
//
// PITFALL #9 (RESEARCH): slog.NewJSONHandler emits exactly one JSON object
// per Handle call with a trailing newline and no pretty-printing. We
// explicitly construct HandlerOptions to lock the level and to keep
// AddSource off (file:line strings don't help when log lines are aggregated
// across containers and inflate every event).
func New(level string) *slog.Logger {
	return NewWithWriter(os.Stdout, level)
}

// NewWithWriter is the io.Writer-injectable form used by tests. The level
// switch is identical to New(); see PITFALL #9 above for why
// slog.NewJSONHandler is the only handler used.
func NewWithWriter(w io.Writer, level string) *slog.Logger {
	lvl := slog.LevelInfo
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		lvl = slog.LevelDebug
	case "info", "":
		lvl = slog.LevelInfo
		// any other value (warn, error, trace, garbage) falls through to
		// LevelInfo per D-25's two-level surface — we deliberately do not
		// expose warn/error as configurable levels.
	}
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:     lvl,
		AddSource: false,
	})
	return slog.New(h)
}
