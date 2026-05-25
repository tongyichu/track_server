# EMQX 部署方案与配置清单

> 适用于 `track_server` 的“与友同行”数据面接入。
>
> 本文默认 **App Server 已经部署完成**，本文只关注 **EMQX 的部署、配置与联调**。

> **Dashboard 菜单说明：** 不同版本或不同主题下，EMQX Dashboard 的菜单命名可能略有差异。对本项目而言，真正稳定可落地的做法是：**用 `Integration -> Connector` 创建 `HTTP Server` 连接器，再用 `Integration -> Rules` 创建规则与 HTTP 动作**。`Integration -> Webhooks` 更适合轻量直连场景；如果页面里看不到自定义 Header / Request Body 模板，就不要用它来对接本文的 ingest 接口。

---

## 1. 目标

EMQX 在本项目中的职责是：

- 承担“同行”实时位置消息分发；
- 承担成员 presence（在线/离线）事件分发；
- 在 `control` topic 上向客户端广播 `member_left` / `session_ended`；
- 通过 HTTP AuthN / AuthZ 与 App Server 联动做动态鉴权；
- 通过 Rule Engine / Webhook 将位置与连接事件写回 App Server。

对应 App Server 已实现的接口：

- `POST /api/v1/companion/session/:session_id/mqtt/credentials`
- `POST /api/v1/internal/mqtt/auth`
- `POST /api/v1/internal/mqtt/acl`
- `POST /api/v1/internal/companion/mqtt/location-ingest`
- `POST /api/v1/internal/companion/mqtt/presence-ingest`

---

## 2. 推荐部署方案

### 2.1 v1 推荐：单节点 EMQX + Docker 部署

对于当前“同行”一期能力，推荐先采用：

- **单节点 EMQX 5.x**
- **Docker / Docker Compose 部署**
- **独立于 App Server 的单独进程或单独机器**

推荐原因：

- 部署简单，便于快速联调；
- 足以支撑早期实时位置共享场景；
- 未来若并发显著提升，再平滑升级为多节点集群。

### 2.2 机器建议

生产建议：

- 不与 MySQL 部署在同一台小规格机器上；
- 最好与 App Server 分离，避免高频 MQTT 长连接挤压 API 资源；
- 若当前规模较小，也可以先与 App Server 同机 Docker 化部署，但要做好 CPU / 内存隔离。

建议起步规格：

- 2 vCPU / 4 GB RAM 起步；
- SSD 磁盘；
- Linux（Ubuntu / CentOS / Rocky Linux 均可）。

---

## 3. 网络与端口规划

### 3.1 对外端口

按需开放：

| 端口 | 协议 | 用途 | 是否建议公网开放 |
| --- | --- | --- | --- |
| `1883` | MQTT/TCP | 明文 MQTT | 联调可开，生产不建议直接公网开 |
| `8883` | MQTT over TLS | 生产推荐 MQTT 接入端口 | 建议开放 |
| `8083` | WebSocket | 明文 WS | 联调可开 |
| `8084` | WSS | 生产推荐 WebSocket 接入端口 | 按需开放 |
| `18083` | Dashboard / Management API | 管理后台 | **不要直接公网开放**，仅限堡垒机 / 白名单 |

### 3.2 集群相关端口

如果未来扩容为 EMQX 集群，还需要节点间端口，例如：

- `4370`
- `5370`

当前单节点方案无需公网开放这些端口。

### 3.3 域名建议

建议准备：

- `mqtt.example.com`：客户端 MQTT / WSS 连接域名
- `emqx-admin.example.com`：如需单独管理入口，可走内网或白名单反代

App Server 中建议配置：

- `EMQX_BROKER_URL=mqtts://mqtt.example.com:8883`
- `EMQX_WEBSOCKET_URL=wss://mqtt.example.com:8084/mqtt`

---

## 4. 前置准备清单

部署 EMQX 前，请先准备以下信息：

### 4.1 App Server 内部回调地址

EMQX 需要能访问 App Server 的以下内部接口：

- `http://<app-server>/api/v1/internal/mqtt/auth`
- `http://<app-server>/api/v1/internal/mqtt/acl`
- `http://<app-server>/api/v1/internal/companion/mqtt/location-ingest`
- `http://<app-server>/api/v1/internal/companion/mqtt/presence-ingest`

建议：

- 优先使用 **内网 IP / 内网域名**；
- 确保 EMQX 所在机器可以直连这些地址；
- 不要经过额外的人机登录网关。

### 4.2 App Server 环境变量

App Server 侧至少应配置：

- `EMQX_BROKER_URL`
- `EMQX_WEBSOCKET_URL`
- `COMPANION_MQTT_TOPIC_PREFIX`
- `COMPANION_MQTT_CREDENTIAL_SECRET`
- `COMPANION_MQTT_INTERNAL_TOKEN`
- `COMPANION_MQTT_PUBLISHER_CLIENT_ID`
- `COMPANION_MQTT_PUBLISHER_USERNAME`
- `COMPANION_MQTT_PUBLISHER_PASSWORD`
- `COMPANION_MQTT_PUBLISH_TIMEOUT_SECONDS`

### 4.3 证书

生产推荐开启 TLS / WSS：

- MQTT 客户端走 `8883`
- WebSocket 客户端走 `8084`

如果暂时没有证书：

- 可以先用 `1883` / `8083` 做联调；
- 但不要长期暴露公网明文连接。

---

## 5. Docker Compose 部署示例

以下为推荐的单节点 EMQX Compose 示例。

> 文件名可自定，例如：`docker-compose.emqx.yml`

### 5.1 推荐目录结构模板

建议在服务器上准备如下目录：

```text
/opt/emqx/
├── docker-compose.emqx.yml
├── emqx/
│   ├── data/
│   ├── log/
│   └── certs/
│       ├── fullchain.pem
│       └── privkey.pem
```

如果暂时不用 TLS，则 `certs/` 可以先不挂载。

### 5.2 `docker-compose.emqx.yml` 模板

```yaml
services:
  emqx:
    image: crpi-p78v4agazv8zn80d.cn-beijing.personal.cr.aliyuncs.com/track_server/emqx:5.8.6
    container_name: track-emqx
    restart: unless-stopped
    environment:
      - EMQX_NAME=emqx
      - EMQX_HOST=node1.emqx.local
      - EMQX_DASHBOARD__DEFAULT_USERNAME=admin
      - EMQX_DASHBOARD__DEFAULT_PASSWORD=2026@strong_pass
    ports:
      - "1883:1883"
      - "8883:8883"
      - "8083:8083"
      - "8084:8084"
      - "18083:18083"
    volumes:
      - ./emqx/data:/opt/emqx/data
      - ./emqx/log:/opt/emqx/log
    ulimits:
      nofile:
        soft: 1048576
        hard: 1048576
```

启动：

```bash
docker compose -f docker-compose.emqx.yml up -d
```

首次启动后：

1. 打开 `http://<host>:18083`
2. 使用 Dashboard 默认管理员登录（按镜像版本初始值）
3. **第一时间修改管理员密码**
4. 后续按本文第 6 节配置 AuthN / AuthZ / Rule Engine

---

## 6. EMQX 配置清单（推荐按 Dashboard 配置）

本项目推荐：

- **鉴权与规则配置通过 Dashboard 完成**
- **静态服务账号（publisher）与移动端动态 session 鉴权分开配置**

原因：

- App Server 给手机客户端签发的是短期动态 MQTT 凭证；
- 但 App Server 自己作为 control publisher 连接 EMQX 时，使用的是 **固定账号密码**；
- 这两类客户端不应走完全相同的鉴权路径。

### 6.1 鉴权链路设计

推荐配置两类认证器：

1. **内置数据库认证器（给 App Server publisher 用）**
2. **HTTP 认证器（给手机客户端 session 凭证用）**

推荐顺序：

1. 内置数据库认证器
2. HTTP 认证器

这样：

- App Server publisher 账号先被内置数据库命中；
- 手机客户端再走 HTTP AuthN。

### 6.2 Authorizer 顺序

推荐：

1. 内置数据库 / 特定内部账号授权（可选）
2. HTTP Authorization
3. 关闭默认“文件 ACL 放行全部”的 authorizer

**重要：**

- 如果保留默认 ACL file authorizer，并且它最后有 `allow all`，会导致 HTTP AuthZ 失效或被绕过；
- 因此请在 EMQX Dashboard 中检查 Authorization 顺序，**禁用或移除默认 file authorizer**。

### 6.3 Dashboard 实际操作步骤

以下步骤按 **先保证服务端可连，再保证客户端可连，最后打通数据写回** 的顺序执行。

#### 第 1 步：确认监听器可用

Dashboard 路径：

- `Management` / `Listeners`

检查以下监听器是否为启用状态：

- `TCP` : `1883`
- `SSL` : `8883`（如果已配置 TLS）
- `WS` : `8083`
- `WSS` : `8084`（如果已配置 TLS）

建议：

- 联调阶段至少保证 `1883` 或 `8083` 可用；
- 生产优先使用 `8883` / `8084`。

#### 第 2 步：创建 App Server 专用 publisher 账号

Dashboard 路径：

- `Access Control` → `Authentication`

操作：

1. 点击 `Create`
2. 选择 `Password-Based`
3. Backend 选择 `Built-in Database`
4. 保存认证器后，进入该认证器详情页
5. 添加一条用户记录：
   - username: `track_server_control_publisher`
   - password: `<强随机密码>`

如果 Dashboard 支持设置 superuser，建议一期先将该账号设为 superuser；若不支持，就后续在 Authorization 中给它加一条最小权限规则。

#### 第 3 步：调整 Authentication 顺序

Dashboard 路径：

- `Access Control` → `Authentication`

要求顺序：

1. `Built-in Database`
2. `HTTP Server`

原因：

- App Server 作为固定账号先命中内置数据库；
- 移动端短期 MQTT 凭证再走 HTTP 认证。

#### 第 4 步：创建手机客户端 HTTP Authentication

Dashboard 路径：

- `Access Control` → `Authentication` → `Create`

填写：

- Type: `Password-Based`
- Backend: `HTTP Server`
- Method: `POST`
- URL: `http://<app-server-inner>/api/v1/internal/mqtt/auth`
- Headers:
  - `Content-Type: application/json`
  - `X-Internal-Token: <COMPANION_MQTT_INTERNAL_TOKEN>`
- Body:

```json
{"clientid":"${clientid}","username":"${username}","password":"${password}"}
```

- Connect Timeout: `5s`
- Request Timeout: `5s`

保存后，将它拖动到 `Built-in Database` 认证器后面。

#### 第 5 步：创建 HTTP Authorization

Dashboard 路径：

- `Access Control` → `Authorization`

操作：

1. 点击 `Create`
2. 选择 `HTTP Server`
3. 填写：
   - Method: `POST`
   - URL: `http://<app-server-inner>/api/v1/internal/mqtt/acl`
   - Headers:
     - `Content-Type: application/json`
     - `X-Internal-Token: <COMPANION_MQTT_INTERNAL_TOKEN>`
   - Body:

```json
{"clientid":"${clientid}","username":"${username}","action":"${action}","topic":"${topic}"}
```

   - Timeout: `5s`
   - No Match: `deny`

#### 第 6 步：检查并关闭默认放行 ACL

Dashboard 路径：

- `Access Control` → `Authorization`

检查是否存在默认 `File` / `ACL File` authorizer。

如果它的默认行为会放行全部或它排在 HTTP Authorization 之前，请：

- 禁用它；或
- 将其下移到最后，并确保不会 `allow all`。

目标是：

- **手机客户端 publish/subscribe 权限必须由 App Server 的 HTTP Authorization 决定**。

#### 第 7 步：给 publisher 账号最小权限（如果没用 superuser）

Dashboard 路径：

- `Access Control` → `Authorization`

如果你没有给 `track_server_control_publisher` superuser 权限，则需要额外给它加授权规则，只允许：

- publish: `companion/+/control`

不允许：

- 订阅成员位置 topic
- 发布成员位置 topic

#### 第 8 步：创建位置写回 HTTP Connector（推荐）

Dashboard 路径：

- 优先按你当前界面进入：`Integration` → `Connector` → `Create`
- 如果你使用的是旧版菜单，则可能显示为：`Data Integration` → `Data Bridges` → `Create`

选择：

- 类型：`HTTP Server`

建议命名：

- `companion_location_connector`

填写：

- URL: `http://<app-server-inner>`
- 其他高级配置保持默认即可；如果页面支持 `Test Connectivity`，可先测试连通性

`URL` 填写原则：

- 这里填写的是 **EMQX 所在环境访问 App Server 时看到的基础地址**；
- **不要默认写 `http://127.0.0.1`**，除非 EMQX 和 App Server 真的是跑在同一台机器、同一网络命名空间下；
- 如果 EMQX 与 App Server 在 **同一台宿主机但不同容器**，通常应填写：`http://宿主机内网IP:8080` 或同一 Docker 网络内可解析的服务名地址；
- 如果 EMQX 与 App Server 在 **不同机器**，填写 App Server 的内网地址，例如：`http://10.0.0.12:8080`；
- 如果你给 App Server 配了内网域名，也可以直接填域名，例如：`http://track-api.internal:8080`。

常见示例：

- 同机、App Server 直接跑宿主机进程：`http://127.0.0.1:8080`
- 同机、EMQX 与 App Server 都在 Docker 且同一网络：`http://api:8080`
- 跨机器内网调用：`http://172.18.0.10:8080`

说明：

- 这一步只是在创建可复用的 HTTP 连接器；
- **真正的 Method / URL Path / Header / Request Body 放在下一步 Rule Action 中配置**；
- 某些版本也可能把部分 Header 能力放到 Connector 或 Action 的高级设置里，按页面实际位置填写即可。

如果你当前尝试的是 `Integration -> Webhooks -> Create`，但页面里只有 `Name / Trigger / Method / URL` 一类字段、**没有**自定义 `Headers` 或 `Request Body` 模板，那不建议用它对接本项目的位置写回接口，因为服务端要求的入参是业务字段化 JSON，而不是 EMQX 默认 webhook 包体。

#### 第 9 步：创建位置消息 Rule + HTTP Action

Dashboard 路径：

- 优先按你当前界面进入：`Integration` → `Rules` → `Create`
- 如果你使用的是旧版菜单，则可能显示为：`Data Integration` → `Rules` → `Create`

建议：

- Rule Name: `companion_location_rule`
- SQL 中匹配 topic：`companion/+/member/+/location`
- Action Type：`HTTP Server`
- Connector：选择刚刚创建的 `companion_location_connector`
- Method：`POST`
- URL Path：`/api/v1/internal/companion/mqtt/location-ingest`
- Headers:
  - `Content-Type: application/json`
  - `X-Internal-Token: <COMPANION_MQTT_INTERNAL_TOKEN>`

Body 模板建议：

```json
{
  "session_id": "${payload.session_id}",
  "user_id": ${payload.user_id},
  "track_id": "${payload.track_id}",
  "latitude": ${payload.latitude},
  "longitude": ${payload.longitude},
  "coordinate_system": "${payload.coordinate_system}",
  "speed_kmh": ${payload.speed_kmh},
  "heading": ${payload.heading},
  "accuracy_m": ${payload.accuracy_m},
  "altitude": ${payload.altitude},
  "recorded_at": "${payload.recorded_at}",
  "seq": ${payload.seq},
  "source": "mqtt_rule_engine",
  "client_id": "${clientid}",
  "username": "${username}"
}
```

如果页面要求必须填写 SQL，推荐直接使用下面这段：

```sql
SELECT
  payload.session_id AS session_id,
  payload.user_id AS user_id,
  payload.track_id AS track_id,
  payload.latitude AS latitude,
  payload.longitude AS longitude,
  payload.coordinate_system AS coordinate_system,
  payload.speed_kmh AS speed_kmh,
  payload.heading AS heading,
  payload.accuracy_m AS accuracy_m,
  payload.altitude AS altitude,
  payload.recorded_at AS recorded_at,
  payload.seq AS seq,
  clientid AS client_id,
  username AS username,
  timestamp AS event_timestamp
FROM
  "companion/+/member/+/location"
```

说明：

- 这段 SQL 的前提是：**客户端 publish 的 payload 本身就是 JSON**，并且字段名与上面一致；
- 其中 `payload.xxx` 直接从 MQTT 消息体里取字段；
- `clientid` / `username` / `timestamp` 来自 EMQX 规则上下文；
- 这里不用 `SELECT * FROM "#"`，因为我们只想匹配同行位置 topic，而不是匹配全站所有消息。

如果你想先做最小验证，也可以先用更简单的一版：

```sql
SELECT
  *,
  clientid AS client_id,
  username AS username
FROM
  "companion/+/member/+/location"
```

但正式联调建议还是用上面那版显式字段映射，后续排查最方便。

**重要：** 如果你在 Rule 的 SQL 中已经把字段 alias 成了 `session_id`、`user_id`、`track_id` 等名字，那么对应 HTTP Action 的 Body 最好也改为直接引用这些别名，而不是再写 `${payload.xxx}`。

例如位置写回 HTTP Action 的 Body 建议改成：

```json
{
  "session_id": "${session_id}",
  "user_id": ${user_id},
  "track_id": "${track_id}",
  "latitude": ${latitude},
  "longitude": ${longitude},
  "coordinate_system": "${coordinate_system}",
  "speed_kmh": ${speed_kmh},
  "heading": ${heading},
  "accuracy_m": ${accuracy_m},
  "altitude": ${altitude},
  "recorded_at": "${recorded_at}",
  "seq": ${seq},
  "source": "mqtt_rule_engine",
  "client_id": "${client_id}",
  "username": "${username}"
}
```

#### 第 10 步：创建 presence 写回 HTTP Connector（推荐）

Dashboard 路径：

- 优先按你当前界面进入：`Integration` → `Connector` → `Create`
- 如果你使用的是另一套旧版菜单，则可能显示为：`Data Integration` → `Data Bridges` → `Create`

建议分别建两个 connector：

- `companion_presence_online_connector`
- `companion_presence_offline_connector`

统一基础地址：

- `http://<app-server-inner>`

填写规则与上面位置写回 connector 相同：这里仍然是 **EMQX 访问 App Server 的基础地址**，不是写完整 ingest 路径。

#### 第 11 步：创建 connected / disconnected Rule + HTTP Action

Dashboard 路径：

- 优先按你当前界面进入：`Integration` → `Rules` → `Create`
- 如果你使用的是旧版菜单，则可能显示为：`Data Integration` → `Rules` → `Create`

分别创建：

- `companion_presence_online_rule`
- `companion_presence_offline_rule`

规则来源：

- client connected event
- client disconnected event

Action 统一填写：

- Action Type：`HTTP Server`
- Connector：
  - online 选择 `companion_presence_online_connector`
  - offline 选择 `companion_presence_offline_connector`
- Method：`POST`
- URL Path：`/api/v1/internal/companion/mqtt/presence-ingest`
- Headers:
  - `Content-Type: application/json`
  - `X-Internal-Token: <COMPANION_MQTT_INTERNAL_TOKEN>`

online body 示例：

```json
{
  "session_id": "${payload.session_id}",
  "user_id": ${payload.user_id},
  "presence_status": "online",
  "last_seen_at": "${timestamp}",
  "client_id": "${clientid}",
  "username": "${username}"
}
```

offline body 示例：

```json
{
  "session_id": "${payload.session_id}",
  "user_id": ${payload.user_id},
  "presence_status": "offline",
  "last_seen_at": "${timestamp}",
  "client_id": "${clientid}",
  "username": "${username}"
}
```

如果页面要求填写 SQL，则推荐分别使用：

online：

```sql
SELECT
  payload.session_id AS session_id,
  payload.user_id AS user_id,
  clientid AS client_id,
  username AS username,
  timestamp AS event_timestamp
FROM
  "$events/client_connected"
```

offline：

```sql
SELECT
  payload.session_id AS session_id,
  payload.user_id AS user_id,
  clientid AS client_id,
  username AS username,
  timestamp AS event_timestamp
FROM
  "$events/client_disconnected"
```

如果 connected / disconnected 事件里拿不到 `payload.session_id`、`payload.user_id`，那就把 presence bridge 的 Body 简化为只传：

```json
{
  "session_id": "",
  "user_id": 0,
  "presence_status": "online",
  "last_seen_at": "${event_timestamp}",
  "client_id": "${client_id}",
  "username": "${username}"
}
```

然后由 App Server 结合 `username/client_id` 做校验和归属判断。

#### 第 12 步：用 Dashboard 逐项验证

验证顺序建议：

1. 先验证 App Server publisher：
   - 看 App Server 启动日志里是否有 `companion control publisher enabled`
2. 再验证客户端 CONNECT：
   - 观察 EMQX 是否调用了 HTTP AuthN
3. 再验证 subscribe / publish：
   - 观察 HTTP AuthZ 是否命中
4. 再验证位置写回：
   - 发布一条 location，看 App Server snapshot 是否更新
5. 最后验证 control topic：
   - 成员 leave 时是否收到 `member_left`
   - owner end 时是否收到 `session_ended`

---

## 7. 具体配置项清单

### 7.1 创建 App Server 专用 publisher 账号

用途：

- 供 App Server 主动向 `companion/{session_id}/control` 发布：
  - `member_left`
  - `session_ended`

推荐账号：

- username: `track_server_control_publisher`
- password: `<强随机密码>`

推荐做法（二选一）：

#### 方案 A：将 publisher 账号标记为 superuser（推荐一期先这样）

优点：

- 配置最简单；
- 仅供服务端内网账号使用，风险可控；
- 不影响手机客户端仍走 HTTP AuthZ。

要求：

- 该账号密码只放在 App Server 环境变量中；
- 不下发给客户端；
- 最好仅允许 App Server 所在网段访问 Broker。

#### 方案 B：不给 superuser，单独配 topic 授权

仅允许该账号：

- publish：`companion/+/control`

不允许：

- 订阅成员位置 topic
- 发布成员位置 topic

如果团队更偏向最小权限原则，可以采用该方案。

### 7.1.1 publisher 账号模板

推荐直接准备一组固定值：

```text
username: track_server_control_publisher
clientid: track-server-companion-publisher
password: <强随机密码>
```

如果一期优先追求落地效率，建议：

- 先在 EMQX 内置数据库中创建该账号；
- 先赋予 superuser 或最小发布权限；
- 待链路稳定后再收敛权限。

### 7.2 App Server 环境变量填写

与上面 publisher 账号对应，App Server 侧填写：

```env
EMQX_BROKER_URL=mqtts://mqtt.example.com:8883
EMQX_WEBSOCKET_URL=wss://mqtt.example.com:8084/mqtt
COMPANION_MQTT_TOPIC_PREFIX=companion
COMPANION_MQTT_CREDENTIAL_TTL_SECONDS=3600
COMPANION_MQTT_CREDENTIAL_SECRET=<签发短期凭证用密钥>
COMPANION_MQTT_INTERNAL_TOKEN=<EMQX 调用 App Server 内部接口的共享令牌>

COMPANION_MQTT_PUBLISHER_CLIENT_ID=track-server-companion-publisher
COMPANION_MQTT_PUBLISHER_USERNAME=track_server_control_publisher
COMPANION_MQTT_PUBLISHER_PASSWORD=<与 EMQX 内置账号一致>
COMPANION_MQTT_PUBLISH_TIMEOUT_SECONDS=5
```

---

## 8. HTTP Authentication 配置清单

EMQX Dashboard 中创建 **Password-Based / HTTP Server** 认证器。

### 8.1 认证器用途

用途：

- 给客户端基于 `/companion/session/:session_id/mqtt/credentials` 返回的动态凭证做 CONNECT 校验。

### 8.2 推荐配置

| 配置项 | 值 |
| --- | --- |
| Method | `POST` |
| URL | `http://<app-server-inner>/api/v1/internal/mqtt/auth` |
| Headers | `Content-Type: application/json` |
| 自定义 Header | `X-Internal-Token: <COMPANION_MQTT_INTERNAL_TOKEN>` |
| Body | `{"clientid":"${clientid}","username":"${username}","password":"${password}"}` |
| 超时 | `5s` 左右 |

### 8.3 Dashboard 填写模板

可以按下面模板逐项填写：

```text
Authentication Type: Password-Based
Backend: HTTP Server
Method: POST
URL: http://<app-server-inner>/api/v1/internal/mqtt/auth
Headers:
  Content-Type: application/json
  X-Internal-Token: <COMPANION_MQTT_INTERNAL_TOKEN>
Body:
  {"clientid":"${clientid}","username":"${username}","password":"${password}"}
Connect Timeout: 5s
Request Timeout: 5s
```

建议顺序：

1. 内置数据库认证器（publisher 账号）
2. HTTP 认证器（手机客户端动态凭证）

### 8.3 App Server 期望响应

成功：

```json
{
  "result": "allow",
  "is_superuser": false
}
```

失败：

```json
{
  "result": "deny"
}
```

---

## 9. HTTP Authorization 配置清单

EMQX Dashboard 中创建 **HTTP Authorization**。

### 9.1 授权器用途

用途：

- 校验客户端是否可以 publish / subscribe 指定 topic。

### 9.2 推荐配置

| 配置项 | 值 |
| --- | --- |
| Method | `POST` |
| URL | `http://<app-server-inner>/api/v1/internal/mqtt/acl` |
| Headers | `Content-Type: application/json` |
| 自定义 Header | `X-Internal-Token: <COMPANION_MQTT_INTERNAL_TOKEN>` |
| Body | `{"clientid":"${clientid}","username":"${username}","action":"${action}","topic":"${topic}"}` |
| 超时 | `5s` 左右 |
| No match | `deny` |

### 9.3 Dashboard 填写模板

```text
Authorization Type: HTTP Server
Method: POST
URL: http://<app-server-inner>/api/v1/internal/mqtt/acl
Headers:
  Content-Type: application/json
  X-Internal-Token: <COMPANION_MQTT_INTERNAL_TOKEN>
Body:
  {"clientid":"${clientid}","username":"${username}","action":"${action}","topic":"${topic}"}
Request Timeout: 5s
No Match: deny
```

### 9.4 当前 App Server 权限模型

当前允许：

- publish 自己的：
  - `companion/{session_id}/member/{user_id}/location`
  - `companion/{session_id}/member/{user_id}/presence`
- subscribe 当前 session 的：
  - `companion/{session_id}/member/+/location`
  - `companion/{session_id}/member/+/presence`
  - `companion/{session_id}/control`

---

## 10. Rule Engine / Webhook 配置清单

本项目推荐：

- 用 Rule Engine 订阅消息与事件
- 下游用 `HTTP Server Connector + Rule Action` 回调 App Server

说明：

- 如果你的 Dashboard 左侧菜单显示为 `Integration -> Webhooks`，它更适合做轻量直连；
- **本项目推荐 `Integration -> Connector` + `Integration -> Rules`**，因为需要明确配置 Header、URL Path、Request Body，并把 MQTT 字段映射成服务端 ingest 接口要求的 JSON。

### 10.1 位置消息写回

监听 topic：

- `companion/+/member/+/location`

下游回调：

- `POST http://<app-server-inner>/api/v1/internal/companion/mqtt/location-ingest`

Header：

- `Content-Type: application/json`
- `X-Internal-Token: <COMPANION_MQTT_INTERNAL_TOKEN>`

建议透传字段：

```json
{
  "session_id": "<从 topic 或 payload 提取>",
  "user_id": "<从 topic 或 payload 提取>",
  "track_id": "<payload.track_id>",
  "latitude": "<payload.latitude>",
  "longitude": "<payload.longitude>",
  "coordinate_system": "<payload.coordinate_system>",
  "speed_kmh": "<payload.speed_kmh>",
  "heading": "<payload.heading>",
  "accuracy_m": "<payload.accuracy_m>",
  "altitude": "<payload.altitude>",
  "recorded_at": "<payload.recorded_at>",
  "seq": "<payload.seq>",
  "source": "mqtt_rule_engine",
  "client_id": "${clientid}",
  "username": "${username}"
}
```

说明：

- `session_id` / `user_id` 可以从 topic 提取，也可以由客户端一并放到 payload；
- 推荐 **topic 与 payload 同时带 session_id / user_id**，便于排查问题；
- App Server 会再次校验 principal 与 session/user 的一致性。

#### 10.1.1 位置写回 HTTP Action 模板

如果你在 Dashboard 中配置 `HTTP Server Connector + Rule Action`，可按下面模板：

```text
Connector Name: companion_location_connector
Connector URL: http://<app-server-inner>
Rule Name: companion_location_rule
Action Type: HTTP Server
Method: POST
URL Path: /api/v1/internal/companion/mqtt/location-ingest
Headers:
  Content-Type: application/json
  X-Internal-Token: <COMPANION_MQTT_INTERNAL_TOKEN>
Request Body(JSON):
  {
    "session_id": "${payload.session_id}",
    "user_id": ${payload.user_id},
    "track_id": "${payload.track_id}",
    "latitude": ${payload.latitude},
    "longitude": ${payload.longitude},
    "coordinate_system": "${payload.coordinate_system}",
    "speed_kmh": ${payload.speed_kmh},
    "heading": ${payload.heading},
    "accuracy_m": ${payload.accuracy_m},
    "altitude": ${payload.altitude},
    "recorded_at": "${payload.recorded_at}",
    "seq": ${payload.seq},
    "source": "mqtt_rule_engine",
    "client_id": "${clientid}",
    "username": "${username}"
  }
```

如果你的 EMQX 规则语法无法直接用 `${payload.xxx}` 这种写法，请在 Dashboard 中改为对应版本支持的占位符表达式。

### 10.2 Presence 写回

推荐监听 EMQX 连接生命周期事件：

- client connected -> `online`
- client disconnected -> `offline`

回调地址：

- `POST http://<app-server-inner>/api/v1/internal/companion/mqtt/presence-ingest`

Header：

- `Content-Type: application/json`
- `X-Internal-Token: <COMPANION_MQTT_INTERNAL_TOKEN>`

建议透传字段：

```json
{
  "session_id": "<从 username principal 解析或规则中提取>",
  "user_id": "<从 username principal 解析或规则中提取>",
  "presence_status": "online",
  "last_seen_at": "<事件时间>",
  "client_id": "${clientid}",
  "username": "${username}"
}
```

说明：

- 如果 EMQX 规则侧不方便直接从 principal 拆出 `session_id / user_id`，也可以先只传 `client_id / username`，再由 App Server 侧结合 principal 做校验；
- 但为减少歧义，推荐规则侧一并带上 `session_id / user_id`。

#### 10.2.1 Presence 写回 HTTP Action 模板

在线事件模板：

```text
Connector Name: companion_presence_online_connector
Rule Name: companion_presence_online_rule
Action Type: HTTP Server
Method: POST
URL Path: /api/v1/internal/companion/mqtt/presence-ingest
Headers:
  Content-Type: application/json
  X-Internal-Token: <COMPANION_MQTT_INTERNAL_TOKEN>
Request Body(JSON):
  {
    "session_id": "${payload.session_id}",
    "user_id": ${payload.user_id},
    "presence_status": "online",
    "last_seen_at": "${timestamp}",
    "client_id": "${clientid}",
    "username": "${username}"
  }
```

离线事件模板：

```text
Connector Name: companion_presence_offline_connector
Rule Name: companion_presence_offline_rule
Action Type: HTTP Server
Method: POST
URL Path: /api/v1/internal/companion/mqtt/presence-ingest
Headers:
  Content-Type: application/json
  X-Internal-Token: <COMPANION_MQTT_INTERNAL_TOKEN>
Request Body(JSON):
  {
    "session_id": "${payload.session_id}",
    "user_id": ${payload.user_id},
    "presence_status": "offline",
    "last_seen_at": "${timestamp}",
    "client_id": "${clientid}",
    "username": "${username}"
  }
```

### 10.3 推荐的客户端上报 payload 模板

为了让 Rule Engine 配置最简单，推荐客户端发布 location 时使用如下 JSON：

```json
{
  "session_id": "sess_xxx",
  "user_id": 1001,
  "track_id": "NO.00000001",
  "latitude": 30.123,
  "longitude": 120.456,
  "coordinate_system": "GCJ02",
  "speed_kmh": 5.2,
  "heading": 90,
  "accuracy_m": 8,
  "altitude": 100,
  "recorded_at": "2026-05-24T10:00:00Z",
  "seq": 1024
}
```

对应 publish topic：

- `companion/{session_id}/member/{user_id}/location`

---

## 11. control topic 使用约定

Topic：

- `companion/{session_id}/control`

服务端会主动发布：

### 11.1 成员离开

```json
{
  "event": "member_left",
  "session_id": "sess_xxx",
  "member_user_id": 1002,
  "operator_user_id": 1002,
  "reason": "member_left",
  "at": "2026-05-23T18:00:00Z"
}
```

### 11.2 会话结束

```json
{
  "event": "session_ended",
  "session_id": "sess_xxx",
  "operator_user_id": 1001,
  "reason": "owner_ended",
  "at": "2026-05-23T18:05:00Z"
}
```

或：

```json
{
  "event": "session_ended",
  "session_id": "sess_xxx",
  "operator_user_id": 0,
  "reason": "all_members_left",
  "at": "2026-05-23T18:06:00Z"
}
```

客户端处理建议：

- 收到 `member_left`：从地图中移除该成员；
- 收到 `session_ended`：立即停止上报位置、取消订阅、退出同行页面；
- 如果 `session_ended` 与滞后位置消息乱序，以 `session_ended` 为准。

---

## 12. 上线前检查清单

### 12.1 Broker 层

- [ ] EMQX 单节点启动正常
- [ ] Dashboard 管理密码已修改
- [ ] `18083` 未直接暴露公网，或已做白名单限制
- [ ] 若上线生产，已启用 TLS / WSS

### 12.2 鉴权层

- [ ] 已创建 publisher 专用账号
- [ ] publisher 账号可成功连接 EMQX
- [ ] 手机客户端 session 凭证可成功通过 HTTP AuthN
- [ ] HTTP AuthZ 顺序正确
- [ ] 默认 file ACL authorizer 已禁用或下移且不会 `allow all`

### 12.3 回调层

- [ ] EMQX 能访问 App Server 内网地址
- [ ] `/api/v1/internal/mqtt/auth` 可返回 allow/deny
- [ ] `/api/v1/internal/mqtt/acl` 可返回 allow/deny
- [ ] `/api/v1/internal/companion/mqtt/location-ingest` 写回成功
- [ ] `/api/v1/internal/companion/mqtt/presence-ingest` 写回成功

### 12.4 业务联调层

- [ ] 创建 session 后能拿到 MQTT credentials
- [ ] 成员加入后能收到其他成员位置
- [ ] 成员离开时客户端能收到 `member_left`
- [ ] owner 结束时客户端能收到 `session_ended`
- [ ] 所有人离开后能 auto-end

---

## 13. 联调建议顺序

建议按以下顺序联调，问题最少：

1. 先把 EMQX 单节点跑起来；
2. 先创建 publisher 专用账号，并验证 App Server 能连上 EMQX；
3. 配 HTTP AuthN，验证手机客户端 CONNECT；
4. 配 HTTP AuthZ，验证 publish/subscribe 权限；
5. 配位置写回规则，验证 snapshot 可更新；
6. 配 presence 写回规则，验证在线状态同步；
7. 最后联调 `control` topic 的 `member_left` / `session_ended`。

---

## 14. 常见问题

### 14.1 客户端明明鉴权失败，但还能收发消息

优先检查：

- 是否还保留了默认 file ACL authorizer；
- 是否存在“最后 allow all”的授权规则；
- HTTP AuthZ 是否排在真正生效的位置。

### 14.2 App Server 可以签发 MQTT 凭证，但客户端连不上 EMQX

优先检查：

- `EMQX_BROKER_URL` / `EMQX_WEBSOCKET_URL` 是否正确；
- EMQX 公开端口是否放通；
- AuthN 回调 URL 是否能从 EMQX 所在机器访问；
- `COMPANION_MQTT_INTERNAL_TOKEN` 是否与 App Server 一致。

### 14.3 control 消息没有发出来

优先检查：

- App Server 是否配置了：
  - `COMPANION_MQTT_PUBLISHER_USERNAME`
  - `COMPANION_MQTT_PUBLISHER_PASSWORD`
- publisher 账号是否能通过 EMQX 认证；
- App Server 日志中是否有 `companion control publisher enabled`；
- App Server 日志中是否出现 `companion control event publish failed`。

### 14.4 location 能写回，presence 不更新

优先检查：

- EMQX 是否真的配置了连接/断开事件规则；
- 回调 payload 中是否包含 `client_id` / `username`；
- App Server 是否能根据 principal 校验到对应 session/member。

---

## 15. 后续演进建议

后续可以继续演进：

- EMQX 多节点集群；
- 使用专门的内网负载均衡承接 MQTT / WSS；
- 引入 LWT 与更细粒度的断线补偿；
- 对 `control` topic 增加 `member_kicked`、`owner_transferred` 等事件；
- 对 publisher 账号从 superuser 收敛到最小权限 ACL。
