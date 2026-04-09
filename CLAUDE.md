# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A Temporal CLI extension (`temporal ts-net`) that runs `temporal server start-dev` and exposes it on a Tailscale tailnet via tsnet. The binary must be named `temporal-ts_net` (underscores for subcommand separators, required by the Temporal extension system).

## Build and test commands

```bash
go run mage.go build     # Build to ./bin/temporal-ts_net
go run mage.go test      # All tests with -race -shuffle=on
go run mage.go install   # Install to $GOPATH/bin
go test ./internal/app/  # Run only app package tests
go test -short ./...     # Skip integration tests (tsnet e2e)
go test ./internal/app/ -run TestLoadConfig  # Single test
```

## Architecture

```
cmd/temporal-ts_net/main.go    Entry point, delegates to app.Run()
internal/app/
  app.go       Orchestration: parse args → load config → start temporal subprocess → start tsnet proxy
  parse.go     Hand-rolled arg parser. Extension flags extracted, everything else passed through to temporal server start-dev
  config.go    TOML config loading from [ts-net] section of the Temporal CLI config file
internal/tailscale/
  tsnet.go     TCP proxy: tsnet listeners accept tailnet connections, proxy to local temporal server
```

The extension starts `temporal server start-dev` as a child process, then starts a tsnet node that proxies tailnet connections to it. Both gRPC and UI ports are proxied.

## Config precedence

CLI flags > `TS_AUTHKEY` env var > `[ts-net]` section in `~/.config/temporalio/temporal.toml` > defaults.

The `flagsSet` map on `ExtensionOptions` tracks which flags were explicitly passed so config file values only fill in gaps.

## Key patterns

- **Flag tracking**: `opts.flagsSet["flag-name"] = true` in every parse branch. `IsSet()` checks this during config merge.
- **Pointer types in FileConfig**: `*int` and `*float64` distinguish "absent from TOML" (nil) from "set to zero."
- **Config file sharing**: Reads from the same `temporal.toml` the Temporal CLI uses. The CLI's TOML parser ignores unknown top-level sections (non-strict mode), so `[ts-net]` coexists safely.
- **Graceful shutdown**: SIGINT to child process, wait with timeout, then SIGKILL. tsnet listeners close first, WaitGroup drains active proxies.

## Testing

- `internal/app/`: Unit tests for arg parsing, config loading, config merging, precedence
- `internal/tailscale/`: Integration tests using Tailscale's testcontrol (in-memory coordination server, no real Tailscale needed)
- `internal/app/e2e_test.go`: Full workflow execution through the proxy using a real Temporal dev server

## Releases

Bump the `VERSION` file and merge to main. GitHub Actions creates the tag and release, GoReleaser builds cross-platform binaries. The goreleaser config targets the real GitHub repo name (`temporal-start-dev-ext`) since that can't be renamed yet, but `project_name` is set to `temporal-ts-net` for archive naming.
