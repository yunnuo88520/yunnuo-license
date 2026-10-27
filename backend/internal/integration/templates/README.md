# 一次接入，统一分发

开发者只需接入一次。同一程序、同一公开配置可卖给所有客户；客户在产品安装向导中输入自己的卡密和绑定信息，不必为每个客户编译。

包内包含本应用配置和 SDK 源码。Node.js 18+、Go 1.22+ 或 JDK 17+ / Maven 3。Java 首次编译需联网下载 Maven 插件与 Jackson。

1. config.json 只填写授权站 HTTPS 根地址、AppID 和验签公钥等公共信息。不含 AppSecret。bind_value 可留空，仅作本机联调默认值。
2. 在授权站生成测试卡。命令支持 activate CARD_CODE BIND_VALUE；设备与实例模式可自动取本机持久身份，其他模式由安装向导收集。组合绑定为 JSON 对象；GUI 请在输入框中提交，避免命令行空格转义问题。
3. 运行以下命令。示例默认严格在线，每次受保护业务重新校验。

```text
{{COMMAND}} activate CARD_CODE
{{COMMAND}} verify
{{COMMAND}} heartbeat
{{COMMAND}} serve
```

首次激活先读取 /v1/cards/requirements，按卡密套餐处理绑定，保存随机安装 ID、私有凭证和授权状态到 .license-data。保存目录必须持久且私有，不能位于网站静态目录；Docker 挂载持久卷。不要把激活后的数据打入标准发行包，也不要让不同客户共用同一私有目录。备份还原到原安装允许恢复；复制到另一机器不等于获得额外设备配额。

serve 监听 127.0.0.1:3000（DEMO_PORT 可改），/healthz 是不含业务数据的存活探针。/protected 执行实际授权检查。域名产品通过真实授权域名访问反向代理，或本地执行 curl -H 'Host: 你的授权域名' http://127.0.0.1:3000/protected。读取实际 Host，不信任任意 X-Forwarded-Host。将这个检查接到产品的核心服务入口，不能只隐藏前端按钮。

泛域名登记 *.example.com，默认仅匹配一级子域且不含根域；校验时提交实际域名，CLI 可执行 verify app.example.com。CIDR 登记网段，校验时提交实际受保护 IP。此 IP 是应用部署地址，不是任意访客 IP；产品需从可信部署环境获取，不能直接信任用户输入或转发头。组合绑定由所有规定维度共同匹配。集群 ID 应由部署控制器生成，实例与集群分别管理。

未激活、吊销、使用到期、错误绑定返回 403；授权站不可达返回 503。默认不自动离线放行。can_use 管当前版本的使用；can_update 管升级；使用宽限期不会延长更新资格。永久使用、更新到期可继续运行原版；订阅使用到期应停止核心功能。

## 续费、归属与席位

```text
{{COMMAND}} renew RENEW_CARD_CODE
{{COMMAND}} attach LICENSE_NO
```

续费卡必须属于同一应用，套餐可分别续使用或更新服务，已售配额和功能不被新默认值改写。attach 仅用于不要求安装身份的旧授权；实例授权用原卡和原数据目录重新激活恢复。

执行 claim 获取十分钟一次性关联凭证，由客户登录用户中心关联授权。不能以公开授权号任意认领或转移已有归属。换绑和禁用安装受套餐冻结的次数、冷却与配额约束。

并发席位需要每进程 session_id，acquire 获取 seat_id，运行期间续租，退出 release。借用席位占用配额直到到期，不能靠禁用实例或提前释放重复借出。示例自动在线申请席位；完全离线借用需验证签名、绑定、实例、seat_id 与截止时间。

## 更新

Node/Go/Java 接入包均支持 updates 1.0.0 和 download 1.0.0。配置 target 指明 os、arch、runtime、channel、kind；同一版本可发布多平台文件。检查、下载、下载后和真正安装前均校验资格；到期或安全处置后，即使之前下载过也必须中止安装。验证签名、产品、公钥版本、平台、大小与 SHA-256 后存入私有目录。

快速接入包只负责获取经过验证的文件。真实备份、切换、健康检查、回滚见 examples/update-demo（Web）与 examples/universal-client（GUI、CLI、Docker 镜像归档）。持久数据、配置、身份不能随程序覆盖。

## 离线与凭证

三个 SDK 支持离线签名验证。离线文件按设备签发并有独立期限，断网不能实时知道吊销或换绑。离线缓存还需检查 offline_until、实例和席位，以及持久时间高水位，不能修改本地 JSON 就放行。完全离线文件不赋予在线更新资格。未知 key_version 从可信授权站更新公钥环，不能跳过验签。

AppID 和公钥可以公开；AppSecret、管理员登录 token、产品私钥、数据库密码不能分发。SDK 请求字段 app_key 使用 yn + 16 位十六进制 AppID。私有安装凭证和授权状态不提交 Git，不记录卡密、token 或完整授权到日志；排障保留 code 与 request_id。
