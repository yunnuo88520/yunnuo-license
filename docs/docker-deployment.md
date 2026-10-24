# Docker 单镜像部署

该镜像内置以下组件：

- Go API 与三套前端静态页面
- MySQL 8.4
- 数据库初始化与健康检查

容器只需要向宿主机映射 Web 端口 `8080`。MySQL 仅监听容器内的
`127.0.0.1:3306`，不对外提供端口。

## 构建

```bash
docker build -t yunnuo-license:latest .
```

Dockerfile 默认从 Docker Hub 获取官方 Go 和 MySQL 基础镜像。如果当前网络无法
直接访问 Docker Hub，可以改用 DaoCloud 镜像源：

```bash
docker build \
  --build-arg GO_IMAGE=docker.m.daocloud.io/library/golang:1.26-bookworm \
  --build-arg MYSQL_IMAGE=docker.m.daocloud.io/library/mysql:8.4 \
  -t yunnuo-license:latest .
```

## 启动

```bash
docker volume create yunnuo-license-data

docker run -d \
  --name yunnuo-license \
  --restart unless-stopped \
  -p 8080:8080 \
  -v yunnuo-license-data:/var/lib/mysql \
  -e MYSQL_PASSWORD='替换为数据库用户强密码' \
  -e MYSQL_ROOT_PASSWORD='替换为数据库 root 强密码' \
  -e YN_CARD_PEPPER='替换为随机长字符串' \
  -e YN_DATA_KEY='替换为 64 位十六进制随机密钥' \
  yunnuo-license:latest
```

可使用以下命令生成数据加密密钥：

```bash
openssl rand -hex 32
```

首次运行访问任意页面会跳转 `/install`。选择内置 MySQL 时填写地址 `127.0.0.1`、端口 `3306`、库名 `yunnuo_license`、用户名 `yunnuo` 和启动时设置的 `MYSQL_PASSWORD`；管理员用户名与密码在向导中单独设置。完成安装后显示以下入口：

- 授权查询：`http://服务器地址:8080/`
- 管理端：`http://服务器地址:8080/admin`
- 代理端：`http://服务器地址:8080/agent-console/`

配置默认位于持久卷的 `/var/lib/mysql/.yunnuo/database.json`。编辑其中 `admin_path`（默认 `/admin`）后执行 `docker restart yunnuo-license`，新管理页面入口生效。请保留同一文件中的数据库和密钥字段，详细步骤见 [安装与账号恢复](installation-and-recovery.md)。

## 运维

查看健康状态：

```bash
docker inspect --format '{{.State.Health.Status}}' yunnuo-license
```

查看日志：

```bash
docker logs -f yunnuo-license
```

升级时只替换容器，继续挂载原数据卷。`MYSQL_PASSWORD`、`MYSQL_ROOT_PASSWORD`、
`YN_CARD_PEPPER` 和 `YN_DATA_KEY` 必须保持不变，否则已有数据将无法正常使用。

## 镜像测试

测试脚本会构建镜像、启动临时容器，验证页面、静态资源、管理员登录以及容器重启后的
MySQL 数据持久化，完成后自动清理临时容器和数据卷：

```bash
./docker/test-image.sh yunnuo-license:test
```
