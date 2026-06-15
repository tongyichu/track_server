# 轨迹接口

> 公共请求、认证和错误响应见 [common.md](common.md)；通用对象字段见 [models.md](models.md)。

## 1. 创建轨迹

创建一个新的轨迹记录；支持在请求体中直接传入轨迹摘要字段。未传的字段会使用默认值。

**需要认证**

### 请求

```
POST /api/v1/track/create
Content-Type: application/json
Authorization: Bearer <token>
```

**请求体：**（所有字段均为可选）

```json
{
  "title": "傍晚夜跑",
  "session_id": "sess_xxx",
  "city_code": "330100",
  "locate_addr": "杭州市西湖区",
  "track_type": "running",
  "source_tag": "manual_seed",
  "coordinate_system": "GCJ02",
  "start_time": "2026-04-20T12:00:00Z",
  "end_time": "2026-04-20T12:30:00Z",
  "distance": 1200.5,
  "duration": 1800,
  "calories_burned": 96.5,
  "elevation_gain": 80,
  "raw_track_url": "https://<bucket>.oss-<region>.aliyuncs.com/prod/track/.../xxx.dat",
  "track_screenshot_url": "https://<bucket>.oss-<region>.aliyuncs.com/prod/track/.../xxx.jpg",
  "track_no_map_bg_screenshot_url": "https://<bucket>.oss-<region>.aliyuncs.com/prod/track/.../xxx_no_map_bg.jpg",
  "is_running": false,
  "avg_speed_kmh": 12.3
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `title` | string | 否 | 轨迹标题，默认 `新的轨迹` |
| `session_id` | string | 否 | 关联的同行会话 ID；参加同行结束后上传个人轨迹时传同一个 `session_id`，用于把本次同行内多人轨迹串联起来 |
| `city_code` | string | 否 | 城市 Code（标识轨迹所属城市） |
| `locate_addr` | string | 否 | 轨迹的具体位置信息，最大长度 `255` 字符 |
| `track_type` | string | 否 | 轨迹类型，使用 `/track/types` 返回的英文 `type`：`hiking` / `running` / `climbing` / `riding` / `driving`；服务端兼容历史中文名输入，但入库会归一为英文 code |
| `source_tag` | string | 否 | 轨迹来源/运营标签；允许值为空字符串或 `manual_seed`（人工录入冷启动轨迹），未传为空字符串 |
| `coordinate_system` | string | 否 | 坐标系，例如 `WGS84` / `GCJ02` / `BD09` |
| `start_time` | string | 否 | 开始时间，RFC3339/ISO8601 格式 |
| `end_time` | string | 否 | 结束时间，RFC3339/ISO8601 格式，必须 `>= start_time` |
| `distance` | number | 否 | 距离（米），必须 `>= 0` |
| `duration` | int | 否 | 时长（秒），必须 `>= 0` |
| `calories_burned` | number | 否 | 热量消耗（千卡），必须 `>= 0` |
| `elevation_gain` | int | 否 | 累计爬升（米），必须 `>= 0` |
| `raw_track_url` | string | 否 | 原始轨迹文件 OSS 地址（建议传 OSS HTTP URL，可带签名参数；轨迹地图索引支持 JSON/GeoJSON/KML/KMZ） |
| `track_screenshot_url` | string | 否 | 轨迹截图 OSS 地址（建议传 OSS HTTP URL，可带签名参数） |
| `track_no_map_bg_screenshot_url` | string | 否 | 无地图背景的轨迹路线截图 OSS 地址（建议传 OSS HTTP URL，可带签名参数） |
| `is_running` | bool | 否 | 是否进行中，默认 `true`；当为 `true` 或未传时，当前用户不能已处于 active 同行中 |
| `avg_speed_kmh` | number | 否 | 平均速度（km/h），必须 `>= 0` |

### 响应

**状态码：** `200 OK`

返回创建后的 `Track` 对象，使用统一响应格式 `StandardResponse`：

```json
{
  "code": 0,
  "data": {
    "id": "No.1713520800123456789",
    "user_id": 1001,
    "session_id": "sess_xxx",
    "city_code": "330100",
    "locate_addr": "杭州市西湖区",
    "track_type": "running",
    "coordinate_system": "GCJ02",
    "title": "傍晚夜跑",
    "start_time": "2026-04-20T12:00:00Z",
    "end_time": "2026-04-20T12:00:00Z",
    "distance": 0,
    "duration": 0,
    "avg_speed_kmh": 0,
    "calories_burned": 0,
    "elevation_gain": 0,
    "raw_track_url": "/api/v1/static/raw_tracks/No.1713520800123456789.dat",
    "track_screenshot_url": "/api/v1/static/screenshots/No.1713520800123456789.jpg",
    "track_no_map_bg_screenshot_url": "/api/v1/static/screenshots/No.1713520800123456789_no_map_bg.jpg",
    "is_running": false,
    "status": 1,
    "created_at": "2026-04-20T12:00:00Z",
    "updated_at": "2026-04-20T12:00:00Z",
    "earned_rewards": [
      {
        "code": "first_track",
        "type": "badge",
        "category": "新手",
        "name": "第一条轨迹",
        "description": "完成首条有效轨迹",
        "rarity": "common",
        "icon_url": "",
        "target_value": 1,
        "earned": true,
        "earned_at": "2026-04-20T12:00:00Z",
        "current_value": 1,
        "progress": 1
      }
    ]
  }
}
```

补充说明：

- 客户端连接 EMQX 后，应订阅 `topics.control_subscribe`；
- 服务端会在该 topic 发布：
  - `member_left`
  - `member_kicked`
  - `session_ended`
- 具体消息格式见 `track_companion.md` 的 control topic 说明。

说明：`raw_track_url` / `track_screenshot_url` / `track_no_map_bg_screenshot_url` 在请求时是 OSS 地址，但响应会被服务端替换为可直接从业务服务器下载的本地链接（路径在 `/api/v1/static/...` 下，需要登录态）。

成就奖励：创建已完成轨迹（`is_running=false`）后，服务端会立即幂等结算成就；如果本次结算产生新的奖励，会直接在响应 `data.earned_rewards` 中返回，客户端可用于轨迹完成页即时弹窗。同一用户已获得过的奖励不会重复返回。

约束：同一用户同一时间只能处于一种进行中状态。创建普通进行中轨迹（`is_running=true` 或未传）时，若用户已经加入 active 同行，会返回 `400`；创建已完成轨迹（`is_running=false`）不受该限制，可用于同行结束后上传个人轨迹并携带 `session_id`。

### 错误响应

- `400 Bad Request`
  - `you already joined an active companion session: {title}`
  - `end_time must be >= start_time`
  - `session_id is required` / `session_id is too long`
  - `distance must be >= 0` / `elevation_gain must be >= 0` / `avg_speed_kmh must be >= 0`

### 示例（curl）

```bash
curl -X POST "http://<host>:<port>/api/v1/track/create" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -H "X-User-ID: 1001" \
  -d '{
    "title": "傍晚夜跑",
    "session_id": "sess_xxx",
    "city_code": "330100",
    "locate_addr": "杭州市西湖区",
    "track_type": "running",
    "start_time": "2026-04-20T12:00:00Z",
    "end_time": "2026-04-20T12:30:00Z",
    "distance": 1200.5,
    "duration": 1800,
    "calories_burned": 96.5,
    "elevation_gain": 80,
    "raw_track_url": "https://<bucket>.oss-<region>.aliyuncs.com/prod/track/.../xxx.dat?<签名参数>",
    "track_screenshot_url": "https://<bucket>.oss-<region>.aliyuncs.com/prod/track/.../xxx.jpg?<签名参数>",
    "track_no_map_bg_screenshot_url": "https://<bucket>.oss-<region>.aliyuncs.com/prod/track/.../xxx_no_map_bg.jpg?<签名参数>",
    "is_running": false,
    "avg_speed_kmh": 12.3
  }'
```

---

## 2. 推荐轨迹列表

获取推荐轨迹列表。

**需要认证**

### 说明

- 该接口 **不会返回** `is_running=true` 的轨迹（进行中的轨迹会被过滤）。
- 返回的是 `TrackSummary` 列表（轻量字段），而不是完整的 `Track`。
- 返回结果中 `collected` 表示当前鉴权用户是否已收藏该轨迹。
- 返回结果中的 `collect_count` 表示该轨迹被收藏的总数。
- 返回结果中的 `navigate_count` 表示该轨迹被其他用户用于导航的次数。
- 返回结果中的 `nickname` / `user_avatar_url` 为轨迹所属用户的昵称/头像 URI。
- 返回结果中的 `city_code` / `city_name` 为轨迹所属城市 Code 及其对应的城市名称。
- 返回结果中的 `locate_addr` 为轨迹的具体位置信息。
- 返回结果中的 `track_type` 为英文轨迹类型 code，例如 `hiking` / `running` / `climbing` / `riding` / `driving`。
- 返回结果中的 `start_time` 为运动开始时间。
- 返回结果中的 `end_time` 为运动结束时间。
- 返回结果中的 `avg_speed_kmh` 为平均速度（km/h）。
- 返回结果中的 `calories_burned` 为热量消耗（千卡）。
- 返回结果中的 `raw_track_url` / `track_screenshot_url` / `track_no_map_bg_screenshot_url` 为服务端本地可下载链接（不是 OSS 地址）。
- 接口已支持基于 `cursor` 的瀑布流分页，排序规则为 `start_time DESC, id DESC`。
- 首次请求不传 `cursor`；继续翻页时透传上一次返回的 `next_cursor`。
- `limit` 为可选参数，默认 `20`，最大 `50`；超出最大值时服务端会自动截断到 `50`。
- 响应中的 `has_more` 表示是否还有下一页；仅当 `has_more=true` 时才会返回 `next_cursor`。

### 请求

```
GET /api/v1/track/recommend/list?limit=20&cursor=<next_cursor>
Authorization: Bearer <token>
```

**Query 参数：**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `limit` | int | 否 | 每页返回条数，默认 `20`，最大 `50`。 |
| `cursor` | string | 否 | 分页游标。首屏不传，翻页时透传上一次响应里的 `next_cursor`。 |

### 响应

**状态码：** `200 OK`

返回 `StandardResponse`，`data` 为分页对象：

```json
{
  "code": 0,
  "data": {
    "items": [
      {
        "id": "trk1",
        "user_id": 1001,
        "session_id": "sess_xxx",
        "city_code": "330100",
        "locate_addr": "杭州市西湖区",
        "track_type": "hiking",
        "start_time": "2026-04-20T12:00:00Z",
        "end_time": "2026-04-20T12:10:00Z",
        "city_name": "杭州市",
        "nickname": "Alice",
        "user_avatar_url": "https://example.com/avatar.png",
        "title": "西湖徒步",
        "distance": 1200.5,
        "duration": 360,
        "avg_speed_kmh": 12.3,
        "calories_burned": 96.5,
        "elevation_gain": 80,
        "raw_track_url": "/api/v1/static/raw_tracks/trk1.dat",
        "track_screenshot_url": "/api/v1/static/screenshots/trk1.jpg",
        "track_no_map_bg_screenshot_url": "/api/v1/static/screenshots/trk1_no_map_bg.jpg",
        "collected": true,
        "collect_count": 12,
        "navigate_count": 3
      }
    ],
    "next_cursor": "eyJzdGFydF90aW1lIjoiMjAyNi0wNC0yMFQxMjowMDowMFoiLCJpZCI6InRyazEifQ",
    "has_more": true
  }
}
```

**字段说明：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `data.items` | `TrackSummary[]` | 当前页轨迹列表。 |
| `data.next_cursor` | string | 下一页游标；当 `has_more=false` 时为空或不返回。 |
| `data.has_more` | bool | 是否还有下一页数据。 |

### 示例（curl）

```bash
curl -X GET "http://<host>:<port>/api/v1/track/recommend/list?limit=20" \
  -H "Authorization: Bearer <token>" \
  -H "X-User-ID: 1001"
```

翻下一页：

```bash
curl -X GET "http://<host>:<port>/api/v1/track/recommend/list?limit=20&cursor=<next_cursor>" \
  -H "Authorization: Bearer <token>" \
  -H "X-User-ID: 1001"
```

---

## 3. 轨迹详情

获取指定轨迹的详情信息。

**需要认证**

### 请求

```
GET /api/v1/track/:track_id/detail
Authorization: Bearer <token>
```

**Path 参数：**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `track_id` | string | 是 | 轨迹 ID |

### 响应

**状态码：** `200 OK`

返回 `StandardResponse`，`data` 为 `Track`：

```json
{
  "code": 0,
  "data": {
    "id": "trk-detail",
    "user_id": 1001,
    "session_id": "sess_xxx",
    "track_type": "hiking",
    "coordinate_system": "WGS84",
    "title": "详情轨迹",
    "start_time": "2026-04-20T12:00:00Z",
    "end_time": "2026-04-20T12:10:00Z",
    "distance": 1200.5,
    "duration": 600,
    "avg_speed_kmh": 7.2,
    "elevation_gain": 80,
    "raw_track_url": "/api/v1/static/raw_tracks/trk-detail.dat",
    "track_screenshot_url": "/api/v1/static/screenshots/trk-detail.jpg",
    "is_running": false,
    "status": 1,
    "created_at": "2026-04-20T12:00:00Z",
    "updated_at": "2026-04-20T12:10:00Z"
  }
}
```

### 示例（curl）

```bash
curl -X GET "http://<host>:<port>/api/v1/track/trk-detail/detail" \
  -H "Authorization: Bearer <token>" \
  -H "X-User-ID: 1001"
```

---

## 7. 轨迹搜索列表

按关键字搜索轨迹列表。

**需要认证**

### 说明

- 返回的是 `TrackSummary` 分页结果（轻量字段），而不是完整的 `Track`。
- 返回结果中 `collected` 表示当前鉴权用户是否已收藏该轨迹。
- 返回结果中的 `collect_count` 表示该轨迹被收藏的总数。
- 返回结果中的 `navigate_count` 表示该轨迹被其他用户用于导航的次数。
- 返回结果中的 `nickname` / `user_avatar_url` 为轨迹所属用户的昵称/头像 URI。
- 返回结果中的 `city_code` / `city_name` 为轨迹所属城市 Code 及其对应的城市名称。
- 返回结果中的 `locate_addr` 为轨迹的具体位置信息。
- 返回结果中的 `track_type` 为英文轨迹类型 code，例如 `hiking` / `running` / `climbing` / `riding` / `driving`。
- 返回结果中的 `start_time` 为运动开始时间。
- 返回结果中的 `end_time` 为运动结束时间。
- 返回结果中的 `avg_speed_kmh` 为平均速度（km/h）。
- 返回结果中的 `calories_burned` 为热量消耗（千卡）。
- 返回结果中的 `raw_track_url` / `track_screenshot_url` / `track_no_map_bg_screenshot_url` 为服务端本地可下载链接（不是 OSS 地址）。
- 接口已支持基于 `cursor` 的瀑布流分页，排序规则为 `start_time DESC, id DESC`。
- 首次请求不传 `cursor`；继续翻页时透传上一次返回的 `next_cursor`。
- `limit` 为可选参数，默认 `20`，最大 `50`；超出最大值时服务端会自动截断到 `50`。
- 响应中的 `has_more` 表示是否还有下一页；仅当 `has_more=true` 时才会返回 `next_cursor`。
- 响应中的 `total_count` 表示“我的轨迹”总数（按本接口口径：排除删除与进行中，包含 `正常/私密`）。

### 请求

```
GET /api/v1/track/search/list?keyword=:keyword&limit=20&cursor=<next_cursor>
Authorization: Bearer <token>
```

**Query 参数：**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `keyword` | string | 否 | 搜索关键字；为空时返回最近轨迹列表。 |
| `limit` | int | 否 | 每页返回条数，默认 `20`，最大 `50`。 |
| `cursor` | string | 否 | 分页游标。首屏不传，翻页时透传上一次响应里的 `next_cursor`。 |

### 响应

**状态码：** `200 OK`

返回 `StandardResponse`，`data` 为分页对象：

```json
{
  "code": 0,
  "data": {
    "items": [
      {
        "id": "trk1",
        "user_id": 1001,
        "session_id": "sess_xxx",
        "city_code": "330100",
        "locate_addr": "杭州市西湖区",
        "track_type": "hiking",
        "start_time": "2026-04-20T12:00:00Z",
        "end_time": "2026-04-20T12:10:00Z",
        "city_name": "杭州市",
        "nickname": "Alice",
        "user_avatar_url": "https://example.com/avatar.png",
        "title": "西湖徒步",
        "distance": 1200.5,
        "duration": 360,
        "avg_speed_kmh": 12.3,
        "calories_burned": 96.5,
        "elevation_gain": 80,
        "raw_track_url": "/api/v1/static/raw_tracks/trk1.dat",
        "track_screenshot_url": "/api/v1/static/screenshots/trk1.jpg",
        "track_no_map_bg_screenshot_url": "/api/v1/static/screenshots/trk1_no_map_bg.jpg",
        "collected": true,
        "collect_count": 12,
        "navigate_count": 3
      }
    ],
    "next_cursor": "eyJzdGFydF90aW1lIjoiMjAyNi0wNC0yMFQxMjowMDowMFoiLCJpZCI6InRyazEifQ",
    "has_more": true
  }
}
```

**字段说明：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `data.items` | `TrackSummary[]` | 当前页轨迹列表。 |
| `data.next_cursor` | string | 下一页游标；当 `has_more=false` 时为空或不返回。 |
| `data.has_more` | bool | 是否还有下一页数据。 |

### 示例（curl）

```bash
curl -X GET "http://<host>:<port>/api/v1/track/search/list?keyword=西湖&limit=20" \
  -H "Authorization: Bearer <token>" \
  -H "X-User-ID: 1001"
```

翻下一页：

```bash
curl -X GET "http://<host>:<port>/api/v1/track/search/list?keyword=西湖&limit=20&cursor=<next_cursor>" \
  -H "Authorization: Bearer <token>" \
  -H "X-User-ID: 1001"
```

---

## 9. 我的轨迹列表

获取当前登录用户自己发布的轨迹列表。

**需要认证**

### 说明

- 仅返回当前鉴权用户自己的轨迹，不支持查看其他用户的数据。
- 返回的是 `MyTrackSummary` 分页列表，而不是完整的 `Track`。
- 仅返回已结束的轨迹；进行中的轨迹（`is_running=true`）不会出现在结果中。
- 返回轨迹状态为 `正常` 和 `私密` 的记录，不返回已删除记录。
- 接口已支持基于 `cursor` 的瀑布流分页，排序规则为 `start_time DESC, id DESC`。
- 首次请求不传 `cursor`；继续翻页时透传上一次返回的 `next_cursor`。
- 返回结果**不包含** `nickname`、`user_avatar_url`、`collected` 字段。
- 返回结果**包含** `collect_count`、`navigate_count` 统计字段，便于客户端直接展示。
- 返回结果中的 `locate_addr` 为轨迹的具体位置信息。
- 返回结果中的 `avg_speed_kmh` 为平均速度（km/h）。
- `raw_track_url` / `track_screenshot_url` 为服务端本地可下载链接（不是 OSS 地址）。
- 若轨迹记录的 `raw_track_url` 为空，则该轨迹不会出现在本接口返回结果中。
- `limit` 为可选参数，默认 `20`，最大 `50`；超出最大值时服务端会自动截断到 `50`。
- 响应中的 `has_more` 表示是否还有下一页；仅当 `has_more=true` 时才会返回 `next_cursor`。
- 响应中的 `total_count` 表示“我的轨迹”总数（按本接口口径：排除删除与进行中，包含 `正常/私密`，并且仅统计 `raw_track_url` 非空的轨迹）。

### 请求

```
GET /api/v1/track/my/list?limit=20&cursor=<next_cursor>
Authorization: Bearer <token>
```

#### Headers

| Header | 必填 | 说明 |
|--------|------|------|
| `Authorization` | 是 | `Bearer <token>`（从 Token 中解析当前用户身份） |

#### Query 参数

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `limit` | int | 否 | 每页返回条数，默认 `20`，最大 `50`。 |
| `cursor` | string | 否 | 分页游标。首屏不传，翻页时透传上一次响应里的 `next_cursor`。 |

#### Body

无

### 响应

**状态码：** `200 OK`

返回 `StandardResponse`，`data` 为分页对象：

```json
{
  "code": 0,
  "data": {
    "items": [
      {
        "id": "trk1",
        "user_id": 1001,
        "session_id": "sess_xxx",
        "city_code": "330100",
        "locate_addr": "杭州市西湖区",
        "track_type": "hiking",
        "start_time": "2026-04-20T12:00:00Z",
        "end_time": "2026-04-20T12:10:00Z",
        "city_name": "杭州市",
        "title": "西湖徒步",
        "distance": 1200.5,
        "duration": 360,
        "avg_speed_kmh": 12.3,
        "calories_burned": 96.5,
        "elevation_gain": 80,
        "collect_count": 12,
        "navigate_count": 3,
        "track_screenshot_url": "/api/v1/static/screenshots/trk1.jpg",
        "raw_track_url": "/api/v1/static/raw_tracks/trk1.dat"
      }
    ],
    "total_count": 2,
    "next_cursor": "eyJzdGFydF90aW1lIjoiMjAyNi0wNC0yMFQxMjowMDowMFoiLCJpZCI6InRyazEifQ",
    "has_more": true
  }
}
```

**字段说明：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `data.items` | `MyTrackSummary[]` | 当前页我的轨迹列表。 |
| `data.total_count` | int64 | 我的轨迹总数（按本接口口径）。 |
| `data.next_cursor` | string | 下一页游标；当 `has_more=false` 时为空或不返回。 |
| `data.has_more` | bool | 是否还有下一页数据。 |
| `data.items[].id` | string | 轨迹 ID。 |
| `data.items[].user_id` | int64 | 当前轨迹所属用户 ID。 |
| `data.items[].session_id` | string | 关联的同行会话 ID；非同行轨迹为空字符串。 |
| `data.items[].city_code` | string | 城市 Code。 |
| `data.items[].locate_addr` | string | 轨迹的具体位置信息。 |
| `data.items[].track_type` | string | 英文轨迹类型 code，例如 `hiking` / `running` / `climbing` / `riding` / `driving`。 |
| `data.items[].start_time` | string | 运动开始时间。 |
| `data.items[].end_time` | string | 运动结束时间。 |
| `data.items[].city_name` | string | 城市名称，由 `city_code` 映射得到。 |
| `data.items[].title` | string | 轨迹标题。 |
| `data.items[].distance` | number | 距离，单位米。 |
| `data.items[].duration` | int | 时长，单位秒。 |
| `data.items[].avg_speed_kmh` | number | 平均速度（km/h）。 |
| `data.items[].calories_burned` | number | 热量消耗（千卡）。 |
| `data.items[].elevation_gain` | int | 累计爬升，单位米。 |
| `data.items[].collect_count` | int64 | 该轨迹被收藏的总数。 |
| `data.items[].navigate_count` | int64 | 该轨迹被其他用户用于导航的次数。 |
| `data.items[].track_screenshot_url` | string | 服务端本地缓存的轨迹截图可下载 URL。 |
| `data.items[].raw_track_url` | string | 服务端本地缓存的原始轨迹文件可下载 URL。 |

### 错误响应

- `400 Bad Request`
  - `limit` 不是正整数（返回 `{"error":"invalid limit"}`）
  - `cursor` 非法或已损坏（返回 `{"error":"invalid cursor"}`）
- `401 Unauthorized`
  - 缺少/无效/过期的 Token
- `500 Internal Server Error`
  - 服务端查询我的轨迹列表失败

错误响应格式示例：

```json
{
  "error": "..."
}
```

### 示例（curl）

```bash
curl -X GET "http://<host>:<port>/api/v1/track/my/list?limit=20" \
  -H "Authorization: Bearer <token>"
```

翻下一页：

```bash
curl -X GET "http://<host>:<port>/api/v1/track/my/list?limit=20&cursor=<next_cursor>" \
  -H "Authorization: Bearer <token>"
```

---

## 11. 更新轨迹信息

更新指定轨迹的摘要信息（`UpdateTrackInfo`）。

### 说明

- 该接口用于“补全”轨迹记录：**只有当数据库中对应字段为空（字符串为空串或数值为 0）时才会更新**；否则该字段会被忽略并保持原值不变。
- 仅允许更新属于当前鉴权用户（Authorization Token 解析出的 `user_id`）的轨迹；若 `track_id` 不属于该用户，返回 `403 Forbidden`。
- `is_running` 不支持更新：请求体中传入会被服务端**静默忽略**，并保持数据库原值不变。
- 资源字段（`raw_track_url` / `track_screenshot_url` / `track_no_map_bg_screenshot_url`）在请求时通常是 OSS 地址，但响应会被服务端替换为可直接从业务服务器下载的本地链接（路径在 `/api/v1/static/...` 下，需要登录态）。
- 兼容旧字段：`screenshot_url` 会被当作 `track_screenshot_url` 处理。

### 请求

```
PUT /api/v1/track/:track_id/update
Content-Type: application/json
Authorization: Bearer <token>
```

**Path 参数：**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `track_id` | string | 是 | 轨迹 ID |

**请求体：**（至少传 1 个字段）

```json
{
  "session_id": "sess_xxx",
  "city_code": "330100",
  "locate_addr": "杭州市西湖区",
  "track_type": "running",
  "source_tag": "manual_seed",
  "coordinate_system": "GCJ02",
  "raw_track_url": "https://<bucket>.oss-<region>.aliyuncs.com/prod/track/.../xxx.dat",
  "track_screenshot_url": "https://<bucket>.oss-<region>.aliyuncs.com/prod/track/.../xxx.jpg",
  "track_no_map_bg_screenshot_url": "https://<bucket>.oss-<region>.aliyuncs.com/prod/track/.../xxx_no_map_bg.jpg",
  "distance": 1200.5,
  "duration": 1800,
  "elevation_gain": 80,
  "avg_speed_kmh": 12.3
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `session_id` | string | 否 | 关联的同行会话 ID，仅当原值为空时才会写入；用于把同一次同行结束后各成员上传的个人轨迹串联起来 |
| `city_code` | string | 否 | 城市 Code（仅当原值为空时才会写入） |
| `locate_addr` | string | 否 | 轨迹的具体位置信息，最大长度 `255` 字符（仅当原值为空时才会写入） |
| `track_type` | string | 否 | 轨迹类型，仅当原值为空时才会写入；使用 `/track/types` 返回的英文 `type`，服务端兼容历史中文名输入但会归一为英文 code |
| `source_tag` | string | 否 | 轨迹来源/运营标签；允许值为空字符串或 `manual_seed`（人工录入冷启动轨迹），仅当原值为空时才会写入；普通列表摘要不返回该字段 |
| `coordinate_system` | string | 否 | 坐标系，例如 `WGS84` / `GCJ02` / `BD09`（仅当原值为空时才会写入） |
| `raw_track_url` | string | 否 | 原始轨迹文件 OSS 地址（仅当原值为空时才会写入） |
| `track_screenshot_url` | string | 否 | 轨迹截图 OSS 地址（仅当原值为空时才会写入） |
| `track_no_map_bg_screenshot_url` | string | 否 | 无地图背景的轨迹路线截图 OSS 地址（仅当原值为空时才会写入） |
| `distance` | number | 否 | 距离（米），必须 `>= 0`（仅当原值为 `0` 时才会写入） |
| `duration` | int | 否 | 时长（秒），必须 `>= 0`（仅当原值为 `0` 时才会写入） |
| `elevation_gain` | int | 否 | 累计爬升（米），必须 `>= 0`（仅当原值为 `0` 时才会写入） |
| `avg_speed_kmh` | number | 否 | 平均速度（km/h），必须 `>= 0`（仅当原值为 `0` 时才会写入） |

### 响应

**状态码：** `200 OK`

返回更新后的 `Track` 对象，使用统一响应格式 `StandardResponse`。

### 示例（curl）

```bash
curl -X PUT "http://<host>:<port>/api/v1/track/trk1/update" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "sess_xxx",
    "city_code": "330100",
    "locate_addr": "杭州市西湖区",
    "track_no_map_bg_screenshot_url": "https://<bucket>.oss-<region>.aliyuncs.com/prod/track/.../xxx_no_map_bg.jpg"
  }'
```

### 错误响应

- `400 Bad Request`
  - 请求体不是合法 JSON（返回 `{"error":"invalid payload"}`）
  - 请求体未包含任何字段（返回 `{"error":"no fields to update"}`）
  - `session_id` 为空或超过最大长度（返回 `{"error":"session_id is required"}` / `{"error":"session_id is too long"}`）
  - `locate_addr` 超过最大长度（返回 `{"error":"locate_addr is too long"}`）
  - `distance` / `elevation_gain` / `avg_speed_kmh` 为负数
- `401 Unauthorized`
  - 缺少/无效/过期的 Token
- `403 Forbidden`
  - 更新他人的轨迹
- `404 Not Found`
  - 轨迹不存在
- `500 Internal Server Error`
  - 服务端更新轨迹失败


---

## 12. 删除轨迹

删除指定轨迹（软删除）：仅标记 `status=0` 并记录 `deleted_at`。

### 说明

- 仅允许删除属于当前鉴权用户（Authorization Token 解析出的 `user_id`）的轨迹；若 `track_id` 不属于该用户，返回 `403 Forbidden`。
- 软删除后：轨迹会从“推荐列表/搜索列表/我的轨迹列表”等列表接口中被过滤（列表均不返回 `status=0` 的轨迹）。

### 请求

```
DELETE /api/v1/track/:track_id
Authorization: Bearer <token>
```

**Path 参数：**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `track_id` | string | 是 | 轨迹 ID |

### 响应

**状态码：** `200 OK`

返回统一响应格式 `StandardResponse`：

```json
{
  "code": 0,
  "data": {
    "status": "ok"
  }
}
```

### 错误响应

- `401 Unauthorized`
  - 缺少/无效/过期的 Token
- `403 Forbidden`
  - 删除他人的轨迹
- `404 Not Found`
  - 轨迹不存在
- `500 Internal Server Error`
  - 服务端删除轨迹失败


---

## 16. 获取运动类型

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

### 请求

```
GET /api/v1/track/types
Authorization: Bearer <token>
```

### 响应

**状态码：** `200 OK`

返回统一响应格式 `StandardResponse`：

```json
{
  "code": 0,
  "data": [
    {
      "type": "hiking",
      "name": "徒步",
      "theme_color": "#345631",
      "icon_url": "/api/v1/static/track_type_icon/hiking.svg",
      "icon_anim_url": "",
      "milestone": {
        "month": {
          "distance": 120.5,
          "track_count": 1,
          "duration": 600,
          "calories": 80.5
        },
        "year": {
          "distance": 200.5,
          "track_count": 2,
          "duration": 1000,
          "calories": 140.5
        }
      }
    },
    {
      "type": "running",
      "name": "跑步",
      "theme_color": "#F26A4B",
      "icon_url": "/api/v1/static/track_type_icon/running.svg",
      "icon_anim_url": "",
      "milestone": {
        "month": {
          "distance": 300,
          "track_count": 1,
          "duration": 700,
          "calories": 90
        },
        "year": {
          "distance": 800,
          "track_count": 2,
          "duration": 1600,
          "calories": 190
        }
      }
    }
  ]
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `data` | `TrackTypeOption[]` | 可选运动类型列表。 |
| `data[].type` | string | 运动类型英文标识，例如 `hiking` / `running`。 |
| `data[].name` | string | 运动类型名称。 |
| `data[].theme_color` | string | 运动类型主题色。 |
| `data[].icon_url` | string | 运动类型图标静态资源链接，路径位于 `/api/v1/static/track_type_icon/` 下。 |
| `data[].icon_anim_url` | string | 运动类型 Lottie 动画文件链接；当前默认空字符串。 |
| `data[].milestone.month.distance` | number | 当前用户该运动类型最近一个月总里程，单位米。 |
| `data[].milestone.month.track_count` | int64 | 当前用户该运动类型最近一个月轨迹次数。 |
| `data[].milestone.month.duration` | int64 | 当前用户该运动类型最近一个月总耗时，单位秒。 |
| `data[].milestone.month.calories` | number | 当前用户该运动类型最近一个月总热量，单位千卡。 |
| `data[].milestone.year.distance` | number | 当前用户该运动类型最近一年总里程，单位米。 |
| `data[].milestone.year.track_count` | int64 | 当前用户该运动类型最近一年轨迹次数。 |
| `data[].milestone.year.duration` | int64 | 当前用户该运动类型最近一年总耗时，单位秒。 |
| `data[].milestone.year.calories` | number | 当前用户该运动类型最近一年总热量，单位千卡。 |

### 错误响应

- `401 Unauthorized`
  - 缺少/无效/过期的 Token
---
