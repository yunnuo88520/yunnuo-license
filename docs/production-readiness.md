# 授权站商用部署与验收

本轮交付以授权和被授权产品的更新服务为主线。首期收款继续采用管理员确认入账、代理预付余额；没有商户配置时不伪造在线支付、退款和分账能力。授权站自身仍采用经过备份的人工版本升级。

## 授权和更新规则

- `subscription`：使用期限到期后按应用配置的使用宽限处理，宽限结束拒绝使用；更新权限在更新期限或使用期限任一到期时停止。
- `perpetual`：既有版本永久使用，更新权益单独到期。永久卡可以把相应权益变为永久；已经永久的权益不会被普通续费卡缩短。
- 所有截止时间以服务器 UTC 为准，`now >= deadline` 即到期。使用宽限不会延长更新资格。续费从原截止时间和当前时间的较晚者起算。
- 续费卡在事务内与目标授权关联。同一卡、同授权及有效绑定重试返回当前权益，不重复延长；换成其他授权、其他归属或错误绑定则拒绝。历史已消费但没有关联记录的卡不重新消费。
- 激活、续费、换绑在同一授权聚合上串行；同卡激活只产生一份授权，最大绑定数量、换绑次数和冷却期必须在锁内检查。代理扣款、发卡及价格历史仍在同一事务中完成。
- 更新服务默认关闭。草稿不可见，发布后可检查和下载；下架、删除、应用停用、吊销、风控或安全处置立即影响后续请求。下载地址不是公开静态文件地址，不签发可绕过期限的长期下载票据。
- 检查、下载、下载文件哈希校验后及安装前均重新判断更新资格。安装前调用 `POST /v1/updates/{releaseID}/authorize`，字段同在线校验，成功返回 `authorized: true`；该响应只表示当次请求通过，不能缓存为未来安装的许可。Node SDK 下载后自动复验，安装器修改文件和数据库前再次调用 `authorizeUpdate`。
- 产品应用必须执行使用决定、安装前复验和失败回滚。移除客户端校验的破解程序、此前已经下载的文件不能由授权服务器远程收回；SDK 和在线接口共同提供执行点。

## 验收映射

| 范围 | 自动化证据 |
| --- | --- |
| 订阅/永久使用 × 无绑定/设备/域名/IPv4/IPv6/账号 × 更新、使用、宽限的到期前/恰好到期/到期后 | `backend/internal/service/authorization_matrix_test.go`，132 个时间边界子场景，校验、心跳、检查、下载及安装许可 |
| 未到期/已过期续费，永久卡、普通卡、重复重试、错误绑定 | 同文件的续费矩阵、`entitlements_test.go`、`customers_test.go` |
| 已检查后吊销/下架/关闭更新/停用应用/换绑定/跨应用访问 | 期限矩阵、`releases_test.go`、`security_test.go` |
| 真实网站升级、数据迁移、过期拒绝、故障回滚 | `examples/update-demo/test_e2e.py` 的 subscription/perpetual 两次执行 |
| 三种语言从页面下载接入包后编译运行，过期/吊销/断网等 | `examples/platform-e2e/test_integration.py`；各 SDK 测试 |
| 并发激活、三张卡累加 90 天、同卡竞争、续费重试、真实混合负载 | `test_production.py`，SQLite 和 Docker 内 MySQL 均执行 |
| 代理等级、套餐、扣款幂等、身份及产品隔离、换绑、恢复、分类日志、盗版处置 | `test_stage1.py` 至 `test_stage4.py`、`test_agent_accounts.py`、`test_agent_tiers.py`、`test_recovery_history.py` |
| 可信代理链/伪造转发头、限流和失败锁定一致、请求大小、安装令牌 | `production_security_test.go`、`production_operations_test.go`、`test_install.py` |
| 管理员动态码、一次性恢复码、重放拒绝、撤销旧会话 | `admin_mfa_test.go`、`test_production.py` 的页面验收 |
| 容量并发防超额、下架删除释放空间、已删除版本不可复用、临时文件清理 | `resources_test.go`、`test_production.py` |
| 数据库/审计写入/包存储故障，监控认证 | `production_operations_test.go`，故障触发 `/readyz` 503、`/healthz` 保持存活 |
| 通知持久化、失败重试、重启后投递、去重、完成后清除正文 | `outbox_test.go`、`expiry_reminders_test.go` |
| 一次迁移、历史校验和拒绝、迁移失败回滚、并发启动 | `store/migrations_test.go`；MySQL Docker 重启验收 |
| 加密备份、全新 SQLite 实例恢复、坏包/错密钥拒绝、日志归档失败不删除 | `maintenance/*_test.go` |
| 全新 MySQL 数据库与第二个进程恢复，期限/余额/迁移记录一致，继续下载并校验签名包 | `docker/test-production-restore.sh`、`docker/test-product-updates.py` |

执行命令：

```bash
go -C backend test ./...
go -C backend vet ./...
go -C backend test -race ./internal/service ./internal/httpapi ./internal/store ./internal/maintenance
npm --prefix frontend ci
npm --prefix frontend run check
npm --prefix frontend run check:sdk
npm --prefix frontend run build
go -C backend build -o /tmp/yunnuo-acceptance ./cmd/server
python3 examples/platform-e2e/test_production.py --server /tmp/yunnuo-acceptance --screenshots --artifacts /tmp/yunnuo-production
python3 examples/update-demo/test_e2e.py --license-server /tmp/yunnuo-acceptance --license-mode subscription --artifacts /tmp/yunnuo-subscription
python3 examples/update-demo/test_e2e.py --license-server /tmp/yunnuo-acceptance --license-mode perpetual --artifacts /tmp/yunnuo-perpetual
YN_DOCKER_ARTIFACTS=/tmp/yunnuo-mysql ./docker/test-image.sh yunnuo-license:test
```

截图、报告与混合请求的 P50/P95 随验收产出。混合场景使用 40 个受控测试客户端、并发 8、160 次请求，保留默认限流；这不是生产容量承诺。正式容量按硬件、包大小和业务频率复测。本版以单应用进程为部署基线；多实例部署应先把限流、失败锁定和文件协调迁移到共享入口/存储方案。

## 部署交付

Docker 日常启动使用根目录 `compose.yml`；HTTPS 和可选监控使用 `deploy/compose.production.yml`。Linux amd64/arm64 发布包同时提供完整包、独立后端包和前端包，包含 Nginx、systemd/Supervisor 生成工具。分别参见 [Docker Compose](docker-deployment.md) 和 [Nginx + Go / 宝塔](native-deployment.md)。分离部署保留私有安装令牌和后台入口重启生效。

## 公网入口和管理员

首次运行强制进入 `/install`。服务器会在数据库配置文件旁生成 `*.install-token`，权限 0600；安装时填入该令牌，再配置数据库和管理员账号。日志只显示文件路径。也可通过私有环境配置 `YN_SETUP_TOKEN`（32–256 字符），不要把令牌写在 URL、前端配置或公共日志中。完成后拒绝重复安装。

普通管理员登录只填写账号、密码，以及站点开启时的图形验证码。管理员可在「系统 → 账号与安全」为自己的账号选择开启两步验证，使用 TOTP 验证器的六位/30 秒验证码；密钥十分钟内完成绑定，成功后生成十个一次性备用恢复码并只展示一次，需离线保存。

已开启的账号在密码及图形验证码通过后才进入第二步，输入验证器验证码；无法使用验证器时主动切换到备用恢复码。第二步不重复提交密码或已消费的图形验证码，不在密码阶段签发登录会话。受限验证凭据有效五分钟，绑定可信客户端 IP、账号会话版本及当前 MFA 状态，不能用于管理 API；成功一次即失效，修改密码、关闭两步验证或其他成功 MFA 登录都会使原凭据失效。两步共享原有登录限流和失败锁定。启用、关闭都会使原会话失效；密码重置不会绕过已开启的两步验证。

生产参考配置在 `deploy/compose.production.yml`，Caddy 提供 HTTPS，仅公开 80/443。启动前设置私有环境文件中的数据库密码、域名、`YN_METRICS_TOKEN`；执行 `deploy/configure.sh` 生成监控配置，再在 Alertmanager 中设置实际获授权的接收端。

`YN_TRUST_PROXY_HEADERS` 默认 false；开启时必须设置精确的 `YN_TRUSTED_PROXY_CIDRS`，仅信任反向代理的真实连接地址。平台从右向左解析转发链，业务、限流、失败锁定和审计使用同一 IP。`YN_ADMIN_ALLOWED_CIDRS` 可把后台页面及管理 API 限定在 VPN/办公网。自定义管理入口仍通过私有数据库配置文件的 `admin_path` 修改，重启生效。

服务器提供 JSON 请求大小上限、上传并发和体积限制、读取/空闲超时、安全响应头、禁止业务响应缓存。NTP 必须保持同步；期限和动态码都以服务器时间执行。

## 运行监控和容量

- `/healthz` 是进程存活；`/readyz` 实测数据库连接、审计写入并回滚、升级目录写入与剩余空间。未安装时可就绪以提供安装页面，并返回 `installed: false`。
- `/metrics` 默认不存在；配置 `YN_METRICS_TOKEN` 后使用 Bearer 认证。指标只有固定业务分类，没有授权号、AppSecret 或用户标识标签。
- 后台「运行与容量」展示状态、包容量、请求/拒绝/服务异常及通知状态。长期指标由 Prometheus 保存，参考配置保留 30 天。
- 规则覆盖不可用、服务异常比例、P95、容量、通知重试、拒绝激增、时钟偏差、HTTPS 证书和备份过期。使用 node_exporter/blackbox_exporter 检测主机和公网端点。告警接收端默认空白，必须由运营方填入真实地址并进行告警及恢复演练。

| 环境变量 | 默认 |
| --- | --- |
| `YN_RELEASE_TOTAL_BYTES` | 16 GiB |
| `YN_RELEASE_DEVELOPER_BYTES` | 8 GiB |
| `YN_RELEASE_PRODUCT_BYTES` | 2 GiB |
| `YN_RELEASE_PRODUCT_VERSIONS` | 100 |
| `YN_RELEASE_CONCURRENT_UPLOADS` | 2 |
| `YN_RELEASE_CONCURRENT_DOWNLOADS` | 8 |
| `YN_RELEASE_MIN_FREE_BYTES` | 256 MiB |

单包仍为 256 MiB。下载并发独立限额防止文件句柄和哈希工作挤占授权流量，写响应的总时限为 15 分钟。数据库锁内核算平台/开发者/产品额度；HTTP 层在解析大文件前取得上传槽位并预留临时文件空间。发布版本不能删除，先下架再确认删除可释放容量；版本号不复用。每小时清理应用自有且超过 24 小时的临时文件，保留正常发布包和不明确的恢复副本。

通知在数据库中加密保存，失败指数退避，最多 12 次，租约超时可由重启后的工作进程继续发送；最终失败在后台由超级管理员重试。SMTP 只能实现至少一次投递，服务端接收邮件后进程崩溃仍可能重发；这不会重复扣款或发卡。密码找回码继续同步发送，不把过期验证码排队重发。

## 备份、恢复和日志保留

构建 `go -C backend build -o /opt/yunnuo/yunnuo-ops ./cmd/maintenance`，镜像内已自带 `yunnuo-ops`。

1. 在独立运维机器生成密钥：`yunnuo-ops keygen --output /secure/offline-identity`。输出的 `age1...` 是备份公钥；私钥文件始终留在运维机器，权限 0600。
2. 以与服务器相同的环境配置运行：`yunnuo-ops backup --recipient age1... --output /private/backups/license.age`。备份包含一致性数据库快照、所有保留版本的文件、实际生效的数据密钥/卡密盐/后台入口及 SHA-256 清单；环境变量覆盖的密钥也会保存。包发布、删除与备份互斥，备份也与结构迁移协调，授权业务可继续运行。包文件及目录先持久化再提交版本记录；备份和日志归档先持久化文件及目录再报告成功或删除旧日志。
3. SQLite 使用 `VACUUM INTO`，MySQL 使用 `mysqldump --single-transaction`；不复制活动数据库文件。数据库凭据经 0600 临时选项文件传入，不放在进程命令行。
4. 使用 `deploy/backup.sh` 和 `deploy/systemd/` 定时任务，每天上海时间 03:00 后随机延迟不超过 15 分钟。配置 `YN_BACKUP_DIR`、`YN_BACKUP_RECIPIENT`、`YN_BACKUP_REMOTE`、`YN_BACKUP_OPS`，通过私有 rclone 配置连接不同机器/账号的远端。脚本复制后下载比对字节，成功后才标记监控并清理超过 30 天的本地备份，不自动删除远端历史。
5. 从可信远端取回文件，在停止旧实例或隔离的全新环境执行：`yunnuo-ops restore --input license.age --identity /secure/offline-identity --destination /new/instance`。目标路径必须不存在；先解密和核对所有文件，再生成可运行配置。拒绝软链接、路径穿越、重复条目、超限解压、损坏和错密钥。
6. MySQL 额外传入 `--mysql-dsn-file /secure/new-db.dsn`（0600），指向完全空白的数据库。恢复过程不覆盖已有数据库；导入失败时该全新目标需由运维清理后重新建库。
7. 读取恢复目录中的 `runtime.env`，使 `YN_DB_CONFIG_FILE` 和 `YN_RELEASE_DIR` 指向恢复位置；清除旧实例的 `YN_DATA_KEY`/`YN_CARD_PEPPER` 环境覆盖，使用备份中的有效密钥。启动后核对管理员登录、原签名/授权、续费、更新包、代理余额和流水，再切换流量。备份还原了会话数据及签名密钥，应在隔离环境处理并按事件需要重置受影响的账号凭据。

日志默认保留在线记录，不静默自动删除。可调度 `yunnuo-ops archive-logs --table operation_logs --keep-days 365 --batch 1000 --recipient age1... --output /archive/operations.age`，同样支持 `audit_logs` 和 `email_logs`。归档为 age 加密的 gzip JSONL，首行包含表名、截止日期及记录数；文件成功落盘后才删除这批旧行。每次最多 5000 行，至少保留 30 天，归档应一并复制至异地。财务流水、授权归属和卡密记录不在清理范围内。

真实异地接收端、生产域名、SMTP、告警收件人和备份私钥都属于部署方配置；仓库提供实现、配置和隔离环境验证，不能把这些外部资源写成已经上线。至少每月在全新实例恢复一次并保存证据。

## 版本升级和交付

数据库通过 `schema_migrations` 记录文件名、SHA-256 和应用时间。已应用脚本不重复运行，修改历史脚本会拒绝启动；并发启动通过 SQLite 文件锁或 MySQL 命名锁协调。后续变更只能追加迁移。

更新前完成并验证加密全量备份，记录旧镜像 digest、应用版本及环境配置。新版本先在备份恢复的副本运行验收，再停止旧应用并启动新镜像，等待就绪和业务冒烟通过后切换流量。MySQL DDL 不保证整组事务回滚；失败时恢复备份到全新数据库并切回旧镜像，禁止仅降低二进制版本后继续使用已升级数据库。

CI 对 dev 和 main 执行后端/竞态检查、三语言 SDK、前端和真实浏览器、真实网站升级、MySQL 与全新实例恢复；增加 Go 漏洞检查、npm 高危审计、镜像可修复高危/严重漏洞门禁与 CycloneDX SBOM。构建支持 `APP_VERSION`、`VCS_REF`、`BUILD_TIME`；正式分发固定镜像 digest，保存 CI 证据、SBOM 与 SHA-256。镜像移除未使用的 MySQL Shell，并用当前 Go 重编译所需的 gosu，保留数据库、备份与恢复客户端。

平台服务条款、隐私说明、应用停用/更新权益、收款入账及退款规则应采用运营方的实际主体、联系渠道和商业约定，参考 `operating-policy-template.md`；不能把示例联系方式当作运营承诺。
