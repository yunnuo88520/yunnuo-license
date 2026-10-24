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
2. 填写版本号和更新说明，上传升级包。版本格式为 `主版本.次版本.修订号`，如 `1.10.0`；暂不支持预发布版本、渠道和平台变体。
3. 包格式支持 `.zip`、`.tar.gz`、`.tgz`，单个包最大 256 MiB。服务把文件当作不透明归档保存，不解压或验证产品内部结构。
4. 上传后为草稿，客户不可获取。点击“发布版本”后，检查更新接口返回比客户端当前版本更高的最高版本。
5. 下架后停止新的下载。已发出的文件无法收回；已开始传输的下载不会中断。

版本号和包上传后不可覆盖。同一产品的版本号唯一，修复错误包时应下架并上传新版本号。
客户端不会收到降级指令；已安装更高版本的客户不会因为下架而自动回滚。

## 授权规则

检查更新及每次下载都使用现有在线授权校验：产品有效、卡密已激活、授权未吊销、绑定有效且匹配，并通过现有风控规则。
临时授权到期时即停止更新资格；授权运行宽限期不延长更新权限。永久授权不受时间限制。
续期后恢复资格。关闭升级服务后不再提供版本和下载。

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
| 版本列表 / 上传 | `GET/POST /developer/products/{productID}/releases` |
| 发布 / 下架 | `POST /developer/products/{productID}/releases/{releaseID}/publish` 或 `/withdraw` |
| 管理员分配产品 | `POST /admin/products/{productID}/developer`，`{ "developer_id": "账号ID" }`，空字符串取消分配 |
| 超级管理员启停开发者 | `POST /admin/developers/{userID}/enable` 或 `/disable` |

管理员使用 `/admin/products/{productID}/updates` 和 `/admin/products/{productID}/releases` 对应接口管理所有产品。
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

常见错误：`LICENSE_EXPIRED`、`LICENSE_REVOKED`、`BINDING_MISMATCH`、`PRODUCT_DISABLED`、`UPDATES_DISABLED`、`RELEASE_NOT_FOUND`、`RELEASE_INTEGRITY_FAILED`。

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
Go、Java、PHP 等同样可以接入上述 HTTP 接口；本次未给 Go/Java SDK 增加专用升级方法。

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
