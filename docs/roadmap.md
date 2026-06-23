# Appa Roadmap

This roadmap tracks delivery phases and future evolution. Architecture constraints
and system design decisions live in [ARCHITECTURE.md](./architecture.md).

## v0.1.x — Foundation (Complete)

### MVP

- Self-hostable single-node platform stack with Docker Compose.
- Go API, PostgreSQL, BuildKit, Caddy, and React dashboard.
- Git repository and ZIP archive deployments.
- Zero-config Railpack builds for common application runtimes.
- BuildKit image builds streamed to `docker load`.
- Docker container lifecycle management from the API.
- Dynamic Caddy route registration for deployment subdomains.
- Route restoration for `RUNNING` deployments after platform restart.
- WebSocket build and runtime log streaming.
- Persisted deployment records and historical logs.
- Local development flow using `localhost` subdomains.

### CLI & Remote Operations

- Appa CLI for remote instance management and provisioning.
- Instance configs in `~/.appa/instances/<name>/config.toml`.
- SSH target configuration with identity file support.
- Preflight checks for SSH access, OS, ports, DNS, Docker readiness.
- Remote setup through `appa setup <instance>`, backed by Ansible playbooks.
- Idempotent `appa apply <instance>` for configuration changes.
- Remote operations: `status`, `logs`, `restart`, `upgrade`.
- Ansible hardening roles (UFW, SSH, access control, kernel, auditd) with Molecule tests.
- Compliance-scan playbook for CIS-style checks.
- CI workflow with lint + molecule matrix.
- Makefile targets for build, run, lint, and test workflows.
- Project CRUD (`init`, `edit`) with TOML config.
- `appa deploy <project>` — rsync source shipping + API build pipeline.
- `--quiet` flag for deploy output control.

## v0.2.x — Project Lifecycle & Environment Variables (Complete)

### API

- `project_id` foreign key on deployments table.
- Single deployment lookup (`GET /v1/deployments/:id`).
- Project-filtered deployment list (`GET /v1/deployments?project_name=X`).
- `project_envs` table (migration 000008) with upsert semantics via `ON CONFLICT`.
- Project env var CRUD (`GET /v1/projects/:id/env`, `POST /v1/projects/:id/env`, `DELETE /v1/projects/:id/env/:key`).
- Pipeline: project env vars merged with deployment-level env vars at creation time (deployment-level overrides project-level).

### CLI

- `project logs <name>` — stream WebSocket logs from latest deployment.
- `project stop <name>` — stop running or cancel pending deployment.
- `project restart <name>` — stop + trigger new deployment.
- `project env set <name> KEY=VALUE [...]` — upsert env vars via API.
- `project env get <name> [KEY]` — list all or get a single env var.
- `project env unset <name> KEY [...]` — delete env vars via API.
- `deploy` passes `project_name` and `project_id` in POST body.

## v0.3.0 — Domain & TLS

- `appa server set-domain <name> <domain> [--cf-token]` — CLI command to wire up existing config model (server config already has `Domain` and `CloudflareToken` fields, Ansible Caddyfile templates support TLS via DNS-01 conditionally).
- Cloudflare API integration: DNS record creation, wildcard TLS via DNS-01.
- Pre-build `caddy-cloudflare` image with `xcaddy` and push to GHCR.
- Caddy TLS certificate verification.
- Multi-provider DNS interface for future providers (Route53, DigitalOcean, Google Cloud DNS).

## v0.3.1 — Docker Stack Migration (Mostly Complete)

### Completed

- Ansible deploy playbook uses `docker stack deploy`.
- Swarm initialization and stack deployment (data + base services).
- `update_config.failure_action: rollback` for automated rollbacks.
- `update_config.order: start-first` for blue/green-like zero-downtime swaps.
- Migrated from legacy `compose.yml` to Swarm stack templates.

### Remaining

- CLI commands (`status`, `logs`, `restart`, `upgrade`) need `docker stack` equivalents.

## v0.4.0 — Observability

- Integrated Prometheus and Grafana stack for platform and app monitoring.
- Structured logs and basic metrics (build duration, failure rates, route registration latency, container health).
- Persistent build cache volumes for Railpack and BuildKit.
- Better cleanup for failed builds, failed containers, stale images, stale routes.
- Health checks for platform services and deployed application containers.
- Clearer deployment state transitions for retries and recovery.
- CPU and memory limits per deployed container through the Docker API.

## v0.5.0 — Revamped Dashboard

- A basic React dashboard already exists (deployment list/detail, log streaming, deploy form, auth pages).
- Planned revamp to add full project lifecycle visibility, env var management, and deployment history with feature parity against CLI and API.

## v0.6.0 — Authentication & Authorization (Partial)

### Completed

- Multi-user support with email/password registration and activation.
- Token-based API authentication (24h expiry).
- Dashboard login, logout, and session management.

### Remaining

- GitHub sign-in integration.
- CLI-native auth (sign-in, token management).
- Role-based access for dashboard users (deploy, view, admin).

## Future

### DNS Provider Abstraction

Architecture should keep DNS provider details behind a narrow interface so
additional providers (Route53, DigitalOcean, Google Cloud DNS) can be added
without changing deployment routing or certificate lifecycle code.

### Multi-Node Orchestration

Full multi-node operation is a long-term direction for users who need higher
availability. Docker Stack on single-node Swarm (v0.3.1) is the intermediate step.

### Project-to-Instance Mapping

One CLI managing personal, staging, and client Appa instances from a single
config directory.

### Webhook Receiver

Generic endpoint accepting GitHub push events for push-to-deploy.

### Managed Add-Ons

Managed PostgreSQL and Redis for user applications. Automated PostgreSQL
backups to S3-compatible storage.

### Encrypted Instance Profiles

Ansible Vault-compatible encryption for `~/.appa/instances/<name>/config.toml`
to safely store, version control, or share profile configurations.

### Content-Addressable Sync Engine

A pure-Go CAS engine over SSH as a future replacement for rsync-based source
shipping. See [cas-sync.md](./cas-sync.md) for the design research.

### Development Tooling

- A root `.mise.toml` to pin Go and project tooling versions for contributors and CI.
- Vagrant dev environment at `deploy/ansible/dev/` for end-to-end testing (Makefile targets: `vagrant/up`, `vagrant/destroy`).
