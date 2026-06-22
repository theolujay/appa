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

## v0.2.0 — Project Lifecycle & Environment Variables (Current)

### API

- Add `project_name` to deployments table and create handler.
- Single deployment lookup (`GET /v1/deployments/:id`).
- Project-filtered deployment list (`GET /v1/deployments?project_name=X`).
- Project env var CRUD (`GET/POST/DELETE /v1/projects/:name/env`).
- Pipeline: merge project env vars into build step alongside deployment-specific vars.

### CLI

- `project logs <name>` — stream WebSocket logs from latest deployment.
- `project stop <name>` — stop running or cancel pending deployment.
- `project restart <name>` — stop + trigger new deployment.
- `project env set/get/unset <name>` — environment var management through the API.
- `deploy` now passes `project_name` in POST body.

## v0.3.0 — Domain & TLS

- `appa instance set-domain <name> <domain> [--cf-token]`.
- Cloudflare API integration: DNS record creation, wildcard TLS via DNS-01.
- Pre-build `caddy-cloudflare` image with `xcaddy` and push to GHCR.
- Caddy TLS certificate verification.
- Multi-provider DNS interface for future providers (Route53, DigitalOcean, Google Cloud DNS).

## v0.3.1 — Docker Stack Migration

- Ansible: swap Docker Compose → Docker Stack in deploy playbook.
- Rollbacks by switching traffic back to a previously successful image tag.
- Zero-downtime container swaps (health-check-aware blue/green via Caddy).
- Stronger restart, update, and service-management semantics on single-node Swarm.

## v0.4.0 — Observability

- Integrated Prometheus and Grafana stack for platform and app monitoring.
- Structured logs and basic metrics (build duration, failure rates, route registration latency, container health).
- Persistent build cache volumes for Railpack and BuildKit.
- Better cleanup for failed builds, failed containers, stale images, stale routes.
- Health checks for platform services and deployed application containers.
- Clearer deployment state transitions for retries and recovery.
- CPU and memory limits per deployed container through the Docker API.

## v0.5.0 — Revamped Dashboard

- Dashboard UI rebuilt on top of all CLI/API features from v0.1.x–v0.4.0.
- Full project lifecycle visibility, env var management, deployment history.

## v0.6.0 — Authentication & Authorization

- Multi-user support with accounts.
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

A root `.mise.toml` to pin Go and project tooling versions for contributors and CI.
