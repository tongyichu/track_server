# 同行内部接口

> 本文件包含 `/internal/*`、EMQX 回调和 MQTT 数据面写回接口。公开业务接口见 [companion.md](companion.md)。

## 25. EMQX HTTP AuthN 回调

供 EMQX 在客户端 CONNECT 时调用，校验 `clientid / username / password` 是否有效。

**内部接口**

### 请求

```http
POST /api/v1/internal/mqtt/auth
X-Internal-Token: <internal-token>
Content-Type: application/json
```

```json
{
  "clientid": "cmp-sess_xxx-1001-abc123",
  "username": "cmpv1:sess_xxx:1001:1770000000:abc123",
  "password": "signed_password"
}
```

### 响应

```json
{
  "result": "allow",
  "is_superuser": false
}
```

`result` 可能为 `allow` 或 `deny`。

---

## 26. EMQX HTTP AuthZ 回调

供 EMQX 在 publish / subscribe 前调用，校验当前 MQTT 凭证是否允许访问指定 topic。

**内部接口**

### 请求

```http
POST /api/v1/internal/mqtt/acl
X-Internal-Token: <internal-token>
Content-Type: application/json
```

```json
{
  "clientid": "cmp-sess_xxx-1001-abc123",
  "username": "cmpv1:sess_xxx:1001:1770000000:abc123",
  "action": "publish",
  "topic": "companion/sess_xxx/member/1001/location"
}
```

### 响应

```json
{
  "result": "allow"
}
```

当前策略：

- 允许 publish 自己的 `location` / `presence` / `danmaku`（上行）topic；
- 允许 subscribe 当前 session 的 `member/+/location`、`member/+/presence`、`control` topic 与 `companion/{session_id}/danmaku` 广播 topic；
- 客户端 **不允许** publish `companion/{session_id}/danmaku`（仅服务端可发）；
- 其他 topic 返回 `deny`。

---

## 27. EMQX 数据面写回接口

供 EMQX Rule Engine / WebHook 将 MQTT 数据面事件写回 App Server。

**内部接口**

### 27.1 位置写回

```http
POST /api/v1/internal/companion/mqtt/location-ingest
X-Internal-Token: <internal-token>
Content-Type: application/json
```

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
  "recorded_at": "2026-05-23T16:19:58Z",
  "seq": 1024,
  "source": "mqtt_rule_engine",
  "client_id": "cmp-sess_xxx-1001-abc123",
  "username": "cmpv1:sess_xxx:1001:1770000000:abc123"
}
```

成功响应：

```json
{
  "result": "ok"
}
```

说明：

- 服务端会校验 `session_id / user_id` 是否与 MQTT principal 一致；
- 仅保留每位成员最新一条位置快照；
- 若 `seq` 更旧，或 `seq` 相同且 `recorded_at` 不更新，则忽略该消息。

### 27.2 Presence 写回

```http
POST /api/v1/internal/companion/mqtt/presence-ingest
X-Internal-Token: <internal-token>
Content-Type: application/json
```

```json
{
  "session_id": "sess_xxx",
  "user_id": 1001,
  "presence_status": "online",
  "last_seen_at": "2026-05-23T16:20:00Z",
  "client_id": "cmp-sess_xxx-1001-abc123",
  "username": "cmpv1:sess_xxx:1001:1770000000:abc123"
}
```

成功响应：

```json
{
  "result": "ok"
}
```

### 27.3 弹幕写回（同行文字弹幕）

供 EMQX Rule Engine 在收到客户端上行弹幕消息（`companion/{session_id}/member/{user_id}/danmaku`）后回调。
服务端会做 principal 复核、内容/限速校验、落库，并通过服务端 publisher 向 `companion/{session_id}/danmaku` 广播给所有成员（含发送者）。

```http
POST /api/v1/internal/companion/mqtt/danmaku-ingest
X-Internal-Token: <internal-token>
Content-Type: application/json
```

```json
{
  "session_id": "sess_xxx",
  "user_id": 1001,
  "content": "加油！",
  "client_id": "cmp-sess_xxx-1001-abc123",
  "username": "cmpv1:sess_xxx:1001:1770000000:abc123"
}
```

请求字段：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `session_id` | string | 是 | 同行会话 ID（由 Rule SQL 从 topic 提取） |
| `user_id` | int64 | 是 | 发送者 user_id（由 Rule SQL 从 topic 提取） |
| `content` | string | 是 | 弹幕内容；首尾空白会被去除，UTF-8 长度需 ≤ 200 |
| `client_id` | string | 是 | MQTT 客户端 ID，用于 principal 复核 |
| `username` | string | 是 | MQTT 用户名，用于 principal 复核 |

成功响应：

```json
{
  "result": "ok"
}
```

广播消息（服务端发布到 `companion/{session_id}/danmaku`）：

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

广播字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `message_id` | int64 | 服务端落库自增 ID，可用于客户端去重 |
| `session_id` | string | 同行会话 ID |
| `user_id` | int64 | 发送者 user_id |
| `nickname` | string | 发送者昵称 |
| `avatar_url` | string | 发送者头像 URL（按现有头像缓存策略改写） |
| `content` | string | 弹幕文本 |
| `created_at` | string(datetime) | 服务端落库时间，RFC3339 |

错误响应（HTTP 状态码语义）：

- `400 Bad Request`
  - `session_id is required` / `user_id is required` / `content is required`
  - `content exceeds 200 characters`
  - `content contains sensitive content`（命中本地敏感词词库；命中词不向客户端暴露）
  - `danmaku rate limit exceeded`（单成员 10 秒滚动窗口内最多 5 条）
  - `session danmaku rate limit exceeded`（整个 session 10 秒滚动窗口内最多 50 条）
  - `danmaku disabled`（owner 已通过 27.4 接口关闭弹幕）
- `401 Unauthorized` / `403 Forbidden`
  - 缺少或无效 `X-Internal-Token`
  - `client_id / username` 与 `session_id / user_id` 不匹配（principal 复核失败）
  - 当前用户不是该 session 的 `joined` 成员
- `404 Not Found`
  - session 不存在或已结束

客户端约定（Plan A 失败检测）：

- 客户端 publish 后启动 **3s** 超时计时器；
- 在订阅的 `companion/{session_id}/danmaku` 上收到 `user_id` 等于自身且 `content` 与本次匹配的消息 → 标记发送成功；
- 超时未收到 → 标记发送失败，UI 提示用户重试（失败原因可能是开关已关闭 / 命中敏感词 / 限速 / 网络抖动，UI 文案保持通用）；
- 广播 topic 不使用 retained，断线重连后只展示新到的弹幕，历史不补发。

