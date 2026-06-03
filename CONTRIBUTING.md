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

This Compose flow is the local development path. Production operations are
moving toward the Appa CLI: the operator installs the CLI locally, creates an
instance profile, and uses Ansible-backed commands to provision Appa Server on a
remote VPS.

## Product Surfaces

Use these names consistently when discussing or documenting Appa:

| Surface | Meaning |
|---|---|
| Appa | The whole product. |
| Appa CLI | Local operator/developer command-line tool, binary `appa`. |
| Appa Server | Remote API, dashboard, and deployment runtime. |
| Appa Instance | One remote Appa Server installation managed by the CLI. |
| Appa Stack | Server-side services: API, UI, PostgreSQL, BuildKit, Caddy. |

`appa.dev/install.sh` should install the local CLI. It should not be documented
as a script that is run directly on the VPS to install the server stack.

The planned first-time production flow is:

```bash
curl -fsSL https://appa.dev/install.sh | sh
appa instance init personal
appa instance set-host personal root@203.0.113.10
appa preflight personal
appa setup personal
```

The CLI should orchestrate remote setup and friendly error reporting. Ansible
should perform host and Appa Stack state changes. The Appa Server API remains
the authority for deployments, builds, app containers, routes, logs, users, and
tokens.

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
├── cmd/cli/           # Planned Appa CLI entry point
├── cmd/api/           # HTTP handlers, routing, middleware
├── deploy/ansible/    # Planned playbooks, roles, inventory templates, Molecule tests
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
│   └── vcs/           # Binary version info
├── migrations/        # SQL migration files
├── scripts/           # Utility scripts (db-init, etc.)
├── ui/                # React frontend (TanStack Router + Query)
├── Caddyfile
├── Dockerfile
├── Makefile           # Dev workflow: run, build, migrate, audit
├── compose.yml
└── go.mod
```

## Planned CLI And Ansible Structure

The CLI should be a thin command router and user-experience layer. Keep command
definitions near the CLI entry point, but put reusable behavior in internal
packages so it can be tested without invoking a terminal command.

```text
cmd/
├── api/
│   └── main.go
└── cli/
    └── main.go              # Builds the `appa` binary

internal/
├── cli/
│   ├── app.go               # Root command construction and global options
│   ├── commands/
│   │   ├── instance.go      # `appa instance ...`
│   │   ├── preflight.go     # `appa preflight <instance>`
│   │   ├── setup.go         # `appa setup <instance>`
│   │   ├── apply.go         # `appa apply <instance>`
│   │   ├── status.go        # `appa status <instance>`
│   │   └── logs.go          # `appa logs <instance> <service>`
│   ├── config/              # Local instance profiles and redaction
│   ├── ansible/             # Inventory generation and ansible-playbook runner
│   ├── ssh/                 # SSH/preflight helpers
│   └── output/              # Tables, prompts, progress, JSON output
└── ...

deploy/
└── ansible/
    ├── ansible.cfg
    ├── requirements.yml
    ├── playbooks/
    │   ├── preflight.yml
    │   ├── setup.yml
    │   ├── apply.yml
    │   ├── status.yml
    │   └── upgrade.yml
    ├── roles/
    │   ├── appa_preflight/
    │   ├── docker/
    │   ├── appa_stack/
    │   ├── caddy/
    │   ├── firewall/
    │   └── hardening/
    ├── inventory/
    │   └── README.md        # CLI-generated inventories are not committed
    └── molecule/
        └── appa_stack/
```

`cmd/cli/main.go` should only call into `internal/cli`. Command handlers should
parse arguments, load the selected instance profile, call a service package, and
render output. They should not contain SSH, YAML rendering, Ansible process
management, or API client logic inline.

`deploy/ansible/` should own every remote host mutation. The CLI may generate
temporary inventory and variable files, but Ansible should install packages,
write files, configure Caddy, manage firewall rules, and start Compose services.
Commit playbooks, roles, templates, and Molecule scenarios; do not commit
operator-generated inventories, secrets, or rendered `.env` files.

## Codebase Tour

Start with `cmd/api/main.go` to understand how the server bootstraps, then follow a request through `cmd/api/routes.go` → the relevant handler → `internal/pipeline/` to see how a deployment is triggered end to end. `internal/hub/hub.go` is the WebSocket broadcast layer worth understanding early if you are touching anything log-related.

When the CLI is added, start with `cmd/cli/` for command routing and local
instance-profile handling, then follow the Ansible invocation path under
`deploy/ansible/`. Do not duplicate the deployment pipeline in the CLI; project
deployments should call the Appa Server API.

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

## Planned CLI Reference

The CLI surface is not implemented yet, but this is the intended vocabulary for
contributors to design around:

```bash
appa instance init <name>              # Create a local instance profile
appa instance list                     # List known Appa instances
appa instance show <name>              # Show redacted instance config
appa instance set-host <name> <target> # Set SSH target, e.g. root@203.0.113.10
appa preflight <name>                  # Validate SSH, OS, ports, DNS, and inputs
appa setup <name>                      # First-time remote Appa Server setup
appa apply <name>                      # Re-apply instance config idempotently
appa status <name>                     # Show remote Appa Stack health
appa logs <name> <service>             # Show logs for api, ui, db, caddy, buildkit
appa restart <name>                    # Restart the Appa Stack
appa upgrade <name>                    # Upgrade remote Appa Server assets
appa uninstall <name>                  # Remove the remote stack, with safeguards
```

Longer term, project-level commands can use the Appa Server API for developer
workflows such as `appa deploy`, `appa logs`, `appa env`, and rollbacks.

## Coding Conventions

- Follow existing patterns for imports, error handling, and naming.
- Run `make tidy` and `make audit` before committing.
