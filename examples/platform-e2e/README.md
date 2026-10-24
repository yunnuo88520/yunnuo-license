# 平台分阶段真实服务验收

在仓库根目录执行：

```bash
cd backend
go build -o /tmp/yunnuo-platform-server ./cmd/server
cd ../frontend
npm ci
npm run build
cd ..
python3 examples/platform-e2e/test_stage1.py \
  --server /tmp/yunnuo-platform-server --artifacts /tmp/yunnuo-platform/stage1
```

脚本自动创建临时数据库、随机测试账号和密钥，服务退出后删除临时数据。验证授权归属隔离、越权拒绝、续费、换绑幂等、旧绑定失效、冷却限制、默认禁止转移，以及分类日志和敏感信息排除。失败返回非零并将报告标为失败。

需要页面证据时安装 Python `playwright` 和 Chromium，再追加 `--screenshots`。浏览器默认使用 `/usr/bin/chromium`。截图来自真实页面，包含用户授权、失败提示、后台用户与操作日志；不要在真实站点使用这些测试数据。

已有隔离测试服务可传 `--base-url http://127.0.0.1:8080`，并通过 `YN_TEST_ADMIN_PASSWORD` 提供该测试服务管理员密码。Docker 验收脚本会自动使用此方式验证内置 MySQL；CI 自动运行不带截图的相同检查。
