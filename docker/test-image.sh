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
platform_artifacts="$(mktemp -d)"

cleanup() {
  rm -f "$update_state"
  rm -f "$kit_archive"
  rm -r "$platform_artifacts"
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

check_public_license() {
  local response masked
  response="$(curl --fail --silent --show-error -X POST "$base/v1/licenses/query" \
    -H 'Content-Type: application/json' -d "{\"license_no\":\"$license_no\"}")"
  masked="$(json_get 'data["data"]["license_no"]' <<<"$response")"
  [[ "$masked" != "$license_no" && "$masked" == *'****'* ]]
  [[ "$(json_get 'data["data"]["product_id"]' <<<"$response")" == "$product_id" ]]
  if grep -Fq "$license_no" <<<"$response"; then
    echo "Public license query leaked the full license number" >&2
    return 1
  fi
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
  -e YN_ADMIN_USERNAME=admin \
  -e YN_TRUST_PROXY_HEADERS=true \
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
curl --fail --silent --show-error -X POST "$base/v1/setup/initialize" -H 'Content-Type: application/json' -d '{"database_driver":"mysql","database_host":"127.0.0.1","database_port":3306,"database_name":"yunnuo_license","database_user":"yunnuo","database_password":"docker-test-db-password","admin_username":"admin","admin_password":"docker-test-admin-password"}' | grep -q '"initialized":true'
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

product_response="$(curl --fail --silent --show-error -X POST "$base/admin/products" \
  -H 'Content-Type: application/json' -H "Authorization: Bearer $admin_token" \
  -d '{"name":"Docker 测试产品","bind_mode":"device","max_bind_count":1,"bind_conflict_strategy":"reject","offline_grace_days":15,"expire_grace_days":3}')"
product_id="$(json_get 'data["data"]["id"]' <<<"$product_response")"
app_key="$(json_get 'data["data"]["app_key"]' <<<"$product_response")"
[[ "$app_key" == "$product_id" ]]
[[ "$product_id" =~ ^yn[0-9a-f]{16}$ ]]
app_secret="$(json_get 'data["data"]["app_secret"]' <<<"$product_response")"
[[ "$app_secret" =~ ^[0-9a-f]{32}$ ]]
application_token_payload="$(printf '{"grant_type":"client_credential","appid":"%s","secret":"%s"}' "$product_id" "$app_secret")"
application_token_response="$(curl --fail --silent --show-error -X POST "$base/v1/app/token" -H 'Content-Type: application/json' -d "$application_token_payload")"
application_token="$(json_get 'data["data"]["access_token"]' <<<"$application_token_response")"
curl --fail --silent --show-error "$base/v1/app/profile" -H "Authorization: Bearer $application_token" | grep -Fq "$product_id"
reset_secret_response="$(curl --fail --silent --show-error -X POST "$base/admin/products/$product_id/credentials/reset" -H 'Content-Type: application/json' -H "Authorization: Bearer $admin_token" -d '{"expected_version":1,"reason":"Docker credential rotation acceptance"}')"
[[ "$(curl --silent --output /dev/null --write-out '%{http_code}' "$base/v1/app/profile" -H "Authorization: Bearer $application_token")" == "401" ]]
[[ "$(curl --silent --output /dev/null --write-out '%{http_code}' -X POST "$base/v1/app/token" -H 'Content-Type: application/json' -d "$application_token_payload")" == "401" ]]
app_secret="$(json_get 'data["data"]["app_secret"]' <<<"$reset_secret_response")"
application_token_payload="$(printf '{"grant_type":"client_credential","appid":"%s","secret":"%s"}' "$product_id" "$app_secret")"
application_token_response="$(curl --fail --silent --show-error -X POST "$base/v1/app/token" -H 'Content-Type: application/json' -d "$application_token_payload")"
application_token="$(json_get 'data["data"]["access_token"]' <<<"$application_token_response")"

for language in node go java; do
  curl --fail --silent --show-error --get "$base/admin/products/$product_id/integration-kit" \
    -H "Authorization: Bearer $admin_token" \
    --data-urlencode "base_url=$base" --data-urlencode "language=$language" \
    --data-urlencode 'bind_value=docker-integration-instance' -o "$kit_archive"
  python3 - "$kit_archive" "$language" "$product_id" <<'PY'
import json,sys,zipfile
with zipfile.ZipFile(sys.argv[1]) as archive:
    expected={'node':'sdk/src/index.js','go':'sdk/client.go','java':'src/main/java/com/yunnuo/license/LicenseClient.java'}[sys.argv[2]]
    assert expected in archive.namelist() and 'LICENSE' in archive.namelist()
    config=json.loads(archive.read('config.json'))
    assert config['bind_mode']=='device' and config['bind_value']=='docker-integration-instance'
    assert config['product_id']==sys.argv[3] and config['app_key']==sys.argv[3] and config['public_keys']
PY
done

risk_block_response="$(curl --fail --silent --show-error -X POST "$base/admin/risk/blocks" \
  -H 'Content-Type: application/json' -H "Authorization: Bearer $admin_token" \
  -d "{\"product_id\":\"$product_id\",\"kind\":\"ip\",\"value\":\"203.0.113.77\",\"reason\":\"Docker E2E risk check\"}")"
grep -Fq '203.0.113.*' <<<"$risk_block_response"
curl --fail --silent --show-error "$base/admin/risk/summary" \
  -H "Authorization: Bearer $admin_token" | grep -q '"active_blocks":1'

batch_response="$(curl --fail --silent --show-error -X POST "$base/admin/card-batches" \
  -H 'Content-Type: application/json' -H "Authorization: Bearer $admin_token" \
  -d "{\"product_id\":\"$product_id\",\"name\":\"Docker E2E\",\"quantity\":1,\"duration_days\":30}")"
card_code="$(json_get 'data["data"]["codes"][0]' <<<"$batch_response")"

blocked_response="$(curl --silent --show-error -X POST "$base/v1/licenses/activate" \
  -H 'Content-Type: application/json' -H 'X-Forwarded-For: 203.0.113.77' \
  -d "{\"app_key\":\"$app_key\",\"card_code\":\"$card_code\",\"bind_mode\":\"device\",\"bind_value\":\"docker-machine\"}" \
  -w $'\n%{http_code}')"
blocked_status="${blocked_response##*$'\n'}"
blocked_body="${blocked_response%$'\n'*}"
[[ "$blocked_status" == "403" ]]
grep -q 'RISK_IP_BLOCKED' <<<"$blocked_body"

activate_response="$(curl --fail --silent --show-error -X POST "$base/v1/licenses/activate" \
  -H 'Content-Type: application/json' \
  -d "{\"app_key\":\"$app_key\",\"card_code\":\"$card_code\",\"bind_mode\":\"device\",\"bind_value\":\"docker-machine\",\"device_name\":\"Docker E2E\"}")"
license_no="$(json_get 'data["data"]["license_no"]' <<<"$activate_response")"

check_public_license

account_product_response="$(curl --fail --silent --show-error -X POST "$base/admin/products" \
  -H 'Content-Type: application/json' -H "Authorization: Bearer $admin_token" \
  -d '{"name":"账号查询测试产品","bind_mode":"account","max_bind_count":1,"bind_conflict_strategy":"reject"}')"
account_product_id="$(json_get 'data["data"]["id"]' <<<"$account_product_response")"
account_app_key="$(json_get 'data["data"]["app_key"]' <<<"$account_product_response")"
account_batch_response="$(curl --fail --silent --show-error -X POST "$base/admin/card-batches" \
  -H 'Content-Type: application/json' -H "Authorization: Bearer $admin_token" \
  -d "{\"product_id\":\"$account_product_id\",\"name\":\"Account Lookup E2E\",\"quantity\":1,\"duration_days\":30}")"
account_card_code="$(json_get 'data["data"]["codes"][0]' <<<"$account_batch_response")"
curl --fail --silent --show-error -X POST "$base/v1/licenses/activate" \
  -H 'Content-Type: application/json' \
  -d "{\"app_key\":\"$account_app_key\",\"card_code\":\"$account_card_code\",\"bind_mode\":\"account\",\"bind_value\":\"13812348000\"}" >/dev/null
account_query_response="$(curl --fail --silent --show-error -X POST "$base/v1/licenses/query" \
  -H 'Content-Type: application/json' -d '{"query":"13812348000","query_type":"account"}')"
grep -Fq '138****8000' <<<"$account_query_response"
if grep -Fq '13812348000' <<<"$account_query_response"; then
  echo "public account query leaked the raw binding value" >&2
  exit 1
fi

YN_TEST_ADMIN_PASSWORD="$admin_password" python3 ./docker/test-product-updates.py prepare "$base" "$update_state"
YN_TEST_ADMIN_PASSWORD="$admin_password" python3 ./examples/platform-e2e/test_stage1.py --base-url "$base" --artifacts "$platform_artifacts/stage1"
YN_TEST_ADMIN_PASSWORD="$admin_password" python3 ./examples/platform-e2e/test_stage2.py --base-url "$base" --artifacts "$platform_artifacts/stage2"
YN_TEST_ADMIN_PASSWORD="$admin_password" python3 ./examples/platform-e2e/test_stage3.py --base-url "$base" --artifacts "$platform_artifacts/stage3"
YN_TEST_ADMIN_PASSWORD="$admin_password" python3 ./examples/platform-e2e/test_stage4.py --base-url "$base" --artifacts "$platform_artifacts/stage4"

YN_TEST_ADMIN_PASSWORD="$admin_password" python3 ./examples/platform-e2e/test_recovery_history.py --base-url "$base" --artifacts "$platform_artifacts/recovery-history"
# Isolate the additional login-heavy suite's rate-limit window while retaining MySQL data.
restart_and_wait
YN_TEST_ADMIN_PASSWORD="$admin_password" python3 ./examples/platform-e2e/test_agent_accounts.py --base-url "$base" --artifacts "$platform_artifacts/agent-accounts"

restart_and_wait
check_public_license

python3 ./docker/test-product-updates.py verify "$base" "$update_state"
YN_TEST_ADMIN_PASSWORD="$admin_password" python3 ./examples/platform-e2e/test_agent_accounts.py --base-url "$base" --artifacts "$platform_artifacts/agent-accounts" --verify-state "$platform_artifacts/agent-accounts/report.json"
curl --fail --silent --show-error "$base/v1/app/profile" -H "Authorization: Bearer $application_token" | grep -Fq "$product_id"
curl --fail --silent --show-error -X POST "$base/v1/app/token" -H 'Content-Type: application/json' -d "$application_token_payload" | grep -q '"expires_in":7200'
YN_TEST_ADMIN_PASSWORD="$admin_password" python3 ./examples/platform-e2e/test_stage2.py --base-url "$base" --artifacts "$platform_artifacts/stage2" --verify-state "$platform_artifacts/stage2/report.json"
YN_TEST_ADMIN_PASSWORD="$admin_password" python3 ./examples/platform-e2e/test_stage4.py --base-url "$base" --artifacts "$platform_artifacts/stage4" --verify-state "$platform_artifacts/stage4/report.json"
echo "Docker image end-to-end test passed: $image"
