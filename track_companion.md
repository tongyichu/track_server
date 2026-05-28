# 同行能力技术方案（控制面 + MQTT 数据面预留）

> Base URL: `http://<host>:<port>/api/v1`
>
> 本文说明“同行”能力的整体技术设计，并标记当前仓库已落地的 **控制面 App Server** 能力。

---

## 1. 目标

“同行”用于让一组用户在一次临时会话内共享彼此的当前位置，并在客户端地图上实时展示。

本文采用的总体思路是：

- **App Server 负责控制面**：创建/加入/离开/结束 session、成员资格校验、快照查询、后续 MQTT 凭证签发；
- **EMQX 负责数据面**：位置 publish / subscribe、presence、控制消息广播；
- **新成员加入时立即看到所有成员的最新位置**：由 App Server 返回 HTTP Snapshot；
- **owner 掉线不立刻结束 session**：owner 重连后可主动结束；若所有成员都业务退出，则服务端自动结束 session。

---

## 2. 术语

| 术语 | 含义 |
| --- | --- |
| Companion Session | 一次“同行”会话，由 owner 创建，其他成员通过 `join_token` 加入 |
| Owner | 同行发起人，默认拥有结束 session 的权限 |
| Member | 参与当前 session 的普通成员 |
| Snapshot | 当前 session 的静态快照：成员列表 + 最新位置列表 |
| Presence | 连接状态：`online` / `offline` |
| Control Plane | App Server 提供的会话管理、权限校验、快照查询能力 |
| Data Plane | EMQX Broker 提供的实时位置消息收发能力 |

---

## 3. 总体架构

### 3.1 控制面（当前仓库已实现）

App Server 提供以下职责：

- 创建同行 session
- 通过 `join_token` 加入 session
- 查询当前用户的 active session
- 拉取 session snapshot
- 成员主动离开 session
- owner 主动结束 session
- 维护成员业务状态（`joined/left/ended`）和连接状态（`online/offline`）
- 维护每个成员的最新位置快照（为后续 EMQX 接入预留）

### 3.2 数据面（待后续接入 EMQX）

后续会接入 EMQX，承担：

- 实时位置消息分发
- Presence 广播
- session 控制消息广播
- LWT 离线通知

当前代码已为后续接入预留：

- `CompanionLivePosition` 最新位置快照模型
- `CompanionService.UpdatePresence(...)`
- `CompanionService.UpsertPositionSnapshot(...)`

---

## 4. 生命周期与状态机

### 4.1 Session 状态

| 状态 | 说明 |
| --- | --- |
| `active` | 进行中，可加入、可拉快照、可被后续数据面写入位置 |
| `ended` | 已结束，不可再加入或继续作为 active session 使用 |

### 4.2 Member 业务状态

| 状态 | 说明 |
| --- | --- |
| `joined` | 当前仍然是有效成员 |
| `left` | 主动离开，不再属于该 session |
| `kicked` | 预留，后续用于 owner 移除成员 |
| `ended` | session 结束后的归档状态 |

### 4.3 Presence 状态

| 状态 | 说明 |
| --- | --- |
| `online` | 在线 |
| `offline` | 离线 |

### 4.4 关键业务规则

1. 同一用户同一时间只允许加入 **一个 active companion session**。
2. owner 掉线不自动结束 session。
3. owner 在有其他 `joined` 成员时**不能直接 leave**，只能调用 `end`。
4. 当 session 中 `member_status=joined` 的成员数变为 0 时，服务端自动结束 session。
5. 新成员加入时，服务端必须返回当前快照，不依赖 MQTT retained 消息。

---

## 5. 数据模型

### 5.1 CompanionSession

核心字段：

- `session_id`
- `owner_user_id`
- `status`
- `join_token`
- `join_token_expire_at`
- `title`
- `max_members`
- `started_at`
- `ended_at`

### 5.2 CompanionSessionMember

核心字段：

- `session_id`
- `user_id`
- `role`（`owner/member`）
- `member_status`
- `presence_status`
- `joined_at`
- `left_at`
- `last_seen_at`
- `mqtt_client_id`（后续 EMQX 接入时使用）
- `mqtt_principal`（后续 EMQX 接入时使用）

### 5.3 CompanionLivePosition

核心字段：

- `session_id`
- `user_id`
- `track_id`
- `latitude`
- `longitude`
- `coordinate_system`
- `speed_kmh`
- `heading`
- `accuracy_m`
- `altitude`
- `recorded_at`
- `seq`
- `source`

说明：

- 每个 `(session_id, user_id)` 仅保存一条最新位置快照；
- 不在控制面阶段存高频历史点流；
- 后续 EMQX 可通过 rule engine 或内部回调把 MQTT 消息落到该表。

---

## 6. 控制面接口设计（当前仓库已实现）

### 6.1 创建 session

- `POST /api/v1/companion/session/create`

入参：

```json
{
  "title": "周末同行",
  "max_members": 8
}
```

返回：

- `session`
- `join`（包含 `join_token`）
- `snapshot`

### 6.2 加入 session

- `POST /api/v1/companion/session/join`

入参：

```json
{
  "join_token": "abcd1234EFGH5678"
}
```

返回：

- `session`
- `snapshot`

### 6.3 获取当前 active session

- `GET /api/v1/companion/session/current`

### 6.4 获取指定 session 快照

- `GET /api/v1/companion/session/:session_id/snapshot`

### 6.5 成员主动离开

- `POST /api/v1/companion/session/:session_id/leave`

### 6.6 owner 主动结束

- `POST /api/v1/companion/session/:session_id/end`

---

## 7. Snapshot 设计

Snapshot 是控制面的关键输出，结构为：

```json
{
  "snapshot_at": "2026-05-23T16:20:00Z",
  "members": [
    {
      "user_id": 1001,
      "role": "owner",
      "member_status": "joined",
      "presence_status": "offline",
      "nickname": "Tom",
      "avatar_url": "/api/v1/static/default_avatars/boy_01.png",
      "joined_at": "2026-05-23T16:00:00Z"
    }
  ],
  "positions": [
    {
      "session_id": "sess_xxx",
      "user_id": 1001,
      "track_id": "NO.00000001",
      "latitude": 30.123,
      "longitude": 120.456,
      "coordinate_system": "GCJ02",
      "recorded_at": "2026-05-23T16:19:58Z",
      "seq": 1024,
      "source": "mqtt"
    }
  ]
}
```

### 设计要点

- 只返回 `member_status=joined` 的成员；
- 只返回属于当前 joined 成员集合的最新位置；
- 头像字段遵循当前用户体系：若用户未设置头像，返回默认头像地址；
- `snapshot_at` 用于客户端处理快照与后续实时流的先后顺序。

---

## 8. MQTT / EMQX 数据面对接（当前仓库已实现服务端接口）

### 8.1 Topic 规划

- `companion/{session_id}/member/{user_id}/location`
- `companion/{session_id}/member/{user_id}/presence`
- `companion/{session_id}/control`

### 8.2 鉴权边界

当前实现：

- App JWT 只用于调用 App Server 控制面；
- App Server 为 EMQX 颁发短期 MQTT 连接凭证；
- EMQX 用 HTTP AuthN/AuthZ 做动态权限控制；
- EMQX Rule Engine / WebHook 通过服务端内部接口写回位置快照与 presence。

### 8.3 已实现接口

#### 8.3.1 客户端获取 MQTT 凭证

- `POST /api/v1/companion/session/:session_id/mqtt/credentials`

说明：

- 仅该 session 的 `joined` 成员可调用；
- 返回短期 `client_id / username / password`；
- 同时返回客户端应使用的 topic 绑定信息。

返回字段示例：

```json
{
  "code": 0,
  "data": {
    "session_id": "sess_xxx",
    "broker_url": "mqtt://emqx.example.com:1883",
    "websocket_url": "wss://emqx.example.com:8084/mqtt",
    "client_id": "cmp-sess_xxx-1001-abc123",
    "username": "cmpv1:sess_xxx:1001:1770000000:abc123",
    "password": "signed_password",
    "expires_at": "2026-05-23T18:00:00Z",
    "topics": {
      "location_publish": "companion/sess_xxx/member/1001/location",
      "location_subscribe": "companion/sess_xxx/member/+/location",
      "presence_publish": "companion/sess_xxx/member/1001/presence",
      "presence_subscribe": "companion/sess_xxx/member/+/presence",
      "control_subscribe": "companion/sess_xxx/control"
    }
  }
}
```

#### 8.3.2 EMQX HTTP AuthN

- `POST /api/v1/internal/mqtt/auth`

说明：

- 供 EMQX 在客户端 CONNECT 时调用；
- 通过 `X-Internal-Token` 做服务间鉴权；
- 返回 `{ "result": "allow|deny", "is_superuser": false }`。

#### 8.3.3 EMQX HTTP AuthZ

- `POST /api/v1/internal/mqtt/acl`

说明：

- 供 EMQX 在 publish / subscribe 时调用；
- 当前允许：
  - 成员 publish 自己的 `location` / `presence` topic；
  - 成员 subscribe 当前 session 的位置通配 topic、presence 通配 topic、control topic。

#### 8.3.4 位置写回接口

- `POST /api/v1/internal/companion/mqtt/location-ingest`

说明：

- 供 EMQX Rule Engine / WebHook 在收到位置消息后回调；
- 服务端会校验 `session_id / user_id` 与 MQTT principal 是否匹配；
- 仅保留每位成员最新一条位置快照；
- 若新消息 `seq` 更旧，或 `seq` 相同但 `recorded_at` 不更新，则会被忽略。

#### 8.3.5 Presence 写回接口

- `POST /api/v1/internal/companion/mqtt/presence-ingest`

说明：

- 供 EMQX 连接事件 / Rule Engine 回调；
- 将成员在线状态写回 `online / offline` 与 `last_seen_at`。

#### 8.3.6 服务端主动发布 control 消息

当前仓库已实现：

- 成员离开时，服务端向 `companion/{session_id}/control` 发布 `member_left`；
- owner 主动结束时，服务端向 `companion/{session_id}/control` 发布 `session_ended`；
- 所有人离开导致 auto-end 时，也会发布 `session_ended`。

实现位置：

- `CompanionService.LeaveSession(...)`
- `CompanionService.endSessionInternal(...)`
- `CompanionService.publishControlEvent(...)`

说明：

- 发布为 **best-effort**，若 Broker 不可达，仅记日志，不阻塞控制面主流程；
- 推荐 control 消息使用 **QoS 1**；
- control 消息 **不使用 retained**。

### 8.4 快照与实时流的关系

- 新成员加入：先拉 HTTP Snapshot；
- 再连接 EMQX 订阅增量流；
- `seq` / `recorded_at` 由客户端做去重和乱序保护。

### 8.5 control topic 消息格式

Topic：

- `companion/{session_id}/control`

消息体：

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

或：

```json
{
  "event": "session_ended",
  "session_id": "sess_xxx",
  "operator_user_id": 1001,
  "reason": "owner_ended",
  "at": "2026-05-23T18:05:00Z"
}
```

字段说明：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `event` | string | 事件类型：`member_left` / `session_ended` |
| `session_id` | string | 同行会话 ID |
| `member_user_id` | int64 | 被影响成员 user_id，仅 `member_left` 必有 |
| `operator_user_id` | int64 | 发起操作的用户；auto-end 时可为 `0` |
| `reason` | string | 原因，当前可能为 `member_left` / `owner_ended` / `all_members_left` |
| `at` | string | 事件时间，RFC3339 |

客户端约定：

- 收到 `member_left`：将对应成员从当前实时展示集合移除，并停止展示其最后位置；
- 收到 `session_ended`：立即停止当前位置上传、取消所有 companion topic 订阅、退出同行 UI，并回退到普通地图态；
- 若 `session_ended` 先于某些滞后 location 消息到达，以 `session_ended` 为准；
- control 消息只做会话控制，不作为快照真值来源；冷启动真值仍来自 HTTP Snapshot。

### 8.6 EMQX 推荐配置样例

以下为 **推荐联调方式**，重点是说明字段映射与回调方向；具体语法可按实际 EMQX 版本微调。

#### 8.6.1 HTTP AuthN

EMQX 在客户端 CONNECT 时回调：

- URL: `POST http://<app-server>/api/v1/internal/mqtt/auth`
- Header: `X-Internal-Token: ${COMPANION_MQTT_INTERNAL_TOKEN}`

请求体示例：

```json
{
  "clientid": "${clientid}",
  "username": "${username}",
  "password": "${password}"
}
```

#### 8.6.2 HTTP AuthZ

EMQX 在 publish / subscribe 前回调：

- URL: `POST http://<app-server>/api/v1/internal/mqtt/acl`
- Header: `X-Internal-Token: ${COMPANION_MQTT_INTERNAL_TOKEN}`

请求体示例：

```json
{
  "clientid": "${clientid}",
  "username": "${username}",
  "action": "${action}",
  "topic": "${topic}"
}
```

#### 8.6.3 Rule Engine：位置消息写回

推荐从 topic：

- `companion/+/member/+/location`

抽取字段后回调：

- URL: `POST http://<app-server>/api/v1/internal/companion/mqtt/location-ingest`
- Header: `X-Internal-Token: ${COMPANION_MQTT_INTERNAL_TOKEN}`

建议透传字段：

- `session_id`
- `user_id`
- `track_id`
- `latitude`
- `longitude`
- `coordinate_system`
- `speed_kmh`
- `heading`
- `accuracy_m`
- `altitude`
- `recorded_at`
- `seq`
- `source`
- `client_id`
- `username`

#### 8.6.4 Rule Engine：连接 / 断开事件写回

推荐监听连接生命周期事件：

- connected -> `presence_status=online`
- disconnected / client.disconnected -> `presence_status=offline`

统一回调：

- URL: `POST http://<app-server>/api/v1/internal/companion/mqtt/presence-ingest`
- Header: `X-Internal-Token: ${COMPANION_MQTT_INTERNAL_TOKEN}`

#### 8.6.5 服务端 control publisher

App Server 作为一个普通 MQTT client 连接 EMQX，使用单独客户端身份：

- `COMPANION_MQTT_PUBLISHER_CLIENT_ID`
- `COMPANION_MQTT_PUBLISHER_USERNAME`
- `COMPANION_MQTT_PUBLISHER_PASSWORD`

其职责仅为：

- 向 `companion/{session_id}/control` 发布 `member_left` / `session_ended`；
- 不参与位置订阅与快照真值判断。

### 8.7 建议的环境变量

- `EMQX_BROKER_URL`
- `EMQX_WEBSOCKET_URL`
- `COMPANION_MQTT_TOPIC_PREFIX`（默认 `companion`）
- `COMPANION_MQTT_CREDENTIAL_TTL_SECONDS`（默认 `3600`）
- `COMPANION_MQTT_CREDENTIAL_SECRET`
- `COMPANION_MQTT_INTERNAL_TOKEN`
- `COMPANION_MQTT_PUBLISHER_CLIENT_ID`
- `COMPANION_MQTT_PUBLISHER_USERNAME`
- `COMPANION_MQTT_PUBLISHER_PASSWORD`
- `COMPANION_MQTT_PUBLISH_TIMEOUT_SECONDS`（默认 `5`）

---

## 9. 当前仓库的实现范围

### 已实现

- Companion 领域模型
- Companion 仓储接口
- In-memory / MySQL / Mongo(stub) 三套 companion repository
- CompanionService 业务逻辑
- CompanionHandler HTTP 接口
- 路由注册与依赖注入
- Handler 层基础测试
- MQTT 短期凭证签发接口
- EMQX HTTP AuthN / AuthZ 回调接口
- MQTT 位置写回 / presence 写回内部接口
- 服务端主动发布 `member_left` / `session_ended` control 消息

### 暂未实现

- LWT / Broker 侧清理策略编排文档
- 踢人、owner 转移、多端登录冲突控制

---

## 10. 建议的后续迭代顺序

1. 在 EMQX 侧完成 HTTP AuthN / AuthZ 与 Rule Engine 实际配置
2. 联调 `control` topic 的客户端消费逻辑（`member_left` / `session_ended`）
3. 联调 LWT / disconnect 事件到 `presence-ingest`
4. 按需要扩展踢人、owner 转移、多端登录控制等能力
