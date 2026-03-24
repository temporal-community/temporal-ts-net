# temporal-ts-net

Extension command for Temporal CLI that exposes the Temporal development server on your Tailscale tailnet.

This extension provides:

- `temporal ts-net` as an enhanced wrapper for `temporal server start-dev`
- Tailscale networking integration to expose your dev server across your tailnet
- Simple setup with automatic Tailscale authentication

## Install

Build the extension binary:

```bash
make build
# or
go build -o ./bin/temporal-ts_net ./cmd/temporal-ts_net
```

Add `./bin` to your `PATH` and verify discovery:

```bash
temporal help --all
```

You should see `ts-net` listed as an extension command.

## Usage

Start local dev server without Tailscale:

```bash
temporal ts-net
```

Expose dev server on Tailscale tailnet:

```bash
temporal ts-net \
    --tailscale \
    --tailscale-hostname your-dev-host
```

`--tsnet` and related `--tsnet-*` flags are also accepted aliases.

Pass any `temporal server start-dev` flags through directly:

```bash
temporal ts-net \
    --tailscale \
    --port 7234 \
    --ui-port 8234 \
    --db-filename /tmp/temporal-dev.db
```

## Extension flags

- `--tailscale` / `--tsnet`: enable tsnet listener and proxy
- `--tailscale-hostname` / `--tsnet-hostname`: tsnet hostname (default `temporal-dev`)
- `--tailscale-authkey` / `--tsnet-authkey`: auth key for non-interactive auth (or set `TS_AUTHKEY` env var)
- `--tailscale-state-dir` / `--tsnet-state-dir`: local state dir for tsnet node

All non-extension flags are forwarded to `temporal server start-dev`.
