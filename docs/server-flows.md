# 服务端关键业务流程

从原 AGENTS.md 迁出的开发参考；修改对应功能前按 AGENTS 索引读取相关章节，无需每次通读。验证、授权和通用行为以当前 AGENTS.md 为准；实现细节以源码和最新接口文档为准。

代码与文档路径相对仓库根目录；协议变更同时维护 docs/api/ 对应分册和路由索引。

## 目录

- [流程索引](#流程索引)
- [存量标题回填](#存量标题回填)
- [启动流程](#启动流程)
- [推荐第一版流程](#推荐第一版流程)
- [埋点采集与同步流程](#埋点采集与同步流程)
- [成就结算流程](#成就结算流程)
- [轨迹投稿流程](#轨迹投稿流程)
- [首页地图索引流程](#首页地图索引流程)
- [短信登录等级信息](#短信登录等级信息)
- [关注/粉丝关系流程](#关注粉丝关系流程)
- [同行自动收尾流程](#同行自动收尾流程)
- [同行结束摘要更新](#同行结束摘要更新)
- [同行关键事件时间线](#同行关键事件时间线)
- [后台批量导入轨迹流程](#后台批量导入轨迹流程)
- [典型业务调用链](#典型业务调用链)
- [账号限制流程](#账号限制流程)
- [意见反馈流程](#意见反馈流程)
- [资源缓存与静态发布包](#资源缓存与静态发布包)
- [静态轨迹预览图](#静态轨迹预览图)
- [内置 H5 页面](#内置-h5-页面)
- [历史轨迹预览补齐](#历史轨迹预览补齐)
- [动态纠错配置](#动态纠错配置)

## 流程索引

### 存量标题回填
`cmd/backfill-track-titles` 是一次性 MySQL 工具，操作说明见 `docs/backfill-track-titles.md`，生成逻辑在 `internal/service/track_title_backfill.go`。默认只处理未删除、已结束轨迹的时间型默认标题（包括旧版 • 分隔符），优先复用投稿标题；用户明确要求移除日期前后缀时，以 -include-dated-titles 扩大计划范围，保留已有路线描述。缺少具体地点时按距离和运动类型生成，不再使用日期兜底；没有完整轨迹点时不得臆造环线或终点地址。默认只生成计划，应用时必须匹配计划数据库、作者和原始标题，只更新标题及更新时间，不修改投稿审核状态、资源、统计和索引。不得降级为内存仓储或在回填时自动建表。

### 手机号登录与绑定
短信登录、注册和后续第三方账号绑定必须保证非空手机号全局唯一。资料更新不写手机号；绑定使用本人 JWT、短信验证及仓储原子 `BindPhone`。已有 MySQL 先处理重复账号再安装唯一索引，不在启动失败降级流程中自动执行约束迁移；操作边界见 [手机号唯一性](phone-uniqueness.md)。

### 启动流程
（`cmd/server/main.go`）：
```text
Load Config → 选择 Repository（Memory/MySQL/Mongo，失败降级为 Memory）
→ 构造 Service（Track / TrackSubmission / Recommendation / User / Login / OSSToken / AssetCache）
→ 将 OSSTokenService 作为 downloader 注入 AssetCache
→ （可选）加载 TLS 证书
→ RegisterRoutes(Hertz, Deps)
→ （可选）SCHEDULER_ENABLED=true 时启动 Scheduler（注册 danmaku_cleanup、companion_session_autoclose、track_map_index、track_route_group、recommendation_* 等任务）
→ h.Spin()
```

### 推荐第一版流程

```text
GET /api/v1/track/recommend/list
  → RECOMMENDATION_ENABLED=false：原 TrackService Legacy 响应
  → 新 Feed：RecommendationService 读取画像/物料统计
  → 城市硬过滤 + 排除本人 → 内容偏好/城市热度/关注作者/质量/探索打分
  → 已收藏/已导航强降权 → RouteGroup/作者/运动类型多样性重排
  → 不足时 Legacy 补位 → 固化最多 200 条、TTL 60 分钟的 Feed Session
  → 后续游标只读取 Session；批量加载 Track，失效内容向后扫描并保留原始 recommend_rank
```

### 埋点采集与同步流程

```text
POST /api/v1/analytics/events（公开接口）
  → AnalyticsHandler 校验 JSON batch 与 body 大小
  → AnalyticsService 清理手机号/token/原始经纬度/OSS 签名 URL 等敏感字段
  → append 到 <LogDir>/analytics/events/<yyyy-MM-dd>/<HH>/events-*.jsonl.writing
  → 文件按大小或时间轮转为 .jsonl
  → Scheduler(analytics_sync，默认每天 03:00)
  → 按 event_date/hour 合并小 JSONL 为 128 MB 内的 part-*.jsonl
  → OSSTokenService 使用 STS 临时凭证上传到 OSS: analytics/ods/event_date=.../hour=.../part-*.jsonl
  → 上传成功后删除本地源 JSONL 和临时 part，并尽力清理空目录
  → AnalyticsRepository 写入 analytics_sync_summaries 同步摘要
```
埋点 OSS 同步任务默认 cron 为 `ANALYTICS_SYNC_CRON=0 3 * * *`；`ANALYTICS_ENABLED=false` 会关闭采集接口。缺少 OSS 配置时服务仍可本地落盘，但同步任务会失败重试，避免影响接口写入。同步上传强制使用 `OSS_INTERNAL_ENDPOINT` 内网域名，不允许回退公网 Endpoint。

### 成就结算流程

```text
track/create(is_running=false) 或 track upload/update 完成轨迹
  → TrackService 调用 AchievementService.SettleTrackCompleted
  → AchievementService 基于有效轨迹实时聚合 XP / 进度
  → AchievementRepository 幂等写入 user_achievement_rewards
  → track/create 响应通过 data.earned_rewards 返回本次新获得奖励
  → 客户端可再通过 /achievement/summary 或 /achievement/rewards 拉取全量展示
```
轨迹创建、轨迹补全和同行创建接口以 `/track/types` 返回的英文 `type` 为权威入库值；服务端兼容历史中文名输入，但写入前会归一为英文 code。成就侧按英文 code 计算，并兼容历史中文类型。`/achievement/summary` 与 `/achievement/rewards` 查询前会对该用户历史有效轨迹做幂等奖励补齐，用于兼容早期未结算数据。
运维可通过 `POST /api/v1/ops/achievement/refresh` 按手机号手动触发单用户成就幂等补齐；该接口不走业务 JWT，使用 `OPS_INTERNAL_TOKEN` 配置值对应的 `X-Internal-Token` 鉴权。
成就系统 MVP 只实现成长等级与勋章体系；里程碑体系暂不结算、不下发 `type=milestone` 奖励。调整成就定义时同步更新 `track_achievement.md`、`track_achievement_client.md` 和 `docs/api/achievement.md`。

### 轨迹投稿流程

```text
POST/PUT /api/v1/track/:track_id/submission（业务 JWT + 账号限制）
  → TrackSubmissionService 校验轨迹归属、完成状态和结构化路线资料
  → TrackSubmissionRepository 原子写入当前投稿、可选图片和事件流水
  → 管理后台 /admin/api/track-submissions/:submission_id/review 审核
  → approved 投稿在推荐列表当前分页窗口内优先，并成为 RouteGroup 代表轨迹候选
  → POST /api/v1/track/:track_id/submission/withdraw 转为 withdrawn 并取消资格
```
投稿状态为 `pending/approved/rejected/withdrawn/invalidated`；重投增加 `revision`，管理审核使用 `expected_revision` 防止审核旧版本。离线聚合仍先用 medoid 计算中心、半径、区域和相似度，再按“存量人工代表 > approved 投稿中与 medoid 最相似者 > medoid”选择展示代表；自动选择投稿代表不得修改路线介绍稳定锚点。

投稿和审核通过共用轨迹资格校验：本人已完成、正常公开、有原始文件；提交错误按录制状态、公开状态与原始文件区分。预览图及旧截图由轨迹上传流程维护，不属于投稿资格或审核通过条件。到达方式说明选填、trim 后最多 500 字符，交通方式选项仍至少一项。没有投稿图片时展示封面可读取新版日间横图、回退旧截图；封面字段缺失不拦截投稿，不修改存量轨迹资料。

### 首页地图索引流程

```text
track/create(is_running=false) 或 track upload/update 完成轨迹
  → TrackService 调用 TrackMapIndexService.EnqueueTrackIndexIfEligible
  → TrackMapRepository 写入 track_map_index_jobs(status=pending)
  → 入队成功后非阻塞 kick 后台立即 RunOnce（合并连续触发；不得在请求主链路同步下载 OSS raw track 或解析轨迹点）
  → Scheduler(track_map_index，默认 TRACK_MAP_INDEX_CRON=@every 1m) 作为失败重试与补偿兜底
  → 补偿扫描已完成但缺少 track_geo_indexes 的轨迹（限每 10 分钟一次；kick 与定时轮通过 TryLock 串行，已在处理时跳过本轮）
  → 小批量 claim pending job
  → 通过 rawTrackCache + OSS_INTERNAL_ENDPOINT 下载/复用 raw track 本地缓存
  → 解析 JSON/GeoJSON 轨迹点，计算 bbox/中心点/起终点/简化折线
  → Upsert track_geo_indexes，任务标记 succeeded；失败则延迟重试
  → Scheduler(track_route_group，默认 TRACK_ROUTE_GROUP_CRON=0 4 * * *)
  → 全量读取 eligible track_geo_indexes，按 track_type 严格隔离；中心距离召回后按距离比例、正反向起终点、离散 Fréchet 折线相似度构建候选关系图
  → 连通分量按 medoid 直接相似约束拆组，先用 medoid 计算 center、radius_m、bbox、member_count 与相似度
  → 再按人工代表、approved 投稿候选、medoid 的优先级选择 representative_track_id
  → 按新旧成员重合复用历史 group_id 与人工名称；自动名称按人工种子/标题共识、区域名、城市名依次兜底
  → 用内置区域目录离线匹配 medoid 中心并写入 area_id；未命中保持空字符串
  → 重建替换 track_route_groups / track_route_group_members（存量聚合结果可清空重算；MySQL 实现使用事务）
```
首页地图模式客户端接口挂在 auth 组：`GET /api/v1/track-map/view`、`GET /api/v1/track-map/groups`、`GET /api/v1/track-map/groups/:group_id/detail`、`GET /api/v1/track-map/groups/:group_id/tracks`。区域介绍页 `GET /api/v1/track-map/areas/:area_id/introduction.html` 和聚合路线介绍页 `GET /api/v1/track-map/groups/:group_id/introduction.html` 是公开 H5，不走业务 JWT。只有 `track_route_introductions.status=published` 时路线对象才返回 `introduction_url`；介绍以 `anchor_track_id` 为稳定锚点，路线组全量重建后仓储必须按新成员关系更新 `current_group_id`，禁止把运营介绍直接存进会被清空的 `track_route_groups`。`group_id` 来自 `track_route_groups.group_id`，不等同于单条 `track_id`；列表和地图聚合使用 RouteGroup 数量口径，不返回 `user_count` / `track_count`。调整字段、缩放分层、区域/路线介绍协议、聚合数量口径或 group_id 语义时，同步更新 `docs/api/track-map.md`、`docs/api/route-index.md` 与 `track_map.md`。
管理中心聚合路线列表和详情展示路线组摘要，以及已存 `area_id` 对应的区域名称、类型、城市与介绍页入口；admin 请求链路只按 ID 查询目录，不重新执行空间匹配。RouteGroup 对客户端表示聚合区域，使用 `center` + `radius_m` 绘制，不再下发代表路线折线。
路线组离线重建必须先通过 `ListAllRouteGroups` / `ListAllRouteGroupMembers` 读取历史身份，按成员重合复用 `group_id`；人工改名后的 `manual/mixed` 名称不得被自动任务覆盖。medoid 或展示代表变化都不能据此无条件更换已有 GroupID；仍属于新组的人工代表优先保留。

### 短信登录等级信息
`POST /api/v1/login/sms` 成功响应会附带 `achievement_level`，由 `LoginHandler` 调用 `AchievementService.GetLevelInfo` 基于当前有效轨迹实时计算；修改登录响应或等级字段时同步更新 `login.md`。

### 关注/粉丝关系流程
`POST /api/v1/user/:user_id/follow` 与 `DELETE /api/v1/user/:user_id/follow` 使用当前 JWT 用户作为关注者，写入或删除 `user_follows(follower_user_id, followee_user_id)` 单向关系；禁止关注自己，重复关注/取消关注按幂等成功处理。`GET /api/v1/user/:user_id/detail` 可查看任意用户公开主页，只有查看自己时返回手机号、客户端语言和当前生效的账号限制 `account_restriction`；详情、关注列表、粉丝列表会返回 `following_count`、`follower_count` 与当前登录用户视角的 `is_following`。用户详情还会返回公开成就摘要 `achievement`，包含当前等级、已获得勋章总数和最近 3 个勋章，查看自己和查看他人都返回。调整关注关系字段、计数口径、主页隐私字段、用户详情账号限制/成就字段或分页游标时，同步更新 `docs/api/user.md` 与 `docs/api/route-index.md`。

### 同行自动收尾流程

```text
Scheduler(companion_session_autoclose，每 10 分钟)
  → CompanionService.AutoCloseInactiveSessions
  → 按运动类型内置策略判断 active session 是否全员无活动 / 超过最大持续时间 / 无 joined 成员
  → 复用同行结束逻辑更新 session、members 及 end_reason / end_source / end_operator_user_id
  → 通过 companion control topic 广播 session_ended
```
同行自动收尾策略写在 `internal/service/companion_service.go` 代码常量中，不通过环境变量或启动参数配置；调整该策略时同步更新 `track_companion.md`。
同行控制面返回的 `session.expires_at`、附近房间列表的 `expires_at` 均由 `session.started_at + 运动类型最大持续时间` 派生，不落库；调整最大持续时间或返回字段时同步更新 `docs/api/companion.md` 与 `track_companion.md`。

### 同行结束摘要更新
owner 可在同行结束后调用 `PUT /api/v1/companion/session/:session_id/update` 补写 `locate_addr`、`total_distance`、`total_duration`、`track_screenshot_url`、`actual_participant_count`；该接口只允许 owner 操作 `status=ended` 的 session。`companion/session/history` 与 `companion/session/nearby` 都返回这些字段，其中截图响应应通过截图资源缓存改写为 `/api/v1/static/screenshots/...`。

### 同行关键事件时间线
owner 可通过 `POST /api/v1/companion/session/:session_id/events` 上报成员离开、断线/重连、同行周知、关键点、风险提醒或自定义事件，并通过 `GET /api/v1/companion/session/:session_id/events` 按 `event_time,id` 游标查询。事件持久化在 `companion_events`，`client_event_id` 是同一 session 内的幂等键；单个 `session_id` 最多 100 条事件，达到上限后新事件返回 `companion event limit exceeded`，但重复 `client_event_id` 仍返回已有事件。调整事件类型、字段、上限、幂等或时间校验规则时，同步更新 `docs/api/companion.md` 与 `track_companion.md`。

### 后台批量导入轨迹流程

```text
[admin UI] /admin/tracks.html「批量导入」
  → POST /admin/api/tracks/import（multipart：files≤50、owner_user_id 必须内部人员、track_type、coordinate_system 默认 WGS84）
  → service.AdminTrackImportService 逐文件 parseTrackPointsFile 解析（JSON/GeoJSON/KML/KMZ）
  → 计算距离/起止时间/耗时/爬升/均速；城市与 locate_addr 用 maparea 目录按中位点（转 GCJ02）离线解析
  → 非 KMZ 文件先转换为 gx:Track KMZ（客户端详情/地图只解析该格式），KMZ 原样保留
  → OSSTokenService.UploadUserObject 服务端直传原始文件到归属人用户目录，返回公开 URL
  → TrackService.CreateTrack（owner=归属人，source_tag=manual_seed，is_running=false）
  → 自动排队 track_map_index 并结算成就；逐文件返回 {file_name, track_id?, title?, error?}
```
导入不生成预览四图与路线描述（auto/v0 留空），由客户端 DevMode 后续补齐；OSS 未配置时不注入导入服务，接口返回 500。

### 典型业务调用链

```text
HTTP Request
  → middleware.RequestMeta → middleware.JWTAuth（auth 组）
  → middleware.AccountRestriction（仅拦截上传/发起同行/关注/收藏/修改资料等受限动作）
  → handler.<Xxx>Handler（参数校验 + 响应封装）
  → service.<Xxx>Service（业务编排 + 资源缓存下发）
  → repository.<Xxx>Repository（数据持久化）
```

### 账号限制流程
管理中心/运维通过 `/api/v1/ops/users/:user_id/restrictions*` 写入或解除 `user_account_restrictions`；当前生效条件为 `status=active AND (expires_at IS NULL OR expires_at > now)`。业务 JWT 通过后，`AccountRestrictionMiddleware` 只拦截上传内容（含上传用 OSS STS、轨迹创建/上传/更新）、发起同行、关注、收藏、点赞、修改个人信息；取消点赞、取消收藏、取消关注、退出登录和查询类接口放行。限制错误返回 `403`，`message` 使用“账号已被限制，禁止...”动作文案。新增会产生内容、关系或用户资料变更的业务接口时，需要评估是否加入 `internal/middleware/account_restriction.go` 的受限路由表，并同步更新 `docs/api/account-restriction.md` 与 `docs/api/route-index.md`。

### 意见反馈流程
`POST /api/v1/feedback` 使用 `multipart/form-data` 提交文字与最多 3 张图片；`FeedbackService` 会先检查同一用户未处理反馈数量，`pending` + `processing` 最多 5 条，超限返回 429 且不会写入图片。通过校验后再检查图片真实类型和大小，并私有落盘到 `<LogDir>/feedback/images/<user_id>/<yyyyMMdd>/`，数据库 `user_feedbacks.images_json` 只保存相对路径和元信息。用户历史列表/详情只返回本人的反馈，图片读取必须经业务 JWT 校验归属；`/ops/feedback/*` 使用 `X-Internal-Token` 供运营查看和更新状态，更新为 `resolved` 时 `reply` 必填且会下发给提交用户。调整反馈字段、图片限制、状态枚举、未处理上限、运营回复规则或访问路径时，同步更新 `docs/api/feedback.md` 与 `docs/api/route-index.md`。

### 资源缓存与静态发布包
客户端经 OSS 直传（头像 / 轨迹截图 / 无地图背景轨迹截图 / 同行轨迹截图 / 原始轨迹文件），列表/详情/同行历史/附近房间接口返回时由 `AssetCacheService` 按需从 OSS 拉回本地 `<LogDir>/static/<category>/`，对外走 `GET /api/v1/static/<category>/<file>`（需登录）；无地图背景轨迹截图使用 `<track_id>_no_map_bg` 作为缓存 key，`/track/my/list` 等轨迹列表返回 `track_no_map_bg_screenshot_url`。管理后台上传的 App 发布包直接写入 `<LogDir>/static/release/<platform>/`，对外走公开的 `GET /api/v1/static/release/<platform>/<file>`，供升级下载使用。

### 静态轨迹预览图
`Track.PreviewImages` / `preview_images` 保存客户端上传的 1:1、4:3 日夜四张图片，MySQL 使用可空 JSON 列，Mongo 和内存使用值对象；创建/更新字段可选，提供时须四图完整，更新整组替换。正常 Android 上传使用长宽各为窗口基准 1/4 的 WebP 85，现有 AssetCacheService 已允许 `.webp`，旧 JPEG 保持兼容；独立历史补图仍校验固定尺寸 JPEG，不能把该约束套到普通创建/更新。Track 及所有摘要统一返回带源版本的 `/api/v1/track/:track_id/previews/:variant?v=...`，列表只构造 URL，不同步拉取四图。入口见 `internal/service/track_preview.go` 与 `internal/handler/track_preview_handler.go`；图片复用 AssetCacheService 按需下载，使用轨迹归属用户的 OSS 凭证，缓存位于 `<LogDir>/track_previews/`，不挂通用 static；每次读取校验轨迹可见范围和源版本。协议与路由变更同步 `docs/api/track.md`、`models.md`、`route-index.md`。

### 内置 H5 页面
客户端等级规则页使用公开路由 `GET /api/v1/achievement/level-rules.html`，HTML 文件内置在 `internal/handler/static/achievement_level_rules.html` 并通过 `go:embed` 打包；页面支持 `lang=english` 切英文、`is_dark=true` 切夜间模式。修改等级 XP 规则、等级阈值、语言或主题参数时，同步更新该页面、`track_achievement_client.md` 和 `docs/api/achievement.md`。

### 历史轨迹预览补齐
DevMode 客户端经业务 JWT 调用 `/track/preview-backfill/list`、`/:track_id/data`、`/:track_id/prepare`、`/:track_id/complete`；按用户决定不增加管理员或作者权限校验，待补作用域为全站已结束、未删除、有原始轨迹且四图不完整的记录。入口为 `internal/service/track_preview_backfill*.go`，复用原始轨迹解析器返回完整 GPS，绘图仍由 Android 完成。prepare 仅签发对应作者目录、独立 upload_id 下四个对象的 PUT URL；complete 经 OSS 内网核对 JPEG 与固定尺寸，再调用 `TrackRepository.FillMissingPreviewImages` 条件写入四图，防止覆盖期间新上传的完整图片或不同源数据。`POST /track/preview-backfill/clear` 必须显式选择 `all=true` 或单个 `track_id`，清空未删除轨迹的四图地址，不删除 OSS 文件或 GPS；`ClearPreviewImages` 在既有 JSON/BSON 写入不对外暴露的清空版本，source_version 和条件写入均包含此版本，禁止旧任务重新写回。三种仓储实现位于 `internal/repository/track_preview_backfill.go`；无新表/列，完成状态以持久化四图为准。账号限制沿用上传规则，接口文档见 `docs/api/track.md`。

隐私政策与用户服务协议使用公开路由 `GET /api/v1/legal/privacy-policy.html` 和 `GET /api/v1/legal/terms-of-service.html`，由 `legal_handler.go` 嵌入 `static/privacy_policy.html`、`static/terms_of_service.html`。支持 `lang=english`、`is_dark=true/false`，未指定主题时跟随系统。草稿中的 `{xxxx}` 须按运营事实补齐；数据采集、公开、SDK、保存或删除行为变化时同步核对中英文正文及真实授权流程。接口与内容维护见 `docs/api/legal.md`。

地图景区/区县介绍页使用公开路由 `GET /api/v1/track-map/areas/:area_id/introduction.html`，统一模板位于 `internal/handler/static/map_area_introduction.html`；生成区县基线位于 `internal/maparea/districts.json`，人工景区/区县覆盖位于 `catalog.json`，页面支持 `lang=english` 与 `is_dark=true`。新增或调整区域时必须使用 GCJ-02 坐标、保持 `area.id` 稳定，并同步核对 `docs/api/track-map.md` 与 `track_map.md`。

- 评论图片每条最多一张，发布参数 image_ids 数组长度不能超过 1；客户端系统单选、输入区内预览和移除，服务端独立校验数量。

### 动态纠错配置
`internal/service/feedback_correction.go` 提供服务端默认选项、配置校验和提交快照；业务 `GET /feedback/correction-options` 仅下发启用项，后台 `/admin/api/feedback-correction-config` 全量读取/按 version 条件保存。`FeedbackRepository.GetCorrectionConfig/SaveCorrectionConfig` 在 `internal/repository/feedback_correction.go` 实现三种仓储，MySQL 启动补建 `feedback_correction_config` 与 `user_feedbacks.correction_json`。客户端提交稳定 option ID 和 track ID，名称与真实轨迹标题由服务端读取并保存快照；改名/停用不能改写历史反馈，停用项提交返回 409。仅配置不存在时使用服务端默认值，全停用不能恢复默认；客户端不得硬编码业务选项。协议见 `docs/api/feedback.md`，后台入口见 `internal/admin/AGENTS.md`。

## 景点图片后台同步

启动时在持久景点仓储与地点服务可用的情况下注册scenic_photos（每分钟），由ScenicSpotCatalogService.SyncPhotosOnce轮转补齐新景点及存量缺图，将供应商图片下载压缩到本站，再通过既有景点版本CAS更新照片。任务游标/逐景点租约/负缓存与退避使用现有TrackPlaceRepository；MySQL、Mongo和内存仓储接口不变，内存降级不启动同步。详情、发现列表和标签读取不调用供应商、下载图片或核验来源ID；图片鉴权沿用原静态资源入口。详细边界见[景点图片独立巡检](track-place-discovery.md#景点图片独立巡检)。

## 客态个人页喜欢列表

GET /user/:user_id/likes → 业务JWT → TrackHandler.ListProfileLikes → TrackService.ListProfileLikes。目标用户必须存在，使用目标ID读取点赞关系，批量取轨迹并过滤不可见项；按点赞时间与ID组成游标，所有互动状态按浏览者ID装饰。与客态已传/收藏复用认证、错误、摘要和资源处理，不开放私密轨迹、获赞明细或点赞人名单。接口及分页说明见[用户主页](api/user.md#喜欢的轨迹)。
