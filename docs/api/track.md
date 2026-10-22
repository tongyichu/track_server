# 轨迹接口

> 公共请求、认证和错误响应见 [common.md](common.md)；通用对象字段见 [models.md](models.md)。

轨迹投稿、审核状态和优质投稿展示字段见 [track-submission.md](track-submission.md)。审核通过的推荐/搜索摘要会返回 `is_featured`、`featured_description`、`featured_cover_url`，并用投稿标题覆盖响应中的 `title`；`track_records.title` 不会被修改。


## 静态轨迹预览图

客户端在实际上传轨迹前，使用真实 GPS 数据和现有地图预览组件依次生成四张 JPEG，直传 OSS 后随创建或更新请求传入 `preview_images`。录制结束时仅创建远程空记录的流程无需提供图片。DevMode 开关仅控制是否显示上传确认预览，不控制正式上传所需的四图生成。

```json
{
  "preview_images": {
    "square_day_url": "https://<bucket>.oss-<region>.aliyuncs.com/.../square_day.jpg",
    "square_night_url": "https://<bucket>.oss-<region>.aliyuncs.com/.../square_night.jpg",
    "landscape_day_url": "https://<bucket>.oss-<region>.aliyuncs.com/.../landscape_day.jpg",
    "landscape_night_url": "https://<bucket>.oss-<region>.aliyuncs.com/.../landscape_night.jpg"
  }
}
```

- `square_*` 为 1:1，`landscape_*` 为 4:3；`day` / `night` 对应日间 / 夜间。
- 字段整体可选，旧客户端继续正常工作；提供时四个 HTTP(S) URL 均必填，每个最多 2048 字符。更新时整组替换，不受旧截图字段“仅原值为空时补写”的限制；省略或 null 保持原值，不支持传空对象删除。
- Track、TrackSummary、MyTrackSummary、CollectedTrackSummary 均返回 `preview_images`。存量无图轨迹返回空对象，不做请求时补图。首页及复用卡片按当前主题使用对应的 `landscape_*`，缺图时显示占位图，不在列表创建地图或重新生成。
- 响应中的地址形如 `/api/v1/track/NO.00000001/previews/landscape_day?v=<source_version>`，客户端必须保留版本参数并携带业务 JWT。列表和详情仅构造地址，不同步拉取四张 OSS 图片。
- 图片来源与文字随上传时的地图 provider / 语言固定；查看者切换语言不会重新生成底图。

### 获取预览图片

`GET /api/v1/track/:track_id/previews/:variant?v=<source_version>`，需要认证。`variant` 为上述四种 key（不带 `_url`）。作者可访问未删除的本人轨迹；其他用户仅可访问正常、已结束且已上传原始轨迹的记录。

命中缓存直接读取；未命中时使用轨迹归属用户的 OSS 下载凭证拉取所需的单张图片，同一资源并发请求合并。缓存 key 包含轨迹 ID、variant 和源 URL 摘要，避免日夜图或更新前后的图串用；返回 `Cache-Control: private, max-age=86400`。失效版本、未知 variant、缺图、删除或不可见返回 404，资源暂不可用返回 503。成功响应为图片二进制。

### 历史轨迹预览补齐（DevMode）

本组接口供 Android DevMode 手动批处理全站历史轨迹使用。沿用业务 JWT，不增加管理员、轨迹作者或二次权限验证；prepare、complete 和 clear 沿用账号限制中的上传规则。不会创建或重新发布轨迹，仅维护 `preview_images`。

1. `GET /api/v1/track/preview-backfill/list?cursor=<cursor>&limit=20`
   - 查询已结束、未删除、有 `raw_track_url` 且四图不完整的轨迹，按 `start_time DESC, id DESC` 游标分页。`limit` 默认 20、最大 50；筛选在仓储分页前完成。
   - `data.items` 每项为 `track_id`、`title`、`track_type`、`coordinate_system`、`source_version`；响应还包含 `next_cursor`、`has_more`、全站当前 `pending_count` 及缺图但无原始轨迹的 `without_raw_count`。列表不下载 GPS 或四图。
   - `source_version` 为原始轨迹源地址、坐标系、运动类型及最近清空版本的 SHA-256，不因点赞或其他无关字段变化而变化；每次清空都会改变版本。
2. `GET /api/v1/track/preview-backfill/:track_id/data`
   - 从原始轨迹缓存读取，复用 JSON / GeoJSON / KML / KMZ 解析器返回完整 GPS，不做路线简化或地图绘制。
   - 响应 `data = {"state":"pending","task":<最新任务元信息>,"points":[{"latitude":39.9,"longitude":116.4}]}`；坐标保留源坐标系，Android 复用现有转换后绘制。可选 `source_version` 查询参数不匹配时返回 409；省略时获取最新源。
   - 已有完整四图时 `state=already_complete`、`points=[]`。已删除、进行中或无原始文件返回 404；不可解析或无 GPS 返回 400，资源下载暂不可用返回 503。
3. `POST /api/v1/track/preview-backfill/:track_id/prepare`
   - JSON 请求体为 `{"source_version":"<data 返回的版本>","upload_id":"<客户端持久化的32位小写十六进制随机ID>"}`。
   - 返回 `data = {"state":"pending","upload_urls":{"square_day":"<签名PUT地址>","square_night":"...","landscape_day":"...","landscape_night":"..."}}`。使用 `Content-Type: image/jpeg` 直接 PUT，不携带业务 JWT；签名一般有效 15 分钟，不超过 STS 剩余有效期。失效后用同一 upload_id 重新请求，已经成功上传的对象可复用。
   - 地址限定在轨迹归属作者的 `previews/<owner>/backfill/<track_id>/<source_version>/<upload_id>/` 四个对象；不下发作者的 OSS STS 凭证。不同批次使用独立 upload_id。
   - 轨迹已齐图时仅返回 `{"state":"already_complete"}`，不签发上传地址。
4. `POST /api/v1/track/preview-backfill/:track_id/complete`
   - 使用与 prepare 相同的请求体；客户端不指定图片 URL。
   - 服务端经 OSS 内网逐张核对文件存在、JPEG、每张不超过 5 MiB，且 `square_*` 必须为 1024×1024、`landscape_*` 为 1024×768，然后条件更新四图及 `updated_at`，不更新其他轨迹字段。
   - 返回 `data = {"state":"completed","preview_images":<带版本的四图读取地址>}`。已完成重试或被其他流程抢先补齐时返回 `state=already_complete`，不覆盖现有完整四图。
   - 源地址、坐标系、运动类型、清空版本或原有四图在校验期间发生变化时不覆盖，返回 409、`error_code=preview_source_changed`。客户端重新读取最新任务后重试。缺图或图片不合规格返回 400。
5. `POST /api/v1/track/preview-backfill/clear`
   - 清空单条：`{"track_id":"NO.00000001"}`；清空全部：`{"all":true}`。两种作用域互斥，空请求、空 ID 或同时指定两者返回 400，绝不隐式清空全部。轨迹 ID 保留原始外部 ID，最长 64 字符。
   - 作用域为所有未删除轨迹（包括暂未上传原始 GPS 或进行中的轨迹），只清空四张预览图地址并更新 `updated_at`；不删除轨迹、原始 GPS、旧截图、OSS 对象或服务端图片缓存文件。单条不存在或已删除返回 404；全站没有轨迹时成功并返回 0。
   - 成功响应 `data = {"affected_count":123,"all":true}` 或 `{"affected_count":1,"all":false,"track_id":"NO.00000001"}`。数量表示重置的轨迹数，包含原本无图的记录。随后列表/详情的 `preview_images` 为空对象，旧版本图片新请求返回 404；已有设备本地图片缓存不会被远程擦除。
   - 为每次清空生成持久化内部版本，旧补图任务无法在清空后提交；客户端必须重新读取 GPS 任务并生成四图。内部版本放在既有 `preview_images` JSON/BSON 中，不下发给普通列表/详情，也不接受客户端写入，无新列或新表。
   - Android 清空成功后重置本账号补图游标、队列、计数及失败记录，清理专用补图缓存并重新统计待补数；刷新当前列表中的预览地址。不会自动开始补图。已结束且有原始 GPS 的轨迹重新进入待补列表，缺原始 GPS 的轨迹计入缺源数量。

无新任务表，完成状态以数据库四图为准。客户端按账号保存分页游标、尚未处理的当页队列、成功/跳过计数及失败记录；串行执行、后台/锁屏暂停、回到前台手动恢复。文件及逐图上传进度保存在单独缓存目录，失败项可单独重试。单批次固定语言、地图 provider、样式及尺寸；设置变化时暂停并提示恢复设置或重新扫描。

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
| `calories_burned` | number | 否 | 热量消耗（千卡），必须 `>= 0`；未传或为 `0` 且 `duration > 0` 时，服务端按运动类型 MET、默认体重 65kg 和时长兜底计算 |
| `elevation_gain` | int | 否 | 累计爬升（米），必须 `>= 0` |
| `raw_track_url` | string | 否 | 原始轨迹文件 OSS 地址（建议传 OSS HTTP URL，可带签名参数；轨迹地图索引支持 JSON/GeoJSON/KML/KMZ） |
| `track_screenshot_url` | string | 否 | 轨迹截图 OSS 地址（建议传 OSS HTTP URL，可带签名参数） |
| `track_no_map_bg_screenshot_url` | string | 否 | 无地图背景的轨迹路线截图 OSS 地址（建议传 OSS HTTP URL，可带签名参数） |
| `preview_images` | object | 否 | 四张轨迹预览图片 OSS 地址，见上方静态轨迹预览协议 |
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
- `RECOMMENDATION_ENABLED=false` 时完整沿用 Legacy 推荐和旧游标；开启后首次请求生成固定 Feed Session，并使用不透明的新游标稳定分页。
- `city_code` 非空时仅返回该城市的轨迹；不传、空字符串或仅包含空白字符时保持全城市推荐。
- `city_code` 使用精确匹配；没有匹配轨迹或传入未知 Code 时返回空列表，不返回参数错误。
- 首次请求不传 `cursor`；继续翻页时透传上一次返回的 `next_cursor`。
- `limit` 为可选参数，默认 `20`，最大 `50`；超出最大值时服务端会自动截断到 `50`。
- 响应中的 `has_more` 表示是否还有下一页；仅当 `has_more=true` 时才会返回 `next_cursor`。

### 请求

```
GET /api/v1/track/recommend/list?city_code=330100&limit=20&cursor=<next_cursor>
Authorization: Bearer <token>
```

**Query 参数：**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `city_code` | string | 否 | 城市 Code。非空时精确筛选对应城市；为空时返回全城市推荐。 |
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
        "navigate_count": 3,
        "recommend_rank": 1,
        "candidate_source": "content_affinity",
        "recommend_reason": "你常看的徒步路线"
      }
    ],
    "next_cursor": "eyJwcm90b2NvbCI6ImZlZWQtdjEiLCJyZXF1ZXN0X2lkIjoicmVjX3h4eCIsIm9mZnNldCI6MjB9",
    "has_more": true,
    "recommendation": {
      "request_id": "rec_xxx",
      "strategy": "hybrid"
    }
  }
}
```

**字段说明：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `data.items` | `TrackSummary[]` | 当前页轨迹列表。 |
| `data.next_cursor` | string | 下一页游标；当 `has_more=false` 时为空或不返回。 |
| `data.has_more` | bool | 是否还有下一页数据。 |
| `data.recommendation.request_id` | string | 本次 Feed Session 的归因 ID；新推荐成功响应（含空列表）必返。 |
| `data.recommendation.strategy` | string | `personalized` / `hybrid` / `legacy`。 |
| `data.items[].recommend_rank` | int | Session 中从 1 开始的原始排名；内容失效被跳过时允许跳号。 |
| `data.items[].candidate_source` | string | 主召回来源：`content_affinity` / `city_hot` / `followed_author` / `quality` / `exploration` / `legacy_fill` / `legacy`。 |
| `data.items[].recommend_reason` | string | 服务端生成的展示文案，客户端原样展示。 |

Feed Session 固定 60 分钟、最多 200 条。Session 内轨迹删除或转私密后，服务端会向后扫描补足当前页；不会返回 `items=[]` 且 `has_more=true`。客户端必须原样传递游标，不得解析或自行计算排名。

**游标错误：**

- Session 过期、不存在或与当前用户不匹配：`400`，`error_code=recommend_cursor_expired`；客户端保持筛选条件、去掉游标静默刷新第一页，最多自动重试一次。
- Session 仓储暂时不可用：`503`，`error_code=recommend_session_unavailable`；客户端保留当前页面和游标有限重试，不得刷新 Feed 或切换 Legacy。

开关关闭或新 Feed 首次建立时 Session 仓储不可用，服务端允许返回不含上述推荐字段的兼容 Legacy 响应。已有新 Session 的后续分页绝不切换 Legacy。

### 示例（curl）

```bash
curl -X GET "http://<host>:<port>/api/v1/track/recommend/list?city_code=330100&limit=20" \
  -H "Authorization: Bearer <token>" \
  -H "X-User-ID: 1001"
```

翻下一页：

```bash
curl -X GET "http://<host>:<port>/api/v1/track/recommend/list?city_code=330100&limit=20&cursor=<next_cursor>" \
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

详情直接返回 `collect_count`（当前收藏用户总数）与 `collected`（当前 JWT 用户是否收藏），与列表的收藏总数口径一致，无收藏返回 0。客户端复用轨迹信息展示数量与星星状态，不需要额外请求收藏计数。收藏/取消收藏响应也返回操作后的状态和总数，见 [collect-navigation.md](collect-navigation.md)。

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
    "collected": true,
    "collect_count": 12,
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

按关键字搜索轨迹列表，并可按城市、运动类型精确筛选。

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
- `city_code` 非空时仅返回该城市的轨迹；不传、空字符串或仅包含空白字符时保持全城市搜索。
- `city_code` 使用精确匹配；没有匹配轨迹或传入未知 Code 时返回空列表，不返回参数错误。
- `track_type` 非空时仅返回该运动类型的轨迹，使用 `/track/types` 返回的 `type`；不传、空字符串或仅包含空白字符时不过滤运动类型。
- `track_type` 使用存储值精确匹配，不对历史中文值做转换；无匹配结果时返回空列表。与 `city_code` 同时传入时，结果必须同时满足两个条件。
- 首次请求不传 `cursor`；继续翻页时透传上一次返回的 `next_cursor`。
- 翻页时应保持相同的 `keyword`、`city_code` 和 `track_type` 筛选条件。
- `limit` 为可选参数，默认 `20`，最大 `50`；超出最大值时服务端会自动截断到 `50`。
- 响应中的 `has_more` 表示是否还有下一页；仅当 `has_more=true` 时才会返回 `next_cursor`。

### 请求

```
GET /api/v1/track/search/list?keyword=:keyword&city_code=330100&track_type=hiking&limit=20&cursor=<next_cursor>
Authorization: Bearer <token>
```

**Query 参数：**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `keyword` | string | 否 | 搜索关键字；为空时返回最近轨迹列表。 |
| `city_code` | string | 否 | 城市 Code。非空时精确筛选对应城市；为空时返回全城市搜索结果。 |
| `track_type` | string | 否 | 运动类型，使用 `/track/types` 返回的 `type`；非空时精确筛选，空值不过滤。 |
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
curl -X GET "http://<host>:<port>/api/v1/track/search/list?keyword=西湖&city_code=330100&track_type=hiking&limit=20" \
  -H "Authorization: Bearer <token>" \
  -H "X-User-ID: 1001"
```

翻下一页：

```bash
curl -X GET "http://<host>:<port>/api/v1/track/search/list?keyword=西湖&city_code=330100&track_type=hiking&limit=20&cursor=<next_cursor>" \
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
- `raw_track_url` / `track_screenshot_url` / `track_no_map_bg_screenshot_url` 为服务端本地可下载链接（不是 OSS 地址）。
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
        "track_no_map_bg_screenshot_url": "/api/v1/static/screenshots/trk1_no_map_bg.jpg",
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
| `data.items[].track_no_map_bg_screenshot_url` | string | 服务端本地缓存的无地图背景轨迹截图可下载 URL。 |
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
  "calories_burned": 318,
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
| `preview_images` | object | 否 | 可整体替换的四图对象；省略保持原值，见静态轨迹预览协议 |
| `distance` | number | 否 | 距离（米），必须 `>= 0`（仅当原值为 `0` 时才会写入） |
| `duration` | int | 否 | 时长（秒），必须 `>= 0`（仅当原值为 `0` 时才会写入） |
| `calories_burned` | number | 否 | 热量消耗（千卡），必须 `>= 0`（仅当原值为 `0` 时才会写入）；仍为 `0` 时服务端会按运动类型、65kg 默认体重和时长兜底计算 |
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
  - `distance` / `calories_burned` / `elevation_gain` / `avg_speed_kmh` 为负数
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
- 同时返回 `name_en`：上述五种分别为 `Hiking`、`Running`、`Climbing`、`Cycling`、`Driving`。`name` 保留原有中文/配置展示名以兼容旧客户端；接口始终返回双语字段，不随语言请求头改写内容，客户端按当前 App 语言选择。类型 code、存储值和统计口径不受显示语言影响。
- 客户端可继续按账号缓存同一份双语元数据，切换语言时重新选择显示字段，无需清空缓存。旧缓存/旧服务端没有 `name_en` 时，内置类型使用客户端英文资源兜底；未知自定义类型 `name_en` 为空，沿用原配置名称，不猜测翻译。
- 服务端可通过环境变量 `TRACK_TYPES` 配置运动类型展示名，支持使用英文逗号、分号、中文逗号、顿号或竖线分隔，例如：`TRACK_TYPES=徒步,跑步,爬山,骑行,自驾,滑雪`。
- 服务端会自动过滤空项和重复项；若未配置或配置为空，则使用默认列表。
- 内置运动类型的 `icon_url` 返回 MingCute 原版线性 SVG，统一为透明背景、24×24 viewBox；徒步使用 `hiking`、跑步使用 `run`、爬山使用 `mountain_2`、骑行使用 `bike`、自驾使用 `car`。图形随服务端二进制内置，不依赖磁盘文件；文件名带版本，更新图形时更换 URL，避免客户端沿用旧图片缓存。客户端应直接使用下发地址，并按场景使用日夜主题色染色。
- 内置图标资源通过 `GET /api/v1/track/types/icons/:icon_file` 获取，支持 `HEAD`，需要相同的登录 Token。成功响应为 `image/svg+xml`，缓存策略为 `private, max-age=31536000, immutable`，不存在的文件返回 `404`。自定义运动类型仍沿用 `/api/v1/static/track_type_icon/<名称>.svg`。
- 每个运动类型会额外返回：`type`（英文标识）、`theme_color`（日间主题色）、`theme_color_dark`（夜间主题色）、`icon_anim_url`（Lottie 动画文件链接；当前默认空字符串，后续可通过配置补充）。
- 日夜颜色均由服务端配置下发，客户端按当前主题直接取对应字段，不再对有效色值自动调整饱和度或亮度。`theme_color` 继续作为日间色字段；旧服务端或本地缓存缺少 `theme_color_dark`、或该字段为空/非法时，夜间回退到 `theme_color`。未配置元信息的自定义类型两个颜色字段均为空字符串，由客户端使用本地样式兜底。
- 统计数据按 `Authorization` Token 解析出的当前用户统计 `track_records`：排除删除与进行中轨迹，仅统计 `正常/私密` 轨迹。
- 统计维度按运动类型拆分，并返回最近一个月（`month`）与最近一年（`year`）的总里程、轨迹次数、总耗时、总热量。
- 内置颜色在 `internal/config/config.go` 的 `DefaultTrackTypeConfigs` 中统一维护：

| 类型 | 日间 `theme_color` | 夜间 `theme_color_dark` |
|------|-------------------|------------------------|
| 徒步 `hiking` | `#00875A` | `#62D9A2` |
| 跑步 `running` | `#F26A4B` | `#FF947A` |
| 爬山 `climbing` | `#6C4CE1` | `#AA92F5` |
| 骑行 `riding` | `#2F80ED` | `#78B4FF` |
| 自驾 `driving` | `#F5A623` | `#FFD083` |

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
      "name_en": "Hiking",
      "theme_color": "#00875A",
      "theme_color_dark": "#62D9A2",
      "icon_url": "/api/v1/track/types/icons/mingcute-hiking-line-v1.svg",
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
      "name_en": "Running",
      "theme_color": "#F26A4B",
      "theme_color_dark": "#FF947A",
      "icon_url": "/api/v1/track/types/icons/mingcute-run-line-v1.svg",
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
| `data[].name` | string | 运动类型中文/配置名称，保留兼容。 |
| `data[].name_en` | string | 运动类型英文名称，英文 UI 优先使用；未知自定义类型为空字符串。 |
| `data[].theme_color` | string | 运动类型日间主题色，`#RRGGBB`；保持字段兼容，未配置时为空字符串。 |
| `data[].theme_color_dark` | string | 运动类型夜间主题色，`#RRGGBB`；缺失、为空或非法时夜间回退到 `theme_color`。 |
| `data[].icon_url` | string | 运动类型 SVG 链接；内置 MingCute 图标位于 `/api/v1/track/types/icons/`，自定义类型沿用原静态资源路径。客户端不得硬编码图标路径。 |
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

## 点赞与收藏

轨迹详情及列表摘要下发 `liked` 与 `like_count`，点赞和收藏互相独立。点赞、取消点赞和获赞统计见 [track-likes.md](track-likes.md)。

轨迹详情、创建/更新响应及列表摘要直接下发 `comment_count` 与 `can_comment`；前者为未删除主评论与回复总数，后者沿用点赞的轨迹可见范围。不可评论时返回 false/0。客户端首屏复用轨迹信息，点击入口后才加载评论列表，不额外调用评论状态接口。发布/删除响应带回更新后的总数。协议见 [track-comments.md](track-comments.md)。
