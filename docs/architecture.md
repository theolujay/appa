# Appa Architecture

Appa is a self-hostable deployment platform that turns a Git repository or ZIP
archive into a running container behind a stable URL. The product has two
planned surfaces: the Appa CLI installed on the operator's machine, and an Appa
Server instance running on a remote VPS. The server stack combines a Go API,
PostgreSQL, BuildKit, Railpack, Docker, Caddy, and a React dashboard into a
single-node platform runtime.

This document is the technical architecture reference. Setup instructions and
contribution guidelines live in [CONTRIBUTING.md](./CONTRIBUTING.md). Delivery
phases and future work live in [ROADMAP.md](./roadmap.md).

## Architectural Questions

This document answers:

1. What runtime components make up an Appa instance?
2. How does the Appa CLI provision and manage a remote instance?
3. How does source code become a routed application container?
4. How are build and runtime logs delivered to the dashboard?
5. What implementation invariants must not be violated?
6. How does the system recover from expected failures?

## Glossary

| Term | Meaning |
| --- | --- |
| Appa | The whole product, including the CLI and remote server runtime. |
| Appa CLI | Local operator/developer command-line tool, binary `appa`. |
| Appa Server | Remote API, dashboard, and deployment runtime installed on a VPS. |
| Appa Instance | One remote Appa Server installation managed by the CLI. |
| Appa Stack | Server-side services: API, UI, PostgreSQL, BuildKit, Caddy, and their runtime configuration. |
| Instance profile | Local CLI configuration for one Appa Instance, including SSH target and redacted or encrypted settings. |
| Operator | Person running an Appa instance on their own server. |
| Developer | User who deploys applications through Appa. |
| Deployment | One submitted source package and its lifecycle state. |
| Source | Git URL or extracted ZIP archive path used for a deployment. |
| Railpack | Runtime detector and build-plan generator. |
| BuildKit | Build daemon that executes the Railpack build plan. |
| Build plan | `railpack-plan.json`, the declarative input consumed by the Railpack BuildKit frontend. |
| Image tag | Local Docker image name produced for a deployment, currently `appa-{id}`. |
| App container | User workload container created from the built image. |
| Route | Caddy reverse-proxy mapping from deployment hostname to app container address. |
| Hub | In-process WebSocket broadcaster for deployment logs and status changes. |
| `appa_net` | Internal Docker network shared by platform services and app containers. |

## Components

```plaintext
                        Operator Machine
               ┌────────────────────────────────┐
               │            Appa CLI            │
               │ instance profiles, preflight,  │
               │ setup/apply/status operations  │
               └───────────────┬────────────────┘
                               │ SSH + Ansible
                               ▼
                   Remote VPS / Appa Instance
        ┌──────────────────────────────────────────────┐
        │                 Appa Stack                   │
        │ Docker Compose services + generated config   │
        └──────────────────────┬───────────────────────┘
                               │
                               ▼
                   Operator / Developer Browser
                               │
                               ▼
                         [ React Dashboard ]
                         deploy, cancel, logs
                               │
                               ▼
            ┌──────────── [ Caddy Gateway ] ────────────┐
            │        HTTP routing + TLS boundary        │
            ▼                                           ▼
      [ Appa API ]                                [ User App Route ]
   Go HTTP server, auth,                         {id}.localhost in dev
   deployment pipeline,                                 │
   WebSocket hub                                        ▼
            │                                      [ App Container ]
            │                                     appa-{deployment_id}
            │                                            │
            ├──────────────┬──────────────┬──────────────┤
            ▼              ▼              ▼              ▼
      [ PostgreSQL ]   [ BuildKit ]   [ Docker API ]  [ Caddy Admin API ]
   users, tokens,     privileged     image load,     dynamic routes,
   deployments, logs  build daemon   container run   route restore
            ▲              ▲
            │              │
            └────── [ Railpack CLI + BuildKit Frontend ]
                     runtime detection and image build
```

The CLI is the operator-facing control surface for provisioning and maintenance.
The Appa Server remains the authority for application deployments, builds, app
containers, routes, logs, users, and tokens.

## Core Flows

### Operator Provisions an Appa Instance

1. The operator installs the Appa CLI on their own machine with
   `appa.dev/install.sh`.
2. The operator creates a local instance profile, for example
   `appa instance init personal`.
3. The operator sets an SSH target with
   `appa instance set-host personal root@203.0.113.10`.
4. `appa preflight personal` checks SSH access, supported OS, required ports,
   DNS readiness when configured, and required operator inputs.
5. `appa setup personal` invokes Ansible with generated inventory and variables.
6. Ansible prepares the host, installs Docker/Compose dependencies, creates the
   Appa runtime directory, writes environment and Caddy configuration, and
   starts the Appa Stack.
7. The CLI reports the Appa Server URL and stores enough local state for later
   `appa apply`, `appa status`, `appa logs`, `appa restart`, and `appa upgrade`
   operations.

First setup should support progressive configuration. An operator should be able
to define only the SSH target first, then add domain, Cloudflare, SMTP, backup,
and monitoring settings as they become available.

### User Deploys a Git Repository

1. An activated user submits a Git URL and optional environment variables.
2. The API validates the deployment input and creates a `Deployment` row with
   `pending` status.
3. A background pipeline task is registered so it can be cancelled later.
4. The deployment status changes to `building`.
5. The pipeline clones the repository into a temporary build directory.
6. `railpack prepare` inspects the source and writes `railpack-plan.json` and
   `railpack-info.json`.
7. `buildctl build` runs the Railpack BuildKit frontend with the generated plan.
8. The built Docker image tar stream is piped directly into `docker load`.
9. The deployment status changes to `deploying`.
10. The Docker API creates and starts `appa-{deployment_id}` on `appa_net`.
11. The pipeline waits for the selected container port to accept TCP traffic.
12. The Caddy Admin API receives a route from `{deployment_id}.localhost` to the
    app container address.
13. The deployment row is updated to `running` with its URL and internal address.

### User Deploys a ZIP Archive

1. The API accepts a multipart upload with a 100 MB request limit.
2. The uploaded ZIP is extracted into `/tmp/appa-upload/{uuid}`.
3. A `Deployment` row is created with source label `uploaded-project`.
4. The background pipeline runs against the extracted local directory.
5. The rest of the build, container startup, routing, and logging flow is the
   same as a Git deployment.

Uploaded project directories are treated as build inputs, not long-term storage.
The deployment artifact of record is the loaded Docker image and running
container.

### User Streams Deployment Logs

1. An authenticated user opens `/v1/deployments/{id}/logs`.
2. The API verifies that the deployment belongs to the user.
3. The HTTP connection is upgraded to WebSocket.
4. Historical log rows are loaded from PostgreSQL and sent first.
5. The connection registers with the in-process hub for live events.
6. Build, deploy, route, cancellation, and status events are sent as they happen.
7. Ping/pong deadlines detect dead clients and unregister them.

Logs are persisted before live publication. A disconnected client can reconnect
and replay the database-backed history before receiving live events.

### API Startup Restores Routes

1. The API creates a Caddy router client for `caddy:2019`.
2. A background startup task queries PostgreSQL for `running` deployments.
3. Each running deployment with a stored internal address is re-registered in
   Caddy.
4. Individual route-restore failures are logged and do not abort the whole
   restore pass.

This makes Caddy configuration recoverable from API or Caddy restarts without
requiring users to redeploy applications.

### User Cancels or Stops a Deployment

1. The API verifies the requesting user owns the deployment.
2. If the deployment has an active pipeline task, the task context is cancelled.
3. If no active task exists, the pipeline stops the app container.
4. The Caddy route is removed.
5. The deployment status changes to `stopped` when a running container is stopped,
   or `canceled` when an active pipeline observes cancellation during failure
   handling.

## Enforceable Invariants

These are implementation rules, not suggestions.

1. **Every deployment read or mutation exposed to a user must be ownership-scoped.**
   User-facing handlers must check `deployment.user_id` before returning logs,
   cancelling, stopping, or exposing deployment details.

2. **Binary build output must never pass through text log scanners.**
   `buildctl` stdout is the Docker image tar stream and must pipe directly into
   `docker load`. Only stderr build progress is safe to scan and broadcast.

3. **Railpack CLI and Railpack frontend versions must be kept compatible.**
   The Railpack CLI generates the plan and the frontend consumes it. Upgrade
   them as one unit.

4. **Caddy Admin API must remain internal to `appa_net`.**
   Runtime route mutation is powerful enough to control public traffic. Port
   `2019` must not be exposed to the public internet.

5. **User app containers must be reachable by stable internal names.**
   Caddy routes dial `appa-{deployment_id}:{port}` on `appa_net`. Container
   naming and network attachment are part of the routing contract.

6. **Deployment status changes must be persisted and published.**
   PostgreSQL is the source of truth for reloads and history. The hub is only the
   live delivery path.

7. **Route registration happens only after container readiness succeeds.**
   Caddy should not route public traffic to a container that has not accepted a
   TCP connection on its selected port.

8. **Logs must be persisted before WebSocket publication.**
   Live delivery is best-effort; persisted logs are required for reconnect and
   historical viewing.

9. **Uploaded archives must be extracted into isolated per-upload directories.**
   ZIP deployments must not share a build directory across deployments.

10. **Platform services and user workloads share a network, not a process.**
    User applications run as separate Docker containers; they are not executed
    inside the API process.

11. **The CLI must not become a second deployment engine.**
    Instance operations may use Ansible to manage host and Appa Stack state, but
    project deployments should go through the Appa Server API.

12. **Remote provisioning must be idempotent.**
    `appa setup` and `appa apply` should be safe to rerun and should converge
    the remote host toward the requested instance profile.

13. **Operator secrets must not be logged.**
    Domain provider tokens, SMTP credentials, database passwords, backup keys,
    and deployment environment variables must be redacted in CLI output,
    Ansible logs, and documentation examples.

## Core Data Model

| Model | Represents | Important Fields |
| --- | --- | --- |
| `User` | Account and ownership boundary | `id`, `name`, `email`, `password_hash`, `activated`, `version` |
| `Token` | Authentication or activation credential | `hash`, `user_id`, `expiry`, `scope` |
| `Deployment` | One app deployment lifecycle | `id`, `user_id`, `source`, `status`, `image_tag`, `address`, `env_vars`, `url`, `version` |
| `Log` | Ordered deployment log event | `id`, `deployment_id`, `phase`, `line`, `ts` |

```plaintext
User ─── (1:N) ─── Deployment ─── (1:N) ─── Log
  └── (1:N) ─── Token
```

`deployments.user_id` is the ownership boundary for user-facing deployment
operations. `logs.deployment_id` cascades on deployment deletion, so log history
does not outlive the deployment row.

## State Machines

### Deployment Lifecycle

```plaintext
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

```plaintext
source acquisition ──▶ railpack prepare ──▶ buildkit build
         ──▶ docker load ──▶ container start ──▶ caddy route
```

The phase labels persisted in logs are currently `build`, `deploy`, `routing`,
and `cancel`.

## Failure Model

Appa assumes external processes and containers can fail. The intended recovery
model is built from durable deployment rows, persisted logs, route restoration,
and explicit status updates.

| Failure | Recovery Behavior |
| --- | --- |
| Git clone fails | The pipeline records a build failure, marks the deployment `failed`, publishes status, and removes the temporary build directory. |
| `railpack prepare` fails | The deployment remains inspectable with persisted build logs and is marked `failed`. |
| `RAILPACK_VERSION` is missing | Build fails before invoking the Railpack frontend; the deployment is marked `failed`. |
| BuildKit or `docker load` fails | Build logs are persisted from stderr; the deployment is marked `failed`. |
| API process exits during build | In-memory active-task state and live hub clients are lost; persisted deployment and logs remain. Automatic build resumption is not implemented. |
| Container fails readiness check | The deployment is marked `failed`; logs include the readiness failure. |
| Caddy route registration fails | The deployment does not become `running`; the route error is surfaced through the pipeline failure path. |
| Caddy restarts | Startup route restoration re-adds routes for deployments stored as `running` with an address. |
| WebSocket client disconnects | The hub unregisters the client; persisted logs allow replay on reconnect. |
| User stops a running deployment | The container is stopped, the Caddy route is removed, and status becomes `stopped`. |

Known implementation gap: `Pipeline.Run` must ensure build, container startup,
and route registration errors cannot fall through to the final `running` status
update. Failure handling after `Prepare` should be audited before relying on the
failure model above in production.

## Design Decisions

### Caddy as the Network Boundary

Caddy is the only public HTTP boundary. It routes platform traffic to the UI/API
and deployment traffic to user app containers by host header. The API mutates
Caddy configuration through the Admin API instead of writing static Caddyfile
fragments per deployment.

For development, routes use `{deployment_id}.localhost`. For production,
wildcard subdomains require wildcard TLS, which is why DNS-01 support is part of
the production path.

### Caddy as a Containerized Service

Caddy runs as a Docker container rather than a host-installed binary. This keeps
local development and production topology close, lets Appa distribute a custom
production Caddy image, and keeps the Admin API on the internal Docker network.

The production Caddy image is expected to be built with `xcaddy` and the
`caddy-dns/cloudflare` plugin so Caddy can obtain wildcard certificates through
DNS-01 challenges.

### BuildKit as a Separate Privileged Service

BuildKit needs elevated Linux capabilities for build isolation, filesystem
layers, and container namespaces. Appa isolates those privileges in a dedicated
BuildKit service instead of putting them in the API container.

The API talks to the daemon through `BUILDKIT_HOST=docker-container://buildkit`.
This keeps the build daemon off the public network and prevents BuildKit crashes
from taking down the API process.

### Two-Phase Railpack Build

Each deployment uses two Railpack phases:

- `railpack prepare` inspects the source tree and emits build metadata.
- `buildctl build` invokes the Railpack BuildKit frontend to execute the plan.

This split lets Appa stream useful planning and build output while keeping the
actual build execution inside BuildKit.

### Docker Image and Container Naming

The current image tag and container name are deterministic:

- image: `appa-{deployment_id}`
- container: `appa-{deployment_id}`

That makes route restoration and human debugging simple. Rollback support will
require versioned image tags, but the stable container naming contract should
remain separate from image versioning.

### Port Selection

Appa uses the first exposed image port when the image declares one. If no ports
are exposed, it defaults to `3000/tcp`, with a small heuristic for common static
servers that listen on `80/tcp`.

The route points to the container name and selected container port inside
`appa_net`. Host port publishing is not part of the routing path.

### WebSocket Log Streaming with the Hub Pattern

Build and runtime logs are streamed with a hub pattern: one goroutine owns
connection registration, unregistration, and broadcast state. This avoids shared
map mutation across deployment goroutines.

The database remains the durable log store. The hub only handles live delivery.

### Wildcard TLS via DNS-01 Challenge

HTTP-01 ACME challenges do not work for wildcard certificates. Appa needs a
wildcard certificate because each deployment gets its own subdomain and issuing a
new certificate for every deployment would hit certificate rate limits on active
instances.

DNS-01 proves control of the DNS zone with a TXT record at
`_acme-challenge.{domain}`. Caddy DNS plugins automate creating and deleting
that TXT record during certificate issuance and renewal.

### CLI-Managed Remote Provisioning

`appa.dev/install.sh` installs the Appa CLI on the operator's machine. It should
not be treated as a server-side bootstrap script to run directly on the VPS.

The CLI owns local instance profiles, command-line validation, preflight checks,
Ansible inventory generation, and user-facing progress output. Ansible owns the
remote host mutations: package installation, Docker setup, firewall and
hardening tasks, Appa Stack file placement, environment rendering, and Compose
service lifecycle.

The Appa Server API remains responsible for application deployment behavior. This
keeps the boundary clear: Ansible installs and operates the platform; the API
deploys and manages user applications inside the platform.

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
- MVP deployment targets single-node Docker Compose. Orchestration evolution is
  tracked in [ROADMAP.md](./roadmap.md).

## Repository Structure

```text
.
├── cmd/
|   ├──cli/            # Planned CLI entry point for instance and project commands
|   ├──cmd/api/        # Server entry point: bootstrap, flags, server config
├── deploy/ansible/    # Planned playbooks, roles, inventory templates, Molecule tests
├── internal/
│   ├── data/          # Database models and query methods
│   ├── hub/           # WebSocket broadcast hub (single-goroutine ownership pattern)
│   ├── mailer/        # Email templating and delivery
│   ├── pipeline/      # Core orchestration layer
│   │   ├── pipeline.go    # Deployment lifecycle coordinator
│   │   ├── builder.go     # Railpack prepare and BuildKit build phases
│   │   ├── runner.go      # Docker container lifecycle (start, stop, remove)
│   │   └── router.go      # Caddy Admin API route registration and restoration
│   ├── validator/     # Input validation helpers
│   └── vcs/           # Binary version info from runtime/debug.ReadBuildInfo
├── migrations/        # Ordered SQL migrations (up and down)
├── scripts/           # Utility scripts (database initialization)
├── ui/                # React dashboard (TanStack Router + Query)
├── docs/architecture.md    # This document
├── docs/roadmap.md         # Delivery phases and future evolution
├── CONTRIBUTING.md    # Setup, API reference, and contribution guidelines
├── Caddyfile          # Reverse proxy and routing configuration
├── Dockerfile         # Multi-stage build for the Go API and its dependencies
├── Makefile           # Development workflow: run, build, migrate, audit
└── compose.yml        # Full platform stack (API, DB, BuildKit, Caddy, UI)
```

## Trade-offs and Open Questions

**DNS Provider Coupling**

The v1 implementation is optimized for **Cloudflare**. While the architecture supports other providers via different `caddy-dns` plugins, Cloudflare remains the default for the "zero-config" experience. Provider abstraction is tracked in [ROADMAP.md](./roadmap.md).

**Railpack CLI and Frontend Version Coupling**

The Railpack CLI (installed in the API container at build time) and the Railpack BuildKit frontend image (pulled at runtime via `buildctl`) must be kept at matching versions. The Railpack CLI generates the build plan; the frontend consumes it. A version mismatch between them can produce silent build failures or unexpected behavior. Both are currently pinned and must be updated together whenever Railpack is upgraded.

**Orchestration Scope**

Appa currently leverages standard **Docker Compose** to fulfill its promise of a "single-command" setup for single-node environments. While Compose provides the simplicity required for v1, it is fundamentally a development tool that lacks advanced production orchestration features such as health-based service restarts, zero-downtime updates, and multi-node scaling.

The architectural constraint is that deployment and routing code should not assume Compose is the only possible service backend. The evolution path is tracked in [ROADMAP.md](./roadmap.md).

**CLI Scope**

The CLI starts as an operator tool for Appa Instance provisioning and
maintenance. Long term, it can also become a developer workflow surface for
project deployment, logs, environment variables, and rollbacks. Project-level
commands should call the Appa Server API instead of bypassing it with direct
SSH, Docker, or Ansible operations.

## Reference Documentation

### Core Technologies
*   **Railpack:** [CLI Reference](https://railpack.com/reference/cli) | [Frontend Guide](https://railpack.com/reference/frontend) | [Running in Production](https://railpack.com/guides/running-railpack-in-production)
*   **BuildKit:** [buildctl Reference](https://github.com/moby/buildkit/blob/master/docs/reference/buildctl.md) | [buildkitd.toml Config](https://docs.docker.com/build/buildkit/toml-configuration) | [Depot: BuildKit in Depth](https://depot.dev/blog/buildkit-in-depth) | [SparkFabrik: Docker BuildKit Deep Dive (Caching)](https://tech.sparkfabrik.com/en/blog/docker-cache-deep-dive) | [Earthly: What is BuildKit?](https://earthly.dev/blog/what-is-buildkit-and-what-can-i-do-with-it/)
*   **Caddy:** [Caddyfile Concepts](https://caddyserver.com/docs/caddyfile/concepts) | [Admin API Docs](https://caddyserver.com/docs/admin-api) | [Wildcard TLS Guide](https://oneuptime.com/blog/post/2026-02-08-how-to-run-caddy-with-docker-and-automatic-https-wildcard-certificates/view) | [Wildcard TLS for Multi-Tenant Systems](https://www.skeptrune.com/posts/wildcard-tls-for-multi-tenant-systems/) | [Dev/Prod Caddyfile Pattern](https://dev.to/tylerlwsmith/using-the-same-caddyfile-for-both-development-and-production-5a23) | [caddy-dns/cloudflare (GitHub)](https://github.com/caddy-dns/cloudflare)

### Infrastructure & Security
*   **Ansible:** [Ansible Lockdown (Hardening)](https://github.com/ansible-lockdown) | [Security Hardening Guide](https://oneuptime.com/blog/post/2026-01-21-ansible-security-hardening/view) | [Jeff Geerling's Ansible 101](https://www.youtube.com/playlist?list=PL2_OBreMn7FqZkvMYt6ATmgC0KAGGJNAN) | [Ansible Vault Guide](https://docs.ansible.com/ansible/latest/vault_guide/index.html)
*   **Mise:** [Getting Started](https://mise.jdx.dev/getting-started.html) | [Environments](https://mise.jdx.dev/environments)

### CLI Development
*   **urfave/cli:** [Official docs](https://cli.urfave.org/) | [v3 guide](https://cli.urfave.org/v3/) | [v3 migration guide](https://cli.urfave.org/migrate-v2-to-v3/)
*   **Cobra:** [Official repository](https://github.com/spf13/cobra) | [Cobra docs](https://cobra.dev/) | [cobra-cli generator](https://github.com/spf13/cobra-cli)
*   **Example CLIs:** [Railpack](https://github.com/railwayapp/railpack) | [Railway CLI](https://github.com/railwayapp/cli) | [GitHub CLI](https://github.com/cli/cli) | [flyctl](https://fly.io/docs/flyctl/) | [Pulumi CLI architecture](https://pulumi-developer-docs.readthedocs.io/latest/docs/architecture/README.html)

### DNS Automation
*   **Cloudflare API:** [Create Subdomain](https://developers.cloudflare.com/dns/manage-dns-records/how-to/create-subdomain) | [Zones and DNS Records](https://developers.cloudflare.com/api/resources/zones/methods/create)
*   **ACME:** [lego](https://github.com/go-acme/lego) — Go ACME client for native cert provisioning

### Backups & Container Registry
*   **Backups:** [Restic (S3-compatible)](https://restic.net) | [Docker PG Backup](https://github.com/kartoza/docker-pg-backup)
*   **Registry:** [Harbor (self-hosted OCI)](https://goharbor.io) | [Container Registry Comparison 2026](https://distr.sh/blog/container-image-registry-comparison)

### Observability
*   **Monitoring:** [dockprom (Prometheus/Grafana)](https://github.com/stefanprodan/dockprom) | [Prometheus Getting Started](https://prometheus.io/docs/prometheus/latest/getting_started/)
*   **Comparison:** [Docker Monitoring Tools Comparison 2026](https://www.dash0.com/comparisons/best-docker-monitoring-tools)
