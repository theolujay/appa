# Appa

[![CI](https://github.com/theolujay/appa/actions/workflows/ci.yml/badge.svg)](https://github.com/theolujay/appa/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/theolujay/appa)](https://goreportcard.com/report/github.com/theolujay/appa)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Appa is a self-hostable, production-ready deployment platform. Bring your own VPS, connect your domain, and deploy apps from Git repositories or ZIP uploads—without writing Dockerfiles, configuring web servers, or managing routing manually.

![Appa dashboard](main.png)

## Architecture Overview

```
Operator Machine (Local)             Remote Server (VPS)
 └─ Appa CLI (appa)                    ├─ Caddy Gateway (public proxy, automatic HTTPS)
     │                                 ├─ Appa API (Go, WS log broadcaster)
     │ SSH + Ansible (setup/apply)     ├─ React Dashboard (dashboard UI)
     └─────────────────────────────────┼─ BuildKit + Railpack (isolated Dockerless builds)
                                       ├─ PostgreSQL (persistent app metadata)
                                       └─ Deployed Containers (user application workloads)
```

For a deep dive into decisions, failure states, and data paths, see the [Architecture Document](docs/architecture.md).

## Features

- **Dockerless Builds:** Uses **Railpack** and **BuildKit** to automatically detect runtimes and build images without requiring Dockerfiles.
- **WebSocket Streaming:** Stream live build progress and runtime container logs directly to the dashboard or CLI.
- **Dynamic Routing:** Automatic Caddy reverse-proxy routing to running app containers.
- **Robust Restoration:** Automatically re-registers and restores application routes in Caddy after platform updates or server reboots.
- **Secure Provisioning:** Includes an Ansible-driven setup with UFW, SSH hardening, kernel-level optimizations, and CIS-compliant configuration audits out of the box.

## Requirements

- **Operator Machine (Local):** macOS or Linux with network connectivity to target VPS.
- **Remote Host (VPS):**
  - Fresh install of Ubuntu (22.04 LTS or newer recommended) or Debian.
  - SSH access with root or sudo permissions.
  - Ports `80` (HTTP) and `443` (HTTPS) open.

---

## Local Setup (Development)

To spin up a local development cluster of the Appa Stack:

```bash
# Clone the repository
git clone https://github.com/theolujay/appa.git
cd appa

# Configure the local environment
cp .env.example .env

# Start dev services (Go API, Vite UI dev server, PostgreSQL, Caddy, BuildKit)
docker compose up --build -d
```

Open [http://localhost](http://localhost) in your browser. For further environment setup, Makefile commands, and DB migrations, see [CONTRIBUTING.md](CONTRIBUTING.md).

---

## Production Setup & CLI Quick Start

Remote VPS servers are managed from your local machine using the Appa CLI.

### 1. Install Appa CLI

Install the `appa` CLI utility to `/usr/local/bin`:

```bash
curl -fsSL https://appa.theolujay.dev/install.sh | sh -s -- -y
```

*Alternatively, compile from source:*
```bash
make build/cli
./bin/appa --help
```

### 2. Configure and Provision the Host

Run these commands locally to configure your remote VPS instance:

```bash
# 1. Initialize local configuration file
appa instance init my-server

# 2. Configure target connection and SSH private key
appa instance set-host my-server root@203.0.113.10 -i ~/.ssh/id_ed25519

# 3. Perform automated preflight validation checks
appa preflight my-server

# 4. Provision VPS security hardening and deploy Appa Stack services
appa setup my-server
```

Once `appa setup` completes, the CLI will output the URL to access the React dashboard and configure your initial admin account.

### 3. Remote Instance Operations

You can manage, inspect, and update your remote instance using the Appa CLI:

- **Check status:** View the status and health of the remote Appa Stack services.
  ```bash
  appa status my-server
  ```
- **Stream logs:** Tail and stream service logs (with optional service filtering).
  ```bash
  # Tail all logs
  appa logs my-server

  # Filter by service (e.g., API)
  appa logs my-server -s api
  ```
- **Apply configuration changes:** Update environment variables or SSH targets and re-apply settings idempotently.
  ```bash
  appa apply my-server
  ```
- **Restart or Upgrade:** Perform zero-downtime upgrades or restart services.
  ```bash
  appa restart my-server
  appa upgrade my-server --version v0.1.2
  ```

---

## Documentation Index

- [Architecture Reference](docs/architecture.md) — Detailed diagram, invariants, and structural decisions.
- [Ansible Reference](docs/ansible.md) — Information on security roles, Molecule tests, and Vagrant local setups.
- [Project Roadmap](docs/roadmap.md) — Completed milestones and future features (including secrets encryption).
- [Contributing Guide](CONTRIBUTING.md) — API routes, local development commands, and coding conventions.
