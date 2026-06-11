# 收藏与导航接口

> 公共请求、认证和错误响应见 [common.md](common.md)；通用对象字段见 [models.md](models.md)。

## 5. 收藏轨迹

将指定轨迹加入当前用户的收藏列表。

- 返回统一响应格式 `StandardResponse`。
- 用户身份仅通过 `Authorization` 中的 JWT 解析，不需要也不应传递 `user_id`。
- 当前实现通过 Query 参数接收 `track_id`。

**需要认证**

### 请求

```
POST /api/v1/track_collect?track_id=:track_id
Authorization: Bearer <token>
```

**Query 参数：**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `track_id` | string | 是 | 要收藏的轨迹 ID |

### 响应

**状态码：** `200 OK`

```json
{
  "code": 0,
  "data": {
    "status": "ok"
  }
}
```

### 常见错误

- `400 Bad Request`
  - `track_id` 缺失
- `401 Unauthorized`
  - 缺少/无效/过期的 Token

---

## 6. 取消收藏轨迹

将指定轨迹从当前用户的收藏列表中移除。

- 返回统一响应格式 `StandardResponse`。
- 用户身份仅通过 `Authorization` 中的 JWT 解析，不需要也不应传递 `user_id`。
- 当前实现通过 Query 参数接收 `track_id`。

**需要认证**

### 请求

```
DELETE /api/v1/track_collect?track_id=:track_id
Authorization: Bearer <token>
```

**Query 参数：**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `track_id` | string | 是 | 要取消收藏的轨迹 ID |

### 响应

**状态码：** `200 OK`

```json
{
  "code": 0,
  "data": {
    "status": "ok"
  }
}
```

### 常见错误

- `400 Bad Request`
  - `track_id` 缺失
- `401 Unauthorized`
  - 缺少/无效/过期的 Token
- `500 Internal Server Error`
  - 服务端执行取消收藏失败

### 示例（curl）

```bash
curl -X DELETE "http://<host>:<port>/api/v1/track_collect?track_id=trk2" \
  -H "Authorization: Bearer <token>"
```

---

## 8. 导航使用上报

客户端在“使用别人的轨迹进行导航”后上报一次使用记录，服务端将据此统计 `navigate_count`。

**需要认证**

### 请求

```
POST /api/v1/track/:track_id/navigation/report
Authorization: Bearer <token>
```

#### Headers

| Header | 必填 | 说明 |
|--------|------|------|
| `Authorization` | 是 | `Bearer <token>`（从 Token 中解析当前用户身份） |

#### Query 参数

无

**Path 参数：**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `track_id` | string | 是 | 轨迹 ID |

#### Body

无（可传空 body）

### 响应

**状态码：** `200 OK`

```json
{
  "code": 0,
  "data": {
    "status": "ok"
  }
}
```

### 说明

- 仅统计“其他用户”使用该轨迹导航的次数（自己导航自己的轨迹不会计入）。
- 每次成功调用会新增一条“导航使用记录”，`navigate_count` 的统计口径为该轨迹对应记录条数的累计（不做去重）。
- 该接口**非幂等**：重复调用会重复计数；建议客户端侧自行做去重/限频（例如一次导航结束仅上报一次）。

### 错误响应

- `400 Bad Request`
  - 上报了自己创建的轨迹（不允许计入导航次数）
- `401 Unauthorized`
  - 缺少/无效/过期的 Token
- `404 Not Found`
  - 轨迹不存在
- `500 Internal Server Error`
  - 服务端写入导航使用记录失败

错误响应格式示例：

```json
{
  "error": "..."
}
```

---

## 13. 用户已收藏轨迹列表

获取当前登录用户已收藏的轨迹列表。

### 说明

- 列表按“收藏时间”倒序返回（收藏记录的 `created_at`）。
- 与推荐/搜索列表口径保持一致：仅返回 `status=1` 且 `is_running=false` 的轨迹。
- 返回结构与 [推荐轨迹列表](#2-推荐轨迹列表) 保持一致，但每个 item **不返回** `collected` 字段（因为该列表内的轨迹天然已收藏）。
- `cursor` 为服务端生成的分页游标，客户端应原样透传，不要解析。
- 响应中的 `total_count` 表示“收藏列表”总数（按本接口口径：仅统计 `status=1` 且 `is_running=false` 的轨迹）。

### 请求

```
GET /api/v1/track/collected/list?limit=20&cursor=<next_cursor>
Authorization: Bearer <token>
```

**Query 参数：**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `limit` | int | 否 | 每页数量，默认 `20`，最大 `50` |
| `cursor` | string | 否 | 分页游标（上一页返回的 `next_cursor`） |

### 响应

**状态码：** `200 OK`

返回统一响应格式 `StandardResponse`：

```json
{
  "code": 0,
  "data": {
    "items": [
      {
        "id": "NO.00000001",
        "user_id": 1001,
        "city_code": "330100",
        "locate_addr": "杭州市西湖区",
        "track_type": "hiking",
        "start_time": "2026-04-20T12:00:00Z",
        "end_time": "2026-04-20T13:30:00Z",
        "city_name": "杭州市",
        "nickname": "Alice",
        "user_avatar_url": "https://example.com/avatar.png",
        "title": "西湖徒步",
        "distance": 1200.5,
        "duration": 1800,
        "avg_speed_kmh": 12.34,
        "calories_burned": 96.5,
        "elevation_gain": 80,
        "collect_count": 10,
        "navigate_count": 2,
        "track_screenshot_url": "/api/v1/static/screenshots/NO.00000001.jpg",
        "track_no_map_bg_screenshot_url": "/api/v1/static/screenshots/NO.00000001_no_map_bg.jpg",
        "raw_track_url": "/api/v1/static/raw_tracks/NO.00000001.dat"
      }
    ],
    "total_count": 10,
    "next_cursor": "<opaque>",
    "has_more": true
  }
}
```

**字段说明：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `data.items` | `CollectedTrackSummary[]` | 当前页收藏的轨迹列表（字段与 `TrackSummary` 基本一致，但不返回 `collected`）。 |
| `data.total_count` | int64 | 收藏列表总数（按本接口口径）。 |
| `data.next_cursor` | string | 下一页游标；当 `has_more=false` 时为空或不返回。 |
| `data.has_more` | bool | 是否还有下一页数据。 |

### 错误响应

- `400 Bad Request`
  - `cursor` 非法（返回 `{"error":"invalid cursor"}`）
- `401 Unauthorized`
  - 缺少/无效/过期的 Token
- `500 Internal Server Error`
  - 服务端查询收藏列表失败


---

