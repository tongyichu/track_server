# 同行接口

> 公共请求、认证和错误响应见 [common.md](common.md)。内部 MQTT/EMQX 回调见 [companion-internal.md](companion-internal.md)。

## 17. 创建同行会话

创建一场新的“同行”会话，并返回 `join_token` 与初始 snapshot。

**需要认证**

### 请求

```http
POST /api/v1/companion/session/create
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "title": "周末同行",
  "track_type": "hiking",
  "locate_addr": "北京市海淀区颐和园",
  "max_members": 8,
  "visibility": "private"
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `title` | string | 否 | 会话标题，默认 `与友同行` |
| `track_type` | string | 否 | 运动类型 code（`hiking` / `running` / `climbing` / `riding` / `driving` 等，与 `track_types` 接口返回的 `type` 一致），默认空字符串 |
| `locate_addr` | string | 否 | 创建会话时的位置信息文本（由客户端逆地理获取），默认空字符串，最大 255 字符 |
| `max_members` | int | 否 | 最大成员数，默认 `8`，最小 `2`，最大 `32` |
| `visibility` | string | 否 | 可见性：`private`（默认，私密房间，必须凭 `join_token` 加入） / `public`（公开房间，可凭 `session_id` 加入，且会出现在「附近房间」列表中） |

### 响应

```json
{
  "code": 0,
  "data": {
    "session": {
      "session_id": "sess_xxx",
      "owner_user_id": 1001,
      "status": "active",
      "visibility": "private",
      "title": "周末同行",
      "track_type": "hiking",
      "locate_addr": "北京市海淀区颐和园",
      "max_members": 8,
      "started_at": "2026-05-23T16:00:00Z",
      "expires_at": "2026-05-24T08:00:00Z",
      "created_at": "2026-05-23T16:00:00Z",
      "updated_at": "2026-05-23T16:00:00Z"
    },
    "join": {
      "join_token": "abcd1234EFGH5678"
    },
    "snapshot": {
      "snapshot_at": "2026-05-23T16:00:00Z",
      "members": [],
      "positions": []
    }
  }
}
```

### `session` 字段补充

| 字段 | 类型 | 说明 |
|------|------|------|
| `session.expires_at` | string(datetime) | 同行会话硬到期时间，用于客户端展示 session 过期倒计时；由 `started_at + track_type 对应最大持续时间` 派生，不是 MQTT 凭证过期时间。 |

创建、加入、预览、获取当前会话、踢出成员、弹幕开关切换等返回 `session` 的同行控制面接口均包含 `expires_at`。附近公开房间列表在每条房间记录中也返回 `expires_at`。

### 错误响应

- `400 Bad Request`
  - `you already have a running track: {track_id}`
  - `you already have an active companion session: {title}`
  - `you already joined an active companion session: {title}`
  - `max_members must be >= 2`
  - `visibility must be private or public`
- `401 Unauthorized`
- `404 Not Found`
  - 当前用户不存在

---

## 18. 加入同行会话

通过 `join_token` 或 `session_id` 加入一场 active session，并立即返回成员/位置 snapshot。

- 私密房间（`visibility=private`）必须凭 `join_token` 加入；
- 公开房间（`visibility=public`）可凭 `session_id` 直接加入（典型场景：从「附近房间」卡片点击加入）；也支持继续凭 `join_token` 加入。

`join_token` 与 `session_id` 二选一，至少填一个。

**需要认证**

### 请求

```http
POST /api/v1/companion/session/join
Authorization: Bearer <token>
Content-Type: application/json
```

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

### 错误响应

- `400 Bad Request`
  - `join_token or session_id is required`
  - `companion session already ended`
  - `companion session is full`
  - `you already have a running track: {track_id}`
  - `you already joined an active companion session: {title}`
- `403 Forbidden`
  - 凭 `session_id` 试图加入私密房间
- `404 Not Found`
  - `join_token` / `session_id` 对应 session 不存在

---

## 18.1 预览同行会话

通过 `join_token` 查询一场 active session 的房间预览，但不加入 session、不写入成员资格，也不返回实时位置。

**需要认证**

### 请求

```http
GET /api/v1/companion/session/preview?join_token=abcd1234EFGH5678
Authorization: Bearer <token>
```

### 响应

返回：

- `session`：包含 `expires_at`，客户端可据此展示 session 过期倒计时
- `snapshot`：只包含当前 joined 成员资料；`positions` 与 `join_token` 不返回

### 错误响应

- `400 Bad Request`
  - `join_token is required`
  - `companion session already ended`
- `401 Unauthorized`
- `404 Not Found`
  - `join_token` 对应 session 不存在

---

## 19. 获取当前同行会话

返回当前登录用户已加入的 active session 及其 snapshot；`session.expires_at` 可用于客户端重连后恢复 session 过期倒计时。

**需要认证**

### 请求

```http
GET /api/v1/companion/session/current
Authorization: Bearer <token>
```

### 错误响应

- `404 Not Found`
  - 当前无 active session

---

## 20. 获取同行快照

返回指定 session 的当前 snapshot，仅允许该 session 的 joined 成员访问。

**需要认证**

### 请求

```http
GET /api/v1/companion/session/:session_id/snapshot
Authorization: Bearer <token>
```

### 成功响应

```json
{
  "code": 0,
  "data": {
    "snapshot_at": "2026-05-23T16:00:00Z",
    "join_token": "ab12cd34",
    "members": [],
    "positions": []
  }
}
```

### 字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `data.snapshot_at` | string(datetime) | 快照生成时间。 |
| `data.join_token` | string | 加入口令；仅 `status=active` 时返回，可用于邀请他人加入；会话结束自动失效。 |
| `data.members` | `CompanionMemberSnapshot[]` | 当前 `joined` 成员列表。 |
| `data.positions` | `CompanionLivePosition[]` | 当前 `joined` 成员的最近一条上报位置。 |

### 错误响应

- `400 Bad Request`
  - `session_id is required`
  - `companion session already ended`
- `403 Forbidden`
  - 当前用户不是该 session 的 joined 成员
- `404 Not Found`
  - session 不存在

---

## 21. 离开同行会话

当前成员主动离开 session。owner 在仍有其他 joined 成员时不能直接离开，需调用结束接口。

**需要认证**

### 请求

```http
POST /api/v1/companion/session/:session_id/leave
Authorization: Bearer <token>
```

### 成功响应

```json
{
  "code": 0,
  "data": {
    "status": "ok"
  }
}
```

### 错误响应

- `400 Bad Request`
  - `owner cannot leave active session while other members are joined; end session instead`
- `403 Forbidden`
  - 当前用户不是该 session 的 joined 成员

---

## 22. 结束同行会话

owner 主动结束一场 active session。

**需要认证**

### 请求

```http
POST /api/v1/companion/session/:session_id/end
Authorization: Bearer <token>
```

### 成功响应

```json
{
  "code": 0,
  "data": {
    "status": "ok"
  }
}
```

### 错误响应

- `403 Forbidden`
  - 当前用户不是 owner
- `404 Not Found`
  - session 不存在

---

## 22.1 踢出同行成员

owner 将某个 joined 成员踢出本次 active session。踢出成功后，服务端会通过 `companion/{session_id}/control` 广播 `member_kicked` 事件；被踢用户后续不能继续获取当前 session、snapshot 或 MQTT 凭证。

**需要认证**

### 请求

```http
POST /api/v1/companion/session/:session_id/members/:user_id/kick
Authorization: Bearer <token>
```

### 成功响应

返回标准 `CompanionSessionState`，其中 `snapshot.members` 与 `snapshot.positions` 已不包含被踢成员。

```json
{
  "code": 0,
  "data": {
    "session": {
      "session_id": "sess_xxx",
      "owner_user_id": 1001,
      "status": "active",
      "visibility": "private",
      "title": "周末同行",
      "track_type": "hiking",
      "locate_addr": "北京市海淀区颐和园",
      "max_members": 8,
      "danmaku_enabled": true,
      "started_at": "2026-05-23T16:00:00Z",
      "expires_at": "2026-05-24T08:00:00Z",
      "created_at": "2026-05-23T16:00:00Z",
      "updated_at": "2026-05-23T16:00:00Z"
    },
    "join": {
      "join_token": "abcd1234EFGH5678"
    },
    "snapshot": {
      "snapshot_at": "2026-05-23T16:20:00Z",
      "join_token": "abcd1234EFGH5678",
      "members": [],
      "positions": []
    }
  }
}
```

### 错误响应

- `400 Bad Request`
  - `session_id is required`
  - `user_id is required`
  - `owner cannot kick self`
  - `companion session already ended`
  - `companion member is not joined`
- `403 Forbidden`
  - 当前用户不是 owner
- `404 Not Found`
  - session 不存在或被踢成员不存在

---

## 22.2 更新已结束同行摘要（owner）

owner 在同行结束后补写本次同行的汇总数据。该接口只允许 session owner 调用，且仅允许更新 `status=ended` 的会话。

**需要认证**

### 请求

```http
PUT /api/v1/companion/session/:session_id/update
Authorization: Bearer <token>
Content-Type: application/json
```

请求体至少携带一个字段：

```json
{
  "locate_addr": "北京市海淀区颐和园",
  "total_distance": 12345.6,
  "total_duration": 3661,
  "track_screenshot_url": "https://<bucket>.oss-<region>.aliyuncs.com/prod/companion/.../xxx.png",
  "actual_participant_count": 2
}
```

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `locate_addr` | string | 否 | 本次同行地点文本，类似 `track_records.locate_addr`。 |
| `total_distance` | number | 否 | 本次同行总里程，单位米，必须 `>= 0`。 |
| `total_duration` | int64 | 否 | 本次同行总耗时，单位秒，必须 `>= 0`。 |
| `track_screenshot_url` | string | 否 | 本次同行轨迹截图 OSS 地址；响应会改写为服务端本地静态下载链接。 |
| `actual_participant_count` | int64 | 否 | 本次实际参与同行的成员人数，必须 `>= 0`。 |

### 成功响应

返回更新后的 `CompanionSession`：

```json
{
  "code": 0,
  "data": {
    "session_id": "sess_xxx",
    "owner_user_id": 1001,
    "status": "ended",
    "locate_addr": "北京市海淀区颐和园",
    "total_distance": 12345.6,
    "total_duration": 3661,
    "track_screenshot_url": "/api/v1/static/screenshots/companion_sess_xxx.png",
    "actual_participant_count": 2
  }
}
```

### 错误响应

- `400 Bad Request`
  - `nothing to update`
  - `total_distance must be >= 0`
  - `total_duration must be >= 0`
  - `actual_participant_count must be >= 0`
  - `companion session is not ended`
- `403 Forbidden`
  - 当前用户不是 owner
- `404 Not Found`
  - session 不存在

---

## 22.3 上报同行关键事件（owner）

owner 记录同行过程中的关键事件，例如成员离开、成员断线、成员重连、同行周知消息、到达关键点、风险提醒或自定义事件。该事件时间线只允许 session owner 写入和查询。

**需要认证**

### 请求

```http
POST /api/v1/companion/session/:session_id/events
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "event_type": "member_disconnected",
  "target_user_id": 1002,
  "title": "成员断线",
  "content": "Jerry 已超过 30 秒无位置更新",
  "event_time": "2026-06-09T10:20:30Z",
  "client_event_id": "ios-uuid-xxx",
  "metadata": {
    "last_seen_seconds": 32
  }
}
```

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `event_type` | string | 是 | 事件类型：`member_left` / `member_disconnected` / `member_reconnected` / `notice_sent` / `checkpoint_reached` / `risk_reported` / `custom`。 |
| `target_user_id` | int64 | 否 | 事件关联成员 user_id；无关联成员传 `0` 或不传。传入大于 0 时，该用户必须属于该 session。 |
| `title` | string | 否 | 事件标题，最多 64 个字符。 |
| `content` | string | 否 | 事件内容，最多 500 个字符。 |
| `event_time` | string(datetime) | 否 | 事件发生时间；不传时服务端取当前时间。不能早于 `started_at - 5min`，不能晚于服务端当前时间 `+1min`；已结束会话还不能晚于 `ended_at + 5min`。 |
| `client_event_id` | string | 是 | 客户端幂等事件 ID，建议 UUID；同一 session 内重复上报同一个值会返回已有事件，不重复写入。最大 128 字符。 |
| `metadata` | object | 否 | 客户端扩展 JSON 对象，最大 2048 bytes；服务端仅存储和透传，不解释业务含义。 |

单个 `session_id` 最多写入 100 条关键事件。达到上限后，新 `client_event_id` 会返回 `400 companion event limit exceeded`；重复上报已存在的 `client_event_id` 仍返回已有事件。

### 成功响应

```json
{
  "code": 0,
  "data": {
    "id": 12345,
    "session_id": "sess_xxx",
    "owner_user_id": 1001,
    "event_type": "member_disconnected",
    "target_user_id": 1002,
    "title": "成员断线",
    "content": "Jerry 已超过 30 秒无位置更新",
    "event_time": "2026-06-09T10:20:30Z",
    "client_event_id": "ios-uuid-xxx",
    "metadata": {
      "last_seen_seconds": 32
    },
    "created_at": "2026-06-09T10:20:31Z"
  }
}
```

### 错误响应

- `400 Bad Request`
  - `session_id is required`
  - `client_event_id is required`
  - `client_event_id is too long`
  - `invalid event_type`
  - `title exceeds 64 characters`
  - `content exceeds 500 characters`
  - `target_user_id must be >= 0`
  - `event_time is too early`
  - `event_time is too late`
  - `invalid metadata`
  - `metadata must be an object`
  - `metadata exceeds 2048 bytes`
  - `companion event limit exceeded`
- `403 Forbidden`
  - 当前用户不是 owner
- `404 Not Found`
  - session 不存在或 `target_user_id` 不属于该 session

---

## 22.4 查询同行关键事件时间线（owner）

owner 按时间线查询自己上报过的同行关键事件，支持游标分页。

**需要认证**

### 请求

```http
GET /api/v1/companion/session/:session_id/events?limit=50&cursor=<cursor>&order=asc
Authorization: Bearer <token>
```

### Query 参数

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `limit` | int | 否 | 每页数量，默认 `20`，最大 `50`。 |
| `cursor` | string | 否 | 分页游标，首次请求不传；后续将上次返回的 `next_cursor` 原样透传。 |
| `order` | string | 否 | 排序方向：`asc` / `desc`，默认 `asc`。排序键为 `event_time, id`。 |

### 响应

```json
{
  "code": 0,
  "data": {
    "items": [
      {
        "id": 12345,
        "session_id": "sess_xxx",
        "owner_user_id": 1001,
        "event_type": "notice_sent",
        "target_user_id": 0,
        "title": "同行周知",
        "content": "前方补给点休整 10 分钟",
        "event_time": "2026-06-09T10:30:00Z",
        "client_event_id": "ios-uuid-yyy",
        "metadata": {
          "level": "info"
        },
        "created_at": "2026-06-09T10:30:01Z"
      }
    ],
    "next_cursor": "eyJldmVudF90aW1lIjoiMjAyNi0wNi0wOVQxMDozMDowMFoiLCJpZCI6MTIzNDV9",
    "has_more": true
  }
}
```

### 错误响应

- `400 Bad Request`
  - `invalid limit`
  - `invalid cursor`
  - `invalid order`
- `403 Forbidden`
  - 当前用户不是 owner
- `404 Not Found`
  - session 不存在

---

## 23. 当前用户参与过的同行记录列表

返回当前登录用户参与过的同行记录列表，支持分页。

**需要认证**

### 请求

```http
GET /api/v1/companion/session/history?limit=20&cursor=<cursor>
Authorization: Bearer <token>
```

### Query 参数

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `limit` | int | 否 | 每页数量，默认 `20`，最大 `50` |
| `cursor` | string | 否 | 分页游标，首次请求不传；后续将上次返回的 `next_cursor` 原样透传 |

### 响应

```json
{
  "code": 0,
  "data": {
    "items": [
      {
        "session_id": "sess_xxx",
        "title": "周末同行",
        "track_type": "hiking",
        "locate_addr": "北京市海淀区颐和园",
        "participant_count": 3,
        "started_at": "2026-05-23T16:00:00Z",
        "duration_seconds": 7200,
        "total_distance": 12345.6,
        "total_duration": 3661,
        "track_screenshot_url": "/api/v1/static/screenshots/companion_sess_xxx.png",
        "actual_participant_count": 2,
        "status": "active",
        "join_token": "ab12cd34",
        "participants": [
          {
            "user_id": 1001,
            "nickname": "Tom",
            "avatar_url": "/api/v1/static/default_avatars/boy_01.png"
          },
          {
            "user_id": 1002,
            "nickname": "Jerry",
            "avatar_url": "https://example.com/avatar/1002.png"
          }
        ]
      }
    ],
    "total_count": 12,
    "next_cursor": "eyJzdGFydGVkX2F0IjoiMjAyNi0wNS0yM1QxNjowMDowMFoiLCJzZXNzaW9uX2lkIjoic2Vzc194eHgifQ",
    "has_more": true
  }
}
```

### 字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `data.items` | `CompanionHistoryItem[]` | 当前页记录列表，按 `started_at DESC, session_id DESC` 排序。 |
| `data.items[].session_id` | string | 同行会话 ID。 |
| `data.items[].title` | string | 同行标题。 |
| `data.items[].track_type` | string | 运动类型 code（`hiking` / `running` / `climbing` / `riding` / `driving` 等），创建会话时传入；若未设置则为空字符串。 |
| `data.items[].locate_addr` | string | 同行地点文本；创建会话时可传入，owner 也可在结束后通过更新接口补写。 |
| `data.items[].participant_count` | int64 | 该场同行人数：若 `status=ended`，返回参与过的人数；若 `status=active`，返回当前仍为 `joined` 的人数。 |
| `data.items[].started_at` | string(datetime) | 同行开始时间。 |
| `data.items[].duration_seconds` | int64 | 同行总耗时（秒）：`status=ended` 取 `ended_at - started_at`；`status=active` 取 `now - started_at`。 |
| `data.items[].total_distance` | number | owner 通过更新接口补写的本次同行总里程，单位米；未补写时为 `0`。 |
| `data.items[].total_duration` | int64 | owner 通过更新接口补写的本次同行总耗时，单位秒；未补写时为 `0`。 |
| `data.items[].track_screenshot_url` | string | owner 通过更新接口补写的轨迹截图，返回服务端本地静态下载链接 `/api/v1/static/screenshots/...`；未补写时为空字符串。 |
| `data.items[].actual_participant_count` | int64 | owner 通过更新接口补写的实际参与同行人数；未补写时为 `0`。 |
| `data.items[].status` | string | 同行状态：`active` / `ended`。 |
| `data.items[].join_token` | string | 加入口令；仅 `status=active` 时返回，可用于邀请他人加入；`ended` 时不返回此字段。 |
| `data.items[].participants` | `CompanionHistoryParticipant[]` | 人员列表口径与 `participant_count` 一致：若 `status=ended` 返回参与过的人员；若 `status=active` 返回当前仍为 `joined` 的人员。 |
| `data.items[].participants[].user_id` | int64 | 参与人 user_id。 |
| `data.items[].participants[].nickname` | string | 参与人昵称。 |
| `data.items[].participants[].avatar_url` | string | 参与人头像链接；若用户未设置头像，则返回默认头像。 |
| `data.total_count` | int64 | 当前用户参与过的同行总数。 |
| `data.next_cursor` | string | 下一页游标；若为空表示没有下一页。 |
| `data.has_more` | bool | 是否还有下一页。 |

### 错误响应

- `400 Bad Request`
  - `invalid limit`
  - `invalid cursor`
- `401 Unauthorized`
  - 缺少/无效/过期的 Token

---

## 24. 获取同行 MQTT 凭证

为当前登录用户签发当前 session 的短期 MQTT 连接凭证。

**需要认证**

### 请求

```http
POST /api/v1/companion/session/:session_id/mqtt/credentials
Authorization: Bearer <token>
```

### 响应

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

### 错误响应

- `403 Forbidden`
  - 当前用户不是该 session 的 joined 成员
- `404 Not Found`
  - session 不存在
- `503 Service Unavailable`
  - `companion mqtt not configured`

---

### 27.4 弹幕开关切换（owner）

供同行会话 owner 开启或关闭整场会话的弹幕能力。关闭后所有成员均无法发送，
服务端通过 `companion/{session_id}/control` topic 广播 `danmaku_toggled` 事件，
客户端据此即时更新 UI（禁用/恢复输入框）。

```http
POST /api/v1/companion/session/:session_id/danmaku/toggle
Authorization: Bearer <jwt>
Content-Type: application/json
```

```json
{
  "enabled": false
}
```

请求字段：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `enabled` | bool | 是 | 目标状态：true=开启，false=关闭 |

成功响应：返回标准 `CompanionSessionState`，其中 `session.danmaku_enabled` 已是更新后的值。

广播事件（服务端发布到 `companion/{session_id}/control`）：

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
| `event` | string | 固定为 `danmaku_toggled` |
| `operator_user_id` | int64 | 操作者 user_id（owner） |
| `reason` | string | `danmaku_enabled` / `danmaku_disabled` |
| `enabled` | bool | 切换后的目标状态 |

错误响应：

- `400 Bad Request`
  - `enabled is required`（请求体缺失或字段为 null）
  - `companion session already ended`（session 非 active）
- `403 Forbidden`：调用者不是 session owner
- `404 Not Found`：session 不存在

幂等：当 `enabled` 与当前状态一致时，服务端不更新 DB、不广播事件，直接返回当前 state。

会话 snapshot / state 中的 `danmaku_enabled` 字段（位于 `session` 对象内）反映当前开关状态，
客户端进入会话时可据此初始化 UI。

---

## 29. 附近 active 同行房间列表

返回当前位置附近所有 `status=active` 且 `visibility=public` 的同行房间，供客户端「同行首页」轮播卡片展示。

设计要点：

- 仅公开房间：`visibility=private` 的房间不会出现在该列表中，避免向陌生用户暴露私密房间的 `session_id` / `join_token`。
- 锚点位置：服务端取每个 session 中 owner 的最新位置（来自 EMQX 上行的 `companion_live_positions`）作为该房间的定位锚点；owner 尚未上传任何位置的房间会被跳过。
- 距离估算：使用 Haversine 公式计算锚点与请求位置的球面距离，过滤超过半径的房间。
- 隐私保护：响应只返回 `distance_m + recorded_at`，不暴露房间锚点经纬度，避免反向定位。
- 全量返回：不过滤已满 / 当前用户已加入的房间，由前端展示已满灰态、跳过自己已加入的房间。

**需要认证**

### 请求

```http
GET /api/v1/companion/session/nearby?latitude=39.9087&longitude=116.3975&radius_m=5000&limit=50
Authorization: Bearer <token>
```

### Query 参数

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `latitude` | float | 是 | 客户端当前纬度（WGS84），范围 `[-90, 90]`。 |
| `longitude` | float | 是 | 客户端当前经度（WGS84），范围 `[-180, 180]`。 |
| `radius_m` | float | 否 | 搜索半径（米），默认 `5000`，最大 `20000`；超过最大值时按 `20000` 处理。 |
| `limit` | int | 否 | 返回上限，默认 / 最大 `50`；超过最大值时按 `50` 处理。 |

### 响应

```json
{
  "code": 0,
  "data": {
    "items": [
      {
        "session_id": "sess_xxx",
        "title": "周末同行",
        "track_type": "hiking",
        "locate_addr": "北京市海淀区颐和园",
        "join_token": "ab12cd34",
        "max_members": 8,
        "member_count": 3,
        "total_distance": 0,
        "total_duration": 0,
        "track_screenshot_url": "",
        "actual_participant_count": 0,
        "started_at": "2026-05-23T16:00:00Z",
        "expires_at": "2026-05-24T08:00:00Z",
        "anchor": {
          "distance_m": 1234.56,
          "recorded_at": "2026-05-23T16:19:58Z"
        },
        "members": [
          {
            "user_id": 1001,
            "role": "owner",
            "nickname": "Tom",
            "avatar_url": "/api/v1/static/avatars/1001.png"
          },
          {
            "user_id": 1002,
            "role": "member",
            "nickname": "Jerry",
            "avatar_url": "/api/v1/static/default_avatars/boy_01.png"
          }
        ]
      }
    ],
    "radius_m": 5000,
    "center_at": "2026-05-23T18:30:00Z"
  }
}
```

### 字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `data.items` | `CompanionNearbyItem[]` | 命中半径的 active 房间列表，按 `distance_m` 升序。 |
| `data.items[].session_id` | string | 同行会话 ID。 |
| `data.items[].title` | string | 同行标题。 |
| `data.items[].track_type` | string | 运动类型 code（`hiking` / `running` / `climbing` / `riding` / `driving` 等），创建会话时传入；若未设置则为空字符串。 |
| `data.items[].locate_addr` | string | 同行地点文本；创建会话时可传入，owner 也可在结束后通过更新接口补写。 |
| `data.items[].join_token` | string | 加入口令，可用于直接调用 `/companion/session/join`。 |
| `data.items[].max_members` | int | 房间最大人数。 |
| `data.items[].member_count` | int | 当前 `member_status=joined` 的成员数；前端可结合 `max_members` 判断是否已满。 |
| `data.items[].total_distance` | number | 本次同行总里程，单位米；通常由 owner 在会话结束后补写，active 房间未补写时为 `0`。 |
| `data.items[].total_duration` | int64 | 本次同行总耗时，单位秒；通常由 owner 在会话结束后补写，active 房间未补写时为 `0`。 |
| `data.items[].track_screenshot_url` | string | 本次同行轨迹截图，返回服务端本地静态下载链接 `/api/v1/static/screenshots/...`；未补写时为空字符串。 |
| `data.items[].actual_participant_count` | int64 | 本次实际参与同行人数；通常由 owner 在会话结束后补写，未补写时为 `0`。 |
| `data.items[].started_at` | string(datetime) | 会话开始时间。 |
| `data.items[].expires_at` | string(datetime) | 同行会话硬到期时间；由 `started_at + track_type 对应最大持续时间` 派生，用于房间卡片展示过期倒计时。 |
| `data.items[].anchor` | object | 距离锚点信息（owner 最新位置投影后的距离 + 采样时间）；若 owner 尚未上传位置，该房间不会出现在列表中。 |
| `data.items[].anchor.distance_m` | float | 锚点与请求位置的球面距离（米）。 |
| `data.items[].anchor.recorded_at` | string(datetime) | 锚点对应位置的采样时间，客户端可据此评估锚点新鲜度。 |
| `data.items[].members` | `CompanionNearbyMember[]` | 当前 `joined` 成员列表，owner 排在前。 |
| `data.items[].members[].user_id` | int64 | 成员 user_id。 |
| `data.items[].members[].role` | string | `owner` / `member`，前端可据此在卡片上标注房主。 |
| `data.items[].members[].nickname` | string | 成员昵称。 |
| `data.items[].members[].avatar_url` | string | 成员头像；若用户未设置头像，则返回默认头像。 |
| `data.radius_m` | float | 服务端实际使用的搜索半径（米）；用于客户端展示。 |
| `data.center_at` | string(datetime) | 服务端处理本次请求的时间，可与 `anchor.recorded_at` 结合判断锚点滞后。 |

### 错误响应

- `400 Bad Request`
  - `latitude and longitude are required`
  - `invalid latitude` / `invalid longitude`
  - `latitude must be in [-90, 90]` / `longitude must be in [-180, 180]`
  - `invalid radius_m`
  - `invalid limit`
- `401 Unauthorized`
  - 缺少 / 无效 / 过期的 Token

---


获取客户端创建/编辑轨迹时可选择的运动类型列表，并返回当前用户按运动类型拆分的最近一个月、最近一年统计数据。

**需要认证**

### 说明

- 默认返回五个内置运动类型，`type` 分别为 `hiking`、`running`、`climbing`、`riding`、`driving`，`name` 分别为 `徒步`、`跑步`、`爬山`、`骑行`、`自驾`。
- 服务端可通过环境变量 `TRACK_TYPES` 配置运动类型展示名，支持使用英文逗号、分号、中文逗号、顿号或竖线分隔，例如：`TRACK_TYPES=徒步,跑步,爬山,骑行,自驾,滑雪`。
- 服务端会自动过滤空项和重复项；若未配置或配置为空，则使用默认列表。
- 每个运动类型会返回一个图标链接 `icon_url`，图标文件来自服务端静态目录 `/api/v1/static/track_type_icon/`；默认对应关系为：`徒步 -> hiking.svg`、`跑步 -> running.svg`、`爬山 -> climbing.svg`、`骑行 -> riding.svg`、`自驾 -> driving.svg`。
- 每个运动类型会额外返回：`type`（英文标识）、`theme_color`（主题色）、`icon_anim_url`（Lottie 动画文件链接；当前默认空字符串，后续可通过配置补充）。
- 统计数据按 `Authorization` Token 解析出的当前用户统计 `track_records`：排除删除与进行中轨迹，仅统计 `正常/私密` 轨迹。
- 统计维度按运动类型拆分，并返回最近一个月（`month`）与最近一年（`year`）的总里程、轨迹次数、总耗时、总热量。

