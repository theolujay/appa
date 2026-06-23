#!/usr/bin/env bash
set -euo pipefail
[ -n "$BASH_VERSION" ] || { echo "Error: this script requires bash, not sh/ash/dash." >&2; exit 1; }

# Usage: ./make-secrets.sh <env_file> [--create|--update] [--prefix <prefix>]
# --create: Create new secrets in Docker Swarm (skip existing)
# --update: Rotate existing secrets using zero-downtime temp-secret pattern
# --prefix: Prepend <prefix>- to every derived secret name (e.g. staging, prod)

env_file=""
mode=""
prefix=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --create) mode="create"; shift ;;
    --update) mode="update"; shift ;;
    --prefix) prefix="$2"; shift 2 ;;
    --help|-h)
      echo "Usage: $0 <env_file> [--create|--update] [--prefix <prefix>]"
      echo ""
      echo "Creates or updates Docker Swarm secrets from an .env file."
      echo "The secret name is derived from the variable name (lowercase, hyphenated)."
      echo "Use --prefix to scope secrets to an environment (e.g. staging, prod)."
      echo ""
      echo "Options:"
      echo "  --create         Create new secrets (skip if already exists)"
      echo "  --update         Rotate existing secrets with zero-downtime"
      echo "  --prefix <name>  Prepend <name>- to all secret names"
      echo ""
      echo "Examples:"
      echo "  $0 .env.staging --prefix staging --create"
      echo "  $0 .env.production --prefix prod --update"
      exit 0
      ;;
    -*)
      echo "Unknown option: $1" >&2; exit 1 ;;
    *)
      if [[ -z "$env_file" ]]; then env_file="$1"
      else echo "Unexpected argument: $1" >&2; exit 1; fi
      shift ;;
  esac
done

if [[ -z "$env_file" ]]; then
  echo "Error: env file required" >&2
  echo "Usage: $0 <env_file> [--create|--update] [--prefix <prefix>]" >&2
  exit 1
fi

if [[ ! -f "$env_file" ]]; then
  echo "Error: env file '$env_file' not found" >&2
  exit 1
fi

log_info()  { echo -e "\033[0;34m[INFO]\033[0m $1"; }
log_ok()    { echo -e "\033[0;32m[OK]\033[0m $1"; }
log_warn()  { echo -e "\033[1;33m[WARN]\033[0m $1"; }

# Discover services currently using a given secret
find_services_using_secret() {
  local secret_name="$1"
  docker service ls --format '{{.Name}}' 2>/dev/null | while read -r svc; do
    if docker service inspect "$svc" --format \
        '{{range .Spec.TaskTemplate.ContainerSpec.Secrets}}{{.SecretName}}{{"\n"}}{{end}}' \
        2>/dev/null | grep -qx "$secret_name"; then
      echo "$svc"
    fi
  done
}

# Rotate a single secret using zero-downtime temp-secret pattern
rotate_secret() {
  local secret_name="$1"
  local value="$2"
  local temp_secret="${secret_name}-temp"

  if ! docker secret ls --format '{{.Name}}' | grep -qx "$secret_name"; then
    log_warn "Secret '$secret_name' does not exist. Use --create first."
    return 1
  fi

  local services
  services=$(find_services_using_secret "$secret_name")

  if [[ -z "$services" ]]; then
    log_info "No services using '$secret_name'. Performing simple replacement."
    docker secret rm "$secret_name" 2>/dev/null || true
    printf "%s" "$value" | docker secret create "$secret_name" -
    log_ok "Replaced secret: $secret_name"
    return 0
  fi

  log_info "Rotating '$secret_name' across: $(echo "$services" | tr '\n' ' ')"

  # Step 1: Create temp secret
  printf "%s" "$value" | docker secret create "$temp_secret" -

  # Step 2: Switch services to temp secret
  for svc in $services; do
    docker service update \
      --secret-rm "$secret_name" \
      --secret-add "source=$temp_secret,target=$secret_name" \
      "$svc" >/dev/null
  done

  log_info "Waiting for services to stabilize..."
  sleep 10

  # Step 3: Remove old secret
  docker secret rm "$secret_name"

  # Step 4: Create new secret with original name
  printf "%s" "$value" | docker secret create "$secret_name" -

  # Step 5: Switch services back to original name
  for svc in $services; do
    docker service update \
      --secret-rm "$temp_secret" \
      --secret-add "source=$secret_name,target=$secret_name" \
      "$svc" >/dev/null
  done

  log_info "Waiting for services to stabilize..."
  sleep 10

  # Step 6: Clean up temp secret
  docker secret rm "$temp_secret"

  log_ok "Rotated secret: $secret_name"
}

echo -e "Processing $env_file (mode: ${mode:-dry-run}, prefix: ${prefix:-none})...\n"

count_created=0
count_updated=0
count_skipped=0

while IFS= read -r line; do
  key="${line%%=*}"
  value="${line#*=}"

  [ -n "$key" ] || continue

  # If the value is an absolute path to a readable file, use its contents as the secret value
  if [[ "$value" == /* && -f "$value" ]]; then
    log_info "$key: reading secret content from file: $value"
    value=$(< "$value")
  fi

  secret_name=$(echo "$key" | tr '[:upper:]_' '[:lower:]-' | sed 's/-*$//')
  [[ -n "$prefix" ]] && secret_name="${prefix}-${secret_name}"

  if [[ "$mode" == "create" ]]; then
    if docker secret ls --format '{{.Name}}' | grep -qx "$secret_name"; then
      log_warn "Secret already exists: $secret_name (use --update to rotate)"
      count_skipped=$(( count_skipped + 1 ))
    else
      printf "%s" "$value" | docker secret create "$secret_name" - >/dev/null
      log_ok "Created secret: $secret_name"
      count_created=$(( count_created + 1 ))
    fi
  elif [[ "$mode" == "update" ]]; then
    if docker secret ls --format '{{.Name}}' | grep -qx "$secret_name"; then
      rotate_secret "$secret_name" "$value"
      count_updated=$(( count_updated + 1 ))
    else
      log_warn "Secret '$secret_name' does not exist. Creating..."
      printf "%s" "$value" | docker secret create "$secret_name" - >/dev/null
      log_ok "Created secret: $secret_name"
      count_created=$(( count_created + 1 ))
    fi
  else
    if docker secret ls --format '{{.Name}}' | grep -qx "$secret_name"; then
      echo "  ~  Would update: $secret_name"
      count_skipped=$(( count_skipped + 1 ))
    else
      echo "  +  Would create: $secret_name"
      count_created=$(( count_created + 1 ))
    fi
  fi
done < <(grep -vE '^(#|$)' "$env_file" || true)

if [[ -n "$mode" ]]; then
  echo -e "\nDone — created: $count_created, updated: $count_updated, skipped: $count_skipped."
  echo "Run 'docker secret ls' to verify."
else
  echo -e "\nDry run — would create: $count_created, would update: $count_skipped."
  echo "Use --create or --update to apply changes."
fi
