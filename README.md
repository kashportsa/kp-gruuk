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
  <a href="#for-users">For Users</a> &bull;
  <a href="#cli-reference">CLI Reference</a> &bull;
  <a href="#for-developers">For Developers</a> &bull;
  <a href="#deployment">Deployment</a> &bull;
  <a href="#security">Security</a>
</p>

---

## Overview

KP-Gruuk is an internal [ngrok](https://ngrok.com)-like tunneling tool built for Kashport. It lets you expose a local service to the internet through an authenticated WebSocket tunnel on the `gk.kspt.dev` domain — no port forwarding or VPN required.

Each developer gets a personal subdomain derived from their Okta email:

```
juan.perez@kashport.com  ->  https://juan-perez.gk.kspt.dev
david.villa@kashport.com ->  https://david-villa.gk.kspt.dev
```

Non-alphanumeric characters in the local part of the email are replaced with hyphens, and consecutive hyphens are collapsed.

---

## For Users

### Installation

**One-line install (macOS / Linux):**

```sh
curl -sSL https://raw.githubusercontent.com/kashportsa/kp-gruuk/main/install.sh | sh
```

The script auto-detects your OS and architecture, downloads the correct binary from the latest GitHub release, and installs it to `/usr/local/bin` (or `~/.local/bin` as a fallback if sudo is unavailable).

Supported platforms: `darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64`.

**From source (requires Go 1.22+):**

```sh
git clone https://github.com/kashportsa/kp-gruuk.git
cd kp-gruuk
make build
# Binaries: ./bin/gruuk  ./bin/gruuk-server
```

**Verify:**

```sh
gruuk version
```

---

### Quick Start

```sh
# Expose a local service running on port 3000
gruuk expose 3000
```

The first time you run this, Gruuk opens your browser for Okta authentication:

```
  To authenticate, open your browser at:
  https://kashport.okta.com/activate?user_code=WXYZ-ABCD

  Or go to https://kashport.okta.com/activate and enter code: WXYZ-ABCD

  Waiting for authentication...
  Authenticated successfully!

  Tunnel active!
  https://juan-perez.gk.kspt.dev -> http://localhost:3000

  Press Ctrl+C to stop.
```

Anyone can now reach your local service at `https://juan-perez.gk.kspt.dev`. No inbound firewall rules, no VPN, no DNS changes needed on your end.

---

### Authentication

Gruuk uses the [OAuth 2.0 Device Authorization Flow](https://developer.okta.com/docs/guides/device-authorization-grant/main/) (RFC 8628). This works from any environment, including SSH sessions where a browser is not available locally.

**How it works:**

1. Gruuk requests a short-lived user code from Okta
2. You open the displayed URL in any browser and approve the request (you have ~5 minutes)
3. Gruuk polls in the background and receives your token automatically
4. The token is saved to `~/.gruuk/token.json` with `0600` permissions

**Subsequent runs:** Gruuk reuses the stored token silently — no browser prompt. If the access token is expired, it is automatically refreshed using the stored refresh token. You only need to re-authenticate if you explicitly log out or the refresh token itself expires (typically 24h or configured by the Okta policy).

**Token files:**

| File | Contents |
|------|----------|
| `~/.gruuk/token.json` | Access token, refresh token, expiry |
| `~/.gruuk/config.json` | Server URL, Okta issuer and client ID |

---

### Reconnection

If the tunnel drops (network hiccup, server restart, sleep/wake), Gruuk automatically reconnects:

```
  Reconnecting... (attempt 1)
  Reconnected! https://juan-perez.gk.kspt.dev -> http://localhost:3000
```

Backoff starts at 2s and caps at 30s with 25% jitter to avoid thundering-herd. Your public URL never changes — it is always derived from your email.

---

### CLI Reference

#### `gruuk expose <port>`

Expose `localhost:<port>` through a tunnel.

```sh
gruuk expose 8080
gruuk expose 3000 --server-url wss://gk.kspt.dev   # explicit server
gruuk expose 8080 --token <jwt>                     # skip Okta flow, use provided token
```

Press `Ctrl+C` for a clean shutdown. The subdomain is released immediately.

#### `gruuk status`

Show whether you are currently authenticated and when the token expires.

```sh
gruuk status
# Authenticated. Token valid until 2026-04-01T18:00:00Z
# — or —
# Not authenticated. Run 'gruuk expose <port>' to authenticate.
# — or —
# Token expired. Run 'gruuk expose <port>' to re-authenticate.
```

#### `gruuk logout`

Clear all stored tokens. The next `gruuk expose` will trigger a new Okta login.

```sh
gruuk logout
# Logged out. Stored tokens cleared.
```

#### `gruuk version`

Print the installed binary version.

```sh
gruuk version
# gruuk v1.3.0
```

#### Global flags

| Flag | Default | Description |
|------|---------|-------------|
| `--server-url` | `wss://gk.kspt.dev` | Override the tunnel server URL |
| `--token` | — | Use a specific JWT (skips the Okta device flow entirely) |

---

### Client Configuration

The `~/.gruuk/config.json` file is **fully optional**. All fields have built-in defaults pointing to the Kashport production server and Okta tenant — a fresh install works with no configuration.

You only need a config file if you want to override a default:

```json
{
  "server_url": "wss://gk.kspt.dev",
  "okta_issuer": "https://kashport.okta.com/oauth2/default",
  "okta_client_id": "0oa..."
}
```

---

### Troubleshooting

**"Device not activated — internal error" from Okta**

Your Okta account must be assigned to the Gruuk application. Contact the platform team to add your account to the `kashport-admins` group or the Gruuk app directly.

**"invalid token" / 401 on connect**

Run `gruuk logout` then `gruuk expose <port>` to get a fresh token.

**"subdomain_taken" error on connect**

Another active session is already using your subdomain. Close any other running `gruuk` processes. If a previous session crashed, the server releases the subdomain within a few seconds of detecting the dead WebSocket.

**Tunnel connects but requests fail with 502 Bad Gateway**

Your local service on the given port is not running or not accepting connections. Start your local server first.

**Requests time out (504 Gateway Timeout)**

The server waits up to 30 seconds for a response from your local service. If your service takes longer, visitors will receive a 504. The tunnel itself remains connected.

**Browser doesn't open automatically**

Copy the URL printed to the terminal and open it manually. The automatic browser open is a best-effort convenience — it uses `open` on macOS and `xdg-open` on Linux.

---

## For Developers

This section covers contributing to and running kp-gruuk itself.

### Architecture

```
                         +----------------------------------+
                         |          gk.kspt.dev             |
  Visitors               |                                  |
  HTTP/HTTPS ----------> |  ALB (port 443, wildcard TLS)   |
                         |          |                       |
                         |          v                       |
                         |   gruuk-server :8080             |
                         |   +-- /_health      (health)    |
                         |   +-- /_ws/connect  (tunnels)   |
                         |   +-- *.gk.kspt.dev (proxy)     |
                         +----------+------------------------+
                                    |
                              WebSocket (wss://)
                                    |
                              gruuk CLI
                           (developer laptop)
                                    |
                              localhost:PORT
                           (your local service)
```

**Request flow — visitor to local service:**

1. Browser hits `https://juan-perez.gk.kspt.dev/api/users`
2. ALB forwards to gruuk-server (Host header preserved)
3. Server extracts subdomain `juan-perez` and looks up the registered tunnel
4. Server sends an `http_request` envelope over the WebSocket
5. CLI receives it, forwards the request to `http://localhost:PORT`
6. CLI sends back an `http_response` envelope
7. Server writes the response to the visitor

**Tunnel registration flow:**

1. CLI dials `wss://gk.kspt.dev/_ws/connect` with `Authorization: Bearer <jwt>`
2. Server validates the JWT via Okta JWKS endpoint (`/v1/keys`)
3. Server extracts the `email` claim and converts it to a subdomain
4. If the subdomain is free, the connection is registered in the in-memory registry
5. Server sends a `connected` message containing `subdomain` and `public_url`
6. CLI enters the request processing loop

---

### Protocol

All messages are JSON text frames over WebSocket. Every message has the shape:

```json
{
  "type": "http_request",
  "request_id": "550e8400-e29b-41d4-a716-446655440000",
  "payload": { ... }
}
```

| Type | Direction | Description |
|------|-----------|-------------|
| `connected` | server → client | Tunnel ready; contains `subdomain` and `public_url` |
| `http_request` | server → client | Incoming visitor request to forward |
| `http_response` | client → server | Response from local service |
| `ping` | server → client | Heartbeat, sent every 30s |
| `pong` | client → server | Heartbeat reply |
| `error` | server → client | Fatal error (e.g., `subdomain_taken`) |

Request and response bodies are base64-encoded in the `body` field. The WebSocket read limit is 16 MB on both sides.

---

### Local Development

You can run the full stack locally without Okta using `--skip-auth` mode.

```sh
# Terminal 1: start the server (no JWT validation)
go run ./cmd/gruuk-server --skip-auth

# Terminal 2: start a test service
python3 -m http.server 9090

# Terminal 3: connect a tunnel
# In skip-auth mode, pass your email via --token (value is ignored) and X-Email header
go run ./cmd/gruuk expose 9090 \
  --server-url ws://localhost:8080 \
  --token dummy

# Terminal 4: test it
curl -H "Host: dummy.localhost:8080" http://localhost:8080/
```

In `--skip-auth` mode the server reads identity from the `X-Email` request header or `?email=` query parameter. The `--token` flag bypasses the Okta device flow on the client.

**Docker Compose (local):**

```sh
# Requires a .env file with OKTA_ISSUER and OKTA_CLIENT_ID
docker compose up
```

The compose file maps container port 8080 to host port 8090.

---

### Running Tests

```sh
make test
# go test -race ./...
```

The test suite includes:

| Package | Coverage |
|---------|----------|
| `internal/auth` | Token store CRUD, expiry, file permissions |
| `internal/config` | Config loading, defaults, env overrides |
| `internal/tunnel` | Mux register/resolve/cancel/timeout, protocol encoding |
| `internal/server` | HTTP routing, auth middleware, subdomain extraction |
| `internal/client` | LocalProxy: HTTP methods, headers, status codes, large bodies |
| `test/integration` | End-to-end: HTTP methods, body passthrough, headers, timeouts (504), subdomain conflicts, concurrent clients, large responses |

All tests use real HTTP servers and real WebSocket connections — no I/O mocks.

---

### Project Structure

```
kp-gruuk/
├── cmd/
│   ├── gruuk/              # Client binary entrypoint
│   └── gruuk-server/       # Server binary entrypoint
├── internal/
│   ├── auth/               # Okta device flow, JWT validation, token store
│   ├── client/             # CLI commands, reconnect loop, local HTTP proxy
│   ├── config/             # Server and client config loading
│   ├── server/             # HTTP server, subdomain routing, visitor proxy, registry
│   └── tunnel/             # WebSocket protocol types, request multiplexer (Mux)
├── test/
│   └── integration/        # End-to-end tunnel tests
├── deploy/
│   └── terraform/          # Reference Terraform (ALB + ECS + Route53 + ACM)
├── assets/                 # Logo images
├── .github/
│   └── workflows/
│       └── deploy.yml      # CI/CD pipeline
├── Dockerfile.server       # Server container image
├── docker-compose.yml      # EC2 production deployment
├── install.sh              # One-line installer
└── Makefile
```

---

### Build

```sh
make build          # Build both binaries to ./bin/
make build-server   # Server only: ./bin/gruuk-server
make build-client   # Client only: ./bin/gruuk
make release        # Cross-compile all 8 platform binaries to ./bin/
make docker         # Build the server Docker image
make lint           # Run golangci-lint
make clean          # Remove ./bin/
```

The version string is embedded at build time:

```sh
VERSION=$(git describe --tags --always --dirty)
go build -ldflags "-s -w -X github.com/kashportsa/kp-gruuk/internal/client.version=${VERSION}" ./cmd/gruuk
```

---

### CI/CD Pipeline

Every push to `main` runs the full pipeline defined in `.github/workflows/deploy.yml`:

```
test  ->  version  ->  build-server  ->  deploy
                   ->  build-client  ->  release
```

| Job | What it does |
|-----|--------------|
| `test` | `go test -race ./...` — fails the whole pipeline if any test fails |
| `version` | Creates a Git tag from [conventional commits](https://www.conventionalcommits.org/). `feat:` bumps minor, `fix:` bumps patch, breaking change bumps major. Default: patch. |
| `build-server` | Builds Docker image, pushes to ECR with the new version tag and `latest` |
| `deploy` | SSHes to EC2, writes `.env`, pulls the new image, restarts with `docker compose up -d` |
| `build-client` | Cross-compiles `gruuk` for `darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64` |
| `release` | Creates a GitHub release with the auto-generated changelog and attaches the 4 client binaries |

**Commit message convention:**

```
feat: add WebSocket ping timeout        # bumps minor
fix: prevent panic on double Mux.Close  # bumps patch
feat!: change tunnel protocol version   # bumps major (breaking)
```

**Required GitHub Secrets:**

| Secret | Description |
|--------|-------------|
| `AWS_ACCESS_KEY_ID` | IAM credentials with ECR push + EC2 SG write permissions |
| `AWS_SECRET_ACCESS_KEY` | Corresponding secret |
| `EC2_SSH_PRIVATE_KEY` | PEM private key for the deployment EC2 instance |
| `GRUUK_OKTA_ISSUER` | e.g., `https://kashport.okta.com/oauth2/default` |
| `GRUUK_OKTA_CLIENT_ID` | Okta application client ID |

---

## Deployment

### Production Setup (EC2 + Docker Compose)

The production server runs as a Docker container on EC2, behind an Application Load Balancer with a wildcard ACM certificate.

**Infrastructure summary:**

| Component | Details |
|-----------|---------|
| EC2 | `t3.medium`, Amazon Linux 2023, app at `/opt/apps/gruuk/` |
| ALB | Internet-facing, idle timeout 3600s (required for long-lived WebSocket connections) |
| ACM | Wildcard certificate for `*.gk.kspt.dev` and apex `gk.kspt.dev` |
| Route53 | `A` alias records: `gk.kspt.dev` and `*.gk.kspt.dev` point to ALB |
| Ports | Container :8080 → host :8090 → ALB :443 |

**Manual deploy:**

```sh
# On the EC2 host at /opt/apps/gruuk/
cat > .env <<EOF
OKTA_ISSUER=https://kashport.okta.com/oauth2/default
OKTA_CLIENT_ID=0oa...
EOF
chmod 600 .env

export IMAGE=805149719934.dkr.ecr.us-east-2.amazonaws.com/gruuk-server:latest
docker compose pull
docker compose up -d
```

**Health check:**

```sh
curl https://gk.kspt.dev/_health
# ok
```

**Live logs:**

```sh
docker compose -f /opt/apps/gruuk/docker-compose.yml logs -f
```

---

### Server Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DOMAIN` | `gk.kspt.dev` | Base domain for tunnel subdomains |
| `LISTEN_ADDR` | `:8080` | Server listen address |
| `OKTA_ISSUER` | — | Okta authorization server URL (required) |
| `OKTA_CLIENT_ID` | — | Okta application client ID (required) |
| `SKIP_AUTH` | `false` | Disable JWT validation — never use in production |

The `--skip-auth` CLI flag overrides the `SKIP_AUTH` variable and is intended for local development only.

---

### Okta Application Requirements

The Gruuk Okta app must be configured as a **Native** (public client) application with:

| Setting | Required value |
|---------|---------------|
| Application type | Native |
| Grant types | Device Authorization Code, Refresh Token |
| Token endpoint auth method | None (no client secret) |
| Scopes | `openid`, `email`, `profile`, `offline_access` |
| Authorization server policy | Must include the Gruuk `client_id` |
| Policy rule grant types | Must allow `urn:ietf:params:oauth:grant-type:device_code` |
| User assignment | Users or groups must be assigned to the app |

---

### Reference Terraform (ALB + ECS)

A reference Terraform configuration is available at `deploy/terraform/` for deploying the server to ECS Fargate. This documents the AWS resource model but is not used in the current EC2-based production setup.

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

---

## Security

- **Tunnel auth:** All connections require a valid Okta JWT. The server validates the signature (via JWKS), issuer, client ID (`cid` claim), and expiration on every connection attempt.
- **Token storage:** Tokens are written to `~/.gruuk/token.json` with `0600` permissions (owner read/write only). The config directory `~/.gruuk/` is created with `0700`.
- **Transport encryption:** All traffic is TLS-encrypted. The CLI connects via `wss://` and the ALB enforces HTTPS with TLS 1.3 (`ELBSecurityPolicy-TLS13-1-2-2021-06`).
- **Visitor access:** Traffic to `*.gk.kspt.dev` does not require authentication by default. The public URL is effectively accessible to anyone who knows it — do not expose services containing sensitive data without adding application-level auth to your local service.
- **Request limits:** Visitor request bodies are capped at 10 MB at the proxy. WebSocket frames are limited to 16 MB.
- **Subdomain isolation:** Each authenticated user holds at most one subdomain at a time. A second connection from the same email fails with `subdomain_taken` until the first disconnects.
- **No credentials in URLs:** Tokens are never passed as URL query parameters.

---

## License

Internal use only. Proprietary to Kashport.
