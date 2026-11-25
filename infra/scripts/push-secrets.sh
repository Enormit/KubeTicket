#!/usr/bin/env bash
set -euo pipefail

# Push KubeTicket runtime secrets (8 keys expected by ExternalSecret) to AWS Secrets Manager.
# Usage:
#   ./infra/scripts/push-secrets.sh <dev|prod> [env-file] [--dry-run]
#
# Recommended local env file name: infra/scripts/.env.secrets.local (git-ignored via .env.*.local)

ENV_NAME="${1:-}"
if [[ "$ENV_NAME" != "dev" && "$ENV_NAME" != "prod" ]]; then
  echo "Usage: $0 <dev|prod> [env-file] [--dry-run]" >&2
  exit 1
fi
shift

DRY_RUN=false
ENV_FILE=""
for arg in "$@"; do
  if [[ "$arg" == "--dry-run" ]]; then
    DRY_RUN=true
  elif [[ -z "$ENV_FILE" ]]; then
    ENV_FILE="$arg"
  fi
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
ENV_FILE="${ENV_FILE:-$SCRIPT_DIR/.env.secrets.local}"
SECRET_ID="kubeticket/${ENV_NAME}/secrets"

if [[ ! -f "$ENV_FILE" && "$DRY_RUN" != "true" ]]; then
  echo "ERROR: Env file not found: $ENV_FILE" >&2
  echo "Copy $SCRIPT_DIR/.env.secrets.example -> $SCRIPT_DIR/.env.secrets.local and fill real values." >&2
  exit 1
fi

# Resolve REDIS_URL from Terraform outputs if omitted in env file
TF_REDIS_URL=""
if command -v terraform >/dev/null 2>&1; then
  TF_DIR="$REPO_ROOT/infra/envs/$ENV_NAME"
  if [[ -d "$TF_DIR/.terraform" ]]; then
    TF_REDIS_URL="$(terraform -chdir="$TF_DIR" output -raw redis_url 2>/dev/null || true)"
  fi
fi

TMP_JSON="$(mktemp)"
trap 'rm -f "$TMP_JSON"' EXIT
chmod 600 "$TMP_JSON"

AWS_REGION="${AWS_REGION:-}"
RESOLVED_REGION="$(
python3 - "$ENV_FILE" "$TF_REDIS_URL" "$DRY_RUN" "$TMP_JSON" <<'PY'
import json, os, secrets, sys

env_file, tf_redis_url, dry_run_str, out_path = sys.argv[1:5]
dry_run = dry_run_str == "true"

# Parse .env safely without bash evaluation (handles unquoted '&' in Neon URLs)
parsed = {}
if os.path.isfile(env_file):
    with open(env_file, "r", encoding="utf-8") as f:
        for raw in f:
            line = raw.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            k, v = line.split("=", 1)
            k = k.strip()
            v = v.strip()
            if len(v) >= 2 and ((v[0] == v[-1] == '"') or (v[0] == v[-1] == "'")):
                v = v[1:-1]
            if k:
                parsed[k] = v

def get_val(key, default=""):
    return parsed.get(key) or os.environ.get(key) or default

region = get_val("AWS_REGION", "ap-southeast-1")
db_url = get_val("DATABASE_URL") or get_val("NEON_DATABASE_POOLED_URL")
direct_db_url = get_val("NEON_DATABASE_DIRECT_URL") or db_url
redis_url = get_val("REDIS_URL") or tf_redis_url

jwt_secret = get_val("JWT_SECRET")
if not jwt_secret or "change-me" in jwt_secret or "dev-insecure" in jwt_secret:
    jwt_secret = secrets.token_hex(32)
    print("INFO: Generated strong random JWT_SECRET.", file=sys.stderr)

qr_hmac = get_val("TICKET_QR_HMAC_SECRET")
if not qr_hmac or "change-me" in qr_hmac or "dev-insecure" in qr_hmac:
    qr_hmac = secrets.token_hex(32)
    print("INFO: Generated strong random TICKET_QR_HMAC_SECRET.", file=sys.stderr)

payload = {
    "DATABASE_URL": db_url,
    "NEON_DATABASE_DIRECT_URL": direct_db_url,
    "REDIS_URL": redis_url,
    "JWT_SECRET": jwt_secret,
    "TICKET_QR_HMAC_SECRET": qr_hmac,
    "TURNSTILE_SECRET_KEY": get_val("TURNSTILE_SECRET_KEY"),
    "STRIPE_SECRET_KEY": get_val("STRIPE_SECRET_KEY"),
    "STRIPE_WEBHOOK_SECRET": get_val("STRIPE_WEBHOOK_SECRET"),
}

if not dry_run:
    for req in ("DATABASE_URL", "NEON_DATABASE_DIRECT_URL", "REDIS_URL"):
        if not payload[req]:
            print(f"ERROR: {req} must not be empty.", file=sys.stderr)
            sys.exit(1)

with open(out_path, "w", encoding="utf-8") as out:
    json.dump(payload, out, indent=2)

print(region)
PY
)"
AWS_REGION="${AWS_REGION:-$RESOLVED_REGION}"

if [[ "$DRY_RUN" == "true" ]]; then
  echo "[DRY-RUN] Target Secret ID: $SECRET_ID (region: $AWS_REGION)"
  python3 - "$TMP_JSON" <<'PY'
import json, sys
data = json.load(open(sys.argv[1]))
for k, v in data.items():
    status = f"set ({len(v)} chars)" if v else "EMPTY"
    print(f"  - {k}: {status}")
PY
  exit 0
fi

if ! command -v aws >/dev/null 2>&1; then
  echo "ERROR: aws CLI is required." >&2
  exit 1
fi

if aws secretsmanager describe-secret --region "$AWS_REGION" --secret-id "$SECRET_ID" >/dev/null 2>&1; then
  aws secretsmanager put-secret-value \
    --region "$AWS_REGION" \
    --secret-id "$SECRET_ID" \
    --secret-string "file://$TMP_JSON" >/dev/null
  echo "OK: Updated secret value for '$SECRET_ID' in $AWS_REGION."
else
  aws secretsmanager create-secret \
    --region "$AWS_REGION" \
    --name "$SECRET_ID" \
    --description "KubeTicket $ENV_NAME runtime secrets (synced by External Secrets Operator)" \
    --secret-string "file://$TMP_JSON" >/dev/null
  echo "OK: Created and populated secret '$SECRET_ID' in $AWS_REGION."
fi
