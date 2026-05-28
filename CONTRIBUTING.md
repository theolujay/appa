# Contributing

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

## Available Commands

### Development

```bash
make run/api        # Run the Go API locally (requires PostgreSQL)
make db/psql        # Connect to the database via psql
make db/migrations/new name=<name>  # Create a new migration
make db/migrations/up               # Apply pending migrations
```

### Quality Control

```bash
make tidy    # Tidy modules and format Go files
make audit   # Vet, lint, and run tests
```

### Build

```bash
make build/api  # Build the Go binary for host and linux/amd64
```

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
└── go.mod
```

## Codebase Tour

Start with `cmd/api/main.go` to understand how the server bootstraps, then follow a request through `cmd/api/routes.go` → the relevant handler → `internal/pipeline/` to see how a deployment is triggered end to end. `internal/hub/hub.go` is the WebSocket broadcast layer worth understanding early if you are touching anything log-related.

For the full architecture design and design decisions, see [`ARCHITECTURE.md`](./ARCHITECTURE.md).

## API Reference

All routes are prefixed with `/v1/` and proxied through Caddy.

### Public

| Method | Path | Description |
|---|---|---|
| `GET` | `/v1/healthcheck` | Health check with env and version info |
| `POST` | `/v1/users` | Register a new user (sends activation email) |
| `PUT` | `/v1/users/activated` | Activate account via email token |
| `POST` | `/v1/tokens/authentication` | Log in, receive a bearer token |

### Authenticated

| Method | Path | Description |
|---|---|---|
| `GET` | `/v1/deployments` | List deployments — supports `?status=`, `?page=`, `?sort=` |
| `POST` | `/v1/deployments` | Trigger a Git-based deployment |
| `POST` | `/v1/deployments/upload` | Deploy via ZIP file (multipart) |
| `PATCH` | `/v1/deployments/{id}` | Cancel an active deployment or stop a container |
| `GET` | `/v1/deployments/{id}/logs` | WebSocket endpoint for live log streaming |

## Coding Conventions

- **No comments in production code** unless they explain a non-obvious trade-off.
- Follow existing patterns for imports, error handling, and naming.
- Run `make tidy` and `make audit` before committing.
- Architecture decisions belong in `ARCHITECTURE.md`, not in code comments.
