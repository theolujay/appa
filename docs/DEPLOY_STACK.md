# Deploy Stack Playbook Plan

The `deploy-stack.yml` playbook is the second Ansible playbook called by
`appa setup <name>` (after `security-hardening.yml`). It prepares the remote VPS
to run the Appa Stack and starts the platform services.

## What the CLI Passes

The CLI generates a temporary inventory and passes these `extra_vars`:

| Variable | Source | Purpose |
|---|---|---|
| `appa_domain` | Profile domain | Production domain or empty (IP-only) |
| `cloudflare_token` | Profile token | DNS-01 wildcard TLS |
| `smtp_host`, `smtp_port`, `smtp_username`, `smtp_password` | Profile SMTP | Email delivery |

## Playbook Structure

```
deploy-stack.yml
└── roles/
    ├── docker/              # Docker + compose installation
    ├── appa_stack/          # Dirs, env, compose, stack lifecycle
    └── caddy/               # Production Caddy image + TLS config
```

## Role: `docker`

**Tasks:**
1. Install Docker from the official repo (not distro-pinned)
2. Install Docker Compose plugin (`docker-compose-plugin`)
3. Add the operator SSH user to the `docker` group
4. Verify Docker works (`docker info`) and fail with a clear message if not
5. Start and enable `docker.service`

**Idempotence:** Standard package/module modules are idempotent. The `docker
group` task uses `user` with `append: yes` and `groups: docker`.

**Molecule:** Can use the Docker driver with a sibling container or
`docker_in_docker` image, or can be tested separately via Vagrant (existing
`deploy/ansible/dev/`).

## Role: `appa_stack`

**Tasks:**

### 1. Create directory structure

```
/opt/appa/
├── .env                    # Rendered from template
├── compose.yml             # Stack service definitions
├── Caddyfile               # Production Caddy config
├── scripts/
│   └── entrypoint.sh       # DB migration + API start
└── data/
    ├── caddy/              # Caddy data + config volumes
    ├── postgres/           # PG data volume bind
    └── railpack-cache/     # Build cache
```

### 2. Render `.env`

Template `env.j2` containing all variables the API container needs at runtime:

```ini
APPA_DB_DSN=postgres://appa:{{ pg_password }}@db:5432/appa?sslmode=disable
POSTGRES_USER=appa
POSTGRES_PASSWORD={{ pg_password }}
POSTGRES_DB=appa
PORT=8080
ENV=production
SMTP_HOST={{ smtp_host }}
SMTP_PORT={{ smtp_port }}
SMTP_USERNAME={{ smtp_username }}
SMTP_PASSWORD={{ smtp_password }}
SMTP_SENDER="Appa <{{ smtp_sender }}>"
CORS_TRUSTED_ORIGINS={{ cors_origins }}
RAILPACK_VERSION=v0.23.0
LIMITER_RPS=2
LIMITER_BURST=4
LIMITER_ENABLED=true
DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=25
DB_MAX_IDLE_TIME=15m
```

**Operator secrets are never logged.** The template task includes
`no_log: true` on the `.env` rendering step to prevent Ansible from spilling
the file contents into logs.

### 3. Generate a random PostgreSQL password if none set

Use `ansible.builtin.password` with `length=32` and store it in a file
(`/opt/appa/.pg_password`) on first run. Subsequent runs read the existing
password so the stack doesn't get a new database password on every `apply`.

### 4. Render `compose.yml`

Template `compose.yml.j2` mirrors the dev `compose.yml` with these
production adjustments:

| Change | Reason |
|---|---|
| Use `image: ghcr.io/theolujay/appa-api:latest` instead of `build: .` | No Docker build context on the VPS |
| Use `image: ghcr.io/theolujay/appa-ui:latest` instead of `build: ./ui` | Same |
| Mount `.env` file instead of `env_file: .env` | Generated env file |
| Remove `develop` blocks | Not needed in production |
| Add `restart: unless-stopped` to all services | Survivability |
| Bind-mount `./data/postgres` instead of named volume | Easier backup/access |
| Bind-mount `./data/caddy/` for caddy data | Persist certs across restarts |
| Bind-mount `./data/railpack-cache/` for build cache | Speed up builds |
| Set `PRODUCTION_HOST_IP` env on the API | Caddy needs the host IP for ACME |
| Pin service image versions | Deterministic deployments |

### 5. Render `Caddyfile`

Template `Caddyfile.j2` for production:

```
{
    admin 0.0.0.0:2019
    debug
}

{{ appa_domain }}:80, {{ appa_domain }}:443 {
    tls {
        dns cloudflare {{ cloudflare_token }}
    }

    handle /v1/* {
        reverse_proxy api:8080
    }

    handle {
        reverse_proxy ui:5173
    }
}
```

If `appa_domain` is not set (IP-only setup), fall back to a simpler config
without TLS:

```
{
    admin 0.0.0.0:2019
    auto_https off
    debug
}

http://{{ ansible_host }}:80 {
    handle /v1/* {
        reverse_proxy api:8080
    }

    handle {
        reverse_proxy ui:5173
    }
}
```

### 6. Render `entrypoint.sh`

Template that runs DB migrations and starts the API binary. Identical to
`scripts/entrypoint.sh` but placed at `/opt/appa/scripts/entrypoint.sh`.

### 7. Start the Appa Stack

```yaml
- name: Start the Appa Stack
  community.docker.docker_compose:
    project_src: /opt/appa
    state: present
    restarted: "{{ stack_updated }}"
```

Alternatively, use `docker compose up -d` via `ansible.builtin.command` if
Docker Compose integration is more reliable.

## Role: `caddy`

**Tasks:**
1. If `cloudflare_token` is set, build or pull a custom Caddy image with
   `caddy-dns/cloudflare` plugin

   Use a Dockerfile template and build on the target:
   ```dockerfile
   FROM caddy:2.11.2-alpine
   RUN apk add --no-cache xcaddy && \
       xcaddy build \
         --with github.com/caddy-dns/cloudflare
   ```

   Tag the result as `caddy-cloudflare:local` and reference it in `compose.yml`.

2. If no Cloudflare token, use the official `caddy:2.11.2-alpine` image with
   `auto_https off`.

This role only runs when `cloudflare_token` is set. Otherwise the Caddy config
in `appa_stack` handles the non-TLS case.

## File Layout

```
deploy/ansible/
├── roles/
│   ├── docker/
│   │   └── tasks/main.yml
│   ├── appa_stack/
│   │   ├── tasks/main.yml
│   │   ├── templates/
│   │   │   ├── env.j2
│   │   │   ├── compose.yml.j2
│   │   │   ├── Caddyfile.j2
│   │   │   └── entrypoint.sh.j2
│   │   └── defaults/main.yml
│   └── caddy/
│       ├── tasks/main.yml
│       └── defaults/main.yml
├── playbooks/
│   └── deploy-stack.yml
├── group_vars/
│   └── all/
│       └── deploy-stack.yml    # Defaults for stack vars
└── pyproject.toml
```

## Implementation Order

1. Create `roles/docker/tasks/main.yml`
2. Create `roles/appa_stack/tasks/main.yml` and its templates
3. Create `roles/caddy/tasks/main.yml`
4. Create `playbooks/deploy-stack.yml`
5. Add molecule scenario for `deploy-stack` (requires a VM or Docker-in-Docker)
6. Wire into the CLI's `setup` and `apply` commands (already done)

## Testing Strategy

The Docker role and stack deployment are hard to test inside a Docker container
(since you need a Docker daemon to test Docker installation). Use:

- **Vagrant** (existing `deploy/ansible/dev/`) for full integration tests
- **Syntax check** and `--check` mode for linting playbook logic
- **Molecule with `docker` driver** for individual role syntax testing (use
  `delegate_to: localhost` or a dedicated Docker image with `docker-ce`
  preinstalled)

After the playbook passes `ansible-lint` and `--check` on a Vagrant box, wire
it into the existing `make ansible/molecule/test/all` flow.

## Open Questions

1. **Image registry:** Where are the API and UI images published? The dev
   compose builds from source, but production needs pre-built images. Options:
   GitHub Container Registry (`ghcr.io`), Docker Hub, or a private registry.

2. **Caddy custom image build:** Building `xcaddy` on the target VPS takes
   time. Should we pre-publish `caddy-cloudflare` to GHCR instead? This would
   speed up `appa setup` significantly.

3. **Staging path:** The CLI `setup` command currently runs security-hardening
   first, then deploy-stack. After deploy-stack starts the stack, the API needs
   a database to be ready. The entrypoint script runs migrations before starting
   the API binary. Does the CLI need to handle a longer initial startup delay on
   first run (no DB volume yet)?

4. **PostgreSQL password rotation:** The `/opt/appa/.pg_password` file is a
   stopgap. Long term this should use Ansible Vault. Document the migration
   path in `docs/ansible.md`.
