# Phase 1 API Quickstart

启动后端：

```bash
cd backend
YN_ADDR=:18080 \
YN_ADMIN_USERNAME=admin \
YN_ADMIN_PASSWORD='change-this-password' \
go run ./cmd/server
```

首次启动会创建一个 `super_admin`。如果未设置环境变量，开发环境默认账号为
首次启动通过网页初始化向导设置管理员；也可以通过 `YN_ADMIN_USERNAME` 与 `YN_ADMIN_PASSWORD` 在启动时自动创建首个管理员。数据库中已有管理员时，启动配置不会覆盖
已有账号或密码。

管理台：

```text
http://127.0.0.1:18080/admin-console/
```

代理工作台：

```text
http://127.0.0.1:18080/agent-console/
```

无需登录的授权查询：

```text
http://127.0.0.1:18080/
```

健康检查：

```bash
curl http://127.0.0.1:18080/healthz
```

管理员登录：

```bash
curl -X POST http://127.0.0.1:18080/admin/login \
  -H 'Content-Type: application/json' \
  -d '{
    "username": "admin",
    "password": "change-this-password"
  }'
```

后续所有 `/admin/*` 请求（除登录外）都必须携带返回的 Bearer token：

```bash
export ADMIN_TOKEN='admin_access_token_xxx'
```

创建产品：

```bash
curl -X POST http://127.0.0.1:18080/admin/products \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "name": "测试产品",
    "code": "YN",
    "bind_mode": "device",
    "max_bind_count": 1,
    "bind_conflict_strategy": "reject",
    "offline_grace_days": 15,
    "expire_grace_days": 3
  }'
```

生成卡密：

```bash
curl -X POST http://127.0.0.1:18080/admin/card-batches \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "product_id": "prod_xxx",
    "name": "first batch",
    "quantity": 10,
    "duration_days": 30
  }'
```

停用或恢复产品：

```bash
curl -X POST http://127.0.0.1:18080/admin/products/prod_xxx/disable \
  -H "Authorization: Bearer $ADMIN_TOKEN"

curl -X POST http://127.0.0.1:18080/admin/products/prod_xxx/enable \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

停用产品会禁止新发卡、新激活和已有授权验证；恢复后已有有效授权可继续验证。

查看产品公钥环：

```bash
curl http://127.0.0.1:18080/admin/products/prod_xxx/keys \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

轮换产品 RSA 签名密钥（仅 `super_admin`）：

```bash
curl -X POST http://127.0.0.1:18080/admin/products/prod_xxx/keys/rotate \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

轮换后新授权使用新私钥签名，历史公钥不会删除。产品客户端可通过 AppKey 无登录获取
公钥环，并根据 token 中的 `key_version` 选择公钥：

```bash
curl 'http://127.0.0.1:18080/v1/products/keys?app_key=app_xxx'
```

该接口只返回产品编码和公钥，不返回私钥。生产客户端应缓存并随版本发布可信公钥环，
不要把实时拉取公钥作为唯一信任来源。

查看和导出批次卡密：

```bash
curl http://127.0.0.1:18080/admin/card-batches/batch_xxx/cards \
  -H "Authorization: Bearer $ADMIN_TOKEN"

curl -X POST http://127.0.0.1:18080/admin/card-batches/batch_xxx/export \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

作废未使用卡密：

```bash
curl -X POST http://127.0.0.1:18080/admin/cards/void \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "card_id": "card_xxx",
    "reason": "duplicate_generation"
  }'
```

只有 `unused` 卡密可以作废。已激活卡密需要通过授权吊销处理，不能改写成作废状态。
批次明文导出和卡密作废都会写入审计日志。

激活：

```bash
curl -X POST http://127.0.0.1:18080/v1/licenses/activate \
  -H 'Content-Type: application/json' \
  -d '{
    "app_key": "app_xxx",
    "card_code": "YN-XXXX-XXXX-XXXX-XX",
    "bind_mode": "device",
    "bind_value": "machine-A",
    "device_name": "MacBook Pro"
  }'
```

验证：

```bash
curl -X POST http://127.0.0.1:18080/v1/licenses/verify \
  -H 'Content-Type: application/json' \
  -d '{
    "app_key": "app_xxx",
    "license_no": "lic_xxx",
    "bind_mode": "device",
    "bind_value": "machine-A"
  }'
```

公开查询授权号：

```bash
curl -X POST http://127.0.0.1:18080/v1/licenses/query \
  -H 'Content-Type: application/json' \
  -d '{
    "license_no": "lic_xxx"
  }'
```

也可以将请求体改为 `{"card_code":"YN-XXXX-XXXX-XXXX-XX"}` 查询卡密状态。
该接口不需要登录，只返回产品、状态、授权规格及激活/到期时间，不返回绑定值、
设备信息或代理归属。

心跳：

```bash
curl -X POST http://127.0.0.1:18080/v1/licenses/heartbeat \
  -H 'Content-Type: application/json' \
  -d '{
    "app_key": "app_xxx",
    "license_no": "lic_xxx",
    "bind_mode": "device",
    "bind_value": "machine-A"
  }'
```

解绑：

```bash
curl -X POST http://127.0.0.1:18080/v1/licenses/unbind \
  -H 'Content-Type: application/json' \
  -d '{
    "app_key": "app_xxx",
    "license_no": "lic_xxx",
    "bind_mode": "device",
    "bind_value": "machine-A",
    "reason": "change_device"
  }'
```

管理员吊销授权：

```bash
curl -X POST http://127.0.0.1:18080/admin/licenses/revoke \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "license_no": "lic_xxx",
    "reason": "customer_request"
  }'
```

分页查询授权：

```bash
curl 'http://127.0.0.1:18080/admin/licenses?page=1&page_size=20&status=active&product_id=product_xxx&agent_id=agent_xxx&q=lic_' \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

`status` 可选 `active`、`expired`、`revoked`，`q` 按授权号查询。
`page_size` 最大为 100。响应中的 `data` 结构如下：

```json
{
  "items": [],
  "total": 0,
  "page": 1,
  "page_size": 20
}
```

## 完全离线授权

管理员根据客户端生成的机器码签发离线授权：

```bash
curl -X POST http://127.0.0.1:18080/admin/offline-licenses \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "product_id": "prod_xxx",
    "machine_code": "MACHINE-ABCDEF-123456",
    "label": "客户内网服务器",
    "duration_days": 365,
    "is_permanent": false
  }'
```

查询记录并下载 `license.key`：

```bash
curl http://127.0.0.1:18080/admin/offline-licenses \
  -H "Authorization: Bearer $ADMIN_TOKEN"

curl http://127.0.0.1:18080/admin/offline-licenses/offlic_xxx/download \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -o license.key
```

吊销离线授权：

```bash
curl -X POST http://127.0.0.1:18080/admin/offline-licenses/offlic_xxx/revoke \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{"reason":"deployment_retired"}'
```

机器码在数据库中以 HMAC 哈希和 AES-GCM 密文保存，列表仅返回脱敏值。吊销后系统
停止再次下载该文件，但已经分发到完全离线环境的文件无法被远程实时收回。

## 代理发卡

创建代理：

```bash
curl -X POST http://127.0.0.1:18080/admin/agents \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "name": "华东代理",
    "contact_name": "张三"
  }'
```

创建代理登录账号：

```bash
curl -X POST http://127.0.0.1:18080/admin/agents/agent_xxx/users \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "username": "owner",
    "password": "secret123",
    "display_name": "负责人",
    "role": "agent_owner"
  }'
```

代理登录：

```bash
curl -X POST http://127.0.0.1:18080/agent/login \
  -H 'Content-Type: application/json' \
  -d '{
    "login_code": "YN-ABC123",
    "username": "owner",
    "password": "secret123"
  }'
```

`login_code` 是面向代理的短登录代码。长 `agent_no` 仅作为内部标识保留，不要求
代理记忆；代理工作台会在成功登录后记住最近一次使用的短代码。

配置代理可售产品：

```bash
curl -X POST http://127.0.0.1:18080/admin/agent-policies \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "agent_id": "agent_xxx",
    "product_id": "prod_xxx",
    "can_generate": true,
    "can_export_plain_code": true,
    "allowed_duration_days": [30, 90, 365],
    "allow_permanent": false,
    "max_batch_quantity": 100
  }'
```

发放额度：

```bash
curl -X POST http://127.0.0.1:18080/admin/agent-quotas/grant \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "agent_id": "agent_xxx",
    "product_id": "prod_xxx",
    "duration_days": 30,
    "quantity": 100
  }'
```

代理自助生成卡密：

```bash
curl -X POST http://127.0.0.1:18080/agent/card-batches \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer agent_access_token_xxx' \
  -d '{
    "product_id": "prod_xxx",
    "name": "代理批次",
    "quantity": 10,
    "duration_days": 30
  }'
```

查询代理额度：

```bash
curl http://127.0.0.1:18080/agent/quotas \
  -H 'Authorization: Bearer agent_access_token_xxx'
```

查询可售产品、卡密批次和授权记录：

```bash
curl http://127.0.0.1:18080/agent/products \
  -H 'Authorization: Bearer agent_access_token_xxx'

curl http://127.0.0.1:18080/agent/card-batches \
  -H 'Authorization: Bearer agent_access_token_xxx'

curl http://127.0.0.1:18080/agent/licenses \
  -H 'Authorization: Bearer agent_access_token_xxx'
```

查看指定批次的卡密状态：

```bash
curl http://127.0.0.1:18080/agent/card-batches/batch_xxx/cards \
  -H 'Authorization: Bearer agent_access_token_xxx'
```

导出指定批次的明文卡密：

```bash
curl -X POST http://127.0.0.1:18080/agent/card-batches/batch_xxx/export \
  -H 'Authorization: Bearer agent_access_token_xxx'
```

代理卡密和授权查询始终使用登录 token 中的 `agent_id` 过滤。批次 ID 不属于当前
代理时返回 `404`。明文导出还要求产品政策开启 `can_export_plain_code`，并且当前账号
角色是 `agent_owner` 或 `agent_manager`。

## 管理员账号与权限

只有 `super_admin` 可以创建和查看管理员账号：

```bash
curl -X POST http://127.0.0.1:18080/admin/users \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "username": "operator",
    "password": "operator-password",
    "display_name": "运营人员",
    "role": "operator"
  }'
```

角色边界：

- `super_admin`：全部管理能力，并可创建管理员账号。
- `admin`：产品、代理、额度和授权管理。
- `operator`：只读查询并可生成平台卡密。
- `auditor`：只读查询。

修改自己的密码：

```bash
curl -X POST http://127.0.0.1:18080/admin/password \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "current_password": "change-this-password",
    "new_password": "a-new-strong-password"
  }'
```

密码修改成功后，该账号此前签发的管理员 token 会立即失效，需要重新登录。

## 代理状态与审计

暂停、停用和恢复代理：

```bash
curl -X POST http://127.0.0.1:18080/admin/agents/agent_xxx/suspend \
  -H "Authorization: Bearer $ADMIN_TOKEN"

curl -X POST http://127.0.0.1:18080/admin/agents/agent_xxx/disable \
  -H "Authorization: Bearer $ADMIN_TOKEN"

curl -X POST http://127.0.0.1:18080/admin/agents/agent_xxx/enable \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

停用或恢复代理账号：

```bash
curl -X POST http://127.0.0.1:18080/admin/agents/agent_xxx/users/aguser_xxx/disable \
  -H "Authorization: Bearer $ADMIN_TOKEN"

curl -X POST http://127.0.0.1:18080/admin/agents/agent_xxx/users/aguser_xxx/enable \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

代理或账号状态改变后，已有代理 token 会在下次请求时立即被拒绝。

查询审计日志：

```bash
curl 'http://127.0.0.1:18080/admin/audit-logs?actor_type=admin&result=success&action=agent.status&limit=100' \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

`limit` 默认 100，最大 200。`action` 使用包含匹配。

## 基础限流

- 管理员登录：单连接 IP 每 5 分钟 20 次。
- 代理登录：单连接 IP 每 5 分钟 30 次。
- 公开授权查询：单连接 IP 每分钟 60 次。

超过限制时返回 `429 RATE_LIMITED` 和 `Retry-After` 响应头。当前限流保存在单进程
内存中；多实例或反向代理部署时仍需在网关层配置共享限流。

## Go SDK

官方 Go SDK 位于 `sdk/go`，独立模块且只依赖 Go 标准库，提供：

- `Activate`、`Verify`、`Heartbeat`、`Renew`、`Unbind`。
- 服务端业务错误码、HTTP 状态码和 `request_id` 透传。
- 在线缓存 token 的 RSA 验签、产品/授权/绑定/有效期检查。
- 完全离线 `license.key` 的 RSA 验签和机器码检查。
- 基于宿主安全存储接口的本地时间高水位守卫。

本地开发可在产品项目的 `go.mod` 使用：

```go
require github.com/yunnuo88520/yunnuo-license/sdk/go v0.0.0

replace github.com/yunnuo88520/yunnuo-license/sdk/go => ../../sdk/go
```

完整示例见 `sdk/go/README.md`。新签发的在线离线 token 包含 `bind_digest`，SDK
据此验证当前绑定值；旧 token 不含该字段时会要求联网刷新。

## Node.js SDK

官方 Node.js SDK 位于 `sdk/node`，要求 Node.js 18+，使用 ESM 和内置 `fetch`、
`node:crypto`，无第三方运行时依赖。功能与 Go SDK 对齐，并附带 TypeScript 声明：

```bash
npm install ../云授权系统/sdk/node
```

```js
import { LicenseClient, OfflineVerifier } from "@yunnuo/license-sdk";

const client = new LicenseClient({
  baseUrl: "https://license.example.com",
  appKey: "app_xxx",
});

const license = await client.activate({
  card_code: "YN-XXXX-XXXX-XXXX-XX",
  bind_mode: "device",
  bind_value: machineCode,
});
```

完整在线调用、错误处理、离线验签和高水位示例见 `sdk/node/README.md`。

## Java SDK

官方 Java SDK 位于 `sdk/java`，要求 Java 17+。SDK 使用 JDK `HttpClient` 和标准
加密 API，通过 Jackson 解析 JSON，提供与 Go、Node.js SDK 一致的在线授权、离线
验签、错误模型和时间高水位守卫。

本地安装：

```bash
cd sdk/java
mvn clean install
```

产品项目引入：

```xml
<dependency>
  <groupId>com.yunnuo</groupId>
  <artifactId>license-sdk</artifactId>
  <version>0.1.0-SNAPSHOT</version>
</dependency>
```

```java
import com.yunnuo.license.LicenseClient;
import com.yunnuo.license.OfflineVerifier;
import com.yunnuo.license.model.ActivateRequest;
import com.yunnuo.license.model.OfflineExpectation;

LicenseClient client = new LicenseClient(
    "https://license.example.com",
    "app_xxx");

var license = client.activate(new ActivateRequest(
    "YN-XXXX-XXXX-XXXX-XX",
    "device",
    machineCode,
    "Office PC",
    "1.0.0"));

OfflineVerifier verifier = new OfflineVerifier(productPublicKeyPem, "YN");
var claims = verifier.verifyOfflineToken(
    license.offlineToken(),
    new OfflineExpectation(license.licenseNo(), "device", machineCode));
```

完整在线调用、异常处理、完全离线 `license.key` 验签和高水位示例见
`sdk/java/README.md`。产品公钥应随客户端发布，产品私钥不得进入客户端。
