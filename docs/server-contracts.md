# 服务端业务契约与数据边界

从原 AGENTS.md 迁出的开发参考；修改对应功能前按 AGENTS 索引读取相关章节，无需每次通读。验证、授权和通用行为以当前 AGENTS.md 为准；实现细节以源码和最新接口文档为准。

代码与文档路径相对仓库根目录。鉴权公开路由的实际清单以 internal/handler/router.go 为准，业务接口默认需要 JWT。

## 目录

- [基础不变量](#基础不变量)
- [权威数据源](#权威数据源)
- [业务契约](#业务契约)
- [第二层（附录）](#第二层附录)
- [不要轻易改动的文件](#不要轻易改动的文件)
- [工程惯例](#工程惯例)
- [常见误解](#常见误解)
- [术语](#术语)
- [设备权限快照](#设备权限快照)

## 基础不变量
- **数据库依赖降级**：服务启动时若连接 MySQL/Mongo 失败，会自动降级为 in-memory 仓储（见 `cmd/server/main.go:46`）。禁止新增任何在降级分支里"必须调用外部服务"的逻辑。
- **轨迹 ID 编码不可变**：`"NO." + 8 位 base36` 是全链路外部 ID（见 `internal/repository/interfaces.go:14`）。不得修改 `trackIDPrefix` / `trackIDLength` / `trackIDBase`，否则旧 ID 将无法解析。
- **OSS 文件哈希桶数**：`config.OSSFileBucketSize = 2000` 参与用户轨迹文件的目录 hash，**不可修改**（见 `internal/config/config.go:17`）。
- **静态文件路由不能直接用 `Static`**：必须用 `StaticFS + PathRewrite`，否则静态资源会 404（原因见 `internal/handler/router.go:57-91`）。
- **鉴权分组**：所有业务接口都在 `api := h.Group("/api/v1")` 下的 `auth` 子组中；公开接口只有 `/ping`、`/captcha`、`/sms/send`、`/login/*`、`/upgrade/check`、`/achievement/level-rules.html`、`/legal/privacy-policy.html`、`/legal/terms-of-service.html`、`/track-map/areas/:area_id/introduction.html`、安装包静态下载。`/internal/*` 与 `/ops/*` 是内部/运维接口，不走业务 JWT，必须使用内部 token 鉴权。新增业务接口默认加到 `auth` 组，除非有明确登录豁免需求。
- **Repository 降级链约束**：MySQL / Mongo / in-memory 三种实现都必须实现 `internal/repository/interfaces.go` 里的 interface 全集；新增接口方法时，三份实现都要补齐，否则启动会编译失败或运行时 panic。
- **搜索结果总数**：`GET /track/search/list` 返回 `total_count`，由三种 TrackRepository 的 `CountSearch` 按关键词、城市、运动类型统计正常且有原始轨迹文件的记录，不受分页游标影响；客户端据此显示搜索总数。
- **发现与活动日历**：`internal/repository/track_discovery.go` 的三种 TrackRepository 实现负责象形轨迹筛选和用户轨迹开始时间查询；`internal/service/track_discovery.go` 提供景点、景点关联轨迹、象形轨迹与近 375 天日历（365 天加 10 天缓冲）。发现景点沿用轨迹详情的客户端可见规则，只展示 active 目录，封面优先首张图片，并返回景点描述及完整目录图片 `photos`（url/title/author/author_url/source_url）供详情页轮播与相册展示；图片沿用目录顺序，过滤空 URL，不为此新增供应商请求；按该景点关联的公开、已完成且有原始文件的轨迹数降序、同数按景点 ID 升序分页，游标携带计数和 ID。两个轨迹列表只含公开已完成且有原始文件的记录。日历按请求的 IANA 时区归日，客户端按连续 12 个月的首月第一个周日裁剪并另加本地未上传记录。协议见 `docs/api/track.md`。
- **反馈图片私有落盘**：意见反馈图片由服务端直接接收，保存到 `<LogDir>/feedback/images/`，必须通过 `/feedback/:feedback_id/images/:image_id` 或 `/ops/feedback/:feedback_id/images/:image_id` 走鉴权 handler 读取；不要放进 `/api/v1/static/*` 公开静态目录。
- **地区目录唯一来源**：`internal/regions/seed.json` 只在存储没有目录时首次导入，此后省市及海外地区均以 `region_catalog_state` 为准；后台在 `/admin/regions.html` 编辑，Android 首次下载并保存本地文件，后续按版本更新。国内现有编码、层级与父级保持稳定。协议见 `docs/api/regions.md`。

## 权威数据源
后台专用形象识别入口 `internal/service/kimi_track_shape.go`，复用 Kimi 图像传输但关闭思考模式，与自动说明生成方法分开；共享契约在 skill 的 `references/shape-only-output.txt`。最近诊断以 `narrative:shape-preview:<ID>` 存于现有记录表，按输入版本展示，POST 不写轨迹标签。接口与验收边界见 `docs/api/admin-tracks.md`。

人工形象反馈入口 `internal/service/track_shape_learning.go`：后台保存实际形象变化后记录最新反馈，索引巡检每轮最多复核一条，共享 narrative 日预算与扫描租约。复用 `track_place_records`，最多引用4条有效案例，管理员清空同样保护；不改模型权重或生产源码。规则及边界见形象 skill 与 `docs/track-narrative.md`。

形象标签识别与提示词维护使用仓库技能 [track-shape-label](../skills/track-shape-label/SKILL.md)。其 `references/shape-rules.txt` 为共享正文，由同目录 `rules.go` 嵌入服务端；调整识别语义时维护 `trackNarrativeRuleVersion`。按一笔画/简笔画整体及关键部位判断，候选包含动物、花草、建筑、数字、地图轮廓和人物头像；不凭头像猜身份，不从地图形象推断实际覆盖区域。

| 主题 | 权威文件 |
| --- | --- |
| HTTP 路由清单 | `internal/handler/router.go` |
| 配置项与默认值 | `internal/config/config.go` |
| Repository 接口契约 | `internal/repository/interfaces.go` |
| 领域模型 | `internal/models/track.go`、`internal/models/recommendation.go`、`internal/models/track_submission.go`、`internal/models/scenic_spot.go`、`internal/models/user.go`（含 AccountRestriction）、`internal/models/companion.go`、`internal/models/achievement.go`、`internal/models/feedback.go`、`internal/models/analytics.go` |
| 地图区域语义与介绍内容 | `internal/service/track_place*.go`（异步景区识别）+ `internal/maparea/districts.json`（区县基线）+ `catalog.json`（区县覆盖/旧介绍兼容）；`mainland.json` 仅用于大陆服务路由判断；坐标转换使用 `maparea.UsesGCJ02`，独立补齐香港/澳门及岸边缓冲；配置及维护见 `docs/track-place-discovery.md` |
| MySQL 表结构 | `mysql.sql` |
| 接口协议 | `docs/api/`（入口 `track_api.md`，路由索引 `docs/api/route-index.md`，账号限制接口 `docs/api/account-restriction.md`，首页地图接口 `docs/api/track-map.md`）、`login.md`、`track_companion.md`、`track_achievement_client.md`、`track_map.md`；推荐第一版设计与客户端归因协议见 `track_recommendation.md`、`track_analytics.md` |
| 客户端埋点方案 | `track_analytics.md`、`docs/api/analytics.md` |
| 成就规则方案 | `track_achievement.md` |
| 轨迹投稿功能与协议 | `track_submission.md`、`docs/api/track-submission.md`；路由权威仍为 `internal/handler/router.go` |

## 业务契约

- 标签日夜强调色通过 theme_color/theme_color_dark 下发；sport 直接继承现有运动类型配置，featured/shape/scenic_spot 配色集中在 `internal/config/track_label_colors.go`，未知运动类型不伪造颜色。统一轨迹标签 `labels` 是只读响应投影，按 featured/sport/scenic_spot/shape 组装，value 为布尔值，code 保存具体内容；精品来自已通过投稿，类型为 `models.TrackLabel`，入口 `internal/service/track_labels.go`；来自已有 shape_label、主景区、track_type，不再建标签表，不触发额外供应商/AI 请求。详情及各种列表统一填充，缺项省略、空列表返回 []；旧字段保留兼容。

- 轨迹具象形象使用独立 `Track.ShapeLabel` / `shape_label`，MySQL `track_records` 启动补列、Mongo 和内存读写一致；创建/更新沿用所有者权限，空串清空，缺省保留。详情与推荐/搜索/我的/收藏/喜欢摘要传递，32字符及控制字符校验在 `internal/service/track_shape.go`；协议见 `docs/api/track.md`。自动生成入口为 `track_narrative_service.go` / `kimi_track_narrative.go`，定时任务和费用边界见 `docs/track-narrative.md`；生成器必须查看一张实际 1:1 预览图并参考用户 title：标题中的形象仅为候选，必须得到轨迹线的整体轮廓和关键特征支持才赋值；标题没有形象时仍需独立识图；图像不支持标题时不沿用标题，可赋予图像明确支持的另一形象。不明确、图片缺失或遮挡严重时留空，不能把地图背景、图标或文字当成轨迹形象。形象标签仅接受积极或中性、适合公开展示的名称；即使相似度高，低俗色情、侮辱辱骂、歧视仇恨、恶意挑衅或其他负面冒犯性形象也不得赋值，含义不明确时留空，不用正面别名包装不当形象。

- 后台本人邮箱验证与密码找回由 `internal/admin/email_*.go` 和 `Module.ConfigureEmailRecovery` 实现，使用现有账号 JSON CAS 持久化验证码摘要/次数/冷却，只允许加密 SMTP；`ADMIN_SMTP_*` 配置和迁移边界见 `docs/admin-email-recovery.md`、`internal/admin/AGENTS.md`。已验证邮箱的账号禁止管理员代重置，全部启用账号完成绑定后永久关闭旧重置入口；无邮件配置时不伪造发信成功，不能复用 OSS 凭证或以内存账号兜底。
- 后台轨迹列表缺项筛选由 `internal/repository/admin_track_missing.go` 实现，`missing=any|place_candidate|scenic_spot|description|city|previews` 参与总数与游标分页，MySQL/Mongo/内存保持一致；main 为内存轨迹仓储注入景点仓储关联快照。候选源摘要校验复用 `TrackPlaceService`，只读筛选不触发供应商调用。景点识别候选移至共享轨迹详情弹窗，读取与确认/忽略均按轨迹模块权限；后台人工候选放宽为围住+采样途经附近（unmatched 也可人工确认），`POST /admin/api/tracks/:track_id/place-refresh` 同步触发单条重新识别，轨迹详情附带主景点与简化轨迹线供弹窗地图绘制；API 与口径见 `docs/api/admin-tracks.md` 和 `internal/admin/AGENTS.md`。

- 后台账号独立持久化在 `admin_account_state`，操作日志在 `admin_audit_logs`，由 `internal/admin/account_storage.go` 实现 MySQL/Mongo 并在 main 注入；不属于 App 用户 Repository，也不随业务仓储降级为内存。环境变量仅首次导入：`admin` 为管理员，其余现有账号默认业务页面只读。角色、页面 none/read/write 权限、跨模块校验、版本冲突、最后管理员保护和会话失效由 `internal/admin/accounts.go`、`permissions.go`、`auth.go` 维护。后台入口 `/admin/accounts.html`；细节与新增接口见 `internal/admin/AGENTS.md` 和 `docs/api/admin-accounts.md`。

- 升级检查 `GET /api/v1/upgrade/check` 响应在保留旧 `release_notes`（最新版本说明）的同时，可选下发 `changelogs` 数组：`AppReleaseService.CheckUpgrade` 汇总该平台所有 `version_code` 大于客户端当前版本的 `published` 版本说明，按版本号倒序、跳过空说明；聚合查询失败时降级为只返回最新版本说明，纯增量协议、老客户端无感知。运维发布接口 `POST /api/v1/ops/releases/android` 支持可选 `operator_name`，服务端保存为 `<operator_name> from api`（缺省 `android-track-release`），后台版本列表发布人列直接展示。版本更新日志独立留存于 `app_release_changelogs` 表：`AppReleaseService.Publish` 在发布成功后按 (platform, version_code) 幂等 upsert（失败仅记日志），删除发布记录会尽力删除本机安装包文件但不动 changelog；后台经 `GET /admin/api/releases/changelogs` 读取并在发布管理页「更新日志」区完整展示。MySQL/内存实现完整，Mongo 为未实现 stub（与 MongoAppReleaseRepository 一致）。协议见 `docs/api/upgrade.md`。

- 客户端活跃统计由 `repository.AppClientStatsRepository` 提供：`app_client_devices` 以 device_id 幂等 upsert 设备最近活跃信息（user_id 仅登录后补绑，其余字段只在新值非空/非零时覆盖），同一本地自然日首次出现对 `app_client_active_daily(日期,平台,版本)` 计数 +1。采集入口为公开的升级检查和三种登录成功路径（`AppReleaseService.RecordClientActivity`），失败仅记日志不影响主流程；客户端经公共请求头 `X-Device-Model` / `X-OS-Version` / `X-Version-Code` 上报机型、系统版本与数字版本号（见 `docs/api/common.md`），缺失归为「未知」。后台发布管理页统计区读取 `GET /admin/api/releases/client-statistics?days=7|30`：活跃设备口径为 last_seen_at 在周期内的去重设备，含版本/机型/系统版本分布、按日版本活跃趋势和最新发布版本渗透概况（含低于强升线设备数）。MySQL/内存实现完整，Mongo 为未实现 stub（与 MongoAppReleaseRepository 一致）。

- 管理后台运营工作台的跨域只读统计统一走 `repository.AdminDashboardStatisticsRepository`，由 `NewAdminDashboardStatisticsRepository` 根据当前 MySQL / Mongo / in-memory 仓储族选择实现；不新增统计表，也不改变业务写入流程。周期指标按 `[start,end)` 统计，互动趋势使用收藏/导航/点赞/评论各自 `created_at`，新增用户转化漏斗使用同周期注册 Cohort 并逐级限定为完成轨迹、投稿和审核通过；用户列表上次登录按当前页批量查询 `login_log`。后台用户列表通过 `AdminUserListFilterRepository` 在仓储层组合筛选昵称、内部人员、资料 `city_code` / `province_code`、性别、生日星座和当前生效限制，筛选必须参与总数统计与游标分页，不得只过滤当前页。数据模型见 `internal/models/admin_dashboard.go`、`admin_feature_statistics.go`，聚合实现见 `internal/repository/admin_dashboard*.go` 与 `admin_feature_statistics*.go`。地图模式新增全量 active 路线组覆盖/规模统计；同行新增近 1/7/30 天创建/结束趋势及服务端去重成员人数分布，后台平均参与人数也按成员记录计算，缺失记录不计入均值；前端共用内置 ECharts，不触发供应商查询。

- 主景点以 `internal/models/scenic_spot.go` 的内部 SCP ID 为唯一业务 ID；MySQL、Mongo、内存仓储均由 `repository.ScenicSpotRepository` 实现。`ScenicSpotCatalogService` 管理 active/archived/merged 状态、来源 ID 校验、轨迹关联、合并拆分、审计和存量回填。后台首次加载景点管理页时通过 `/admin/api/scenic-spots/provider-content/backfill` 分批拉取尚未同步的供应商资料；存量简介由详情弹窗中简介标题旁的“AI联网查询”链接单条检索并填入编辑框，保存后入库。新建景点的空简介在创建前同步尝试生成并随记录保存，失败保留空值；轨迹识别自动创建的新目的地（syncTrackPlaceMatch）也会在创建后异步触发生成，已有简介或 manual_edited 不覆盖，失败保留空值可由后台手动补齐。AI 简介限 200 字，人工已有内容不会被自动覆盖。Kimi API Key 从 `MOONSHOT_API_KEY` 注入，仅在持久化仓储可用时启用；不要把密钥写入代码或提交到仓库。图片加入同一有序 `photos` 数组。后台编辑保存会设置 `manual_edited`，自动同步资料仍归为“自动”。自有图片上传追加到数组末尾，拖动排序后按数组顺序下发客户端。运营后台入口 `/admin/scenic_spots.html`（页面显示名「景点管理」，目录涵盖景区/公园/山系/湖泊），API 路由清单和流程见 `internal/admin/AGENTS.md`；Google 来源 ID 到期后在详情访问时验证。

- 景点供应商调用统计只保存每日计数（景点、供应商、操作、请求/成功/失败数），不保存响应正文或照片引用；后台 `/admin/api/scenic-spots/statistics` 改为只读景点内容与覆盖统计：有效景点数、覆盖城市数、关联有效已完成轨迹数、待完善景点数、景点空间分布和近 1/7/30 天新增入库趋势；供应商调用计数继续保留用于排查，不再展示用量图表。内部景点已关联的详情、图片和来源 ID 核验均计数；按坐标运行的后台识别搜索不归因到单个景点，不纳入景点排行榜。存量轨迹需先运行后台回填才能按内部景点归因。MySQL/Mongo/内存仓储都实现 usage counter，相关表为 `scenic_spot_provider_usage`。

- 精品标识 `is_featured` 仅取投稿当前 `approved` 状态；本人已上传 `/track/my/list` 和收藏 `/track/collected/list` 也明确返回 true/false。复用 `TrackSubmissionService` 的当前页批量查询，禁止逐条补查详情或为标识下载投稿图片；响应 DTO 字段不新增表或索引，协议见 `docs/api/track-submission.md`。

- 收藏数 `collect_count` 为当前用户与轨迹唯一收藏关系的总数，包含作者自藏；沿用 `CollectRepository.CountByTrackIDs`，列表批量统计、详情/创建/更新直接下发。详情及全部轨迹列表摘要（包含 `/track/my/list`、`/track/collected/list`）明确返回按 JWT 浏览者计算的 `collected`，false 不省略；作者身份不代表已收藏。列表通过既有 `CollectRepository.ListCollectedByTrackIDs` 按当前页批量查询，禁止逐条补查；本地 Room 列表不属于 Server 接口。POST/DELETE `/track_collect` 保留 status 并返回操作后的 collected/collect_count，重复操作幂等，不新增计数接口或独立计数表。入口见 `internal/service/track_collect_summary.go` 与 `docs/api/collect-navigation.md`；Mongo 使用用户/轨迹唯一索引和 track_id 统计索引。

- 客户端结束录制后先上传原始轨迹和完整预览，再调用 `POST /track/create` 建立轨迹；已结束轨迹携带稳定 `session_id`，创建重试按用户与 session ID 返回已有记录，避免网络结果不确定时重复建轨迹。MySQL、Mongo、内存仓储都实现 `CompletedTrackSessionRepository`。新建轨迹默认公开；所有者可通过 `PUT /track/:id/update` 将 `status` 设为 1（公开）或 2（私密），设为私密时清理公开地图索引与路线组关联。协议见 `docs/api/track.md`。

- 评论入口为 `CommentService` / `CommentRepository`，协议见 `docs/api/track-comments.md`。沿用点赞的轨迹可见范围；主评论和回复按创建时间、ID 倒序游标分页，回复以 root_id 平铺、reply_to_id 引用直接目标。轨迹详情/列表直接下发 comment_count/can_comment，列表按当前页批量读取计数；发布/删除响应带回总数，不另设评论状态接口。评论页只附最新作者回复摘要；总数、回复数、赞踩数随事务维护，发布以 user_id + client_comment_id 幂等。删除主评论保留有回复的占位，删除引用目标不得下发其原文。评论图片先预约 ID 再 OSS 直传并校验归属，私有缓存不挂通用 static。MySQL/Mongo/内存三实现保持一致，Mongo 评论写入要求副本集。账号限制在 CommentService 中覆盖发布、图片预约和非零评价，允许本人删除与取消评价。

- 轨迹点赞独立于收藏，`track_likes` 以用户与轨迹唯一关系保证 POST/DELETE 幂等。个人获赞总数只统计其他用户对正常公开、已结束、raw_track_url 非空轨迹的现存点赞，排除作者自赞。个人主页展示获赞数并提供「喜欢」tab：当前用户点赞过的轨迹列表经 `GET /track/liked/list` 下发，协议与收藏列表完全同构（`TrackService.ListLikedTracks` 按点赞 created_at desc, track_id desc 游标翻页，剔除删除/私密/进行中，响应复用 `CollectedTrackSummaryPage`）；获赞逐条列表和点赞人名单接口不提供。有效点赞关系长期保留，用于轨迹点赞状态、轨迹点赞数、个人获赞总数和喜欢列表；允许自赞并计入轨迹点赞数，受限账号不能新增点赞但可取消。修改时同步三种仓储、用户详情计数、详情/列表的浏览者状态及 `docs/api/track-likes.md`。

- 客态个人主页的 `GET /user/:user_id/tracks` 和 `GET /user/:user_id/collections` 使用路径用户作为列表所属用户、JWT 用户作为浏览者；`collected` 必须按浏览者计算。已传轨迹在仓储分页前过滤私密、删除、进行中和原始轨迹地址为空的记录；收藏列表跳过失效及不可公开查看的记录后继续补页。`total_count` 只统计接口可见记录，不复用包含私密轨迹的用户详情统计。主态继续使用原有本人列表接口。三种仓储实现与协议入口见 `internal/repository/profile_tracks.go`、`internal/service/profile_tracks.go`、`docs/api/user.md`。

- 个人资料的 `background_url` 用于主页顶部，数据库保存原始 OSS 地址，响应经独立 `profile_backgrounds` 缓存并使用源地址摘要区分版本；`gender` 只允许空串、`male`、`female`，详情仅本人可读。改字段时同步 MySQL 建表及幂等列迁移、Mongo/内存仓储、用户详情/更新协议与缓存注入，参见 `docs/api/user.md`。

- 最近确认所在地通过 `PUT /user/profile/location` 只提交目录中的可选 `city_code`，服务端由持久化地区目录生成 `province_code/province_name/city_code/city_name`，四字段仅本人详情返回。该接口只局部更新省市列，相同值直接成功且不写库或刷新 `updated_at`；普通资料整行更新不得覆盖省市字段。Android 按账号持久化已同步/待同步状态，运动会话首个有效地址立即检查，普通前台定位每 7 天检查一次，只有城市变化才请求，失败退避重试，不新增后台定位。协议见 `docs/api/user.md`。

- `users.is_internal` 是后台内部人员标识，模型 `json:"-"` 保证业务用户 API 和登录/资料响应不下发；MySQL 使用 `TINYINT(1) NOT NULL DEFAULT 0` 并启动幂等补列，Mongo/内存字段同名。仅后台用户列表和详情接口读取/修改，详情保存可更新该字段，普通资料更新必须保留其当前值。

- 默认运动类型由 `internal/config/config.go:DefaultTrackTypeConfigs` 维护，当前 code 为 `hiking`、`running`、`climbing`、`riding`、`driving`，展示名为 `徒步`、`跑步`、`爬山`、`骑行`、`自驾`；`GET /api/v1/track/types`、轨迹入库 `track_type`、运动类型图标元信息和成就系统类型口径应保持一致。颜色元信息同样由该配置维护：`theme_color` 是日间色，`theme_color_dark` 是独立夜间色；客户端按主题直接选色，夜间字段缺失、为空或非法时回退日间色。未知自定义类型两个颜色字段均为空串；颜色不存入轨迹或数据库，协议见 `docs/api/track.md`。`track_geo_indexes.track_type` 与 `track_route_groups.track_type` 只允许保存这五个英文 code，历史别名或未知类型需在地图索引阶段归一/兜底。
- 内置运动图标使用 MingCute 线性 SVG，源文件与许可证位于 `internal/handler/static/track_type_icon/`，由 `internal/handler/track_type_icon_handler.go` 的 go:embed 随二进制打包。登录后通过 GET/HEAD /api/v1/track/types/icons/:icon_file 读取，不依赖磁盘 static 缓存；徒步/跑步/爬山/骑行/自驾分别使用 hiking/run/mountain_2/bike/car。修改图形时必须提升文件名版本并同步 DefaultTrackTypeConfigs.IconFile，防止客户端命中旧缓存；自定义类型仍走原静态目录。
- 运动类型双语名称由同一 `DefaultTrackTypeConfigs` 维护，`/track/types` 始终返回兼容的中文/配置名 `name` 与独立英文名 `name_en`（Hiking/Running/Climbing/Cycling/Driving），不按语言头覆盖 `name`；客户端按当前语言选字段，缓存可同时保留双语，未知自定义类型英文名为空。显示语言不改变 `type`、轨迹存储或统计口径；协议变更同步 `docs/api/track.md`、`models.md` 与路由索引。
- 推荐列表 `GET /api/v1/track/recommend/list` 的可选 `city_code` 使用精确匹配；空值保持全城市推荐。城市条件必须下沉到 Repository 查询并参与游标分页，不能在 Service 取回一页后再过滤；MySQL 推荐查询索引为 `idx_track_recommend_city`。
- 搜索列表 `GET /api/v1/track/search/list` 的可选 `city_code`、`track_type` 使用存储值精确匹配；空值不过滤对应字段，两个条件可组合。筛选条件必须下沉到 Repository 查询并参与游标分页，不能在 Service 取回一页后再过滤；MySQL / Mongo 搜索查询索引分别为 `idx_track_search_city`、`idx_track_search_type`，运动类型筛选不对历史中文值做转换。
- 轨迹 `locate_addr` 最大长度为 255 字符，对应 `track_records.locate_addr VARCHAR(255)`；修改该字段长度时同步更新 `mysql.sql`、`internal/repository/mysql.go` 与 `docs/api/track.md`。
- 轨迹 `source_tag` 是来源/运营标签，对应 `track_records.source_tag VARCHAR(64)`；业务接口只允许空字符串或 `manual_seed`（人工录入冷启动轨迹），更新接口仅在原值为空时补写，普通列表摘要不返回该字段；修改该字段口径时同步更新 `internal/service/track_service.go`、`mysql.sql` 与 `docs/api/track.md`。
- 轨迹 `description` 是客户端依据完整 GPS 轨迹生成的默认路线说明，存于 MySQL `track_records.route_description` / Mongo `description`，详情与创建/更新响应下发。`description_source` 区分 `auto/manual`，`description_rule_version` 记录生成规则版本；存量缺失元数据按 `auto/v0`，当前 Android 生成版本为 1。上传只允许自动来源升级规则版本；DevMode 历史补齐查询空说明或版本落后的自动说明，完成时对说明、来源、规则版本和 GPS 相关事实做 CAS，绝不覆盖 manual。投稿允许编辑的文案独立存于投稿记录。字段变更同步 `internal/models/track.go`、MySQL 建表/迁移、三种仓储、`internal/service/track_service.go` 与 `docs/api/track.md`。
- 首页地图模式的轨迹空间索引由 `track_map_index` 后台任务异步构建：轨迹完成接口只写入 `track_map_index_jobs`，不得在请求主链路同步下载 OSS raw track 或解析轨迹点；后台下载 raw track 必须通过 `OSS_INTERNAL_ENDPOINT` 内网域名，未配置时失败重试且不回退公网；raw track 解析支持 JSON / GeoJSON / KML / KMZ。路线组由 `track_route_group` 离线任务基于 `track_geo_indexes` 聚合生成并写入 `track_route_groups.area_id`，默认每天 04:00 执行。
- 地图查询 `/track-map/view` 与 `/track-map/groups` 支持显式 `track_type=all`，由 `TrackMapService.buildFilter` 转为空类型过滤；未传/空值继续默认 hiking，兼容旧客户端。all 仅为查询哨兵，禁止写入轨迹/地图索引或跨类型合并 RouteGroup；城市/区域气泡汇总多种类型且返回空 track_type，具体路线组保留真实类型。MySQL 区域 SQL 必须按 area_id/网格合并，不再额外按 track_type 拆同 ID 气泡，与 Mongo/内存保持一致；协议见 `docs/api/track-map.md`、`track_map.md`。
- 地图区县兜底来自 `internal/maparea/districts.json` 与 `catalog.json`，覆盖 DataV 全国索引的末级行政区域（区县优先，无下辖区县的城市及台湾省级边界兜底）；同 ID 人工覆盖保留生成 geometry。通过 `cmd/update-map-districts` 批量生成，`districts.manifest.json` 记录索引/边界来源摘要与覆盖清单，缺边界或删除既有 ID 时生成失败；数据更新后由 `track_route_group` 重聚合更新存量归属。`track_place` 后台任务默认每分钟扫描存量/新增空间索引，用高德普通逆地理编码AOI识别景区，不依赖高阶边界权限；准入类型放宽为风景名胜/公园（11xxxx）与自然地名（1904xx 山系/湖泊），环线矩形召回 types=110000|190400；供应商分类归一为景区/公园/山系/湖泊四类，山系/湖泊等开放自然地物边界不稳定，自动识别只进 candidate 候选队列由人工确认，不自动归属；规则 v3 的5个等距样本至少2个命中同一景区，不要求连续，最高票并列不强选；源未变的旧 matched 保留。无采样主归属且无并列时，track_place_ring.go / track_place_search.go 从完整 GPS 提取近闭合子路径并用普通矩形搜索分页召回景点，再判断 POI 是否在圈内；子路径长度至少100米，闭合点距离不超过子路径长度15%（下限25米、上限150米）。父子候选合并后唯一才自动归属；围住多个独立景点时保存为 candidate，仅进入后台确认队列，不下发客户端主景点；超过100候选时不强选。地图聚合 place_match_method 改为 track_relation。开启后旧手工景区矩形不再参与自动归属。Google适配器仅建立附近地点关联，不将viewport或邻近结果当成真实边界；需要独立服务端Key及适用的内容暂存配置，默认关闭Google内容持久化。
- 轨迹详情及所有轨迹列表摘要可选返回只读 `primary_scenic_spot={id,name}`，入口 `internal/service/track_scenic_summary.go`。通过 `TrackPlaceIndexReader.FindPlaceGeoIndexes` 按页批量核对源摘要与规则，再读取共享地点；只下发 matched 主归属，无结果或资料不可用则省略，绝不把 nearby 当主景区、不逐条查询或在 API 请求中调用供应商。响应装饰不持久化到轨迹表，协议见 `docs/api/track.md`；Android 详情在现有标签行最后直接显示景区名称。
- 景区资料、查询缓存、轨迹关联及游标/租约存于 `track_place_records`，每日北京时间预算存于 `track_place_api_usage`；MySQL/Mongo/内存三实现均在 `internal/repository/track_place*.go`。只有后台任务调用供应商；地图接口和每日路线组重建只读取本地记录。高德轨迹 matched/candidate/unmatched 成功结果及共享地点资料一次完成、长期复用，兼容旧版已成功记录，不因时间到期重复外呼；只有 GPS 源摘要或主动升级规则变化才失效。请求级缓存、暂停记录和 Google 内容仍按 TTL 清理，绝不能误删高德永久资料；错误和额度耗尽分别退避，重启不清零预算。路线组重建批量恢复，增量写area_id只允许代表轨迹仍一致。`area.id` 支持稳定 `place-amap-*` ID；未命中继续区县/网格，地图气泡bbox是路线范围而非景区边界。配置/人工验收见 `docs/track-place-discovery.md`。判定规则 v3 与证据版本 v1 分离，源摘要/旧点查询 key 不变，v1/v2 完整 Relations 可直接按新闭环阈值重判；evidence_complete 保留环内搜索失败前的采样证据。扫描游标按规则版本隔离，升级后自动从头重扫。ring 查询共用预算、租约与 TTL，不影响成功结果的一次完成策略。
- 景点管理详情的可交互地图使用独立的 `AMAP_WEB_KEY` 与 `AMAP_SECURITY_JS_CODE`；仅登录后台可读取 Web Key，安全密钥只在同源代理加入高德请求。地图拖动/点选后的坐标经原有景点更新接口保存，人工移动后的坐标系为 GCJ02；供应商坐标在未编辑时保持来源坐标系。
- 新空间索引统一国内GCJ02/境外恒等WGS84数值的地图接口口径，支持WGS84/GCJ02/BD09输入，原始GPS文件不变；旧非GCJ02索引由景区任务用完整GPS逐批重建。coordinate_system 描述原始上传文件，首次附加 raw_track_url 可同步更正创建时声明。索引读取同一份 KML 的 track_export_coordinate_system 标记；旧 source_tag 为空、声明 GCJ02 且具有本项目导出特征的文件可修正为 WGS84，再重建索引。TrackRawCoordinateWriter 的 MySQL/Mongo/内存 CAS 仅改声明和更新时间；v1 景区迁移检查一次此类原生导出，不推断早期空声明或无显式标记的人工导入。服务路由用大陆省级多边形掩码，不能用宽泛经纬度矩形把邻国送到高德。内存降级时不发外部请求。`GET /track-map/groups/:group_id/detail` 可选下发本地有效的 `nearby_places`，不触发供应商请求。
- `track_map_index_jobs` 补偿入队必须保持幂等：对已有 `pending` job 只能保留或提前 `next_run_at`，不能刷新到更晚时间，否则补偿扫描会反复推迟任务导致 `claimed=0`。
- 轨迹投稿图片可选（0～9 张）：客户端直传 OSS，服务端只保存 OSS URL；读取时经独立 `submission_images` AssetCache 缓存到 `<LogDir>/static/submission_images/`，业务响应改写为 `/api/v1/static/submission_images/*`，admin 响应继续改写为 `/admin/api/static/submission_images/*`。撤回投稿使用 `POST /api/v1/track/:track_id/submission/withdraw` 并保留投稿、图片和审核流水，不使用 DELETE。
- Docker 镜像与 `deploy/docker-compose.yml` 默认设置 `TZ=Asia/Shanghai`；robfig/cron 按服务进程本地时区解释 `TRACK_*_CRON`、`ANALYTICS_SYNC_CRON` 等表达式，调整镜像/compose 时区时必须同步评估定时任务触发时间。
- 埋点采集接口 `POST /api/v1/analytics/events` 位于公开路由，用于未登录启动/登录页等匿名事件；`user_id` 非空的事件必须携带同一用户的有效 JWT，`X-User-ID` 不能建立可信身份。事件身份在发生时固化，客户端按身份域隔离队列，账号切换和离线补发不得改写；服务端不得把 A 用户历史事件归属给当前 B 用户。批量接口只做整批确认，200 时 `accepted` 必须等于发送数，不返回部分成功；可重试错误使用原 `event_id` 整批退避重试。服务端完成校验、脱敏和本地 JSONL 落盘，不把原始埋点写入业务 MySQL，凌晨同步 OSS 的任务由 `SCHEDULER_ENABLED` 控制。同步时按 `event_date/hour` 合并小 JSONL，单个 OSS part 目标上限 128 MB。`analytics_sync_summaries` 只记录每次 OSS 同步摘要（源文件列表、OSS part key、字节数、耗时、错误等），不保存原始事件。推荐的收藏、导航、关注等强行为从业务数据库聚合，曝光、点击、详情浏览按每天 03:00 同步链路以 T+1、24 小时级可用，推荐原始事件保留 180 天。调整事件协议、认证策略、本地目录、OSS 前缀、批量上限、同步时间或同步摘要字段时，同步更新 `track_analytics.md`、`docs/api/analytics.md` 与 `mysql.sql`。
- 导航强反馈只认 `track_navigations` 业务记录：客户端在用户已成功开始导航并主动结束后，先持久化本地会话“已尝试上报”标记，再最多调用一次非幂等的 `POST /api/v1/track/:track_id/navigation/report`；点击、取消、初始化失败、开始前退出、崩溃或强杀不计强反馈，响应不确定不得自动重试。只有明确收到 200 才生成 `track_navigation_report_success`；该成功埋点可以按埋点批量接口规则重试。调整此口径时同步更新 `docs/api/collect-navigation.md`、`track_analytics.md` 与 `track_recommendation.md`。
- 推荐第一版归因协议要求推荐响应提供响应级 `request_id` / `strategy` 和 item 级 `recommend_rank` / `candidate_source` / `recommend_reason`；客户端通过 `track_recommend_impression` 上报真实卡片曝光，并在点击、详情、收藏、取消收藏、导航和分享链路中透传同一推荐上下文。曝光必须同时满足 App 前台、推荐页实际可见、卡片至少 50% 可见并连续 500 ms，页面重建继续使用 Feed 级去重；空结果页成功展示也上报 `track_recommend_view`。第一版默认排除本人轨迹，已收藏/导航轨迹只降频不硬排除，Feed Session TTL 为 60 分钟且最多固化 200 条；Session 内内容失效时向后扫描补页并保留原始排名跳号，不得返回 `items=[]`、`has_more=true`。Session 过期后客户端保留筛选条件静默刷新第一页且只自动重试一次，新 Feed 使用新 `request_id`，但历史队列事件和旧详情页后续转化保持原上下文；Session 仓储暂时不可用返回 `recommend_session_unavailable`，不得刷新或切换 Legacy。离线任务由现有进程内 Scheduler 承担。推荐功能只对支持新协议的首发客户端开放：服务端可提前部署但必须保持 `RECOMMENDATION_ENABLED=false`，待两端联调验收通过后再整体启用；不增加客户端能力头或版本分流，旧内部测试包不在兼容范围内。字段、错误码、曝光与归因验收规则以 `track_recommendation.md` 和 `track_analytics.md` 为准。
- 推荐派生数据使用 `recommend_feed_sessions`、`recommend_user_profiles`、`recommend_item_stats_daily`；MySQL/Mongo/in-memory 都必须实现 `RecommendationRepository`。强行为画像和物料统计由 `recommendation_profile`（默认 05:00）与 `recommendation_item_stats`（默认 05:30）离线任务重建，`recommendation_session_cleanup` 默认每 20 分钟清理过期 Session。在线推荐只依赖本进程仓储，不新增外部服务；画像/统计缺失或过期时首屏固化 Legacy Session，已有 Session 读取失败时返回稳定错误，不能换序。
- 推荐多样性重排对 RouteGroup、作者和运动类型只做软约束：优先不同 RouteGroup，但候选不足时必须允许同组轨迹补位，不得永久丢弃候选或因此缩短 Feed；修改重排逻辑时需检查“全部同组”和“候选被页内约束延后后其 RouteGroup 已被选中”两种情况，自动化测试遵循“默认验证方式”。
- 推荐运行参数统一在 `internal/config/config.go` 读取：`RECOMMENDATION_ENABLED` 默认 `false`；计算超时默认 250 ms；Session TTL 默认 60 分钟；派生数据最大年龄默认 48 小时；候选上限默认 300、最终 Feed 上限固定钳制为 200。`SCHEDULER_ENABLED=true` 时推荐离线任务会独立于接口总开关运行，允许上线前预热画像和物料统计；待首批任务成功且两端联调通过后再开启总开关。

## 第二层（附录）

### 不要轻易改动的文件
| 文件 | 原因 |
| --- | --- |
| `internal/regions/seed.json` | 首次建库地区底稿，来自客户端原省市 JSON；持久化后不再覆盖后台目录 |
| `internal/maparea/districts.json` | 全国行政区末级节点 Polygon/MultiPolygon 与边界框基线；按 `internal/maparea/README.md` 使用 `cmd/update-map-districts` 批量更新，核对 manifest，不逐条手改 |
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

## 设备权限快照

- `internal/{models,repository,service,handler}/permission_snapshot.go` 负责 JWT 账号 + 安装实例全量快照；启动时按用户仓储选择 MySQL/Mongo/内存，初始化失败只禁用快照接口，不静默转内存。表/集合为 `user_permission_snapshots`，按 sequence 单调替换，收到时间由服务端生成；未知权限键、分类、状态必须兼容。协议见 `docs/api/permission-snapshots.md`。

- AI 路线说明自动任务仅在持久仓储、预览缓存、Moonshot key 与 scheduler 就绪时启用；首次水位排除未变更历史记录。复用现有索引和一张方图，不重复解析 KMZ；输入摘要去重、持久缓存、分布式租约和每日预算，条件更新保护人工说明/形象。`description_source=ai` 不进入客户端 auto 补齐。


## 景区 A 级与标签文案

景区 A 级采集入口 `internal/service/scenic_spot_grade.go`，仅接受同一有效高德 POI ID 的 v5 detail business.keytag 中明确的 A 级标识，统一为 1A..5A，未知留空；不把评分、酒店星级、名称/简介或 AI 推断当等级。等级独立于文字图片补采，复用既有日配额和调用统计，无新表、环境变量或定时器。状态、来源、采集时间随 JSON/BSON 保存；无等级缓存 unavailable，失败后续重试，人工修改/清空记 manual 且不被覆盖。旧编辑请求省略 scenic_grade 保留原值。

发现景点及轨迹 primary_scenic_spot 下发可选 scenic_grade，读取不调用供应商；未知不能解释为官方未评级。统一 labels 的 scenic_spot.name 由 API 组装为“景点名·5A”（U+00B7 居中点），无等级保留原名，code 仍为稳定景点 ID，primary_scenic_spot.name 保持纯名称。客户端直接消费 labels，不重复拼接。完整同步与字段契约见 [景区等级采集与下发](track-place-discovery.md#景区等级采集与下发)。


## 本人运动类型分布

`GET /track/activity/type-distribution` 只取当前JWT用户，返回全部历史已完成、正常/私密且raw_track_url非空的记录按类型聚合的次数、时长秒数、距离米数。删除、进行中与自动创建的未上传原始文件占位记录排除；负数/缺失指标归零，空集合返回items=[]。权限不支持指定其他用户。入口 internal/service/track_activity.go，复用类型归一函数、合并已知别名，未知类型保留，空类型为other；按时长降序、同值按type code排序。

独立 `repository.ActivityTypeStatisticsRepository` 的MySQL GROUP BY、Mongo聚合与内存实现提供相同口径，不逐条加载轨迹、GPS、图片或供应商内容；三种实现有编译期断言。不新建表、列、索引、缓存或后台任务，不改写业务数据。协议见 [运动类型分布](api/track.md#运动类型分布)。客户端合并本机未上传记录后计算比例，统计范围与近12个月日历分开标注。
