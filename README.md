# 允诺云授权系统

[![CI](https://github.com/yunnuo88520/yunnuo-license/actions/workflows/ci.yml/badge.svg)](https://github.com/yunnuo88520/yunnuo-license/actions/workflows/ci.yml)
[![License](https://img.shields.io/github/license/yunnuo88520/yunnuo-license)](LICENSE)
[![Version](https://img.shields.io/badge/version-%E6%9F%A5%E7%9C%8B%20VERSION%20%E6%96%87%E4%BB%B6-146447)](CHANGELOG.md)

允诺云授权系统是可自托管的应用授权与可选升级服务平台。管理员管理平台，开发者管理所属应用，代理用统一用户账号销售已授予套餐，客户管理自己的许可证。

> 当前为上线前的 V2 核心重构。V1 授权、共享实例密钥及旧 SDK 协议已停用；已有数据库升级前先备份，历史授权只读保留，需在新应用中核对权益后重新签发。迁移处理见 [历史授权](docs/authorization-migration.md)。

## 授权与交付

- 应用自动生成公开 AppID、一次性 AppSecret，独立 RS256 签名密钥和 JWKS，按应用轮换。
- 应用范围仓储强制注入 application_id，复合外键阻止跨应用引用；开发者、代理、客户分别检查所属权限。
- 永久、订阅、浮动席位和按量套餐；使用期限与更新期限分别管理，冻结发售规格。
- 机器指纹、域名与别名、泛域名、IPv4/IPv6/CIDR、持久安装、席位、账号、租户和集群锚点。
- 同一 AppID、同一发行包交付多个客户，支持 Web、Docker、Kubernetes、GUI、CLI、服务、插件、移动、IoT、SaaS；客户凭卡密、一次性邀请或已批准安装关联。
- 卡密只存摘要且原文只当次返回；人工订单确认、独立签名销售回调及幂等退款撤销。
- 独立安装 Ed25519 签名、防重放、限流，标准 RS256 JWT 本地验签与在线状态刷新；独立且有限的离线凭证。
- 浮动租约签出/心跳/超时归还，受限离线借用；按量原子扣减与业务键去重。
- 应用分组、开发者归属、代理等级比例/专属成本、预付余额、事务发卡及流水；首期收款由管理员登记。
- 客户归属、卡密续费、受次数与冷却限制的换绑；默认禁止客户转让授权。
- 页面生成 Go、Java、Python、Rust、TypeScript 的可运行接入项目及 HTTP/OpenAPI 文档；无需必须安装的专有 SDK。
- 可选升级服务按平台/架构/运行时/渠道/来源版本分发，签名清单、文件摘要，检查/下载/安装前均在线核验资格。
- 应用追加审计与诊断、平台分类操作日志，前台免登录脱敏授权查询和代理验真。
- 首次强制安装向导、普通登录/图片验证码/可选两步验证、浅色/深色/系统主题、电脑及手机页面。
- Docker Compose、Nginx + Go/宝塔、前后端独立 tar.gz、私有备份与运行指标。

创建应用后进入“接入文档”，选择套餐、部署形态、绑定和语言，下载代码。开发者只集成一次，不为每个客户重新编译。AppSecret 只用于开发者自己的中央服务端，客户部署的服务器同样属于客户端，不能内置 AppSecret。

完整流程见 [开发者接入](docs/developer-integration.md)、[产品升级](docs/product-updates.md)、[销售回调](docs/sales-webhooks.md) 与 [授权设计](docs/authorization-v2.md)。

离线吊销存在窗口期，本地校验无法阻止客户端代码被篡改；短 TTL、有限宽限和在线关键业务共同约束风险。更新不使用离线宽限，永久使用不代表永久更新。

## 部署方式

| 方式 | 交付内容和指南 |
| --- | --- |
| Docker Compose | 根目录 `compose.yml`，内置 MySQL、前后端及持久卷，见 [Docker 部署](docs/docker-deployment.md) |
| Compose + HTTPS | `deploy/compose.production.yml`，可选监控 profile，见同一 Docker 指南 |
| Linux tar.gz / 宝塔 | amd64、arm64 完整包，Nginx 静态前端 + Go API，systemd/Supervisor 配置，见 [传统部署](docs/native-deployment.md) |
| 两台机器前后端分离 | 独立前端和后端压缩包，由 Nginx 同域反代私网 Go API，见 [分离步骤](docs/native-deployment.md#两台机器部署) |

### Compose 快速开始

在源码仓库根目录运行，配置工具会生成私有 `.env`，再次运行保留已有凭据：

```bash
python3 scripts/configure-compose.py
docker compose up -d --build
docker compose exec license cat /var/lib/mysql/.yunnuo/database.json.install-token
```

默认仅监听 `127.0.0.1:8080`，可通过 Nginx/宝塔配置 HTTPS 反代。直接局域网测试可先执行 `python3 scripts/configure-compose.py --bind 0.0.0.0`。首次打开网站会强制进入 `/install`，填入私有令牌、数据库信息及管理员账号，完成后显示各端入口。不要把令牌放入 URL 或公开日志。

内置 MySQL 地址为 `127.0.0.1:3306`，库名和账号默认 `yunnuo_license` / `yunnuo`，密码见私有 `.env`，数据库端口不会发布到宿主机。

### Nginx + Go / 宝塔快速开始

从 Actions 的 **Release bundles** 下载编译好的 Linux tar.gz，或从源码执行 `python3 scripts/build-release.py`。部署服务器无需安装 Go 编译器、Node.js 或 npm。

```bash
umask 022
tar --no-same-owner -xzf yunnuo-license-v0.4.0-linux-amd64.tar.gz
cd yunnuo-license-v0.4.0-linux-amd64
python3 deploy/native/configure.py --domain license.example.com --user www
```

按工具输出配置 Nginx 和 systemd/Supervisor，网站根目录指向 `frontend/`，数据库、密钥和升级包放在独立私有持久目录。普通 Linux 使用已创建的服务用户替代宝塔的 `www`。完整包、后端独立包、前端独立包及 `SHA256SUMS` 都由打包流程生成，版本号以实际下载文件为准。

默认分离模式由 Nginx 提供全部页面和资源，Go 设置 `YN_SERVE_FRONTEND=false`，安装引导和自定义后台入口仍由 Go 的入口校验控制。缺少 Nginx auth_request 模块时，可选择单机兼容模式。详细的宝塔面板步骤、两台机器部署和升级回退见 [传统部署指南](docs/native-deployment.md)。

商用配置、期限规则、备份恢复与完整验收映射见 [商用部署与验收](docs/production-readiness.md)。

## 本地开发

需要 Go 1.26 和 Node.js 22。后端开发模式使用 SQLite，无需单独安装数据库。
首次启动前构建 Vue 前端：

```bash
cd frontend
npm ci
npm run build
```

然后启动后端：

```bash
cd backend
go run ./cmd/server
```

首次运行访问任意业务页面，都会强制进入 `/install`。本地安装令牌默认位于 `backend/yn-license-config.json.install-token`，由服务器管理员读取并填入向导。向导中选择 SQLite 文件或填写 MySQL 地址、端口、库名、用户名和密码，再设置管理员账号与密码。安装完成后显示管理后台、代理后台、前台首页及用户、开发者入口，禁止重复安装。环境变量中的管理员账号密码不能跳过安装。

默认管理页面为 `/admin`。在 `YN_DB_CONFIG_FILE` 指向的 JSON 配置文件（本地默认 `backend/yn-license-config.json`）中修改 `"admin_path": "/your-panel"`，重启服务后生效。原页面入口停止提供管理页面，`/admin/*` API 路径保持稳定。

安装生成的数据库配置优先于启动环境中的 `YN_DB_DRIVER`、`YN_DB`，避免重启回到临时数据库。新安装未设置 `YN_CARD_PEPPER`、`YN_DATA_KEY` 时自动生成随机密钥并写入权限为 `0600` 的配置文件；已有安装保留原密钥。请同时备份数据库、配置文件和升级包目录。单镜像默认全部存放于 `/var/lib/mysql` 数据卷；显式配置环境密钥时须保持其不变。

账号恢复入口为 `/recover`。管理员在“账号恢复”中核实身份后发放一次性恢复码或重置密码，原会话随即失效。管理员全部忘记密码时可在服务器本地恢复，详见 [安装与账号恢复](docs/installation-and-recovery.md)。资金流水、安全记录和申诉支持分页、筛选与完整 CSV 导出。

API 接入见 [当前协议](docs/api/quickstart.md)、[开发者接入](docs/developer-integration.md) 和 [安全拦截与申诉](docs/security-integration.md)。

## 多语言接入

在应用接入页按套餐、绑定与形态生成 Go、Java、Python、Rust、TypeScript 完整项目，也可下载 OpenAPI 供任意语言实现。项目使用标准 HTTP、Ed25519 安装证明和 RS256 JWT/JWKS，无必须安装的专有 SDK。Python 包还包含真实网站及原生 GUI，Go/Python 提供签名更新传输示例。

仓库 `sdk/` 是历史 V1 SDK 的存档和兼容回归代码，不能用于 V2 新应用。不要按旧 SDK README 初始化新产品。

## 为产品提供升级服务

站点超级管理员在“系统”创建角色为“开发者”的账号。开发者在 `/developer-console/` 登录，
创建自己的产品并接入授权；也可由管理员在“应用授权中心 → 应用与策略”分配应用。
每个产品默认只提供授权，按需启用升级服务后可上传、发布和下架版本。
只有更新资格未到期、未吊销且绑定匹配的客户端才能检查和下载更新。
支持“订阅使用”和“永久使用、限期更新”；使用宽限期不会延长更新资格。

此功能用于分发 Ebox 等被授权产品的升级包。产品客户端负责备份、安装和回滚。
接入方法、验签及存储要求见 [产品升级服务](docs/product-updates.md)。

## 测试

```bash
(cd backend && go test ./...)
(cd frontend && npm ci && npm run check && npm run build)
(cd sdk/go && go test ./...)
(cd sdk/node && npm test && npm run check)
(cd sdk/java && mvn -B test)
./docker/test-image.sh yunnuo-license:test
```

Docker 测试会构建正式镜像并验证 MySQL 初始化、登录、产品与卡密创建、授权激活、公开查询、
端口暴露和数据持久化。

## 安全

不要提交真实的 `.env`、数据库文件、卡密、产品私钥或生产日志。生产环境的
`MYSQL_PASSWORD`、`MYSQL_ROOT_PASSWORD`、`YN_CARD_PEPPER` 和 `YN_DATA_KEY` 在升级时必须保持
一致，否则已有加密数据可能无法读取。

安全问题请不要创建公开 Issue，报告方式见 [SECURITY.md](SECURITY.md)。

## 参与贡献

提交 Issue 或 Pull Request 前请阅读 [CONTRIBUTING.md](CONTRIBUTING.md) 和
[CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)。

## 许可证

本项目使用 [Apache License 2.0](LICENSE)。
