package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/temporal-community/temporal-ts-net/internal/tailscale"
)

const usage = `Temporal CLI extension: temporal ts-net

Runs temporal server start-dev and exposes it on your Tailscale tailnet.

Usage:
  temporal ts-net [flags passed to temporal server start-dev]

Extension flags:
  --config PATH                          Path to config file (default: ~/.config/temporalio/temporal.toml,
                                         or TEMPORAL_CONFIG_FILE env var). Reads the [ts-net] section.
  --tailscale-hostname / --tsnet-hostname VALUE
                                         Tailnet hostname. Default: temporal-dev.
  --tailscale-authkey / --tsnet-authkey VALUE
                                         Tailscale auth key (or TS_AUTHKEY env var).
  --tailscale-state-dir / --tsnet-state-dir VALUE
                                         Directory for tsnet state.
  --max-connections VALUE                Maximum concurrent connections. Default: 1000.
  --connection-rate-limit VALUE          Maximum connections per second. Default: 100.
  --dial-timeout VALUE                   Timeout for dialing backend (e.g., 10s). Default: 10s.
  --idle-timeout VALUE                   Idle timeout for proxy connections (e.g., 5m). Default: 5m.
  -h, --help                             Show this help text.

All other flags are forwarded to:
  temporal server start-dev
`

func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	extOpts, passThrough, err := ParseExtensionArgs(args)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%v\n\n%s", err, usage)
		return 2
	}
	if extOpts.Help {
		_, _ = io.WriteString(stdout, usage)
		return 0
	}

	// Load and apply config file. Precedence: CLI flags > env vars > config file > defaults.
	configPath, err := ResolveConfigPath(extOpts.ConfigPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "failed to resolve config path: %v\n", err)
		return 2
	}
	fileCfg, err := LoadConfig(configPath, extOpts.ConfigPath != "")
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%v\n", err)
		return 2
	}
	ApplyFileConfig(&extOpts, fileCfg)

	serverCfg, err := ParseServerConfig(passThrough)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "invalid server arguments: %v\n", err)
		return 2
	}

	// Determine auth key before starting anything
	authKey := resolveAuthKey(&extOpts)

	// Validate that we can start tsnet before starting the Temporal server
	stateDir := extOpts.TailscaleStateDir
	if stateDir == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "failed to determine config directory: %v\n", err)
			return 2
		}
		stateDir = fmt.Sprintf("%s/tsnet-temporal-ts-net", configDir)
	}
	if err := validateTSNetAuth(authKey, stateDir); err != nil {
		_, _ = fmt.Fprintf(stderr, "%v\n", err)
		return 2
	}

	childArgs := passThrough

	cmd := exec.Command("temporal", append([]string{"server", "start-dev"}, childArgs...)...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	ctx, stopSignal := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignal()

	if err := cmd.Start(); err != nil {
		_, _ = fmt.Fprintf(stderr, "failed to start temporal CLI: %v\n", err)
		return 1
	}

	uiAddr := ""
	if !serverCfg.Headless {
		uiAddr = fmt.Sprintf("%s:%d", serverCfg.EffectiveUIIP, serverCfg.UIPort)
	}

	tailnetSrv, err := tailscale.Start(ctx, tailscale.Options{
		Hostname:            extOpts.TailscaleHostname,
		AuthKey:             authKey,
		StateDir:            extOpts.TailscaleStateDir,
		FrontendAddr:        fmt.Sprintf("%s:%d", serverCfg.EffectiveFrontendIP, serverCfg.Port),
		FrontendPort:        serverCfg.Port,
		UIAddr:              uiAddr,
		UIPort:              serverCfg.UIPort,
		Logger:              slog.New(slog.NewTextHandler(stderr, nil)),
		MaxConnections:      extOpts.MaxConnections,
		ConnectionRateLimit: extOpts.ConnectionRateLimit,
		DialTimeout:         extOpts.DialTimeout,
		IdleTimeout:         extOpts.IdleTimeout,
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "failed to start tailscale proxy: %v\n", err)
		_ = interruptAndWait(cmd)
		return 1
	}
	defer tailnetSrv.Stop()

	_, _ = fmt.Fprintf(stdout, "Tailnet gRPC: %s:%d\n", tailnetSrv.Hostname, serverCfg.Port)
	if !serverCfg.Headless {
		_, _ = fmt.Fprintf(stdout, "Tailnet UI:   http://%s:%d\n", tailnetSrv.Hostname, serverCfg.UIPort)
	}

	err = waitCommand(ctx, cmd)
	if err == nil {
		return 0
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}

	_, _ = fmt.Fprintf(stderr, "command failed: %v\n", err)
	return 1
}

func waitCommand(ctx context.Context, cmd *exec.Cmd) error {
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		if cmd.Process != nil {
			_ = cmd.Process.Signal(os.Interrupt)
		}
		select {
		case err := <-done:
			return err
		case <-time.After(5 * time.Second):
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			return <-done
		}
	}
}

func interruptAndWait(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	_ = cmd.Process.Signal(os.Interrupt)

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		return <-done
	}
}

// resolveAuthKey determines the auth key using precedence:
// CLI flag > TS_AUTHKEY env var > (config file is already in opts)
func resolveAuthKey(opts *ExtensionOptions) string {
	if opts.IsSet("tailscale-authkey") {
		return opts.TailscaleAuthKey
	}
	if envKey := os.Getenv("TS_AUTHKEY"); envKey != "" {
		return envKey
	}
	return opts.TailscaleAuthKey // from config file or empty
}

// validateTSNetAuth checks that tsnet can authenticate either via auth key or existing state
func validateTSNetAuth(authKey, stateDir string) error {
	// If auth key is provided, we're good
	if authKey != "" {
		return nil
	}

	// Check if existing state file exists (node already authenticated)
	stateFile := fmt.Sprintf("%s/tailscaled.state", stateDir)
	if _, err := os.Stat(stateFile); err == nil {
		return nil
	}

	// No auth key and no existing state
	return fmt.Errorf("tsnet authentication required: provide --tsnet-authkey or set TS_AUTHKEY environment variable\n" +
		"Generate an auth key at: https://login.tailscale.com/admin/settings/keys")
}
