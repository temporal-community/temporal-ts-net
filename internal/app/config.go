// ABOUTME: Loads extension configuration from the Temporal CLI config file.
// Reads the [ts-net] section from ~/.config/temporalio/temporal.toml.

package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

// FileConfig represents the [ts-net] section in the Temporal CLI config file.
// Pointer types for numeric fields distinguish "not set" from "set to zero."
type FileConfig struct {
	TailscaleHostname   string   `toml:"tailscale-hostname"`
	TailscaleAuthKey    string   `toml:"tailscale-authkey"`
	TailscaleStateDir   string   `toml:"tailscale-state-dir"`
	MaxConnections      *int     `toml:"max-connections"`
	ConnectionRateLimit *float64 `toml:"connection-rate-limit"`
	DialTimeout         string   `toml:"dial-timeout"`
	IdleTimeout         string   `toml:"idle-timeout"`
}

// temporalConfig is the top-level structure used to extract only the [ts-net]
// section from the Temporal CLI config file. All other sections are ignored.
type temporalConfig struct {
	TsNet FileConfig `toml:"ts-net"`
}

// DefaultConfigPath returns the default Temporal CLI config file path,
// matching the CLI's own default: ~/.config/temporalio/temporal.toml.
func DefaultConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("determine config directory: %w", err)
	}
	return filepath.Join(configDir, "temporalio", "temporal.toml"), nil
}

// ResolveConfigPath returns the config file path from, in order:
//  1. The explicit --config flag value
//  2. The TEMPORAL_CONFIG_FILE environment variable
//  3. The default path
func ResolveConfigPath(flagValue string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	if envPath := os.Getenv("TEMPORAL_CONFIG_FILE"); envPath != "" {
		return envPath, nil
	}
	return DefaultConfigPath()
}

// LoadConfig reads the [ts-net] section from a Temporal CLI config file.
// Returns a zero FileConfig and nil error if the file does not exist and was
// not explicitly requested. Returns an error if the file was explicitly
// specified (via --config flag or env var) and is missing.
func LoadConfig(path string, explicit bool) (FileConfig, error) {
	var cfg temporalConfig

	_, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && !explicit {
			return FileConfig{}, nil
		}
		return FileConfig{}, fmt.Errorf("load config %s: %w", path, err)
	}

	if cfg.TsNet.DialTimeout != "" {
		if _, err := time.ParseDuration(cfg.TsNet.DialTimeout); err != nil {
			return FileConfig{}, fmt.Errorf("invalid dial-timeout %q in %s: %w", cfg.TsNet.DialTimeout, path, err)
		}
	}
	if cfg.TsNet.IdleTimeout != "" {
		if _, err := time.ParseDuration(cfg.TsNet.IdleTimeout); err != nil {
			return FileConfig{}, fmt.Errorf("invalid idle-timeout %q in %s: %w", cfg.TsNet.IdleTimeout, path, err)
		}
	}

	return cfg.TsNet, nil
}

// ApplyFileConfig merges config file values into opts for any field that was
// not explicitly set via CLI flags. Duration strings must already be validated
// by LoadConfig.
func ApplyFileConfig(opts *ExtensionOptions, cfg FileConfig) {
	if !opts.IsSet("tailscale-hostname") && cfg.TailscaleHostname != "" {
		opts.TailscaleHostname = cfg.TailscaleHostname
	}
	if !opts.IsSet("tailscale-authkey") && cfg.TailscaleAuthKey != "" {
		opts.TailscaleAuthKey = cfg.TailscaleAuthKey
	}
	if !opts.IsSet("tailscale-state-dir") && cfg.TailscaleStateDir != "" {
		opts.TailscaleStateDir = cfg.TailscaleStateDir
	}
	if !opts.IsSet("max-connections") && cfg.MaxConnections != nil {
		opts.MaxConnections = *cfg.MaxConnections
	}
	if !opts.IsSet("connection-rate-limit") && cfg.ConnectionRateLimit != nil {
		opts.ConnectionRateLimit = *cfg.ConnectionRateLimit
	}
	if !opts.IsSet("dial-timeout") && cfg.DialTimeout != "" {
		opts.DialTimeout, _ = time.ParseDuration(cfg.DialTimeout)
	}
	if !opts.IsSet("idle-timeout") && cfg.IdleTimeout != "" {
		opts.IdleTimeout, _ = time.ParseDuration(cfg.IdleTimeout)
	}
}
