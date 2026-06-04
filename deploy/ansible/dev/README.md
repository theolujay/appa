# Local Ansible Test Harness

This directory is for local development only. It is not part of the Appa CLI
remote provisioning contract.

## Vagrant

From this directory:

```bash
vagrant up
```

The Vagrant provisioner runs `../playbooks/security-hardening.yml` against the
local VM.

By default, the VM only uses Vagrant's NAT-forwarded SSH port. To also attach a
host-only adapter at `192.168.56.55`, run:

```bash
APPA_VAGRANT_PRIVATE_NETWORK=1 vagrant up
```

For manual Ansible runs against the Vagrant VM:

```bash
ansible-playbook -i inventory.ini ../playbooks/security-hardening.yml
```

If Vagrant times out while waiting for SSH, inspect `serial.log` in this
directory. It captures the guest console during boot.

Production and CLI-driven runs should generate or pass their own inventories
explicitly.
