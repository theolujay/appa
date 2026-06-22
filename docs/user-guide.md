# Appa User Guide

## What is Appa?

Appa is a self-hosted deployment platform you control from your terminal.
You provision a VPS once with the CLI, then deploy projects with a single
command — no Dockerfiles, no web server config, no manual routing.

---

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [CLI Installation](#cli-installation)
3. [Setting Up an Instance](#setting-up-an-instance)
4. [Configuration Reference](#configuration-reference)
5. [Instance Operations](#instance-operations)
6. [Deploying Projects](#deploying-projects)
7. [CLI Command Reference](#cli-command-reference)

---

## Prerequisites

- **A VPS** — any provider (DigitalOcean, Hetzner, Linode, etc.).
  - Fresh Ubuntu 22.04+ or Debian.
  - SSH access as `root`.
  - Ports `80` (HTTP) and `443` (HTTPS) open.
- **Your local machine** — macOS or Linux.

---

## CLI Installation

### Build from source

```bash
git clone https://github.com/theolujay/appa.git
cd appa
make build/cli
./bin/appa --help
```

---

## Setting Up an Instance

An "instance" is a single Appa Server installation on a VPS. The CLI stores
its configuration locally in `~/.appa/instances/<name>/config.toml`.

### 1. Create an instance config

```bash
appa instance init my-server
```

This creates the config and records your current OS username as the
**operator user** — an optional sudo user for manual SSH. You can override
it with `--op-name`:

```bash
appa instance init my-server --op-name jane
```

### 2. Set the SSH target

```bash
appa instance set-host my-server root@203.0.113.10 -i ~/.ssh/id_ed25519
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
and checks for existing Docker installations.

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

### Instance config (`~/.appa/instances/<name>/config.toml`)

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
| `name` | Instance name (matches the directory) |
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

The quickest way to edit an instance config is:

```bash
appa instance edit my-server
```

This opens the TOML file in your `$EDITOR` and validates it on save. If
validation fails, you can re-edit or abort (changes are reverted).

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
| `target` | Target instance name (must match an existing instance) |

Edit with:

```bash
appa project edit my-app
```

---

## Instance Operations

Once your instance is set up, you can manage it remotely:

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

After editing an instance config (e.g. changing the domain or SMTP settings),
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

Projects let you deploy source code from your local machine to an instance.

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

Lets you change the source path or target instance.

### 3. Deploy

```bash
appa deploy my-app
```

What happens under the hood:

1. The CLI **rsyncs** your source directory to `/opt/appa/builds/my-app/` on
   the target instance using SSH.
2. It calls the Appa API with the server-side path.
3. The API runs the pipeline: Railpack detects the runtime, BuildKit builds
   the image, Docker starts the container, and Caddy registers a route.
4. The CLI prints the deployment ID, status, and creation time.

Hide rsync transfer progress:

```bash
appa deploy my-app --quiet
```

---

## CLI Command Reference

### `appa instance init <name>`

Create a new instance config.

| Flag | Description |
|---|---|
| `--op-name` | Operator username (default: current OS user) |

---

### `appa instance edit <name>`

Open instance config in `$EDITOR` with validation on save.

---

### `appa instance set-host <name> <target>`

Set the SSH target for an instance.

| Flag | Description |
|---|---|
| `-i`, `--identity-file` | Path to SSH private key |
| `--skip-verify` | Skip host key verification |

---

### `appa instance list`

List all configured instances.

---

### `appa preflight <name>`

Run preflight checks on a target instance.

| Flag | Description |
|---|---|
| `--skip-verify` | Skip SSH host key verification |

---

### `appa setup <name>`

First-time provisioning of an Appa instance.

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

Show instance health and service status.

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
| `-t`, `--target` | Target instance name |
| `-n`, `--name` | Project name (inferred from source if not specified) |

---

### `appa project edit <name>`

Open project config in `$EDITOR` with validation on save.

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
