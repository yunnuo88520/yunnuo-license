#!/usr/bin/env bash
set -Eeuo pipefail
source "$(dirname -- "${BASH_SOURCE[0]}")/runtime.sh"
exec "$native_release_root/backend/yunnuo-server" "$@"
