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

第二阶段使用相同参数运行 `test_stage2.py`，验证管理员建立套餐和等级价、分组授权快照、不同代理数据隔离、报价及规格防篡改、发卡扣款幂等、并发防透支、卡密激活及撤销授权。追加 `--screenshots` 会真实操作后台授予套餐、登记余额，再登录代理工作台发卡并截图成本和资金流水。报告不包含密码、令牌或卡密。

Docker 测试在重启前运行第二阶段并保存结果，重启后通过 `--verify-state <report.json>` 核对余额、成本流水、卡密批次与撤销状态。

第三阶段 `test_stage3.py` 使用相同参数，验证匿名代理验真的应用范围与暂停状态、公开响应隐私、默认禁止归属转移、管理员开启后的转移、原归属失权和过时归属冲突。浏览器模式截图首页验真、暂停代理、后台归属转移和新用户中心。

第四阶段 `test_stage4.py` 还需要 Node.js 18+。脚本启动独立的 Ebox 演示网站，通过真实 SDK 篡改下载内容并验证拒绝安装和线索上报；再验证只禁更新仍可访问、人工确认后的实际拦截页、用户申诉及解除后的访问与下载恢复。截图模式会操作后台安全策略、核实和用户申诉页面。Docker 在重启后核对安全记录及申诉持久化。
