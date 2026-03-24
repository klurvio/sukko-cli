# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Sukko CLI (`sukko`) is the command-line interface for the Sukko WebSocket infrastructure platform. It provides commands for managing tenants, API keys, connections, subscriptions, publishing, and local development via Docker Compose.

This is a standalone public repository extracted from the private Sukko monorepo. It contains only the CLI — no server, gateway, or provisioning service code.

## Development Commands

```bash
go build .              # Build the CLI binary
go test -race ./...     # Run all tests with race detector
go vet ./...            # Static analysis
golangci-lint run ./... # Lint

# GoReleaser
goreleaser check                      # Validate .goreleaser.yml
goreleaser release --snapshot --clean  # Local dry run (builds all artifacts)
```

## Source Structure

```
sukko-cli/
├── main.go                   # Entrypoint → commands.Execute()
├── internal/version/         # Build-time version info (ldflags)
├── commands/                 # Cobra command definitions
│   ├── root.go              # Root command, global flags, env-backed defaults
│   ├── version.go           # sukko version
│   ├── config_cmd.go        # sukko config defaults/view
│   ├── completion.go        # Shell completion generation
│   ├── init.go              # sukko init (context setup)
│   ├── context.go           # sukko context (switch/list/delete)
│   ├── up.go / down.go      # Docker Compose lifecycle
│   ├── subscribe.go         # WebSocket subscribe
│   ├── publish.go           # WebSocket publish
│   ├── connections.go       # Connection listing
│   ├── tenant.go            # Tenant management
│   ├── api_key.go / key.go  # API key management
│   ├── token_cmd.go         # JWT token generation
│   ├── rules.go             # Channel rules
│   ├── quota.go             # Quota management
│   ├── health.go / status.go / logs.go / test.go
│   └── output.go            # Output formatting helpers
├── client/                   # HTTP + WebSocket client wrappers
├── compose/                  # Docker Compose orchestration
├── context/                  # CLI context store (encrypted credentials)
└── token/                    # JWT token generation
```

## Key Patterns

### Configuration
CLI flags use env-var-backed defaults via `envOrDefault()` in `root.go`:
```go
rootCmd.PersistentFlags().StringVar(&serverURL, "server", envOrDefault("SUKKO_SERVER_URL", "http://localhost:8080"), "Server URL")
```

### Version Injection
Build-time variables set via ldflags:
```bash
go build -ldflags "-X 'github.com/klurvio/sukko-cli/internal/version.Version=v1.0.0' ..."
```

### Error Handling
All errors MUST be wrapped with context using `fmt.Errorf("operation: %w", err)`.

### Testing
Tests MUST be run with `-race` flag. Table-driven tests preferred.

## Commit Message Format

Conventional commits:
```
type: subject

Examples:
feat: add tenant connection tracking
fix: resolve websocket reconnect loop
```

Valid types: feat, fix, docs, style, refactor, perf, test, build, ci, chore, revert

## Release Process

1. Push a `v*` tag: `git tag v1.0.0 && git push origin v1.0.0`
2. GitHub Actions runs GoReleaser automatically
3. Produces: GitHub Release with 6 platform binaries, checksums, .deb, .rpm
4. Updates: Homebrew tap (`klurvio/homebrew-tap`), Scoop bucket (`klurvio/scoop-bucket`)

Pre-release tags (e.g., `v1.0.0-rc1`) are marked as pre-release and do NOT update package manager manifests.
