# SSH Hardening

This role replaces the server's OpenSSH daemon configuration with a stricter
baseline for Appa-managed hosts. The goal is to keep SSH usable for automation
while reducing common remote-access risks: password guessing, broad root access,
weak crypto, uncontrolled forwarding, and loose key permissions.

## What It Changes

The role first checks whether the host uses `ssh.service` or `sshd.service`.
Ubuntu and Debian commonly use `ssh`; RHEL-style systems commonly use `sshd`.
If neither service exists, the role fails instead of guessing.

The role writes `/etc/ssh/sshd_config`, validates it with `sshd -t` before it is
accepted, and restarts the detected SSH service if the configuration changes.
Validation matters because a broken SSH config can lock an operator out of a
remote server. The restart is allowed to fail normally, because a failed SSH
restart is a real operational problem that should not be hidden.

It also:

- Sets strict permissions on SSH host private keys.
- Removes the legacy DSA host key.
- Installs a login banner at `/etc/ssh/banner`.

## Login Policy

SSH listens on `ssh_port`, which defaults to `22`.

Root login is disabled by default:

```text
PermitRootLogin no
```

If `allow_root_ssh_keys` is set to `true`, root login is relaxed to
key-only access:

```text
PermitRootLogin prohibit-password
```

That still blocks root password login. It only allows root login with an
authorized SSH key. The default is stricter because Appa should normally connect
as an operator or deploy user, then use sudo for privileged actions.

Password authentication is disabled by default:

```text
PasswordAuthentication no
PermitEmptyPasswords no
ChallengeResponseAuthentication no
```

This makes SSH key authentication the normal access path and removes the main
attack surface for internet-exposed SSH: repeated password attempts.

## Allowed Users

The role writes an `AllowUsers` rule. By default it includes:

- The current Ansible connection user.
- `deploy`.

Including the current Ansible user is intentional. Without that, a first
hardening run could apply successfully and then immediately lock out the same
user that Ansible is using to connect.

For production, the CLI or inventory should set `ssh_allowed_users` explicitly
to the small set of accounts that should be able to log in.

## Session Limits

The role limits repeated login attempts and idle sessions:

```text
MaxAuthTries 3
MaxSessions 5
LoginGraceTime 30
ClientAliveInterval 300
ClientAliveCountMax 2
```

These settings reduce brute-force tolerance and clear dead SSH sessions after a
reasonable timeout. They are not meant to replace firewall rules or rate
limiting, but they make SSH less permissive.

## Forwarding

TCP forwarding, X11 forwarding, and agent forwarding are disabled by default:

```text
AllowTcpForwarding no
X11Forwarding no
AllowAgentForwarding no
```

Forwarding features are useful for some workflows, but they also let an SSH
session become a tunnel into other services or networks. Appa's provisioning
path does not need them by default, so the role turns them off unless explicitly
enabled.

## Crypto Baseline

The role prefers modern host keys and algorithms:

- Ed25519 host keys first.
- ECDSA and RSA host keys as fallbacks.
- DSA host keys removed.
- Modern key exchange, ciphers, and MACs.

This avoids older SSH primitives that are no longer appropriate for new server
baselines.

## Strict File Checks

The SSH daemon is configured with:

```text
StrictModes yes
```

OpenSSH will reject insecure ownership or permissions on user SSH files. This is
useful because SSH keys are only meaningful if unauthorized users cannot modify
or read the relevant files.

The role also sets host private keys to `0600` and owned by `root:root`.

## Logging

The role uses:

```text
SyslogFacility AUTH
LogLevel VERBOSE
```

Verbose SSH logging records more detail about authentication events, including
key fingerprints. That helps when investigating access, failed login attempts,
and unexpected keys.

## Why `requiretty` Was Removed From Sudoers

`requiretty` is a sudo setting, not an SSH setting, but it directly affects
Ansible over SSH.

When `requiretty` is enabled, sudo refuses to run unless the command has an
interactive terminal attached. That was historically used to make sure sudo was
run by a person in a terminal session.

Ansible usually runs remote commands non-interactively over SSH. This repo also
enables SSH pipelining in `ansible.cfg`, which is specifically designed to avoid
extra remote shell/TTY overhead. With `requiretty` enabled, privileged Ansible
tasks can fail even when the user has valid sudo rights.

Removing `requiretty` keeps automated provisioning reliable.

The security tradeoff is acceptable here because sudo is still constrained by:

- Which users are allowed to use sudo.
- Which commands can be run without a password.
- Sudo logging.
- `use_pty`, which is still enabled and gives sudo a pseudo-terminal for better
  command logging behavior.

So the intent is: do not require a human interactive TTY for automation, but
keep sudo audited and scoped.

## Lockout Risks

This role can lock operators out if applied with bad variables. The risky
settings are:

- `ssh_port`: changing it without opening the same port in the firewall.
- `ssh_allowed_users`: omitting the user Ansible needs for future runs.
- `allow_password_auth: false`: applying this before SSH keys are installed.
- `allow_root_ssh_keys: false`: relying on root SSH after disabling it.

For first-time provisioning, Appa should apply SSH hardening only after it has a
known-good non-root user, working SSH key authentication, and matching firewall
rules.

## Current Defaults

```yaml
ssh_port: 22
ssh_allowed_users:
  - current Ansible user
  - deploy
allow_root_ssh_keys: false
allow_password_auth: false
allow_tcp_forwarding: false
allow_agent_forwarding: false
```

These defaults are strict enough for a public VPS baseline while still trying to
avoid accidental lockout during the first Ansible run.
