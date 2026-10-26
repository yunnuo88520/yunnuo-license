# Linux 压缩包部署

本包已编译 Go 二进制和前端，无需安装 Go 编译器、Node.js 或 npm。
支持 Linux amd64/arm64；SQLite 内置，也可通过安装向导连接 MySQL 8（已验证 8.4）。

1. 按下载页的 `SHA256SUMS` 核对 tar.gz，解压到 Web 根目录之外。
2. 单机部署：`python3 deploy/native/configure.py --domain license.example.com --user www`。
   宝塔通常已有 `www` 用户；普通 Linux 可先创建服务用户，再使用对应 `--user`。
3. 持久目录默认在解压目录的同级 `yunnuo-license-data`，独立保存数据库、密钥、安装令牌和升级包。
   运行 `deploy/native/start.sh` 前，按配置输出设置 `YN_RUNTIME_FILE`（自定义持久目录时尤其需要）。
4. Nginx 站点根目录指向本包的 `frontend`。使用生成的 `nginx-server.conf`，或在宝塔现有 server 块中
   引用生成的 `security-headers.conf` 和 `nginx-locations.conf`。先检查 `nginx -t` 再重载。
5. 首次打开网站会强制进入 `/install`，从持久目录的 `database.json.install-token` 读取安装令牌。
   填写数据库及管理员账号，完成后显示各角色入口。
6. 用生成的 systemd 或 Supervisor 配置守护 Go 进程，二者选其一。

完整命令、宝塔步骤、两台机器部署、HTTPS 与升级回退见 `docs/native-deployment.md`。
前后端独立压缩包分别使用 `--backend-only` / `--frontend-only --upstream 后端私网IP:8080`。
默认 Nginx 分离模式需要 `http_auth_request_module`；缺少该模块时，单机完整包可使用 `--mode compatible`。

升级前备份持久目录及数据库。新版本放在新的解压目录，运行 configure.py 指向原 `--state-dir`，
工具保留数据库和密钥配置，只更新版本文件路径。先检查新配置，再停止旧进程、启动新版本并重载 Nginx。
数据库迁移失败时通过全量备份恢复数据库再回退二进制，不只替换旧程序。
