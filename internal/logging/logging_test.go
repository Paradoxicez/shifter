package logging

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNewLogger_OneJSONLinePerEvent asserts D-24 + PITFALL #9: slog JSON
// handler must emit one JSON object per event terminated by exactly one
// newline. Pretty-printing or multi-line output breaks the docker
// json-file driver's "one event = one line" assumption.
func TestNewLogger_OneJSONLinePerEvent(t *testing.T) {
	var buf bytes.Buffer
	lg := NewWithWriter(&buf, "info")
	lg.Info("first")
	lg.Info("second")
	lg.Info("third")

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	require.Len(t, lines, 3, "expect one JSON line per event (PITFALL #9)")
	for i, line := range lines {
		var m map[string]any
		require.NoErrorf(t, json.Unmarshal([]byte(line), &m), "line %d not valid JSON: %q", i, line)
		require.Equal(t, "INFO", m["level"])
	}
}

func TestNewLogger_RespectsDebugLevel(t *testing.T) {
	var buf bytes.Buffer
	lg := NewWithWriter(&buf, "debug")
	lg.Debug("debug-line")
	require.Contains(t, buf.String(), "debug-line")
}

func TestNewLogger_DefaultLevelInfo(t *testing.T) {
	var buf bytes.Buffer
	lg := NewWithWriter(&buf, "info")
	lg.Debug("debug-line-should-be-dropped")
	require.NotContains(t, buf.String(), "debug-line-should-be-dropped")
}

// TestNewLogger_UnknownLevelFallsBackToInfo: the plan reference says "anything
// else falls back to info" — caller passes a junk level, debug must still be
// dropped, info still emitted.
func TestNewLogger_UnknownLevelFallsBackToInfo(t *testing.T) {
	var buf bytes.Buffer
	lg := NewWithWriter(&buf, "trace") // not a valid level
	lg.Debug("dropped")
	lg.Info("kept")
	require.NotContains(t, buf.String(), "dropped")
	require.Contains(t, buf.String(), "kept")
}

// TestNew_DefaultsToStdout sanity-checks that the convenience constructor
// returns a non-nil logger (we can't easily inspect stdout writes here).
func TestNew_DefaultsToStdout(t *testing.T) {
	lg := New("info")
	require.NotNil(t, lg)
}
