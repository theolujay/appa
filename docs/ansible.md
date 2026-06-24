# Appa Ansible

This directory contains Ansible playbooks and roles for host-level Appa server
hardening. It is intended to be called by the future Appa CLI, but can also be
run directly against a generated or local test inventory.

## Playbooks

- `playbooks/security-hardening.yml` — applies kernel, account, SSH, firewall,
  and audit hardening via all five roles.
- `playbooks/compliance-scan.yml` — read-only checks for the same control areas
  and reports drift.
- `playbooks/deploy-stack.yml` — installs Docker, renders Appa Stack environment
  and Stack templates, starts platform services on the remote VPS.

## Roles

All five are under `roles/`:

| Role | Group | Backend | Description |
|------|-------|---------|-------------|
| `kernel_hardening` | A | `ansible.posix.sysctl` | 26 kernel params + module blacklisting |
| `access_control` | A | `libpam-pwquality`, groups, sudoers | Password policies, groups, sudo, file perms |
| `ssh_hardening` | B | systemd (sshd) | SSH config hardening, key cleanup, banner |
| `firewall` | B | `community.general.ufw` | UFW default deny + allow SSH/HTTP/HTTPS |
| `audit` | B | systemd (auditd) | auditd config + rules for identity/network/kernel monitoring |

**Group A** — no systemd dependency, run in a plain Docker container.

**Group B** — manage system services; need `privileged: true` + `command:
/sbin/init` in their molecule config.

## Molecule Testing

Molecule replaces Vagrant for routine role-level testing. Each role and the
playbook has a `molecule/default/` scenario with three files:

| File | Purpose |
|------|---------|
| `molecule.yml` | Driver, platform image, provisioner config |
| `converge.yml` | Playbook that applies the role under test |
| `verify.yml` | Assertions that the role produced the expected state |

Run tests from the role directory:

```bash
cd deploy/ansible/roles/ssh_hardening
molecule converge   # create container + apply role
molecule verify     # run assertions
molecule test       # full cycle: destroy → create → converge → idempotence → verify → destroy
```

Or use Makefile shortcuts from the repo root:

```bash
make ansible/molecule/role ROLE=ssh_hardening CMD=test
make ansible/molecule/role ROLE=firewall      CMD=converge
make ansible/molecule/playbook               CMD=test
make ansible/molecule/test/all                # all roles + playbook
```

### Per-role caveats

- **`kernel_hardening`** — needs `disable_ipv6: false` set and `/etc/modprobe.d`
  to exist (created in pre_tasks).
- **`access_control`** — sets permissions on `/etc/ssh/sshd_config`; installs
  `openssh-server` in pre_tasks.
- **`ssh_hardening`** — requires `openssh-server` and `/run/sshd` for the
  `sshd -t` template validation.
- **`firewall`** — requires `community.general` collection; runs ufw with
  `ansible.builtin.package` for install.
- **`audit`** — `auditd` cannot start inside Docker containers (needs kernel
  audit subsystem). Config files are deployed and verified; the service start
  is ignored in molecule.

## CI

See `.github/workflows/ansible-tests.yml`. The pipeline:

1. Lint (production profile, all files)
2. Molecule test (matrix of 6 scenarios — 5 roles + playbook — in parallel)

Replace `latest` with a pinned image digest in CI to avoid upstream changes.

## Local Test Run (Vagrant)

The Vagrant harness in `dev/` boots an Ubuntu VM and applies the hardening
playbook:

```bash
make vagrant/up          # from repo root
# or manually:
cd deploy/ansible/dev
source ../.venv/bin/activate && vagrant up
```

Direct playbook runs from `deploy/ansible`:

```bash
ansible-playbook -i dev/inventory.ini playbooks/security-hardening.yml
ansible-playbook -i dev/inventory.ini playbooks/compliance-scan.yml
```

### Known Vagrant Issues

**VirtualBox Guest Additions version mismatch** — The bento box ships with
Guest Additions 7.2.4, while the host may run a newer VirtualBox (e.g. 7.2.8).
The `vagrant-vbguest` plugin auto-matches them, but the box's stale apt cache
can cause 404 errors when installing build dependencies.

**Workaround:** If Vagrant times out or SSH resets repeatedly:

1. Install the plugin: `vagrant plugin install vagrant-vbguest`
2. Destroy and rebuild: `make vagrant/destroy && make vagrant/up`
3. If 404 errors appear, the VM may need a full `apt-get update && apt-get upgrade`
   first. Temporarily uncomment `v.cpus = 2` in `dev/Vagrantfile` and retry.

## Important Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `ssh_port` | `22` | SSH daemon port |
| `ssh_allowed_users` | `['deploy']` | Users allowed SSH login |
| `allow_root_ssh_keys` | `false` | Allow root key-based SSH login |
| `allow_password_auth` | `false` | Allow SSH password auth |
| `disable_ipv6` | `false` | Disable IPv6 via sysctl |
| `firewall_backend` | `ufw` (Debian), `firewalld` (else) | Firewall backend |
| `ufw_rules` | SSH, HTTP, HTTPS | UFW allow rules |
| `firewall_rules` | SSH, HTTP, HTTPS | firewalld rules |
| `iptables_rules` | SSH, HTTP, HTTPS | iptables rules |
| `sudo_rules` | `[]` | Additional sudo rules |

Production inventory and secrets should be generated by the CLI or supplied by
the operator. Do not commit real host credentials, provider tokens, SMTP
passwords, or backup keys.

## Gotchas

- **`meta/main.yml` required** — molecule needs it to auto-link the role.
  Each role has one; keep it updated.
- **`group_vars/` not loaded in role-level molecule tests** — only role
  defaults are available. Group vars are loaded in the playbook scenario.
- **`community.general` and `ansible.posix`** — required for ufw and sysctl
  modules. Install via `ansible-galaxy collection install` or `pyproject.toml`
  dependency group.
- **`deploy/ansible/README.md`** now lives here. The file at
  `deploy/ansible/README.md` is a pointer.
