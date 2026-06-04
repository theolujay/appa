# Appa CLI: Command Specification

Source documents: `docs/architecture.md`, `docs/roadmap.md`, `deploy/ansible/`.

## Command Tree

```
appa
├── instance init <name>          # Create an instance profile
├── instance set-host <name> <target>  # Set SSH target
├── instance list                 # List saved profiles
├── preflight <name>              # Validate target before setup
├── setup <name>                  # First-time provisioning (runs Ansible)
├── apply <name>                  # Re-apply config after changes
├── status <name>                 # Show instance health
├── logs <name>                   # Tail Appa Stack logs
├── restart <name>                # Restart Appa Stack services
├── upgrade <name>                # Upgrade to latest Appa version
└── help
```

---

## `appa instance init <name>`

Create a new local instance profile.

**Flow:**
1. Create a directory `~/.appa/instances/<name>/`
2. Write a blank or default config file (TOML or YAML) ready for `set-host`
3. Print success message

**Profile config** (stored locally, never sent to server):
```toml
name = "personal"
ssh_host = ""
ssh_user = "root"
ssh_port = 22
domain = ""
cloudflare_token = ""
smtp_host = ""
smtp_port = 587
smtp_username = ""
smtp_password = ""
```

**Flags:**
- `--dir` — custom config directory (default `~/.appa/`)

**Exit codes:** 0 on success, 1 on error

---

## `appa instance set-host <name> <target>`

Set the SSH target for an existing profile.

**`<target>` format:** `user@host[:port]` (e.g. `root@203.0.113.10` or `root@203.0.113.10:2222`)

**Flow:**
1. Validate the profile exists
2. Validate `<target>` format (must match `user@host` or `user@host:port`)
3. Test SSH connectivity with a brief connection attempt
4. Update profile config with host info
5. Print the resolved IP and a "connection succeeded/failed" message

**Exit codes:** 0 on success, 1 if profile missing or invalid target, 2 if SSH connection test fails

---

## `appa instance list`

List all saved instance profiles.

**Flow:**
1. Scan `~/.appa/instances/` for subdirectories
2. Load each profile
3. Print table: name, host, domain, setup status (has SSH target? has domain?)

**Example output:**
```
NAME       HOST                    DOMAIN    SETUP
personal   root@203.0.113.10       -         done
staging    root@198.51.100.20      -         preflight OK
```

---

## `appa preflight <name>`

Run preflight checks on the target server before provisioning.

**Checks (in order, stop on first failure):**
1. Profile exists and has SSH target
2. SSH reachable — `ssh -o ConnectTimeout=5 user@host true`
3. OS is supported — run `cat /etc/os-release`, check for `ubuntu` (initially)
4. Required ports open — check 22 (SSH), 80 (HTTP), 443 (HTTPS) aren't firewalled
5. DNS resolves — if a domain is configured in the profile, verify it resolves to the target IP
6. Docker not already installed (warn if found — `which docker`)
7. Required operator inputs present — warn if SMTP, Cloudflare tokens are missing but tell the user they can be added later

**Output:** Green checkmarks for passing, red X for failing. Summary at end: "All checks passed" or "X failures, Y warnings"

**Exit codes:** 0 if all critical checks pass, 1 if SSH/OS/port checks fail, 2 if only non-critical warnings

---

## `appa setup <name>`

First-time provisioning of an Appa instance.

**Prerequisites:**
- Profile exists with SSH target
- Preflight passes (check is re-run or user can skip with `--force`)

**Ansible integration:**
1. Generate an Ansible inventory file from the profile config
2. Generate an extra-vars JSON file from profile (SSH key, domain, SMTP settings, tokens)
3. Run: `ansible-playbook -i <generated-inventory> deploy/ansible/playbooks/security-hardening.yml`
4. Run: `ansible-playbook -i <generated-inventory> deploy/ansible/playbooks/deploy-stack.yml`
5. Wait for the Appa API to become reachable (poll `https://<host>/v1/health` up to 60s)
6. Print the Appa Server URL and next steps

**Flags:**
- `--force` — skip preflight checks
- `--tags` — pass specific Ansible tags (e.g. `--tags firewall,ssh`)
- `--skip-tags` — skip specific Ansible tags

**Profile updates after setup:**
- Mark the profile as "setup complete"
- Store the API URL and initial admin credentials

**Exit codes:** 0 on success, 1 on any failure

---

## `appa apply <name>`

Re-apply configuration changes (idempotent).

**Same as `setup` but:**
- Skips initial preflight (still checks SSH reachability)
- Runs only the applicable Ansible playbooks/roles
- Doesn't re-generate initial credentials
- Meant for changing domain, SMTP, firewall rules, etc.

**Flags:**
- `--tags`, `--skip-tags` — same as `setup`

**Exit codes:** 0 on success, 1 on failure

---

## `appa status <name>`

Show the health of a running Appa instance.

**Checks:**
1. SSH connectivity
2. If setup is complete, check API health endpoint
3. List running Docker Compose services and their state
4. Check disk usage / memory (basic)

**Output:** Table or summary of each service and its state (running/stopped/unknown)

---

## `appa logs <name>`

Tail logs from the Appa Stack services.

**Implementation:** SSH into the instance and run `docker compose -f /opt/appa/compose.yml logs -f`

**Flags:**
- `--service` / `-s` — filter to one service (api, db, buildkit, caddy, ui)
- `--tail` / `-n` — number of lines to show (default 50)

---

## `appa restart <name>`

Restart the Appa Stack services.

**Implementation:** SSH into the instance and run `docker compose -f /opt/appa/compose.yml restart`

**Flags:**
- `--service` / `-s` — restart only one service

---

## `appa upgrade <name>`

Upgrade the Appa Stack to the latest version.

**Flow:**
1. SSH into the instance
2. Pull latest images: `docker compose -f /opt/appa/compose.yml pull`
3. Recreate services: `docker compose -f /opt/appa/compose.yml up -d`
4. Wait for API health check
5. Report new versions

**Flags:**
- `--version` — pin to a specific version tag

---

## Future commands (not yet spec'd)

From docs/roadmap.md:

```
appa project init <name>          # Local project metadata
appa deploy                       # Deploy to an Appa instance
appa logs                         # Tail deployment logs
appa env                          # Manage deployment env vars
appa stop                         # Stop a deployment
ppa rollback                      # Rollback to previous deployment
```

These call the **Appa Server API**, not SSH/Ansible.

---

## Implementation notes

**Config storage:** `~/.appa/instances/<name>/config.toml` — use `BurntSushi/toml` (already a dependency in `go.mod`) or `gopkg.in/yaml.v3`.

**SSH operations:** Use `golang.org/x/crypto/ssh` (already in `go.mod`) or shell out to the system `ssh` command for simplicity. Shelling out is simpler for `docker compose` passthrough and easier to debug.

**Ansible invocation:** Shell out to `ansible-playbook` (must be on PATH). The CLI already adds the project venv to PATH via the Makefile — same pattern.

**Secret handling:** Instance profiles may contain Cloudflare tokens and SMTP passwords. Store the config file with `0600` permissions. Warn if permissions are too permissive.

---

## References & Tutorials

**Cobra:**
- [User Guide (official)](https://github.com/spf13/cobra/blob/main/site/content/user_guide.md)
- [DigitalOcean: How to Use Cobra](https://www.digitalocean.com/community/tutorials/how-to-use-the-cobra-package-in-go)
- [GitHub CLI source](https://github.com/cli/cli) — real-world Cobra CLI, study `cmd/gh/main.go` → `pkg/cmd/`

**urfave/cli (for comparison):**
- [Getting Started (official)](https://cli.urfave.org/)
- [Railpack CLI source](https://github.com/railwayapp/railpack) — mentioned in docs/architecture.md

**Existing deps in go.mod you can reuse:**
- `BurntSushi/toml` — profile config files
- `golang.org/x/crypto/ssh` — SSH operations (or shell out to `ssh`)
- `golang.org/x/crypto/ssh/knownhosts` — SSH host key verification
