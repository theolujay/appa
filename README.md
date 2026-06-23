![GitHub Release](https://img.shields.io/github/v/release/theolujay/appa)
[![Ansible Tests](https://github.com/theolujay/appa/actions/workflows/ansible-tests.yml/badge.svg)](https://github.com/theolujay/appa/actions/workflows/ansible-tests.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/theolujay/appa)](https://goreportcard.com/report/github.com/theolujay/appa)
![GitHub License](https://img.shields.io/github/license/theolujay/appa)


Turn any VPS into your own zero-config deployment platform, effortlessly 🦬

Just **connect** your domain and **push** your code. **Deploy** apps instantly without Dockerfiles or playing sysadmin on hard mode. Even better: **manage** your **entire fleet** of VPS instances from **one terminal**, anywhere!
```
Operator Machine              Remote Server (VPS)
 └─ Appa CLI                    ├─ Caddy Gateway
     │                          ├─ Appa API + Dashboard
     SSH + Ansible + rsync      ├─ BuildKit + Railpack
     └──────────────────────────┼─ PostgreSQL
                                └─ Your containers
```

## Prerequisites

- A fresh Linux VPS with SSH access (key-based authentication).
- At least 4GB of RAM (2GB+ recommended for building images).
- (Optional) A Cloudflare API token for wildcard DNS and automatic TLS.

## Quick Start

```bash
curl -fsSL https://appa.theolujay.dev/install.sh | sh
appa instance init my-server
appa instance set-host my-server root@203.0.113.10 -i ~/.ssh/id_ed25519
appa preflight my-server
appa setup my-server
```

Once setup completes, open the dashboard URL to register your admin account.

## User Guide

See [docs/user-guide.md](docs/user-guide.md) for the full walkthrough — installation, instance management, project deployment, and CLI reference.

## Documentation

| Doc | For |
|---|---|
| [User Guide](docs/user-guide.md) | Setting up and using Appa day-to-day |
| [Architecture](docs/architecture.md) | Design decisions, invariants, and data paths |
| [Contributing](CONTRIBUTING.md) | Development setup, API routes, coding conventions |
| [Roadmap](docs/roadmap.md) | Completed milestones and planned features |
