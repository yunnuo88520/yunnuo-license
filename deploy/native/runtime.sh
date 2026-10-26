#!/usr/bin/env bash
# Sourced by start/ops/backup; runtime.env is a trusted private shell env file.
set -Eeuo pipefail
native_release_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd -P)"
native_default_state="$(dirname -- "$native_release_root")/yunnuo-license-data"
native_runtime_file="${YN_RUNTIME_FILE:-$native_default_state/runtime.env}"
if [[ ! -f "$native_runtime_file" || -L "$native_runtime_file" ]]; then
  printf 'Configure first: python3 %s/deploy/native/configure.py\n' "$native_release_root" >&2
  exit 1
fi
native_permissions="$(stat -c '%a' "$native_runtime_file")"
if (( (8#$native_permissions & 077) != 0 )); then
  echo 'runtime.env must have private permissions (chmod 600)' >&2
  exit 1
fi
set -a
source "$native_runtime_file"
set +a
cd "$native_release_root/backend"
