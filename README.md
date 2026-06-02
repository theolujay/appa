# Appa

Appa is a self-hostable deployment platform. Bring your VPS, connect your
domain, and deploy apps from Git or ZIP uploads without writing Dockerfiles or
managing routing by hand.

Appa is moving toward a CLI-first operator experience: install the Appa CLI on
your own machine, then use it to provision and manage an Appa Server instance on
a remote VPS.

![Appa dashboard](main.png)

## Features

- Deploy from Git URLs or ZIP uploads.
- Build apps without writing Dockerfiles using Railpack and BuildKit.
- Stream build and runtime logs over WebSocket.
- Route each deployment through Caddy.
- Restore routes for running deployments after restarts.
- Manage remote Appa instances through the planned Appa CLI.

## Stack

Go, React, PostgreSQL, Railpack, BuildKit, Docker, Caddy, and planned
Ansible-backed remote provisioning.

## Production Direction

The intended production path is:

```bash
curl -fsSL https://appa.dev/install.sh | sh
appa instance init personal
appa instance set-host personal root@203.0.113.10
appa preflight personal
appa setup personal
```

`appa.dev/install.sh` installs the local Appa CLI. The CLI then connects to the
VPS over SSH and uses Ansible-backed workflows to prepare the host, write Appa
Server configuration, and start the remote Appa Stack.

## Local Development

```bash
git clone https://github.com/theolujay/appa.git
cd appa
cp .env.example .env
docker compose up --build
```

Open [http://localhost](http://localhost). For environment setup, available
commands, and API routes, see [CONTRIBUTING.md](CONTRIBUTING.md).

## Documentation

- [Architecture](ARCHITECTURE.md)
- [Roadmap](ROADMAP.md)
- [Contributing](CONTRIBUTING.md)
