# 轨迹接口文档（创建/更新）

> Base URL: `http://<host>:<port>/api/v1`
>
> 所有请求和响应均使用 **JSON** 格式，`Content-Type` 统一为 `application/json`。

---

## 目录

| 序号 | 接口 | 方法 | 路径 | 需要认证 |
|------|------|------|------|---------|
| 1 | [创建轨迹](#1-创建轨迹) | POST | `/track/create` | ✅ |
| 2 | [推荐轨迹列表](#2-推荐轨迹列表) | GET | `/track/recommend/list` | ✅ |
| 3 | [轨迹详情](#3-轨迹详情) | GET | `/track/:track_id/detail` | ✅ |
| 4 | [获取 OSS STS 临时凭证（直传上传）](#4-获取-oss-sts-临时凭证直传上传) | GET | `/oss/sts-token` | ✅ |
| 5 | [收藏轨迹](#5-收藏轨迹) | POST | `/track_collect` | ✅ |
| 6 | [取消收藏轨迹](#6-取消收藏轨迹) | DELETE | `/track_collect` | ✅ |
| 7 | [轨迹搜索列表](#7-轨迹搜索列表) | GET | `/track/search/list` | ✅ |
| 8 | [导航使用上报](#8-导航使用上报) | POST | `/track/:track_id/navigation/report` | ✅ |

---

## 认证机制（JWT Token）

轨迹接口为 **认证接口**，需要在请求头携带：

```
Authorization: Bearer <token>
```

Token 的获取与说明参考 `login.md`。

---

## 公共请求头

| Header 名称 | 类型 | 必填 | 说明 |
|-------------|------|------|------|
| `Authorization` | string | 是 | `Bearer <token>` |
| `Content-Type` | string | 是 | 固定 `application/json` |
| `X-City-Code` | string | 否 | 城市 Code |
| `X-User-ID` | string | 否（建议携带） | 用户 ID（部分接口历史上依赖该 header，建议统一携带以兼容客户端实现） |
| `X-Device-ID` | string | 否 | 设备唯一标识 |
| `X-Platform` | string | 否 | 客户端平台：`ios` / `android` |
| `X-Client-Version` | string | 否 | 客户端版本号，如 `1.0.0` |
| `X-Client-Language` | string | 否 | 客户端语言，如 `zh-CN` |

---

## 公共错误响应

```json
{
  "error": "错误描述信息"
}
```

| HTTP 状态码 | 含义 |
|------------|------|
| 400 | 请求参数缺失或格式错误 |
| 401 | 认证失败（Token 无效/过期） |
| 403 | 无权限（操作他人的轨迹） |
| 404 | 资源不存在（轨迹不存在） |
| 500 | 服务端内部错误 |

---

## 轨迹对象（Track）字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | 轨迹 ID |
| `user_id` | int64 | 用户 ID |
| `city_code` | string | 城市 Code（用于标识轨迹所属城市；城市/省份映射关系维护在配置文件中） |
| `track_type` | string | 轨迹类型，例如 `徒步` / `跑步` / `骑车` / `自驾` |
| `title` | string | 轨迹标题 |
| `start_time` | string | 开始时间（RFC3339/ISO8601，服务端序列化时间格式） |
| `end_time` | string | 结束时间（RFC3339/ISO8601，服务端序列化时间格式） |
| `distance` | number | 距离（米） |
| `duration` | int | 时长（秒） |
| `avg_speed_kmh` | number | 平均速度（km/h） |
| `elevation_gain` | int | 累计爬升（米） |
| `raw_track_url` | string | 原始轨迹文件可下载链接（服务端本地缓存 URL，例如 `/api/v1/static/raw_tracks/<track_id>.dat`） |
| `track_screenshot_url` | string | 轨迹截图可下载链接（服务端本地缓存 URL，例如 `/api/v1/static/screenshots/<track_id>.jpg`） |
| `is_running` | bool | 是否进行中 |
| `status` | int | 轨迹状态：`0` 删除，`1` 正常，`2` 私密 |
| `created_at` | string | 创建时间 |
| `updated_at` | string | 更新时间 |

---

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
  "city_code": "330100",
  "track_type": "跑步",
  "start_time": "2026-04-20T12:00:00Z",
  "end_time": "2026-04-20T12:30:00Z",
  "distance": 1200.5,
  "duration": 1800,
  "elevation_gain": 80,
  "raw_track_url": "https://<bucket>.oss-<region>.aliyuncs.com/prod/track/.../xxx.dat",
  "track_screenshot_url": "https://<bucket>.oss-<region>.aliyuncs.com/prod/track/.../xxx.jpg",
  "is_running": false,
  "avg_speed_kmh": 12.3
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `title` | string | 否 | 轨迹标题，默认 `新的轨迹` |
| `city_code` | string | 否 | 城市 Code（标识轨迹所属城市） |
| `track_type` | string | 否 | 轨迹类型，例如 `徒步` / `跑步` / `骑车` / `自驾` |
| `start_time` | string | 否 | 开始时间，RFC3339/ISO8601 格式 |
| `end_time` | string | 否 | 结束时间，RFC3339/ISO8601 格式，必须 `>= start_time` |
| `distance` | number | 否 | 距离（米），必须 `>= 0` |
| `duration` | int | 否 | 时长（秒），必须 `>= 0` |
| `elevation_gain` | int | 否 | 累计爬升（米），必须 `>= 0` |
| `raw_track_url` | string | 否 | 原始轨迹文件 OSS 地址（建议传 OSS HTTP URL，可带签名参数） |
| `track_screenshot_url` | string | 否 | 轨迹截图 OSS 地址（建议传 OSS HTTP URL，可带签名参数） |
| `is_running` | bool | 否 | 是否进行中，默认 `true` |
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
    "city_code": "330100",
    "track_type": "跑步",
    "title": "傍晚夜跑",
    "start_time": "2026-04-20T12:00:00Z",
    "end_time": "2026-04-20T12:00:00Z",
    "distance": 0,
    "duration": 0,
    "avg_speed_kmh": 0,
    "elevation_gain": 0,
    "raw_track_url": "/api/v1/static/raw_tracks/No.1713520800123456789.dat",
    "track_screenshot_url": "/api/v1/static/screenshots/No.1713520800123456789.jpg",
    "is_running": true,
    "status": 1,
    "created_at": "2026-04-20T12:00:00Z",
    "updated_at": "2026-04-20T12:00:00Z"
  }
}
```

说明：`raw_track_url` / `track_screenshot_url` 在请求时是 OSS 地址，但响应会被服务端替换为可直接从业务服务器下载的本地链接（路径在 `/api/v1/static/...` 下，需要登录态）。

### 示例（curl）

```bash
curl -X POST "http://<host>:<port>/api/v1/track/create" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -H "X-User-ID: 1001" \
  -d '{
    "title": "傍晚夜跑",
    "city_code": "330100",
    "track_type": "跑步",
    "start_time": "2026-04-20T12:00:00Z",
    "end_time": "2026-04-20T12:30:00Z",
    "distance": 1200.5,
    "duration": 1800,
    "elevation_gain": 80,
    "raw_track_url": "https://<bucket>.oss-<region>.aliyuncs.com/prod/track/.../xxx.dat?<签名参数>",
    "track_screenshot_url": "https://<bucket>.oss-<region>.aliyuncs.com/prod/track/.../xxx.jpg?<签名参数>",
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
- 返回结果中的 `track_type` 为轨迹类型，例如 `徒步` / `跑步` / `骑车` / `自驾`。
- 返回结果中的 `start_time` 为运动开始时间。
- 返回结果中的 `raw_track_url` / `track_screenshot_url` 为服务端本地可下载链接（不是 OSS 地址）。

### 请求

```
GET /api/v1/track/recommend/list
Authorization: Bearer <token>
```

**请求参数：** 无

### 响应

**状态码：** `200 OK`

返回 `StandardResponse`，`data` 为 `TrackSummary[]`：

```json
{
  "code": 0,
  "data": [
    {
      "id": "trk1",
      "user_id": 1001,
      "city_code": "330100",
      "track_type": "徒步",
      "start_time": "2026-04-20T12:00:00Z",
      "city_name": "杭州市",
      "nickname": "Alice",
      "user_avatar_url": "https://example.com/avatar.png",
      "title": "西湖徒步",
      "distance": 1200.5,
      "duration": 360,
      "elevation_gain": 80,
      "raw_track_url": "/api/v1/static/raw_tracks/trk1.dat",
      "track_screenshot_url": "/api/v1/static/screenshots/trk1.jpg",
      "collected": true,
      "collect_count": 12,
      "navigate_count": 3
    }
  ]
}
```

### 示例（curl）

```bash
curl -X GET "http://<host>:<port>/api/v1/track/recommend/list" \
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
    "track_type": "徒步",
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

## 4. 获取 OSS STS 临时凭证（直传上传）

用于客户端“直传 OSS”场景：客户端向服务端申请短时有效的临时凭证（STS），然后使用该凭证直接上传到 OSS。

服务端会将临时凭证的权限限制为：**仅允许上传到当前用户的专属目录前缀**（例如 `prod/track/user/1001/`）。

**需要认证**

### 请求

```
GET /api/v1/oss/sts-token
Authorization: Bearer <token>
```

### 响应

**状态码：** `200 OK`

返回 `StandardResponse`，`data` 为临时凭证与上传上下文信息：

```json
{
  "code": 0,
  "data": {
    "access_key_id": "STS.NNNNN",
    "access_key_secret": "NNNNN",
    "security_token": "CAIS...",
    "expiration": "2026-04-21T12:34:56Z",
    "bucket": "example-bucket",
    "region": "cn-hangzhou",
    "endpoint": "https://oss-cn-hangzhou.aliyuncs.com",
    "dir": "prod/track/user/1001/"
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `access_key_id` | string | STS 临时 AccessKeyId |
| `access_key_secret` | string | STS 临时 AccessKeySecret |
| `security_token` | string | STS SecurityToken（使用 OSS SDK/签名请求时需要携带） |
| `expiration` | string | 过期时间（服务端透传 STS 返回值） |
| `bucket` | string | Bucket 名称 |
| `region` | string | Bucket 所在 Region（可用于客户端选择配置） |
| `endpoint` | string | OSS Endpoint（可用于客户端直传） |
| `dir` | string | 服务端分配的上传目录前缀；建议对象 key 以该前缀开头（如 `dir + filename`） |

### 常见错误

- `401 Unauthorized`：缺少/无效/过期的 Token
- `500 Internal Server Error`
  - 服务端未配置 STS（返回 `{"error":"oss sts not configured"}`）
  - 调用阿里云 STS 失败（返回 `{"error":"..."}`）

### 示例（curl）

```bash
curl -X GET "http://<host>:<port>/api/v1/oss/sts-token" \
  -H "Authorization: Bearer <token>" \
  -H "X-User-ID: 1001"
```

---

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
- `404 Not Found`
  - `track_id` 对应轨迹不存在
- `500 Internal Server Error`
  - 服务端执行收藏失败

### 示例（curl）

```bash
curl -X POST "http://<host>:<port>/api/v1/track_collect?track_id=trk2" \
  -H "Authorization: Bearer <token>"
```

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

## 7. 轨迹搜索列表

按关键字搜索轨迹列表。

**需要认证**

### 请求

```
GET /api/v1/track/search/list?keyword=:keyword
Authorization: Bearer <token>
```

**Query 参数：**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `keyword` | string | 否 | 搜索关键字；为空时返回最近的部分轨迹（最多 50 条） |

### 响应

**状态码：** `200 OK`

直接返回 `TrackSummary[]`（非 `StandardResponse`）：

```json
[
  {
    "id": "trk1",
    "user_id": 1001,
    "city_code": "330100",
    "track_type": "徒步",
    "start_time": "2026-04-20T12:00:00Z",
    "city_name": "杭州市",
    "nickname": "Alice",
    "user_avatar_url": "https://example.com/avatar.png",
    "title": "西湖徒步",
    "distance": 1200.5,
    "duration": 360,
    "elevation_gain": 80,
    "raw_track_url": "/api/v1/static/raw_tracks/trk1.dat",
    "track_screenshot_url": "/api/v1/static/screenshots/trk1.jpg",
    "collected": true,
    "collect_count": 12,
    "navigate_count": 3
  }
]
```

### 示例（curl）

```bash
curl -X GET "http://<host>:<port>/api/v1/track/search/list?keyword=西湖" \
  -H "Authorization: Bearer <token>" \
  -H "X-User-ID: 1001"
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
