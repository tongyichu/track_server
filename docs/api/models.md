# API 通用模型

> 公共请求、认证和错误响应见 [common.md](common.md)。

运动类型对象 `TrackTypeOption` 同时提供中文/配置名 `name` 和英文名 `name_en`，客户端按当前语言选择显示；`type` 为不随语言变化的业务标识。旧客户端仍可读取原 `name`，完整字段与兜底规则见 [运动类型协议](track.md#16-获取运动类型)。

## 轨迹对象（Track）字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | 轨迹 ID |
| `user_id` | int64 | 用户 ID |
| `liked` | bool | 当前浏览者是否点赞（非持久化轨迹字段） |
| `like_count` | int64 | 轨迹点赞数；详情与摘要均返回 |
| `collect_count` | int64 | 当前收藏该轨迹的用户总数，包含作者自藏；详情、创建/更新响应及列表摘要均返回，未收藏为 0 |
| `collected` | bool | 当前 JWT 浏览者是否已收藏；详情与所有轨迹列表摘要均明确返回 true/false，包括本人的已上传与收藏列表，false 不省略；轨迹归属自己不代表已收藏 |
| `comment_count` | int64 | 未删除主评论与回复总数；详情与摘要均返回 |
| `can_comment` | bool | 是否开放评论；不可评论时数量返回 0 |
| `session_id` | string | 关联的同行会话 ID；非同行轨迹为空字符串 |
| `city_code` | string | 城市 Code（用于标识轨迹所属城市；城市/省份映射关系维护在配置文件中） |
| `locate_addr` | string | 轨迹的具体位置信息 |
| `track_type` | string | 轨迹类型 code，例如 `hiking` / `running` / `climbing` / `riding` / `driving` |
| `source_tag` | string | 轨迹来源/运营标签；允许值为空字符串或 `manual_seed`（人工录入冷启动轨迹）；普通列表摘要不返回该字段 |
| `coordinate_system` | string | 坐标系，例如 `WGS84` / `GCJ02` / `BD09` |
| `title` | string | 轨迹标题 |
| `is_featured` | boolean | 推荐、搜索、我的轨迹及收藏列表摘要中的精品标识；仅投稿当前已审核通过时为 true，false 也明确返回 |
| `featured_description` | string | 审核通过的投稿简介；非优质投稿省略 |
| `featured_cover_url` | string | 投稿首图的本地静态缓存地址；无投稿图时回退轨迹截图 |
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
| `preview_images` | object | 静态轨迹预览图：`square_day_url` / `square_night_url`（1:1）、`landscape_day_url` / `landscape_night_url`（4:3）；详情及所有轨迹摘要返回鉴权图片地址，无图或 DevMode 清空后为空对象；存储内部清空版本不下发；完整协议见 [track.md](track.md#静态轨迹预览图) |
| `primary_scenic_spot` | object | 可选主景区；发布后的目录记录使用内部 `{id,name,category,description,photos,sources}`，详情可实时附加 `address` 和供应商图片。列表只读目录数据，不调用地图供应商。未完成迁移/发布时兼容旧 `{id,name,provider,provider_id}`。只读，完整协议见 [track.md](track.md#主景区标签) |
| `is_running` | bool | 是否进行中 |
| `status` | int | 轨迹状态：`0` 删除，`1` 正常，`2` 私密 |
| `created_at` | string | 创建时间 |
| `updated_at` | string | 更新时间 |
| `deleted_at` | string | 删除时间（软删除；未删除时为空或不返回） |
| `earned_rewards` | array | 本次轨迹完成即时新获得的成就奖励；仅完成轨迹结算产生新奖励时返回，结构同 `/achievement/rewards` 单条奖励 |

---

### 用户背景图、性别、生日与星座

`User` 包含 `background_url`（字符串，空串表示默认主页背景）、`gender`（字符串，空串未设置、`male`、`female`）和 `birthday`（`YYYY-MM-DD` 字符串，空串表示未设置）。用户详情的 `background_url` 为公开主页展示字段，`gender`、`birthday` 仅在本人详情返回；本人登录与资料更新响应也下发生日。更新个人信息的请求/响应与兼容规则见 [user.md](user.md#14-更新个人信息)。

本人详情、登录 `user` 和资料更新响应额外包含只读 `zodiac_sign`（英文 code；无有效生日时为空串），由服务端依据生日月日实时派生，不属于数据库 `User` 字段。客户端只本地化名称、不独立计算，客态详情省略该字段；固定区间见 [星座派生规则](user.md#星座派生规则)。

点赞状态、轨迹点赞数和用户 `received_like_count` 的统计规则见 [track-likes.md](track-likes.md)。

## 评论

轨迹评论、回复、作者回复摘要、引用、图片及当前用户赞踩字段见 [track-comments.md](track-comments.md)。Track、TrackSummary、MyTrackSummary 和 CollectedTrackSummary 直接下发 comment_count/can_comment；不存在独立的评论状态接口。
