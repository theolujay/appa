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

```bash
make build/cli
./bin/appa instance init my-server
./bin/appa instance set-host my-server root@203.0.113.10
./bin/appa preflight my-server
./bin/appa setup my-server
```

## Documentation

- [Architecture](docs/architecture.md)
- [Roadmap](docs/roadmap.md)
- [Contributing](CONTRIBUTING.md)
- [Ansible Reference](docs/ansible.md)
- [Deploy Stack Plan](docs/DEPLOY_STACK.md)
