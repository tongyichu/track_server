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
| 9 | [我的轨迹列表](#9-我的轨迹列表) | GET | `/track/my/list` | ✅ |
| 10 | [获取用户详情](#10-获取用户详情) | GET | `/user/:user_id/detail` | ✅ |
| 11 | [更新轨迹信息](#11-更新轨迹信息) | PUT | `/track/:track_id/update` | ✅ |
| 12 | [删除轨迹](#12-删除轨迹) | DELETE | `/track/:track_id` | ✅ |
| 13 | [用户已收藏轨迹列表](#13-用户已收藏轨迹列表) | GET | `/track/collected/list` | ✅ |
| 14 | [更新个人信息](#14-更新个人信息) | PUT | `/user/profile/update` | ✅ |
| 15 | [App 升级检查](#15-app-升级检查) | GET | `/upgrade/check` | ❌ |
| 16 | [获取运动类型](#16-获取运动类型) | GET | `/track/types` | ✅ |

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
| `locate_addr` | string | 轨迹的具体位置信息 |
| `track_type` | string | 轨迹类型，例如 `徒步` / `跑步` / `骑车` / `自驾` |
| `coordinate_system` | string | 坐标系，例如 `WGS84` / `GCJ02` / `BD09` |
| `title` | string | 轨迹标题 |
| `start_time` | string | 开始时间（RFC3339/ISO8601，服务端序列化时间格式） |
| `end_time` | string | 结束时间（RFC3339/ISO8601，服务端序列化时间格式） |
| `distance` | number | 距离（米） |
| `duration` | int | 时长（秒） |
| `avg_speed_kmh` | number | 平均速度（km/h） |
| `calories_burned` | number | 热量消耗（千卡） |
| `elevation_gain` | int | 累计爬升（米） |
| `raw_track_url` | string | 原始轨迹文件可下载链接（服务端本地缓存 URL，例如 `/api/v1/static/raw_tracks/<track_id>.dat`） |
| `track_screenshot_url` | string | 轨迹截图可下载链接（服务端本地缓存 URL，例如 `/api/v1/static/screenshots/<track_id>.jpg`） |
| `track_no_map_bg_screenshot_url` | string | 无地图背景的轨迹路线截图可下载链接（服务端本地缓存 URL，例如 `/api/v1/static/screenshots/<track_id>_no_map_bg.jpg`） |
| `is_running` | bool | 是否进行中 |
| `status` | int | 轨迹状态：`0` 删除，`1` 正常，`2` 私密 |
| `created_at` | string | 创建时间 |
| `updated_at` | string | 更新时间 |
| `deleted_at` | string | 删除时间（软删除；未删除时为空或不返回） |

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
  "locate_addr": "杭州市西湖区",
  "track_type": "跑步",
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
| `city_code` | string | 否 | 城市 Code（标识轨迹所属城市） |
| `locate_addr` | string | 否 | 轨迹的具体位置信息，最大长度 `128` |
| `track_type` | string | 否 | 轨迹类型，例如 `徒步` / `跑步` / `骑车` / `自驾` |
| `coordinate_system` | string | 否 | 坐标系，例如 `WGS84` / `GCJ02` / `BD09` |
| `start_time` | string | 否 | 开始时间，RFC3339/ISO8601 格式 |
| `end_time` | string | 否 | 结束时间，RFC3339/ISO8601 格式，必须 `>= start_time` |
| `distance` | number | 否 | 距离（米），必须 `>= 0` |
| `duration` | int | 否 | 时长（秒），必须 `>= 0` |
| `calories_burned` | number | 否 | 热量消耗（千卡），必须 `>= 0` |
| `elevation_gain` | int | 否 | 累计爬升（米），必须 `>= 0` |
| `raw_track_url` | string | 否 | 原始轨迹文件 OSS 地址（建议传 OSS HTTP URL，可带签名参数） |
| `track_screenshot_url` | string | 否 | 轨迹截图 OSS 地址（建议传 OSS HTTP URL，可带签名参数） |
| `track_no_map_bg_screenshot_url` | string | 否 | 无地图背景的轨迹路线截图 OSS 地址（建议传 OSS HTTP URL，可带签名参数） |
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
    "locate_addr": "杭州市西湖区",
    "track_type": "跑步",
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
    "is_running": true,
    "status": 1,
    "created_at": "2026-04-20T12:00:00Z",
    "updated_at": "2026-04-20T12:00:00Z"
  }
}
```

说明：`raw_track_url` / `track_screenshot_url` / `track_no_map_bg_screenshot_url` 在请求时是 OSS 地址，但响应会被服务端替换为可直接从业务服务器下载的本地链接（路径在 `/api/v1/static/...` 下，需要登录态）。

### 示例（curl）

```bash
curl -X POST "http://<host>:<port>/api/v1/track/create" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -H "X-User-ID: 1001" \
  -d '{
    "title": "傍晚夜跑",
    "city_code": "330100",
    "locate_addr": "杭州市西湖区",
    "track_type": "跑步",
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
- 返回结果中的 `track_type` 为轨迹类型，例如 `徒步` / `跑步` / `骑车` / `自驾`。
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
        "city_code": "330100",
        "locate_addr": "杭州市西湖区",
        "track_type": "徒步",
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
    "track_type": "徒步",
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

### 说明

- 返回的是 `TrackSummary` 分页结果（轻量字段），而不是完整的 `Track`。
- 返回结果中 `collected` 表示当前鉴权用户是否已收藏该轨迹。
- 返回结果中的 `collect_count` 表示该轨迹被收藏的总数。
- 返回结果中的 `navigate_count` 表示该轨迹被其他用户用于导航的次数。
- 返回结果中的 `nickname` / `user_avatar_url` 为轨迹所属用户的昵称/头像 URI。
- 返回结果中的 `city_code` / `city_name` 为轨迹所属城市 Code 及其对应的城市名称。
- 返回结果中的 `locate_addr` 为轨迹的具体位置信息。
- 返回结果中的 `track_type` 为轨迹类型，例如 `徒步` / `跑步` / `骑车` / `自驾`。
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
        "city_code": "330100",
        "locate_addr": "杭州市西湖区",
        "track_type": "徒步",
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
        "city_code": "330100",
        "locate_addr": "杭州市西湖区",
        "track_type": "徒步",
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
| `data.items[].city_code` | string | 城市 Code。 |
| `data.items[].locate_addr` | string | 轨迹的具体位置信息。 |
| `data.items[].track_type` | string | 轨迹类型，例如 `徒步` / `跑步` / `骑车` / `自驾`。 |
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

## 10. 获取用户详情

获取指定用户的详细信息，并返回用户轨迹相关的统计数据。

**需要认证**

### 请求

```
GET /api/v1/user/:user_id/detail
Authorization: Bearer <token>
```

#### Path 参数

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `user_id` | int64 | 是 | 用户 ID。 |

#### Body

无

### 响应

**状态码：** `200 OK`

返回 `StandardResponse`，`data` 为用户信息 + 统计字段：

```json
{
  "code": 0,
  "data": {
    "id": 1001,
    "nickname": "Alice",
    "avatar_url": "/api/v1/static/default_avatars/girl_01.png",
    "signature": "",
    "phone": "13800000000",
    "client_language": "zh-CN",
    "created_at": "2026-04-20T12:00:00Z",
    "updated_at": "2026-04-20T12:00:00Z",

    "total_distance": 2000,
    "track_count": 2,
    "track_used_count": 3
  }
}
```

**字段说明：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `data.id` | int64 | 用户 ID。 |
| `data.nickname` | string | 用户昵称。 |
| `data.avatar_url` | string | 用户头像 URL（为空时服务端会返回默认头像；若启用头像缓存，可能返回服务端本地静态下载地址 `/api/v1/static/avatars/...`）。 |
| `data.signature` | string | 个性签名。 |
| `data.phone` | string | 手机号（可能为空）。 |
| `data.client_language` | string | 客户端语言。 |
| `data.created_at` | string | 创建时间。 |
| `data.updated_at` | string | 更新时间。 |
| `data.total_distance` | number | 总里程（米）：该用户在 `track_records` 中的轨迹 `distance` 加和（按“我的轨迹”口径：排除删除与进行中）。 |
| `data.track_count` | int64 | 轨迹总数：该用户在 `track_records` 中的轨迹数量（同口径）。 |
| `data.track_used_count` | int64 | 轨迹被使用总次数：该用户轨迹在 `track_navigations` 中产生的使用记录数总和（同口径过滤）。 |

### 错误响应

- `400 Bad Request`
  - `user_id` 非法
- `401 Unauthorized`
  - 缺少/无效/过期的 Token
- `404 Not Found`
  - 用户不存在
- `500 Internal Server Error`
  - 服务端统计计算失败

错误响应格式示例：

```json
{
  "error": "..."
}
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
  "city_code": "330100",
  "locate_addr": "杭州市西湖区",
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
| `city_code` | string | 否 | 城市 Code（仅当原值为空时才会写入） |
| `locate_addr` | string | 否 | 轨迹的具体位置信息，最大长度 `128`（仅当原值为空时才会写入） |
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
    "city_code": "330100",
    "locate_addr": "杭州市西湖区",
    "track_no_map_bg_screenshot_url": "https://<bucket>.oss-<region>.aliyuncs.com/prod/track/.../xxx_no_map_bg.jpg"
  }'
```

### 错误响应

- `400 Bad Request`
  - 请求体不是合法 JSON（返回 `{"error":"invalid payload"}`）
  - 请求体未包含任何字段（返回 `{"error":"no fields to update"}`）
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
        "track_type": "徒步",
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

## 14. 更新个人信息

支持一次更新一个或多个字段。

**需要认证**

### 说明

- 用户身份以 `Authorization` 解析出的 `user_id` 为准；接口不接收 `user_id` 参数。
- 只允许更新自己的个人信息。
- 更新头像成功后，响应中的 `avatar_url` 会被服务端替换为本地静态资源下载链接（例如 `/api/v1/static/avatars/<user_id>.png`）。

### 请求

```
PUT /api/v1/user/profile/update
Authorization: Bearer <token>
Content-Type: application/json
```

**请求体（字段均为可选，至少传一个）：**

```json
{
  "avatar_url": "https://example.com/avatar.png",
  "name": "Alice",
  "signature": "hello world"
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `avatar_url` | string | 否 | 新头像地址（必须为非空字符串） |
| `name` | string | 否 | 新昵称（必须为非空字符串） |
| `signature` | string | 否 | 个性签名（允许传空串用于清空） |

### 响应

**状态码：** `200 OK`

返回统一响应格式 `StandardResponse`，`data` 为更新后的 `User`：

```json
{
  "code": 0,
  "data": {
    "id": 1001,
    "nickname": "Alice",
    "avatar_url": "/api/v1/static/avatars/1001.png",
    "signature": "hello world",
    "phone": "",
    "client_language": "",
    "created_at": "2026-04-20T12:00:00Z",
    "updated_at": "2026-04-20T12:00:00Z"
  }
}
```

### 错误响应

- `400 Bad Request`
  - 请求体不是合法 JSON（返回 `{"error":"invalid payload"}`）
  - 未传任何可更新字段（返回 `{"error":"no fields to update"}`）
  - `avatar_url` 或 `name` 传了空字符串（返回 `{"error":"avatar_url is required"}` / `{"error":"name is required"}`）
- `401 Unauthorized`
  - 缺少/无效/过期的 Token
- `500 Internal Server Error`
  - 服务端更新个人信息失败

---

## 15. App 升级检查

> 客户端在「启动 / 切回前台 / 设置页手动检查」等场景调用，根据当前平台与本地 `version_code` 询问服务端是否有新版本。

### 请求

`GET /api/v1/upgrade/check`

**注意：本接口为公开接口，不需要 `Authorization` Header。**

### 请求头

| Header 名称 | 类型 | 必填 | 说明 |
|-------------|------|------|------|
| `X-Platform` | string | 是 | 客户端平台，固定为 `android` 或 `ios`（沿用公共请求头，不再单独通过 query 传递） |

### Query 参数

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `version_code` | int64 | 是 | 当前安装版本的 `version_code`（单调递增整数）。未知时传 `0` |

### 请求示例

```
GET /api/v1/upgrade/check?version_code=120
X-Platform: android
```

### 响应

**状态码：** `200 OK`

返回统一响应格式 `StandardResponse`，`data` 为 `UpgradeCheckResult`。

#### 字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `has_update` | bool | 是否存在可升级版本。`false` 时其它字段无意义 |
| `force_update` | bool | 是否强制升级。判定规则见下文 |
| `latest_version_name` | string | 服务端最新发布版本的版本名（如 `1.2.3`） |
| `latest_version_code` | int64 | 服务端最新发布版本的 `version_code` |
| `min_supported_version_code` | int64 | 最低支持版本号；客户端 `version_code < min_supported_version_code` 时必须强升 |
| `package_url` | string | 安装包下载地址。**Android** 为 OSS 直接下载链接；**iOS** 为 AppStore 跳转链接 |
| `package_size` | int64 | 安装包大小（字节）。iOS 通常为 `0` |
| `package_md5` | string | 安装包 MD5（可选）。iOS 通常为空 |
| `release_notes` | string | 版本说明文案，可包含换行 |

#### 强制升级判定逻辑

服务端会基于以下规则计算 `force_update`：

1. 若服务端没有任何 `published` 状态的版本，返回 `has_update=false`；
2. 若 `version_code >= latest_version_code`，返回 `has_update=false`；
3. 否则 `has_update=true`，并按以下任一条件触发强升（`force_update=true`）：
   - 客户端 `version_code < min_supported_version_code`
   - 服务端最新版本被标记为 `force_update=true`

客户端处理建议：
- `has_update=false`：不提示。
- `has_update=true && force_update=false`：可关闭的升级提示。
- `has_update=true && force_update=true`：不可关闭的升级提示，未升级前阻断核心功能。

### 响应示例

#### 1）有可升级版本，非强制

```json
{
  "code": 0,
  "data": {
    "has_update": true,
    "force_update": false,
    "latest_version_name": "1.3.0",
    "latest_version_code": 130,
    "min_supported_version_code": 100,
    "package_url": "https://track-resource.oss-cn-beijing.aliyuncs.com/release/android/1714378800-app-1.3.0.apk",
    "package_size": 28456321,
    "package_md5": "a1b2c3d4e5f60718293a4b5c6d7e8f90",
    "release_notes": "1. 新增轨迹分享\n2. 修复地图卡顿"
  }
}
```

#### 2）当前版本过低，必须强升

```json
{
  "code": 0,
  "data": {
    "has_update": true,
    "force_update": true,
    "latest_version_name": "1.3.0",
    "latest_version_code": 130,
    "min_supported_version_code": 120,
    "package_url": "https://apps.apple.com/cn/app/idXXXXXXXXX",
    "package_size": 0,
    "package_md5": "",
    "release_notes": "本次为安全更新，必须升级后才能继续使用。"
  }
}
```

#### 3）已是最新或服务端无发布版本

```json
{
  "code": 0,
  "data": {
    "has_update": false,
    "force_update": false,
    "latest_version_name": "",
    "latest_version_code": 0,
    "min_supported_version_code": 0,
    "package_url": "",
    "package_size": 0,
    "package_md5": "",
    "release_notes": ""
  }
}
```

### 错误响应

- `400 Bad Request`
  - `version_code` 非整数（返回 `{"error":"version_code must be an integer"}`）
  - `platform` 不是 `android` / `ios`（返回 `{"error":"platform must be android or ios"}`）
  - `version_code < 0`（返回 `{"error":"current_version_code must be >= 0"}`）
- `500 Internal Server Error`
  - 服务端查询发布信息失败

---

## 16. 获取运动类型

获取客户端创建/编辑轨迹时可选择的运动类型列表，并返回当前用户的轨迹统计数据。

**需要认证**

### 说明

- 默认返回：`徒步`、`跑步`、`爬山`、`骑行`。
- 服务端可通过环境变量 `TRACK_TYPES` 配置运动类型，支持使用英文逗号、分号、中文逗号、顿号或竖线分隔，例如：`TRACK_TYPES=徒步,跑步,爬山,骑行,滑雪`。
- 服务端会自动过滤空项和重复项；若未配置或配置为空，则使用默认列表。
- 每个运动类型会返回一个图标链接 `icon_url`，图标文件来自服务端静态目录 `/api/v1/static/track_type_icon/`；默认对应关系为：`徒步 -> hiking.svg`、`跑步 -> running.svg`、`爬山 -> climbing.svg`、`骑行 -> riding.svg`。
- 统计数据按 `Authorization` Token 解析出的当前用户统计 `track_records`：排除删除与进行中轨迹，仅统计 `正常/私密` 轨迹。
- 统计字段包括：总里程、轨迹次数、总耗时、总热量。

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
  "data": {
    "items": [
      {
        "name": "徒步",
        "icon_url": "/api/v1/static/track_type_icon/hiking.svg"
      },
      {
        "name": "跑步",
        "icon_url": "/api/v1/static/track_type_icon/running.svg"
      }
    ],
    "stats": {
      "total_distance": 2000.5,
      "track_count": 2,
      "total_duration": 1000,
      "total_calories": 140.5
    }
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `data.items` | `TrackTypeOption[]` | 可选运动类型列表。 |
| `data.items[].name` | string | 运动类型名称。 |
| `data.items[].icon_url` | string | 运动类型图标静态资源链接，路径位于 `/api/v1/static/track_type_icon/` 下。 |
| `data.stats.total_distance` | number | 当前用户总里程，单位米。 |
| `data.stats.track_count` | int64 | 当前用户轨迹次数。 |
| `data.stats.total_duration` | int64 | 当前用户总耗时，单位秒。 |
| `data.stats.total_calories` | number | 当前用户总热量，单位千卡。 |

### 错误响应

- `401 Unauthorized`
  - 缺少/无效/过期的 Token
