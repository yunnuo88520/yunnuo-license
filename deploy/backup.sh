#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
# Run as the application owner. EnvironmentFile supplies the same effective
# config and keys as the server, plus a public age recipient and rclone target.
: "${YN_BACKUP_DIR:?set a private backup directory}"
: "${YN_BACKUP_RECIPIENT:?set the offline age public recipient}"
: "${YN_BACKUP_REMOTE:?set the rclone remote path (a different host/account)}"
: "${YN_BACKUP_OPS:?set absolute path to yunnuo-ops}"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "$YN_BACKUP_DIR"
archive="$YN_BACKUP_DIR/license-$stamp.age"
"$YN_BACKUP_OPS" backup --recipient "$YN_BACKUP_RECIPIENT" --output "$archive"
# copyto never deletes older remote backups. rclone credentials stay in its
# private config; the encryption private key never resides on this server.
rclone copyto "$archive" "$YN_BACKUP_REMOTE/license-$stamp.age"
rclone check --one-way --download "$YN_BACKUP_DIR" "$YN_BACKUP_REMOTE" --include "license-$stamp.age"
if [[ -n "${YN_BACKUP_METRICS_DIR:-}" ]]; then
  mkdir -p "$YN_BACKUP_METRICS_DIR"
  metric_tmp="$(mktemp "$YN_BACKUP_METRICS_DIR/.backup-XXXXXX")"
  printf 'yunnuo_backup_last_success_timestamp_seconds %s\n' "$(date +%s)" > "$metric_tmp"
  chmod 0644 "$metric_tmp"
  mv "$metric_tmp" "$YN_BACKUP_METRICS_DIR/yunnuo-backup.prom"
fi
# Local age files are pruned only after offsite copy AND content comparison.
find "$YN_BACKUP_DIR" -maxdepth 1 -type f -name 'license-*.age' -mtime +30 -delete
