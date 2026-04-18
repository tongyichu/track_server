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

## 方式一：单架构构建（适用于当前平台）

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

## 方式二：跨平台构建（推荐）

使用 Docker Buildx 构建 linux/amd64 镜像（适用于在 Mac M1/M2/M3 上构建 Linux 服务器可用的镜像）。

### 1. 创建并使用 buildx 构建器
```bash
docker buildx create --use --name multiarch-builder
```

### 2. 构建并直接推送（一步完成）
```bash
# 格式：docker buildx build --platform <目标平台> -t <完整镜像名> -f <Dockerfile> . --push

docker buildx build --platform linux/amd64 \
  -t crpi-p78v4agazv8zn80d.cn-beijing.personal.cr.aliyuncs.com/track_server/track_server_go:latest \
  -f deploy/Dockerfile . \
  --push
```

---

## 拉取与运行

### 拉取镜像
```bash
docker pull crpi-p78v4agazv8zn80d.cn-beijing.personal.cr.aliyuncs.com/track_server/track_server_go:latest
```

### 运行单个容器
```bash
# 使用 Makefile
make docker-run

# 或直接使用 docker
docker run --rm --name hertz-track-api --env-file deploy/.env -p 8080:8080 hertz-track:latest
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

- **基础镜像**: gcr.io/distroless/static-debian12（极小镜像，仅约20MB）
- **暴露端口**: 8080
- **环境变量**:
  - `SERVER_ADDR`: 服务监听地址，默认 `:8080`
  - `MONGO_URI`: MongoDB 连接地址

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
