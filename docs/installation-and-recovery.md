# 安装、入口配置与账号恢复

## 首次安装

构建前端并启动服务后，访问任意业务页面会跳转 `/install`。安装前业务 API 返回 `503 INSTALL_REQUIRED`，健康检查和安装资源仍可用；环境变量中的管理员用户名与密码不再自动创建账号。

向导支持 SQLite 文件路径，或 MySQL 地址、端口、库名、用户名及密码。数据库应已创建且账号具有建表权限。连接失败不会完成安装，可修正后重试。管理员用户名最多 80 字节，显示名称最多 160 字节，密码 8 至 256 字节。

安装完成显示管理后台、代理后台、前台首页、用户中心和开发者控制台链接。再次访问 `/install` 仅显示入口，初始化 API 拒绝重复安装，不覆盖已有管理员。

配置文件由 `YN_DB_CONFIG_FILE` 指定。本地默认 `backend/yn-license-config.json`；Docker 默认 `/var/lib/mysql/.yunnuo/database.json`。它包含数据库连接、管理页面路径及自动生成的密钥，权限为 `0600`，不保存管理员明文密码。新安装未显式提供环境密钥时生成随机密钥；已有安装沿用原密钥。数据库配置写入后优先于 `YN_DB_DRIVER` 和 `YN_DB`，避免启动环境把已安装平台切回临时数据库。备份时须保留配置文件、数据库及升级包目录。

## 修改管理页面入口

在已有 JSON 文件中仅修改 `admin_path`，保留其他字段，例如：

```json
"admin_path": "/company-panel"
```

重启服务后，管理页面改为 `/company-panel`。默认值为 `/admin`。路径支持字母、数字、下划线、横线及多级目录；保留路由冲突会导致启动失败并给出配置错误。原管理页面入口返回 404。后台 API 继续使用 `/admin/*`，客户端无需修改接口地址。首页和安装完成页会读取配置并展示新入口。

Docker 可复制配置到本地编辑，再复制回同一位置并重启：

```bash
docker cp yunnuo-license:/var/lib/mysql/.yunnuo/database.json ./yunnuo-config.json
# 编辑 admin_path，保留全部数据库和密钥字段；不要提交此私有文件。
docker cp ./yunnuo-config.json yunnuo-license:/var/lib/mysql/.yunnuo/database.json
docker exec yunnuo-license chmod 600 /var/lib/mysql/.yunnuo/database.json
docker restart yunnuo-license
```

## 忘记密码

各端登录页提供 `/recover` 链接。平台管理员应先核实持有人身份，再在“账号恢复”选择普通用户、代理账号或管理员与开发者，填写原因并生成一次性恢复码。默认有效 30 分钟，可设 5 至 1440 分钟。恢复码只在创建后返回，数据库仅保存其摘要，新建恢复码使此前未使用的码失效。

持有人在恢复页输入恢复码和新密码。到期时间一到即拒绝，重复提交只能成功一次；账号禁用或密码已被修改时也拒绝。重置成功会撤销旧会话。管理员也可直接设置新密码，所有操作记录原因。普通管理员仅可恢复用户、代理和开发者，超级管理员才可重置平台管理员。

管理员全部忘记密码时，拥有服务器和配置文件访问权限的运维人员可执行本地命令。密码从标准输入读取，不通过命令行参数传递：

```bash
read -r -s -p '新的管理员密码: ' recovery_password
printf '\n'
printf '%s\n' "$recovery_password" | docker exec -i -w /opt/yunnuo/backend \
  yunnuo-license /usr/local/bin/yunnuo-server \
  --reset-admin admin --reason '服务器持有人核实后恢复'
unset recovery_password
```

本地二进制部署在后端运行目录执行 `./yunnuo-server --reset-admin admin --reason '恢复原因'`，通过标准输入提供一行新密码。须使用与运行服务相同的数据库配置和密钥环境。命令只能重置已有账号，不启用禁用账号，不创建管理员，记录 `local-console` 审计并撤销旧会话。

## 历史记录

代理资金流水、后台安全处置和申诉、用户自己的安全记录及申诉支持分页、编号或说明搜索、类型、状态和时间筛选，CSV 导出覆盖全部匹配记录。时间区间包含开始、不含结束。代理身份和用户归属由服务端会话确定，传入其他身份编号不能跨权限读取数据；用户看不到内部证据和核实说明。CSV 对自由文本中的电子表格公式前缀做转义。
