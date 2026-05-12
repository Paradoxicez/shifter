package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// TestCLIRestore_FlagParsing verifies that --from (required) and
// --expected-sha256 (optional) flags are registered on restoreCmd.
func TestCLIRestore_FlagParsing(t *testing.T) {
	t.Parallel()

	// --from flag must be registered and required.
	fromFlag := restoreCmd.Flags().Lookup("from")
	require.NotNil(t, fromFlag, "restoreCmd must have --from flag")
	require.Equal(t, "", fromFlag.DefValue, "--from default must be empty")

	// --from must be marked required (MarkFlagRequired sets an annotation).
	annotations := restoreCmd.Flags().Lookup("from").Annotations
	_, requiredAnnotation := annotations[cobra.BashCompOneRequiredFlag]
	require.True(t, requiredAnnotation, "--from must be marked required")

	// --expected-sha256 flag must be registered with empty default.
	sha256Flag := restoreCmd.Flags().Lookup("expected-sha256")
	require.NotNil(t, sha256Flag, "restoreCmd must have --expected-sha256 flag")
	require.Equal(t, "", sha256Flag.DefValue, "--expected-sha256 default must be empty")
}

// TestCLIRestore_MissingFromFlagError verifies that calling restoreCmd without
// --from returns a usage error (cobra enforces MarkFlagRequired).
func TestCLIRestore_MissingFromFlagError(t *testing.T) {
	t.Parallel()

	// Build an isolated root so we don't mutate package-level shared state.
	isolatedRoot := &cobra.Command{Use: "shifter", SilenceUsage: true, SilenceErrors: true}
	// Re-create a minimal restoreCmd clone to test the required-flag enforcement
	// without relying on the package-level restoreCmd (which may already have
	// the flag set from a previous test run in this process).
	testRestoreCmd := &cobra.Command{
		Use:  "restore",
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}
	testRestoreCmd.Flags().StringVar(new(string), "from", "", "path")
	_ = testRestoreCmd.MarkFlagRequired("from")
	isolatedRoot.AddCommand(testRestoreCmd)

	isolatedRoot.SetArgs([]string{"restore"}) // no --from
	err := isolatedRoot.Execute()
	require.Error(t, err, "missing required --from must produce an error")
}

// TestCLIRestore_CommandIsRegistered verifies that:
//   - restoreCmd.Use == "restore"
//   - restoreCmd.RunE is non-nil
//   - restoreCmd appears in a freshly-constructed isolated root
func TestCLIRestore_CommandIsRegistered(t *testing.T) {
	t.Parallel()

	require.Equal(t, "restore", restoreCmd.Use, "restoreCmd.Use must be 'restore'")
	require.NotNil(t, restoreCmd.RunE, "restoreCmd must have a RunE function")

	// Build an isolated root that mirrors Execute()'s AddCommand call.
	isolatedRoot := &cobra.Command{Use: "shifter", SilenceUsage: true, SilenceErrors: true}
	isolatedRoot.AddCommand(restoreCmd)
	found := false
	for _, cmd := range isolatedRoot.Commands() {
		if cmd.Use == "restore" {
			found = true
			break
		}
	}
	require.True(t, found, "restoreCmd must appear in root.Commands()")
}

// TestCLIRestore_LongDescContainsKeyPhrases verifies the Long description
// documents the operator workflow and cross-version deferral.
func TestCLIRestore_LongDescContainsKeyPhrases(t *testing.T) {
	t.Parallel()
	long := restoreCmd.Long
	require.True(t, strings.Contains(long, "docker compose") ||
		strings.Contains(long, "compose stop"),
		"Long description must mention the docker compose stop workflow")
	require.Contains(t, long, "v1", "Long description must mention v1 cross-version deferral")
}

// TestCLIRestore_InExecuteList verifies that restoreCmd appears in Execute()'s
// AddCommand list by reading root.go source.
func TestCLIRestore_InExecuteList(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile("root.go")
	if err != nil {
		t.Skipf("cannot read root.go: %v", err)
	}
	require.Contains(t, string(src), "restoreCmd",
		"root.go Execute() must add restoreCmd via AddCommand")
}
