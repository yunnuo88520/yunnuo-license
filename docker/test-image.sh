#!/usr/bin/env bash
set -Eeuo pipefail

image="${1:-yunnuo-license:test}"
skip_build="${YN_DOCKER_SKIP_BUILD:-0}"
container="yunnuo-license-test-$$"
volume="yunnuo-license-test-$$"
port="${YN_DOCKER_TEST_PORT:-18083}"
admin_password="docker-test-admin-password"

cleanup() {
  docker rm -f "$container" >/dev/null 2>&1 || true
  docker volume rm "$volume" >/dev/null 2>&1 || true
}
trap cleanup EXIT

json_get() {
  local expression="$1"
  python3 -c 'import json,sys; data=json.load(sys.stdin); print(eval(sys.argv[1], {"__builtins__": {}}, {"data": data}))' "$expression"
}

if [[ "$skip_build" != "1" ]]; then
  docker build -t "$image" .
fi
docker run -d --name "$container" \
  -p "127.0.0.1:${port}:8080" \
  -v "$volume:/var/lib/mysql" \
  -e MYSQL_PASSWORD=docker-test-db-password \
  -e MYSQL_ROOT_PASSWORD=docker-test-root-password \
  -e YN_ADMIN_PASSWORD="$admin_password" \
  -e YN_CARD_PEPPER=docker-test-card-pepper \
  -e YN_DATA_KEY=docker-test-data-key \
  "$image" >/dev/null

for attempt in $(seq 1 120); do
  health="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$container")"
  if [[ "$health" == "healthy" ]]; then
    break
  fi
  if [[ "$health" == "unhealthy" ]] || [[ "$(docker inspect --format '{{.State.Running}}' "$container")" != "true" ]]; then
    docker logs "$container"
    exit 1
  fi
  if [[ "$attempt" == "120" ]]; then
    docker logs "$container"
    echo "container did not become healthy" >&2
    exit 1
  fi
  sleep 1
done

base="http://127.0.0.1:${port}"
curl --fail --silent --show-error "$base/healthz" | grep -q '"status":"ok"'
curl --fail --silent --show-error "$base/" | grep -q '允诺云授权'
curl --fail --silent --show-error "$base/styles.css?v=5" | grep -q -- '--green'
curl --fail --silent --show-error "$base/assets/yunnuo-mark.svg" | grep -q '<svg'
curl --fail --silent --show-error "$base/assets/lucide.min.js?v=1" | grep -q 'createIcons'
curl --fail --silent --show-error "$base/assets/ui.js?v=1" | grep -q 'setupSignalCanvas'
curl --fail --silent --show-error "$base/admin-console/" | grep -q '管理端安全登录'

published_ports="$(docker port "$container")"
grep -q '^8080/tcp' <<<"$published_ports"
if grep -Eq '^3306/tcp|^33060/tcp' <<<"$published_ports"; then
  echo "MySQL port must not be published to the host" >&2
  exit 1
fi

login_payload="$(printf '{"username":"admin","password":"%s"}' "$admin_password")"
login_response="$(curl --fail --silent --show-error -X POST "$base/admin/login" \
  -H 'Content-Type: application/json' -d "$login_payload")"
admin_token="$(json_get 'data["data"]["access_token"]' <<<"$login_response")"

product_response="$(curl --fail --silent --show-error -X POST "$base/admin/products" \
  -H 'Content-Type: application/json' -H "Authorization: Bearer $admin_token" \
  -d '{"name":"Docker 测试产品","code":"DCK","bind_mode":"device","max_bind_count":1,"bind_conflict_strategy":"reject","offline_grace_days":15,"expire_grace_days":3}')"
product_id="$(json_get 'data["data"]["id"]' <<<"$product_response")"
app_key="$(json_get 'data["data"]["app_key"]' <<<"$product_response")"

batch_response="$(curl --fail --silent --show-error -X POST "$base/admin/card-batches" \
  -H 'Content-Type: application/json' -H "Authorization: Bearer $admin_token" \
  -d "{\"product_id\":\"$product_id\",\"name\":\"Docker E2E\",\"quantity\":1,\"duration_days\":30}")"
card_code="$(json_get 'data["data"]["codes"][0]' <<<"$batch_response")"

activate_response="$(curl --fail --silent --show-error -X POST "$base/v1/licenses/activate" \
  -H 'Content-Type: application/json' \
  -d "{\"app_key\":\"$app_key\",\"card_code\":\"$card_code\",\"bind_mode\":\"device\",\"bind_value\":\"docker-machine\",\"device_name\":\"Docker E2E\"}")"
license_no="$(json_get 'data["data"]["license_no"]' <<<"$activate_response")"

curl --fail --silent --show-error -X POST "$base/v1/licenses/query" \
  -H 'Content-Type: application/json' -d "{\"license_no\":\"$license_no\"}" | grep -q "$license_no"

docker restart "$container" >/dev/null
for attempt in $(seq 1 120); do
  [[ "$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{end}}' "$container")" == "healthy" ]] && break
  sleep 1
done
curl --fail --silent --show-error -X POST "$base/v1/licenses/query" \
  -H 'Content-Type: application/json' -d "{\"license_no\":\"$license_no\"}" | grep -q "$license_no"

echo "Docker image end-to-end test passed: $image"
