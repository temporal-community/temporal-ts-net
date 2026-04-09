// ABOUTME: Tests for config file loading and merging with CLI flags.
// Covers LoadConfig, ResolveConfigPath, ApplyFileConfig, and precedence.

package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	t.Run("missing file returns zero config when not explicit", func(t *testing.T) {
		cfg, err := LoadConfig("/nonexistent/temporal.toml", false)
		require.NoError(t, err)
		require.Equal(t, FileConfig{}, cfg)
	})

	t.Run("missing file errors when explicit", func(t *testing.T) {
		_, err := LoadConfig("/nonexistent/temporal.toml", true)
		require.Error(t, err)
	})

	t.Run("parses ts-net section from temporal config", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "temporal.toml")
		content := `
[profile.default]
address = "localhost:7233"

[ts-net]
tailscale-hostname = "my-dev"
tailscale-authkey = "tskey-auth-test"
tailscale-state-dir = "/tmp/tsnet"
max-connections = 500
connection-rate-limit = 50.0
dial-timeout = "15s"
idle-timeout = "10m"
`
		require.NoError(t, os.WriteFile(path, []byte(content), 0600))

		cfg, err := LoadConfig(path, false)
		require.NoError(t, err)
		require.Equal(t, "my-dev", cfg.TailscaleHostname)
		require.Equal(t, "tskey-auth-test", cfg.TailscaleAuthKey)
		require.Equal(t, "/tmp/tsnet", cfg.TailscaleStateDir)
		require.NotNil(t, cfg.MaxConnections)
		require.Equal(t, 500, *cfg.MaxConnections)
		require.NotNil(t, cfg.ConnectionRateLimit)
		require.Equal(t, 50.0, *cfg.ConnectionRateLimit)
		require.Equal(t, "15s", cfg.DialTimeout)
		require.Equal(t, "10m", cfg.IdleTimeout)
	})

	t.Run("ignores unknown sections", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "temporal.toml")
		content := `
[profile.default]
address = "localhost:7233"
namespace = "prod"

[ts-net]
tailscale-hostname = "box"
`
		require.NoError(t, os.WriteFile(path, []byte(content), 0600))

		cfg, err := LoadConfig(path, false)
		require.NoError(t, err)
		require.Equal(t, "box", cfg.TailscaleHostname)
	})

	t.Run("partial config leaves other fields zero", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "temporal.toml")
		content := `
[ts-net]
tailscale-hostname = "partial"
`
		require.NoError(t, os.WriteFile(path, []byte(content), 0600))

		cfg, err := LoadConfig(path, false)
		require.NoError(t, err)
		require.Equal(t, "partial", cfg.TailscaleHostname)
		require.Empty(t, cfg.TailscaleAuthKey)
		require.Nil(t, cfg.MaxConnections)
		require.Empty(t, cfg.DialTimeout)
	})

	t.Run("no ts-net section returns zero config", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "temporal.toml")
		content := `
[profile.default]
address = "localhost:7233"
`
		require.NoError(t, os.WriteFile(path, []byte(content), 0600))

		cfg, err := LoadConfig(path, false)
		require.NoError(t, err)
		require.Equal(t, FileConfig{}, cfg)
	})

	t.Run("invalid duration errors", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "temporal.toml")
		content := `
[ts-net]
dial-timeout = "not-a-duration"
`
		require.NoError(t, os.WriteFile(path, []byte(content), 0600))

		_, err := LoadConfig(path, false)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid dial-timeout")
	})

	t.Run("invalid toml errors", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "temporal.toml")
		require.NoError(t, os.WriteFile(path, []byte("not valid [[ toml"), 0600))

		_, err := LoadConfig(path, false)
		require.Error(t, err)
	})
}

func TestResolveConfigPath(t *testing.T) {
	t.Run("flag value takes priority", func(t *testing.T) {
		t.Setenv("TEMPORAL_CONFIG_FILE", "/env/path.toml")
		path, err := ResolveConfigPath("/flag/path.toml")
		require.NoError(t, err)
		require.Equal(t, "/flag/path.toml", path)
	})

	t.Run("env var used when no flag", func(t *testing.T) {
		t.Setenv("TEMPORAL_CONFIG_FILE", "/env/path.toml")
		path, err := ResolveConfigPath("")
		require.NoError(t, err)
		require.Equal(t, "/env/path.toml", path)
	})

	t.Run("default path when neither set", func(t *testing.T) {
		t.Setenv("TEMPORAL_CONFIG_FILE", "")
		path, err := ResolveConfigPath("")
		require.NoError(t, err)
		require.Contains(t, path, "temporalio")
		require.Contains(t, path, "temporal.toml")
	})
}

func TestApplyFileConfig(t *testing.T) {
	t.Run("config values fill unset flags", func(t *testing.T) {
		maxConn := 500
		rateLimit := 50.0
		cfg := FileConfig{
			TailscaleHostname:   "from-config",
			TailscaleAuthKey:    "tskey-from-config",
			TailscaleStateDir:   "/config/state",
			MaxConnections:      &maxConn,
			ConnectionRateLimit: &rateLimit,
			DialTimeout:         "15s",
			IdleTimeout:         "10m",
		}

		opts := ExtensionOptions{
			TailscaleHostname:   "temporal-dev",
			MaxConnections:      defaultMaxConnections,
			ConnectionRateLimit: defaultConnectionRate,
			DialTimeout:         defaultDialTimeout,
			IdleTimeout:         defaultIdleTimeout,
			flagsSet:            make(map[string]bool),
		}

		ApplyFileConfig(&opts, cfg)

		require.Equal(t, "from-config", opts.TailscaleHostname)
		require.Equal(t, "tskey-from-config", opts.TailscaleAuthKey)
		require.Equal(t, "/config/state", opts.TailscaleStateDir)
		require.Equal(t, 500, opts.MaxConnections)
		require.Equal(t, 50.0, opts.ConnectionRateLimit)
		require.Equal(t, 15*time.Second, opts.DialTimeout)
		require.Equal(t, 10*time.Minute, opts.IdleTimeout)
	})

	t.Run("cli flags override config values", func(t *testing.T) {
		maxConn := 500
		cfg := FileConfig{
			TailscaleHostname: "from-config",
			MaxConnections:    &maxConn,
			DialTimeout:       "15s",
		}

		opts := ExtensionOptions{
			TailscaleHostname: "from-flag",
			MaxConnections:    2000,
			DialTimeout:       30 * time.Second,
			flagsSet: map[string]bool{
				"tailscale-hostname": true,
				"max-connections":    true,
				"dial-timeout":       true,
			},
		}

		ApplyFileConfig(&opts, cfg)

		require.Equal(t, "from-flag", opts.TailscaleHostname)
		require.Equal(t, 2000, opts.MaxConnections)
		require.Equal(t, 30*time.Second, opts.DialTimeout)
	})

	t.Run("nil pointer fields leave defaults", func(t *testing.T) {
		cfg := FileConfig{
			TailscaleHostname: "from-config",
		}

		opts := ExtensionOptions{
			TailscaleHostname:   "temporal-dev",
			MaxConnections:      defaultMaxConnections,
			ConnectionRateLimit: defaultConnectionRate,
			flagsSet:            make(map[string]bool),
		}

		ApplyFileConfig(&opts, cfg)

		require.Equal(t, "from-config", opts.TailscaleHostname)
		require.Equal(t, defaultMaxConnections, opts.MaxConnections)
		require.Equal(t, defaultConnectionRate, opts.ConnectionRateLimit)
	})
}

func TestParseExtensionArgs_Config(t *testing.T) {
	t.Run("parses --config flag", func(t *testing.T) {
		opts, _, err := ParseExtensionArgs([]string{"--config", "/path/to/config.toml"})
		require.NoError(t, err)
		require.Equal(t, "/path/to/config.toml", opts.ConfigPath)
	})

	t.Run("parses --config= flag", func(t *testing.T) {
		opts, _, err := ParseExtensionArgs([]string{"--config=/path/to/config.toml"})
		require.NoError(t, err)
		require.Equal(t, "/path/to/config.toml", opts.ConfigPath)
	})

	t.Run("IsSet true for explicit flags", func(t *testing.T) {
		opts, _, err := ParseExtensionArgs([]string{
			"--tailscale-hostname", "explicit",
			"--max-connections", "100",
		})
		require.NoError(t, err)
		require.True(t, opts.IsSet("tailscale-hostname"))
		require.True(t, opts.IsSet("max-connections"))
		require.False(t, opts.IsSet("tailscale-authkey"))
		require.False(t, opts.IsSet("dial-timeout"))
	})
}

func TestConfigPrecedence(t *testing.T) {
	t.Run("cli flag beats config file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "temporal.toml")
		content := `
[ts-net]
tailscale-hostname = "from-config"
max-connections = 500
`
		require.NoError(t, os.WriteFile(path, []byte(content), 0600))

		opts, _, err := ParseExtensionArgs([]string{
			"--config", path,
			"--tailscale-hostname", "from-flag",
		})
		require.NoError(t, err)

		cfg, err := LoadConfig(path, true)
		require.NoError(t, err)
		ApplyFileConfig(&opts, cfg)

		require.Equal(t, "from-flag", opts.TailscaleHostname)
		require.Equal(t, 500, opts.MaxConnections)
	})

	t.Run("TS_AUTHKEY env var beats config file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "temporal.toml")
		content := `
[ts-net]
tailscale-authkey = "from-config"
`
		require.NoError(t, os.WriteFile(path, []byte(content), 0600))
		t.Setenv("TS_AUTHKEY", "from-env")

		opts, _, err := ParseExtensionArgs([]string{"--config", path})
		require.NoError(t, err)

		cfg, err := LoadConfig(path, true)
		require.NoError(t, err)
		ApplyFileConfig(&opts, cfg)

		// Replicate the logic from Run(): env var overrides config when flag not set.
		authKey := opts.TailscaleAuthKey
		if !opts.IsSet("tailscale-authkey") {
			if envKey := os.Getenv("TS_AUTHKEY"); envKey != "" {
				authKey = envKey
			}
		}
		require.Equal(t, "from-env", authKey)
	})

	t.Run("cli flag beats TS_AUTHKEY env var", func(t *testing.T) {
		t.Setenv("TS_AUTHKEY", "from-env")

		opts, _, err := ParseExtensionArgs([]string{
			"--tailscale-authkey", "from-flag",
		})
		require.NoError(t, err)

		authKey := opts.TailscaleAuthKey
		if !opts.IsSet("tailscale-authkey") {
			if envKey := os.Getenv("TS_AUTHKEY"); envKey != "" {
				authKey = envKey
			}
		}
		require.Equal(t, "from-flag", authKey)
	})
}
