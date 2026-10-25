# 授权接入：从此目录开始

接入包已包含本应用配置及 SDK 源码，不需要先从 npm、Maven Central 或 Go 公共仓库安装本 SDK。
Node.js 需要 18+；Go 需要 1.22+；Java 需要 JDK 17+ 和 Maven 3，首次编译需联网下载 Maven 插件与 Jackson 依赖。

1. 打开 config.json，核对 base_url、app_key、product_code、bind_mode 和 bind_value。服务器地址是授权站的根地址，不能填写 /admin 或控制台地址。生产使用 HTTPS。
2. 在授权站为本应用生成一张测试卡。域名只填域名（不含协议/路径），IP 使用实际部署 IP，设备标识应稳定，账号标识由产品服务端决定；不要把所有客户都写成同一固定值。
3. 以下命令中的 CARD_CODE 换为测试卡密。Java 将 COMMAND 替换为整个命令，例如 -Dexec.args="activate CARD_CODE"。

```text
{{COMMAND}} activate CARD_CODE
{{COMMAND}} verify
{{COMMAND}} heartbeat
{{COMMAND}} serve
```

serve 默认监听 127.0.0.1:3000，可设置 DEMO_PORT。打开 http://127.0.0.1:3000/protected：每次进入受保护接口都重新向授权站校验，只在 can_use=true 时执行示例业务。不要仅在前端隐藏按钮，也不要只在首次激活时检查。

授权站不可用时示例拒绝业务并返回 503；未激活、绑定错误、吊销、使用到期等返回 403。默认没有自动离线放行。将 verify 的判断放到真实产品的服务端中间件或核心功能入口，并按产品负载设计短期校验缓存与心跳；错误码和 request_id 用于排障，日志不得记录卡密或 token。

## 已有授权、续费和到期

```text
{{COMMAND}} attach LICENSE_NO
{{COMMAND}} renew RENEW_CARD_CODE
```

attach 会先校验授权号与当前绑定，不能获得其他用户的授权归属。客户端直接激活只绑定部署实例；若需用户中心管理，请让客户在用户中心激活，或由管理员分配归属，然后使用 attach。续费卡必须属于同一产品；需要更换绑定时按授权站的用户换绑规则操作，修改配置中的旧绑定不会自动完成换绑。

使用资格看 can_use，更新资格看 can_update，分别读取 usage_expired_at 和 updates_expired_at。永久使用且更新过期的客户可继续使用旧版，但不能获取新版本；订阅使用到期应停止受保护功能。使用宽限期不能延长升级下载资格。

## 可选升级（Node.js 接入包）

```text
node app.mjs updates 1.0.0
node app.mjs download 1.0.0
```

管理员/开发者先启用当前产品的升级服务，上传并发布高于当前版本的包。download 会检查资格、校验签名和 SHA-256 后保存 update.zip，且使用 release.key_version 选择随包提供的公钥。它不会替产品执行数据库迁移、覆盖程序、备份或回滚。真实在线安装流程见仓库 examples/update-demo。Go/Java 可按页面 HTTP 文档实现升级；当前两种 SDK 的快速接入包专注授权与本地验签。

## 离线与密钥轮换

三个 SDK 均包含离线验签与历史公钥环支持，见随包 SDK-README.md。离线凭证只在签名、产品、绑定、授权期限和 offline_until 都通过时可用；完全离线文件使用设备绑定。离线期间无法立即获知吊销、换绑与安全处置，需结合短期缓存与重新联网校验。未知 key_version 时从可信的授权站刷新公钥环，不得跳过验签。配合系统安全存储和时间高水位检查，不能仅修改本地时间或 JSON 就放行。

## 配置与凭证

App Key、应用代码、公钥可以随产品分发，它们不是管理员凭证。不要把管理员/开发者登录 token、产品私钥或数据库密码放进产品。激活后的 .license.json 由示例保存为私有文件，包含授权凭证，已加入 .gitignore；生产请使用受保护的服务器存储，桌面端使用系统安全存储。
