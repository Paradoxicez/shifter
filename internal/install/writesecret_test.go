package install

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestWriteSecret covers the three cases for the idempotent writeSecret helper.
func TestWriteSecret(t *testing.T) {
	t.Run("absent file is written", func(t *testing.T) {
		dir := t.TempDir()
		path, err := writeSecret(dir, "my_token", "abc123")
		require.NoError(t, err)
		require.Equal(t, filepath.Join(dir, "my_token"), path)
		got, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, "abc123", string(got))
	})

	t.Run("existing file with matching content is skipped (trim-aware)", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "chirpstack_api_token")

		// Pre-populate the file as the bootstrap script would (with trailing newline).
		require.NoError(t, os.WriteFile(filePath, []byte("  abc  \n"), 0o600))

		// Capture mtime before the call.
		info1, err := os.Stat(filePath)
		require.NoError(t, err)

		// Operator pastes "abc" (trimmed). Should match without writing.
		path, err := writeSecret(dir, "chirpstack_api_token", "abc")
		require.NoError(t, err)
		require.Equal(t, filePath, path)

		// Verify file was NOT rewritten: mtime must be unchanged.
		info2, err := os.Stat(filePath)
		require.NoError(t, err)
		require.Equal(t, info1.ModTime(), info2.ModTime(), "mtime changed — writeSecret wrote the file when it should have skipped")

		// Content still original (not overwritten with the trimmed value).
		got, err := os.ReadFile(filePath)
		require.NoError(t, err)
		require.Equal(t, "  abc  \n", string(got))
	})

	t.Run("existing file with different content returns error", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "chirpstack_api_token")

		require.NoError(t, os.WriteFile(filePath, []byte("old_token"), 0o600))

		_, err := writeSecret(dir, "chirpstack_api_token", "new_token")
		require.Error(t, err)
		require.Contains(t, err.Error(), "different content")
	})
}
