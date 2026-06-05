# Appa

Appa is a self-hostable deployment platform. Bring your VPS, connect your
domain, and deploy apps from Git or ZIP uploads without writing Dockerfiles or
managing routing by hand.

![Appa dashboard](main.png)

## Features

- Deploy from Git URLs or ZIP uploads.
- Build apps without writing Dockerfiles using Railpack and BuildKit.
- Stream build and runtime logs over WebSocket.
- Route each deployment through Caddy.
- Restore routes for running deployments after restarts.
- **CLI** (`appa`) for remote instance management, provisioning, and operations.
- **Ansible** playbooks for security hardening and server setup, tested with
  Molecule.

## Stack

Go, React, PostgreSQL, Railpack, BuildKit, Docker, and Caddy.

## Local Setup

```bash
git clone https://github.com/theolujay/appa.git
cd appa
cp .env.example .env
docker compose up --build -d
```

Open [http://localhost](http://localhost). For environment setup, available
commands, and API routes, see [CONTRIBUTING.md](CONTRIBUTING.md).

## CLI Quick Start

### Install

```bash
# Interactive (prompts before installing)
curl -fsSL https://appa.theolujay.dev/install.sh | sh

# Non-interactive (for scripts)
curl -fsSL https://appa.theolujay.dev/install.sh | sh -s -- -y
```

Or build from source:

```bash
make build/cli
./bin/appa --help
```

### Use

```bash
appa instance init my-server
appa instance set-host my-server root@203.0.113.10
appa preflight my-server
appa setup my-server
```

## Documentation

- [Architecture](docs/architecture.md)
- [Roadmap](docs/roadmap.md)
- [Contributing](CONTRIBUTING.md)
- [Ansible Reference](docs/ansible.md)

