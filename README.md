# 户外轨迹 App - 服务端

基于 Hertz 框架的 Go 服务端项目。

## Docker 构建与推送指南

### 前置准备

1. 安装 Docker
2. 准备阿里云镜像仓库信息：
   - Registry 地址
   - 命名空间
   - 仓库名称
   - 用户名/密码

---

## 方式一：单架构构建（适用于mac本地）

### 1. 构建镜像
```bash
# 使用 Makefile
make docker-build

# 或直接使用 docker
docker build -t hertz-track:latest -f deploy/Dockerfile .
```

### 2. 为镜像打标签
```bash
# 格式：docker tag <本地镜像> <Registry地址>/<命名空间>/<仓库名>:<标签>
docker tag hertz-track:latest crpi-p78v4agazv8zn80d.cn-beijing.personal.cr.aliyuncs.com/track_server/track_server_go:latest
```

### 3. 登录阿里云镜像仓库
```bash
docker login --username <用户名> --password <密码> <Registry地址>

# 示例
docker login --username 359309156@qq.com --password code7app.org crpi-p78v4agazv8zn80d.cn-beijing.personal.cr.aliyuncs.com
```

### 4. 推送镜像
```bash
docker push crpi-p78v4agazv8zn80d.cn-beijing.personal.cr.aliyuncs.com/track_server/track_server_go:latest
```

---

## 方式二：跨平台构建（linux服务器）

使用 Docker Buildx 构建 linux/amd64 镜像（适用于在 Mac M1/M2/M3 上构建 Linux 服务器可用的镜像）。

### 1. 创建并使用 buildx 构建器
```bash
docker buildx create --use --name multiarch-builder
```

### 2. 构建并直接推送（一步完成）
```bash
# 格式：docker buildx build --platform <目标平台> -t <完整镜像名> -f <Dockerfile> . --push

docker buildx build --platform linux/amd64 -t crpi-p78v4agazv8zn80d.cn-beijing.personal.cr.aliyuncs.com/track_server/track_server_go:latest -f deploy/Dockerfile . --push
```

---

## 服务器上拉取与运行

### 拉取镜像
```bash
docker pull crpi-p78v4agazv8zn80d.cn-beijing.personal.cr.aliyuncs.com/track_server/track_server_go:latest
```

### 停止旧进程->删除旧的本地镜像


### 方式 A：只用 `docker run`（服务器无需拷贝仓库文件）

说明：该服务默认依赖 mysql。mysql提前安装好，或者购买云数据库。登录mysql: mysql -u root -p

```bash
# 启动 API（只需拉取你的业务镜像）
docker run -d --name track_server_go -p 80:8080 -e 'MYSQL_DSN=root:track_server_6509HbK@tcp(172.18.0.1:3306)/track_db?charset=utf8mb4&parseTime=True' -e SERVER_ADDR=:8080 -e LOG_DIR=/var/log/track_server -v /var/log/track_server:/var/log/track_server 47947fd68507 

# 进入docker容器
docker exec -it track_server_go /bin/bash

# （可选）如果你更习惯 .env 文件，也可以把它放在服务器任意路径：
# docker run ... --env-file /opt/track_server/track_server.env <image>
```

### 使用 docker-compose 运行完整栈（API + MongoDB）
```bash
# 启动
make compose-up

# 停止
make compose-down
```

---

## 镜像说明

- **基础镜像**: debian:12-slim（运行时）
- **暴露端口**: 8080
- **环境变量**:
  - `SERVER_ADDR`: 服务监听地址，默认 `:8080`
  - `MONGO_URI`: MongoDB 连接地址
  - `MYSQL_DSN`: MySQL DSN（设置后优先使用 MySQL）
  - `JWT_SECRET`: JWT 签名密钥（生产务必修改）

---

## Makefile 快捷命令

| 命令 | 说明 |
|------|------|
| `make run` | 本地运行服务 |
| `make test` | 运行所有测试 |
| `make docker-build` | 构建 Docker 镜像 |
| `make docker-run` | 运行单个容器 |
| `make compose-up` | 启动 API + MongoDB 栈 |
| `make compose-down` | 停止并移除栈 |

---

## Docker 部署启用 HTTPS

项目已支持在 Go 应用层通过 Hertz `server.WithTLS` 直接处理 TLS，只需通过环境变量 + 证书文件挂载即可启用。

### 原理

```
┌─── 宿主机 ─────────────────────────────────────┐
│                                                 │
│  deploy/certs/server.crt ──┐                    │
│  deploy/certs/server.key ──┤  volume mount (ro) │
│                            │                    │
│  deploy/.env ──────────────┤  环境变量注入       │
│    TLS_CERT_FILE=...       │                    │
│    TLS_KEY_FILE=...        ▼                    │
│  ┌─── Docker 容器 ────────────────────────┐     │
│  │  /app/certs/server.crt  (只读挂载)     │     │
│  │  /app/certs/server.key  (只读挂载)     │     │
│  │                                        │     │
│  │  Go 程序读取环境变量 → 加载证书/密钥    │     │
│  │  server.WithTLS() → 监听 HTTPS :8443   │     │
│  └────────────────────────────────────────┘     │
│                                                 │
│  宿主机 :8443 ←── 端口映射 ──→ 容器 :8443       │
└─────────────────────────────────────────────────┘
```

### 步骤

#### 1. 准备证书文件

将 SSL 证书和私钥放到 `deploy/certs/` 目录下：

```
deploy/
├── certs/
│   ├── server.crt    # 证书文件
│   └── server.key    # 私钥文件
├── docker-compose.yml
├── .env
└── ...
```

> 如需自签名证书用于测试：
> ```bash
> mkdir -p deploy/certs
> openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 \
>   -keyout deploy/certs/server.key -out deploy/certs/server.crt \
>   -days 365 -nodes -subj "/CN=localhost"
> ```

#### 2. 修改 docker-compose.yml

打开 `deploy/docker-compose.yml`，取消 TLS 相关的注释：

```yaml
    ports:
      - "${APP_PORT:-8080}:8080"
      - "${APP_TLS_PORT:-8443}:8443"    # 取消注释
    volumes:
      - ./certs:/app/certs:ro            # 取消注释
```

#### 3. 配置 .env

编辑 `deploy/.env`，设置 TLS 相关变量：

```bash
# 监听地址改为 8443
BIND_ADDR=:8443

# 指向容器内的证书路径（不是宿主机路径）
TLS_CERT_FILE=/app/certs/server.crt
TLS_KEY_FILE=/app/certs/server.key
```

#### 4. 启动服务

```bash
cd deploy
docker compose up -d --build
```

启动后通过 `https://<服务器IP>:8443` 即可访问。

### 环境变量说明

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `TLS_CERT_FILE` | TLS 证书文件路径（容器内路径） | 空（不启用 TLS） |
| `TLS_KEY_FILE` | TLS 私钥文件路径（容器内路径） | 空（不启用 TLS） |
| `BIND_ADDR` | 容器内监听地址 | `:8080` |
| `APP_TLS_PORT` | 宿主机 HTTPS 映射端口 | `8443` |

> **注意**: 当 `TLS_CERT_FILE` 和 `TLS_KEY_FILE` 都为空时，服务以普通 HTTP 模式启动，行为与之前完全一致。

---

## 使用 Let's Encrypt 免费证书（生产推荐）

[Let's Encrypt](https://letsencrypt.org/) 提供免费、自动化的 SSL 证书，有效期 90 天，可自动续期。以下是在 Linux 服务器上从零开始的完整操作流程。

### 前置条件

| 条件 | 说明 |
|------|------|
| 一个域名 | 如 `api.example.com`，已将 DNS A 记录指向你的服务器公网 IP |
| 服务器 80 端口可访问 | Let's Encrypt 通过 HTTP-01 验证域名所有权，需要从公网访问你的 80 端口 |
| 服务器有 root/sudo 权限 | 安装 certbot 和操作证书目录需要 |

> ⚠️ **重要**：申请证书前，必须确保域名已经解析到服务器 IP，且 80 端口没有被防火墙/安全组拦截。

### 一、首次申请证书（初始化）

#### 1. 安装 Certbot

```bash
# Ubuntu / Debian
sudo apt update
sudo apt install -y certbot

# CentOS / RHEL
sudo yum install -y epel-release
sudo yum install -y certbot

# 验证安装
certbot --version
```

#### 2. 申请证书

使用 **standalone 模式**（certbot 临时启动一个 HTTP 服务来完成验证）：

```bash
# 申请前需要先停掉占用 80 端口的服务
# 如果你的 docker 容器映射了 80 端口，先停掉：
docker compose -f deploy/docker-compose.yml down

# 申请证书（把 api.example.com 替换成你的真实域名）
sudo certbot certonly --standalone -d api.example.com
```

按提示输入邮箱（用于接收到期提醒），同意协议即可。

申请成功后，证书文件保存在：

```
/etc/letsencrypt/live/api.example.com/
├── fullchain.pem   # 证书文件（包含完整证书链）
├── privkey.pem     # 私钥文件
├── cert.pem        # 仅服务器证书
└── chain.pem       # 中间证书
```

> 我们需要的是 `fullchain.pem`（证书）和 `privkey.pem`（私钥）。

#### 3. 复制证书到项目目录

```bash
# 创建证书目录
sudo mkdir -p /opt/track_server/certs

# 复制证书（Let's Encrypt 原始文件是符号链接，用 -L 跟随链接复制实际文件）
sudo cp -L /etc/letsencrypt/live/api.example.com/fullchain.pem /opt/track_server/certs/server.crt
sudo cp -L /etc/letsencrypt/live/api.example.com/privkey.pem   /opt/track_server/certs/server.key

# 确保容器能读取（设置合适的权限）
sudo chmod 644 /opt/track_server/certs/server.crt
sudo chmod 600 /opt/track_server/certs/server.key
```

#### 4. 配置 docker-compose.yml

取消 `deploy/docker-compose.yml` 中 TLS 相关的注释，并将证书卷指向服务器上的证书路径：

```yaml
    ports:
      - "80:8080"           # HTTP（可保留，也可去掉）
      - "443:8443"          # HTTPS，宿主机 443 映射到容器 8443
    volumes:
      - /opt/track_server/certs:/app/certs:ro   # 挂载服务器上的证书目录
```

#### 5. 配置 .env

编辑 `deploy/.env`：

```bash
BIND_ADDR=:8443
TLS_CERT_FILE=/app/certs/server.crt
TLS_KEY_FILE=/app/certs/server.key
```

#### 6. 启动服务

```bash
cd deploy
docker compose up -d --build
```

验证：

```bash
# 测试 HTTPS 是否正常（把域名替换成你的）
curl -v https://api.example.com/api/v1/ping
```

---

### 二、证书自动续期（常态运维）

Let's Encrypt 证书有效期只有 **90 天**，所以必须配置自动续期。

#### 方案：Cron 定时任务自动续期 + 重启容器

创建一个续期脚本：

```bash
sudo tee /opt/track_server/renew-cert.sh > /dev/null << 'EOF'
#!/bin/bash
set -e

DOMAIN="api.example.com"
CERT_DIR="/opt/track_server/certs"
COMPOSE_DIR="/opt/track_server/deploy"

# 1. 停掉容器释放 443/80 端口（certbot standalone 需要 80）
docker compose -f ${COMPOSE_DIR}/docker-compose.yml down

# 2. 尝试续期
certbot renew --standalone

# 3. 复制新证书
cp -L /etc/letsencrypt/live/${DOMAIN}/fullchain.pem ${CERT_DIR}/server.crt
cp -L /etc/letsencrypt/live/${DOMAIN}/privkey.pem   ${CERT_DIR}/server.key
chmod 644 ${CERT_DIR}/server.crt
chmod 600 ${CERT_DIR}/server.key

# 4. 重新启动容器
docker compose -f ${COMPOSE_DIR}/docker-compose.yml up -d

echo "[$(date)] certificate renewed and service restarted" >> /var/log/cert-renew.log
EOF

sudo chmod +x /opt/track_server/renew-cert.sh
```

添加 cron 定时任务（每月 1 号和 15 号凌晨 3 点执行）：

```bash
sudo crontab -e
```

添加一行：

```cron
0 3 1,15 * * /opt/track_server/renew-cert.sh >> /var/log/cert-renew.log 2>&1
```

> **为什么每月两次？** Let's Encrypt 推荐在到期前 30 天续期。每月两次确保在 90 天有效期内有足够的续期机会，即使某次失败也还有重试时间。

#### 手动测试续期

```bash
# 模拟续期（不会真正续期，仅测试流程是否正常）
sudo certbot renew --dry-run
```

---

### 三、常见问题与注意事项

#### Q1: 申请证书时报错 "Could not bind to port 80"

80 端口被其他进程占用了。先停掉占用 80 端口的服务：

```bash
# 查看谁在占用 80
sudo lsof -i :80

# 如果是 docker 容器
docker compose -f deploy/docker-compose.yml down
```

#### Q2: 证书到期了服务会怎样？

证书过期后，HTTPS 请求会被浏览器/客户端拒绝（提示证书不安全）。Go 服务本身不会崩溃，但客户端会报 TLS 错误。**所以自动续期非常重要**。

#### Q3: 续期时服务会中断吗？

使用上述 standalone 方案，续期过程中会短暂停机（通常几秒到半分钟）。如果不能接受停机，可以考虑：
- 使用 Nginx 反代方案（在 Nginx 层终止 TLS，用 `certbot --nginx` 无缝续期）
- 使用 DNS-01 验证方式（不需要停服务，但需要 DNS 服务商 API 支持）

#### Q4: 一台服务器多个域名怎么办？

```bash
# 申请时指定多个域名
sudo certbot certonly --standalone -d api.example.com -d www.example.com
```

#### Q5: 如何查看证书到期时间？

```bash
# 查看所有 certbot 管理的证书
sudo certbot certificates

# 或用 openssl 查看具体证书
openssl x509 -in /opt/track_server/certs/server.crt -noout -dates
```

#### Q6: 如何吊销/删除证书？

```bash
sudo certbot revoke --cert-name api.example.com
sudo certbot delete --cert-name api.example.com
```

---

### 四、完整操作清单（Checklist）

首次部署时逐项确认：

- [ ] 域名 DNS A 记录已指向服务器公网 IP
- [ ] 服务器 80 端口安全组/防火墙已放通
- [ ] 服务器 443 端口安全组/防火墙已放通
- [ ] 已安装 certbot
- [ ] 已成功申请证书（`sudo certbot certificates` 可查看）
- [ ] 证书已复制到 `/opt/track_server/certs/`
- [ ] `docker-compose.yml` 已取消 TLS 注释并配置正确的卷挂载路径
- [ ] `.env` 已配置 `TLS_CERT_FILE` 和 `TLS_KEY_FILE`
- [ ] `docker compose up -d` 启动成功
- [ ] `curl -v https://你的域名/api/v1/ping` 返回正常
- [ ] 已配置 cron 定时续期任务
- [ ] `sudo certbot renew --dry-run` 模拟续期测试通过
