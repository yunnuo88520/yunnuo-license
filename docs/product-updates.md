# 产品在线升级（V2）

升级服务按应用独立启用。只使用授权的开发者无需上传版本；启用后，可在应用的“在线升级”页面上传不可覆盖的草稿包，指定 x.y.z 版本、操作系统、架构、运行时、渠道、包类型及允许的来源版本，再发布或下架。

## 更新资格

使用与更新独立计时：永久使用可搭配 30/365 天更新；订阅到期可按应用策略给使用宽限，但**使用宽限不延长更新期限**。更新天数 0 表示没有更新资格。

检查、下载和安装前授权都会重新检查应用开关、许可证状态、使用及更新截止、安装签名、实时绑定规则、浮动租约、版本发布状态与目标平台。到截止时刻即拒绝；下载过包也不能在权益到期或版本下架后取得安装许可。安装器不得缓存某次许可用于未来升级，断网不能进入离线更新。

## 接口

三个接口都使用当前安装 Ed25519 私钥签名，格式见页面接入文档；每次生成新 nonce。

```json
{"app_id":"<AppID>","activation_id":"<激活返回值>","binding":{"type":"domain","value":"example.com"},"form":"web","current_version":"1.0.0","target":{"os":"linux","arch":"amd64","runtime":"python","channel":"stable","kind":"archive"}}
```

- `POST /v2/updates/check`：返回 available、匹配版本和 `manifest_jwt`。
- `POST /v2/updates/{release_id}/download`：返回文件流，响应头 `X-YN-Update-Manifest` 是本次签名清单。包和密钥均属于该 AppID。
- `POST /v2/updates/{release_id}/authorize`：修改产品文件/数据之前立即复验，返回新清单；拒绝则停止安装。

清单是标准 RS256 JWT，`token_use=update_manifest`。验证 iss/aud/exp、安装和绑定，再检查 release 中的 ID、版本、平台目标、大小、SHA-256、镜像身份。下载后和安装前都校验实际文件；不能只相信 HTTP 200、文件名或另一个应用的公钥。

## 生成项目与产品安装器

Python 接入包包含 `update.py`：设置 `YN_CURRENT_VERSION` 和 `YN_UPDATE_OS/ARCH/RUNTIME/CHANNEL/KIND`，依次运行 `check`、`download`、`authorize`。其他语言可按相同 HTTP 契约接入。生成器并不声称能为任意产品自动写出安全的数据库迁移或系统安装器。

产品升级流程：暂存下载 → 验签和哈希 → 备份客户数据和配置 → 最后在线复验 → 迁移数据库 → 原子切换版本 → 真实健康检查 → 成功提交；失败恢复文件与数据库。路径穿越、符号链接、重复文件和解压体积均需限制。示例安装器只处理示例自身的两个文件与三版 SQLite 模式，不能直接用于未知产品。

Docker 更新使用镜像归档，发布时绑定不可变镜像 ID。导入后核对 ID 再切换；保留客户环境文件、数据卷和安装身份，运行健康失败回滚到旧镜像。升级控制器或部署管理员执行 Docker，不把授权站或产品 Web 进程暴露为任意宿主机命令执行器。

GUI 使用暂存目录和独立更新器替换可执行文件；Windows/macOS 的签名、权限和正在运行文件约束由产品更新器处理。授权站提供相同的升级资格与包真实性契约。

## 验证

`python3 examples/update-demo/test_v2.py --server /path/to/server --artifacts /tmp/v2-update-evidence` 运行真实生成客户端、真实 HTTP 下载、订阅/永久期限、下载后到期拒绝安装、文件篡改、SQLite 保留数据迁移与回滚。授权站自身部署升级仍先备份再人工发布，见部署指南。
