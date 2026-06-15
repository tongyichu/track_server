# AGENTS.md — track_server

> 本文面向 AI Coding Agent（Codex / Claude Code / Trae / Cursor）。
> 仅保留稳定约束、改动导航与提交前最小检查；短期数据、压测结论等不写入本文。

## 第一层（必读）

### 使用规则
- 从当前工作目录向上逐级查找 `AGENTS.md`，以最近的一份为准。
- 用 `AGENTS.md` 定位相关文件、约束、流程与验证步骤；实现细节以代码为准。
- 若 `AGENTS.md` 与代码不一致，以代码为准，并在回复中明确指出不一致点。

### 维护规则
仅当代码变更导致以下信息失准时，才更新最近的相关 `AGENTS.md`：
- 目录总览 / 模块职责
- 关键流程或调用路径
- 硬性约束、不变量、权威数据源
- 提交前验证命令

不要因"润色措辞、格式、统一风格"去修改 `AGENTS.md`。

### AGENTS.md 更新强制规则
凡是新增功能、模块、接口、数据表、后台任务、外部依赖、关键业务流程、客户端协议、服务端配置项时，都必须检查并更新最近的相关 `AGENTS.md`。

即使原有文字没有明显错误，只要新增内容会影响后续 Agent 理解代码入口、模块职责、调用流程、数据权威来源或提交前检查，也必须同步更新。

提交前必须自查：
- 是否新增/修改 HTTP 路由？
- 是否新增/修改 repository interface 或实现？
- 是否新增/修改数据库表结构？
- 是否新增/修改客户端协议文档？
- 是否新增/修改关键业务流程？
- 是否新增/修改配置项或环境变量？
- 是否新增需要特殊验证的测试命令？

若任一项为"是"，需要同步更新最近的 `AGENTS.md`；若判断不需要更新，应在最终回复中说明原因。

### 硬性约束
- **数据库依赖降级**：服务启动时若连接 MySQL/Mongo 失败，会自动降级为 in-memory 仓储（见 `cmd/server/main.go:46`）。禁止新增任何在降级分支里"必须调用外部服务"的逻辑。
- **轨迹 ID 编码不可变**：`"NO." + 8 位 base36` 是全链路外部 ID（见 `internal/repository/interfaces.go:14`）。不得修改 `trackIDPrefix` / `trackIDLength` / `trackIDBase`，否则旧 ID 将无法解析。
- **OSS 文件哈希桶数**：`config.OSSFileBucketSize = 2000` 参与用户轨迹文件的目录 hash，**不可修改**（见 `internal/config/config.go:17`）。
- **静态文件路由不能直接用 `Static`**：必须用 `StaticFS + PathRewrite`，否则静态资源会 404（原因见 `internal/handler/router.go:57-91`）。
- **鉴权分组**：所有业务接口都在 `api := h.Group("/api/v1")` 下的 `auth` 子组中；公开接口只有 `/ping`、`/captcha`、`/sms/send`、`/login/*`、`/upgrade/check`、`/achievement/level-rules.html`、安装包静态下载。`/internal/*` 与 `/ops/*` 是内部/运维接口，不走业务 JWT，必须使用内部 token 鉴权。新增业务接口默认加到 `auth` 组，除非有明确登录豁免需求。
- **Repository 降级链约束**：MySQL / Mongo / in-memory 三种实现都必须实现 `internal/repository/interfaces.go` 里的 interface 全集；新增接口方法时，三份实现都要补齐，否则启动会编译失败或运行时 panic。
- **反馈图片私有落盘**：意见反馈图片由服务端直接接收，保存到 `<LogDir>/feedback/images/`，必须通过 `/feedback/:feedback_id/images/:image_id` 或 `/ops/feedback/:feedback_id/images/:image_id` 走鉴权 handler 读取；不要放进 `/api/v1/static/*` 公开静态目录。
- **生成文件不手改**：`internal/config/china_city_raw.json`、`internal/config/china_province_raw.json` 为外部数据，不要逐行手工编辑。

### 目录总览
```
track_server/
├── cmd/server/             # main 入口：加载配置 → 选仓储 → 注入 service → 注册路由
├── internal/
│   ├── config/             # 环境变量加载；内置省市数据 + 昵称字典 + 同行弹幕敏感词词库
│   ├── handler/            # Hertz HTTP handler + router.go 路由表（权威）
│   ├── middleware/         # JWT 鉴权、请求元信息、Token 黑名单
│   ├── models/             # 领域模型（Track / User / UserFollow / Companion / CompanionEvent / Achievement / Feedback / Analytics / 相关光标/子结构）
│   ├── repository/         # 持久化接口 + mysql / mongo / memory 三实现
│   ├── scheduler/          # 进程内定时任务（基于 robfig/cron/v3，按 SCHEDULER_ENABLED 启停）
│   └── service/            # 业务编排：登录、轨迹、用户、同行控制面、成就、OSS STS、资源缓存、埋点落盘
├── deploy/                 # Dockerfile / docker-compose / nginx / systemd
├── Makefile                # run / test / docker-build / compose-up/down
├── go.mod / go.sum
├── mysql.sql               # MySQL 初始化 SQL（表结构权威之一）
├── track_api.md            # 业务接口文档轻量入口；完整分册见 docs/api/
├── docs/api/               # 业务接口分册文档：README + route-index + common/models + 各业务模块
├── track_map.md            # 首页地图模式与路线发现能力方案（路线组 / 附近与城市地图查询 / 客户端交互草案）
├── track_companion.md      # 同行能力技术方案设计（控制面 / MQTT 数据面规划）
├── track_achievement.md    # 轨迹成就产品/规则方案（等级、XP、勋章、会员边界）
├── track_achievement_client.md # 成就系统客户端对接文档
├── track_analytics.md      # 客户端埋点方案（事件命名 / 公共属性 / 业务事件 / 隐私验收）
└── login.md                # 登录流程与协议说明
```

### 权威数据源
| 主题 | 权威文件 |
| --- | --- |
| HTTP 路由清单 | `internal/handler/router.go` |
| 配置项与默认值 | `internal/config/config.go` |
| Repository 接口契约 | `internal/repository/interfaces.go` |
| 领域模型 | `internal/models/track.go`、`internal/models/user.go`、`internal/models/companion.go`、`internal/models/achievement.go`、`internal/models/feedback.go`、`internal/models/analytics.go` |
| MySQL 表结构 | `mysql.sql` |
| 接口协议 | `docs/api/`（入口 `track_api.md`，路由索引 `docs/api/route-index.md`，首页地图接口 `docs/api/track-map.md`）、`login.md`、`track_companion.md`、`track_achievement_client.md`、`track_map.md` |
| 客户端埋点方案 | `track_analytics.md`、`docs/api/analytics.md` |
| 成就规则方案 | `track_achievement.md` |

### 稳定默认值

- 默认运动类型由 `internal/config/config.go:DefaultTrackTypeConfigs` 维护，当前 code 为 `hiking`、`running`、`climbing`、`riding`、`driving`，展示名为 `徒步`、`跑步`、`爬山`、`骑行`、`自驾`；`GET /api/v1/track/types`、轨迹入库 `track_type`、运动类型图标元信息和成就系统类型口径应保持一致。
- 轨迹 `locate_addr` 最大长度为 255 字符，对应 `track_records.locate_addr VARCHAR(255)`；修改该字段长度时同步更新 `mysql.sql`、`internal/repository/mysql.go` 与 `docs/api/track.md`。
- 轨迹 `source_tag` 是来源/运营标签，对应 `track_records.source_tag VARCHAR(64)`；业务接口只允许空字符串或 `manual_seed`（人工录入冷启动轨迹），更新接口仅在原值为空时补写，普通列表摘要不返回该字段；修改该字段口径时同步更新 `internal/service/track_service.go`、`mysql.sql` 与 `docs/api/track.md`。
- 首页地图模式的轨迹空间索引由 `track_map_index` 后台任务异步构建：轨迹完成接口只写入 `track_map_index_jobs`，不得在请求主链路同步下载 OSS raw track 或解析轨迹点；后台下载 raw track 必须通过 `OSS_INTERNAL_ENDPOINT` 内网域名，未配置时失败重试且不回退公网。路线组由 `track_route_group` 离线任务基于 `track_geo_indexes` 聚合生成，默认每天 04:00 执行。
- 埋点采集接口 `POST /api/v1/analytics/events` 默认公开可访问，用于未登录启动/登录页等事件；服务端只做校验、脱敏和本地 JSONL 落盘，不把原始埋点写入业务 MySQL，凌晨同步 OSS 的任务由 `SCHEDULER_ENABLED` 控制。同步时按 `event_date/hour` 合并小 JSONL，单个 OSS part 目标上限 128 MB。`analytics_sync_summaries` 只记录每次 OSS 同步摘要（源文件列表、OSS part key、字节数、耗时、错误等），不保存原始事件。调整事件协议、认证策略、本地目录、OSS 前缀、批量上限、同步时间或同步摘要字段时，同步更新 `track_analytics.md`、`docs/api/analytics.md` 与 `mysql.sql`。

### 关键流程

**启动流程**（`cmd/server/main.go`）：
```
Load Config → 选择 Repository（Memory/MySQL/Mongo，失败降级为 Memory）
→ 构造 Service（Track / User / Login / OSSToken / AssetCache×3）
→ 将 OSSTokenService 作为 downloader 注入 AssetCache
→ （可选）加载 TLS 证书
→ RegisterRoutes(Hertz, Deps)
→ （可选）SCHEDULER_ENABLED=true 时启动 Scheduler（注册 danmaku_cleanup、companion_session_autoclose、track_map_index、track_route_group 等任务）
→ h.Spin()
```

**埋点采集与同步流程**：
```
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

**成就结算流程**：
```
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

**首页地图索引流程**：
```
track/create(is_running=false) 或 track upload/update 完成轨迹
  → TrackService 调用 TrackMapIndexService.EnqueueTrackIndexIfEligible
  → TrackMapRepository 写入 track_map_index_jobs(status=pending)
  → Scheduler(track_map_index，默认 TRACK_MAP_INDEX_CRON=@every 1m)
  → 补偿扫描已完成但缺少 track_geo_indexes 的轨迹
  → 小批量 claim pending job
  → 通过 rawTrackCache + OSS_INTERNAL_ENDPOINT 下载/复用 raw track 本地缓存
  → 解析 JSON/GeoJSON 轨迹点，计算 bbox/中心点/起终点/简化折线
  → Upsert track_geo_indexes，任务标记 succeeded；失败则延迟重试
  → Scheduler(track_route_group，默认 TRACK_ROUTE_GROUP_CRON=0 4 * * *)
  → 扫描尚未归组的 track_geo_indexes，按 track_type 严格分组，正反向相似路线可合并
  → Upsert track_route_groups / track_route_group_members
```
首页地图模式客户端接口挂在 auth 组：`GET /api/v1/track-map/view`、`GET /api/v1/track-map/groups`、`GET /api/v1/track-map/groups/:group_id/detail`、`GET /api/v1/track-map/groups/:group_id/tracks`。`group_id` 来自 `track_route_groups.group_id`，不等同于单条 `track_id`；列表和地图聚合使用 RouteGroup 数量口径，不返回 `user_count` / `track_count`。调整字段、缩放分层、聚合数量口径或 group_id 语义时，同步更新 `docs/api/track-map.md`、`docs/api/route-index.md` 与 `track_map.md`。

**短信登录等级信息**：`POST /api/v1/login/sms` 成功响应会附带 `achievement_level`，由 `LoginHandler` 调用 `AchievementService.GetLevelInfo` 基于当前有效轨迹实时计算；修改登录响应或等级字段时同步更新 `login.md`。

**关注/粉丝关系流程**：`POST /api/v1/user/:user_id/follow` 与 `DELETE /api/v1/user/:user_id/follow` 使用当前 JWT 用户作为关注者，写入或删除 `user_follows(follower_user_id, followee_user_id)` 单向关系；禁止关注自己，重复关注/取消关注按幂等成功处理。`GET /api/v1/user/:user_id/detail` 可查看任意用户公开主页，只有查看自己时返回手机号和客户端语言；详情、关注列表、粉丝列表会返回 `following_count`、`follower_count` 与当前登录用户视角的 `is_following`。用户详情还会返回公开成就摘要 `achievement`，包含当前等级、已获得勋章总数和最近 3 个勋章，查看自己和查看他人都返回。调整关注关系字段、计数口径、主页隐私字段、用户详情成就字段或分页游标时，同步更新 `docs/api/user.md` 与 `docs/api/route-index.md`。

**同行自动收尾流程**：
```
Scheduler(companion_session_autoclose，每 10 分钟)
  → CompanionService.AutoCloseInactiveSessions
  → 按运动类型内置策略判断 active session 是否全员无活动 / 超过最大持续时间 / 无 joined 成员
  → 复用同行结束逻辑更新 session、members 及 end_reason / end_source / end_operator_user_id
  → 通过 companion control topic 广播 session_ended
```
同行自动收尾策略写在 `internal/service/companion_service.go` 代码常量中，不通过环境变量或启动参数配置；调整该策略时同步更新 `track_companion.md`。
同行控制面返回的 `session.expires_at`、附近房间列表的 `expires_at` 均由 `session.started_at + 运动类型最大持续时间` 派生，不落库；调整最大持续时间或返回字段时同步更新 `docs/api/companion.md` 与 `track_companion.md`。

**同行结束摘要更新**：owner 可在同行结束后调用 `PUT /api/v1/companion/session/:session_id/update` 补写 `locate_addr`、`total_distance`、`total_duration`、`track_screenshot_url`、`actual_participant_count`；该接口只允许 owner 操作 `status=ended` 的 session。`companion/session/history` 与 `companion/session/nearby` 都返回这些字段，其中截图响应应通过截图资源缓存改写为 `/api/v1/static/screenshots/...`。

**同行关键事件时间线**：owner 可通过 `POST /api/v1/companion/session/:session_id/events` 上报成员离开、断线/重连、同行周知、关键点、风险提醒或自定义事件，并通过 `GET /api/v1/companion/session/:session_id/events` 按 `event_time,id` 游标查询。事件持久化在 `companion_events`，`client_event_id` 是同一 session 内的幂等键；单个 `session_id` 最多 100 条事件，达到上限后新事件返回 `companion event limit exceeded`，但重复 `client_event_id` 仍返回已有事件。调整事件类型、字段、上限、幂等或时间校验规则时，同步更新 `docs/api/companion.md` 与 `track_companion.md`。

**典型业务调用链**：
```
HTTP Request
  → middleware.RequestMeta → middleware.JWTAuth（auth 组）
  → handler.<Xxx>Handler（参数校验 + 响应封装）
  → service.<Xxx>Service（业务编排 + 资源缓存下发）
  → repository.<Xxx>Repository（数据持久化）
```

**意见反馈流程**：`POST /api/v1/feedback` 使用 `multipart/form-data` 提交文字与最多 3 张图片；`FeedbackService` 会先检查同一用户未处理反馈数量，`pending` + `processing` 最多 5 条，超限返回 429 且不会写入图片。通过校验后再检查图片真实类型和大小，并私有落盘到 `<LogDir>/feedback/images/<user_id>/<yyyyMMdd>/`，数据库 `user_feedbacks.images_json` 只保存相对路径和元信息。用户历史列表/详情只返回本人的反馈，图片读取必须经业务 JWT 校验归属；`/ops/feedback/*` 使用 `X-Internal-Token` 供运营查看和更新状态，更新为 `resolved` 时 `reply` 必填且会下发给提交用户。调整反馈字段、图片限制、状态枚举、未处理上限、运营回复规则或访问路径时，同步更新 `docs/api/feedback.md` 与 `docs/api/route-index.md`。

**资源缓存与静态发布包**：客户端经 OSS 直传（头像 / 轨迹截图 / 同行轨迹截图 / 原始轨迹文件），列表/详情/同行历史/附近房间接口返回时由 `AssetCacheService` 按需从 OSS 拉回本地 `<LogDir>/static/<category>/`，对外走 `GET /api/v1/static/<category>/<file>`（需登录）。管理后台上传的 App 发布包直接写入 `<LogDir>/static/release/<platform>/`，对外走公开的 `GET /api/v1/static/release/<platform>/<file>`，供升级下载使用。

**内置 H5 页面**：客户端等级规则页使用公开路由 `GET /api/v1/achievement/level-rules.html`，HTML 文件内置在 `internal/handler/static/achievement_level_rules.html` 并通过 `go:embed` 打包；页面支持 `lang=english` 切英文、`is_dark=true` 切夜间模式。修改等级 XP 规则、等级阈值、语言或主题参数时，同步更新该页面、`track_achievement_client.md` 和 `docs/api/achievement.md`。

### 提交前最小检查
- 构建可通过：`go build ./...`
- 全量测试：`make test`（等价 `go test ./...`）
- 新增/修改路由：核对 `internal/handler/router.go` 是否挂载在正确的 `auth` 分组。
- 新增 Repository 方法：`mysql.go` / `mongo.go` / `memory.go` 三实现必须同步。
- 改动与协议相关（字段增删、登录流程、错误码、同行控制面、同行关键事件、成就系统、意见反馈、埋点采集）：同步更新 `docs/api/` 对应分册与 `docs/api/route-index.md`、`login.md`、`track_companion.md`、`track_achievement_client.md` 或 `track_analytics.md`。
- 用户要求提交代码时，commit message 必须使用中文，并尽量详细说明：做了什么、为什么做、影响哪些模块、是否包含数据结构/协议/文档/测试变更。

---

## 第二层（附录）

### 不要轻易改动的文件
| 文件 | 原因 |
| --- | --- |
| `internal/config/china_city_raw.json` / `china_province_raw.json` | 外部数据源，批量导入 |
| `internal/config/nickname.go` | 昵称词库，顺序与索引参与随机分配 |
| `internal/config/sensitive_words.json` | 同行弹幕敏感词词库，由 `go:embed` 加载，运维维护，不要逐行手改 |
| `mysql.sql` | 线上表结构基线，仅在正式变更表结构时同步修改 |

### 工程惯例
- **包命名**：所有业务代码位于 `internal/`，外部仓库不可 import。
- **错误语义**：`repository` 层只返回 `ErrNotFound` / `ErrAlreadyExists` / `ErrForbidden`（见 `interfaces.go:28`）；上层 service / handler 根据这三种语义翻译为 HTTP 状态码。
- **Context 约定**：全链路传递 `context.Context`；Handler 从 Hertz `RequestContext` 取值后传下去，不要在 service 层重新从 `app.RequestContext` 取上下文。
- **日志**：统一使用标准库 `log` + Hertz `hlog`，并通过 `cmd/server/main.go:setupLogging` 把输出同时落到 `LogDir/server.log`。
- **配置读取**：只在 `internal/config/config.go` 里读 `os.Getenv`；其他包通过 `*config.Config` 注入，不自己读环境变量。

### 常见误解
- **误解**：MySQL 启用后就不用 in-memory 了。
  **事实**：启动时若 MySQL `Ping` 失败会降级为 in-memory，所以线上必须通过日志确认实际仓储类型（搜 `using mysql repositories` / `using in-memory repositories`）。
- **误解**：可以用 `auth.Static("/static", root)` 挂载静态资源。
  **事实**：Hertz 的 `Static` 会把完整 URL 拼到 Root 下，造成 404；必须用 `StaticFS + PathRewrite`（见 `router.go:57`）。
- **误解**：STS 不配置就无法启动。
  **事实**：OSS STS 缺配置时会降级为"禁用 STS"，对应接口返回 `"oss sts not configured"`，服务仍可启动。

### 术语
| 术语 | 含义 |
| --- | --- |
| Track | 一条用户轨迹记录，外部 ID 形如 `NO.00000001` |
| Waypoint | 轨迹上的多媒体打点 |
| Collect | 用户对他人轨迹的收藏 |
| Follow | 用户对另一个用户的单向关注关系，存储在 `user_follows` |
| Navigation | "使用他人轨迹导航"的使用次数统计（与 Collect 独立） |
| AssetCache | 把 OSS 私有对象按需缓存到服务器本地并通过 `/api/v1/static/*` 下发的服务 |
| STS | 阿里云临时凭证，用于客户端直传 OSS |
