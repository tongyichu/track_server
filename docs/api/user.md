# 用户接口

> 公共请求、认证和错误响应见 [common.md](common.md)。

## 10. 获取用户详情

获取指定用户的公开主页信息，并返回用户轨迹、关注/粉丝和成就相关统计。当前登录用户查看自己时会额外返回手机号和客户端语言；查看他人时不返回这些私有字段。成就信息为公开主页信息，查看自己和查看他人都会返回。

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
    "track_used_count": 3,
    "following_count": 8,
    "follower_count": 12,
    "is_following": false,
    "is_self": true,
    "achievement": {
      "level": {
        "level": 1,
        "name": "初上路",
        "xp": 0
      },
      "earned_badge_count": 3,
      "recent_badges": [
        {
          "code": "run_10k",
          "type": "badge",
          "category": "跑步",
          "name": "10K 完成",
          "description": "单次跑步距离达到 10km",
          "rarity": "rare",
          "icon_url": "",
          "target_value": 10,
          "earned": true,
          "earned_at": "2026-04-20T12:00:00Z",
          "current_value": 10,
          "progress": 1
        }
      ]
    }
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
| `data.phone` | string | 手机号（可能为空）；仅查看自己时返回。 |
| `data.client_language` | string | 客户端语言；仅查看自己时返回。 |
| `data.created_at` | string | 创建时间。 |
| `data.updated_at` | string | 更新时间。 |
| `data.total_distance` | number | 总里程（米）：该用户在 `track_records` 中的轨迹 `distance` 加和（按“我的轨迹”口径：排除删除与进行中）。 |
| `data.track_count` | int64 | 轨迹总数：该用户在 `track_records` 中的轨迹数量（同口径）。 |
| `data.track_used_count` | int64 | 轨迹被使用总次数：该用户轨迹在 `track_navigations` 中产生的使用记录数总和（同口径过滤）。 |
| `data.following_count` | int64 | 该用户关注的人数。 |
| `data.follower_count` | int64 | 该用户的粉丝数。 |
| `data.is_following` | bool | 当前登录用户是否关注该用户；查看自己时固定为 `false`。 |
| `data.is_self` | bool | 当前登录用户是否正在查看自己的主页。 |
| `data.achievement.level` | object | 用户当前等级，结构同 `AchievementLevel`：`level`、`name`、`xp`。 |
| `data.achievement.earned_badge_count` | int64 | 用户已获得的勋章总数，仅统计 `type=badge` 的成就奖励。 |
| `data.achievement.recent_badges` | array | 用户最近获得的 3 个勋章，按获得时间倒序；单条结构同 [achievement.md](achievement.md#31-成就奖励列表) 中 `rewards[]`。 |

### 错误响应

- `400 Bad Request`
  - `user_id` 非法
- `401 Unauthorized`
  - 缺少/无效/过期的 Token
- `404 Not Found`
  - 用户不存在
- `500 Internal Server Error`
  - 服务端统计或成就摘要计算失败

错误响应格式示例：

```json
{
  "error": "..."
}
```


---

## 42. 关注用户

当前登录用户关注指定用户。重复关注幂等成功；不能关注自己。

**需要认证**

### 请求

```
POST /api/v1/user/:user_id/follow
Authorization: Bearer <token>
```

### 响应

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
  - `user_id` 非法或关注自己
- `401 Unauthorized`
  - 缺少/无效/过期的 Token
- `404 Not Found`
  - 被关注用户不存在

---

## 43. 取消关注用户

当前登录用户取消关注指定用户。若原本未关注，也幂等成功。

**需要认证**

### 请求

```
DELETE /api/v1/user/:user_id/follow
Authorization: Bearer <token>
```

### 响应

```json
{
  "code": 0,
  "data": {
    "status": "ok"
  }
}
```

---

## 44. 查询关注状态

查询当前登录用户是否关注指定用户。

**需要认证**

### 请求

```
GET /api/v1/user/:user_id/follow/status
Authorization: Bearer <token>
```

### 响应

```json
{
  "code": 0,
  "data": {
    "is_following": true
  }
}
```

---

## 45. 关注列表

查询指定用户关注的人，按关注时间倒序分页。`cursor` 为服务端返回的 `next_cursor`，客户端原样透传。

**需要认证**

### 请求

```
GET /api/v1/user/:user_id/following/list?limit=20&cursor=<cursor>
Authorization: Bearer <token>
```

### 响应

```json
{
  "code": 0,
  "data": {
    "items": [
      {
        "id": 1002,
        "nickname": "Bob",
        "avatar_url": "/api/v1/static/default_avatars/girl_2.png",
        "signature": "",
        "following_count": 3,
        "follower_count": 5,
        "is_following": true,
        "created_at": "2026-06-12T10:00:00Z"
      }
    ],
    "next_cursor": "",
    "has_more": false,
    "total_count": 1
  }
}
```

---

## 46. 粉丝列表

查询指定用户的粉丝，按关注时间倒序分页。响应结构与关注列表一致。

**需要认证**

### 请求

```
GET /api/v1/user/:user_id/follower/list?limit=20&cursor=<cursor>
Authorization: Bearer <token>
```

### 响应

同 [关注列表](#45-关注列表)。

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
