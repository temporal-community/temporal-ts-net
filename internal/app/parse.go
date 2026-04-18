package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	defaultServerIP       = "localhost"
	defaultServerPort     = 7233
	defaultMaxConnections = 1000
	defaultConnectionRate = 100.0
	defaultDialTimeout    = 10 * time.Second
	defaultIdleTimeout    = 5 * time.Minute
)

type ExtensionOptions struct {
	Help                bool
	ConfigPath          string
	TailscaleHostname   string
	TailscaleAuthKey    string
	TailscaleStateDir   string
	MaxConnections      int
	ConnectionRateLimit float64
	DialTimeout         time.Duration
	IdleTimeout         time.Duration
	flagsSet            map[string]bool
}

// IsSet returns true if the named flag was explicitly provided on the command line.
func (o *ExtensionOptions) IsSet(flag string) bool {
	return o.flagsSet[flag]
}

type ServerConfig struct {
	IP                  string
	UIIP                string
	Port                int
	UIPort              int
	Headless            bool
	EffectiveFrontendIP string
	EffectiveUIIP       string
}

func ParseExtensionArgs(args []string) (ExtensionOptions, []string, error) {
	// The extension system injects "start-dev" as args[0]; strip it before forwarding.
	if len(args) > 0 && args[0] == "start-dev" {
		args = args[1:]
	}
	opts := ExtensionOptions{
		TailscaleHostname:   "temporal-dev",
		MaxConnections:      defaultMaxConnections,
		ConnectionRateLimit: defaultConnectionRate,
		DialTimeout:         defaultDialTimeout,
		IdleTimeout:         defaultIdleTimeout,
		flagsSet:            make(map[string]bool),
	}
	passThrough := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			passThrough = append(passThrough, args[i:]...)
			break
		}

		switch {
		case arg == "-h" || arg == "--help":
			opts.Help = true
		case arg == "--config":
			if i+1 >= len(args) {
				return opts, nil, fmt.Errorf("missing value for --config")
			}
			i++
			opts.ConfigPath = args[i]
		case strings.HasPrefix(arg, "--config="):
			opts.ConfigPath = arg[len("--config="):]
		case arg == "--tailscale-hostname":
			if i+1 >= len(args) {
				return opts, nil, fmt.Errorf("missing value for --tailscale-hostname")
			}
			i++
			opts.TailscaleHostname = args[i]
			opts.flagsSet["tailscale-hostname"] = true
		case arg == "--tsnet-hostname":
			if i+1 >= len(args) {
				return opts, nil, fmt.Errorf("missing value for --tsnet-hostname")
			}
			i++
			opts.TailscaleHostname = args[i]
			opts.flagsSet["tailscale-hostname"] = true
		case strings.HasPrefix(arg, "--tailscale-hostname="):
			opts.TailscaleHostname = arg[len("--tailscale-hostname="):]
			opts.flagsSet["tailscale-hostname"] = true
		case strings.HasPrefix(arg, "--tsnet-hostname="):
			opts.TailscaleHostname = arg[len("--tsnet-hostname="):]
			opts.flagsSet["tailscale-hostname"] = true
		case arg == "--tailscale-authkey":
			if i+1 >= len(args) {
				return opts, nil, fmt.Errorf("missing value for --tailscale-authkey")
			}
			i++
			opts.TailscaleAuthKey = args[i]
			opts.flagsSet["tailscale-authkey"] = true
		case arg == "--tsnet-authkey":
			if i+1 >= len(args) {
				return opts, nil, fmt.Errorf("missing value for --tsnet-authkey")
			}
			i++
			opts.TailscaleAuthKey = args[i]
			opts.flagsSet["tailscale-authkey"] = true
		case strings.HasPrefix(arg, "--tailscale-authkey="):
			opts.TailscaleAuthKey = arg[len("--tailscale-authkey="):]
			opts.flagsSet["tailscale-authkey"] = true
		case strings.HasPrefix(arg, "--tsnet-authkey="):
			opts.TailscaleAuthKey = arg[len("--tsnet-authkey="):]
			opts.flagsSet["tailscale-authkey"] = true
		case arg == "--tailscale-state-dir":
			if i+1 >= len(args) {
				return opts, nil, fmt.Errorf("missing value for --tailscale-state-dir")
			}
			i++
			opts.TailscaleStateDir = args[i]
			opts.flagsSet["tailscale-state-dir"] = true
		case arg == "--tsnet-state-dir":
			if i+1 >= len(args) {
				return opts, nil, fmt.Errorf("missing value for --tsnet-state-dir")
			}
			i++
			opts.TailscaleStateDir = args[i]
			opts.flagsSet["tailscale-state-dir"] = true
		case strings.HasPrefix(arg, "--tailscale-state-dir="):
			opts.TailscaleStateDir = arg[len("--tailscale-state-dir="):]
			opts.flagsSet["tailscale-state-dir"] = true
		case strings.HasPrefix(arg, "--tsnet-state-dir="):
			opts.TailscaleStateDir = arg[len("--tsnet-state-dir="):]
			opts.flagsSet["tailscale-state-dir"] = true
		case arg == "--max-connections":
			if i+1 >= len(args) {
				return opts, nil, fmt.Errorf("missing value for --max-connections")
			}
			i++
			val, err := strconv.Atoi(args[i])
			if err != nil {
				return opts, nil, fmt.Errorf("invalid --max-connections value: %w", err)
			}
			opts.MaxConnections = val
			opts.flagsSet["max-connections"] = true
		case strings.HasPrefix(arg, "--max-connections="):
			val, err := strconv.Atoi(arg[len("--max-connections="):])
			if err != nil {
				return opts, nil, fmt.Errorf("invalid --max-connections value: %w", err)
			}
			opts.MaxConnections = val
			opts.flagsSet["max-connections"] = true
		case arg == "--connection-rate-limit":
			if i+1 >= len(args) {
				return opts, nil, fmt.Errorf("missing value for --connection-rate-limit")
			}
			i++
			val, err := strconv.ParseFloat(args[i], 64)
			if err != nil {
				return opts, nil, fmt.Errorf("invalid --connection-rate-limit value: %w", err)
			}
			opts.ConnectionRateLimit = val
			opts.flagsSet["connection-rate-limit"] = true
		case strings.HasPrefix(arg, "--connection-rate-limit="):
			val, err := strconv.ParseFloat(arg[len("--connection-rate-limit="):], 64)
			if err != nil {
				return opts, nil, fmt.Errorf("invalid --connection-rate-limit value: %w", err)
			}
			opts.ConnectionRateLimit = val
			opts.flagsSet["connection-rate-limit"] = true
		case arg == "--dial-timeout":
			if i+1 >= len(args) {
				return opts, nil, fmt.Errorf("missing value for --dial-timeout")
			}
			i++
			val, err := time.ParseDuration(args[i])
			if err != nil {
				return opts, nil, fmt.Errorf("invalid --dial-timeout value: %w", err)
			}
			opts.DialTimeout = val
			opts.flagsSet["dial-timeout"] = true
		case strings.HasPrefix(arg, "--dial-timeout="):
			val, err := time.ParseDuration(arg[len("--dial-timeout="):])
			if err != nil {
				return opts, nil, fmt.Errorf("invalid --dial-timeout value: %w", err)
			}
			opts.DialTimeout = val
			opts.flagsSet["dial-timeout"] = true
		case arg == "--idle-timeout":
			if i+1 >= len(args) {
				return opts, nil, fmt.Errorf("missing value for --idle-timeout")
			}
			i++
			val, err := time.ParseDuration(args[i])
			if err != nil {
				return opts, nil, fmt.Errorf("invalid --idle-timeout value: %w", err)
			}
			opts.IdleTimeout = val
			opts.flagsSet["idle-timeout"] = true
		case strings.HasPrefix(arg, "--idle-timeout="):
			val, err := time.ParseDuration(arg[len("--idle-timeout="):])
			if err != nil {
				return opts, nil, fmt.Errorf("invalid --idle-timeout value: %w", err)
			}
			opts.IdleTimeout = val
			opts.flagsSet["idle-timeout"] = true
		default:
			passThrough = append(passThrough, arg)
		}
	}

	if opts.TailscaleHostname == "" {
		return opts, nil, fmt.Errorf("--tailscale-hostname cannot be empty")
	}

	return opts, passThrough, nil
}

func ParseServerConfig(args []string) (ServerConfig, error) {
	cfg := ServerConfig{
		IP:   defaultServerIP,
		Port: defaultServerPort,
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}

		switch {
		case arg == "--headless":
			cfg.Headless = true
		case strings.HasPrefix(arg, "--headless="):
			v, err := parseBool(arg[len("--headless="):])
			if err != nil {
				return cfg, fmt.Errorf("invalid --headless value: %w", err)
			}
			cfg.Headless = v

		case arg == "--ip":
			val, ok := nextArgValue(args, &i)
			if !ok {
				return cfg, fmt.Errorf("missing value for --ip")
			}
			cfg.IP = val
		case strings.HasPrefix(arg, "--ip="):
			cfg.IP = arg[len("--ip="):]

		case arg == "--ui-ip":
			val, ok := nextArgValue(args, &i)
			if !ok {
				return cfg, fmt.Errorf("missing value for --ui-ip")
			}
			cfg.UIIP = val
		case strings.HasPrefix(arg, "--ui-ip="):
			cfg.UIIP = arg[len("--ui-ip="):]

		case arg == "--port":
			val, ok := nextArgValue(args, &i)
			if !ok {
				return cfg, fmt.Errorf("missing value for --port")
			}
			port, err := strconv.Atoi(val)
			if err != nil {
				return cfg, fmt.Errorf("invalid --port value %q", val)
			}
			cfg.Port = port
		case strings.HasPrefix(arg, "--port="):
			val := arg[len("--port="):]
			port, err := strconv.Atoi(val)
			if err != nil {
				return cfg, fmt.Errorf("invalid --port value %q", val)
			}
			cfg.Port = port

		case arg == "-p":
			val, ok := nextArgValue(args, &i)
			if !ok {
				return cfg, fmt.Errorf("missing value for -p")
			}
			port, err := strconv.Atoi(val)
			if err != nil {
				return cfg, fmt.Errorf("invalid -p value %q", val)
			}
			cfg.Port = port
		case strings.HasPrefix(arg, "-p="):
			val := arg[len("-p="):]
			port, err := strconv.Atoi(val)
			if err != nil {
				return cfg, fmt.Errorf("invalid -p value %q", val)
			}
			cfg.Port = port
		case strings.HasPrefix(arg, "-p") && len(arg) > 2:
			val := arg[len("-p"):]
			port, err := strconv.Atoi(val)
			if err != nil {
				return cfg, fmt.Errorf("invalid -p value %q", val)
			}
			cfg.Port = port

		case arg == "--ui-port":
			val, ok := nextArgValue(args, &i)
			if !ok {
				return cfg, fmt.Errorf("missing value for --ui-port")
			}
			port, err := strconv.Atoi(val)
			if err != nil {
				return cfg, fmt.Errorf("invalid --ui-port value %q", val)
			}
			cfg.UIPort = port
		case strings.HasPrefix(arg, "--ui-port="):
			val := arg[len("--ui-port="):]
			port, err := strconv.Atoi(val)
			if err != nil {
				return cfg, fmt.Errorf("invalid --ui-port value %q", val)
			}
			cfg.UIPort = port

		}
	}

	if cfg.UIIP == "" {
		cfg.UIIP = cfg.IP
	}
	if cfg.UIPort == 0 && !cfg.Headless {
		cfg.UIPort = min(cfg.Port+1000, 65535)
	}

	cfg.EffectiveFrontendIP = normalizeDialHost(cfg.IP)
	cfg.EffectiveUIIP = normalizeDialHost(cfg.UIIP)

	return cfg, nil
}

func normalizeDialHost(host string) string {
	if host == "" || host == "localhost" {
		return "127.0.0.1"
	}
	return host
}

func nextArgValue(args []string, idx *int) (string, bool) {
	if *idx+1 >= len(args) {
		return "", false
	}
	*idx = *idx + 1
	return args[*idx], true
}

func parseBool(s string) (bool, error) {
	v, err := strconv.ParseBool(s)
	if err != nil {
		return false, fmt.Errorf("%q is not a boolean", s)
	}
	return v, nil
}
