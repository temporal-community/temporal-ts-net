package app

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseExtensionArgs(t *testing.T) {
	t.Run("extracts extension flags and preserves pass-through", func(t *testing.T) {
		opts, pass, err := ParseExtensionArgs([]string{
			"--tailscale-hostname", "devbox",
			"--tailscale-authkey=tskey-auth-foo",
			"--port", "7239",
			"--headless",
		})
		require.NoError(t, err)
		require.Equal(t, "devbox", opts.TailscaleHostname)
		require.Equal(t, "tskey-auth-foo", opts.TailscaleAuthKey)
		require.Equal(t, []string{"--port", "7239", "--headless"}, pass)
	})

	t.Run("defaults hostname", func(t *testing.T) {
		opts, pass, err := ParseExtensionArgs([]string{"--port", "7240"})
		require.NoError(t, err)
		require.Equal(t, "temporal-dev", opts.TailscaleHostname)
		require.Equal(t, []string{"--port", "7240"}, pass)
	})

	t.Run("supports tsnet aliases", func(t *testing.T) {
		opts, pass, err := ParseExtensionArgs([]string{
			"--tsnet-hostname", "alias-host",
			"--tsnet-authkey", "alias-key",
			"--tsnet-state-dir", "/tmp/tsnet-state",
			"--ip", "127.0.0.1",
		})
		require.NoError(t, err)
		require.Equal(t, "alias-host", opts.TailscaleHostname)
		require.Equal(t, "alias-key", opts.TailscaleAuthKey)
		require.Equal(t, "/tmp/tsnet-state", opts.TailscaleStateDir)
		require.Equal(t, []string{"--ip", "127.0.0.1"}, pass)
	})

	t.Run("strips start-dev injected by extension system", func(t *testing.T) {
		opts, pass, err := ParseExtensionArgs([]string{"start-dev", "--port", "7240"})
		require.NoError(t, err)
		require.Equal(t, "temporal-dev", opts.TailscaleHostname)
		require.Equal(t, []string{"--port", "7240"}, pass)
	})

	t.Run("errors on missing value", func(t *testing.T) {
		_, _, err := ParseExtensionArgs([]string{"--tailscale-hostname"})
		require.Error(t, err)
	})
}

func TestParseServerConfig(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		cfg, err := ParseServerConfig(nil)
		require.NoError(t, err)
		require.Equal(t, "localhost", cfg.IP)
		require.Equal(t, 7233, cfg.Port)
		require.Equal(t, 8233, cfg.UIPort)
		require.Equal(t, "127.0.0.1", cfg.EffectiveFrontendIP)
	})

	t.Run("parses standard flags", func(t *testing.T) {
		cfg, err := ParseServerConfig([]string{
			"--ip", "0.0.0.0",
			"--port", "9000",
			"--ui-port", "9100",
			"--ui-ip", "127.0.0.1",
		})
		require.NoError(t, err)
		require.Equal(t, "0.0.0.0", cfg.IP)
		require.Equal(t, 9000, cfg.Port)
		require.Equal(t, 9100, cfg.UIPort)
		require.Equal(t, "127.0.0.1", cfg.UIIP)
	})

	t.Run("parses shorthand port", func(t *testing.T) {
		cfg, err := ParseServerConfig([]string{"-p", "8123"})
		require.NoError(t, err)
		require.Equal(t, 8123, cfg.Port)
		require.Equal(t, 9123, cfg.UIPort)
	})

	t.Run("headless keeps ui port unset", func(t *testing.T) {
		cfg, err := ParseServerConfig([]string{"--headless"})
		require.NoError(t, err)
		require.True(t, cfg.Headless)
		require.Equal(t, 0, cfg.UIPort)
	})
}
