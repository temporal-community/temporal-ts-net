# Lessons Learned

## Temporal CLI Config File Loading (2026-04-09)

### The Temporal CLI config system lives in the Go SDK, not the CLI

The TOML config parsing is in `go.temporal.io/sdk/contrib/envconfig`, not in the CLI repo itself. The CLI calls into it. When investigating how config works, you need to look at both repos:
- **SDK** (`contrib/envconfig/client_config_toml.go`): `FromTOML`, `LoadClientConfig`, strict mode, `AdditionalProfileFields`
- **CLI** (`internal/temporalcli/commands.config.go`): calls the SDK, handles `temporal config set/get/delete/list`

### Unknown TOML keys are silently ignored by default

`FromTOML` has a `ConfigFileStrict` option that defaults to `false`. The CLI never enables strict mode. This means you can add arbitrary top-level sections (like `[ts-net]`) to `~/.config/temporalio/temporal.toml` and the CLI will not error. It just skips undecoded keys.

There is also an `AdditionalProfileFields` mechanism that preserves unknown keys within profiles when the CLI rewrites the file (e.g. via `temporal config set`). Top-level sections outside `[profile.*]` are also preserved through round-trips because the CLI reads and re-encodes the full TOML.

### Extensions can share the Temporal CLI config file

Because of the non-strict parsing, a CLI extension can store its own config in the same `temporal.toml` file under its own top-level TOML section. This is better UX than a separate config file -- one file to manage. The extension just decodes into a struct that only maps its own section and ignores everything else.

The extension proposal doc (`temporalio/proposals/cli/cli-extensions.md`) mentions that extensions can "mutate an environment configuration file and subsequent built-in commands can use it," but there is no formal extension config namespace. The non-strict parsing is what makes it work in practice.

### Config file location follows the same resolution as the CLI

The Temporal CLI resolves its config file path as:
1. `--config-file` CLI flag
2. `TEMPORAL_CONFIG_FILE` environment variable
3. `DefaultConfigFilePath()` -> `~/.config/temporalio/temporal.toml`

Extensions sharing the config file should use the same resolution chain (with their own `--config` flag name) so that if a user has overridden the config location, the extension finds the same file.

### Precedence: flags > env vars > config file > defaults

This is standard Unix convention (Docker, kubectl, Terraform, the Temporal CLI itself all follow it). The implementation challenge is distinguishing "user passed a flag with the same value as the default" from "user didn't pass the flag at all." A simple `map[string]bool` tracking which flags were explicitly parsed solves this without changing the options struct's public types.

### BurntSushi/toml is the standard

Both the Temporal CLI (via the SDK's envconfig) and the Tailscale library use `github.com/BurntSushi/toml`. It was already a transitive dependency. When adding TOML support to a Temporal extension, this is the library to use -- no new dependency needed, just promote from indirect to direct.

### Profile-scoped vs top-level config for extensions

The Temporal CLI config is organized around profiles (`[profile.default]`, `[profile.staging]`, etc.) which map to different Temporal server connections. An extension that runs a dev server (not a client connection) doesn't fit the profile model -- its config isn't per-environment. A top-level section (`[ts-net]`) is the right choice here. An extension that configures client behavior (auth, TLS, etc.) might belong under a profile instead.

### Duration fields in TOML

TOML has no native duration type. Use strings (`"10s"`, `"5m"`) and parse with `time.ParseDuration`. Validate during config load so errors surface early with the file path in the message, not later when the value is used.

### Pointer types for optional numeric fields

In a TOML-mapped struct, `int` and `float64` fields can't distinguish "not in the config file" from "set to zero." Use `*int` and `*float64` so nil means absent. String fields don't need this -- empty string is the natural zero value and none of the config keys treat empty string as a meaningful value.
