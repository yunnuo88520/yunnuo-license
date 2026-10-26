#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
: "${YN_DOMAIN:?set public hostname without scheme or path}"
: "${YN_METRICS_TOKEN:?set the same random metrics token as the application}"
[[ "$YN_DOMAIN" =~ ^[A-Za-z0-9.-]+$ ]] || { echo 'invalid hostname' >&2; exit 1; }
cd "$(dirname "$0")"
mkdir -p secrets monitoring/textfile backups
printf '%s' "$YN_METRICS_TOKEN" > secrets/metrics-token
printf '%s\n' '- targets:' "    - https://$YN_DOMAIN/healthz" > monitoring/public-targets.yml
# node_exporter needs to traverse and read the metrics directory only.
chmod 0755 monitoring/textfile
printf '%s\n' 'Monitoring target and private metrics token configured; supply an authorized alert receiver before production.'
