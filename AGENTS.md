# AGENTS.md — track_server

> 本文面向 AI Coding Agent（Codex / Claude Code / Trae / Cursor）。
> 仅保留稳定约束、改动导航与提交前最小检查；短期数据、压测结论等不写入本文。

## 第一层（必读）

### 使用规则
- 从当前工作目录向上逐级查找 `AGENTS.md`，以最近的一份为准。
- 用 `AGENTS.md` 定位相关文件、约束、流程与验证步骤；实现细节以代码为准。
- 若 `AGENTS.md` 与代码不一致，以代码为准，并在回复中明确指出不一致点。

### 默认验证方式
- 用户约定（2026-10-20）：默认由用户手动验证功能；Agent 不专门新增测试用例、不主动运行单元测试、集成测试、race 检查、接口自动化或全量回归，也不为验证启动测试数据库、容器或额外搭建测试环境。
- 代码改动只做必要的编译/构建检查及改动自查，Go 默认使用 `go build ./...`；纯文档或规则变更无需构建业务代码。
- 只有用户明确要求编写或运行测试时，才执行对应范围；此约定覆盖本文件及 scoped AGENTS 中旧的默认测试要求，不得因技能建议自行扩大范围。已有测试保留，仅在本次改动造成编译/接口失配时做必要修正。
- 交付时简短说明编译结果，并注明功能待用户验证；未执行的测试不得声称通过。

### 发布与部署
- 用户约定（2026-10-22）：发布 App 或部署 Server 时，同时同步、提交、推送实际发布的对应仓库，无需为常规 Git 步骤再次确认；仅部署 Server 时只处理本仓库。入口见同级 Android 仓库的 [android-track-release](../android-track/.agents/skills/android-track-release/SKILL.md)，服务端先完成 Git 推送，再构建镜像和更新生产。
- 仅修改发布技能不触发发版、部署或 Git 写操作；敏感配置、签名、构建产物和缓存不入库。Git 与生产更新分别核对结果，不能把“代码已推送”报告为“部署成功”。

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
- **鉴权分组**：所有业务接口都在 `api := h.Group("/api/v1")` 下的 `auth` 子组中；公开接口只有 `/ping`、`/captcha`、`/sms/send`、`/login/*`、`/upgrade/check`、`/achievement/level-rules.html`、`/legal/privacy-policy.html`、`/legal/terms-of-service.html`、`/track-map/areas/:area_id/introduction.html`、安装包静态下载。`/internal/*` 与 `/ops/*` 是内部/运维接口，不走业务 JWT，必须使用内部 token 鉴权。新增业务接口默认加到 `auth` 组，除非有明确登录豁免需求。
- **Repository 降级链约束**：MySQL / Mongo / in-memory 三种实现都必须实现 `internal/repository/interfaces.go` 里的 interface 全集；新增接口方法时，三份实现都要补齐，否则启动会编译失败或运行时 panic。
- **反馈图片私有落盘**：意见反馈图片由服务端直接接收，保存到 `<LogDir>/feedback/images/`，必须通过 `/feedback/:feedback_id/images/:image_id` 或 `/ops/feedback/:feedback_id/images/:image_id` 走鉴权 handler 读取；不要放进 `/api/v1/static/*` 公开静态目录。
- **生成文件不手改**：`internal/config/china_city_raw.json`、`internal/config/china_province_raw.json` 为外部数据，不要逐行手工编辑。

### 目录总览
```
track_server/
├── cmd/server/             # main 入口：加载配置 → 选仓储 → 注入 service → 注册路由
├── cmd/update-map-districts/ # 从 DataV 全国索引生成行政区边界与覆盖清单，支持缓存离线复现
├── cmd/backfill-track-titles/ # MySQL 存量时间标题回填工具：先生成计划，再按旧标题条件更新
├── internal/
│   ├── config/             # 环境变量加载；内置省市数据 + 昵称字典 + 同行弹幕敏感词词库
│   ├── handler/            # Hertz HTTP handler + router.go 路由表（权威）
│   ├── maparea/            # 内置 GCJ-02 景区/区县目录；生成区县基线 + 人工覆盖 + 介绍页内容
│   ├── middleware/         # JWT 鉴权、请求元信息、Token 黑名单
│   ├── models/             # 领域模型（Track / Recommendation / TrackSubmission / RouteGroup 路线介绍 / ScenicSpot / User / UserFollow / AccountRestriction / Companion / CompanionEvent / Achievement / Feedback / Analytics / 相关光标/子结构）
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
├── track_recommendation.md # 推荐轨迹个性化推荐第一版设计（规则召回 / Feed Session / Legacy 兜底 / 推荐归因）
├── track_submission.md     # 轨迹投稿功能方案（审核、结构化路线资料、图片缓存、推荐与代表轨迹）
└── login.md                # 登录流程与协议说明
```

### 权威数据源
| 主题 | 权威文件 |
| --- | --- |
| HTTP 路由清单 | `internal/handler/router.go` |
| 配置项与默认值 | `internal/config/config.go` |
| Repository 接口契约 | `internal/repository/interfaces.go` |
| 领域模型 | `internal/models/track.go`、`internal/models/recommendation.go`、`internal/models/track_submission.go`、`internal/models/scenic_spot.go`、`internal/models/user.go`（含 AccountRestriction）、`internal/models/companion.go`、`internal/models/achievement.go`、`internal/models/feedback.go`、`internal/models/analytics.go` |
| 地图区域语义与介绍内容 | `internal/service/track_place*.go`（异步景区识别）+ `internal/maparea/districts.json`（区县基线）+ `catalog.json`（区县覆盖/旧介绍兼容）；`mainland.json` 仅用于服务路由和坐标转换；配置及维护见 `docs/track-place-discovery.md` |
| MySQL 表结构 | `mysql.sql` |
| 接口协议 | `docs/api/`（入口 `track_api.md`，路由索引 `docs/api/route-index.md`，账号限制接口 `docs/api/account-restriction.md`，首页地图接口 `docs/api/track-map.md`）、`login.md`、`track_companion.md`、`track_achievement_client.md`、`track_map.md`；推荐第一版设计与客户端归因协议见 `track_recommendation.md`、`track_analytics.md` |
| 客户端埋点方案 | `track_analytics.md`、`docs/api/analytics.md` |
| 成就规则方案 | `track_achievement.md` |
| 轨迹投稿功能与协议 | `track_submission.md`、`docs/api/track-submission.md`；路由权威仍为 `internal/handler/router.go` |

### 稳定默认值

- 管理后台运营工作台的跨域只读统计统一走 `repository.AdminDashboardStatisticsRepository`，由 `NewAdminDashboardStatisticsRepository` 根据当前 MySQL / Mongo / in-memory 仓储族选择实现；不新增统计表，也不改变业务写入流程。周期指标按 `[start,end)` 统计，互动趋势使用收藏/导航/点赞/评论各自 `created_at`，新增用户转化漏斗使用同周期注册 Cohort 并逐级限定为完成轨迹、投稿和审核通过；用户列表上次登录按当前页批量查询 `login_log`。后台用户列表通过 `AdminUserListFilterRepository` 在仓储层组合筛选昵称、内部人员、资料 `city_code`、性别、生日星座和当前生效限制，筛选必须参与总数统计与游标分页，不得只过滤当前页。数据模型见 `internal/models/admin_dashboard.go`、`admin_feature_statistics.go`，聚合实现见 `internal/repository/admin_dashboard*.go` 与 `admin_feature_statistics*.go`。地图模式新增全量 active 路线组覆盖/规模统计；同行新增近 1/7/30 天创建/结束趋势及服务端去重成员人数分布，后台平均参与人数也按成员记录计算，缺失记录不计入均值；前端共用内置 ECharts，不触发供应商查询。

- 主景点以 `internal/models/scenic_spot.go` 的内部 SCP ID 为唯一业务 ID；MySQL、Mongo、内存仓储均由 `repository.ScenicSpotRepository` 实现。`ScenicSpotCatalogService` 管理草稿/发布、来源 ID 校验、轨迹关联、合并拆分、审计和存量回填。后台首次加载景点管理页时通过 `/admin/api/scenic-spots/provider-content/backfill` 分批拉取尚未同步的供应商资料；存量简介由详情弹窗中简介标题旁的“AI联网查询”链接单条检索并填入编辑框，保存后入库。新建景点的空简介在创建前同步尝试生成并随记录保存，失败保留空值。AI 简介限 200 字，人工已有内容不会被自动覆盖。Kimi API Key 从 `MOONSHOT_API_KEY` 注入，仅在持久化仓储可用时启用；不要把密钥写入代码或提交到仓库。图片加入同一有序 `photos` 数组。后台编辑保存会设置 `manual_edited`，自动同步资料仍归为“自动”。自有图片上传追加到数组末尾，拖动排序后按数组顺序下发客户端。运营后台入口 `/admin/scenic_spots.html`，API 路由清单和流程见 `internal/admin/AGENTS.md`；Google 来源 ID 到期后在详情访问时验证。

- 景点供应商调用统计只保存每日计数（景点、供应商、操作、请求/成功/失败数），不保存响应正文或照片引用；后台 `/admin/api/scenic-spots/statistics` 改为只读景点内容与覆盖统计：有效景点数、覆盖城市数、关联有效已完成轨迹数、待完善景点数、景点空间分布和近 1/7/30 天新增入库趋势；供应商调用计数继续保留用于排查，不再展示用量图表。内部景点已关联的详情、图片和来源 ID 核验均计数；按坐标运行的后台识别搜索不归因到单个景点，不纳入景点排行榜。存量轨迹需先运行后台回填才能按内部景点归因。MySQL/Mongo/内存仓储都实现 usage counter，相关表为 `scenic_spot_provider_usage`。

- 精品标识 `is_featured` 仅取投稿当前 `approved` 状态；本人已上传 `/track/my/list` 和收藏 `/track/collected/list` 也明确返回 true/false。复用 `TrackSubmissionService` 的当前页批量查询，禁止逐条补查详情或为标识下载投稿图片；响应 DTO 字段不新增表或索引，协议见 `docs/api/track-submission.md`。

- 收藏数 `collect_count` 为当前用户与轨迹唯一收藏关系的总数，包含作者自藏；沿用 `CollectRepository.CountByTrackIDs`，列表批量统计、详情/创建/更新直接下发。详情及全部轨迹列表摘要（包含 `/track/my/list`、`/track/collected/list`）明确返回按 JWT 浏览者计算的 `collected`，false 不省略；作者身份不代表已收藏。列表通过既有 `CollectRepository.ListCollectedByTrackIDs` 按当前页批量查询，禁止逐条补查；本地 Room 列表不属于 Server 接口。POST/DELETE `/track_collect` 保留 status 并返回操作后的 collected/collect_count，重复操作幂等，不新增计数接口或独立计数表。入口见 `internal/service/track_collect_summary.go` 与 `docs/api/collect-navigation.md`；Mongo 使用用户/轨迹唯一索引和 track_id 统计索引。

- 客户端结束录制后先上传原始轨迹和完整预览，再调用 `POST /track/create` 建立轨迹；已结束轨迹携带稳定 `session_id`，创建重试按用户与 session ID 返回已有记录，避免网络结果不确定时重复建轨迹。MySQL、Mongo、内存仓储都实现 `CompletedTrackSessionRepository`。新建轨迹默认公开；所有者可通过 `PUT /track/:id/update` 将 `status` 设为 1（公开）或 2（私密），设为私密时清理公开地图索引与路线组关联。协议见 `docs/api/track.md`。

- 评论入口为 `CommentService` / `CommentRepository`，协议见 `docs/api/track-comments.md`。沿用点赞的轨迹可见范围；主评论和回复按创建时间、ID 倒序游标分页，回复以 root_id 平铺、reply_to_id 引用直接目标。轨迹详情/列表直接下发 comment_count/can_comment，列表按当前页批量读取计数；发布/删除响应带回总数，不另设评论状态接口。评论页只附最新作者回复摘要；总数、回复数、赞踩数随事务维护，发布以 user_id + client_comment_id 幂等。删除主评论保留有回复的占位，删除引用目标不得下发其原文。评论图片先预约 ID 再 OSS 直传并校验归属，私有缓存不挂通用 static。MySQL/Mongo/内存三实现保持一致，Mongo 评论写入要求副本集。账号限制在 CommentService 中覆盖发布、图片预约和非零评价，允许本人删除与取消评价。

- 轨迹点赞独立于收藏，`track_likes` 以用户与轨迹唯一关系保证 POST/DELETE 幂等。个人获赞总数只统计其他用户对正常公开、已结束、raw_track_url 非空轨迹的现存点赞，排除作者自赞。个人主页仅展示获赞数；获赞逐条列表、聚合轨迹列表和点赞人名单接口已移除，不保留旧列表兼容入口。有效点赞关系长期保留，用于轨迹点赞状态、轨迹点赞数和个人获赞总数；允许自赞并计入轨迹点赞数，受限账号不能新增点赞但可取消。修改时同步三种仓储、用户详情计数、详情/列表的浏览者状态及 `docs/api/track-likes.md`。

- 客态个人主页的 `GET /user/:user_id/tracks` 和 `GET /user/:user_id/collections` 使用路径用户作为列表所属用户、JWT 用户作为浏览者；`collected` 必须按浏览者计算。已传轨迹在仓储分页前过滤私密、删除、进行中和原始轨迹地址为空的记录；收藏列表跳过失效及不可公开查看的记录后继续补页。`total_count` 只统计接口可见记录，不复用包含私密轨迹的用户详情统计。主态继续使用原有本人列表接口。三种仓储实现与协议入口见 `internal/repository/profile_tracks.go`、`internal/service/profile_tracks.go`、`docs/api/user.md`。

- 个人资料的 `background_url` 用于主页顶部，数据库保存原始 OSS 地址，响应经独立 `profile_backgrounds` 缓存并使用源地址摘要区分版本；`gender` 只允许空串、`male`、`female`，详情仅本人可读。改字段时同步 MySQL 建表及幂等列迁移、Mongo/内存仓储、用户详情/更新协议与缓存注入，参见 `docs/api/user.md`。

- 最近确认所在地通过 `PUT /user/profile/location` 只提交归一化 `city_code`，服务端由内置行政区数据生成 `province_code/province_name/city_code/city_name`，四字段仅本人详情返回。该接口只局部更新省市列，相同值直接成功且不写库或刷新 `updated_at`；普通资料整行更新不得覆盖省市字段。Android 按账号持久化已同步/待同步状态，运动会话首个有效地址立即检查，普通前台定位每 7 天检查一次，只有城市变化才请求，失败退避重试，不新增后台定位。协议见 `docs/api/user.md`。

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
- 地图区县兜底来自 `internal/maparea/districts.json` 与 `catalog.json`，覆盖 DataV 全国索引的末级行政区域（区县优先，无下辖区县的城市及台湾省级边界兜底）；同 ID 人工覆盖保留生成 geometry。通过 `cmd/update-map-districts` 批量生成，`districts.manifest.json` 记录索引/边界来源摘要与覆盖清单，缺边界或删除既有 ID 时生成失败；数据更新后由 `track_route_group` 重聚合更新存量归属。`track_place` 后台任务默认每分钟扫描存量/新增空间索引，用高德普通逆地理编码AOI识别景区，不依赖高阶边界权限；规则 v2 的5个等距样本至少2个命中同一景区，不要求连续，最高票并列不强选；源未变的旧 matched 保留。无采样主归属且无并列时，track_place_ring.go / track_place_search.go 用普通矩形搜索分页召回景点，再按实际近闭合轨迹判断 POI 是否在圈内；父子候选合并后唯一才归属，超过100候选时不强选。地图聚合 place_match_method 改为 track_relation。开启后旧手工景区矩形不再参与自动归属。Google适配器仅建立附近地点关联，不将viewport或邻近结果当成真实边界；需要独立服务端Key及适用的内容暂存配置，默认关闭Google内容持久化。
- 轨迹详情及所有轨迹列表摘要可选返回只读 `primary_scenic_spot={id,name}`，入口 `internal/service/track_scenic_summary.go`。通过 `TrackPlaceIndexReader.FindPlaceGeoIndexes` 按页批量核对源摘要与规则，再读取共享地点；只下发 matched 主归属，无结果或资料不可用则省略，绝不把 nearby 当主景区、不逐条查询或在 API 请求中调用供应商。响应装饰不持久化到轨迹表，协议见 `docs/api/track.md`；Android 详情在现有标签行最后直接显示景区名称。
- 景区资料、查询缓存、轨迹关联及游标/租约存于 `track_place_records`，每日北京时间预算存于 `track_place_api_usage`；MySQL/Mongo/内存三实现均在 `internal/repository/track_place*.go`。只有后台任务调用供应商；地图接口和每日路线组重建只读取本地记录。高德轨迹 matched/unmatched 成功结果及共享地点资料一次完成、长期复用，兼容旧版已成功记录，不因时间到期重复外呼；只有 GPS 源摘要或主动升级规则变化才失效。请求级缓存、暂停记录和 Google 内容仍按 TTL 清理，绝不能误删高德永久资料；错误和额度耗尽分别退避，重启不清零预算。路线组重建批量恢复，增量写area_id只允许代表轨迹仍一致。`area.id` 支持稳定 `place-amap-*` ID；未命中继续区县/网格，地图气泡bbox是路线范围而非景区边界。配置/人工验收见 `docs/track-place-discovery.md`。判定规则 v2 与证据版本 v1 分离，源摘要/旧点查询 key 不变，旧完整 Relations 可直接重判；evidence_complete 保留环内搜索失败前的采样证据。ring 查询共用预算、租约与 TTL，不影响成功结果的一次完成策略。
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

### 关键流程

**存量标题回填**：`cmd/backfill-track-titles` 是一次性 MySQL 工具，操作说明见 `docs/backfill-track-titles.md`，生成逻辑在 `internal/service/track_title_backfill.go`。默认只处理未删除、已结束轨迹的时间型默认标题（包括旧版 • 分隔符），优先复用投稿标题；用户明确要求移除日期前后缀时，以 -include-dated-titles 扩大计划范围，保留已有路线描述。缺少具体地点时按距离和运动类型生成，不再使用日期兜底；没有完整轨迹点时不得臆造环线或终点地址。默认只生成计划，应用时必须匹配计划数据库、作者和原始标题，只更新标题及更新时间，不修改投稿审核状态、资源、统计和索引。不得降级为内存仓储或在回填时自动建表。

**启动流程**（`cmd/server/main.go`）：
```
Load Config → 选择 Repository（Memory/MySQL/Mongo，失败降级为 Memory）
→ 构造 Service（Track / TrackSubmission / Recommendation / User / Login / OSSToken / AssetCache）
→ 将 OSSTokenService 作为 downloader 注入 AssetCache
→ （可选）加载 TLS 证书
→ RegisterRoutes(Hertz, Deps)
→ （可选）SCHEDULER_ENABLED=true 时启动 Scheduler（注册 danmaku_cleanup、companion_session_autoclose、track_map_index、track_route_group、recommendation_* 等任务）
→ h.Spin()
```

**推荐第一版流程**：
```
GET /api/v1/track/recommend/list
  → RECOMMENDATION_ENABLED=false：原 TrackService Legacy 响应
  → 新 Feed：RecommendationService 读取画像/物料统计
  → 城市硬过滤 + 排除本人 → 内容偏好/城市热度/关注作者/质量/探索打分
  → 已收藏/已导航强降权 → RouteGroup/作者/运动类型多样性重排
  → 不足时 Legacy 补位 → 固化最多 200 条、TTL 60 分钟的 Feed Session
  → 后续游标只读取 Session；批量加载 Track，失效内容向后扫描并保留原始 recommend_rank
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

**轨迹投稿流程**：
```
POST/PUT /api/v1/track/:track_id/submission（业务 JWT + 账号限制）
  → TrackSubmissionService 校验轨迹归属、完成状态和结构化路线资料
  → TrackSubmissionRepository 原子写入当前投稿、可选图片和事件流水
  → 管理后台 /admin/api/track-submissions/:submission_id/review 审核
  → approved 投稿在推荐列表当前分页窗口内优先，并成为 RouteGroup 代表轨迹候选
  → POST /api/v1/track/:track_id/submission/withdraw 转为 withdrawn 并取消资格
```
投稿状态为 `pending/approved/rejected/withdrawn/invalidated`；重投增加 `revision`，管理审核使用 `expected_revision` 防止审核旧版本。离线聚合仍先用 medoid 计算中心、半径、区域和相似度，再按“存量人工代表 > approved 投稿中与 medoid 最相似者 > medoid”选择展示代表；自动选择投稿代表不得修改路线介绍稳定锚点。

**首页地图索引流程**：
```
track/create(is_running=false) 或 track upload/update 完成轨迹
  → TrackService 调用 TrackMapIndexService.EnqueueTrackIndexIfEligible
  → TrackMapRepository 写入 track_map_index_jobs(status=pending)
  → Scheduler(track_map_index，默认 TRACK_MAP_INDEX_CRON=@every 10m)
  → 补偿扫描已完成但缺少 track_geo_indexes 的轨迹
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

**短信登录等级信息**：`POST /api/v1/login/sms` 成功响应会附带 `achievement_level`，由 `LoginHandler` 调用 `AchievementService.GetLevelInfo` 基于当前有效轨迹实时计算；修改登录响应或等级字段时同步更新 `login.md`。

**关注/粉丝关系流程**：`POST /api/v1/user/:user_id/follow` 与 `DELETE /api/v1/user/:user_id/follow` 使用当前 JWT 用户作为关注者，写入或删除 `user_follows(follower_user_id, followee_user_id)` 单向关系；禁止关注自己，重复关注/取消关注按幂等成功处理。`GET /api/v1/user/:user_id/detail` 可查看任意用户公开主页，只有查看自己时返回手机号、客户端语言和当前生效的账号限制 `account_restriction`；详情、关注列表、粉丝列表会返回 `following_count`、`follower_count` 与当前登录用户视角的 `is_following`。用户详情还会返回公开成就摘要 `achievement`，包含当前等级、已获得勋章总数和最近 3 个勋章，查看自己和查看他人都返回。调整关注关系字段、计数口径、主页隐私字段、用户详情账号限制/成就字段或分页游标时，同步更新 `docs/api/user.md` 与 `docs/api/route-index.md`。

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
  → middleware.AccountRestriction（仅拦截上传/发起同行/关注/收藏/修改资料等受限动作）
  → handler.<Xxx>Handler（参数校验 + 响应封装）
  → service.<Xxx>Service（业务编排 + 资源缓存下发）
  → repository.<Xxx>Repository（数据持久化）
```

**账号限制流程**：管理中心/运维通过 `/api/v1/ops/users/:user_id/restrictions*` 写入或解除 `user_account_restrictions`；当前生效条件为 `status=active AND (expires_at IS NULL OR expires_at > now)`。业务 JWT 通过后，`AccountRestrictionMiddleware` 只拦截上传内容（含上传用 OSS STS、轨迹创建/上传/更新）、发起同行、关注、收藏、点赞、修改个人信息；取消点赞、取消收藏、取消关注、退出登录和查询类接口放行。限制错误返回 `403`，`message` 使用“账号已被限制，禁止...”动作文案。新增会产生内容、关系或用户资料变更的业务接口时，需要评估是否加入 `internal/middleware/account_restriction.go` 的受限路由表，并同步更新 `docs/api/account-restriction.md` 与 `docs/api/route-index.md`。

**意见反馈流程**：`POST /api/v1/feedback` 使用 `multipart/form-data` 提交文字与最多 3 张图片；`FeedbackService` 会先检查同一用户未处理反馈数量，`pending` + `processing` 最多 5 条，超限返回 429 且不会写入图片。通过校验后再检查图片真实类型和大小，并私有落盘到 `<LogDir>/feedback/images/<user_id>/<yyyyMMdd>/`，数据库 `user_feedbacks.images_json` 只保存相对路径和元信息。用户历史列表/详情只返回本人的反馈，图片读取必须经业务 JWT 校验归属；`/ops/feedback/*` 使用 `X-Internal-Token` 供运营查看和更新状态，更新为 `resolved` 时 `reply` 必填且会下发给提交用户。调整反馈字段、图片限制、状态枚举、未处理上限、运营回复规则或访问路径时，同步更新 `docs/api/feedback.md` 与 `docs/api/route-index.md`。

**资源缓存与静态发布包**：客户端经 OSS 直传（头像 / 轨迹截图 / 无地图背景轨迹截图 / 同行轨迹截图 / 原始轨迹文件），列表/详情/同行历史/附近房间接口返回时由 `AssetCacheService` 按需从 OSS 拉回本地 `<LogDir>/static/<category>/`，对外走 `GET /api/v1/static/<category>/<file>`（需登录）；无地图背景轨迹截图使用 `<track_id>_no_map_bg` 作为缓存 key，`/track/my/list` 等轨迹列表返回 `track_no_map_bg_screenshot_url`。管理后台上传的 App 发布包直接写入 `<LogDir>/static/release/<platform>/`，对外走公开的 `GET /api/v1/static/release/<platform>/<file>`，供升级下载使用。

**静态轨迹预览图**：`Track.PreviewImages` / `preview_images` 保存客户端上传的 1:1、4:3 日夜四张图片，MySQL 使用可空 JSON 列，Mongo 和内存使用值对象；创建/更新字段可选，提供时须四图完整，更新整组替换。正常 Android 上传使用长宽各为窗口基准 1/4 的 WebP 85，现有 AssetCacheService 已允许 `.webp`，旧 JPEG 保持兼容；独立历史补图仍校验固定尺寸 JPEG，不能把该约束套到普通创建/更新。Track 及所有摘要统一返回带源版本的 `/api/v1/track/:track_id/previews/:variant?v=...`，列表只构造 URL，不同步拉取四图。入口见 `internal/service/track_preview.go` 与 `internal/handler/track_preview_handler.go`；图片复用 AssetCacheService 按需下载，使用轨迹归属用户的 OSS 凭证，缓存位于 `<LogDir>/track_previews/`，不挂通用 static；每次读取校验轨迹可见范围和源版本。协议与路由变更同步 `docs/api/track.md`、`models.md`、`route-index.md`。

**内置 H5 页面**：客户端等级规则页使用公开路由 `GET /api/v1/achievement/level-rules.html`，HTML 文件内置在 `internal/handler/static/achievement_level_rules.html` 并通过 `go:embed` 打包；页面支持 `lang=english` 切英文、`is_dark=true` 切夜间模式。修改等级 XP 规则、等级阈值、语言或主题参数时，同步更新该页面、`track_achievement_client.md` 和 `docs/api/achievement.md`。

**历史轨迹预览补齐**：DevMode 客户端经业务 JWT 调用 `/track/preview-backfill/list`、`/:track_id/data`、`/:track_id/prepare`、`/:track_id/complete`；按用户决定不增加管理员或作者权限校验，待补作用域为全站已结束、未删除、有原始轨迹且四图不完整的记录。入口为 `internal/service/track_preview_backfill*.go`，复用原始轨迹解析器返回完整 GPS，绘图仍由 Android 完成。prepare 仅签发对应作者目录、独立 upload_id 下四个对象的 PUT URL；complete 经 OSS 内网核对 JPEG 与固定尺寸，再调用 `TrackRepository.FillMissingPreviewImages` 条件写入四图，防止覆盖期间新上传的完整图片或不同源数据。`POST /track/preview-backfill/clear` 必须显式选择 `all=true` 或单个 `track_id`，清空未删除轨迹的四图地址，不删除 OSS 文件或 GPS；`ClearPreviewImages` 在既有 JSON/BSON 写入不对外暴露的清空版本，source_version 和条件写入均包含此版本，禁止旧任务重新写回。三种仓储实现位于 `internal/repository/track_preview_backfill.go`；无新表/列，完成状态以持久化四图为准。账号限制沿用上传规则，接口文档见 `docs/api/track.md`。

隐私政策与用户服务协议使用公开路由 `GET /api/v1/legal/privacy-policy.html` 和 `GET /api/v1/legal/terms-of-service.html`，由 `legal_handler.go` 嵌入 `static/privacy_policy.html`、`static/terms_of_service.html`。支持 `lang=english`、`is_dark=true/false`，未指定主题时跟随系统。草稿中的 `{xxxx}` 须按运营事实补齐；数据采集、公开、SDK、保存或删除行为变化时同步核对中英文正文及真实授权流程。接口与内容维护见 `docs/api/legal.md`。

地图景区/区县介绍页使用公开路由 `GET /api/v1/track-map/areas/:area_id/introduction.html`，统一模板位于 `internal/handler/static/map_area_introduction.html`；生成区县基线位于 `internal/maparea/districts.json`，人工景区/区县覆盖位于 `catalog.json`，页面支持 `lang=english` 与 `is_dark=true`。新增或调整区域时必须使用 GCJ-02 坐标、保持 `area.id` 稳定，并同步核对 `docs/api/track-map.md` 与 `track_map.md`。

- 评论图片每条最多一张，发布参数 image_ids 数组长度不能超过 1；客户端系统单选、输入区内预览和移除，服务端独立校验数量。

**动态纠错配置**：`internal/service/feedback_correction.go` 提供服务端默认选项、配置校验和提交快照；业务 `GET /feedback/correction-options` 仅下发启用项，后台 `/admin/api/feedback-correction-config` 全量读取/按 version 条件保存。`FeedbackRepository.GetCorrectionConfig/SaveCorrectionConfig` 在 `internal/repository/feedback_correction.go` 实现三种仓储，MySQL 启动补建 `feedback_correction_config` 与 `user_feedbacks.correction_json`。客户端提交稳定 option ID 和 track ID，名称与真实轨迹标题由服务端读取并保存快照；改名/停用不能改写历史反馈，停用项提交返回 409。仅配置不存在时使用服务端默认值，全停用不能恢复默认；客户端不得硬编码业务选项。协议见 `docs/api/feedback.md`，后台入口见 `internal/admin/AGENTS.md`。

### 提交前最小检查
- 构建可通过：`go build ./...`
- 仅用户明确要求测试时执行：`make test`（等价 `go test ./...`），或按要求缩小到相关包。
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
