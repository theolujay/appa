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

This Compose flow is the local development path. Production operations use the
Appa CLI: the operator installs the CLI locally, creates an instance profile,
and uses Ansible-backed commands to provision Appa Server on a remote VPS.

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

The first-time production flow is:

```bash
curl -fsSL https://appa.dev/install.sh | sh
appa instance init personal
appa instance set-host -i ~/.ssh/id_ed25519 personal root@203.0.113.10
appa preflight personal
appa setup personal
```

The CLI orchestrates remote setup and friendly error reporting. Ansible
performs host and Appa Stack state changes. The Appa Server API remains
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
make build/api  # Build the Go API binary for host and linux/amd64
make build/cli  # Build the Appa CLI binary
```

### Run CLI

```bash
make run/cli ARGS="instance init my-server"
make run/cli ARGS="instance list"
./bin/appa --help                # Built binary
go run ./cmd/cli --help          # Direct without build
```

## Project Structure

```text
.
├── cmd/
│   └── api/                # HTTP handlers, routing, middleware
│   └── cli/                # Appa CLI entry point
├── deploy/ansible/         # Playbooks, roles, inventory, Molecule tests
│   ├── playbooks/
│   │   ├── security-hardening.yml
│   │   └── compliance-scan.yml
│   ├── roles/
│   │   ├── kernel_hardening/    # sysctl, module disabling
│   │   ├── access_control/      # sudoers, pwquality, login.defs
│   │   ├── ssh_hardening/       # sshd config
│   │   ├── firewall/            # UFW rules
│   │   ├── audit/               # auditd install, config, rules
│   │   └── ... (docker, appa_stack, caddy — planned)
│   ├── group_vars/
│   └── dev/                # Vagrant development VM
├── internal/
│   ├── cli/                # CLI implementation
│   │   ├── app.go              # Root command
│   │   ├── commands/           # instance, preflight, setup, apply, status, logs, restart, upgrade
│   │   ├── config/             # TOML instance profiles
│   │   ├── ansible/            # Inventory generation + ansible-playbook runner
│   │   ├── ssh/                # SSH connectivity and command execution
│   │   └── output/             # Tables, checkmarks, progress output
│   ├── data/              # Database models and queries
│   ├── hub/               # WebSocket broadcast hub
│   ├── mailer/            # Email templating and delivery
│   ├── pipeline/          # Build → run → route orchestration
│   │   ├── pipeline.go        # Orchestrator
│   │   ├── builder.go         # Railpack builds
│   │   ├── runner.go          # Docker container lifecycle
│   │   └── router.go          # Caddy admin API integration
│   ├── validator/         # Input validation helpers
│   └── vcs/               # Binary version info
├── migrations/            # SQL migration files
├── scripts/               # Utility scripts (db-init, entrypoint)
├── ui/                    # React frontend (TanStack Router + Query)
├── docs/
│   ├── architecture.md
│   ├── roadmap.md
│   ├── ansible.md
│   ├── DEPLOY_STACK.md
│   └── cli-commands.md
├── Caddyfile
├── Dockerfile
├── Makefile
├── compose.yml
└── go.mod
```

## CLI And Ansible Structure

The CLI is a thin command router and user-experience layer. Command
definitions are near the CLI entry point; reusable behavior lives in internal
packages so it can be tested without invoking a terminal command.

```text
cmd/
└── cli/
    └── main.go              # Builds the `appa` binary

internal/cli/
├── app.go               # Root command construction
├── commands/
│   ├── instance.go      # appa instance init|set-host|list
│   ├── preflight.go     # appa preflight <name>
│   ├── setup.go         # appa setup <name> and appa apply <name>
│   └── operations.go    # appa status|logs|restart|upgrade <name>
├── config/              # TOML instance profiles (~/.appa/instances/<name>/config.toml)
├── ansible/             # Inventory generation and ansible-playbook runner
├── ssh/                 # SSH connectivity, command execution, identity file support
└── output/              # Tables, checkmarks, section headers
```

`cmd/cli/main.go` calls into `internal/cli`. Command handlers parse arguments,
load the selected instance profile, call a service package, and render output.
They do not contain SSH, YAML rendering, Ansible process management, or API
client logic inline.

`deploy/ansible/` owns every remote host mutation. The CLI generates temporary
inventory and variable files; Ansible installs packages, writes files,
configures Caddy, manages firewall rules, and starts Compose services. Commit
playbooks, roles, templates, and Molecule scenarios; do not commit operator-
generated inventories, secrets, or rendered `.env` files.

## Codebase Tour

Start with `cmd/api/main.go` to understand how the server bootstraps, then follow a request through `cmd/api/routes.go` → the relevant handler → `internal/pipeline/` to see how a deployment is triggered end to end. `internal/hub/hub.go` is the WebSocket broadcast layer worth understanding early if you are touching anything log-related.

For the CLI, start with `cmd/cli/main.go` for command routing and local
instance-profile handling, then follow the Ansible invocation path under
`deploy/ansible/`. Do not duplicate the deployment pipeline in the CLI; project
deployments should call the Appa Server API.

For the full architecture design and design decisions, see [`ARCHITECTURE.md`](./docs/architecture.md).

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

## CLI Reference

```bash
appa instance init <name>              # Create a local instance profile
appa instance list                     # List known Appa instances
appa instance set-host <name> <target> # Set SSH target, e.g. root@203.0.113.10
  -i, --identity-file <path>           # SSH private key for this instance
appa preflight <name>                  # Validate SSH, OS, ports, DNS, and inputs
appa setup <name>                      # First-time remote Appa Server setup
  --force                              # Skip preflight checks
  --tags, --skip-tags                  # Pass Ansible tag filters
appa apply <name>                      # Re-apply instance config idempotently
  --tags, --skip-tags                  # Pass Ansible tag filters
appa status <name>                     # Show remote Appa Stack health
appa logs <name>                       # Tail Appa Stack logs
  -s, --service <name>                 # Filter to one service
  -n, --tail <lines>                   # Number of lines (default 50)
appa restart <name>                    # Restart the Appa Stack
  -s, --service <name>                 # Restart only one service
appa upgrade <name>                    # Upgrade remote Appa Stack images
  --version <tag>                      # Pin to a specific version tag
```

Longer term, project-level commands can use the Appa Server API for developer
workflows such as `appa deploy`, `appa logs`, `appa env`, and rollbacks.

## Coding Conventions

- Follow existing patterns for imports, error handling, and naming.
- Run `make tidy` and `make audit` before committing.
