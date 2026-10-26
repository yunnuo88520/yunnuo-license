# 独立网站在线升级 Demo

这个 Demo 启动两个真实服务：允诺授权后端，以及独立的 Ebox 示例网站。它使用当前 Node.js SDK，
从授权站检查版本、验证授权、下载并验签，然后备份数据库、迁移数据库及切换网站文件。
下载完成后及实际安装前会重新验证当前版本的更新资格，到期、吊销或下架时中止安装。
新版健康检查失败时恢复原网站和数据库。全部测试数据位于临时目录，不使用已有站点或生产数据。

需要 Go 1.26、Node.js 18+ 和 Python 3。仓库根目录运行：

```bash
(cd backend && go build -o /tmp/yunnuo-demo-server ./cmd/server)
python3 examples/update-demo/test_e2e.py \
  --license-server /tmp/yunnuo-demo-server \
  --artifacts /tmp/yunnuo-demo-results

python3 examples/update-demo/test_e2e.py \
  --license-mode perpetual \
  --license-server /tmp/yunnuo-demo-server \
  --artifacts /tmp/yunnuo-demo-perpetual-results
```

安装了 Python Playwright 和 Chromium 时，加 `--screenshots` 可通过浏览器点击更新并生成前后截图。
报告写入指定目录的 `report.json`。脚本使用随机端口和随机临时凭据，结束时关闭它启动的服务并删除临时配置及数据库。

验证内容：

1. 更新到期后拒绝更新，网站文件仍为 v1.0.0。订阅模式同时拒绝验证、心跳、网站页面与业务数据访问；永久使用模式继续允许验证、心跳和网站访问（仅修改隔离测试数据库中的截止时间）。
2. 恢复有效授权后，网站由 v1.0.0 实际升级至 v2.0.0，HTML 文件摘要发生变化。
3. 数据库从 Schema 1 迁移至 Schema 2，已有客户文章保留。
4. 重启示例网站，版本和客户数据继续保留。
5. 发布健康检查失败的 v2.1.0，恢复 v2.0.0 网站文件和 Schema 2 数据库，撤销本次 Schema 3 变更。
6. 下架故障版本后，当前站点显示无更新。

安装器只接受两个指定文件，限制解压大小，拒绝重复文件和符号链接，且仅支持此 Demo 的三种固定数据库结构。
示例监听回环地址，没有公网部署。它演示产品方如何接入；不是适用于任意网站、任意数据库迁移的通用生产升级器。
生产产品仍需设计自己的权限、维护模式、进程切换、数据备份及回滚策略。
