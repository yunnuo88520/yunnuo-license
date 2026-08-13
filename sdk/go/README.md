# 允诺云授权 Go SDK

该模块封装在线激活、验证、心跳、续费、解绑，以及在线缓存 token 和完全离线
`license.key` 的本地 RSA 验签。仅依赖 Go 标准库。

## 安装

```bash
go get github.com/yunnuo88520/yunnuo-license/sdk/go
```

在代码中导入：

```go
import ynlicense "github.com/yunnuo88520/yunnuo-license/sdk/go"
```

## 在线授权

```go
client, err := ynlicense.NewClient(
    "https://license.example.com",
    "app_xxx",
    ynlicense.WithUserAgent("my-product/1.0"),
)
if err != nil {
    log.Fatal(err)
}

license, err := client.Activate(ctx, ynlicense.ActivateRequest{
    CardCode:  "YN-XXXX-XXXX-XXXX-XX",
    BindMode:  "device",
    BindValue: machineCode,
    DeviceName: "Office PC",
})
if err != nil {
    if ynlicense.IsAPIErrorCode(err, "CARD_INVALID") {
        // 提示卡密无效。
    }
    log.Fatal(err)
}
```

`Verify`、`Heartbeat`、`Renew` 和 `Unbind` 使用对应请求结构。SDK 自动加入
`app_key`，并在 `APIError` 中保留 HTTP 状态码、业务错误码和 `request_id`。

## 在线缓存验签

产品公钥可从管理后台产品记录获取，并应随产品应用发布：

```go
verifier, err := ynlicense.NewOfflineVerifier(productPublicKeyPEM, "YN")
if err != nil {
    log.Fatal(err)
}

claims, err := verifier.VerifyOfflineToken(license.OfflineToken, ynlicense.OfflineExpectation{
    LicenseNo: license.LicenseNo,
    BindMode:  "device",
    BindValue: machineCode,
})
```

验签同时检查产品、授权号、当前绑定摘要、授权有效期、离线窗口和异常未来签发时间。
旧版不含 `bind_digest` 的 token 会返回 `BINDING_MISMATCH`，客户端应联网刷新 token。

## 完全离线文件

```go
content, err := os.ReadFile("license.key")
if err != nil {
    log.Fatal(err)
}
claims, err := verifier.VerifyLicenseFile(content, machineCode)
if err != nil {
    log.Fatal(err)
}
```

该方法验证文件格式、RSA 签名、产品、机器码、签发时间和到期时间。

## 时间高水位

`HighWaterGuard` 用于发现明显的系统时间倒拨：

```go
guard, err := ynlicense.NewHighWaterGuard(secureStore, time.Minute)
if err != nil {
    log.Fatal(err)
}
if err := guard.CheckAndUpdate(ctx, time.Now()); err != nil {
    // CLOCK_ROLLBACK 时锁定离线授权并要求联网。
}
```

宿主应用必须实现 `HighWaterStore`，并使用 Windows DPAPI、macOS Keychain、Linux
Secret Service 或等效安全存储。SDK 不会把高水位时间明文写入普通文件。
