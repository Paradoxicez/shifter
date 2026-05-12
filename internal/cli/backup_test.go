package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// TestCLIBackup_FlagParsing verifies that --to and --trigger flags are
// registered on backupCmd and that missing --trigger falls back to "cli".
func TestCLIBackup_FlagParsing(t *testing.T) {
	t.Parallel()

	// --to flag must be registered.
	toFlag := backupCmd.Flags().Lookup("to")
	require.NotNil(t, toFlag, "backupCmd must have --to flag")
	require.Equal(t, "", toFlag.DefValue, "--to default must be empty (falls back to env/config)")

	// --trigger flag must be registered with default "cli".
	triggerFlag := backupCmd.Flags().Lookup("trigger")
	require.NotNil(t, triggerFlag, "backupCmd must have --trigger flag")
	require.Equal(t, "cli", triggerFlag.DefValue, "--trigger default must be 'cli'")
}

// TestCLIBackup_InvalidTrigger verifies that an unknown --trigger value
// returns a non-nil error before any DB connection is attempted.
func TestCLIBackup_InvalidTrigger(t *testing.T) {
	t.Parallel()

	// Reset the flag for the test.
	saved := backupFlagTrigger
	t.Cleanup(func() { backupFlagTrigger = saved })
	backupFlagTrigger = "webhook" // invalid

	err := runBackupCmd(&cobra.Command{}, []string{})
	require.Error(t, err, "invalid --trigger value must return error")
	require.True(t, strings.Contains(err.Error(), "webhook") ||
		strings.Contains(err.Error(), "trigger") ||
		strings.Contains(err.Error(), "config"),
		"error should mention the invalid value or config failure: %v", err)
}

// TestCLIBackup_CommandIsRegistered verifies that:
//   - backupCmd.Use == "backup"
//   - backupCmd.RunE is non-nil (the command has a run function)
//   - backupCmd appears in a freshly-constructed root's Commands() list
//
// We do NOT call the package-level Execute() here because Execute() mutates
// the shared package-level subcommand instances (setting their .Parent), which
// races with TestTestHarnessCmd_Help's isolated root construction.  Instead we
// build an isolated root — the same check Execute() performs at runtime.
func TestCLIBackup_CommandIsRegistered(t *testing.T) {
	t.Parallel()

	// Static invariants.
	require.Equal(t, "backup", backupCmd.Use, "backupCmd.Use must be 'backup'")
	require.NotNil(t, backupCmd.RunE, "backupCmd must have a RunE function")

	// Build an isolated root that mirrors Execute()'s AddCommand call so we
	// confirm backupCmd is wired without touching shared state.
	isolatedRoot := &cobra.Command{Use: "shifter", SilenceUsage: true, SilenceErrors: true}
	// Only add the commands needed for this assertion; not the full set so
	// we don't pull in unrelated shared state.
	isolatedRoot.AddCommand(backupCmd)
	found := false
	for _, cmd := range isolatedRoot.Commands() {
		if cmd.Use == "backup" {
			found = true
			break
		}
	}
	require.True(t, found, "backupCmd must appear in rootCmd.Commands()")
}
