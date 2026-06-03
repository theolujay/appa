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

## Stack

Go, React, PostgreSQL, Railpack, BuildKit, Docker, and Caddy.

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
