# 用户接口

> 公共请求、认证和错误响应见 [common.md](common.md)。

## 10. 获取用户详情

获取指定用户的公开主页信息，并返回用户轨迹、关注/粉丝和成就相关统计。当前登录用户查看自己时会额外返回手机号、客户端语言、性别、生日和星座；查看他人时不返回这些私有字段。成就信息为公开主页信息，查看自己和查看他人都会返回。

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
    "background_url": "",
    "gender": "",
    "birthday": "",
    "zodiac_sign": "",
    "phone": "13800000000",
    "client_language": "zh-CN",
    "created_at": "2026-04-20T12:00:00Z",
    "updated_at": "2026-04-20T12:00:00Z",

    "total_distance": 2000,
    "track_count": 2,
    "track_used_count": 3,
    "following_count": 8,
    "follower_count": 12,
    "received_like_count": 6,
    "is_following": false,
    "is_self": true,
    "account_restriction": {
      "id": 12,
      "user_id": 1001,
      "status": "active",
      "reason": "违规上传内容",
      "operator": "ops",
      "expires_at": "2026-06-25T10:00:00+08:00",
      "created_at": "2026-06-18T10:00:00+08:00",
      "updated_at": "2026-06-18T10:00:00+08:00"
    },
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
| `data.background_url` | string | 个人主页顶部背景图；空串使用默认背景，启用缓存时为 `/api/v1/static/profile_backgrounds/<user_id>_<source_hash>.jpg`。 |
| `data.gender` | string | 仅本人详情返回；空串未设置，`male` 男，`female` 女。 |
| `data.birthday` | string | 仅本人详情返回；`YYYY-MM-DD` 日期字符串，空串表示未设置。查看他人时省略，不自动填入默认生日。 |
| `data.zodiac_sign` | string | 仅本人详情返回；根据生日计算的只读英文 code，未设置/无效生日为空串，查看他人时省略。客户端按 code 本地化显示，空串/缺失/未知值隐藏；规则见下方“星座派生规则”。 |
| `data.province_code` | string | 仅本人详情返回；最近确认所在地的省级行政区编码，未设置为空串。 |
| `data.province_name` | string | 仅本人详情返回；服务端根据行政区编码生成的省份名称，未设置为空串。 |
| `data.city_code` | string | 仅本人详情返回；最近确认所在地的归一化城市编码，未设置为空串。 |
| `data.city_name` | string | 仅本人详情返回；服务端根据行政区编码生成的城市名称，未设置为空串。 |
| `data.phone` | string | 手机号（可能为空）；仅查看自己时返回。 |
| `data.client_language` | string | 客户端语言；仅查看自己时返回。 |
| `data.created_at` | string | 创建时间。 |
| `data.updated_at` | string | 更新时间。 |
| `data.total_distance` | number | 总里程（米）：该用户在 `track_records` 中的轨迹 `distance` 加和（按“我的轨迹”口径：排除删除与进行中）。 |
| `data.track_count` | int64 | 轨迹总数：该用户在 `track_records` 中的轨迹数量（同口径）。 |
| `data.track_used_count` | int64 | 轨迹被使用总次数：该用户轨迹在 `track_navigations` 中产生的使用记录数总和（同口径过滤）。 |
| `data.following_count` | int64 | 该用户关注的人数。 |
| `data.follower_count` | int64 | 该用户的粉丝数。 |
| `data.received_like_count` | int64 | 其他用户对该用户公开、已结束且已上传轨迹的现存点赞总数（不含自赞）；统计规则见 [track-likes.md](track-likes.md)。 |
| `data.is_following` | bool | 当前登录用户是否关注该用户；查看自己时固定为 `false`。 |
| `data.is_self` | bool | 当前登录用户是否正在查看自己的主页。 |
| `data.account_restriction` | object | 当前登录用户查看自己且账号存在生效限制时返回；查看他人或未被限制时不返回。结构同 [account-restriction.md](account-restriction.md#账号限制错误) 中的限制记录。 |
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

支持一次更新一个或多个字段。省略或 `null` 的字段保持不变，显式空串可清空签名、背景图、性别或生日；旧客户端继续使用原有字段。背景图应用于个人主页顶部。

**需要认证**

### 说明

- 用户身份以 `Authorization` 解析出的 `user_id` 为准；接口不接收 `user_id` 参数。
- 只允许更新自己的个人信息。
- 生日为可选私有资料，只在本人详情、本人登录和本人资料更新响应中下发，不在他人主页、关注/粉丝列表中公开。日期不含时间和时区，不转换为时间戳。
- `zodiac_sign` 由服务端根据生日派生，不接受单独设置，不存数据库；修改生日立即重算，清除生日同时返回空串。客户端只提交 `birthday`，不提交星座。
- 背景图沿用 OSS 上传和已鉴权静态资源下载；数据库保存原始 OSS 地址，客户端响应使用独立缓存分类 `profile_backgrounds`，缓存键包含地址摘要以避免旧图缓存。背景图缓存配置启用时，更新前先完成下载，失败返回 502 且保留旧值。
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
  "signature": "hello world",
  "background_url": "https://example.com/background.jpg",
  "gender": "female",
  "birthday": "2000-01-01"
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `avatar_url` | string | 否 | 新头像地址（必须为非空字符串） |
| `name` | string | 否 | 新昵称（必须为非空字符串） |
| `signature` | string | 否 | 个性签名（允许传空串用于清空） |
| `background_url` | string | 否 | 客户端经现有 OSS STS 上传后的 HTTP(S) 原始地址，最多 2048 字节；空串恢复默认背景。不能提交本机文件地址。 |
| `gender` | string | 否 | 空串未设置、`male` 男、`female` 女；其他值返回 400。 |
| `birthday` | string | 否 | 严格 `YYYY-MM-DD`，范围为 `1900-01-01` 至服务端本地当天（含）；非法日期、未来日期或格式不匹配返回 400。空串清除，省略/`null` 保持原值。 |

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
    "background_url": "/api/v1/static/profile_backgrounds/1001_<source_hash>.jpg",
    "gender": "female",
    "birthday": "2000-01-01",
    "zodiac_sign": "capricorn",
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
  - `gender` 非法或 `background_url` 不是 HTTP(S) 地址、超过 2048 字节
  - `birthday` 格式、实际日期或日期范围非法（返回 `{"error":"birthday must be empty or a valid YYYY-MM-DD date between 1900-01-01 and today"}`）；无效请求不修改原资料
  - 未传任何可更新字段（返回 `{"error":"no fields to update"}`）
  - `avatar_url` 或 `name` 传了空字符串（返回 `{"error":"avatar_url is required"}` / `{"error":"name is required"}`）
- `401 Unauthorized`
  - 缺少/无效/过期的 Token
- `502 Bad Gateway`
  - 新背景图无法缓存，未修改原背景图
- `500 Internal Server Error`
  - 服务端更新个人信息失败

---

### 存量数据兼容

MySQL 启动时幂等补充 `users.background_url VARCHAR(2048) NULL`、`users.gender VARCHAR(16) NOT NULL DEFAULT ''`、`users.birthday VARCHAR(10) NOT NULL DEFAULT ''`；旧记录均显示默认背景、未设置性别和未设置生日。Mongo 文档缺少新字段时按空值读取；内存仓储保持相同语义。先部署服务端（包含迁移与静态图片缓存），再发布调用新字段的客户端。

客户端在个人信息的性别下方展示生日。未设置显示“未设置”，首次打开选择器定位到 `2000-01-01`，但仅确认后提交；已有值时回显，取消不保存，清除提交 `{"birthday":""}`。旧服务端可能忽略新字段，客户端必须检查更新响应回显的生日与提交值一致，不能只按 HTTP 200 判断保存成功。

### 星座派生规则

`zodiac_sign` 在本人详情、登录 `user` 和 `PUT /user/profile/update` 响应中返回。服务端唯一计算入口为 `internal/service/user_zodiac.go`，只依据已保存生日的公历月日，采用以下固定区间（起止日期均包含）；不是依赖出生年份、时间或地点的精确星盘。有效闰年 `02-29` 归双鱼座。空值、格式错误、非真实日期或超出生日允许范围时返回空串，不兜底成某个星座。

| code | 中文 | 公历日期区间 |
| --- | --- | --- |
| `aries` | 白羊座 | 03-21 ～ 04-19 |
| `taurus` | 金牛座 | 04-20 ～ 05-20 |
| `gemini` | 双子座 | 05-21 ～ 06-20 |
| `cancer` | 巨蟹座 | 06-21 ～ 07-22 |
| `leo` | 狮子座 | 07-23 ～ 08-22 |
| `virgo` | 处女座 | 08-23 ～ 09-22 |
| `libra` | 天秤座 | 09-23 ～ 10-22 |
| `scorpio` | 天蝎座 | 10-23 ～ 11-21 |
| `sagittarius` | 射手座 | 11-22 ～ 12-21 |
| `capricorn` | 摩羯座 | 12-22 ～ 次年 01-19 |
| `aquarius` | 水瓶座 | 01-20 ～ 02-18 |
| `pisces` | 双鱼座 | 02-19 ～ 03-20 |

区间参考 [Zodiac Sign Dates](https://astrologist.com/learn/zodiac-signs/zodiac-signs-dates)，以上表格为本产品固定协议，不随外部页面更新而变更。客户端不复制计算表，只翻译 code；本地缓存随生日一同更新，未确认的日期选择不会提前改变标签。星座目前同生日仅本人可见，公开星座须单独确定隐私策略。

## 更新最近确认所在地

更新当前登录用户最近确认的国内省市。客户端只提交归一化城市编码，不提交原始经纬度、省份或城市名称；服务端使用内置行政区数据生成并保存 `province_code`、`province_name`、`city_code`、`city_name`。四字段仅在本人用户详情返回，客态主页、关注和粉丝列表不公开。

**需要认证**

### 请求

```
PUT /api/v1/user/profile/location
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "city_code": "330100"
}
```

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `city_code` | string | 是 | 六位城市 AdCode。普通城市使用地级市编码，直辖市使用 `110000`、`120000`、`310000` 或 `500000`。未知编码、境外或其他不受支持的编码返回 400。 |

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

接口只局部更新四个省市字段，不覆盖昵称、头像、生日等其他资料。新旧四字段完全一致时也返回成功，但不执行数据库写入且不刷新 `updated_at`，因此客户端可在本地状态缺失时安全重试。

### 错误响应

- `400 Bad Request`：请求体不是合法 JSON，或 `city_code` 不是受支持的国内城市编码。
- `401 Unauthorized`：缺少、无效或过期的 Token。
- `403 Forbidden`：账号当前被限制修改个人资料。
- `404 Not Found`：当前用户不存在。
- `500 Internal Server Error`：读取或更新所在地失败。

### 客户端同步口径与存量兼容

Android 只消费已有定位结果中的有效省市，不为该功能新增后台定位或逐轨迹点逆地理编码。每次运动会话首个有效地址立即检查一次；普通前台定位每 7 天检查一次。已同步、待同步、检查时间和失败退避状态按账号隔离，只有城市编码变化才调用本接口，失败保留待同步值并退避重试。

MySQL 启动时幂等补充 `users.province_code VARCHAR(6)`、`users.province_name VARCHAR(64)`、`users.city_code VARCHAR(6)`、`users.city_name VARCHAR(64)`，均为 `NOT NULL DEFAULT ''`。Mongo 和内存仓储缺失字段按空值处理。先部署服务端，再发布调用本接口的客户端。

---

## 个人主页轨迹列表

以下接口用于查看指定用户的主页，均需有效业务 JWT；`user_id` 是主页所属用户，当前浏览者仍以 Token 为准。不能用这两个接口代替主态的 `/track/my/list` 和 `/track/collected/list`。

### 已传轨迹

`GET /api/v1/user/:user_id/tracks?limit=20&cursor=<next_cursor>`

只返回该用户 `status=normal`、`is_running=false` 且原始轨迹地址非空的已上传轨迹，排除私密与删除记录。按 `start_time DESC, id DESC` 排序，过滤条件在仓储分页前生效；即使本人访问该公开接口，也不返回私密轨迹。

### 收藏的轨迹

`GET /api/v1/user/:user_id/collections?limit=20&cursor=<next_cursor>`

返回该用户收藏中仍可公开查看的轨迹，排除不存在、已删除、私密和进行中的轨迹。按收藏时间倒序，使用收藏时间和轨迹 ID 组成的游标。遇到失效收藏会继续查找后续可见记录，避免空页造成错误的列表结束。

### 共同响应与错误

```json
{
  "code": 0,
  "data": {
    "items": [],
    "total_count": 0,
    "has_more": false
  }
}
```

- `items` 使用 `TrackSummary`，含作者资料、资源 URL、收藏/使用总数以及 `collected`；空列表固定为 `[]`。
- `collected` 表示**当前浏览者**是否收藏，不表示主页所属用户的收藏状态。收藏列表中该字段也可能为 `false`。
- `total_count` 仅统计本接口可见记录，与当前列表口径一致，不直接使用用户详情中可能包含私密轨迹的 `track_count`。
- `limit` 默认 20、最大 50；`cursor` 原样透传，两个接口的游标不能混用。仅 `has_more=true` 时提供 `next_cursor`。
- 缺少或无效 Token 返回 401；非法 `user_id`、`limit` 或 `cursor` 返回 400；目标用户不存在返回 404；已存在用户没有记录时返回 200 和空列表。
- 无数据库 schema 变更；MySQL、Mongo 和内存仓储均支持。先部署服务端，再发布客态主页客户端。

### 个人主页获赞

用户详情继续返回 `received_like_count`，主态和客态个人主页仅展示获赞总数，不提供点击入口。获赞列表和点赞人名单查询接口已移除。统计规则见 [track-likes.md](track-likes.md)。
