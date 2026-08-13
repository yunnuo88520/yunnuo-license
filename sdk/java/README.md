# 允诺云授权 Java SDK

官方 Java SDK，支持在线激活、验证、心跳、续费、解绑，以及在线缓存 token 和完全
离线 `license.key` 的本地 RSA 验签。要求 Java 17+。

SDK 使用 JDK `HttpClient` 和标准加密 API；JSON 使用 Jackson 2.x，避免在签名载荷与
服务端响应上维护不可靠的自制解析器。

## 本地构建与安装

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

## 在线授权

```java
import com.yunnuo.license.LicenseClient;
import com.yunnuo.license.model.ActivateRequest;

LicenseClient client = LicenseClient.builder(
        "https://license.example.com",
        "app_xxx")
    .userAgent("my-product/1.0")
    .build();

var license = client.activate(new ActivateRequest(
        "YN-XXXX-XXXX-XXXX-XX",
        "device",
        machineCode,
        "Office PC",
        "1.0.0"));
```

`verify`、`heartbeat`、`renew`、`unbind` 接收 `model` 包中的对应 record。SDK 自动
注入构造时配置的 AppKey，并提供默认 10 秒请求超时和 2 MiB 响应限制。

## 错误处理

```java
try {
    client.verify(request);
} catch (APIException error) {
    if (error.hasCode("LICENSE_REVOKED")) {
        // 立即锁定产品功能。
    }
    throw error;
}
```

`APIException` 保留业务错误码、HTTP 状态码和 `request_id`。

## 在线缓存验签

产品公钥应随产品应用发布，产品私钥绝不能进入客户端：

```java
import com.yunnuo.license.OfflineVerifier;
import com.yunnuo.license.model.OfflineExpectation;

OfflineVerifier verifier = new OfflineVerifier(productPublicKeyPem, "YN");
var claims = verifier.verifyOfflineToken(
        license.offlineToken(),
        new OfflineExpectation(license.licenseNo(), "device", machineCode));
```

验签覆盖 RSA 签名、产品、授权号、绑定摘要、授权有效期、离线窗口和异常未来签发
时间。旧 token 不含 `bind_digest` 时返回 `BINDING_MISMATCH`，应联网刷新。

## 完全离线文件

```java
byte[] content = Files.readAllBytes(Path.of("license.key"));
var claims = verifier.verifyLicenseFile(content, machineCode);
```

## 时间高水位

```java
HighWaterGuard guard = new HighWaterGuard(secureStore, Duration.ofMinutes(1));
guard.checkAndUpdate(Instant.now());
```

`secureStore` 实现 `HighWaterStore`。生产环境应使用 DPAPI、Keychain、Secret Service
或等效安全存储，不要把高水位时间明文写到普通文件。

## 验证

```bash
mvn test
mvn package
```
