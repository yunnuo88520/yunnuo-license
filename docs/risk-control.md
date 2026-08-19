# 风控中心

风控中心为在线授权接口提供全局或产品级的 IP、设备指纹封禁，并记录可人工处置的异常行为告警。
该能力不依赖 Redis 或消息队列，SQLite 开发环境和 MySQL 单镜像部署使用相同业务规则。

## 黑名单

- `ip`：接受 IPv4 或 IPv6，写入前会标准化。
- `device`：接受客户端提交的完整设备绑定值。
- `product_id` 为空时对所有产品生效；指定产品时只影响该产品。
- 激活、验证、心跳、续期和解绑都会执行黑名单检查。
- 命中黑名单时返回 `RISK_IP_BLOCKED` 或 `RISK_DEVICE_BLOCKED`，同时聚合一条严重告警。

风控表只保存带 Pepper 的目标哈希和脱敏展示值，不保存完整 IP 或设备指纹。修改 `YN_CARD_PEPPER`
会导致已有黑名单无法匹配，因此生产升级时必须保持该配置不变。

## 异常告警

当前内置规则：

| 规则 | 默认级别 | 触发条件 |
| --- | --- | --- |
| 黑名单 IP 访问 | 严重 | 命中 IP 黑名单 |
| 黑名单设备访问 | 严重 | 命中设备黑名单 |
| 设备关联多授权 | 高风险 | 同一设备关联至少 3 个授权 |
| IP 高频激活 | 高风险 | 同一产品、同一 IP 在 10 分钟内成功激活至少 10 次 |
| 连续激活失败 | 中风险 | 同一产品、同一 IP 在 10 分钟内失败至少 5 次 |

相同产品、规则和目标产生的未处理告警会聚合到同一记录并增加发生次数。异常规则只告警，
不会自动封禁；管理员确认风险后再手动加入黑名单，以降低共享网络和批量部署场景的误伤概率。

## 客户端 IP

默认使用 TCP 连接的来源地址，并忽略客户端提交的转发头。只有服务部署在受信反向代理之后，且代理会覆盖
外部请求携带的 `X-Forwarded-For` 和 `X-Real-IP` 时，才应设置 `YN_TRUST_PROXY_HEADERS=true`。
直接暴露单镜像端口时必须保持默认值 `false`，避免客户端伪造来源地址绕过 IP 风控。

## 管理接口

所有接口都需要管理员 Bearer Token。审计员和运营人员可以查看，只有超级管理员和管理员可以处置。

```text
GET  /admin/risk/summary
GET  /admin/risk/blocks
POST /admin/risk/blocks
POST /admin/risk/blocks/{blockID}/disable
GET  /admin/risk/alerts
POST /admin/risk/alerts/{alertID}/resolve
```

创建全局 IP 封禁示例：

```json
{
  "kind": "ip",
  "value": "203.0.113.10",
  "reason": "confirmed automated abuse"
}
```
