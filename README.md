# 允诺云授权系统

[![CI](https://github.com/yunnuo88520/yunnuo-license/actions/workflows/ci.yml/badge.svg)](https://github.com/yunnuo88520/yunnuo-license/actions/workflows/ci.yml)
[![License](https://img.shields.io/github/license/yunnuo88520/yunnuo-license)](LICENSE)
[![Version](https://img.shields.io/badge/version-0.2.0-146447)](CHANGELOG.md)

允诺云授权系统（Yunnuo License）是一套可自托管的软件授权与卡密管理系统，面向桌面软件、
插件、私有化应用和 SaaS 产品。项目提供公开授权查询、管理后台、代理工作台、在线与离线
授权能力，以及 Go、Java、Node.js SDK。

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
- Go、Java、Node.js 客户端 SDK
- 单 Docker 镜像部署，内置 MySQL，仅需开放 Web 端口

本项目不包含支付、订单、退款、分佣或资金结算。代理额度仅代表可生成的卡密数量，
不代表账户余额。

## 版本与更新

当前版本为 `v0.2.0`。管理员登录后可在“系统设置”查看运行版本、构建信息、发布通道、
升级能力状态和结构化更新记录；这些内部信息不在公开查询页展示。

“系统设置”也提供全局站点品牌配置，可统一修改网站名称、浏览器标题、Logo 和网站图标。
配置保存在数据库中，容器重启或升级后仍会保留。

完整迭代内容见 [CHANGELOG.md](CHANGELOG.md)，版本发布与未来在线升级设计见
[版本发布与升级](docs/release-management.md)。当前版本尚未开放在线升级，避免在缺少升级包
签名验证、数据库备份和失败回滚时执行不安全的自更新。

## 快速部署

需要 Docker 24 或更高版本。

```bash
docker build -t yunnuo-license:latest .
docker volume create yunnuo-license-data

docker run -d \
  --name yunnuo-license \
  --restart unless-stopped \
  -p 8080:8080 \
  -v yunnuo-license-data:/var/lib/mysql \
  -e MYSQL_PASSWORD='请替换为数据库用户强密码' \
  -e MYSQL_ROOT_PASSWORD='请替换为数据库 root 强密码' \
  -e YN_ADMIN_USERNAME='admin' \
  -e YN_ADMIN_PASSWORD='请替换为管理员强密码' \
  -e YN_CARD_PEPPER='请替换为随机长字符串' \
  -e YN_DATA_KEY="$(openssl rand -hex 32)" \
  yunnuo-license:latest
```

启动后访问：

| 入口 | 地址 |
| --- | --- |
| 授权查询（无需登录） | <http://127.0.0.1:8080/> |
| 管理后台 | <http://127.0.0.1:8080/admin-console/> |
| 代理工作台 | <http://127.0.0.1:8080/agent-console/> |
| 健康检查 | <http://127.0.0.1:8080/healthz> |

公共授权查询支持许可证号、卡密、域名、IP、QQ、手机号和业务账号。绑定标识仅用于精确匹配，接口只返回脱敏后的查询值和授权摘要。

MySQL 只监听容器内部 `127.0.0.1:3306`，不会映射至宿主机。生产环境建议在服务前增加
HTTPS 反向代理，并妥善备份 Docker 数据卷。完整参数与升级说明见
[Docker 部署文档](docs/docker-deployment.md)。

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

首次打开 <http://127.0.0.1:8080> 会进入初始化向导，可选择 SQLite 或填写外部 MySQL DSN，并设置首个超级管理员账号和密码。初始化完成后才可进入管理后台；不会再强制使用默认管理员密码。
单镜像部署时，向导生成的数据库配置和 SQLite 文件会写入 `/var/lib/mysql` 数据卷，容器重启后继续生效；选择外部 MySQL 时只需确保容器可访问所填地址。
任何可被其他设备访问的环境都必须设置 `YN_CARD_PEPPER` 和 `YN_DATA_KEY`。也可以同时设置 `YN_ADMIN_USERNAME` 与 `YN_ADMIN_PASSWORD` 跳过向导并自动创建首个管理员。可参考 [.env.example](.env.example) 配置环境变量。已有部署也可以通过 `YN_DB_DRIVER` 与 `YN_DB` 直接指定数据库；环境变量优先于初始化向导生成的配置文件。

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
