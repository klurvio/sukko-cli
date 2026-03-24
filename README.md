# Sukko CLI

Command-line interface for the [Sukko](https://github.com/klurvio/sukko) WebSocket infrastructure platform.

## Install

### Homebrew (macOS/Linux)

```bash
brew install klurvio/tap/sukko
```

### Scoop (Windows)

```powershell
scoop bucket add klurvio https://github.com/klurvio/scoop-bucket
scoop install sukko
```

### Go

```bash
go install github.com/klurvio/sukko-cli@latest
```

### Binary

Download from [Releases](https://github.com/klurvio/sukko-cli/releases). Available for Linux, macOS, and Windows on amd64 and arm64.

## Quick Start

```bash
# Initialize a context (stores server URLs and credentials locally)
sukko init

# Start local development stack
sukko up

# Create a tenant
sukko tenant create --name my-app

# Generate an API key
sukko key create --tenant <tenant-id>

# Subscribe to a channel
sukko subscribe --channel chat --key <api-key>

# Publish a message
sukko publish --channel chat --data '{"text": "hello"}' --key <api-key>

# Stop local stack
sukko down
```

## Commands

| Command | Description |
|---------|-------------|
| `sukko init` | Set up a CLI context |
| `sukko context` | Switch, list, or delete contexts |
| `sukko up` / `down` | Start/stop local dev environment (Docker Compose) |
| `sukko tenant` | Manage tenants |
| `sukko key` | Manage API keys |
| `sukko token` | Generate JWT tokens |
| `sukko subscribe` | Subscribe to WebSocket channels |
| `sukko publish` | Publish messages to channels |
| `sukko connections` | List active connections |
| `sukko rules` | Manage channel rules |
| `sukko quota` | Manage quotas |
| `sukko health` | Check service health |
| `sukko status` | Show platform status |
| `sukko logs` | View service logs |
| `sukko test` | Run connectivity tests |
| `sukko config` | View/set config defaults |
| `sukko completion` | Generate shell completions |
| `sukko version` | Print version info |

## Configuration

Flags can be set via environment variables:

| Flag | Environment Variable | Default |
|------|---------------------|---------|
| `--api-url` | `SUKKO_API_URL` | `http://localhost:8080` |
| `--token` | `SUKKO_TOKEN` | — |
| `--context` | `SUKKO_CONTEXT` | — |

Resolution order: **flags > context > environment variables > defaults**.

## License

MIT
