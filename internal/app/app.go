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

	"github.com/chaptersix/temporal-ts-net/internal/tailscale"
)

const usage = `Temporal CLI extension: temporal ts-net

Usage:
  temporal ts-net [flags passed to temporal server start-dev]

Extension flags:
  --tailscale / --tsnet                  Expose the dev server on a Tailscale tailnet.
  --tailscale-hostname / --tsnet-hostname VALUE
                                         Tailnet hostname. Default: temporal-dev.
  --tailscale-authkey / --tsnet-authkey VALUE
                                         Tailscale auth key (or TS_AUTHKEY).
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

	serverCfg, err := ParseServerConfig(passThrough)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "invalid server arguments: %v\n", err)
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

	var tailnetSrv *tailscale.Server
	if extOpts.Tailscale {
		authKey := extOpts.TailscaleAuthKey
		if authKey == "" {
			authKey = os.Getenv("TS_AUTHKEY")
		}

		uiAddr := ""
		if !serverCfg.Headless {
			uiAddr = fmt.Sprintf("%s:%d", serverCfg.EffectiveUIIP, serverCfg.UIPort)
		}

		tailnetSrv, err = tailscale.Start(ctx, tailscale.Options{
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
	time.Sleep(250 * time.Millisecond)
	_ = cmd.Process.Kill()
	return cmd.Wait()
}
