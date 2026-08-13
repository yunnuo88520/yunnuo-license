# 允诺云授权 Node.js SDK

官方 Node.js SDK，支持在线激活、验证、心跳、续费、解绑，以及在线缓存 token 和
完全离线 `license.key` 的本地 RSA 验签。要求 Node.js 18+，无第三方运行时依赖。

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
