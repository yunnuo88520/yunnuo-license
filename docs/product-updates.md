# 产品升级服务

以小王的 Ebox 为例：Ebox 接入授权站后可以只用授权，也可以把版本和升级包托管到授权站。
用户在授权有效期内检查并下载新版，由 Ebox 自己执行备份、安装及回滚。
授权站不会执行开发者上传的程序，也不会替换客户网站的文件。

## 账号和产品归属

1. 站点超级管理员在“系统 → 创建账号”选择“开发者”角色。
2. 开发者在 `/developer-console/` 登录，创建自己的产品、生成卡密并查看已激活授权。
3. 开发者只能访问自己的产品、授权及版本。后端按会话和产品归属校验。
4. 管理员在“产品升级”可将已有产品分配或转交给开发者。未分配的产品保留站点管理。
5. 超级管理员可停用开发者账号，已有会话立即失效；重新启用后需要重新登录。

开发者没有站点管理、代理管理、其他开发者数据或全局审计权限。站点管理员保留全局管理能力。
开发者账号由站点创建，本版本没有开放注册。

## 发布步骤

每个产品的升级服务默认关闭，不影响原有激活、验证、续期和心跳接口。

1. 在产品升级面板启用升级服务。
2. 填写版本号和更新说明，上传升级包。版本格式为 `主版本.次版本.修订号`，如 `1.10.0`；支持平台、CPU、运行时、渠道和来源版本范围；暂不支持预发布版本号。
3. 包格式支持 `.zip`、`.tar.gz`、`.tgz`，单个包最大 256 MiB。服务把文件当作不透明归档保存，不解压或验证产品内部结构。
4. 上传后为草稿，客户不可获取。点击“发布版本”后，检查更新接口返回比客户端当前版本更高的最高版本。
5. 下架后停止新的下载。已发出的文件无法收回；已开始传输的下载不会中断。

版本号和包上传后不可覆盖。同一产品的版本号与目标平台组合唯一，修复错误包时应下架并上传新版本号。
客户端不会收到降级指令；已安装更高版本的客户不会因为下架而自动回滚。

## 授权规则

检查更新及每次下载都由服务端校验：产品有效、卡密已激活、授权未吊销、绑定有效且匹配，并通过现有风控规则。
使用期限与更新期限分别记录，以服务端时间为准，截止时刻本身即视为到期。

| 产品授权模式 | 使用到期后的行为 | 更新到期后的行为 |
| --- | --- | --- |
| `subscription` 订阅使用（默认） | 拒绝验证、心跳和重新激活；配置了使用宽限期时可在宽限期内使用 | 拒绝检查更新和下载，不受使用宽限期影响 |
| `perpetual` 永久使用、限期更新 | 使用期限为 `null`，仍需遵守产品状态、绑定、风控和吊销规则 | 保留已有版本的使用权限，拒绝检查更新和下载；续期后恢复更新资格 |

创建产品时选择 `license_mode`，`update_duration_days` 指定每次发卡或续期授予的更新天数，0 表示跟随卡密期限。
订阅的卡密天数控制使用期限；永久使用模式的卡密只控制更新期限。
订阅的更新期限最多到使用期限结束。永久卡配合 `update_duration_days=0` 包含永久更新，指定正数则仍只有该天数的更新服务。
更新截止时间为 `null` 表示永久更新，使用截止时间为 `null` 表示永久使用，两者互不等同。

验证、激活和续期响应增加 `can_use`、`can_update`、`license_mode`、`usage_expired_at`、`updates_expired_at`。
`expired_at` 保持兼容，仍代表使用截止时间。`can_update` 表示授权具备更新资格，是否启用升级服务、是否有新版还需调用检查更新接口。
应用以验证结果控制使用；禁止把更新资格到期误判为停止使用。离线缓存仍受 `offline_until` 限制，但不会因为永久使用模式的更新期限结束而失效。
验证或心跳返回授权无效错误时，应用应拒绝继续使用；永久使用模式的更新到期只会让更新接口返回 `UPDATES_EXPIRED`，使用验证仍可成功。

已有产品、已有授权迁移后保留订阅模式和原期限，不会自动变成永久使用或永久更新。
产品未发卡前可在期限规则面板修改；发卡或创建授权后规则锁定，以保护已售权益，需要不同规则时创建新产品。
续期在未到期时从原截止时间顺延，到期后从续期时间开始；永久使用模式只延长更新期限。吊销不能通过续期解除。
关闭升级服务后不再提供版本和下载。

必须先在线激活并持有 `license_no` 和绑定信息；独立的离线授权文件本身不能用于在线下载。
采用 `none` 无绑定策略时，许可证号也是在线校验凭据，产品方应妥善保管。
生产环境使用 HTTPS，勿在 URL 查询参数中传卡密或绑定信息。

## API

API 沿用 `{ "success": true, "data": ... }` 响应格式。开发者管理接口使用开发者 Bearer token。

| 操作 | 接口 |
| --- | --- |
| 开发者登录 | `POST /developer/login`，`username`、`password` |
| 当前账号 / 修改密码 | `GET /developer/profile` / `POST /developer/password` |
| 自己的产品列表 / 创建产品 | `GET /developer/products` / `POST /developer/products` |
| 发卡 | `POST /developer/products/{productID}/card-batches` |
| 自己产品的授权 | `GET /developer/products/{productID}/licenses?page=1` |
| 升级设置 | `GET/POST /developer/products/{productID}/updates`，POST 为 `{ "enabled": true }` |
| 使用与更新规则 | `GET/POST /developer/products/{productID}/license-policy`，包含 `license_mode`、`update_duration_days`、`expire_grace_days`；GET 额外返回 `locked` |
| 版本列表 / 上传 | `GET/POST /developer/products/{productID}/releases` |
| 发布 / 下架 | `POST /developer/products/{productID}/releases/{releaseID}/publish` 或 `/withdraw` |
| 管理员分配产品 | `POST /admin/products/{productID}/developer`，`{ "developer_id": "账号ID" }`，空字符串取消分配 |
| 超级管理员启停开发者 | `POST /admin/developers/{userID}/enable` 或 `/disable` |

管理员使用 `/admin/products/{productID}/updates`、`license-policy` 和 `releases` 对应接口管理所有产品。
上传使用 `multipart/form-data`，字段为 `version`、`notes` 和文件字段 `package`。
开发者发卡的请求体沿用现有卡密接口，产品始终以路径为准。
修改密码使用 `current_password` 和 `new_password`，成功后旧会话失效。

客户端检查更新：

```http
POST /v1/updates/check
Content-Type: application/json

{
  "app_key": "产品 App Key",
  "license_no": "已激活的许可证号",
  "bind_mode": "domain",
  "bind_value": "customer.example.com",
  "current_version": "1.0.0"
}
```

返回 `enabled`、`update_available`；有更新时返回 `release`，包含 `id`、`product_id`、`version`、`notes`、`filename`、`size`、`sha256`、`key_version` 和 `manifest_token`。
未启用服务或没有更高的已发布版本时，`update_available` 为 `false`，不返回 `release`。

下载使用 `POST /v1/updates/{releaseID}/download`，JSON 请求体包含同样的 `app_key`、`license_no`、`bind_mode`、`bind_value`，不包含 `current_version`。
成功返回二进制附件，禁止缓存。每次下载重新校验当前授权，不提供永久公开下载链接。
GET 不提供下载。不支持断点续传。

常见错误：`LICENSE_EXPIRED`（使用期限结束）、`UPDATES_EXPIRED`（更新资格结束）、`LICENSE_REVOKED`、`BINDING_MISMATCH`、`PRODUCT_DISABLED`、`UPDATES_DISABLED`、`RELEASE_NOT_FOUND`、`RELEASE_INTEGRITY_FAILED`、`LICENSE_POLICY_LOCKED`。
下载在准备文件后还会再次校验授权，避免校验文件期间已经到期仍开始分发。
已经开始的传输或此前下载的文件无法远程撤回；需要限制安装时机的产品应在安装前重新在线校验。

## 安装前验签

`manifest_token` 使用既有产品 RSA 密钥签名，格式为 `base64url(JSON).base64url(RSA-SHA256签名)`。
签名包含 `type=product_release`、`product_id`、`release_id`、`version`、`size`、`sha256` 和 `key_version`。

客户端应预置可信的产品 ID 和公钥，验证签名及字段归属，再检查下载长度与 SHA-256。
不能直接信任更新响应里临时附带的公钥。密钥轮换后，需通过受信任的发布渠道更新客户端信任密钥；旧清单保留原始签名与密钥版本。
展示用文件名和更新说明不属于签名载荷，不能据此选择安装路径或执行命令。

Node.js SDK 示例（使用仓库中的 `sdk/node`，尚未发布到公共包仓库）：

```js
import { LicenseClient } from "@yunnuo/license-sdk";
import { writeFile } from "node:fs/promises";

const client = new LicenseClient({
  baseUrl: "https://license.example.com",
  appKey: PRODUCT_APP_KEY,
  timeoutMs: 120_000,
});
const binding = {
  license_no: storedLicenseNo,
  bind_mode: "domain",
  bind_value: "customer.example.com",
};
const update = await client.checkUpdate({ ...binding, current_version: "1.0.0" });
if (update.update_available) {
  const bytes = await client.downloadUpdate(update.release, binding, {
    publicKeyPem: PINNED_PRODUCT_PUBLIC_KEY,
    productId: PINNED_PRODUCT_ID,
  }); // 验证签名、产品、长度及 SHA-256 后才返回
  await writeFile("./staging/update.zip", bytes, { flag: "wx", mode: 0o600 });
  // 产品自己的流程负责备份、校验归档路径、安装和检查运行状态。
}
```

Node 下载方法在内存中缓冲升级包；大包可直接用 HTTP 接口流式下载到临时文件，验证完成后再安装。
Go、Java、Node.js SDK 均有专用检查、签名验证、下载后资格复验方法；PHP 等语言可接入相同 HTTP 协议。详见 [统一产品授权与更新](licensing-scenarios.md) 和 [通用产品实际安装器](../examples/universal-client/README.md)。

## 存储和部署

- `YN_RELEASE_DIR` 指定非公开包目录，本地默认为后端工作目录下的 `yn-releases`。
- Docker 默认保存到 `/var/lib/mysql/.yunnuo/releases`，复用现有持久卷，重启后保留。
- 包不能放入前端静态目录，不要由反向代理配置无需鉴权的直链。
- 备份数据库时同时备份升级包目录以及原有数据密钥、签名密钥相关配置。
- 反向代理上传体积和超时需允许所选包大小；服务端单包仍限制为 256 MiB。

客户网站的升级脚本、数据库迁移、备份、原子切换及失败回滚由产品提供。
此功能独立于授权站自身升级；授权站自身的在线升级能力仍未启用。

## 独立网站联调示例

[在线升级 Demo](../examples/update-demo/README.md) 提供可重复运行的完整网站升级验证：
启动独立网站，检查授权、验签下载、替换实际网站文件、迁移 SQLite、保留客户文章，
并在新版健康检查失败时恢复网站文件和数据库。示例及报告覆盖正常升级、授权到期拒绝和失败回滚。
安装器仅针对这个 Demo，产品方仍需实现适配自己网站的升级器。


安装前复验：下载及暂存完成后，向 `POST /v1/updates/{releaseID}/authorize` 提交同样的应用、授权号和绑定，确认该版本仍已发布且当前更新权益有效。Node SDK 提供 `authorizeUpdate(releaseID, binding)`，并在下载结束时自动复验。实例安装器修改文件和数据库前须再次调用；复验失败或断网时停止安装。该确认不能缓存为以后的更新许可。


多平台版本请求需携带 target 与 current_version，完整字段和 Docker 镜像归档交付见 [场景说明](licensing-scenarios.md#多平台版本与安装)。旧无目标版本保留兼容，生产多形态产品应显式设置目标。
