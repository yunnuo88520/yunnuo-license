#!/usr/bin/env bash
set -Eeuo pipefail

image="${1:-yunnuo-license:test}"
skip_build="${YN_DOCKER_SKIP_BUILD:-0}"
container="yunnuo-license-test-$$"
volume="yunnuo-license-test-$$"
port="${YN_DOCKER_TEST_PORT:-18083}"
admin_password="docker-test-admin-password"
update_state="$(mktemp)"
kit_archive="$(mktemp)"
product_work="$(mktemp -d)"
platform_artifacts="${YN_DOCKER_ARTIFACTS:-$(mktemp -d)}"
mkdir -p "$platform_artifacts"

cleanup() {
  rm -f "$update_state"
  rm -f "$kit_archive"
  rm -r "$product_work"
  if [[ -z "${YN_DOCKER_ARTIFACTS:-}" ]]; then rm -r "$platform_artifacts"; fi
  docker rm -f "$container" >/dev/null 2>&1 || true
  docker volume rm "$volume" >/dev/null 2>&1 || true
}
trap cleanup EXIT
trap 'printf "Docker verification failed at script line %s\n" "$LINENO" >&2' ERR

json_get() {
  local expression="$1"
  python3 -c 'import json,sys; data=json.load(sys.stdin); print(eval(sys.argv[1], {"__builtins__": {}}, {"data": data}))' "$expression"
}

restart_and_wait() {
  docker restart "$container" >/dev/null
  for attempt in $(seq 1 120); do
    [[ "$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{end}}' "$container")" == "healthy" ]] && return
    sleep 1
  done
  echo "restarted container did not become healthy" >&2
  return 1
}

if [[ "$skip_build" != "1" ]]; then
  docker build -t "$image" .
fi
docker run -d --name "$container" \
  -p "127.0.0.1:${port}:8080" -p "127.0.0.1:$((port+1)):8081" \
  -v "$volume:/var/lib/mysql" \
  -e MYSQL_PASSWORD=docker-test-db-password \
  -e MYSQL_ROOT_PASSWORD=docker-test-root-password \
  -e YN_ADMIN_PASSWORD="$admin_password" \
  -e YN_ADMIN_USERNAME=admin \
  -e YN_CLIENT_WRITE_RATE_LIMIT=2000 \
  -e YN_TRUST_PROXY_HEADERS=true \
  -e YN_TRUSTED_PROXY_CIDRS='127.0.0.1/32,172.16.0.0/12' \
  -e YN_SETUP_TOKEN='docker-test-installation-token-32-characters' \
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
[[ "$(curl --silent --output /dev/null --write-out '%{http_code}' "$base/")" == "307" ]]
curl --fail --silent --show-error "$base/install" | grep -q '安装授权平台'
curl --fail --silent --show-error -X POST "$base/v1/setup/initialize" -H 'Content-Type: application/json' -H 'X-Setup-Token: docker-test-installation-token-32-characters' -d '{"database_driver":"mysql","database_host":"127.0.0.1","database_port":3306,"database_name":"yunnuo_license","database_user":"yunnuo","database_password":"docker-test-db-password","admin_username":"admin","admin_password":"docker-test-admin-password"}' | grep -q '"initialized":true'
curl --fail --silent --show-error "$base/v1/site/settings" | grep -q '"site_name":"允诺云授权"'
curl --fail --silent --show-error "$base/" | grep -q '允诺云授权'
curl --fail --silent --show-error "$base/assets/yunnuo-mark.svg" | grep -q '<svg'
curl --fail --silent --show-error "$base/" | grep -q '/assets/public-'
curl --fail --silent --show-error "$base/admin" | grep -q '管理控制台'
curl --fail --silent --show-error "$base/agent-console/" | grep -q '代理工作台'

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
expected_version="$(tr -d '[:space:]' < VERSION)"
curl --fail --silent --show-error "$base/admin/system/version" \
  -H "Authorization: Bearer $admin_token" | grep -q "\"current_version\":\"$expected_version\""
curl --fail --silent --show-error -X POST "$base/admin/site-settings" \
  -H 'Content-Type: application/json' -H "Authorization: Bearer $admin_token" \
  -d '{"site_name":"Docker 授权中心","browser_title":"Docker License","logo_data_url":"","favicon_data_url":""}' | grep -q 'Docker 授权中心'

YN_TEST_ADMIN_PASSWORD="$admin_password" python3 examples/platform-e2e/test_v2_image.py setup --base-url "$base" --state "$product_work/state.json" --artifacts "$platform_artifacts/v2-source"
restart_and_wait
YN_TEST_ADMIN_PASSWORD="$admin_password" python3 examples/platform-e2e/test_v2_image.py verify --base-url "$base" --state "$product_work/state.json" --artifacts "$platform_artifacts/v2-restarted"
YN_TEST_ADMIN_PASSWORD="$admin_password" python3 examples/platform-e2e/test_password.py --base-url "$base" --artifacts "$platform_artifacts/password"
./docker/test-production-restore.sh "$container" "$product_work/state.json" "$((port+1))"
echo "Docker V2 image end-to-end test passed: $image"
