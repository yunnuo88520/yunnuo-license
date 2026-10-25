# 允诺云授权 Node.js SDK

支持可选的 `securityDecision`、`reportIntegrity` 和网站入口 `enforceWebLicense`。
启用 `reportIntegrityFailures` 后，升级包校验失败可向平台上报待核实线索；不会自行确认盗版。
应用端接入、仅禁更新、拦截、申诉与解除说明见 [安全接入](../../docs/security-integration.md)。

官方 Node.js SDK，支持在线激活、验证、心跳、续费、解绑，以及在线缓存 token 和
完全离线 `license.key` 的本地 RSA 验签。要求 Node.js 18+，无第三方运行时依赖。

可选的产品升级服务支持 `checkUpdate` 和 `downloadUpdate`。后者使用预置信任公钥校验版本清单签名、产品归属、包长度与 SHA-256，成功后返回二进制内容；不会自动安装。
调用示例见 [产品升级服务](../../docs/product-updates.md)。

授权响应分别返回 `can_use`、`can_update`、`usage_expired_at`、`updates_expired_at`。
永久使用模式中，更新到期会使 `can_update=false`，已有版本的 `can_use` 仍为 true。
更新权限必须在线校验；不要把更新截止时间用于离线停用已有版本。

## 本地安装

```bash
npm install ../云授权系统/sdk/node
```

ESM 项目中使用：

```js
import { LicenseClient } from "@yunnuo/license-sdk";

const client = new LicenseClient({
  baseUrl: "https://license.example.com",
  appKey: "app_xxx",
  userAgent: "my-product/1.0",
});

const license = await client.activate({
  card_code: "YN-XXXX-XXXX-XXXX-XX",
  bind_mode: "device",
  bind_value: machineCode,
  device_name: "Office PC",
});
```

`verify`、`heartbeat`、`renew` 和 `unbind` 使用服务端 JSON 的 snake_case 字段。
构造函数中的 `appKey` 会自动且强制加入每个请求，调用参数无法覆盖它。

## 错误处理

```js
import { isAPIErrorCode } from "@yunnuo/license-sdk";

try {
  await client.verify({
    license_no: license.license_no,
    bind_mode: "device",
    bind_value: machineCode,
  });
} catch (error) {
  if (isAPIErrorCode(error, "LICENSE_REVOKED")) {
    // 立即锁定产品功能。
  }
  throw error;
}
```

`APIError` 保留 `code`、`httpStatus` 和 `requestId`。默认请求超时为 10 秒，可通过
`timeoutMs` 调整，也可为每次调用传入 `{ signal }` 主动取消。

## 在线缓存验签

产品公钥应随产品发布，不得把产品私钥放进客户端：

```js
import { OfflineVerifier } from "@yunnuo/license-sdk";

const verifier = new OfflineVerifier({
  publicKeyPem: productPublicKeyPem,
  productCode: "YN",
});

const claims = verifier.verifyOfflineToken(license.offline_token, {
  licenseNo: license.license_no,
  bindMode: "device",
  bindValue: machineCode,
});
```

验签覆盖 RSA 签名、产品、授权号、设备绑定摘要、授权有效期、离线窗口和异常未来
签发时间。旧 token 不含 `bind_digest` 时返回 `BINDING_MISMATCH`，应联网刷新。

### 密钥环模式（支持密钥轮换）

服务器轮换签名密钥后，新 token 由新私钥签发。使用密钥环验签器可按 token 的
`key_version` 自动选择对应版本的公钥，已部署客户端无需发版升级：

```js
import { offlineVerifierFromKeyRing } from "@yunnuo/license-sdk";

// 从授权站拉取公钥环（GET /v1/products/keys，含全部历史版本）并构造验签器。
const verifier = await offlineVerifierFromKeyRing(client, { appKey: "app_xxx" });

// 也可用自带的公钥环直接构造：
// new OfflineVerifier({ publicKeys: { 1: pemV1, 2: pemV2 }, productCode: "YN" })
```

未知 `key_version` 返回 `KEY_VERSION_UNKNOWN`，此时应刷新公钥环后重试。

## 完全离线文件

```js
import { readFile } from "node:fs/promises";

const content = await readFile("license.key");
const claims = verifier.verifyLicenseFile(content, machineCode);
```

## 时间高水位

```js
import { HighWaterGuard } from "@yunnuo/license-sdk";

const guard = new HighWaterGuard({
  store: secureStore,
  allowedRollbackMs: 60_000,
});
await guard.checkAndUpdate(new Date());
```

`secureStore` 需实现异步或同步的 `load()`、`save(date)`。生产环境应使用 DPAPI、
Keychain、Secret Service 或等效安全存储，不要把高水位时间明文写到普通文件。

## 验证

```bash
npm test
npm run check
```
