#!/usr/bin/env bash
set -Eeuo pipefail
source "$(dirname -- "${BASH_SOURCE[0]}")/runtime.sh"
exec bash "$native_release_root/deploy/backup.sh" "$@"
