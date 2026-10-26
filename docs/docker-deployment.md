# Docker Compose 部署

根目录 `compose.yml` 提供日常部署；`deploy/compose.production.yml` 增加 HTTPS，并可选启用监控。两种配置共用同一个明确命名的数据卷，保留数据库、加密密钥、安装配置和升级包。容器内包含 Go、全部前端页面、MySQL 8.4、维护命令和健康检查，宿主机不需要 Go、Node.js 或单独安装 MySQL。

## 1. 生成私有配置

要求 Docker Engine 24+、Compose V2，配置工具使用 Python 3.8+。从仓库根目录运行：

```bash
python3 scripts/configure-compose.py
```

生成权限为 0600 的 `.env`，数据库及监控密码随机生成且不打印；再次执行会保留已有凭据，只更新构建元数据。不要提交或遗失该文件。

默认 Web 端口只绑定 `127.0.0.1:8080`，适合宝塔/Nginx 反向代理。需要直接从局域网访问时明确改为：

```bash
python3 scripts/configure-compose.py --bind 0.0.0.0 --port 8080
```

也可按 `.env.example` 手动编辑；先把文件权限设为 0600，并填入真实密码。不要把示例密钥或每次随机生成的新密钥写进启动命令。

## 2. 启动与首次安装

```bash
docker compose up -d --build
docker compose ps
docker compose logs -f license
```

默认访问 `http://127.0.0.1:8080`。远程服务器使用已配置的 HTTPS 反向代理域名；选择 `--bind 0.0.0.0` 时可通过服务器 IP 和端口访问测试。

安装前读取私有安装令牌：

```bash
docker compose exec license cat /var/lib/mysql/.yunnuo/database.json.install-token
```

首次打开网站强制进入 `/install`。填入令牌、数据库配置和管理员账号：

| 项目 | 内置 MySQL 值 |
| --- | --- |
| 地址 / 端口 | `127.0.0.1` / `3306` |
| 数据库 | `.env` 的 `MYSQL_DATABASE`，默认 `yunnuo_license` |
| 用户 | `.env` 的 `MYSQL_USER`，默认 `yunnuo` |
| 数据库密码 | `.env` 的 `MYSQL_PASSWORD` |
| 管理员账号、密码 | 在向导中设置，环境变量不能跳过安装 |

MySQL 只在容器回环地址监听，不发布 3306 或 33060。安装后给出所有角色入口：前台 `/`、管理员 `/admin`、代理 `/agent-console/`、用户 `/user-console/`、开发者 `/developer-console/`。管理员可在安装之后为自己的账号选择开启两步验证；登录图形验证码由站点独立开关控制。

## 3. HTTPS 与可选监控

域名解析到服务器并允许 80/443 后：

```bash
python3 scripts/configure-compose.py --domain license.example.com
docker compose --env-file .env -f deploy/compose.production.yml up -d --build
```

Caddy 为该域名提供 HTTPS，Go 和 MySQL 不直接发布端口。已有宝塔/Nginx 时使用根目录 Compose 并反代 `127.0.0.1:8080` 即可，不要再让 Caddy 与其争用 80/443。

需要监控时：

```bash
docker compose --env-file .env -f deploy/compose.production.yml --profile monitoring up -d
```

监控默认不公开端口。实际告警接收端在 `deploy/monitoring/alertmanager.yml` 配置；启动监控不等于已经有人接收告警。配置和备份规则见 [商用部署与验收](production-readiness.md)。

配置工具默认将监控令牌、生成的探测目标和备份目录放在 `deploy/` 的独立子目录，均排除在源码和发布包之外。需要放到仓库外时使用 `--config-dir /opt/yunnuo/compose-config`；路径会写入私有 `.env`，再次生成时保留。

两种 Compose 配置均使用 `.env` 的 `YN_DATA_VOLUME`，默认 `yunnuo-license-data`。从旧部署迁移时先用 `docker volume ls` 核对原卷名；旧高级 Compose 的卷可能名为 `yunnuo-license_data`，须将 `YN_DATA_VOLUME` 设置为该实际名称，不能凭空换一个新卷。

## 宝塔/Nginx 反向代理 Compose

这是 Go 提供页面的容器部署方式；与 [Nginx 直接提供前端的传统分离部署](native-deployment.md) 不同。宝塔网站反代目标设置为 `http://127.0.0.1:8080`，上传限制至少 257 MiB，反代读取超时 900 秒，传入：

```nginx
proxy_set_header Host $host;
proxy_set_header X-Real-IP $remote_addr;
proxy_set_header X-Forwarded-For $remote_addr;
proxy_set_header X-Forwarded-Proto $scheme;
```

由于请求来自宿主机到容器，Go 看到的连接地址通常为该 Docker 网络网关而非 127.0.0.1。核对实际网关后，在 `.env` 中开启 `YN_TRUST_PROXY_HEADERS=true`，将 `YN_TRUSTED_PROXY_CIDRS` 设为该网关的精确 `/32` 或 `/128`；不要信任所有外部地址。修改后 `docker compose up -d` 重建容器使环境生效。

根目录 Compose 可这样查看网关（不输出密码）：

```bash
docker inspect --format '{{range .NetworkSettings.Networks}}{{.Gateway}}{{"\n"}}{{end}}' "$(docker compose ps -q license)"
```

## 常用命令与数据

```bash
docker compose ps
docker compose logs --tail 100 license
docker compose exec license yunnuo-healthcheck
docker compose restart license
```

`/healthz` 表示进程存活，`/readyz` 实测数据库、审计和包存储。健康检查不要求先安装，安装前返回 `installed: false`。

私有配置位于卷内 `/var/lib/mysql/.yunnuo/database.json`。更改后台入口时只修改该文件的 `admin_path`，例如 `/private/control`，然后重启 `license`；保留其中数据库和密钥字段。`/admin/*` API 不随页面入口改变。

## 备份、升级与回退

容器内有 `yunnuo-ops`：

```bash
docker compose exec license yunnuo-ops backup \
  --recipient age1这里填离线生成的公钥 \
  --output /var/lib/mysql/.yunnuo/before-upgrade.age
docker compose cp license:/var/lib/mysql/.yunnuo/before-upgrade.age ./before-upgrade.age
```

备份私钥保存在独立运维机器，复制出的文件是加密备份。完整恢复和异地备份步骤见 [商用部署与验收](production-readiness.md)。

升级前保留原 `.env`、数据卷和备份。更新源码后重新执行配置工具以更新构建提交信息，再运行原来使用的 Compose 命令：

```bash
git pull --ff-only
python3 scripts/configure-compose.py
docker compose up -d --build
docker compose exec license yunnuo-healthcheck
```

如果使用 HTTPS 文件，继续用该 `--env-file .env -f deploy/compose.production.yml` 命令，避免混用启动方式。不要运行 `down -v`，它会删除数据卷。单独更改 `.env` 的数据库密码不会修改已有 MySQL 账号密码；已有环境密钥也必须保留。

迁移失败需要先恢复完整数据库备份，再回到旧镜像，不能只降二进制版本。正式部署可以在 `YN_IMAGE` 中指定已核对的镜像 digest，原 `.env` 的密码与卷名保持不变。

## 镜像来源与验证

Dockerfile 支持覆盖 `GO_IMAGE`、`MYSQL_IMAGE`、`NODE_IMAGE` 构建参数，三者应使用可信来源。首次构建会下载依赖和编译前后端，无需在宿主机准备这些编译环境。

```bash
./docker/test-image.sh yunnuo-license:test
python3 examples/platform-e2e/test_compose_deployment.py \
  --image yunnuo-license:test --artifacts /tmp/yunnuo-compose-evidence
```

测试创建独立随机卷，检查配置、安装令牌、MySQL 端口隔离、真实授权及重启后期限/会话保留；不使用正式数据卷。
