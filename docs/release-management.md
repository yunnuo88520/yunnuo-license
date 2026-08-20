# 版本发布与升级

## 版本规则

项目采用语义化版本 `MAJOR.MINOR.PATCH`：

- `MAJOR`：包含不兼容的 API、配置或数据变更。
- `MINOR`：向后兼容的新功能。
- `PATCH`：向后兼容的问题修复和安全更新。

正式发布前必须同步修改：

1. `backend/internal/buildinfo/buildinfo.go` 中的默认版本和结构化更新记录。
2. `frontend/package.json` 与 `frontend/package-lock.json` 中的版本。
3. 根目录 `CHANGELOG.md` 和 `README.md` 中的当前版本。
4. `Dockerfile` 的默认 `APP_VERSION` 与 Docker 端到端断言。

## 镜像构建

正式镜像应注入版本、Git 提交和 UTC 构建时间：

```bash
docker build \
  --build-arg APP_VERSION=0.2.0 \
  --build-arg VCS_REF="$(git rev-parse HEAD)" \
  --build-arg BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -t yunnuo-license:0.2.0 .
```

这些值会同时进入后端版本接口和 OCI 镜像标签。前端显示以后端接口返回的运行版本为准。

## 发布检查

- 前端类型检查与生产构建通过。
- Go 后端及三种 SDK 测试通过。
- 单镜像端到端测试通过，且宿主机只暴露 Web 端口。
- 数据库迁移已在现有数据副本上验证。
- `CHANGELOG.md` 与页面结构化更新记录一致。
- 创建带签名的 Git 标签并保留对应镜像摘要。

## 在线升级边界

管理员认证接口 `GET /admin/system/version` 会暴露 `capabilities`，当前所有升级能力均为 `false`。实现在线升级时，
至少需要以下完整流程，不能只执行镜像替换或下载脚本：

1. 从受信任的发布清单读取最新版本、兼容范围和升级包摘要。
2. 使用内置发布公钥验证清单与升级包签名。
3. 检查磁盘空间、数据库版本、配置兼容性和当前任务状态。
4. 创建数据库与配置备份，并记录可恢复点。
5. 使用独立升级进程切换版本，主服务不能覆盖正在运行的自身文件。
6. 执行迁移和健康检查，失败时自动恢复程序、数据库和配置。
7. 在审计日志中记录检查、下载、安装、回滚和最终结果。

建议首个在线升级版本只支持管理员手动触发；自动检查可以先上线，但自动安装应在签名验证和
回滚流程经过多版本测试后再启用。
