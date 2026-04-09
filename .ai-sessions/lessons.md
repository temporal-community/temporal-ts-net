# Lessons Learned

## Recent
<!-- 10 most recent lessons, newest first -->

- Temporal CLI extension support was added in v1.6.0 (PR #889 merged 2025-12-18, shipped 2026-02-05). Always check the merge date and next stable release to determine minimum version requirements. (2026-04-09)
- `/usr/local/bin` is the standard install location for user-installed binaries on macOS and Linux. `$GOPATH/bin` is only useful for Go developers and often isn't on PATH. (2026-04-09)
- When adding config to a Temporal CLI extension, research the CLI's own config system first. The non-strict TOML parsing means extensions can share `temporal.toml` under their own top-level section. (2026-04-09)
- The `flagsSet map[string]bool` pattern solves the "is this the default or did the user explicitly set it" problem for config precedence without changing public struct types. (2026-04-09)
- `BurntSushi/toml` is used by both the Temporal CLI (via SDK envconfig) and Tailscale. It's likely already a transitive dep in Temporal ecosystem projects. (2026-04-09)
- Temporal CLI extension binaries must use `_` for subcommand separators and dashes: `temporal ts-net` looks up `temporal-ts_net` on PATH. (2026-04-09)
- tsnet authenticates the app as its own Tailscale node, not the machine. Tailscale does not need to be installed on the host. (2026-04-09)
- GoReleaser defaults `ProjectName` to `release.github.name`. Set `project_name` explicitly if they should differ (e.g., repo name vs display name). (2026-04-09)
- Use pointer types (`*int`, `*float64`) in TOML config structs to distinguish "absent" from "set to zero." Strings don't need this since empty string is the natural zero. (2026-04-09)
- Validate TOML duration strings at load time, not at use time. Include the file path in error messages so users know which config file to fix. (2026-04-09)

## Categories
<!-- Lessons organized by topic -->

### Temporal CLI Extensions

- Temporal CLI extension support was added in v1.6.0 (PR #889 merged 2025-12-18, shipped 2026-02-05). Always check the merge date and next stable release to determine minimum version requirements. (2026-04-09)
- Temporal CLI extension binaries must use `_` for subcommand separators and dashes: `temporal ts-net` looks up `temporal-ts_net` on PATH. (2026-04-09)
- When adding config to a Temporal CLI extension, research the CLI's own config system first. The non-strict TOML parsing means extensions can share `temporal.toml` under their own top-level section. (2026-04-09)

### Temporal CLI Config System

- The Temporal CLI config system lives in the Go SDK (`go.temporal.io/sdk/contrib/envconfig`), not the CLI repo. The CLI calls into it. Look at both repos when investigating config behavior. (2026-04-09)
- `FromTOML` has `ConfigFileStrict` defaulting to `false`. The CLI never enables strict mode, so arbitrary top-level TOML sections are silently ignored. (2026-04-09)
- The `AdditionalProfileFields` mechanism preserves unknown keys within profiles when the CLI rewrites the file via `temporal config set`. (2026-04-09)
- Config file resolution: `--config-file` flag > `TEMPORAL_CONFIG_FILE` env var > `~/.config/temporalio/temporal.toml`. Extensions should follow the same chain. (2026-04-09)
- Profile-scoped config makes sense for client connection settings. Top-level sections make sense for extensions that aren't per-environment (like a dev server). (2026-04-09)

### Go Configuration Patterns

- The `flagsSet map[string]bool` pattern solves the "is this the default or did the user explicitly set it" problem for config precedence without changing public struct types. (2026-04-09)
- Standard Unix precedence: CLI flags > env vars > config file > defaults. This is what Docker, kubectl, Terraform, and the Temporal CLI all follow. (2026-04-09)
- Use pointer types (`*int`, `*float64`) in TOML config structs to distinguish "absent" from "set to zero." Strings don't need this since empty string is the natural zero. (2026-04-09)
- Validate TOML duration strings at load time, not at use time. Include the file path in error messages so users know which config file to fix. (2026-04-09)
- `BurntSushi/toml` is used by both the Temporal CLI (via SDK envconfig) and Tailscale. It's likely already a transitive dep in Temporal ecosystem projects. (2026-04-09)

### Tailscale / tsnet

- tsnet authenticates the app as its own Tailscale node, not the machine. Tailscale does not need to be installed on the host. (2026-04-09)
- tsnet auth URLs are short-lived. The interactive login flow is finicky. Auth keys are the more reliable path for non-interactive use. (2026-04-09)
- Tailscale version shown in the admin console comes from the `tailscale.com` Go module version. Keep the dependency updated. (2026-04-09)

### GoReleaser

- GoReleaser defaults `ProjectName` to `release.github.name`. Set `project_name` explicitly if they should differ (e.g., repo name vs display name). (2026-04-09)
- A VERSION file + GitHub Action watching for changes is a clean way to automate tag creation and release triggering without manual `git tag` commands. (2026-04-09)

### Installation / Distribution

- `/usr/local/bin` is the standard install location for user-installed binaries on macOS and Linux. `$GOPATH/bin` is only useful for Go developers and often isn't on PATH. (2026-04-09)
- A `curl | sh` install script needs OS detection (`uname -s`), arch detection (`uname -m`), and should map `aarch64` to `arm64` and `amd64` to `x86_64` to match GoReleaser archive naming. (2026-04-09)
