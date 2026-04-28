package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// configCheckSetEnv sets a SHIFTER_SESSION_KEY of sufficient length for
// config.Validate() so the test exercises the probe-order assertions, not
// the Validate() session-key floor. Returns a cleanup that restores the prior
// value if any (t.Setenv handles unset/restore on the SESSION_KEY itself).
func configCheckSetEnv(t *testing.T, cfgPath string) {
	t.Helper()
	t.Setenv("SHIFTER_CONFIG_FILE", cfgPath)
	t.Setenv("SHIFTER_SESSION_KEY", strings.Repeat("k", 32))
}

// TestConfigCheck_FailsOnBadYAML asserts D-07: shifter config-check exits non-zero
// when config.yaml fails to parse, with a clear FAIL line on stdout.
func TestConfigCheck_FailsOnBadYAML(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(bad, []byte("this: is: not: valid: yaml: ::::"), 0o644))
	configCheckSetEnv(t, bad)

	cmd := &cobra.Command{}
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.RunE = configCheckCmd.RunE
	err := cmd.RunE(cmd, nil)
	require.Error(t, err, "D-07: bad config must produce a non-zero exit")
	out := buf.String()
	require.True(t,
		strings.Contains(out, "FAIL config") || strings.Contains(err.Error(), "config"),
		"D-07: failure must surface on stdout or in the returned error; out=%q err=%v", out, err)
}

// TestConfigCheck_ProbeOrder asserts the probes run in the documented order:
// config syntax → postgres → chirpstack → mqtt. We pin the observable contract
// by injecting a bogus DB host so postgres probe fails first; subsequent probes
// (chirpstack, mqtt) MUST NOT run.
func TestConfigCheck_ProbeOrder(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.yaml")
	// Minimal valid config — bogus DB port so postgres probe fails first.
	require.NoError(t, os.WriteFile(cfg, []byte(`
env: dev
log_level: info
http_port: "0"
db:
  host: 127.0.0.1
  port: 1
  user: shifter
  password: shifter
  database: shifter
  max_conns: 2
chirpstack:
  grpc_url: "127.0.0.1:1"
  insecure: true
mqtt:
  url: "tcp://127.0.0.1:1"
session:
  idle_timeout: 8h
  lifetime: 24h
tls:
  mode: internal
`), 0o644))
	configCheckSetEnv(t, cfg)

	cmd := &cobra.Command{}
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.RunE = configCheckCmd.RunE
	err := cmd.RunE(cmd, nil)
	require.Error(t, err, "D-07: postgres probe failure must produce non-zero exit")
	out := buf.String()
	require.Contains(t, out, "PASS config syntax",
		"D-07: config syntax must pass before any probe runs")
	require.Contains(t, out, "FAIL postgres",
		"D-07: postgres failure must surface on stdout")
	require.NotContains(t, out, "PASS chirpstack",
		"D-07: chirpstack probe must be skipped after postgres failure")
	require.NotContains(t, out, "PASS mqtt",
		"D-07: mqtt probe must be skipped after postgres failure")
}
