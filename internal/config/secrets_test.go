package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadSecret_DirectEnv(t *testing.T) {
	t.Setenv("SHIFTER_TEST_SECRET", "value")
	v, err := ReadSecret("SHIFTER_TEST_SECRET")
	require.NoError(t, err)
	require.Equal(t, "value", v)
}

func TestReadSecret_FileEnv(t *testing.T) {
	p := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(p, []byte("value\n"), 0o600))
	t.Setenv("SHIFTER_TEST_SECRET_FILE", p)
	v, err := ReadSecret("SHIFTER_TEST_SECRET")
	require.NoError(t, err)
	require.Equal(t, "value", v)
}

func TestReadSecret_CRLF(t *testing.T) {
	// PITFALL #8: Windows-edited secret files have CRLF line endings.
	p := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(p, []byte("value\r\n"), 0o600))
	t.Setenv("SHIFTER_TEST_SECRET_FILE", p)
	v, err := ReadSecret("SHIFTER_TEST_SECRET")
	require.NoError(t, err)
	require.Equal(t, "value", v, "PITFALL #8: must strip \\r\\n")
}

func TestReadSecret_Missing(t *testing.T) {
	// Use an unlikely env name to avoid collisions with the host shell.
	_, err := ReadSecret("NONEXISTENT_VAR_FOR_TESTING_12345")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not set")
}

func TestReadSecret_FileMissing(t *testing.T) {
	t.Setenv("SHIFTER_TEST_SECRET_FILE", filepath.Join(t.TempDir(), "does-not-exist"))
	_, err := ReadSecret("SHIFTER_TEST_SECRET")
	require.Error(t, err)
	require.Contains(t, err.Error(), "read SHIFTER_TEST_SECRET_FILE")
}

func TestReadSecretOrEmpty_NotSet(t *testing.T) {
	v, err := ReadSecretOrEmpty("ANOTHER_NONEXISTENT_VAR_67890")
	require.NoError(t, err)
	require.Equal(t, "", v)
}

func TestReadSecretOrEmpty_DirectEnv(t *testing.T) {
	t.Setenv("SHIFTER_TEST_OPT", "v")
	v, err := ReadSecretOrEmpty("SHIFTER_TEST_OPT")
	require.NoError(t, err)
	require.Equal(t, "v", v)
}

func TestReadSecretOrEmpty_FileUnreadable(t *testing.T) {
	// Different from "not set": file path is set but the file is unreadable.
	// Should propagate, not silently empty.
	t.Setenv("SHIFTER_TEST_OPT_FILE", filepath.Join(t.TempDir(), "missing"))
	_, err := ReadSecretOrEmpty("SHIFTER_TEST_OPT")
	require.Error(t, err)
}
