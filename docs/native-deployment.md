# Linux 压缩包：Nginx + Go / 宝塔部署

发布包内已经有编译后的 Go 程序、迁移文件、SDK 源码和前端文件，服务器不需要 Go 编译器、Node.js 或 npm。Go 程序监听私网/回环地址，Nginx 提供页面、静态文件并反代 API；前后端可在同机或两台机器上，浏览器使用同一网站域名。

## 下载和目录

在 GitHub Actions 的 **Release bundles → Run workflow** 构建发布包。推送与 `VERSION` 一致的 `v版本号` 标签会生成草稿 Release，核对后再公开发布。也可以从源码运行 `python3 scripts/build-release.py`，产物位于 `dist/`。

| 压缩包 | 用途 |
| --- | --- |
| `yunnuo-license-v版本-linux-amd64.tar.gz` | 常见 x86_64 Linux，前后端完整包 |
| `yunnuo-license-v版本-linux-arm64.tar.gz` | aarch64 Linux，前后端完整包 |
| `...-linux-架构-backend.tar.gz` | 独立 Go 后端，包含迁移和 SDK，不含前端 |
| `...-frontend.tar.gz` | 独立前端和 Nginx 配置工具，不含 Go 二进制 |
| `SHA256SUMS` | 核对下载文件是否完整 |

```bash
sha256sum --ignore-missing -c SHA256SUMS
umask 022
tar --no-same-owner -xzf yunnuo-license-v0.3.0-linux-amd64.tar.gz
cd yunnuo-license-v0.3.0-linux-amd64
```

版本号以实际下载文件为准。若从 Actions 下载的是外层 ZIP，先解压 ZIP 再解压其中的 tar.gz。

- `backend/`：三个可执行文件、`migrations/`、用于生成接入包的 `sdk/`。
- `frontend/`：Nginx 站点根目录；只有公开的 HTML、JS、CSS 和图片。
- `deploy/native/`：配置工具、启动脚本、Nginx 和进程守护模板。
- `licenses/`：随 Go 二进制再分发的依赖许可证。
- `BUILD_INFO.json`、`FILES.sha256`：源提交、平台、工具链和内部文件校验。

程序解压到持久目录，例如 `/opt/yunnuo/releases/版本/` 或 `/www/server/yunnuo/releases/版本/`。不要把整个发布目录作为 Nginx 站点根，也不要把数据库/密钥放进 `frontend/`。

## 单机 Nginx + Go

要求 Linux、Nginx、Python 3.8+（只用于生成配置）。SQLite 已内置；外部 MySQL 使用 8.x，验收采用 8.4，5.7 不受支持。需要用 MySQL 做备份/恢复时还应安装对应版本的 `mysqldump` 和 `mysql` 客户端。

普通 Linux 可先由管理员创建服务用户；宝塔通常已有 `www` 用户：

```bash
sudo useradd --system --home-dir /opt/yunnuo --shell /usr/sbin/nologin yunnuo
sudo python3 deploy/native/configure.py \
  --domain license.example.com \
  --state-dir /opt/yunnuo/shared \
  --user yunnuo
```

工具生成私有 `runtime.env`、Nginx、systemd 和 Supervisor 配置。首次启动不需要预设管理员密码或手填加密密钥：

```bash
sudo -u yunnuo env YN_RUNTIME_FILE=/opt/yunnuo/shared/runtime.env /bin/bash ./deploy/native/start.sh
```

默认 Go 监听 `127.0.0.1:8080`。在另一终端 `curl http://127.0.0.1:8080/readyz` 应返回 `ready: true`。这是临时前台启动检查，完成后按下面选择一种进程守护方式。

配置 Nginx：

```bash
sudo cp /opt/yunnuo/shared/deploy/nginx-server.conf /etc/nginx/conf.d/yunnuo-license.conf
sudo nginx -t
sudo systemctl reload nginx
```

默认分离模式要求 Nginx 有 `http_auth_request_module`，可通过 `nginx -V` 查看。Nginx 通过 Go 的 `/v1/setup/page` 查询应提供的固定页面；不缓存安装状态，自定义 `admin_path` 只需重启 Go 即可生效，不必逐个改 Nginx 路由。

## 宝塔面板操作

1. 上传完整 tar.gz，解压到 `/www/server/yunnuo/releases/版本目录/`，不要解压到可直接访问的站点根。
2. 在终端进入解压目录，执行：

   ```bash
   python3 deploy/native/configure.py \
     --domain license.example.com \
     --state-dir /www/server/yunnuo/shared \
     --user www
   ```

3. 在「网站」创建纯静态网站，绑定实际域名，站点根目录设置为输出中的发布目录下 `frontend/`。
4. 打开站点 Nginx 配置，保留宝塔生成的 `listen`、域名、证书和 `/.well-known/` 等配置。删除原有冲突的 `location /` 与整站反向代理规则，然后在同一 `server {}` 内加入：

   ```nginx
   include /www/server/yunnuo/shared/deploy/security-headers.conf;
   include /www/server/yunnuo/shared/deploy/nginx-locations.conf;
   ```

   文件中的路径以 configure.py 的实际输出为准。宝塔 SSL 配置仍由面板申请和维护，保存前先执行 `nginx -t`。无需 PHP，不要同时开启另一份整站代理。
5. 在宝塔 Supervisor 插件新建守护进程，或导入生成的 `supervisor.conf`。核心参数为：

   | 项目 | 值 |
   | --- | --- |
   | 启动命令 | `/usr/bin/env YN_RUNTIME_FILE=/www/server/yunnuo/shared/runtime.env /bin/bash /实际发布目录/deploy/native/start.sh` |
   | 运行目录 | `/实际发布目录/backend` |
   | 用户 | `www` |
   | 自动启动/自动重启 | 开启 |
   | 停止等待 | 90 秒 |
   | 日志 | `/www/server/yunnuo/shared/logs/server.log` |

   宝塔版本的界面名称可能不同，可直接采用输出中的 Supervisor 配置。使用 Go 项目管理器时也可选择 `backend/yunnuo-server`，运行目录必须是 `backend`，并导入同一 `runtime.env` 中的环境；推荐启动脚本避免漏变量。两种管理器不要同时启动同一端口。
6. 打开 `https://license.example.com`，首次会进入 `/install`。

若 Nginx 缺少 auth_request 模块，单机完整包可以重新生成兼容配置：

```bash
python3 deploy/native/configure.py \
  --state-dir /www/server/yunnuo/shared \
  --mode compatible
```

该模式由 Nginx 提供静态资源、Go 提供少量 HTML 并检查安装/后台入口。重启 Go、检查配置并重载 Nginx。使用此模式时，前端文件必须也在 Go 所在机器上。

## 首次安装与后台入口

安装令牌保存在持久目录的 `database.json.install-token`，权限 0600。由服务器管理员读取后填入安装页，不放入 URL 或公开日志：

```bash
cat /www/server/yunnuo/shared/database.json.install-token
```

向导可选择已内置的 SQLite，或者填写宝塔中创建的 MySQL 库名、用户、密码和地址（同机通常是 `127.0.0.1:3306`）。管理员用户名、密码在向导中设置。完成后显示管理员、代理、用户、开发者和前台链接。

默认后台页面 `/admin`。修改持久目录 `database.json` 中的 `admin_path` 后重启 Go，例如改为 `/private/control`；不要改动文件中的数据库和密钥字段。新入口随重启生效，旧 `/admin` 停止提供页面，管理 API 仍固定为 `/admin/*`。

## systemd（普通 Linux）

使用生成的服务文件：

```bash
sudo cp /opt/yunnuo/shared/deploy/yunnuo-license.service /etc/systemd/system/yunnuo-license.service
sudo systemctl daemon-reload
sudo systemctl enable --now yunnuo-license
sudo systemctl status yunnuo-license
sudo journalctl -u yunnuo-license -f
```

服务使用指定的非 root 用户、私有临时目录和 0077 文件掩码。运行数据目录必须可由该用户写入。现有目录迁移时请检查原有文件归属，工具不会改写数据库或轮换密钥。

## 两台机器部署

前端机器 Nginx 使用一个公开网站域名反代后端私网 IP；浏览器仍同域，API 无需开放跨域访问。

后端机器解压 `...-backend.tar.gz`：

```bash
python3 deploy/native/configure.py \
  --backend-only --state-dir /opt/yunnuo/shared --user yunnuo \
  --bind 10.0.0.20 --port 8080 \
  --trusted-proxy-cidrs 10.0.0.10/32
```

前端机器解压 `...-frontend.tar.gz`：

```bash
python3 deploy/native/configure.py \
  --frontend-only --state-dir /etc/yunnuo-frontend --user www \
  --domain license.example.com --upstream 10.0.0.20:8080
```

后端按生成的进程配置启动；前端按生成的 Nginx 配置启动。示例地址替换为实际私网地址，并让后端防火墙仅允许 Nginx 机器访问端口。初次安装仍通过公开网站 `/install`，令牌从后端持久目录读取。域名分离不是必需条件：前后端部署位置独立，浏览器通过 Nginx 保持一个站点域名。

## 升级、备份和回退

1. 使用旧版本的 `deploy/native/ops.sh backup --recipient age1... --output /私有目录/backup.age`，同时保留旧发布目录和私有环境配置。
2. 在新的版本目录解压新包，核对校验和。运行 configure.py 并指定**同一个** `--state-dir`。数据库、密钥和自定义设置会保留，只更新版本文件路径；旧 runtime.env 会保存私有备份。
3. 先验证生成的 Nginx 配置。停止旧 Go 进程，更新 systemd/Supervisor 指向的新版本启动脚本，重载进程管理器并启动；数据库会按版本迁移一次。
4. 检查 `/readyz`、登录、授权验证、更新下载和资金流水，再重载 Nginx 并切换流量。不要直接覆盖正在运行的程序或删除原持久目录。
5. 失败时从加密备份恢复到新目录/空白数据库，再用旧版本启动和切回 Nginx。已有结构迁移不能靠单独换回旧二进制撤销。

`ops.sh` 通过同一私有环境调用打包的 `yunnuo-ops`，避免漏掉实际数据库和加密密钥。异地调度及告警见 [商用部署与验收](production-readiness.md)。

## 常见问题

| 现象 | 检查 |
| --- | --- |
| 502 | Go 进程、监听地址、Nginx upstream、私网防火墙与 `/readyz` |
| 页面空白/JS 404 | 站点根是否为 `frontend/`；是否保留 `/assets/` 的配置 |
| `auth_request` unknown directive | Nginx 模块，或使用单机 `--mode compatible` |
| Nginx 403 | 公开前端及其父目录是否允许 Nginx 用户读取/遍历；私有数据目录应继续保持 0700 |
| 上传 413 | 使用提供的 257 MiB Nginx 上限；平台单包上限为 256 MiB |
| 修改后台路径不生效 | 是否改对持久配置文件并重启实际 Go 进程；页面路径与 `/admin/*` API 是两件事 |
| 重启后要求重新安装 | 是否误换了持久目录、数据库或配置文件路径 |
