# Appa Roadmap

This roadmap tracks delivery phases and future evolution. Architecture constraints
and system design decisions live in [ARCHITECTURE.md](./architecture.md).

## MVP (Complete)

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

## CLI & Remote Operations (Complete)

- Appa CLI (`cmd/cli/`) for remote instance management and provisioning.
- Instance profiles in `~/.appa/instances/<name>/config.toml`.
- SSH target configuration with identity file support (`--identity-file`).
- Preflight checks for SSH access, OS, ports, DNS, Docker readiness,
  and required operator inputs.
- Remote setup through `appa setup <instance>`, backed by Ansible playbooks.
- Idempotent `appa apply <instance>` for configuration changes after initial
  setup.
- Remote operations: `status`, `logs` (with service filter), `restart`,
  and `upgrade` (with optional version pin).
- Ansible hardening roles for UFW, SSH, access control, kernel parameters,
  and auditd, with passing Molecule tests.
- Compliance-scan playbook for CIS-style checks.
- CI workflow with lint + molecule matrix.
- Makefile targets for build, run, lint, and test workflows.

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

### Encrypted Instance Profiles

Currently, the CLI stores instance profiles in plaintext within `~/.appa/instances/<name>/config.toml`. To prevent unauthorized access to sensitive remote server details, SSH configurations, database credentials, and operator keys on the operator's machine, we plan to:
- Integrate Ansible Vault-compatible encryption directly into the Appa CLI.
- Securely encrypt the instance config profiles using a vault password (provided via terminal prompt, environment variable, or key file).
- Allow operators to safely store, version control, or share profile configurations without exposing passwords, database credentials, or SSH private keys.
