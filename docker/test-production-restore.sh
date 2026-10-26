#!/usr/bin/env bash
set -Eeuo pipefail
container="${1:?container name required}"
state="${2:?update state path required}"
port="${3:-18084}"
# All identities and credentials below belong to this disposable acceptance DB.
# A production restore must use an offline identity and a fresh database.
docker exec "$container" sh -c 'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql -uroot -e "CREATE DATABASE yunnuo_restore CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci; GRANT ALL ON yunnuo_restore.* TO '\''yunnuo'\''@'\''%'\'';"'
recipient="$(docker exec "$container" yunnuo-ops keygen --output /tmp/restore-test-identity)"
docker exec "$container" yunnuo-ops backup --recipient "$recipient" --output /tmp/full-backup.age
printf '%s\n' 'yunnuo:docker-test-db-password@tcp(127.0.0.1:3306)/yunnuo_restore?charset=utf8mb4&multiStatements=true&parseTime=false' | docker exec -i "$container" sh -c 'umask 077; cat > /tmp/restore-dsn'
docker exec "$container" yunnuo-ops restore --input /tmp/full-backup.age --identity /tmp/restore-test-identity --destination /tmp/restored-instance --mysql-dsn-file /tmp/restore-dsn
# Verify fresh database parity before booting its own application process.
docker exec "$container" sh -c 'MYSQL_PWD="$MYSQL_PASSWORD" mysql -uyunnuo -N -e "SELECT COUNT(*) FROM yunnuo_restore.licenses; SELECT COUNT(*) FROM yunnuo_restore.agent_money_ledgers; SELECT COUNT(*) FROM yunnuo_restore.schema_migrations;"' > /tmp/yunnuo-restore-counts-$$
docker exec "$container" sh -c 'MYSQL_PWD="$MYSQL_PASSWORD" mysql -uyunnuo -N -e "SELECT COUNT(*) FROM yunnuo_license.licenses; SELECT COUNT(*) FROM yunnuo_license.agent_money_ledgers; SELECT COUNT(*) FROM yunnuo_license.schema_migrations;"' > /tmp/yunnuo-source-counts-$$
cmp /tmp/yunnuo-restore-counts-$$ /tmp/yunnuo-source-counts-$$
rm /tmp/yunnuo-restore-counts-$$ /tmp/yunnuo-source-counts-$$
docker exec -d -e YN_ADDR=:8081 -e YN_DB_CONFIG_FILE=/tmp/restored-instance/config.json -e YN_RELEASE_DIR=/tmp/restored-instance/releases "$container" sh -c 'cd /opt/yunnuo/backend; exec yunnuo-server > /tmp/restored-instance/server.log 2>&1'
# Check readiness before the independent HTTP acceptance script.
for attempt in $(seq 1 60); do
  if docker exec -e YN_ADDR=:8081 "$container" yunnuo-healthcheck >/dev/null 2>&1; then break; fi
  if [[ "$attempt" == 60 ]]; then docker exec "$container" cat /tmp/restored-instance/server.log; exit 1; fi
  sleep 1
done
YN_TEST_ADMIN_PASSWORD=docker-test-admin-password python3 ./docker/test-product-updates.py verify "http://127.0.0.1:$port" "$state"
# Source and restored instances retain identical cryptographic material and
# records; the independent HTTP probe runs inside the container's network.
echo 'MySQL encrypted full backup restored into a fresh DB and separate application process'
