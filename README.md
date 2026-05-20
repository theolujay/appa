# Appa

**A minimal, zero-config deployment platform that builds, runs, and routes your apps. No Dockerfile required.**

Named after Aang's flying bison, because why not. Appa carries your code from source to live URL with as little friction as possible.

![Appa screenshot](main.png)

---

## How It Works

```text
User/UI  ──►  Go Backend  ──►  Railpack (Build)  ──►  Docker (Run)  ──►  Caddy (Route)
   ▲              │  ▲              │                    │
   │              │  │              └────── BuildKit ────┘
   │              │  │
   │              │  └────────── PostgreSQL (Data & Logs)
   │              │
   └──────────────┴── Live Logs via WebSockets · Auth via Bearer Tokens
```

Point Appa at a Git URL or a ZIP file. It detects your stack, builds an optimized container image using Railpack and BuildKit, runs it via the Docker SDK, and provisions a `<id>.localhost` subdomain through Caddy's admin API, all without you touching a config file.

---

## The Stack

| Concern | Tool | Why |
|---|---|---|
| Backend | [Go](https://go.dev/) | Concurrency for the build pipeline -- low overhead |
| Build | [Railpack](https://railpack.com/) + [BuildKit](https://docs.docker.com/build/buildkit/) | Zero-config, language-agnostic image builds |
| Orchestration | Moby (Docker) SDK | Full container lifecycle management |
| Routing | [Caddy](https://caddyserver.com/) Admin API | Dynamic subdomain provisioning at runtime |
| Persistence | [PostgreSQL](https://www.postgresql.org/) | Deployments, users, tokens, and logs |
| Realtime | [WebSockets](https://pkg.go.dev/github.com/gorilla/websocket) | Live build and runtime log streaming |
| Frontend | React + TanStack Router/Query | Lightweight, fast client |

---

## Supported Runtimes

Appa is language-agnostic. Railpack inspects your code and builds an optimized image for:

- Node.js (npm, yarn, pnpm)
- Python (pip, poetry)
- Go
- Rust
- Static sites (HTML/CSS/JS)

No `Dockerfile` needed in any of these cases.

---

## Features

- **Zero-config builds** -- deploy from a Git URL or ZIP upload.
- **Live log streaming** -- watch build and runtime logs in real time over WebSockets.
- **Dynamic subdomains** -- every deployment gets its own `<id>.localhost` URL automatically.
- **Self-healing routes** -- on restart, Appa resyncs with Caddy and restores all active routes.
- **Environment variables** -- inject secrets and config per deployment.
- **Auth** -- register, activate via email token, authenticate with bearer tokens.
- **Deployment management** -- list, filter by status, paginate, cancel, and stop deployments.

---

## Getting Started

**Prerequisites:** Docker and Docker Compose.

```bash
git clone https://github.com/theolujay/appa.git
cd appa
cp .env.example .env
docker compose up --build
```

Open [http://localhost](http://localhost) in your browser.

`docker compose up` starts five services:

| Service | Role |
|---|---|
| `db` | PostgreSQL |
| `api` | Go backend |
| `buildkit` | BuildKit daemon |
| `caddy` | Reverse proxy and router |
| `ui` | Vite dev server |

---

## API

All routes are prefixed with `/v1/` and proxied through Caddy.

**Public**

| Method | Path | Description |
|---|---|---|
| `GET` | `/v1/healthcheck` | Health check with env and version info |
| `POST` | `/v1/users` | Register a new user (sends activation email) |
| `PUT` | `/v1/users/activated` | Activate account via email token |
| `POST` | `/v1/tokens/authentication` | Log in, receive a bearer token |

**Authenticated**

| Method | Path | Description |
|---|---|---|
| `GET` | `/v1/deployments` | List deployments — supports `?status=`, `?page=`, `?sort=` |
| `POST` | `/v1/deployments` | Trigger a Git-based deployment |
| `POST` | `/v1/deployments/upload` | Deploy via ZIP file (multipart) |
| `PATCH` | `/v1/deployments/{id}` | Cancel an active deployment or stop a container |
| `GET` | `/v1/deployments/{id}/logs` | WebSocket endpoint for live log streaming |

---

## Roadmap

These are the next things we are working toward, roughly in priority order:

- **Build caching** -- mount a persistent volume for Railpack's cache so rebuilds skip redundant work.
- **Resource limits** -- use the Docker API to enforce CPU and memory caps per container.
- **Rollbacks** -- switch instantly to a previous successful image tag when a deployment goes wrong.
- **Horizontal scaling** -- deploy multiple instances of the same app and load balance across them.
- **Reliability hardening** -- close edge cases in the pipeline to reduce silent failures.

---

## Project Structure

```text
.
├── cmd/api/           # HTTP handlers, routing, middleware
├── internal/
│   ├── data/          # Database models and queries
│   ├── hub/           # WebSocket broadcast hub
│   ├── mailer/        # Email templating and delivery
│   ├── pipeline/      # Build → run → route orchestration
│   │   ├── pipeline.go    # Orchestrator
│   │   ├── builder.go     # Railpack builds
│   │   ├── runner.go      # Docker container lifecycle
│   │   └── router.go      # Caddy admin API integration
│   ├── validator/     # Input validation helpers
│   └── vcs/           # Build version injection
├── migrations/        # SQL migration files
├── scripts/           # Utility scripts (db-init, etc.)
├── ui/                # React frontend (TanStack Router + Query)
├── Caddyfile
├── Dockerfile
├── Makefile           # Dev workflow: run, build, migrate, audit
├── compose.yml
├── go.mod
└── main.png
```

---

## Contributing

The project is actively being developed. If you'd like to work on this, start with `cmd/api/main.go` to understand how the server bootstraps, then follow a request through `routes.go` → the relevant handler → `internal/pipeline/` to see how a deployment is triggered end to end. `internal/hub/hub.go` is the WebSocket broadcast layer worth understanding early if you are touching anything log-related.