# Appa: System Architecture

# Overview

Appa is a self-hostable, zero-config deployment platform designed to simplify the transition from source code to a live, secure URL. It accepts Git repositories or ZIP archives, builds optimized container images using Railpack and BuildKit, manages the container lifecycle via the Docker SDK, and handles dynamic routing through the Caddy Admin API.

The system is built on three core pillars:
1.  **Zero-Config for Developers:** Language-agnostic builds without requiring a `Dockerfile`.
2.  **Simplified Operations:** Single-command installation and automated VPS management.
3.  **Production Readiness:** Automatic wildcard TLS via DNS-01 challenges and self-healing routing.

```plaintext
        [ 1. PROVISION ]          [ 2. INSTALL ]          [ 3. DEPLOY ]
   Get any Ubuntu VPS   ──▶   appa.dev/install  ──▶   Paste your Git URL
   (DigitalOcean, etc.)      (One-command setup)      (Zero-config build)

           │                        │                       │
           ▼                        ▼                       ▼

   [  VPS IS READY  ]       [ PLATFORM LIVE ]       [  APP IS LIVE  ]
   Fixed monthly cost       TLS & DNS managed       app.yourdomain.com
```

# The Problem

Managed platforms (Railway, Render, Fly.io) provide excellent DX but often lead to unpredictable costs and limited infrastructure control. Setting up a private alternative traditionally requires deep expertise in reverse proxies, container orchestration, TLS management, and CI/CD pipelines.

Appa eliminates this complexity by packaging a complete platform stack into a single, cohesive system. It serves two primary roles:
*   **The Operator:** Manages the Appa instance on their own hardware.
*   **The Developer:** Deploys and manages applications through the Appa interface.

# System Architecture

The system is organized into three logical layers:

### 1. Network Boundary Layer (Caddy)
Caddy acts as the gateway for all inbound traffic on ports 80 and 443. It performs TLS termination (in production) and routes requests based on the host header to either the **Platform UI/API** or the **User Applications**.

### 2. Platform Layer (Services)
The Go API, PostgreSQL, BuildKit, and React UI run as Docker Compose services on a shared internal network (`appa_net`), inaccessible from the public internet except through Caddy. User containers launched by the deployment pipeline are also connected to `appa_net` at startup, making them reachable by Caddy via their container name without routing through the host.

### 3. Pipeline Layer (Orchestration)
The sequence of operations executed by the Go API for every deployment:
`Source Acquisition` ──▶ `Plan Generation` ──▶ `Image Build` ──▶ `Container Start` ──▶ `Route Registration`

# Key Design Decisions

**Installation & Bootstrapping (Planned)**
Appa prioritizes a "Batteries Included" installation. The primary method will be a bash script (`appa.dev/install.sh`) that automates Docker installation, environment configuration, and stack deployment. For advanced infrastructure management, an **Ansible Playbook** (planned for the `ansible/` directory) will handle OS hardening, firewall rules (UFW), and SSH security. Currently, installation requires cloning the repository and running `docker compose up --build` manually.

**Caddy as a Containerized Service**
Caddy runs as a Docker container rather than a host-installed binary. In development, it uses a standard Alpine-based Caddy image. For production (planned), a custom `xcaddy` build compiled with the `caddy-dns/cloudflare` plugin will be used to enable wildcard TLS across various DNS providers.

**Two-Phase Build Pipeline**
*   **Prepare Phase:** Uses the **Railpack CLI** to inspect source code and generate a `railpack-plan.json`. This phase identifies the runtime (e.g., Node.js, Go) and required dependencies.
*   **Build Phase:** Executes the plan using **BuildKit**. Build output (the image tarball) is piped from `buildctl`'s stdout into `docker load`, and build progress from stderr is streamed via WebSockets directly to the UI, providing real-time feedback to the developer.

**Dynamic Route Provisioning**
Appa leverages the **Caddy Admin API** for runtime configuration. When a container starts, the API registers a new route mapping the deployment's subdomain (e.g., `app-123.localhost` in dev, `app-123.yourdomain.com` in production) to the container's internal IP.
*   **Self-Healing:** On startup, the API performs a "Route Restoration" by querying PostgreSQL for all `RUNNING` deployments and re-registering their routes in Caddy, ensuring the platform recovers gracefully from restarts.

**WebSocket log streaming with the hub pattern**.

Build and runtime logs are streamed to connected clients over WebSocket using the hub pattern: a single goroutine owns all broadcast state and receives log events via a channel, eliminating mutex contention across concurrent deployment sessions. Logs are also persisted to PostgreSQL for historical retrieval.

# Trade-offs and Open Questions

**DNS Provider Coupling**

The current v1 implementation is optimized for **Cloudflare**. While the architecture supports other providers via different Caddy plugins, Cloudflare remains the default for the "zero-config" experience. Future iterations will aim for a provider-agnostic plugin system.

**Orchestration Scope**

Appa currently leverages standard **Docker Compose** to fulfill its promise of a "single-command" setup for single-node environments. While Compose provides the simplicity required for v1, it is fundamentally a development tool that lacks advanced production orchestration features such as health-based service restarts, zero-downtime updates, and multi-node scaling.

To bridge this gap without introducing significant complexity, the architecture defines a clear evolutionary path. The next phase will involve a transition to **Docker Stack**, which allows deploying Compose-formatted files to a single-node **Docker Swarm** cluster. This approach provides production-grade orchestration capabilities (e.g., declarative state management and automated rollouts) while maintaining the single-node simplicity that defines Appa's core value proposition. Full multi-node Swarm support remains a long-term goal for high-availability requirements.

# Roadmap & Future Features

### Phase 1: Stability & Performance
*   **Build Caching:** Persistent volumes for Railpack to skip redundant build steps.
*   **Resource Limits:** Enforcement of CPU/Memory caps per container via the Docker API.
*   **Reliability Hardening:** Improved error recovery in the pipeline stages.

### Phase 2: Operations & Scale
*   **Automated Backups:** Project-scoped database snapshots to S3-compatible storage.
*   **Observability:** Integrated Prometheus/Grafana stack for app performance monitoring.
*   **Rollbacks:** Instant switching between successful image tags.

### Phase 3: Advanced Networking
*   **Multi-Provider DNS:** Support for AWS Route53, DigitalOcean, and Google DNS.
*   **Horizontal Scaling:** Basic load balancing across multiple application instances.

# Reference Documentation

### Core Technologies
*   **Railpack:** [CLI Reference](https://railpack.com/reference/cli) | [Frontend Guide](https://railpack.com/reference/frontend) | [Running in Production](https://railpack.com/guides/running-railpack-in-production)
*   **BuildKit:** [buildctl Reference](https://github.com/moby/buildkit/blob/master/docs/reference/buildctl.md) | [buildkitd.toml Config](https://docs.docker.com/build/buildkit/toml-configuration) | [Depot: BuildKit in Depth](https://depot.dev/blog/buildkit-in-depth) | [SparkFabrik: Docker BuildKit Deep Dive (Caching)](https://tech.sparkfabrik.com/en/blog/docker-cache-deep-dive) | [Earthly: What is BuildKit?](https://earthly.dev/blog/what-is-buildkit-and-what-can-i-do-with-it/)
*   **Caddy:** [Caddyfile Concepts](https://caddyserver.com/docs/caddyfile/concepts) | [Admin API Docs](https://caddyserver.com/docs/admin-api) | [Wildcard TLS Guide](https://oneuptime.com/blog/post/2026-02-08-how-to-run-caddy-with-docker-and-automatic-https-wildcard-certificates/view) | [Wildcard TLS for Multi-Tenant Systems](https://www.skeptrune.com/posts/wildcard-tls-for-multi-tenant-systems/) | [Dev/Prod Caddyfile Pattern](https://dev.to/tylerlwsmith/using-the-same-caddyfile-for-both-development-and-production-5a23) | [caddy-dns/cloudflare (GitHub)](https://github.com/caddy-dns/cloudflare)

### Infrastructure & Security
*   **Ansible:** [Ansible Lockdown (Hardening)](https://github.com/ansible-lockdown) | [Security Hardening Guide](https://oneuptime.com/blog/post/2026-01-21-ansible-security-hardening/view) | [Jeff Geerling's Ansible 101](https://www.youtube.com/playlist?list=PL2_OBreMn7FqZkvMYt6ATmgC0KAGGJNAN) | [Ansible Vault Guide](https://docs.ansible.com/ansible/latest/vault_guide/index.html)
*   **Mise:** [Getting Started](https://mise.jdx.dev/getting-started.html) | [Environments](https://mise.jdx.dev/environments)

### DNS Automation
*   **Cloudflare API:** [Create Subdomain](https://developers.cloudflare.com/dns/manage-dns-records/how-to/create-subdomain) | [Zones and DNS Records](https://developers.cloudflare.com/api/resources/zones/methods/create)
*   **ACME:** [lego](https://github.com/go-acme/lego) — Go ACME client for native cert provisioning

### Backups & Container Registry
*   **Backups:** [Restic (S3-compatible)](https://restic.net) | [Docker PG Backup](https://github.com/kartoza/docker-pg-backup)
*   **Registry:** [Harbor (self-hosted OCI)](https://goharbor.io) | [Container Registry Comparison 2026](https://distr.sh/blog/container-image-registry-comparison)

### Observability
*   **Monitoring:** [dockprom (Prometheus/Grafana)](https://github.com/stefanprodan/dockprom) | [Prometheus Getting Started](https://prometheus.io/docs/prometheus/latest/getting_started/)
*   **Comparison:** [Docker Monitoring Tools Comparison 2026](https://www.dash0.com/comparisons/best-docker-monitoring-tools)
