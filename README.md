<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/kashport-logo.png">
    <source media="(prefers-color-scheme: light)" srcset="assets/kashport-logo-dark.png">
    <img src="assets/kashport-logo-dark.png" alt="Kashport" height="48">
  </picture>
</p>

<h1 align="center">KP-Gruuk</h1>

<p align="center">
  <strong>Secure tunnel service for Kashport developers</strong><br>
  Expose local services through <code>gk.kspt.dev</code> with Okta authentication
</p>

<p align="center">
  <a href="#installation">Installation</a> &bull;
  <a href="#quick-start">Quick Start</a> &bull;
  <a href="#how-it-works">How It Works</a> &bull;
  <a href="#configuration">Configuration</a> &bull;
  <a href="#deployment">Deployment</a>
</p>

---

## Overview

KP-Gruuk is an internal [ngrok](https://ngrok.com)-like tunneling tool built for Kashport. It allows developers to expose local services to the internet through authenticated tunnels on the `gk.kspt.dev` domain.

Each developer gets a personal subdomain derived from their Okta email:

```
juan@kashport.com  →  https://juan.gk.kspt.dev
```

## Installation

### One-line install (macOS / Linux)

```sh
curl -sSL https://raw.githubusercontent.com/kashportsa/kp-gruuk/main/install.sh | sh
```

This detects your OS and architecture, downloads the correct binary, and installs it to `/usr/local/bin`.

### From source

```sh
git clone https://github.com/kashportsa/kp-gruuk.git
cd kp-gruuk
make build
# Binaries are in ./bin/
```

> Requires Go 1.22+

## Quick Start

```sh
# Expose a local service running on port 3000
gruuk expose 3000
```

On first run, Gruuk will open your browser for Okta authentication:

```
To authenticate, open your browser at:
  https://kashport.okta.com/activate?user_code=ABCD-EFGH

Waiting for authentication...
Authenticated successfully!

Tunnel active!
  https://juan.gk.kspt.dev -> http://localhost:3000

  Press Ctrl+C to stop.
```

That's it. Anyone can now reach your local service at `https://juan.gk.kspt.dev`.

## How It Works

```
                    ┌──────────────────────┐
                    │    gk.kspt.dev       │
                    │   (ALB + ECS)        │
   Visitors ──────▶│                      │◀──── WebSocket ──── gruuk CLI
  HTTP/HTTPS       │  gruuk-server        │       Tunnel        (developer laptop)
                    │                      │                         │
                    └──────────────────────┘                         │
                                                              localhost:3000
```

1. **Developer** runs `gruuk expose 3000` on their machine
2. **CLI** authenticates with Okta using the [Device Authorization Flow](https://developer.okta.com/docs/guides/device-authorization-grant/main/)
3. **CLI** establishes a WebSocket tunnel to the Gruuk server
4. **Server** assigns a subdomain based on the developer's email
5. **Visitors** access `https://juan.gk.kspt.dev` — requests are proxied through the tunnel to the developer's local service

### Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| **WebSocket tunnel** | Works through firewalls/proxies, simple to implement |
| **JSON message framing** | Easy to debug, acceptable overhead for dev traffic |
| **Device Auth Flow** | Works in SSH sessions, headless envs, best CLI UX |
| **Single instance** | In-memory registry is sufficient for internal use |
| **Public visitor access** | No auth required to reach exposed services |

## CLI Reference

### `gruuk expose <port>`

Expose a local service running on the given port.

```sh
gruuk expose 8080                           # Expose localhost:8080
gruuk expose 3000 --server-url ws://localhost:9090  # Custom server (dev)
gruuk expose 8080 --token <jwt>             # Skip auth (testing)
```

### `gruuk status`

Check authentication status and token validity.

### `gruuk logout`

Clear stored authentication tokens from `~/.gruuk/token.json`.

### `gruuk version`

Print the installed version.

## Configuration

### Client Configuration

Stored at `~/.gruuk/config.json`:

```json
{
  "server_url": "wss://gk.kspt.dev",
  "okta_issuer": "https://kashport.okta.com/oauth2/default",
  "okta_client_id": "0oa..."
}
```

### Server Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DOMAIN` | `gk.kspt.dev` | Base domain for tunnel subdomains |
| `LISTEN_ADDR` | `:8080` | Server listen address |
| `OKTA_ISSUER` | — | Okta authorization server URL |
| `OKTA_CLIENT_ID` | — | Okta application client ID |
| `SKIP_AUTH` | `false` | Disable JWT validation (testing only) |

## Deployment

### Prerequisites

1. **Okta Application** — Create a Native app with:
   - Grant types: Device Authorization, Refresh Token
   - Scopes: `openid`, `email`, `profile`, `offline_access`
   - No client secret (public client)

2. **DNS** — Wildcard record for `*.gk.kspt.dev` pointing to the ALB

3. **TLS** — ACM wildcard certificate for `*.gk.kspt.dev`

### AWS Architecture

```
Route53 (*.gk.kspt.dev)  →  ALB (HTTPS 443, wildcard cert)  →  ECS Fargate (gruuk-server :8080)
```

- **ALB idle timeout**: 3600s (required for long-lived WebSocket connections)
- **ECS Fargate**: 512 CPU / 1024 MiB memory, single instance
- **TLS**: ACM wildcard cert with DNS validation

### Deploy with Terraform

```sh
cd deploy/terraform

terraform init
terraform apply \
  -var="okta_issuer=https://kashport.okta.com/oauth2/default" \
  -var="okta_client_id=0oa..." \
  -var="vpc_id=vpc-..." \
  -var="subnet_ids=[\"subnet-...\",\"subnet-...\"]" \
  -var="hosted_zone_id=Z..."
```

### Docker

```sh
# Build
docker build -t gruuk-server -f Dockerfile.server .

# Run
docker run -p 8080:8080 \
  -e DOMAIN=gk.kspt.dev \
  -e OKTA_ISSUER=https://kashport.okta.com/oauth2/default \
  -e OKTA_CLIENT_ID=0oa... \
  gruuk-server
```

## Development

### Local testing (no auth)

```sh
# Terminal 1: Start server in skip-auth mode
go run ./cmd/gruuk-server --skip-auth

# Terminal 2: Start a local test service
python3 -m http.server 9090

# Terminal 3: Connect the tunnel
go run ./cmd/gruuk expose 9090 --server-url ws://localhost:8080 --token dummy

# Terminal 4: Test it
curl -H "Host: dummy.gk.kspt.dev" http://localhost:8080/
```

### Run tests

```sh
make test          # Unit + integration tests with race detection
```

### Build release binaries

```sh
make release       # Cross-compile for darwin/linux amd64/arm64
```

## Project Structure

```
kp-gruuk/
├── cmd/
│   ├── gruuk/              # CLI client binary
│   └── gruuk-server/       # Server binary
├── internal/
│   ├── auth/               # Okta Device Flow, JWT validation, token store
│   ├── tunnel/             # WebSocket protocol, request multiplexer
│   ├── server/             # HTTP server, subdomain routing, proxy
│   ├── client/             # CLI commands, local proxy, tunnel client
│   └── config/             # Server and client configuration
├── deploy/
│   └── terraform/          # AWS infrastructure (ALB, ECS, Route53, ACM)
├── test/
│   └── integration/        # End-to-end tunnel tests
├── Dockerfile.server
├── Makefile
└── install.sh              # One-line installer script
```

## Security

- Tunnel connections require a valid Okta JWT token
- Tokens are stored with `0600` permissions at `~/.gruuk/token.json`
- Server validates JWT signature, issuer, audience, and expiration via JWKS
- WebSocket connections are encrypted via TLS (wss://)
- Request body size limited to 10MB
- No credentials are transmitted in URL parameters

## License

Internal use only. Proprietary to Kashport.
