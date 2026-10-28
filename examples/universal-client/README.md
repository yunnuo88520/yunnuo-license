> 历史 V1 资料，供存档参考。当前生产授权接口已切换 V2，请使用后台“应用授权中心 → 接入文档”和 [当前接入指南](../../docs/developer-integration.md)，不要使用本文旧接口或 SDK 接入新应用。

# 同一产品的 CLI、GUI 和 Docker 交付示例

此目录演示客户购买后在产品内激活，同一程序和公开配置支持不同套餐。实际授权、签名、下载和期限检查由 Go SDK 与真实授权站完成，GUI 不伪造检查结果。

```bash
go build -o product-client .
./product-client --config product.json --data ./private-data activate CARD_CODE
./product-client --config product.json --data ./private-data verify
./product-client --config product.json --data ./private-data claim
python3 gui.py --client ./product-client --config product.json --data ./private-data
```

product.json 使用控制台接入包的 config.json，含 base_url、app_key、product_id、product_code、public_keys。设备／实例模式自动建立持久标识；域名、IP、租户或组合规则在激活时追加绑定参数。泛域校验使用 --host 实际域名；CIDR 使用 --ip 实际部署地址。完整 CLI 返回字段见源码，不把敏感结果输出到公共日志。

machine-code 返回设备提供器的设备标识；管理员可据此签发完全离线文件。import-offline FILE.key 验签后私有保存，verify-file 可在完全断网时重复验证。错误设备、错误产品、过期、坏签名和时间倒拨均拒绝；离线文件不能取得在线升级资格。GUI 提供文件导入入口。

## 真实 GUI 更新

公开配置 target 指明操作系统、架构、runtime=native、channel=stable、kind=installer。产品升级包为 ZIP，严格包含 product-client（Windows 名称 product-client.exe）及 release.json，后者包含 version、healthy。使用 -ldflags '-X main.bundledVersion=2.0.0' 编译新版，签名版本由授权站上传发布后提供。

GUI 下载、验证签名与哈希，安装前复核当前资格，备份并替换产品核心程序，再执行新版的真实授权和版本健康检查；失败恢复旧程序。客户数据和实例身份保存在独立目录。此参考安装器只针对这个示例的文件布局，其他产品自行实现迁移和安装规范。

## Docker 镜像归档交付

```bash
CGO_ENABLED=0 go build -o product-client .
cp /etc/ssl/certs/ca-certificates.crt .
docker build -t vendor/ebox:1.0.0 .
# 配置 product.json 后：
docker compose up -d
docker compose run --rm product activate CARD_CODE
```

服务进程启动前可能尚未激活，/healthz 只检查进程；/protected 在未激活时拒绝业务。容器使用非 root 身份，/data 命名卷保存随机实例、授权状态与客户数据。删除重建容器会恢复原身份，不新增授权数量。设备 ID 缺省是持久逻辑标识；需要硬件指纹时由可信部署环境提供，不要用临时容器 ID 或 Pod IP。

新版使用 docker save vendor/ebox:2.0.0 | gzip > image.tar.gz，上传为 container_archive。target 附 os、arch、runtime=docker、channel、kind、image_tag，以及 docker image inspect 返回的 sha256 镜像 ID。公开 product.json 的 image_repository 限定为 vendor/ebox。

```bash
python3 container_update.py --container ebox --volume ebox-data \
  --config product.json --staging ./private-staging --port 8080
```

该命令需产品运维用户具有 Docker 权限；不在客户网站暴露 Docker socket。它检查当前容器的 yunnuo.product 标签与应用一致，通过当前镜像调用授权客户端获取签名归档，只接受配置仓库中的单个镜像；复核资格、加载、核对镜像 ID，再次授权后切换。健康失败恢复旧容器和原卷。调用时容器应按示例 updater.start_product 的固定配置创建；不是任意 Compose/Kubernetes 服务的通用部署器。这里使用签名镜像归档，不是 OCI registry。

可加 --stage-only 暂存、--install-staged 稍后安装；安装前资格到期必须拒绝。真实运行验证见 examples/platform-e2e/test_scenarios.py，覆盖容器重建、客户数据保留、升级成功、故障回滚及下载后到期。

Node.js 的 node-driver.mjs 只供跨 SDK 协议验收，产品集成应使用 sdk/node RuntimeLicense。平台与绑定设计见 [授权场景说明](../../docs/licensing-scenarios.md)。
