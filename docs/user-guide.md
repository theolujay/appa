# User Guide

## What is Appa?

Appa is a self-hosted deployment platform you control from your terminal.
You provision a VPS once with the CLI, then deploy projects with a single
command: no Dockerfiles, no web server config, no manual routing.

---

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [CLI Installation](#cli-installation)
3. [Setting Up a Server](#setting-up-a-server)
4. [Configuration Reference](#configuration-reference)
5. [Server Operations](#server-operations)
6. [Deploying Projects](#deploying-projects)
7. [Project Lifecycle & Environment Variables](#project-lifecycle--environment-variables)
8. [CLI Command Reference](#cli-command-reference)

---

## Prerequisites

- **A VPS** — any provider (DigitalOcean, Hetzner, Linode, etc.).
  - Fresh Ubuntu 22.04+ or Debian.
  - SSH access as `root`.
  - Ports `80` (HTTP) and `443` (HTTPS) open.
- **Your local machine** — macOS or Linux.

---

## CLI Installation

### Install script (recommended)

```bash
curl -fsSL https://appa.theolujay.dev/install.sh | sh
```

The script downloads the latest binary, verifies its checksum, and installs it
to a directory in your `$PATH`. It also automatically provisions **uv** and
**Ansible** in an isolated venv under `~/.appa/ansible/`: no system-wide
Python dependencies required.

### Build from source

```bash
git clone https://github.com/theolujay/appa.git
cd appa
make build/cli
./bin/appa --help
```

---

## Setting Up a Server

A "server" is a single Appa Server instance on a VPS. The CLI stores
its configuration locally in `~/.appa/servers/<name>/config.toml`.

### 1. Create a server config

```bash
appa server init my-server
```

This creates the config and records your current OS username as the
**operator user** — an optional sudo user for manual SSH. You can override
it with `--op-name`:

```bash
appa server init my-server --op-name jane
```

### 2. Set the SSH target

```bash
appa server set-host my-server root@203.0.113.10 -i ~/.ssh/id_ed25519
```

The target format is `user@host` or `user@host:port`. The CLI tests the
connection before saving.

Flags:

| Flag | Description |
|---|---|
| `-i`, `--identity-file` | Path to your SSH private key |
| `--skip-verify` | Skip host key verification (e.g. for ephemeral VMs) |

### 3. Run preflight checks

```bash
appa preflight my-server
```

Preflight validates SSH connectivity, OS compatibility, required ports,
and checks for existing Docker installations. Use `--no-tty` to run in
non-interactive mode (e.g. CI pipelines).

| Flag | Description |
|---|---|
| `--no-tty` | Run in non-interactive mode (plain text output) |
| `--skip-verify` | Skip SSH host key verification |

### 4. Provision the server

```bash
appa setup my-server --op-key "$(cat ~/.ssh/id_ed25519.pub)"
```

This is the big one. Setup does all of the following automatically:

1. **Security hardening** — locks down SSH (`PermitRootLogin no`, key-only auth),
   configures UFW firewall, applies kernel hardening, enables audit logging.
2. **Creates users** — a `deploy` user (for CLI automation) and an optional
   `operator` user (your username, for manual SSH), each with your SSH key.
3. **Installs Docker** — Docker Engine and Compose plugin.
4. **Deploys the Appa Stack** — pulls container images for the API, database,
   BuildKit, Caddy, and the web UI, then starts everything via Docker Compose.
5. **Waits for health** — polls the API health endpoint until it responds.

After setup, subsequent SSH operations use the `deploy` user (never `root`).

Flags:

| Flag | Description |
|---|---|
| `--op-key` | **Required.** SSH public key for the deploy and operator users |
| `--force` | Skip preflight checks |
| `--tags` | Only run specific Ansible tags |
| `--skip-tags` | Skip specific Ansible tags |
| `--skip-verify` | Skip SSH host key verification |

### 5. Access the dashboard

Setup prints the dashboard URL. Open it in a browser, register your admin
account, and you're ready to deploy apps through the UI.

---

## Configuration Reference

### Server config (`~/.appa/servers/<name>/config.toml`)

```toml
name = "my-server"
ssh_host = "203.0.113.10"
ssh_user = "deploy"
ssh_port = 22
ssh_identity_file = "/home/you/.ssh/id_ed25519"
skip_ssh_verify = false
operator_user_name = "jane"
setup_done = true
base_api_url = "http://203.0.113.10"
```

| Field | Description |
|---|---|
| `name` | Server name (matches the directory) |
| `ssh_host` | VPS IP or hostname |
| `ssh_user` | SSH user — `root` before setup, `deploy` after |
| `ssh_port` | SSH port (default 22) |
| `ssh_identity_file` | Path to SSH private key |
| `skip_ssh_verify` | Skip host key checking |
| `operator_user_name` | Optional sudo user for manual SSH |
| `domain` | Domain name (optional — for DNS-based TLS) |
| `cloudflare_token` | Cloudflare API token (optional — for automatic wildcard TLS) |
| `smtp_*` | SMTP credentials (optional — for email notifications) |
| `setup_done` | Whether `appa setup` has completed |
| `base_api_url` | API base URL (set automatically after setup) |

The quickest way to edit a server config is:

```bash
appa server edit my-server
```

This opens the TOML file in your `$EDITOR` and validates it on save. If
validation fails, you can re-edit or abort (changes are reverted). If you
rename the server in the editor, the config directory is renamed
automatically.

### Project config (`~/.appa/projects/<name>/config.toml`)

```toml
name = "my-app"
source = "/home/you/code/my-app"
target = "my-server"
```

| Field | Description |
|---|---|
| `name` | Project name (matches the directory) |
| `source` | Absolute path to the project's source directory |
| `target` | Target server name (must match an existing server) |

Edit with:

```bash
appa project edit my-app
```

---

## Server Operations

Once your server is set up, you can manage it remotely:

### Check status

```bash
appa status my-server
```

Shows SSH connectivity, API health, Docker Compose service status, and
disk usage.

### Tail logs

```bash
appa logs my-server
```

Streams logs from all Appa Stack services. Filter to a specific service:

```bash
appa logs my-server -s api
appa logs my-server -s buildkit
```

Control the number of initial lines:

```bash
appa logs my-server -n 100
```

### Apply configuration changes

After editing a server config (e.g. changing the domain or SMTP settings),
re-apply them:

```bash
appa apply my-server
```

This re-runs the Ansible playbooks idempotently.

### Restart services

```bash
appa restart my-server
```

Restart all Appa Stack services, or just one:

```bash
appa restart my-server -s api
```

### Upgrade

```bash
appa upgrade my-server
```

Pulls the latest Appa images and recreates all services. Pin to a specific
version:

```bash
appa upgrade my-server --version v0.2.0
```

---

## Deploying Projects

Projects let you deploy source code from your local machine to a server.

### 1. Create a project

```bash
appa project init /home/you/code/my-app --target my-server
```

- The project name is derived from the source directory name (e.g. `my-app`).
- Override with `--name`:

  ```bash
  appa project init /home/you/code/my-app --target my-server --name production-app
  ```

### 2. (Optional) Edit project config

```bash
appa project edit my-app
```

Lets you change the source path or target server.

### 3. Deploy

```bash
appa deploy my-app
```

What happens under the hood:

1. The CLI **rsyncs** your source directory to `/opt/appa/builds/my-app/` on
   the target server using SSH.
2. The CLI calls the API to **auto-create the project** (if it doesn't already
   exist on the server) and retrieves the project ID.
3. It triggers a deployment with the server-side path and project ID.
4. The API runs the pipeline: Railpack detects the runtime, BuildKit builds
   the image, Docker starts the container, and Caddy registers a route.
5. The CLI prints the deployment ID, status, and creation time.

Hide rsync transfer progress:

```bash
appa deploy my-app --quiet
```

---

## Project Lifecycle & Environment Variables

Once a project has been deployed, you can manage its lifecycle and
configuration through the CLI.

### View project logs

```bash
appa project logs my-app
```

Opens a WebSocket-powered TUI that streams build and runtime logs from the
latest deployment. Supports scrolling, follow mode, and keyboard navigation.

### Stop a project

```bash
appa project stop my-app
```

Stops the running container or cancels a pending deployment.

### Restart a project

```bash
appa project restart my-app
```

Stops the current deployment and triggers a new build and deploy.

### Manage environment variables

```bash
appa project env set my-app KEY=value
appa project env get my-app
appa project env unset my-app KEY
```

Environment variables are stored on the server and merged into the build
step alongside any deployment-specific variables.

---

## CLI Command Reference

### `appa server init <name>`

Create a new server config.

| Flag | Description |
|---|---|
| `--op-name` | Operator username (default: current OS user) |

---

### `appa server edit <name>`

Open server config in `$EDITOR` with validation on save. Renaming the
server in the editor renames the config directory automatically.

---

### `appa server set-host <name> <target>`

Set the SSH target for a server.

| Flag | Description |
|---|---|
| `-i`, `--identity-file` | Path to SSH private key |
| `--skip-verify` | Skip host key verification |

---

### `appa server ls`

List all configured servers.

---

### `appa preflight <name>`

Run preflight checks on a target server.

| Flag | Description |
|---|---|
| `--no-tty` | Run in non-interactive mode (plain text output) |
| `--skip-verify` | Skip SSH host key verification |

---

### `appa setup <name>`

First-time provisioning of an Appa server.

| Flag | Description |
|---|---|
| `--op-key` | **(Required.)** SSH public key for deploy and operator users |
| `--force` | Skip preflight checks |
| `--tags` | Only run specific Ansible tasks |
| `--skip-tags` | Skip specific Ansible tasks |
| `--skip-verify` | Skip SSH host key verification |

---

### `appa apply <name>`

Re-apply configuration changes idempotently.

| Flag | Description |
|---|---|
| `--tags` | Only run specific Ansible tasks |
| `--skip-tags` | Skip specific Ansible tasks |
| `--skip-verify` | Skip SSH host key verification |

---

### `appa status <name>`

Show server health and service status.

---

### `appa logs <name>`

Tail Appa Stack logs.

| Flag | Description |
|---|---|
| `-s`, `--service` | Filter to one service (`api`, `db`, `buildkit`, `caddy`, `ui`) |
| `-n`, `--tail` | Number of lines to show (default 50) |

---

### `appa restart <name>`

Restart Appa Stack services.

| Flag | Description |
|---|---|
| `-s`, `--service` | Restart only one service |

---

### `appa upgrade <name>`

Upgrade the Appa Stack to the latest version.

| Flag | Description |
|---|---|
| `--version` | Pin to a specific version tag |

---

### `appa project init <source>`

Create a new project.

| Flag | Description |
|---|---|
| `-t`, `--target` | Target server name |
| `-n`, `--name` | Project name (inferred from source if not specified) |

---

### `appa project edit <name>`

Open project config in `$EDITOR` with validation on save.

---

### `appa project logs <name>`

Stream WebSocket logs from the latest deployment in a TUI viewer.

---

### `appa project stop <name>`

Stop the running container or cancel a pending deployment.

---

### `appa project restart <name>`

Stop the current deployment and trigger a new build.

---

### `appa project env set <name> <key=value>`

Set an environment variable for a project.

---

### `appa project env get <name>`

List all environment variables for a project.

---

### `appa project env unset <name> <key>`

Remove an environment variable from a project.

---

### `appa deploy <project-name>`

Deploy an already initialized project.

| Flag | Description |
|---|---|
| `--quiet` | Suppress rsync progress output |

---

## Where to Go Next

- [Architecture Document](architecture.md) — deep dive into design decisions.
- [Contributing Guide](../CONTRIBUTING.md) — local development, API routes, coding conventions.
- [Roadmap](roadmap.md) — what's coming next.
