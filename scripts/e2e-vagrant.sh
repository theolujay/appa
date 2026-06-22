#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
VAGRANT_DIR="$REPO_DIR/deploy/ansible/dev"
BIN="$REPO_DIR/bin/appa"

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

# Prerequisites
command -v vagrant >/dev/null 2>&1 || fail "vagrant is required"
command -v go >/dev/null 2>&1 || fail "go is required"
command -v make >/dev/null 2>&1 || fail "make is required"

# Isolate test config from real profiles
TEST_CONFIG_DIR=$(mktemp -d /tmp/appa-e2e-XXXX)
export APPA_CONFIG_DIR="$TEST_CONFIG_DIR"

cleanup() {
    echo "=== Cleanup ==="
    cd "$VAGRANT_DIR" && vagrant destroy -f 2>/dev/null || true
    rm -rf "$TEST_CONFIG_DIR"
}
trap cleanup EXIT

cd "$REPO_DIR"

echo "=== Building CLI ==="
make build/cli 2>&1 || fail "CLI build failed"

echo "=== Booting Vagrant VM (no-provision — CLI handles all Ansible) ==="
cd "$VAGRANT_DIR"
APPA_VAGRANT_PRIVATE_NETWORK=1 vagrant up --no-provision 2>&1 || fail "vagrant up failed"

echo "=== Parsing SSH config ==="
SSH_HOST="192.168.56.55"
# SSH_PORT=22
SSH_USER="vagrant"
IDENTITY_FILE=$(vagrant ssh-config | awk '/IdentityFile/{print $2; exit}' | tr -d '"')
# Expand ~ if present
IDENTITY_FILE="${IDENTITY_FILE/#\~/$HOME}"
echo "  Identity: $IDENTITY_FILE"

cd "$REPO_DIR"

echo "=== 1. appa instance init e2e-test ==="
"$BIN" instance init e2e-test 2>&1 || fail "init failed"

echo "=== 2. appa instance set-host ==="
"$BIN" instance set-host e2e-test "$SSH_USER@$SSH_HOST" -i "$IDENTITY_FILE" --skip-verify 2>&1 || fail "set-host failed"

echo "=== 3. appa preflight ==="
"$BIN" preflight e2e-test 2>&1 || fail "preflight failed"

echo "=== 4. appa setup ==="
"$BIN" setup e2e-test --force 2>&1 || fail "setup failed"

echo "=== 5. appa status ==="
"$BIN" status e2e-test 2>&1 || fail "status failed"

echo "=== 6. appa logs ==="
timeout 5 "$BIN" logs e2e-test -n 10 2>&1 || true

echo ""
echo "====== E2E PASSED ======"
