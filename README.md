![GitHub Release](https://img.shields.io/github/v/release/theolujay/appa)
[![Ansible Tests](https://github.com/theolujay/appa/actions/workflows/ansible-tests.yml/badge.svg)](https://github.com/theolujay/appa/actions/workflows/ansible-tests.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/theolujay/appa)](https://goreportcard.com/report/github.com/theolujay/appa)
![GitHub License](https://img.shields.io/github/license/theolujay/appa)


Turn any VPS into your own zero-config deployment platform, effortlessly!

Just **connect** your domain and **push** your code. **Deploy** apps instantly without Dockerfiles or playing sysadmin on hard mode. Even better: **manage** your **entire fleet** of VPSs from **one terminal**, anywhere!

A single person could wear both hats: operator + developer.

```
   ╭─────── Operator ──────╮       ╭────── Developer ──────╮
   │                       │       │                       │
   │     ┌──────────┐      │       │      · git push       │
   │     │ Appa CLI │      │       |· appa deploy <project>│
   │     └────┬─────┘      │       │           │           │
   │          │            │       │           │           │
   │      SSH · rsync      │       │          API          │
   ╰──────────│────────────╯       ╰───────────│───────────╯
              │                                │
           manages                          deploys
              ▼                                ▼
╭── Fleet of Servers (VPS) ──┬──────────────────────────────────╮
│                            │                                  │
│  ┌─ nyc-prod ──────────┐   │   ┌─ lon-staging ─────────────┐  │
│  │  Blog API · Admin UI│   │   │  Client Dashboard (stg)   │  │
│  └─────────────────────┘   │   └───────────────────────────┘  │
│                    ◄───────┼───────►                          │
│  ┌─ fra-gateway ───────┐   │   ┌─ sfo-preview ─────────────┐  │
│  │  Auth · Webhook     │   │   │  PR previews · E2E tests  │  │
│  └─────────────────────┘   │   └───────────────────────────┘  │
│                    ◄───────┼───────►                          │
│  ┌─ ams-worker ────────┐   │   ┌─ (more)...────────────────┐  │
│  │  Queue · Cron jobs  │   │   │  Any VPS, anywhere        │  │
│  └─────────────────────┘   │   └───────────────────────────┘  │
╰────────────────────────────┴──────────────────────────────────╯
Each server runs: Caddy · Appa API · BuildKit · PostgreSQL · Containers
```

## Prerequisites

- A fresh Linux VPS with SSH access (key-based authentication).
- At least 4GB of RAM (2GB+ recommended for building images).
- (Optional) A Cloudflare API token for wildcard DNS and automatic TLS.

## Quick Start

```bash
curl -fsSL https://appa.theolujay.dev/install.sh | sh
appa server init my-server
appa server set-host my-server root@203.0.113.10 -i ~/.ssh/id_ed25519
appa preflight my-server
appa setup my-server
```

<!-- Once setup completes, open the dashboard URL to register your admin account.

The install script also automatically provisions **uv** and **Ansible** in an isolated venv under `~/.appa/ansible/`, so no system-wide Python dependencies required. -->

## Features

| Area | Capabilities |
|---|---|
| **Server management** | Initialize, provision, configure, and monitor any number of VPS servers from one CLI. |
| **Auto-deploy** | One command ships source via rsync and triggers the build pipeline. Projects are auto-created on the server. |
| **Env vars** | Manage per-project environment variables through `appa project env set/get/unset`. |
| **Project lifecycle** | View deployment logs, stop running deployments, or restart with `appa project logs/stop/restart`. |
| **Zero-config builds** | Railpack auto-detects runtimes — no Dockerfiles needed. |
| **Web dashboard** | React UI for deployment history, project management, and monitoring. |

## User Guide

See [docs/user-guide.md](docs/user-guide.md) for the full walkthrough — installation, server management, project deployment, environment variables, and CLI reference.

## Documentation

| Doc | For |
|---|---|
| [User Guide](docs/user-guide.md) | Installing the CLI, provisioning servers, deploying projects, managing env vars |
| [Architecture](docs/architecture.md) | Design decisions, invariants, data paths, and glossary |
| [Contributing](CONTRIBUTING.md) | Development setup, API routes, project structure, coding conventions |
| [Roadmap](docs/roadmap.md) | Completed milestones and planned features |
