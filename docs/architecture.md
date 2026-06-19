# Appa Architecture

Appa is a self-hosted, CLI-native deployment platform. The product has two
surfaces: the Appa CLI installed on the operator's machine, and an Appa Server
instance running on a remote VPS. The server stack combines a Go API,
PostgreSQL, BuildKit, Railpack, Docker, Caddy, and a React dashboard.

This document answers architectural questions. Setup and contribution guidelines
live in [CONTRIBUTING.md](../CONTRIBUTING.md). Delivery phases live in
[ROADMAP.md](./roadmap.md). Research notes, data model, state machines, and
external references live in [REFERENCE.md](./reference.md).

## Glossary

| Term | Meaning |
| --- | --- |
| Appa CLI | Local operator command-line tool, binary `appa`. |
| Appa Server | Remote API, dashboard, and deployment runtime on a VPS. |
| Appa Instance | One Appa Server installation managed by the CLI. |
| Appa Stack | Server-side services: API, UI, PostgreSQL, BuildKit, Caddy. |
| Instance profile | Local CLI configuration for one instance (SSH target, settings). |
| Deployment | One submitted source package and its lifecycle state. |
| Railpack | Runtime detector and build-plan generator. |
| BuildKit | Build daemon that executes the Railpack build plan. |
| App container | User workload container created from the built image. |
| Route | Caddy reverse-proxy mapping from hostname to container address. |
| Hub | In-process WebSocket broadcaster for logs and status. |
| `appa_net` | Internal Docker network for platform services and app containers. |

## Components

```
Operator Machine
  └── Appa CLI (instance profiles, preflight, setup/apply/status)
        │ SSH + Ansible
        ▼
Remote VPS / Appa Instance
  └── Appa Stack (Docker Compose services + generated config)
        │
        ▼
Browser → [ React Dashboard ] → [ Caddy Gateway ]
                                     ├── [ Appa API ] (Go, auth, pipeline, WebSocket hub)
                                     └── [ User App Route → App Container ]
                                             │
                              ┌──────────────┼──────────────┐
                              ▼              ▼              ▼
                        [ PostgreSQL ]  [ BuildKit ]   [ Docker API / Caddy Admin API ]
                                                │
                                    [ Railpack CLI + Frontend ]
```

The CLI is the operator-facing control surface for provisioning. The Appa Server
is the authority for deployments, builds, containers, routes, logs, users, and
tokens.

## Core Flows

### Operator Provisions an Instance

1. Install CLI (`appa.theolujay.dev/install.sh`).
2. `appa instance init personal` → creates `~/.appa/instances/personal/config.toml`.
3. `appa instance set-host personal root@203.0.113.10` → SSH target.
4. `appa preflight personal` → validates SSH, OS, ports, DNS, inputs.
5. `appa setup personal` → runs Ansible (security-hardening then deploy-stack).
6. Ansible installs Docker, writes env/config/templates, starts Appa Stack.
7. CLI reports URL for later `apply`, `status`, `logs`, `restart`, `upgrade`.

Progressive configuration: SSH target first, then domain, Cloudflare, SMTP, etc.

### User Deploys Code

1. API creates `Deployment` row with `pending` status.
2. Pipeline clones Git repo (or extracts uploaded ZIP).
3. `railpack prepare` inspects source, writes build plan.
4. `buildctl build` runs Railpack frontend; image tar streams into `docker load`.
5. Docker starts `appa-{deployment_id}` on `appa_net`.
6. Pipeline waits for container port to accept TCP.
7. Caddy receives route from `{deployment_id}.localhost` to container.
8. Deployment status → `running`.

### Logs Streaming

1. HTTP upgraded to WebSocket.
2. Historical logs loaded from PostgreSQL first.
3. Hub registers connection for live build/deploy/route events.
4. Logs persisted before live publication; reconnecting clients replay history.

### Route Restoration

1. API queries PostgreSQL for `running` deployments on startup.
2. Each with a stored address is re-registered in Caddy.
3. Individual failures are logged, not aborting the pass.

### Cancel / Stop

1. Active pipeline task is cancelled, or container is stopped.
2. Caddy route removed.
3. Status → `stopped` or `canceled`.

## Enforceable Invariants

1. Every deployment read checks `deployment.user_id` — ownership-scoped.
2. Build output (image tar) pipes directly into `docker load` — never through text scanners.
3. Railpack CLI and frontend versions are kept compatible — upgraded as one unit.
4. Caddy Admin API (port 2019) is internal to `appa_net`.
5. Containers reachable by stable names: `appa-{deployment_id}:{port}` on `appa_net`.
6. Status changes persisted to PostgreSQL before hub publication.
7. Route registration after container passes TCP readiness.
8. Logs persisted before WebSocket publication.
9. Uploaded archives extracted into isolated per-upload directories.
10. Platform and user workloads share a network, not a process.
11. CLI is not a second deployment engine — project deploys go through the API.
12. `setup`/`apply` are idempotent.
13. Operator secrets are never logged.
14. Local instance profiles and credentials are encrypted using Ansible Vault (planned).

## Failure Model

| Failure | Recovery |
| --- | --- |
| Git clone / railpack / BuildKit fails | `failed` status; build logs persisted. |
| API exits during build | In-memory state lost; persisted deployment survives. No automatic resumption. |
| Container fails readiness | `failed` status; error in logs. |
| Caddy route fails | Pipeline failure; not `running`. |
| Caddy restarts | Startup route restoration re-adds `running` deployment routes. |
| WebSocket disconnects | Hub unregisters client; persisted logs for replay. |

## Design Decisions

- **Caddy as network boundary** — Only public HTTP boundary. Routes platform traffic by host header.
- **Caddy in Docker** — Keeps dev/prod close; enables custom image distribution.
- **BuildKit as separate privileged service** — Isolates elevated capabilities from the API.
- **Two-phase Railpack build** — `prepare` inspects, `buildctl` executes.
- **Deterministic naming** — `appa-{deployment_id}` for images, containers, and routes.
- **Port selection** — First exposed image port, default 3000 (80 for static servers).
- **WebSocket hub pattern** — Single goroutine for registration/broadcast; DB as durable store.
- **Wildcard TLS via DNS-01** — HTTP-01 doesn't support wildcards; Cloudflare + caddy-dns plugin.
- **CLI-managed provisioning** — CLI owns profiles/preflight/Ansible gen; Ansible owns host mutations; API owns deployments.
- **Ansible Vault for Local Secrets** (Planned) — Instance profiles and operator configuration (including passwords, tokens, and SSH keys) are encrypted on the operator's disk using Ansible Vault to protect secrets at rest.
