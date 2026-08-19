# 允诺云授权系统设计 V1

## 1. 设计目标

允诺云授权系统定位为公司内部统一授权中台，为 Web SaaS、桌面软件、移动端 App、IDE 插件、付费插件等产品提供统一的卡密、激活、验证、续费、解绑、离线授权和审计能力。

V1 的目标是先做成一套可靠、可接入、可运营的授权底座，而不是一次性覆盖所有高级商业场景。

### 1.1 V1 范围

V1 必须支持：

- 产品管理：产品、密钥、授权策略、绑定策略配置。
- 卡密管理：批量生成、查询、导出、作废。
- 在线激活：卡密激活为授权，绑定设备或账号。
- 在线验证：客户端周期性验证授权状态。
- 心跳上报：维护设备活跃时间，支持席位判断。
- 续费：使用新卡密延长既有授权。
- 解绑：释放绑定席位，支持次数和冷却限制。
- 离线授权：在线缓存离线凭证、管理员生成纯离线文件。
- 代理管理：平台代理、代理员工、代理额度、代理发卡和业绩统计。
- 后台管理：基础 CRUD、审计日志、错误码可查。
- SDK：Go、Java、Node.js 官方 SDK，其他语言提供签名 Demo。

V1 暂缓完整实现：

- 完全转让流程。
- 自动邮件分发。
- 多租户隔离。
- 自动化风险评分和跨节点实时风控。
- 复杂报表和 BI。

系统明确不包含支付、订单结算、折扣、授信、退款和佣金打款能力。代理额度仅表示可生成卡密的数量，不代表资金余额。

## 2. 总体架构

### 2.1 系统组件

```text
Client SDK / Product App
        |
        | HTTPS
        v
API Gateway / HTTP Server
        |
        +-- Auth Middleware
        +-- Rate Limit Middleware
        +-- Audit Middleware
        |
        v
Application Services
        |
        +-- Product Service
        +-- Card Service
        +-- License Service
        +-- Device Service
        +-- Renewal Service
        +-- Offline License Service
        +-- Agent Service
        +-- Admin Service
        |
        v
Domain Repositories
        |
        +-- MySQL / PostgreSQL / SQLite
        +-- Redis / Local Cache
        |
        v
Crypto / Key Management
```

### 2.2 推荐技术栈

- 后端：Go。
- HTTP 框架：Gin、Chi 或 Echo，优先选择团队熟悉的。
- ORM/SQL：推荐 SQLC 或 Ent。若强依赖三库兼容，SQLC + 手写迁移更可控。
- 数据库：
  - 开发默认 SQLite。
  - 生产推荐 PostgreSQL 或 MySQL。
- 缓存：
  - 单机部署可用本地缓存。
  - 多实例部署推荐 Redis。
- 前端：React + TypeScript。
- 后台组件库：Ant Design 或 Shadcn/UI 二选一。
- 迁移工具：Goose、Atlas 或 golang-migrate。

### 2.3 分层原则

- Handler 只负责协议层解析、鉴权、响应格式。
- Service 负责业务流程、事务边界、状态迁移。
- Repository 负责数据库读写。
- Crypto 包集中处理卡密生成、HMAC、RSA/Ed25519 签名、AES-GCM。
- SDK 不复刻业务判断，只封装请求、验签、离线凭证验证和错误码。

## 3. 核心业务语义

### 3.1 卡密与授权的关系

V1 采用如下语义：

- 一张卡密只能激活出一个主授权 `license`。
- 一个主授权可以按产品策略绑定 0 到 N 个主体。
- 绑定主体可以是设备指纹、域名、IP、账号或无绑定。
- 同一张卡密重复激活：
  - 如果是同一绑定主体，返回已有授权，保持幂等。
  - 如果是新绑定主体，进入席位判断流程。
  - 如果产品配置不允许新增主体，返回设备超限或绑定不匹配。

这能同时满足“同一卡密只激活一次”和“多设备/浮动授权”。

### 3.2 绑定方式

绑定方式使用单一主类型，不建议同一个产品同时启用多种绑定方式后由客户端自由选择。V1 推荐每个产品配置一个 `bind_mode`：

- `none`：无绑定。
- `device`：设备指纹。
- `domain`：域名。
- `ip`：IP。
- `account`：业务账号。

如果确实需要混合绑定，使用 `bind_policy` JSON 扩展，但 V1 后端流程仍以一个主绑定值做席位计算。

### 3.3 授权类型

授权时长类型：

- `trial`：试用。
- `days`：指定天数。
- `month`：月卡。
- `quarter`：季卡。
- `year`：年卡。
- `permanent`：永久。

数据库统一存 `duration_days` 和 `is_permanent`。月份、季度、年份在生成卡密时换算为具体到期时间策略。若需要严格自然月，后续增加 `duration_unit` 和 `duration_value`。

### 3.4 代理业务语义

V1 将代理定义为公司外部或内部渠道账号。代理可以在被授权的产品范围内领取或生成卡密，并只能查看自己名下产生的卡密、授权、客户和统计数据。

代理模型：

- 平台管理员拥有全局数据权限。
- 代理账号只拥有自己的数据域。
- 支持一级代理和二级代理树，但 V1 建议默认只开放一级代理。
- 代理可以有多个员工账号。
- 代理可按产品配置可售授权类型、可生成数量和明文导出权限。
- 代理发出的卡密必须保留归属关系，后续激活、续费、作废和审计都能追溯到代理。

V1 代理功能推荐支持两种发卡模式：

- `allocated`：平台管理员生成卡密并分配给代理，代理只负责查看、导出和分发。
- `self_generate`：代理在额度内自助生成卡密。

额度推荐按“产品 + 授权时长”维度管理，而不是只给代理一个总额度。这样可以避免代理拿年卡额度去生成永久卡。

### 3.5 代理数据隔离

所有代理侧查询必须自动附加数据范围：

```text
agent_id in current_agent_scope
```

平台代理用户不能通过 API 参数传入其他代理 ID 来越权查询。服务端必须从登录态或 API token 中解析代理身份。

代理数据范围包括：

- 代理自己创建或被分配的卡密批次。
- 代理自己名下卡密。
- 代理卡密激活出来的授权。
- 代理名下客户。
- 代理相关审计日志。
- 下级代理数据，仅当启用代理树且当前代理有 `view_child_data` 权限。

## 4. 状态机设计

### 4.1 cards.status

```text
unused -> activated
unused -> voided
activated -> consumed_for_renewal
activated -> expired
```

状态说明：

- `unused`：未使用。
- `activated`：已用于首次激活。
- `consumed_for_renewal`：已用于续费。
- `voided`：已作废，不可使用。
- `expired`：由系统任务或查询时懒更新标记，永久卡不进入该状态。

约束：

- 只有 `unused` 卡密可作废。
- 只有 `unused` 卡密可用于首次激活或续费。
- 用于续费的卡密不再生成新授权。

### 4.2 licenses.status

```text
active -> expired
active -> revoked
expired -> active
active -> transferred
```

状态说明：

- `active`：有效。
- `expired`：已过期，宽限期外不可用。
- `revoked`：被管理员吊销。
- `transferred`：已转让，V1 可预留。

注意：

- 宽限期不是独立状态，通过 `expired_at + grace_days` 计算。
- 续费可以把 `expired` 授权恢复为 `active`。

### 4.3 license_bindings.status

```text
active -> unbound
active -> kicked
active -> revoked
```

状态说明：

- `active`：当前绑定有效。
- `unbound`：用户或管理员解绑。
- `kicked`：浮动授权策略踢出。
- `revoked`：授权吊销后失效。

V1 不建议物理删除绑定记录。解绑时改状态，便于审计、冷却限制和风控统计。

### 4.4 agents.status

```text
active -> suspended
active -> disabled
suspended -> active
```

状态说明：

- `active`：正常代理，可登录和发卡。
- `suspended`：临时冻结，不能生成或导出卡密，但历史数据可查看。
- `disabled`：停用，不能登录代理后台。

代理余额或额度不足不是代理状态，而是发卡操作的校验结果。

## 5. 数据库设计

以下字段为逻辑设计，具体 SQL 类型按数据库方言迁移文件实现。

### 5.1 products

产品表。

```text
id
name
code
app_key
app_secret_hash
api_secret_encrypted
public_key_pem
private_key_encrypted
signing_alg
key_version
bind_mode
max_bind_count
bind_conflict_strategy
offline_mode
offline_grace_days
expire_grace_days
unbind_limit
unbind_cooldown_hours
status
created_by
created_at
updated_at
```

关键约束：

- `code` 唯一。
- `app_key` 唯一。
- `status` 枚举：`active`、`disabled`。

字段说明：

- `app_secret_hash`：用于后台校验和展示状态，不保存明文。
- `api_secret_encrypted`：如需服务端间调用，可加密保存。
- `private_key_encrypted`：产品私钥必须加密保存。
- `signing_alg`：建议 V1 使用 `RS256`，后续可扩展 `EdDSA`。
- `offline_mode`：`disabled`、`online_cache`、`full_offline`、`both`。

### 5.2 agents

代理表。

```text
id
agent_no
parent_agent_id
name
contact_name
phone
email
level
status
remark
created_by
created_at
updated_at
disabled_at
```

关键约束：

- `agent_no` 唯一。
- `parent_agent_id` 建索引。
- `status` 枚举：`active`、`suspended`、`disabled`。

字段说明：

- `parent_agent_id` 用于代理树。V1 可只允许为空，保留二级代理扩展。
- 系统不承担支付、价格、折扣、授信、订单结算或退款业务。
- 数据库中的早期结算相关列仅作为兼容占位，业务层和 API 不再使用或返回。

### 5.2.1 agent_login_codes

代理登录短代码表。

```text
agent_id
login_code
created_at
```

关键约束：

- `agent_id` 唯一并关联 `agents.id`。
- `login_code` 全局唯一，采用 `YN-ABC123` 形式。
- `agent_no` 继续作为内部稳定标识，不要求代理用户记忆或输入。
- 已有代理在系统启动时自动补齐短代码，代理工作台记住最近一次成功登录的代码。

### 5.3 agent_users

代理员工账号表。

```text
id
agent_id
username
password_hash
display_name
phone
email
role
status
last_login_at
created_at
updated_at
```

关键约束：

- `agent_id, username` 唯一。
- `agent_id, status` 建索引。

角色：

- `owner`：代理主账号。
- `manager`：可生成和导出卡密，可查看统计。
- `staff`：可查看和分发卡密，不能调整配置。
- `readonly`：只读。

### 5.4 agent_product_policies

代理产品授权政策表。

```text
id
agent_id
product_id
can_generate
can_export_plain_code
allowed_duration_days
allow_permanent
max_batch_quantity
status
created_at
updated_at
```

关键约束：

- `agent_id, product_id` 唯一。
- `product_id, status` 建索引。

说明：

- `allowed_duration_days` 可用 JSON 数组保存，例如 `[30, 90, 365]`。
- 永久卡必须通过 `allow_permanent` 单独控制。

### 5.5 agent_quota_ledgers

代理额度流水表。

```text
id
agent_id
product_id
duration_days
is_permanent
change_type
change_quantity
balance_after
related_batch_id
related_card_id
operator_type
operator_id
remark
created_at
```

关键约束：

- `agent_id, product_id, duration_days, is_permanent` 建索引。
- `related_batch_id` 建索引。

`change_type` 枚举：

- `grant`：平台发放额度。
- `revoke`：平台收回额度。
- `generate_cards`：代理自助生成卡密消耗额度。
- `void_unused_cards`：作废未使用卡密返还额度。
- `manual_adjust`：人工调整。

读取代理可用额度时，可以按流水聚合，也可以维护冗余汇总表 `agent_quotas`。V1 若数据量不大，先用流水聚合；生产数据较大时增加汇总表。

### 5.6 cards

卡密表。

```text
id
product_id
batch_id
agent_id
code_hash
code_encrypted
code_prefix
duration_days
is_permanent
status
activated_license_id
activated_at
consumed_at
voided_at
void_reason
created_by
created_at
updated_at
```

关键约束：

- `code_hash` 全局唯一。
- `product_id, batch_id` 建索引。
- `agent_id, status` 建索引。
- `status, product_id` 建索引。

重要设计：

- 查询卡密时使用 `code_hash = HMAC-SHA256(server_pepper, normalized_code)`。
- `code_encrypted` 只用于后台明文查看和导出。
- AES-GCM 每条记录使用随机 nonce。
- 卡密明文只在生成时返回一次；若必须多次查看，需要后台高权限 + 审计。

### 5.7 card_batches

卡密批次表。

```text
id
product_id
agent_id
name
quantity
duration_days
is_permanent
source
status
export_count
created_by
created_at
updated_at
```

用途：

- 支持批量查询。
- 支持批量导出。
- 支持批量作废未使用卡密。

字段说明：

- `agent_id` 为空表示平台自有批次。
- `source` 枚举：`platform_generated`、`agent_self_generated`、`allocated_to_agent`。

### 5.8 licenses

授权主表。

```text
id
license_no
product_id
card_id
agent_id
customer_id
account_ref
status
issued_at
activated_at
expired_at
last_verify_at
last_heartbeat_at
revoked_at
revoked_reason
offline_token_version
created_at
updated_at
```

关键约束：

- `license_no` 唯一。
- `card_id` 唯一。
- `product_id, status` 建索引。
- `agent_id, status` 建索引。
- `expired_at` 建索引。

说明：

- `license_no` 是对外展示和 SDK 使用的授权号。
- `customer_id` 可为空，V1 可先不做客户中心。
- `agent_id` 从来源卡密继承，不允许激活时由客户端指定。
- 永久授权的 `expired_at` 为空。

### 5.9 license_bindings

授权绑定表，替代原来的 `license_devices`。

```text
id
license_id
product_id
bind_mode
bind_value_hash
bind_value_encrypted
display_name
status
first_seen_ip
last_seen_ip
user_agent
last_heartbeat_at
activated_at
unbound_at
kicked_at
revoked_at
created_at
updated_at
```

关键约束：

- `license_id, bind_mode, bind_value_hash` 唯一。
- `license_id, status` 建索引。
- `bind_value_hash` 建索引。
- `last_heartbeat_at` 建索引。

说明：

- `bind_value_hash` 用于查询和唯一约束。
- `bind_value_encrypted` 用于后台查看。
- 无绑定模式下可不写绑定记录，或写入固定值 `none`。建议写入固定值，方便统一流程。

### 5.10 renewals

续费记录表。

```text
id
license_id
old_expired_at
new_expired_at
card_id
duration_days
created_by
created_at
```

### 5.11 unbind_records

解绑记录表。

```text
id
license_id
binding_id
bind_mode
bind_value_hash
reason
operator_type
operator_id
created_at
```

用途：

- 计算解绑次数。
- 计算解绑冷却期。
- 追踪异常解绑行为。

### 5.12 offline_licenses

离线授权文件记录。

```text
id
product_id
license_id
bind_mode
bind_value_hash
offline_license_no
payload_hash
key_version
issued_at
expired_at
max_offline_until
status
created_by
created_at
revoked_at
```

说明：

- 纯离线授权无法实时吊销，`revoked_at` 只能影响后续在线检查或再次生成。
- 后台应明确提示“已发出的纯离线文件无法被远程强制收回”。

### 5.13 audit_logs

审计日志表。

```text
id
actor_type
actor_id
agent_id
product_id
license_id
card_id
binding_id
action
client_ip
user_agent
request_id
result
error_code
extra_json
created_at
```

所有关键操作必须写审计：

- 登录后台。
- 登录代理后台。
- 创建产品。
- 创建、停用、冻结代理。
- 调整代理产品政策。
- 发放或扣减代理额度。
- 生成卡密。
- 导出卡密明文。
- 激活。
- 验证失败。
- 续费。
- 解绑。
- 踢出设备。
- 吊销授权。
- 生成离线文件。

### 5.14 api_nonces

防重放 nonce 表或缓存。

```text
id
app_key
nonce
request_hash
expires_at
created_at
```

生产推荐 Redis，数据库表作为可选 fallback。

## 6. 卡密设计

### 6.1 格式

推荐格式：

```text
{PRODUCT_PREFIX}-{BLOCK1}-{BLOCK2}-{BLOCK3}-{CHECK}
```

示例：

```text
YN-A7KF-9QXP-M3TW-R6
```

字符集：

```text
ABCDEFGHJKLMNPQRSTUVWXYZ23456789
```

剔除：

- O / 0
- I / 1
- 容易混淆的小写字符

### 6.2 生成规则

- 使用 CSPRNG 生成随机主体。
- Checksum 只用于本地快速拦截低级错误，不作为安全机制。
- 服务端以 `code_hash` 判重。
- 批量生成时若冲突，重试生成。

### 6.3 存储规则

```text
normalized_code = uppercase(remove_dash(input_code))
code_hash = HMAC-SHA256(card_pepper, normalized_code)
code_encrypted = AES-256-GCM(data_key, normalized_code)
```

注意：

- `card_pepper` 不入库，来自环境变量或密钥管理系统。
- AES-GCM nonce 每条随机。
- 后台导出明文必须写审计日志。

## 7. API 设计

### 7.1 通用响应格式

成功：

```json
{
  "success": true,
  "request_id": "req_...",
  "data": {}
}
```

失败：

```json
{
  "success": false,
  "request_id": "req_...",
  "error": {
    "code": "LICENSE_EXPIRED",
    "message": "license expired",
    "details": {}
  }
}
```

### 7.2 客户端 API

#### POST /v1/licenses/activate

用途：卡密首次激活或绑定新主体。

请求：

```json
{
  "app_key": "app_...",
  "card_code": "YN-A7KF-9QXP-M3TW-R6",
  "bind_mode": "device",
  "bind_value": "machine_fingerprint",
  "device_name": "MacBook Pro",
  "client_version": "1.0.0"
}
```

响应：

```json
{
  "license_no": "lic_...",
  "status": "active",
  "expired_at": "2027-07-18T00:00:00Z",
  "grace_until": "2027-07-21T00:00:00Z",
  "license_token": "jwt_or_signed_blob",
  "offline_token": "signed_offline_blob",
  "server_time": "2026-07-18T00:00:00Z"
}
```

处理逻辑：

1. 标准化卡密。
2. 校验 checksum。
3. 计算 `code_hash` 查询卡密。
4. 检查产品状态、卡密状态。
5. 在事务中锁定卡密和授权。
6. 若卡密未激活，创建 license。
7. 根据绑定策略写入或复用 binding。
8. 签发在线 token 和离线 token。
9. 写审计日志。

#### POST /v1/licenses/verify

用途：在线验证授权。

请求：

```json
{
  "app_key": "app_...",
  "license_no": "lic_...",
  "bind_mode": "device",
  "bind_value": "machine_fingerprint",
  "license_token": "jwt_or_signed_blob"
}
```

响应：

```json
{
  "valid": true,
  "status": "active",
  "expired_at": "2027-07-18T00:00:00Z",
  "grace_until": "2027-07-21T00:00:00Z",
  "offline_until": "2026-08-02T00:00:00Z",
  "server_time": "2026-07-18T00:00:00Z"
}
```

处理逻辑：

1. 校验产品。
2. 校验授权存在且未吊销。
3. 校验绑定主体存在且有效。
4. 判断过期和宽限期。
5. 更新 `last_verify_at`。
6. 必要时返回新的离线 token。

#### POST /v1/licenses/heartbeat

用途：上报活跃时间。

请求：

```json
{
  "app_key": "app_...",
  "license_no": "lic_...",
  "bind_mode": "device",
  "bind_value": "machine_fingerprint",
  "runtime": {
    "client_version": "1.0.0",
    "os": "macOS"
  }
}
```

响应：

```json
{
  "accepted": true,
  "server_time": "2026-07-18T00:00:00Z"
}
```

#### POST /v1/licenses/renew

用途：使用新卡密续费已有授权。

请求：

```json
{
  "app_key": "app_...",
  "license_no": "lic_...",
  "renew_card_code": "YN-....",
  "bind_mode": "device",
  "bind_value": "machine_fingerprint"
}
```

续费公式：

```text
new_expired_at = max(old_expired_at, now) + duration
```

永久授权规则：

- 永久授权不可续费，返回 `LICENSE_PERMANENT`。
- 永久卡续费到非永久授权时，将授权升级为永久。

#### POST /v1/licenses/unbind

用途：自助解绑当前主体。

请求：

```json
{
  "app_key": "app_...",
  "license_no": "lic_...",
  "bind_mode": "device",
  "bind_value": "machine_fingerprint",
  "reason": "change_device"
}
```

处理逻辑：

1. 校验授权和绑定。
2. 检查解绑冷却期。
3. 检查解绑次数上限。
4. 将 binding 状态置为 `unbound`。
5. 写 `unbind_records`。

### 7.3 管理后台 API

代理：

- `GET /admin/agents`
- `POST /admin/agents`
- `GET /admin/agents/{id}`
- `PATCH /admin/agents/{id}`
- `POST /admin/agents/{id}/suspend`
- `POST /admin/agents/{id}/disable`
- `POST /admin/agents/{id}/users`
- `PATCH /admin/agents/{id}/product-policies`
- `POST /admin/agents/{id}/quota/grant`
- `POST /admin/agents/{id}/quota/revoke`
- `GET /admin/agents/{id}/quota-ledgers`

产品：

- `GET /admin/products`
- `POST /admin/products`
- `GET /admin/products/{id}`
- `PATCH /admin/products/{id}`
- `POST /admin/products/{id}/disable`
- `POST /admin/products/{id}/rotate-keys`

卡密：

- `POST /admin/card-batches`
- `GET /admin/card-batches`
- `GET /admin/cards`
- `POST /admin/cards/export`
- `POST /admin/cards/void`

授权：

- `GET /admin/licenses`：支持 `status`、`product_id`、`agent_id`、`q`、`page`、`page_size`；返回分页对象。
- `GET /admin/licenses/{id}`
- `POST /admin/licenses/{id}/revoke`
- `POST /admin/licenses/{id}/unbind`
- `GET /admin/licenses/{id}/bindings`

离线授权：

- `POST /admin/offline-licenses`
- `GET /admin/offline-licenses`
- `GET /admin/offline-licenses/{id}/download`
- `POST /admin/offline-licenses/{id}/revoke`

审计：

- `GET /admin/audit-logs`

### 7.4 代理后台 API

代理账号：

- `POST /agent/login`
- `POST /agent/logout`
- `GET /agent/profile`
- `PATCH /agent/profile`
- `GET /agent/users`
- `POST /agent/users`
- `PATCH /agent/users/{id}`

代理产品和额度：

- `GET /agent/products`
- `GET /agent/quotas`
- `GET /agent/quota-ledgers`

代理卡密：

- `POST /agent/card-batches`
- `GET /agent/card-batches`
- `GET /agent/card-batches/{id}`
- `GET /agent/cards`
- `POST /agent/cards/export`
- `POST /agent/cards/void`

代理授权：

- `GET /agent/licenses`
- `GET /agent/licenses/{id}`
- `GET /agent/licenses/{id}/bindings`

代理统计：

- `GET /agent/dashboard`
- `GET /agent/reports/activations`
- `GET /agent/reports/card-sales`

代理生成卡密处理逻辑：

1. 从登录态获取 `agent_id`。
2. 校验代理状态为 `active`。
3. 校验产品在 `agent_product_policies` 中启用。
4. 校验授权时长、永久卡权限、批次数量上限。
5. 若代理为预付额度模式，检查并扣减 `agent_quota_ledgers`。
6. 生成 `card_batches` 和 `cards`，写入 `agent_id`。
7. 写审计日志。

代理导出卡密要求：

- 只能导出自己名下卡密。
- 必须检查 `can_export_plain_code`。
- 导出明文必须写审计日志。
- 已激活卡密默认不展示明文，除非平台配置允许。

## 8. 客户端鉴权与签名设计

### 8.1 两类调用方

V1 明确区分两类调用方：

1. 公开客户端：桌面软件、移动 App、插件。
2. 可信服务端：公司自有后端、订单系统、运营系统。

公开客户端不能内置高权限 `AppSecret`。它可以携带：

- `app_key`
- 卡密
- license token
- 设备指纹
- timestamp
- nonce

可信服务端可以使用 HMAC 签名：

- `app_key`
- `timestamp`
- `nonce`
- `body_hash`
- `signature`

### 8.2 可信服务端签名串

```text
METHOD
PATH
QUERY_STRING
TIMESTAMP
NONCE
SHA256(BODY)
```

签名：

```text
signature = HMAC-SHA256(api_secret, canonical_string)
```

服务端校验：

- timestamp 与服务器时间偏差不能超过 5 分钟。
- nonce 在有效期内不能重复。
- body hash 必须一致。
- app_key 必须启用。

### 8.3 公开客户端防滥用

公开客户端不追求“隐藏密钥式安全”，重点依靠：

- HTTPS。
- 卡密随机强度。
- license token 签名。
- 设备绑定。
- 服务端风控和频率限制。
- SDK 本地离线验签。
- 产品自身混淆和反调试。

## 9. 授权 Token 设计

### 9.1 在线 license_token

格式可使用 JWT 或自定义 JSON + 签名。V1 推荐 JWT，算法 `RS256`。

Payload：

```json
{
  "iss": "yn-license",
  "aud": "product_code",
  "sub": "license_no",
  "product_id": "prod_...",
  "bind_mode": "device",
  "bind_hash": "sha256...",
  "status": "active",
  "exp": 1815868800,
  "iat": 1784332800,
  "kid": "product_key_v1"
}
```

注意：

- token 只代表一次签发时的状态，不替代在线 verify。
- 吊销授权必须通过在线 verify 才能实时生效。

### 9.2 离线 token

离线 token 内容：

```json
{
  "type": "offline_cache",
  "license_no": "lic_...",
  "product_code": "yn_app",
  "bind_mode": "device",
  "bind_hash": "sha256...",
  "license_expired_at": "2027-07-18T00:00:00Z",
  "offline_until": "2026-08-02T00:00:00Z",
  "issued_at": "2026-07-18T00:00:00Z",
  "key_version": 1
}
```

客户端离线判断：

1. 验证签名。
2. 校验产品代码。
3. 校验绑定主体 hash。
4. 校验当前时间不超过 `license_expired_at`。
5. 校验当前时间不超过 `offline_until`。
6. 校验本地高水位时间。

## 10. 离线授权设计

### 10.1 在线缓存模式

适合大多数桌面软件和插件。

流程：

1. 首次在线激活成功。
2. 服务端返回离线 token。
3. 客户端保存 token 和本地高水位时间。
4. 断网时客户端验签并检查 `offline_until`。
5. 恢复联网后调用 verify，刷新离线 token 和高水位。

安全边界：

- 可防止普通用户改文件。
- 不能绝对防止完整系统快照回滚。
- 文档和后台提示应避免承诺“绝对防倒拨”。

### 10.2 纯离线模式

适合 B 端内网交付。

流程：

1. 客户端工具生成机器码。
2. 管理员在后台输入机器码、产品、到期时间、客户信息。
3. 服务端生成 `license.key`。
4. 客户端内置公钥验签。

`license.key` 使用版本化文件包装，`token` 为产品 RSA 私钥签名的 `base64url(payload).base64url(signature)`：

```json
{
  "format": "yn-license-key",
  "version": 1,
  "token": "eyJ2ZXJzaW9uIjox...RSA_SIGNATURE"
}
```

签名载荷包含 `license_no`、产品 ID/编码/AppKey、机器码、签发时间、到期时间和永久
标记。客户端先校验文件格式和版本，再使用产品公钥验证 token 签名，最后比较机器码
和有效期。服务端数据库只保存机器码 HMAC 哈希、AES-GCM 密文和脱敏展示值。

### 10.3 本地高水位时间

客户端保存：

- `last_server_time`
- `last_success_verify_time`
- `last_local_run_time`
- 离线 token hash

保存位置：

- Windows：DPAPI + AppData。
- macOS：Keychain + Application Support。
- Linux：Secret Service 或本地加密文件。

判断：

- 如果当前本地时间早于高水位时间，进入锁定或要求联网。
- 如果本地记录损坏，要求联网恢复。
- 如果超过 `offline_until`，要求联网恢复。

## 11. 并发与事务设计

### 11.1 激活事务

伪流程：

```text
begin transaction
  card = select card by code_hash for update
  validate card

  if card.activated_license_id is null:
      create license
      update card status = activated
  else:
      license = select license for update

  binding = find active binding by license_id + bind_hash
  if binding exists:
      return existing license

  active_count = count active bindings
  if max_bind_count != -1 and active_count >= max_bind_count:
      if strategy == reject:
          rollback and return DEVICE_LIMIT_EXCEEDED
      if strategy == kick_oldest:
          oldest = select oldest active binding for update
          mark oldest kicked

  create binding
commit
```

兼容注意：

- PostgreSQL/MySQL 可使用 `SELECT ... FOR UPDATE`。
- SQLite 写事务是数据库级或页级锁，开发环境可接受。
- 不建议依赖 `DELETE ... ORDER BY ... LIMIT`，跨库兼容性一般。

### 11.2 续费事务

```text
begin transaction
  license = select license for update
  card = select renew card for update
  validate card unused and same product
  new_expired_at = max(old_expired_at, now) + duration
  update license
  update card status = consumed_for_renewal
  insert renewal record
commit
```

### 11.3 解绑事务

```text
begin transaction
  license = select license for update
  binding = select binding for update
  check cooldown and limit
  update binding status = unbound
  insert unbind record
commit
```

### 11.4 代理生成卡密事务

```text
begin transaction
  agent = select agent for update
  validate agent active

  policy = select agent product policy for update
  validate product allowed
  validate duration allowed
  validate batch quantity

  quota = calculate or select quota summary for update
  validate quota enough
  insert quota ledger change_type = generate_cards

  create card batch with agent_id
  create cards with agent_id
  insert audit log
commit
```

注意：

- 额度扣减和卡密生成必须在同一事务中完成。
- 若批量生成数量较大，可先在事务外生成候选随机码，事务内只做判重、落库和扣额度。
- 代理作废未使用卡密并返还额度时，也必须在事务中完成。

## 12. 缓存设计

### 12.1 可缓存内容

- 产品公开配置。
- 产品公钥。
- 授权验证结果短缓存。
- nonce 防重放记录。

### 12.2 不建议缓存内容

- 卡密明文。
- 私钥。
- 管理员权限。
- 高风险授权状态长期缓存。

### 12.3 验证缓存策略

`/verify` 可以缓存短时间结果：

- 正常授权：30 到 120 秒。
- 过期/吊销：不缓存或极短缓存。
- 产品配置变更：通过版本号失效。

缓存 key：

```text
verify:{product_id}:{license_no}:{bind_hash}:{policy_version}
```

## 13. 错误码设计

| 错误码 | 含义 |
| --- | --- |
| `INVALID_REQUEST` | 请求格式错误 |
| `INVALID_APP_KEY` | AppKey 不存在 |
| `PRODUCT_DISABLED` | 产品已停用 |
| `INVALID_SIGNATURE` | 签名错误 |
| `REQUEST_EXPIRED` | Timestamp 超出允许范围 |
| `REPLAY_REQUEST` | nonce 重复 |
| `CARD_INVALID` | 卡密不存在或格式错误 |
| `CARD_VOIDED` | 卡密已作废 |
| `CARD_USED_BY_OTHER_BINDING` | 卡密已被其他主体占用且不允许新增 |
| `LICENSE_NOT_FOUND` | 授权不存在 |
| `LICENSE_EXPIRED` | 授权已过期 |
| `LICENSE_IN_GRACE` | 授权已过期但仍在宽限期 |
| `LICENSE_REVOKED` | 授权已吊销 |
| `BINDING_REQUIRED` | 缺少绑定信息 |
| `BINDING_MISMATCH` | 绑定主体不匹配 |
| `DEVICE_LIMIT_EXCEEDED` | 绑定数量超限 |
| `UNBIND_LIMIT_EXCEEDED` | 解绑次数超限 |
| `UNBIND_COOLDOWN` | 解绑冷却期未结束 |
| `OFFLINE_DISABLED` | 产品不允许离线 |
| `OFFLINE_EXPIRED` | 离线凭证已过期 |
| `TIME_TAMPERED` | 检测到本地时间异常 |
| `AGENT_NOT_FOUND` | 代理不存在 |
| `AGENT_DISABLED` | 代理已停用 |
| `AGENT_SUSPENDED` | 代理已冻结 |
| `AGENT_PRODUCT_NOT_ALLOWED` | 代理无权销售该产品 |
| `AGENT_DURATION_NOT_ALLOWED` | 代理无权生成该授权时长 |
| `AGENT_PERMANENT_NOT_ALLOWED` | 代理无权生成永久卡 |
| `AGENT_BATCH_LIMIT_EXCEEDED` | 代理单批生成数量超限 |
| `AGENT_QUOTA_NOT_ENOUGH` | 代理额度不足 |
| `AGENT_EXPORT_FORBIDDEN` | 代理无权导出明文卡密 |
| `AGENT_DATA_FORBIDDEN` | 代理无权访问该数据 |

## 14. 后台页面设计

### 14.1 导航

- 仪表盘
- 产品管理
- 代理管理
- 卡密批次
- 卡密查询
- 授权管理
- 离线授权
- 审计日志
- 系统设置

### 14.2 仪表盘

指标：

- 今日激活数。
- 今日验证数。
- 即将过期授权。
- 吊销授权数。
- 设备超限次数。
- 离线文件生成数。
- 代理今日发卡数。
- 代理今日激活数。

### 14.3 产品管理

字段：

- 产品名称。
- 产品编码。
- AppKey。
- 绑定模式。
- 最大绑定数。
- 超限策略。
- 离线策略。
- 宽限期。
- 状态。

操作：

- 新增产品。
- 编辑策略。
- 停用产品。
- 轮换密钥。
- 查看公钥。

### 14.4 代理管理

列表字段：

- 代理编号。
- 代理名称。
- 联系人。
- 上级代理。
- 状态。
- 今日发卡数。
- 今日激活数。
- 创建时间。

详情页：

- 基本信息。
- 员工账号。
- 可售产品。
- 产品政策。
- 额度流水。
- 卡密批次。
- 授权列表。
- 审计日志。

操作：

- 新增代理。
- 编辑代理。
- 冻结代理。
- 停用代理。
- 配置可售产品。
- 发放额度。
- 扣减额度。
- 重置代理员工密码。

### 14.5 卡密批次

操作：

- 创建批次。
- 导出卡密。
- 作废未使用卡密。
- 查看使用统计。
- 分配给代理。

导出要求：

- 二次确认。
- 高权限校验。
- 写审计日志。

### 14.6 授权详情

展示：

- 授权号。
- 来源卡密。
- 产品。
- 来源代理。
- 状态。
- 到期时间。
- 绑定主体列表。
- 验证记录。
- 续费记录。
- 解绑记录。
- 审计日志。

操作：

- 吊销授权。
- 管理员解绑。
- 生成离线文件。

### 14.7 代理后台

代理后台导航：

- 概览
- 可售产品
- 我的额度
- 卡密批次
- 卡密查询
- 授权查询
- 员工账号
- 操作记录

代理概览指标：

- 剩余额度。
- 今日生成卡密数。
- 今日激活数。
- 本月激活数。
- 即将过期授权数。

代理卡密页面：

- 创建卡密批次。
- 导出未使用卡密。
- 查询卡密状态。
- 作废未使用卡密。

代理授权页面：

- 查询自己名下授权。
- 查看绑定设备。
- 查看过期时间。
- 查看续费记录。

代理后台不得提供：

- 产品密钥查看。
- 产品策略编辑。
- 全局授权搜索。
- 其他代理数据访问。

## 15. SDK 设计

### 15.1 SDK 职责

SDK 提供：

- 激活。
- 在线验证。
- 心跳。
- 续费。
- 解绑。
- 离线 token 保存和验签。
- 高水位时间维护。
- 错误码枚举。

当前交付状态：Go SDK 位于 `sdk/go`，Node.js SDK 位于 `sdk/node`，Java SDK 位于
`sdk/java`，三套 SDK 均已实现相同签名载荷与错误模型。Go SDK 仅依赖标准库；
Node.js SDK 要求 Node.js 18+、无第三方运行时依赖并附带 TypeScript 声明；Java SDK
要求 Java 17+，使用 JDK HttpClient/加密 API 和 Jackson JSON 解析器。

SDK 不负责：

- 绕过业务系统登录。
- 存储管理员密钥。
- 执行复杂风控。

### 15.2 SDK 推荐接口

```text
client.activate(cardCode, bindValue)
client.verify()
client.heartbeat()
client.renew(cardCode)
client.unbind(reason)
client.verifyOffline()
```

在线离线 token 同时包含服务端内部 `bind_hash` 和客户端可计算的 `bind_digest`。
`bind_digest = SHA256(lower(bind_mode) + NUL + lower(trim(bind_value)))`，并包含在 RSA
签名载荷中。SDK 必须比较当前绑定值摘要；不含该字段的旧 token 应联网刷新。

### 15.3 客户端集成建议

- 启动时优先本地离线验证，避免每次启动卡顿。
- 后台异步发起在线 verify。
- 在线 verify 失败时按错误码决定是否锁定。
- 连续网络失败但离线 token 有效时允许继续使用。
- 离线 token 过期后必须联网。
- 本地高水位通过 SDK 的存储接口写入 DPAPI、Keychain、Secret Service 或等效安全
  存储，不允许默认明文落盘。

## 16. 安全设计

### 16.1 密钥管理

密钥类型：

- `card_pepper`：卡密 HMAC pepper，环境变量或 KMS。
- `data_encryption_key`：AES-GCM 数据加密主密钥。
- `product_private_key`：产品签名私钥。
- `api_secret`：可信服务端调用密钥。

要求：

- 私钥和 API secret 加密入库。
- 加密密钥不和数据库放在一起。
- 支持密钥版本 `key_version`。
- 支持产品级密钥轮换。
- 旧公钥保留到旧 token 过期。

当前实现使用 `product_keys` 保存产品历史公钥，`products` 表保存当前公私钥。只有
`super_admin` 可执行轮换；新 token 的 `key_version` 对应产品密钥版本，客户端据此从
公钥环选钥。完全离线文件的 `version` 仍表示文件格式版本，与签名密钥版本分离。

### 16.2 权限控制

后台角色：

- `super_admin`：系统级配置、密钥轮换。
- `admin`：产品和授权管理。
- `operator`：生成卡密、查询授权。
- `auditor`：只读审计。

代理后台角色：

- `agent_owner`：代理主账号，管理代理员工和自身业务。
- `agent_manager`：生成卡密、导出卡密、查看授权和统计。
- `agent_staff`：查看和分发卡密，查看授权。
- `agent_readonly`：只读查询。

权限边界：

- 平台角色可以按权限查看全局数据。
- 代理角色只能访问自己 `agent_id` 范围内的数据。
- 下级代理数据默认不可见，除非启用代理树并授予 `view_child_data`。
- 代理不能查看产品私钥、AppSecret、系统级配置。
- 代理不能吊销授权，只能作废自己名下未使用卡密。

高危操作：

- 导出卡密明文。
- 吊销授权。
- 生成纯离线文件。
- 轮换密钥。
- 发放或扣减代理额度。
- 配置代理可售产品。

高危操作需要：

- 二次确认。
- 权限校验。
- 审计日志。

### 16.3 频率限制

建议限流维度：

- IP。
- AppKey。
- 卡密 hash。
- license_no。
- bind_hash。

重点接口：

- `/activate`
- `/verify`
- `/renew`
- `/unbind`

## 17. 部署设计

### 17.1 开发环境

```text
Go API + SQLite + Local Cache
React Dev Server
```

### 17.2 单机生产

```text
Go API
PostgreSQL/MySQL
Local Cache
Nginx/Caddy TLS
```

### 17.3 多实例生产

```text
Load Balancer
Go API x N
PostgreSQL/MySQL
Redis
Object Storage for exports
```

## 18. 开发阶段拆分

### Phase 1：授权核心

- 数据库迁移。
- 产品管理。
- 卡密生成与查询。
- 在线激活。
- 在线验证。
- 审计日志。

验收：

- 能创建产品。
- 能生成卡密。
- 能用卡密激活授权。
- 能绑定设备。
- 能验证成功和失败。

### Phase 2：生命周期

- 续费。
- 解绑。
- 设备超限拒绝。
- 踢出最久未活跃设备。
- 吊销授权。

验收：

- 超限策略行为正确。
- 续费公式正确。
- 解绑冷却和次数限制正确。

### Phase 3：代理能力

- 代理管理。
- 代理员工账号。
- 代理产品政策。
- 代理额度发放和扣减。
- 代理自助生成卡密。
- 代理卡密导出。
- 代理数据隔离。

验收：

- 管理员能创建代理并配置可售产品。
- 管理员能给代理发放指定产品和时长的额度。
- 代理只能生成被授权产品和时长的卡密。
- 代理生成卡密会扣减额度。
- 作废代理未使用卡密可按规则返还额度。
- 代理只能看到自己名下卡密和授权。
- 代理不能访问其他代理数据。

### Phase 4：离线能力

- 在线缓存离线 token。
- SDK 本地验签。
- 纯离线文件生成。
- 高水位时间机制。

验收：

- 断网可在离线期限内运行。
- 离线过期后要求联网。
- 机器码不匹配时拒绝。

### Phase 5：后台与 SDK 完善

- React 后台页面。
- React 代理后台页面。
- Go/Java/Node.js SDK。
- API 文档。
- Postman Collection。
- 错误码文档。

## 19. 推荐项目结构

```text
.
├── backend
│   ├── cmd
│   │   └── server
│   ├── internal
│   │   ├── api
│   │   ├── config
│   │   ├── crypto
│   │   ├── domain
│   │   ├── service
│   │   ├── repository
│   │   ├── middleware
│   │   └── migration
│   ├── pkg
│   │   └── sdktypes
│   └── migrations
├── frontend
│   ├── admin
│   └── agent
├── sdks
│   ├── go
│   ├── java
│   └── node
├── docs
│   ├── api
│   ├── system-design-v1.md
│   └── error-codes.md
└── scripts
```

## 20. 关键验收用例

### 20.1 激活幂等

同一卡密、同一设备重复激活：

- 返回同一个 `license_no`。
- 不新增 binding。
- 不改变到期时间。

### 20.2 多设备授权

产品最大绑定数为 2：

- 设备 A 激活成功。
- 设备 B 激活成功。
- 设备 C 激活时，如果策略是拒绝，返回 `DEVICE_LIMIT_EXCEEDED`。
- 设备 C 激活时，如果策略是踢出，A 或最久未活跃设备变为 `kicked`，C 成功。

### 20.3 续费

授权未过期：

```text
new_expired_at = old_expired_at + duration
```

授权已过期：

```text
new_expired_at = now + duration
```

### 20.4 离线缓存

- 在线激活后获得离线 token。
- 断网且未超过 `offline_until` 时可用。
- 超过 `offline_until` 时不可用。
- 绑定主体变化时不可用。

### 20.5 卡密安全

- 数据库泄露时不能直接获得卡密明文。
- 相同卡密输入能通过 hash 查询。
- 明文导出产生审计日志。

### 20.6 代理发卡

- 平台创建代理并配置可售产品。
- 平台给代理发放 100 张 365 天额度。
- 代理生成 10 张 365 天卡密后，剩余额度为 90。
- 代理尝试生成永久卡时，如果未授权，返回 `AGENT_PERMANENT_NOT_ALLOWED`。
- 代理尝试生成超过额度的卡密时，返回 `AGENT_QUOTA_NOT_ENOUGH`。
- 代理只能导出自己名下未使用卡密。

### 20.7 代理数据隔离

- 代理 A 不能查询代理 B 的卡密批次。
- 代理 A 不能查询代理 B 的授权详情。
- 代理 A 不能通过修改请求参数访问平台全量数据。
- 平台管理员可以按代理筛选全局数据。

## 21. 后续可扩展点

- 多租户。
- 二级代理和代理区域保护。
- Webhook 通知。
- License feature flags。
- 按模块授权。
- 灰度策略。
- 风控评分。
- 硬件锁或 TPM 增强。
