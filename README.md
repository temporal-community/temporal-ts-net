# temporal-ts-net

A [Temporal CLI](https://docs.temporal.io/cli) extension that runs `temporal server start-dev` and exposes it on your [Tailscale](https://tailscale.com/) tailnet.

Your dev server becomes reachable at `temporal-dev:7233` from any machine on your tailnet.

## Prerequisites

- [Temporal CLI](https://docs.temporal.io/cli#install) v1.6.0+ (extension support was added in v1.6.0)
- A [Tailscale](https://tailscale.com/) account

## Install

### Quick install

```bash
curl -sSfL https://raw.githubusercontent.com/chaptersix/temporal-start-dev-ext/main/install.sh | sh
```

This detects your OS and architecture, downloads the latest release, and installs to `/usr/local/bin`.

### Manual download

Download from the [Releases](https://github.com/chaptersix/temporal-start-dev-ext/releases) page. 
Extract the binary and place it somewhere on your `PATH`, for example `/usr/local/bin`:

```bash
tar -xzf temporal-ts-net_*.tar.gz
sudo mv temporal-ts_net /usr/local/bin/
```

### Build from source

```bash
go build -o ./bin/temporal-ts_net ./cmd/temporal-ts_net
```

Or using [Mage](https://magefile.org/):

```bash
go run mage.go build
```

Add `./bin` to your `PATH`.

### Verify

```bash
temporal help --all
```

You should see `ts-net` listed as an extension command.

> The binary is named `temporal-ts_net` because the Temporal CLI extension system uses `_` as the separator for subcommands and dashes. Running `temporal ts-net` triggers a `PATH` lookup for `temporal-ts_net`.

## Usage

Start the dev server on your tailnet:

```bash
temporal ts-net
```

This starts `temporal server start-dev` locally and proxies connections from your tailnet into it. 
The extension uses [tsnet](https://tailscale.com/kb/1244/tsnet) to join your tailnet directly. Tailscale does not need to be installed on the machine.

On first run, tsnet will log an authorization URL to stderr. 
Open it in a browser to authorize the app with your tailnet. 
To skip this, provide an [auth key](https://tailscale.com/docs/features/access-control/auth-keys). 
Generate one in the Tailscale admin console under **Settings > Keys > Auth keys**:

```bash
temporal ts-net --tailscale-authkey tskey-auth-...
# or
TS_AUTHKEY=tskey-auth-... temporal ts-net
```

Set a custom hostname:

```bash
temporal ts-net --tailscale-hostname my-temporal
```

All other flags pass through to `temporal server start-dev`:

```bash
temporal ts-net --port 7234 --ui-port 8234 --db-filename /tmp/temporal.db
```

## Extension flags

| Flag | Default | Description |
|------|---------|-------------|
| `--config` | | Path to config file (see [Configuration](#configuration)) |
| `--tailscale-hostname` | `temporal-dev` | Tailnet hostname |
| `--tailscale-authkey` | | Auth key (or `TS_AUTHKEY` env var) |
| `--tailscale-state-dir` | | Local state directory for tsnet node |
| `--max-connections` | `1000` | Maximum concurrent connections |
| `--connection-rate-limit` | `100` | Maximum connections per second |
| `--dial-timeout` | `10s` | Timeout for dialing backend |
| `--idle-timeout` | `5m` | Idle timeout for proxy connections |

Flags also accept the `--tsnet-` prefix (e.g. `--tsnet-hostname`).

## Configuration

The extension reads its configuration from the `[ts-net]` section of the Temporal CLI config file. 
This is the same file used by `temporal config set/get` -- by default `~/.config/temporalio/temporal.toml`.

```toml
[ts-net]
tailscale-hostname = "my-temporal"
tailscale-authkey = "tskey-auth-..."
tailscale-state-dir = "/path/to/state"
max-connections = 500
connection-rate-limit = 50
dial-timeout = "15s"
idle-timeout = "10m"
```

All fields are optional. 
Only set what you want to override.

The config file location is resolved in order:

1. `--config` flag
2. `TEMPORAL_CONFIG_FILE` environment variable
3. `~/.config/temporalio/temporal.toml`

Precedence: CLI flags > environment variables (`TS_AUTHKEY`) > config file > defaults. For example, `--tailscale-authkey` on the command line beats `TS_AUTHKEY` in the environment, which beats `tailscale-authkey` in the config file.

## How to use

Once the server is running, any machine on your tailnet can connect to it.

### Temporal CLI

Point the CLI at the tailnet hostname:

```bash
temporal workflow list --address temporal-dev:7233
```

Or set it in a config profile so you don't have to pass it every time:

```bash
temporal config set --profile tailnet address temporal-dev:7233
temporal --profile tailnet workflow list
```

### Temporal applications

Temporal SDKs support [Environment Configuration](https://docs.temporal.io/develop/environment-configuration).
This lets you configure the server address per environment without changing code.
Set up a `dev` profile pointing at the tailnet address and a `prod` profile pointing at [Temporal Cloud](https://temporal.io/cloud):

```toml
[profile.dev]
address = "temporal-dev:7233"
namespace = "default"

[profile.prod]
address = "your-namespace.a1b2c.tmprl.cloud:7233"
namespace = "your-namespace"
api_key = "your-api-key"
```

Then load the profile in your application.
The SDK reads the active profile from `TEMPORAL_PROFILE` and connects accordingly:

```bash
TEMPORAL_PROFILE=dev go run ./worker
```

```go
// No address hardcoded. The SDK loads it from the active profile.
c, err := client.Dial(envconfig.MustLoadDefaultClientOptions())
```

Your application code stays the same.
The connection target is determined by which profile is active at runtime.

## How it works

The extension starts `temporal server start-dev` as a child process listening on localhost, then starts a [tsnet](https://pkg.go.dev/tailscale.com/tsnet) node that listens on your tailnet. 
Incoming connections on the tailnet are proxied to the local Temporal server over TCP. 
Both the gRPC port and the UI port are proxied.

## Development

Be sure to have [Go 1.26+](https://go.dev/dl/) installed.
This project uses [Mage](https://magefile.org/) for build tasks. 
Mage installation is optional -- all targets work via `go run`:

```bash
go run mage.go -l        # List targets
go run mage.go build     # Build the binary
go run mage.go test      # Run tests
go run mage.go fmt       # Format code
go run mage.go clean     # Remove build artifacts
go run mage.go install   # Install to $GOPATH/bin
```

### Testing

```bash
go test ./...
```

Tailscale integration tests use [testcontrol](https://pkg.go.dev/tailscale.com/tstest/integration/testcontrol) and run entirely in-process.
No Tailscale account or auth keys needed. They run the same way in CI.

The [demo/](demo/) directory has a self-contained example of the proxy pattern:

```bash
go run demo/tailscale-proxy/main.go
```

### Releases

This project uses [GoReleaser](https://goreleaser.com/) with GitHub Actions. To cut a release, bump the version in `VERSION` and merge to main. GitHub Actions will create the tag, build binaries for Linux, macOS, and Windows (amd64/arm64), and publish a GitHub release.
