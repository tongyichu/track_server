# deploy 目录说明

`deploy` 目录用于存放项目部署相关的配置与脚本，当前同时覆盖了以下几种部署方式：

- 基于 `Dockerfile` + `docker-compose.yml` 的容器化部署
- 基于 `systemd` 的二进制部署
- 基于 `Nginx` 的反向代理配置示例

## 目录结构

```text
deploy/
├── .env.example
├── Dockerfile
├── docker-compose.yml
├── nginx/
│   └── hertz-track.conf
├── scripts/
│   └── deploy_binary.sh
└── systemd/
    └── hertz-track.service
```

## 文件说明

### `deploy/.env.example`

Docker Compose 部署使用的环境变量模板文件。

主要作用：

- 定义应用端口、监听地址、MongoDB 连接、MySQL DSN、日志目录等运行参数。
- 提供 TLS 相关变量示例，例如 `TLS_CERT_FILE` 和 `TLS_KEY_FILE`。
- 作为 `deploy/.env` 的初始化模板，正式部署前需要复制一份并填写真实配置。

使用说明：

- `docker-compose.yml` 通过 `env_file: .env` 读取同目录下的 `.env` 文件，因此不能直接只保留 `.env.example`。
- 如果启用 HTTPS，需要同时修改 `BIND_ADDR`、TLS 证书路径，并配合 `docker-compose.yml` 打开证书挂载和 TLS 端口映射。

### `deploy/Dockerfile`

用于构建服务镜像的多阶段 Dockerfile。

主要作用：

- 在构建阶段使用 Go 环境编译 `./cmd/server`，生成 `hertz-track` 二进制。
- 在运行阶段基于精简版 Debian 镜像打包最终产物。
- 预创建 `/var/log/track_server` 日志目录，并暴露 `8080` / `8443` 端口，兼容 HTTP 和 HTTPS 场景。

适用场景：

- 配合 `deploy/docker-compose.yml` 构建并启动完整容器环境。
- 也可以单独执行 `docker build` 构建镜像。

### `deploy/docker-compose.yml`

Docker Compose 编排文件，用于启动容器化部署所需的服务。

主要作用：

- 定义 `api` 服务，使用 `deploy/Dockerfile` 构建应用镜像。
- 定义 `mongo` 服务，作为默认的 MongoDB 存储。
- 通过环境变量将 `.env` 中的配置注入应用容器。
- 挂载宿主机日志目录，以及为 MongoDB 声明持久化数据卷。

当前配置特点：

- 默认对外暴露 HTTP 端口 `8080`。
- TLS 端口映射和证书目录挂载已预留为注释，启用 HTTPS 时需要手动取消注释。
- 使用 `unless-stopped` 重启策略，适合长期运行。

### `deploy/nginx/hertz-track.conf`

Nginx 反向代理配置示例。

主要作用：

- 将外部请求转发到本机 `127.0.0.1:8080` 的后端服务。
- 设置常见的反向代理请求头，例如 `Host`、`X-Real-IP`、`X-Forwarded-For`。
- 提供一组基础安全响应头配置。
- 额外包含一份注释掉的 HTTPS `server` 配置块，便于后续在 Nginx 层终止 TLS。

适用场景：

- 服务运行在本机或内网端口，通过 Nginx 对外提供统一入口。
- 需要在 Nginx 层处理域名、证书与 HTTPS 跳转。

### `deploy/scripts/deploy_binary.sh`

基于 `systemd` 的二进制部署脚本示例。

主要作用：

- 在本地编译 Linux 版本的 `hertz-track` 可执行文件。
- 通过 `scp` 将二进制、环境变量文件、systemd 服务文件上传到远程服务器。
- 通过 `ssh` 在远程机器上安装文件并执行 `systemctl daemon-reload`、`enable --now`。

注意事项：

- 该脚本中的远程主机、SSH 用户、端口均为占位值，使用前必须修改。
- 脚本会读取仓库根目录下的 `.env` 或 `.env.example`，这与 `deploy/.env.example` 面向 Docker Compose 的使用方式不同，部署时需要注意区分。
- 这是示例脚本，适合演示二进制部署流程，不建议不做调整直接用于生产环境。

### `deploy/systemd/hertz-track.service`

`systemd` 服务单元文件，用于将应用注册为 Linux 后台服务。

主要作用：

- 指定服务工作目录为 `/opt/hertz-track`。
- 通过 `EnvironmentFile=/etc/hertz-track.env` 注入环境变量。
- 定义服务启动命令、重启策略和开机自启目标。

注意事项：

- 文件中的 `ExecStart` 仍保留了 `-config /opt/hertz-track/.env` 参数，而注释说明当前服务主要依赖环境变量配置；实际使用前应确认程序是否真的需要该启动参数。
- `User=www-data` 需要与目标机器上的实际运行用户保持一致。

## 快速理解：每个文件分别负责什么

- `deploy/.env.example`：提供容器部署所需的环境变量模板。
- `deploy/Dockerfile`：负责构建应用镜像。
- `deploy/docker-compose.yml`：负责启动应用容器和 MongoDB 容器。
- `deploy/nginx/hertz-track.conf`：提供 Nginx 反向代理与 HTTPS 配置示例。
- `deploy/scripts/deploy_binary.sh`：提供二进制上传和远程安装脚本。
- `deploy/systemd/hertz-track.service`：定义 Linux 后台服务如何启动应用。

## 使用建议

- 如果使用 Docker 部署，优先关注 `Dockerfile`、`docker-compose.yml` 和 `.env.example`。
- 如果使用传统 Linux 部署，优先关注 `scripts/deploy_binary.sh` 和 `systemd/hertz-track.service`。
- 如果需要通过域名对外提供服务，额外参考 `nginx/hertz-track.conf`。
- 启用 TLS 时，要统一确认是由应用自身处理 HTTPS，还是由 Nginx 终止 TLS，避免两套配置重复启用。
