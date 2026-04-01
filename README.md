# Sukko CLI

Command-line interface for the [Sukko](https://github.com/klurvio/sukko-issues) WebSocket infrastructure platform.

The CLI serves two purposes:

- **Local development** — spin up the full Sukko platform locally via Docker Compose (`sukko init` + `sukko up`), with optional observability stack (Prometheus, Grafana, Tempo, Pyroscope)
- **Remote operations** — manage tenants, keys, rules, and inspect any deployed Sukko instance by switching contexts (`sukko context use staging`)

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
# Initialize a local context (stores server URLs and credentials)
sukko init

# Start local development stack (Docker Compose)
sukko up

# Check edition and resource usage
sukko edition

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

### License Key Management

```bash
# Store a license key (encrypted in context)
sukko license set

# View stored license info
sukko license show

# Compare editions
sukko edition compare

# License key is automatically passed to services on sukko up
sukko up
```

### Remote Contexts

```bash
# Create a context for a remote deployment (interactive, secrets masked)
sukko context create

# Switch between environments
sukko context use staging
sukko tenant list
```

### Authentication

```bash
# API key auth (primary method for subscribe/publish)
sukko key create --tenant <tenant-id> --generate   # generates ES256 key pair
sukko subscribe --channel chat                      # uses API key from context

# JWT auth (full production auth path)
sukko key create --generate --tenant local          # generate + register key pair
sukko token generate --tenant local --sub user      # auto-discovers stored key
sukko subscribe --channel chat --token <jwt>        # gateway validates JWT
```

### Testing

The built-in tester service (included in Sukko deployments) runs smoke, load, stress, and validation tests. The CLI delegates test execution to the tester via its REST API.

```bash
# Run tests (CLI passes deployment context to tester automatically)
sukko test smoke
sukko test load --connections 100 --duration 5m
sukko test validate --suite auth
sukko test validate --suite provisioning

# Test with a specific message backend
sukko test load --message-backend=kafka --connections 50
```

### Observability

```bash
# Enable observability during init (Prometheus, Grafana, Tempo, Pyroscope, AlertManager)
sukko init

# Start with observability profile activated
sukko up

# Open Grafana dashboard
sukko grafana
```

## Commands

### Works everywhere (local + remote)

| Command | Description |
|---------|-------------|
| `sukko tenant` | Manage tenants |
| `sukko key` | Manage signing keys (`--generate` for ES256 key pair) |
| `sukko token` | Generate and validate JWT tokens |
| `sukko subscribe` | Subscribe to WebSocket channels |
| `sukko publish` | Publish messages to channels |
| `sukko connections` | List active connections |
| `sukko rules` | Manage channel rules |
| `sukko quota` | Manage quotas |
| `sukko edition` | Show current edition, limits, and resource usage |
| `sukko edition compare` | Compare Community, Pro, and Enterprise editions |
| `sukko license` | Store, view, and remove license keys |
| `sukko health` | Check service health |
| `sukko status` | Show platform status |
| `sukko test` | Run smoke, load, stress, and validation tests |
| `sukko context` | Create, switch, list, or delete contexts |
| `sukko config` | View/set config defaults |
| `sukko completion` | Generate shell completions |
| `sukko version` | Print version info |

### Local development only (Docker Compose)

| Command | Description |
|---------|-------------|
| `sukko init` | Set up local context + infrastructure selections |
| `sukko up` | Start local dev environment (activates selected profiles) |
| `sukko up --pull` | Pull latest images before starting |
| `sukko down` | Stop local dev environment |
| `sukko logs` | View Docker Compose service logs |
| `sukko grafana` | Open Grafana dashboard in browser (requires observability) |

## Configuration

Flags can be set via environment variables:

| Flag | Environment Variable | Default |
|------|---------------------|---------|
| `--api-url` | `SUKKO_API_URL` | `http://localhost:8080` |
| `--token` | `SUKKO_TOKEN` | — |
| `--context` | `SUKKO_CONTEXT` | — |

Resolution order: **flags > context > environment variables > defaults**.

### Contexts

Contexts store per-environment configuration (URLs, encrypted credentials, license keys).

```bash
sukko init                    # creates "local" context with dev defaults
sukko context create          # create a remote context (interactive, secrets masked)
sukko context list            # list all contexts
sukko context use staging     # switch to staging
sukko context current         # show active context details
```

## License

MIT
