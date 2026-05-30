# Appa

**Self-hostable zero-config deployment platform that builds, runs, and routes your apps.**

Named after Aang's flying bison. Appa's your buddy that carries your code from source to live URL with as little friction as possible. Run your own Railway-like platform on a fixed-price VPS instance.

For the full system design, design decisions, and roadmap, see [ARCHITECTURE.md](./ARCHITECTURE.md)

![Appa screenshot](main.png)

## The Stack

- **Go & WebSockets:** High-performance backend and live log streaming.
- **Railpack & BuildKit:** Language-agnostic, zero-config container builds.
- **Caddy:** Dynamic subdomain routing and automatic wildcard TLS.
- **PostgreSQL:** Reliable persistence for deployments and users.
- **React & TanStack:** Modern, snappy dashboard for management.

---

## Supported Runtimes

Appa is language-agnostic. Railpack inspects your code and builds an optimized image for:

- **Node.js** (npm, yarn, pnpm)
- **Python** (pip, poetry)
- **Go**, **Rust**, and **Static sites**

No `Dockerfile` needed.

---

## Core Features

- **Zero-config builds:** Deploy from a Git URL or ZIP upload.
- **Live log streaming:** Watch build and runtime logs in real-time.
- **Dynamic subdomains:** Every app gets its own URL automatically.
- **Self-healing routes:** Automatic recovery after system restarts.
- **Environment variables:** Manage secrets and config per deployment.
- **Built-in Auth:** Secure registration and token-based authentication.

---

## Getting Started

**Prerequisites:** Docker and Docker Compose.

```bash
git clone https://github.com/theolujay/appa.git
cd appa
cp .env.example .env
docker compose up --build
```

Open [http://localhost](http://localhost) in your browser to explore the dashboard.

---

## Documentation

- **[Architecture](./ARCHITECTURE.md):** Deep dive into design decisions, roadmap, and reference docs.
- **[Contributing](./CONTRIBUTING.md):** Local development setup, API reference, and project structure.

---
