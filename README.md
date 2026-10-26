# 允诺云授权系统

[![CI](https://github.com/yunnuo88520/yunnuo-license/actions/workflows/ci.yml/badge.svg)](https://github.com/yunnuo88520/yunnuo-license/actions/workflows/ci.yml)
[![License](https://img.shields.io/github/license/yunnuo88520/yunnuo-license)](LICENSE)
[![Version](https://img.shields.io/badge/version-%E6%9F%A5%E7%9C%8B%20VERSION%20%E6%96%87%E4%BB%B6-146447)](CHANGELOG.md)

允诺云授权系统（Yunnuo License）是一套可自托管的通用应用授权服务平台，面向桌面软件、
插件、私有化应用和 SaaS 产品。管理员管理平台、代理销售已获授权的应用套餐、用户管理自己的授权，
开发者独立管理自己的产品与版本；提供在线与离线授权，以及 Go、Java、Node.js SDK。

> 当前项目处于早期版本，API 和数据结构在 `1.0.0` 前可能调整。请在生产部署前完成备份、
> 安全配置和业务验收。

## 功能

- 产品与签名密钥管理，支持产品密钥轮换
- 卡密批量生成、导出、作废、激活与续期
- 设备、账号、域名、IP 或无绑定授权策略
- 在线验证、心跳、解绑、吊销和公开授权查询
- 在线缓存凭证与完全离线授权文件
- 管理员角色权限与审计日志
- IP/设备黑名单、异常激活检测与风控告警
- 代理账号、产品策略、发卡额度和代理数据隔离
- 独立普通用户中心、授权归属隔离、续费与受次数和冷却时间限制的换绑
- 应用分组、管理员统一套餐、代理等级价格、预付余额及事务发卡成本流水
- 全业务 API 分类操作记录、拒绝请求查询、分页与导出
- 首页免登录代理验真，按公开代理编号与可售应用核对身份
- 可配置安全检测、人工核实处置、用户申诉，以及 Node.js 应用拦截页
- Go、Java、Node.js 客户端 SDK
- 独立开发者控制台，产品、授权和版本按开发者隔离
- 按应用提供授权接入指南及 Node.js、Go、Java 可运行接入包，包含 SDK 源码、配置和受保护接口 demo
- 18 位 AppID 与独立 AppSecret，支持服务端访问令牌、密钥重置和应用范围隔离
- 可选的产品升级服务：上传升级包、发布/下架版本，有效授权检查及下载更新
- 独立的使用期限与更新期限，支持订阅到期停用和永久使用、限期更新
- 单 Docker 镜像部署，内置 MySQL，仅需开放 Web 端口
- 首次安装私有令牌、可选的管理员两步验证、运行指标与容量管理、加密全量备份/恢复和通知重试

平台支持内部预付余额与发卡成本核算，余额由管理员登记。第三方支付、退款及分佣对账需业务系统另行接入。
代理采用统一用户账号加代理资格，一次开通即可用账号或邮箱登录，用户中心与代理工作台可直接切换。等级、套餐和余额在代理详情统一配置，多人协作为可选功能。张数额度为套餐的可选附加限制。流程见 [用户与代理账号](docs/agent-accounts.md)。

创建应用后，在管理员产品行点击“接入授权”，或直接在开发者产品页查看指南。页面会带入当前应用参数并提供接入包下载；不需要假定 SDK 已发布到公共包仓库。具体流程见 [开发者授权接入](docs/developer-integration.md)。

实施与验收范围见 [平台功能说明](docs/platform-implementation.md)，应用端拦截接入与能力边界见
[安全接入说明](docs/security-integration.md)。

## 版本与更新

发布版本维护在仓库根目录 `VERSION` 文件（唯一来源）。管理员登录后可在“系统设置”查看运行版本、构建信息、发布通道、
升级能力状态和结构化更新记录；这些内部信息不在公开查询页展示。

“系统设置”也提供全局站点品牌配置，可统一修改网站名称、浏览器标题、Logo 和网站图标。
配置保存在数据库中，容器重启或升级后仍会保留。

各端页面的“外观主题”支持浅色、深色和跟随系统，默认跟随系统并实时响应深浅变化。手动选择保存在当前浏览器，各入口和标签页同步使用。前台、管理员、代理、用户、开发者、安装与恢复页面均支持电脑及手机布局；手机后台底部导航可左右滑动，宽表格在表格区域内横向滚动。

完整迭代内容见 [CHANGELOG.md](CHANGELOG.md)，版本发布与未来在线升级设计见
[版本发布与升级](docs/release-management.md)。授权站自身仍采用备份后人工升级；被授权产品的在线升级服务已提供。

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
tar --no-same-owner -xzf yunnuo-license-v0.3.0-linux-amd64.tar.gz
cd yunnuo-license-v0.3.0-linux-amd64
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

API 接入示例见 [API Quickstart](docs/api/quickstart.md)，详细设计见
[系统设计](docs/system-design-v1.md)。
风控规则、告警阈值与数据处理方式见 [风控中心](docs/risk-control.md)。

## SDK

| 语言 | 目录 | 最低版本 |
| --- | --- | --- |
| Go | [`sdk/go`](https://github.com/yunnuo88520/yunnuo-license/tree/main/sdk/go) | Go 1.22 |
| Java | [`sdk/java`](https://github.com/yunnuo88520/yunnuo-license/tree/main/sdk/java) | Java 17 |
| Node.js | [`sdk/node`](https://github.com/yunnuo88520/yunnuo-license/tree/main/sdk/node) | Node.js 18 |

SDK 当前随源码提供，尚未发布到公共包仓库。各语言的初始化、在线验证和离线验签示例见对应
目录中的 README。

## 为产品提供升级服务

站点超级管理员在“系统”创建角色为“开发者”的账号。开发者在 `/developer-console/` 登录，
创建自己的产品并接入授权；也可由管理员在“产品升级”将已有产品分配给开发者。
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
