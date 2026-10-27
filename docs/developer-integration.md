# 开发者授权接入

一次集成后统一分发：开发者创建一个应用，接入公开 AppID 和验签公钥；所有客户安装同一产品，在产品内输入各自卡密激活。客户差异存在平台套餐、授权与安装数据中，不在源码或专属编译参数中。

## 从页面开始

管理员在产品行点击“接入授权”，开发者在自己的产品页查看指南。先配置默认条款或销售套餐，选择绑定方式、使用／更新期限、实例／设备／席位数量和功能。不同规格共用 AppID 与版本库；修改默认值不改变库存卡和已售授权。

选择 Node.js、Go 或 Java，下载接入包。包内含 SDK 源码、公开 config.json 和可执行 demo，不依赖本 SDK 发布到公共仓库。绑定值是可留空的联调默认值。生产将卡密和实际部署信息放进产品安装向导；不要为每个客户修改后重新打包。

```text
node app.mjs activate CARD_CODE app.example.com
node app.mjs verify
node app.mjs heartbeat
node app.mjs claim
node app.mjs serve
```

Go 使用 go run .；Java 使用 mvn -q compile exec:java -Dexec.args="activate CARD_CODE app.example.com"。设备和实例模式可省略绑定参数；组合绑定为 JSON，使用产品 UI 可避免命令行转义。初次 Java 编译需要 Maven 插件及 Jackson 依赖。

私有数据保存在 .license-data。每安装创建随机实例 ID 和私有凭证，重启及容器重建必须保留；发行包不能带已激活目录。Web 的 /protected 检查真实请求 Host，反代到 127.0.0.1:3000，或本机 curl -H 'Host: app.example.com' http://127.0.0.1:3000/protected。不信任任意 X-Forwarded-Host。/healthz 仅作存活探针。

## HTTP 流程

所有 JSON 响应包含 success、data、request_id。客户端字段 app_key 使用系统生成的 18 位 AppID（yn + 16 位十六进制）；无需后台登录 token。

1. POST /v1/cards/requirements，提交 app_key、card_code，读取本卡 authorization_terms。按卡的规则获取设备、域名、网段、租户等绑定。
2. POST /v1/licenses/activate，提交卡密、bind_mode、bind_value，启用实例身份时带 instance_id、instance_secret、device_id。返回完整 license_no、can_use、can_update、授权条款及签名凭证。
3. POST /v1/licenses/verify，提交 AppID、完整授权号、实际绑定与实例凭证。仅 can_use=true 执行核心业务；不能只判断 HTTP 200。
4. heartbeat 上报活动；accepted=true 不代替 can_use。renew 增加 renew_card_code，按卡用途分别续使用或更新。
5. 启用席位时每进程创建 session_id，调用 /v1/licenses/seats/acquire 获取 seat_id 后核验；运行期间续租，退出 release。离线借用在期限内持续占用配额。
6. 产品调用 /v1/licenses/claim-ticket，客户把十分钟一次性 claim_token 提交用户中心的 /user/licenses/claim，关联账号。公开授权号不能任意认领或转让已有归属。

完整绑定、设备提供器、实例与席位区别、到期矩阵及边界见 [授权场景说明](licensing-scenarios.md)。域名范围校验不能用固定配置值代替真实 Host；IP 应来自可信部署环境。泛域激活登记范围，使用时提交实际子域；CIDR 使用时提交具体 IP。完全离线文件与在线授权分开验证。

## 在 Node 产品中使用运行适配器

```js
import {RuntimeLicense} from './sdk/src/index.js';
const license = await RuntimeLicense.open({
  baseUrl: config.base_url, appKey: config.app_key,
  productId: config.product_id, productCode: config.product_code,
  publicKeys: config.public_keys, stateDir: '/private/product-license',
  currentVersion: '1.0.0',
  target: {os:'linux',arch:'amd64',runtime:'node',channel:'stable',kind:'archive'}
});
// 安装向导：await license.activate(customerCard, registration);
// 每个受保护请求：await license.verify({request:req});
// 功能入口：await license.requireFeature('export', {request:req});
```

默认每次在线核验。可选正向缓存最多 60 秒，受签名期限和席位截止约束。离线必须显式配置 allowOffline 和持久 HighWaterGuard。明确拒绝、坏签名不能触发离线回退。Go/Java 提供相同协议字段和验签工具；由应用安排心跳、缓存与安装流程。

## 使用期限和更新

can_use 管运行，can_update 管升级。永久使用、更新到期可运行旧版本；订阅使用宽限不延长更新资格。4xx 明确拒绝时停止对应业务；网络故障默认 503，不能当作永久授权。

三个接入包支持 updates 1.0.0 和 download 1.0.0。先启用应用升级服务、上传并发布版本。target 匹配平台、CPU、运行时、渠道、交付类型和来源版本。下载会验证产品、公钥版本、签名、目标、大小、SHA-256，并在传输后复验资格。实际替换前再调用 /v1/updates/{id}/authorize。

安装由产品完成：备份、切换、迁移、健康检查、失败回滚，并保留实例身份和客户数据。参考 [Web 升级](../examples/update-demo/README.md) 与 [GUI、CLI、Docker 示例](../examples/universal-client/README.md)。升级下载不会自动成为适配所有产品的安装器。

## AppSecret 服务端接入

AppSecret 为独立 32 位随机十六进制密钥，创建或重置后只展示一次；服务端仅保存摘要。只放开发者自己的可信服务端，不能随客户产品分发。

1. POST /v1/app/token：{"grant_type":"client_credential","appid":"yn0123456789abcdef","secret":"32位AppSecret"}，返回 access_token、token_type=Bearer、expires_in=7200。
2. 缓存并到期刷新，Bearer 令牌请求 GET /v1/app/profile 核对应用身份。
3. POST /v1/app/licenses/verify，带相同令牌以及授权号、绑定、实例字段，应用由令牌确定；仍分别判断 can_use 和 can_update。

令牌不能访问后台或其他应用。重置要求原因并使旧密钥、旧应用令牌立即失效；不会改变已有授权或验签公钥。普通产品客户端使用 AppID、客户凭证与绑定，无需 AppSecret。

公钥可随产品分发，按 key_version 选历史公钥；未知版本从可信授权站更新，不能跳过验签。私钥、后台 token、数据库密码不进入产品；卡密、完整 token 不写入日志和 URL。排障保留 code、request_id。

## 权限和验证

接入包下载仅限管理员或所属开发者：GET /admin/products/{id}/integration-kit（或 /developer 对应路径），参数 language、base_url、可选 bind_value。默认条款、套餐目录、安装管理同样执行产品范围隔离；用户只管理自己的授权。

CI 会从真实页面下载三个语言接入包并编译运行，验证卡密、实例、席位、心跳、续费、一次性归属、签名升级、实际 Host、到期、吊销和断网；另测原生 GUI、容器镜像实际更新与回滚，并保存报告和截图。
