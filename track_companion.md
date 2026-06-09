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
2. 同一用户同一时间只能处于一种进行中状态：不能一边有 active companion session，一边开始普通 running track；已有普通 running track 时也不能创建 / 加入同行。
3. `track/create` 中 `is_running=false` 表示上传已完成轨迹，不受上一条互斥限制，可用于同行结束后上传个人轨迹并携带 `session_id`。
4. owner 掉线不自动结束 session。
5. owner 在有其他 `joined` 成员时**不能直接 leave**，只能调用 `end`。
6. 当 session 中 `member_status=joined` 的成员数变为 0 时，服务端自动结束 session。
7. 若 owner 忘记结束 session，服务端通过 `companion_session_autoclose` 定时任务兜底自动结束超时 session。
8. 新成员加入时，服务端必须返回当前快照，不依赖 MQTT retained 消息。

---

## 5. 数据模型

### 5.1 CompanionSession

核心字段：

- `session_id`
- `owner_user_id`
- `status`
- `visibility`（`private` / `public`，默认 `private`；公开房间会出现在「附近房间」列表中，且允许凭 `session_id` 加入）
- `join_token`
- `title`
- `max_members`
- `started_at`
- `expires_at`（由 `started_at + track_type 对应最大持续时间` 派生，不入库，用于客户端 session 过期倒计时；`track_type` 使用英文 code，如 `hiking` / `running`）
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
  "max_members": 8,
  "visibility": "private"
}
```

`visibility` 可选，默认 `private`：

- `private`：私密房间，必须凭 `join_token` 加入；不会出现在「附近房间」列表中。
- `public`：公开房间，可凭 `session_id` 直接加入；同时会进入「附近房间」列表。

创建规则：

- 当前用户已有普通 running track 时，不允许创建同行。

返回：

- `session`（包含 `visibility` / `expires_at`）
- `join`（包含 `join_token`）
- `snapshot`

### 6.2 加入 session

- `POST /api/v1/companion/session/join`

入参（`join_token` / `session_id` 二选一，至少填一个）：

```json
{
  "join_token": "abcd1234EFGH5678"
}
```

或：

```json
{
  "session_id": "sess_xxx"
}
```

加入规则：

- 私密房间必须凭 `join_token` 加入；用 `session_id` 加入私密房间会返回 `403 Forbidden`。
- 公开房间可凭 `session_id` 加入（典型场景：从「附近房间」卡片点击加入）；也支持继续凭 `join_token` 加入。
- 当前用户已有普通 running track 时，不允许加入同行。

返回：

- `session`（包含 `expires_at`）
- `snapshot`

### 6.3 预览 session（不加入）

- `GET /api/v1/companion/session/preview?join_token=abcd1234EFGH5678`

规则：

- 必须登录；
- 只通过 `join_token` 查询；
- 只返回 active session；
- 不创建 / 更新成员资格，不影响当前用户已有 active session；
- 返回 `session` 与 `snapshot`，其中 `snapshot` 只包含 joined 成员资料，不返回实时位置与 `join_token`。

### 6.4 获取当前 active session

- `GET /api/v1/companion/session/current`

返回当前用户已加入的 active session 及其 snapshot；`session.expires_at` 用于客户端展示 session 过期倒计时，重连后也以该字段恢复倒计时。

### 6.5 获取指定 session 快照

- `GET /api/v1/companion/session/:session_id/snapshot`

### 6.6 成员主动离开

- `POST /api/v1/companion/session/:session_id/leave`

### 6.7 owner 主动结束

- `POST /api/v1/companion/session/:session_id/end`

### 6.8 owner 踢出成员

- `POST /api/v1/companion/session/:session_id/members/:user_id/kick`

规则：

- 必须登录；
- 仅 session owner 可调用；
- 仅 active session 可踢人；
- 仅可踢出当前 `joined` 成员；
- owner 不能踢自己；
- 被踢成员的 `member_status` 变为 `kicked`，`presence_status` 变为 `offline`，并从 snapshot 成员与位置集合中移除；
- 服务端通过 `companion/{session_id}/control` 广播 `member_kicked` 事件。

### 6.9 同行结束后的个人轨迹上传

同行结束后，每个成员仍通过普通轨迹接口上传自己的完整轨迹：

- `POST /api/v1/track/create`
- `PUT /api/v1/track/:track_id/update`

客户端应在请求体中携带本次同行的 `session_id`：

```json
{
  "session_id": "sess_xxx"
}
```

规则：

- `session_id` 是 `track_records` 上的可选关联字段；
- 同一次同行的各成员轨迹使用相同 `session_id`，后续即可按该字段串联一次同行内所有人的轨迹；
- `track/create` 可在创建轨迹时直接写入 `session_id`；
- `/track/:track_id/update` 可在轨迹创建后补写 `session_id`，但与其它补全字段一致，仅当原值为空时写入。

### 6.10 附近 active 房间列表

- `GET /api/v1/companion/session/nearby`

用于客户端「同行首页」轮播卡片：

- 入参：`latitude` / `longitude`（WGS84 必填）；`radius_m` 可选，默认 5km，最大 20km；`limit` 可选，默认/最大 50；
- 返回半径内所有 `status=active` 且 `visibility=public` 的房间，按距离升序：
  - 仅公开房间：`visibility=private` 不会出现在该列表中，避免向陌生用户暴露私密房间；
  - 锚点 = owner 最新位置（来自 EMQX 上行的 `companion_live_positions`），用 Haversine 估算距离；
  - owner 尚未上传位置的房间无法估算距离，跳过；
  - 不暴露锚点经纬度，仅返回 `anchor.distance_m + anchor.recorded_at`，避免反向定位；
  - 不过滤已满 / 已加入的房间，由前端展示已满灰态、跳过自己已加入的房间；
- 每条记录返回 `session_id` / `title` / `track_type` / `locate_addr` / `join_token` / `max_members` / `member_count` / `started_at` / `expires_at` / `anchor` / `members`，`members` 中标注 `role=owner` 用于客户端展示房主。

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
- `companion/{session_id}/member/{user_id}/danmaku`（**客户端上行**：仅本人 publish）
- `companion/{session_id}/danmaku`（**服务端下行**：仅服务端 publisher publish，客户端 subscribe）
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
      "control_subscribe": "companion/sess_xxx/control",
      "danmaku_publish": "companion/sess_xxx/member/1001/danmaku",
      "danmaku_subscribe": "companion/sess_xxx/danmaku"
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
  - 成员 publish 自己的 `location` / `presence` / `danmaku`（上行）topic；
  - 成员 subscribe 当前 session 的位置通配 topic、presence 通配 topic、control topic、`danmaku` 广播 topic；
  - 成员 **不允许** publish `companion/{session_id}/danmaku`（广播 topic 仅服务端可发）。

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
- 所有人离开导致 auto-end 时，也会发布 `session_ended`；
- `companion_session_autoclose` 兜底结束 session 时，也会发布 `session_ended`。

实现位置：

- `CompanionService.LeaveSession(...)`
- `CompanionService.endSessionInternal(...)`
- `CompanionService.publishControlEvent(...)`

说明：

- 发布为 **best-effort**，若 Broker 不可达，仅记日志，不阻塞控制面主流程；
- 推荐 control 消息使用 **QoS 1**；
- control 消息 **不使用 retained**。

#### 8.3.7 弹幕上行写回接口

- `POST /api/v1/internal/companion/mqtt/danmaku-ingest`

说明：

- 供 EMQX Rule Engine 在收到上行弹幕消息（`companion/{session_id}/member/{user_id}/danmaku`）后回调；
- 服务端会通过 `client_id / username` 复核 MQTT principal，确保 `session_id / user_id` 与凭证绑定一致，否则返回 403；
- 校验 session 仍为 `active`、当前用户为 `joined` 成员；
- 内容做 UTF-8 长度限制（≤ 200 字符），并做滚动窗口限速（单成员 10 秒内最多 5 条）；
- 校验通过后落库，并通过服务端 publisher 向 `companion/{session_id}/danmaku` 广播；
- 入参字段：`session_id / user_id / content / client_id / username`。

请求体示例：

```json
{
  "session_id": "sess_xxx",
  "user_id": 1001,
  "content": "加油！",
  "client_id": "cmp-sess_xxx-1001-abc123",
  "username": "cmpv1:sess_xxx:1001:1770000000:abc123"
}
```

### 8.4 快照与实时流的关系

- 新成员加入：先拉 HTTP Snapshot；
- 再连接 EMQX 订阅增量流；
- `seq` / `recorded_at` 由客户端做去重和乱序保护。

### 8.5 弹幕（同行文字弹幕）

#### 8.5.1 设计目标

- 在 session 内任意成员之间相互发送短文本弹幕；
- 服务端做内容校验、限速、持久化（用于审计/回溯，不出现在 snapshot 中）；
- 失败检测采用 **方案 A**：客户端 publish 后启动超时（建议 3s），收到自身广播即判定成功，超时未收到则视为失败。

#### 8.5.2 数据流（Plan A）

```
Client publish → companion/{sid}/member/{uid}/danmaku
  → EMQX Rule Engine → POST /api/v1/internal/companion/mqtt/danmaku-ingest
  → Server 校验/限速/落库
  → Server publisher publish → companion/{sid}/danmaku
  → 所有成员（含发送者）subscribe 收到
```

#### 8.5.3 服务端约束

- 内容长度：≤ 200 字符（按 UTF-8 rune 计数），首尾空白会被去除；
- 内容审核：通过 `internal/config/sensitive_words.json` 内置敏感词词库（go:embed）做大小写不敏感的子串扫描，命中即整条拒绝（`400 content contains sensitive content`）；命中词只写服务端日志，不向客户端暴露，避免被反向枚举；
- 单成员限速：10 秒滚动窗口内最多 5 条，超出返回 `400 danmaku rate limit exceeded`；
- session 级限速：整个会话所有成员合计 10 秒滚动窗口内最多 50 条，超出返回 `400 session danmaku rate limit exceeded`；
- 会话开关：owner 可通过 `POST /api/v1/companion/session/:session_id/danmaku/toggle` 关闭整场会话的弹幕，关闭状态下 ingest 直接返回 `400 danmaku disabled`，并通过 control topic 广播 `danmaku_toggled` 事件；
- principal 复核：`session_id / user_id` 必须与 `client_id / username` 绑定一致，否则返回 403；
- 必须为 `active` session 中的 `joined` 成员，否则返回 404 / 403；
- 广播 publisher 回退顺序：`SetDanmakuPublisher` 注入的 publisher → `controlPublisher` → 仅记日志（best-effort）；
- 落库表：`companion_danmaku`，主键 `id`，索引 `(session_id, user_id, created_at)`，session 级限速复用 `idx_companion_danmaku_session_time(session_id, created_at)`；
- 数据保留：会话结束（`status=ended`）后弹幕默认保留 7 天，由 `internal/scheduler/jobs.DanmakuCleanup` 任务每天 03:00 批量清理（可通过 `DANMAKU_CLEANUP_CRON` / `DANMAKU_RETENTION_DAYS` 调整；调度器自身受 `SCHEDULER_ENABLED=true` 控制）。

#### 8.5.4 客户端广播消息格式

Topic：`companion/{session_id}/danmaku`

```json
{
  "message_id": 12345,
  "session_id": "sess_xxx",
  "user_id": 1001,
  "nickname": "Tom",
  "avatar_url": "/api/v1/static/avatars/1001.png",
  "content": "加油！",
  "created_at": "2026-05-23T18:10:00Z"
}
```

字段说明：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `message_id` | int64 | 服务端落库自增 ID，可用于客户端去重 |
| `session_id` | string | 同行会话 ID |
| `user_id` | int64 | 发送者 user_id |
| `nickname` | string | 发送者昵称（服务端从用户信息取） |
| `avatar_url` | string | 发送者头像 URL（按现有头像缓存策略改写） |
| `content` | string | 弹幕文本 |
| `created_at` | string | 服务端落库时间，RFC3339 |

#### 8.5.5 客户端约定（Plan A 失败检测）

1. 用户输入文本，本地先做长度（≤ 200）和频率预校验；
2. 客户端 publish 到 `companion/{session_id}/member/{user_id}/danmaku`，QoS 1；
3. 同时启动 **3s** 超时计时器；
4. 在订阅的 `companion/{session_id}/danmaku` 上等待：
   - 收到 `user_id == 当前用户` 且 `content` 与本次发送匹配的消息 → 标记发送成功；
   - 超时未收到 → 标记发送失败，UI 提示用户重试；
5. 发送过程中其它成员的弹幕正常实时上屏；
6. 不要将 `companion/{sid}/danmaku` 设为 retained，断线重连后只展示新到的弹幕，历史弹幕不补发。

#### 8.5.6 EMQX Rule Engine 配置（弹幕）

推荐从 topic 抽取后回调：

- topic 模式：`companion/+/member/+/danmaku`
- URL：`POST http://<app-server>/api/v1/internal/companion/mqtt/danmaku-ingest`
- Header：`X-Internal-Token: ${COMPANION_MQTT_INTERNAL_TOKEN}`
- 透传字段：`session_id / user_id / content / client_id / username`

### 8.6 同行自动收尾任务

`internal/scheduler/jobs.CompanionAutoClose` 注册任务名为 `companion_session_autoclose`，默认每 10 分钟执行一次；调度器本身仍受 `SCHEDULER_ENABLED=true` 控制。

自动结束条件：

1. active session 超过运动类型最大持续时间；
2. 或所有 `member_status=joined` 成员都超过运动类型无活动阈值；
3. 或 session 已无任何 `joined` 成员。

成员最后活跃时间：

```text
last_activity_at = max(member.last_seen_at, latest_position.recorded_at, member.joined_at)
```

内置策略写在代码中，不通过启动参数配置：

| 运动类型 code | 展示名 | 全员无活动超时 | 最大持续时长 |
| --- | --- | --- | --- |
| `running` | 跑步 | 30 分钟 | 8 小时 |
| `hiking` | 徒步 | 30 分钟 | 16 小时 |
| `climbing` | 爬山 | 45 分钟 | 24 小时 |
| `riding` | 骑行 | 30 分钟 | 24 小时 |
| `driving` | 自驾 | 60 分钟 | 72 小时 |
| 未知类型 | - | 45 分钟 | 24 小时 |

控制面返回的 `session.expires_at` 与附近房间列表的 `expires_at` 使用同一套最大持续时间规则计算：

```text
expires_at = session.started_at + MaxDuration(track_type)
```

`expires_at` 仅表示 session 的硬到期时间，不等同于 MQTT credentials 的 `expires_at`。

自动结束会复用同行结束逻辑，更新 session / member 状态并发布 `session_ended` control 事件：

- 全员无活动：`reason=inactive_timeout`
- 超过最大持续时间：`reason=max_duration_exceeded`
- 无 joined 成员：`reason=all_members_left`

为便于后台审计，`companion_sessions` 会持久化结束审计字段：

| 字段 | 说明 |
| --- | --- |
| `end_reason` | 结束原因：`owner_ended` / `all_members_left` / `inactive_timeout` / `max_duration_exceeded` |
| `end_source` | 结束来源：`owner` / `member_flow` / `auto_close`；自动收尾任务关闭的会话固定为 `auto_close` |
| `end_operator_user_id` | 结束操作用户 ID；自动收尾任务为 `0` |

### 8.7 control topic 消息格式

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
  "event": "member_kicked",
  "session_id": "sess_xxx",
  "member_user_id": 1002,
  "operator_user_id": 1001,
  "reason": "member_kicked",
  "at": "2026-05-23T18:03:00Z"
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

或（弹幕开关变更，由 owner 调用 `POST /companion/session/:session_id/danmaku/toggle` 触发）：

```json
{
  "event": "danmaku_toggled",
  "session_id": "sess_xxx",
  "operator_user_id": 1001,
  "reason": "danmaku_disabled",
  "enabled": false,
  "at": "2026-05-23T18:10:00Z"
}
```

字段说明：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `event` | string | 事件类型：`member_left` / `member_kicked` / `session_ended` / `danmaku_toggled` |
| `session_id` | string | 同行会话 ID |
| `member_user_id` | int64 | 被影响成员 user_id，仅 `member_left` / `member_kicked` 必有 |
| `operator_user_id` | int64 | 发起操作的用户；auto-end 时可为 `0` |
| `reason` | string | 原因，当前可能为 `member_left` / `member_kicked` / `owner_ended` / `all_members_left` / `inactive_timeout` / `max_duration_exceeded` / `danmaku_enabled` / `danmaku_disabled` |
| `enabled` | bool | 仅 `danmaku_toggled` 携带，反映切换后的目标状态 |
| `at` | string | 事件时间，RFC3339 |

客户端约定：

- 收到 `member_left`：将对应成员从当前实时展示集合移除，并停止展示其最后位置；
- 收到 `member_kicked`：
  - 若 `member_user_id` 是当前用户：立即停止位置 / presence / 弹幕上行，取消所有 companion topic 订阅，退出同行 UI，并提示已被房主移出；
  - 若 `member_user_id` 是其他成员：将该成员从当前实时展示集合移除，并停止展示其最后位置；
- 收到 `session_ended`：立即停止当前位置上传、取消所有 companion topic 订阅、退出同行 UI，并回退到普通地图态；
- 若 `session_ended` 先于某些滞后 location 消息到达，以 `session_ended` 为准；
- control 消息只做会话控制，不作为快照真值来源；冷启动真值仍来自 HTTP Snapshot。

### 8.8 EMQX 推荐配置样例

以下为 **推荐联调方式**，重点是说明字段映射与回调方向；具体语法可按实际 EMQX 版本微调。

#### 8.8.1 HTTP AuthN

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

#### 8.8.2 HTTP AuthZ

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

#### 8.8.3 Rule Engine：位置消息写回

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

#### 8.8.4 Rule Engine：连接 / 断开事件写回

推荐监听连接生命周期事件：

- connected -> `presence_status=online`
- disconnected / client.disconnected -> `presence_status=offline`

统一回调：

- URL: `POST http://<app-server>/api/v1/internal/companion/mqtt/presence-ingest`
- Header: `X-Internal-Token: ${COMPANION_MQTT_INTERNAL_TOKEN}`

#### 8.8.5 服务端 control / 弹幕 publisher

App Server 作为一个普通 MQTT client 连接 EMQX，使用单独客户端身份：

- `COMPANION_MQTT_PUBLISHER_CLIENT_ID`
- `COMPANION_MQTT_PUBLISHER_USERNAME`
- `COMPANION_MQTT_PUBLISHER_PASSWORD`

其职责为：

- 向 `companion/{session_id}/control` 发布 `member_left` / `session_ended`；
- 向 `companion/{session_id}/danmaku` 发布弹幕广播（默认复用 `controlPublisher`，亦可通过 `SetDanmakuPublisher` 注入独立 publisher）；
- 不参与位置订阅与快照真值判断。

### 8.9 建议的环境变量

- `EMQX_BROKER_URL`（服务端 publisher 自用，建议内网）
- `EMQX_WEBSOCKET_URL`（服务端备用，建议内网）
- `EMQX_CLIENT_BROKER_URL`（下发给客户端连接 EMQX 的地址，建议公网；未配置时回落到 `EMQX_BROKER_URL`）
- `EMQX_CLIENT_WEBSOCKET_URL`（下发给客户端的 WSS 地址，建议公网；未配置时回落到 `EMQX_WEBSOCKET_URL`）
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
- 同行文字弹幕：上行 ingest（限速/校验/落库）+ 服务端广播（Plan A 失败检测）

### 暂未实现

- LWT / Broker 侧清理策略编排文档
- 踢人、owner 转移、多端登录冲突控制

---

## 10. 建议的后续迭代顺序

1. 在 EMQX 侧完成 HTTP AuthN / AuthZ 与 Rule Engine 实际配置
2. 联调 `control` topic 的客户端消费逻辑（`member_left` / `session_ended`）
3. 联调 LWT / disconnect 事件到 `presence-ingest`
4. 按需要扩展踢人、owner 转移、多端登录控制等能力
