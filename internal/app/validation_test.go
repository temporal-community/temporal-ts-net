package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestValidateAuthRequirements_NoAuthKey tests that the app fails fast
// when no auth key is provided and no existing state exists
func TestValidateAuthRequirements_NoAuthKey(t *testing.T) {
	// Create a temp directory for state (empty, no existing auth)
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, "tsnet-state")

	// Clear TS_AUTHKEY env var if set
	oldAuthKey := os.Getenv("TS_AUTHKEY")
	os.Unsetenv("TS_AUTHKEY")
	defer func() {
		if oldAuthKey != "" {
			os.Setenv("TS_AUTHKEY", oldAuthKey)
		}
	}()

	var stdout, stderr bytes.Buffer

	// Run without auth key or existing state
	args := []string{
		"--tsnet-state-dir", stateDir,
		"--tsnet-hostname", "test-host",
	}

	exitCode := Run(args, &bytes.Buffer{}, &stdout, &stderr)

	// Should fail with exit code 2 (validation error, not 1 for runtime error)
	require.Equal(t, 2, exitCode, "expected validation failure exit code")

	// Should fail BEFORE starting temporal server
	// We can verify this by checking that "Temporal Server:" is NOT in output
	combinedOutput := stdout.String() + stderr.String()
	require.NotContains(t, combinedOutput, "Temporal Server:",
		"temporal server should not start without auth")
	require.NotContains(t, combinedOutput, "tsnet running state path",
		"tsnet should not start without auth")

	// Should show a helpful error message
	require.Contains(t, strings.ToLower(combinedOutput), "auth",
		"error message should mention authentication")
	require.Contains(t, combinedOutput, "TS_AUTHKEY",
		"error message should mention TS_AUTHKEY env var")
}

// TestValidateAuthUnit tests the validateTSNetAuth function directly
func TestValidateAuthUnit(t *testing.T) {
	tests := []struct {
		name        string
		authKey     string
		createState bool
		expectErr   bool
	}{
		{
			name:      "with auth key",
			authKey:   "tskey-test-dummy",
			expectErr: false,
		},
		{
			name:        "with existing state",
			authKey:     "",
			createState: true,
			expectErr:   false,
		},
		{
			name:      "no auth or state",
			authKey:   "",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			stateDir := filepath.Join(tmpDir, "tsnet-state")

			if tt.createState {
				require.NoError(t, os.MkdirAll(stateDir, 0755))
				stateFile := filepath.Join(stateDir, "tailscaled.state")
				require.NoError(t, os.WriteFile(stateFile, []byte("dummy"), 0600))
			}

			err := validateTSNetAuth(tt.authKey, stateDir)
			if tt.expectErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), "authentication required")
			} else {
				require.NoError(t, err)
			}
		})
	}
}
