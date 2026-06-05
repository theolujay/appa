# Appa Reference

Research notes, data model, state machines, operational constraints, trade-offs,
and external reference links that inform the Appa architecture.

The canonical architecture document is [architecture.md](./architecture.md).

## Architectural Questions

1. What runtime components make up an Appa instance?
2. How does the Appa CLI provision and manage a remote instance?
3. How does source code become a routed application container?
4. How are build and runtime logs delivered to the dashboard?
5. What implementation invariants must not be violated?
6. How does the system recover from expected failures?

## Glossary Extras

Additional terms from earlier drafts:

| Term | Meaning |
| --- | --- |
| Operator | Person running an Appa instance on their own server. |
| Developer | User who deploys applications through Appa. |
| Source | Git URL or extracted ZIP archive path used for a deployment. |
| Build plan | `railpack-plan.json`, the declarative input consumed by the Railpack BuildKit frontend. |
| Image tag | Local Docker image name produced for a deployment, currently `appa-{id}`. |

## Core Data Model

| Model | Represents | Important Fields |
| --- | --- | --- |
| `User` | Account and ownership boundary | `id`, `name`, `email`, `password_hash`, `activated`, `version` |
| `Token` | Authentication or activation credential | `hash`, `user_id`, `expiry`, `scope` |
| `Deployment` | One app deployment lifecycle | `id`, `user_id`, `source`, `status`, `image_tag`, `address`, `env_vars`, `url`, `version` |
| `Log` | Ordered deployment log event | `id`, `deployment_id`, `phase`, `line`, `ts` |

```
User ─── (1:N) ─── Deployment ─── (1:N) ─── Log
  └── (1:N) ─── Token
```

`deployments.user_id` is the ownership boundary for user-facing deployment
operations. `logs.deployment_id` cascades on deployment deletion.

## State Machines

### Deployment Lifecycle

```
pending ──▶ building ──▶ deploying ──▶ running ──▶ stopped
              │            │             │
              ├────────────┴─────────────┴──▶ failed
              │
              └──▶ canceled
```

`pending` is created synchronously in the request handler. `building`,
`deploying`, and `running` are set by the background pipeline. `stopped` is set
when a running deployment is stopped. `failed` represents pipeline failure.
`canceled` represents cancellation observed while the active pipeline is being
unwound.

### Pipeline Phases

```
source acquisition ──▶ railpack prepare ──▶ buildkit build
         ──▶ docker load ──▶ container start ──▶ caddy route
```

Phase labels persisted in logs: `build`, `deploy`, `routing`, `cancel`.

## Operational Constraints

- Run the API, PostgreSQL, BuildKit, Caddy, and UI on the shared internal Docker
  network expected by `compose.yml`.
- Do not expose PostgreSQL, BuildKit, Docker socket access, or the Caddy Admin
  API to the public internet.
- Keep the Docker socket access limited to the API container; the API is the
  container lifecycle authority.
- BuildKit remains privileged, but that privilege should not be copied into the
  API service.
- Route restoration depends on accurate `deployments.status` and
  `deployments.address` values.
- Environment variables are currently stored as plaintext deployment data; treat
  them as sensitive and avoid logging them.
- CLI-managed instance profiles may contain secrets; store them in redacted or
  encrypted form and pass them to Ansible without exposing them in logs.
- Remote setup must allow configuration to be applied in stages. Domain, DNS,
  SMTP, backups, and monitoring should be configurable after first contact with
  the VPS.
- Ansible playbooks and roles should be idempotent and safe to rerun from
  `appa setup` or `appa apply`.
- Deployment ZIP extraction must preserve per-upload isolation and should reject
  unsafe archive paths before production use.
- MVP deployment targets single-node Docker Compose.

## Trade-offs and Open Questions

**DNS Provider Coupling**

The v1 implementation is optimized for **Cloudflare**. While the architecture
supports other providers via different `caddy-dns` plugins, Cloudflare remains
the default for the "zero-config" experience.

**Railpack CLI and Frontend Version Coupling**

The Railpack CLI (installed in the API container at build time) and the Railpack
BuildKit frontend image (pulled at runtime via `buildctl`) must be kept at
matching versions. The Railpack CLI generates the build plan; the frontend
consumes it. A version mismatch between them can produce silent build failures
or unexpected behavior. Both are currently pinned and must be updated together
whenever Railpack is upgraded.

**Orchestration Scope**

Appa currently leverages standard **Docker Compose** for "single-command" setup
on single-node environments. While Compose provides the simplicity required for
v1, it is fundamentally a development tool that lacks advanced production
orchestration features such as health-based service restarts, zero-downtime
updates, and multi-node scaling.

The architectural constraint is that deployment and routing code should not
assume Compose is the only possible service backend.

**CLI Scope**

The CLI starts as an operator tool for Appa Instance provisioning and
maintenance. Long term, it can also become a developer workflow surface for
project deployment, logs, environment variables, and rollbacks. Project-level
commands should call the Appa Server API instead of bypassing it with direct
SSH, Docker, or Ansible operations.

## Core Technology References

### Railpack
- [CLI Reference](https://railpack.com/reference/cli)
- [Frontend Guide](https://railpack.com/reference/frontend)
- [Running in Production](https://railpack.com/guides/running-railpack-in-production)

### BuildKit
- [buildctl Reference](https://github.com/moby/buildkit/blob/master/docs/reference/buildctl.md)
- [buildkitd.toml Config](https://docs.docker.com/build/buildkit/toml-configuration)
- [Depot: BuildKit in Depth](https://depot.dev/blog/buildkit-in-depth)
- [SparkFabrik: Docker BuildKit Deep Dive (Caching)](https://tech.sparkfabrik.com/en/blog/docker-cache-deep-dive)
- [Earthly: What is BuildKit?](https://earthly.dev/blog/what-is-buildkit-and-what-can-i-do-with-it/)

### Caddy
- [Caddyfile Concepts](https://caddyserver.com/docs/caddyfile/concepts)
- [Admin API Docs](https://caddyserver.com/docs/admin-api)
- [Wildcard TLS Guide](https://oneuptime.com/blog/post/2026-02-08-how-to-run-caddy-with-docker-and-automatic-https-wildcard-certificates/view)
- [Wildcard TLS for Multi-Tenant Systems](https://www.skeptrune.com/posts/wildcard-tls-for-multi-tenant-systems/)
- [Dev/Prod Caddyfile Pattern](https://dev.to/tylerlwsmith/using-the-same-caddyfile-for-both-development-and-production-5a23)
- [caddy-dns/cloudflare (GitHub)](https://github.com/caddy-dns/cloudflare)

### Ansible
- [Ansible Lockdown (Hardening)](https://github.com/ansible-lockdown)
- [Security Hardening Guide](https://oneuptime.com/blog/post/2026-01-21-ansible-security-hardening/view)
- [Jeff Geerling's Ansible 101](https://www.youtube.com/playlist?list=PL2_OBreMn7FqZkvMYt6ATmgC0KAGGJNAN)
- [Ansible Vault Guide](https://docs.ansible.com/ansible/latest/vault_guide/index.html)

### Mise
- [Getting Started](https://mise.jdx.dev/getting-started.html)
- [Environments](https://mise.jdx.dev/environments)

## CLI Development References

### Cobra
- [Official repository](https://github.com/spf13/cobra)
- [Cobra docs](https://cobra.dev/)
- [cobra-cli generator](https://github.com/spf13/cobra-cli)

### Example CLIs
- [Railpack](https://github.com/railwayapp/railpack)
- [Railway CLI](https://github.com/railwayapp/cli)
- [GitHub CLI](https://github.com/cli/cli)
- [flyctl](https://fly.io/docs/flyctl/)
- [Pulumi CLI architecture](https://pulumi-developer-docs.readthedocs.io/latest/docs/architecture/README.html)

## DNS Automation

- [Cloudflare: Create Subdomain](https://developers.cloudflare.com/dns/manage-dns-records/how-to/create-subdomain)
- [Cloudflare: Zones and DNS Records](https://developers.cloudflare.com/api/resources/zones/methods/create)
- [lego (Go ACME client)](https://github.com/go-acme/lego)

## Backups & Container Registry

- [Restic (S3-compatible)](https://restic.net)
- [Docker PG Backup](https://github.com/kartoza/docker-pg-backup)
- [Harbor (self-hosted OCI)](https://goharbor.io)
- [Container Registry Comparison 2026](https://distr.sh/blog/container-image-registry-comparison)

## Observability

- [dockprom (Prometheus/Grafana)](https://github.com/stefanprodan/dockprom)
- [Prometheus Getting Started](https://prometheus.io/docs/prometheus/latest/getting_started/)
- [Docker Monitoring Tools Comparison 2026](https://www.dash0.com/comparisons/best-docker-monitoring-tools)
