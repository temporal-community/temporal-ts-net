package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAuthKeyPrecedence verifies that auth key precedence is:
// CLI flag > TS_AUTHKEY env var > config file > none
func TestAuthKeyPrecedence(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a config file with an auth key
	configFile := filepath.Join(tmpDir, "temporal.toml")
	configContent := `[ts-net]
tailscale-authkey = "config-file-key"
`
	require.NoError(t, os.WriteFile(configFile, []byte(configContent), 0644))

	tests := []struct {
		name          string
		cliFlag       string
		envVar        string
		configFile    string
		expectAuthKey string
	}{
		{
			name:          "CLI flag beats all",
			cliFlag:       "cli-flag-key",
			envVar:        "env-var-key",
			configFile:    configFile,
			expectAuthKey: "cli-flag-key",
		},
		{
			name:          "Env var beats config file",
			cliFlag:       "",
			envVar:        "env-var-key",
			configFile:    configFile,
			expectAuthKey: "env-var-key",
		},
		{
			name:          "Config file used when no flag or env",
			cliFlag:       "",
			envVar:        "",
			configFile:    configFile,
			expectAuthKey: "config-file-key",
		},
		{
			name:          "No auth anywhere",
			cliFlag:       "",
			envVar:        "",
			configFile:    "",
			expectAuthKey: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up env var
			if tt.envVar != "" {
				os.Setenv("TS_AUTHKEY", tt.envVar)
				defer os.Unsetenv("TS_AUTHKEY")
			} else {
				os.Unsetenv("TS_AUTHKEY")
			}

			// Parse extension args
			args := []string{}
			if tt.configFile != "" {
				args = append(args, "--config", tt.configFile)
			}
			if tt.cliFlag != "" {
				args = append(args, "--tsnet-authkey", tt.cliFlag)
			}

			extOpts, _, err := ParseExtensionArgs(args)
			require.NoError(t, err)

			// Load and apply config file
			if tt.configFile != "" {
				fileCfg, err := LoadConfig(tt.configFile, true)
				require.NoError(t, err)
				ApplyFileConfig(&extOpts, fileCfg)
			}

			// Use the same auth key resolution logic as Run()
			authKey := resolveAuthKey(&extOpts)

			require.Equal(t, tt.expectAuthKey, authKey)
		})
	}
}
