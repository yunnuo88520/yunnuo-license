# 平台分阶段真实服务验收

`test_agent_tiers.py` 使用相同参数，验证三个默认等级、管理员编辑名称/比例/排序、按比例结算及分位取整、专属价格优先级、旧报价拒绝、幂等重试保留历史成本、权限与删除限制。浏览器模式保存电脑和手机深浅色截图；CI 上传证据，Docker/MySQL 在重启后核对编辑配置和成交成本。后端另外覆盖默认等级只初始化一次及配置不被重启覆盖。

`test_agent_accounts.py` 使用相同的 `--server`、`--artifacts`、`--screenshots`、`--chromium` 参数，验证统一身份、一次开通、两端免重复登录、等级结算及发卡幂等、跨代理隔离、暂停不影响个人与已售授权、改密撤销两端会话。截图覆盖后台开通、统一配置、个人授权与手机深色工作台，前端 CI 上传证据。Docker 使用 `--base-url` 在 MySQL 执行，并在重启后通过 `--verify-state <report.json>` 核对用户关联、余额和流水。后端测试额外注入审计失败验证开通回滚，以及旧同名用户迁移不会误合并。

开发者接入验收：先构建前端和后端，再运行 `python3 examples/platform-e2e/test_integration.py --server <二进制> --artifacts <目录> --screenshots`。需要 Node.js 18+、Go 1.22+、Java 17+、Maven 3 与 Playwright。脚本从页面创建应用、下载三种 SDK 接入包，并实际编译运行；覆盖 HTTP 示例、业务接口拒绝路径、使用/更新期限分离与 Node 签名升级下载。使用 Playwright 自带浏览器可传 `--chromium bundled`。

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

已有隔离测试服务可传 `--base-url http://127.0.0.1:8080`，并通过 `YN_TEST_ADMIN_PASSWORD` 提供该测试服务管理员密码。Docker 验收脚本会自动使用此方式验证内置 MySQL；前端 CI 同时运行第一、三阶段的浏览器模式，验证卡密识别应用和应用选择流程，并上传截图。使用 Playwright 自带 Chromium 时传 `--chromium bundled`。

第二阶段使用相同参数运行 `test_stage2.py`，验证管理员建立套餐和等级价、分组授权快照、不同代理数据隔离、报价及规格防篡改、发卡扣款幂等、并发防透支、卡密激活及撤销授权。追加 `--screenshots` 会真实操作后台授予套餐、登记余额，再登录代理工作台发卡并截图成本和资金流水。报告不包含密码、令牌或卡密。

Docker 测试在重启前运行第二阶段并保存结果，重启后通过 `--verify-state <report.json>` 核对余额、成本流水、卡密批次与撤销状态。

第三阶段 `test_stage3.py` 使用相同参数，验证匿名代理验真的应用范围与暂停状态、公开响应隐私、默认禁止归属转移、管理员开启后的转移、原归属失权和过时归属冲突。浏览器模式截图首页验真、暂停代理、后台归属转移和新用户中心。

第四阶段 `test_stage4.py` 还需要 Node.js 18+。脚本启动独立的 Ebox 演示网站，通过真实 SDK 篡改下载内容并验证拒绝安装和线索上报；再验证只禁更新仍可访问、人工确认后的实际拦截页、用户申诉及解除后的访问与下载恢复。截图模式会操作后台安全策略、核实和用户申诉页面。Docker 在重启后核对安全记录及申诉持久化。

`test_install.py --server <二进制> --artifacts <目录>` 验证强制安装、环境管理员不能绕过安装、SQLite 安装与私有配置、禁止重装，以及修改 `admin_path` 后重启保持数据库、密钥和登录会话。追加 `--screenshots` 保存真实安装页面和新管理入口截图。

`test_recovery_history.py` 使用与阶段脚本相同的参数，验证恢复码一次性使用、密码重置后的用户与代理会话失效、恢复权限隔离、505 条真实资金调整的历史分页及完整导出、CSV 公式转义，以及日志不记录密码或恢复码。自行启动临时服务时还验证服务器本地管理员密码恢复；Docker 模式使用内置 MySQL 执行相同 HTTP 检查。

`test_appearance.py --server <二进制> --artifacts <目录> --screenshots` 使用真实 Chromium 验证七个入口的深浅主题、系统实时变化、手动优先、跨页及标签页同步、刷新恢复、Vue 加载前主题恢复和键盘操作。覆盖电脑、390px 与 320px 手机布局，以及各端登录表单和管理员功能导航；截图保存两种主题的电脑和手机页面。需要 Python `playwright==1.62.0`，默认使用 `/usr/bin/chromium`，安装 Playwright 自带浏览器时传 `--chromium bundled`。前端 CI 会运行并上传页面证据。
