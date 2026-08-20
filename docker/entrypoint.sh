#!/usr/bin/env bash
set -Eeuo pipefail

export MYSQL_DATABASE="${MYSQL_DATABASE:-yunnuo_license}"
export MYSQL_USER="${MYSQL_USER:-yunnuo}"
: "${MYSQL_PASSWORD:?MYSQL_PASSWORD must be set}"
: "${MYSQL_ROOT_PASSWORD:?MYSQL_ROOT_PASSWORD must be set}"
: "${YN_CARD_PEPPER:?YN_CARD_PEPPER must be set}"
: "${YN_DATA_KEY:?YN_DATA_KEY must be set}"

mysql_pid=""
app_pid=""

shutdown() {
  trap - TERM INT EXIT
  if [[ -n "$app_pid" ]] && kill -0 "$app_pid" 2>/dev/null; then
    kill -TERM "$app_pid" 2>/dev/null || true
  fi
  if [[ -n "$mysql_pid" ]] && kill -0 "$mysql_pid" 2>/dev/null; then
    mysqladmin --protocol=socket -uroot -p"$MYSQL_ROOT_PASSWORD" shutdown >/dev/null 2>&1 || kill -TERM "$mysql_pid" 2>/dev/null || true
  fi
  wait || true
}
trap shutdown TERM INT EXIT

docker-entrypoint.sh mysqld --bind-address=127.0.0.1 &
mysql_pid=$!

for attempt in $(seq 1 120); do
  if mysqladmin --protocol=tcp -h127.0.0.1 -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" ping --silent >/dev/null 2>&1; then
    break
  fi
  if ! kill -0 "$mysql_pid" 2>/dev/null; then
    echo "MySQL exited before becoming ready" >&2
    wait "$mysql_pid"
    exit 1
  fi
  if [[ "$attempt" == "120" ]]; then
    echo "MySQL did not become ready within 120 seconds" >&2
    exit 1
  fi
  sleep 1
done

export YN_ADDR="${YN_ADDR:-:8080}"
export YN_DB_CONFIG_FILE="${YN_DB_CONFIG_FILE:-/var/lib/mysql/.yunnuo/database.json}"
export YN_SQLITE_SETUP_DSN="${YN_SQLITE_SETUP_DSN:-file:/var/lib/mysql/.yunnuo/yn-license.db?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)}"
if [[ -z "${YN_DB_DRIVER:-}" && -z "${YN_DB:-}" && ! -f "$YN_DB_CONFIG_FILE" ]]; then
  export YN_DB_DRIVER=mysql
  export YN_DB="${MYSQL_USER}:${MYSQL_PASSWORD}@tcp(127.0.0.1:3306)/${MYSQL_DATABASE}?charset=utf8mb4&collation=utf8mb4_unicode_ci&multiStatements=true&parseTime=false"
fi
export YN_PUBLIC_STATIC_DIR="${YN_PUBLIC_STATIC_DIR:-/opt/yunnuo/frontend}"
export YN_ADMIN_STATIC_DIR="${YN_ADMIN_STATIC_DIR:-/opt/yunnuo/frontend/admin-console}"
export YN_AGENT_STATIC_DIR="${YN_AGENT_STATIC_DIR:-/opt/yunnuo/frontend/agent-console}"

cd /opt/yunnuo/backend
/usr/local/bin/yunnuo-server &
app_pid=$!

while kill -0 "$mysql_pid" 2>/dev/null && kill -0 "$app_pid" 2>/dev/null; do
  sleep 1
done

if ! kill -0 "$app_pid" 2>/dev/null; then
  wait "$app_pid"
  exit $?
fi
echo "MySQL exited unexpectedly" >&2
wait "$mysql_pid"
