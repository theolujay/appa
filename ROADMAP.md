# Appa Roadmap

This roadmap tracks delivery phases and future evolution. Architecture constraints
and system design decisions live in [ARCHITECTURE.md](./ARCHITECTURE.md).

## MVP

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

## CLI & Remote Operations

- Appa CLI installer at `appa.dev/install.sh`; the script installs the local
  CLI on the operator's machine, not the remote server stack.
- Instance profiles for managing one or more remote Appa installations.
- SSH target configuration for each instance profile.
- Preflight checks for SSH access, supported OS, ports, Docker readiness, DNS,
  and required operator inputs.
- Remote setup through `appa setup <instance>`, backed by Ansible playbooks.
- Idempotent `appa apply <instance>` for configuration changes after initial
  setup.
- Basic remote operations: `status`, service logs, restart, upgrade, and
  uninstall safeguards.
- Local secret handling for instance configuration, with Ansible Vault or a
  comparable encryption path for sensitive values.

## Stability & Performance

- Persistent build cache volumes so Railpack and BuildKit can skip redundant work.
- CPU and memory limits per deployed container through the Docker API.
- Reliability hardening across source acquisition, build, container startup, and
  route registration.
- Better cleanup for failed builds, failed containers, stale images, and stale
  routes.
- Health checks for platform services and deployed application containers.
- Clearer deployment state transitions for retries and recovery.

## Operations & Scale

- Ansible as the default remote provisioning engine for Appa Server instances.
- Host provisioning roles for Docker, Compose, Appa Stack files, environment
  rendering, firewall rules, and service lifecycle management.
- Optional hardening roles for UFW rules, SSH security, unattended upgrades, and
  least-privilege operator access.
- Automated backups for Appa PostgreSQL data to S3-compatible storage.
- Project-scoped backup and restore workflows for deployed applications.
- Integrated Prometheus and Grafana stack for platform and app monitoring.
- Structured logs and basic metrics for build duration, failure rates, route
  registration latency, and container health.
- Rollbacks by switching traffic back to a previously successful image tag.
- Docker Stack support on a single-node Swarm for stronger production
  orchestration while keeping the single-node operating model.

## Advanced Networking

- Production Caddy image built with `xcaddy` and the
  `caddy-dns/cloudflare` plugin.
- Wildcard TLS through DNS-01 challenges.
- Multi-provider DNS support for Route53, DigitalOcean, Google Cloud DNS, and
  other Caddy DNS plugins.
- Basic horizontal scaling across multiple instances of one deployed app.
- Load balancing and health-aware routing for scaled app instances.

## Future Evolution

### Installer and Host Provisioning

Appa's default installation path should start on the operator's machine:
`appa.dev/install.sh` installs the Appa CLI, then the CLI provisions the remote
VPS. `appa setup <instance>` should prepare an Ubuntu VPS, write the required
environment, start the Appa Stack, and print the operator's access URL.

The CLI should allow progressive configuration. The operator should be able to
create an instance profile with only an SSH target, run preflight checks, and add
domain, DNS, SMTP, backup, and monitoring settings as they become available.

### DNS Provider Abstraction

The first production target is Cloudflare because it gives Appa a practical
zero-config path for wildcard TLS. The architecture should still keep DNS
provider details behind a narrow interface so additional providers can be added
without changing deployment routing or certificate lifecycle code.

### Orchestration Path

Docker Compose is the right starting point for Appa's self-hosted v1 because it
keeps the install and mental model simple. The next step is Docker Stack on a
single-node Swarm, which preserves most Compose semantics while adding stronger
restart, update, and service-management behavior. Full multi-node operation is a
long-term direction for users who need higher availability.

### Project-Level CLI

The first CLI milestone should focus on remote instance operations. Later, the
same CLI can become a developer workflow surface for projects:

- `appa project init <name>` for local project metadata.
- `appa deploy` for API-backed deployment to a selected Appa instance.
- `appa logs`, `appa env`, `appa stop`, and `appa rollback` for project
  operations.
- Project-to-instance mappings so one CLI can manage personal, staging, and
  client Appa instances.

### Development Tooling

A root `.mise.toml` should eventually pin Go and project tooling versions for
contributors and CI. Railpack already uses Mise internally for application
runtime installation; Appa only needs Mise for its own development environment.
