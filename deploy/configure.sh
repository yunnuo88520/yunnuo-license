#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
: "${YN_DOMAIN:?set public hostname without scheme or path}"
: "${YN_METRICS_TOKEN:?set the same random metrics token as the application}"
[[ "$YN_DOMAIN" =~ ^[A-Za-z0-9.-]+$ ]] || { echo 'invalid hostname' >&2; exit 1; }
cd "$(dirname "$0")"
config_dir="${YN_DEPLOY_CONFIG_DIR:-$(pwd -P)}"
mkdir -p "$config_dir/secrets" "$config_dir/generated" "$config_dir/monitoring/textfile" "$config_dir/backups"
printf '%s' "$YN_METRICS_TOKEN" > "$config_dir/secrets/metrics-token"
printf '%s\n' '- targets:' "    - https://$YN_DOMAIN/healthz" > "$config_dir/generated/public-targets.yml"
# node_exporter needs to traverse and read the metrics directory only.
chmod 0755 "$config_dir/monitoring/textfile"
printf '%s\n' 'Monitoring target and private metrics token configured; supply an authorized alert receiver before production.'
