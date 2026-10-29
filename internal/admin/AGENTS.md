# AGENTS.md — internal/admin

> 本文面向 AI Coding Agent。仅记录 admin 模块的稳定约束、改动导航与提交前最小检查。
> 通用约束、目录总览、降级链等见仓库根目录 `AGENTS.md`，本文不重复。

## 模块定位

`internal/admin` 是 **track_server 的内置管理后台**，提供：

- 一个独立登录的 Web 控制台（HTML/CSS/JS 经 `go:embed` 内置）；
- 围绕 `AppRelease` 的发布、列表、删除接口；
- 上传安装包到服务端本地静态目录的接口；
- 通过后台会话代理读取服务端静态资源（如用户头像），避免复用业务 JWT；
- 查看和处理用户意见反馈（含图片预览、状态更新、用户可见反馈意见）。
- 查看埋点 OSS 同步摘要（任务状态、文件列表、OSS key、字节数、耗时与错误）。
- 在用户管理页查看、创建和解除账号限制；账号限制会由业务侧中间件拦截上传、发起同行、关注、收藏、修改资料等动作。
- 查看和人工运营路线发现 RouteGroup（展示离线 `area_id` 对应的区域名称、类型与介绍页，支持改名、合并、移除成员、指定代表轨迹，以及编辑、发布和下线双语路线介绍）。
- 通过响应式工作台查看业务库即时统计：日/周/月核心增长、同周期新增用户转化、投稿与反馈处理效率、轨迹内容质量、互动趋势、热门轨迹和同行会话；该口径不依赖 T+1 埋点同步。
- 审核用户轨迹投稿，查看结构化路线资料和经本地缓存改写的投稿图片，按 revision 通过或驳回。
- 管理后台内置静态资源响应支持 gzip 与 ETag 重新验证；HTML 使用 no-cache，其他嵌入资源需重新验证后复用浏览器缓存。工作台先展示 API 返回的核心数字，再异步加载 ECharts 绘制图表。
- 投稿审核页顶部展示当前待审核与超时队列，以及本月通过、驳回、通过率和平均审核时长；复用 `/admin/api/dashboard?period=month` 的只读统计。
- 查看、修正和删除用户轨迹；列表按 ID、城市、标题、运动类型、用户名、轨迹图、状态和操作展示，支持标题及用户名模糊搜索、城市和状态筛选，并保留游标分页。用户名同时展示头像；景点详情中的相关轨迹列表复用相同列顺序。列表不单独放删除按钮；编辑轨迹使用共享 `static/track_editor.js` 表单，底部删除动作需二次确认，删除为软删除并同步清理关联数据。
- 反馈与纠错页默认筛选待处理，展示状态统计、反馈用户名称和完整内容；点击查看会在弹窗中加载详情并处理反馈。`static/feedback_correction.js` 优先读取服务端 `correction` 快照，旧记录兼容完整的六种中英文纠错正文；不识别的内容仍按普通反馈显示，不改写原始正文。页面「纠错选项」可新增、改名、排序、启停，由 API 下发客户端。
- 用户管理按昵称、同行管理按会话标题、轨迹管理按标题和用户名提供服务端模糊搜索；轨迹列表还支持城市和状态筛选，均保留游标分页。

模块对外只暴露 `admin.NewModule(...)` 与 `Module.RegisterRoutes(h)`，不被业务 handler 引用。

## 硬性约束

- **独立鉴权链路**：admin 走自有的 `Authenticator` + `SessionStore`（cookie 名 `admin_session`），**不要复用** `internal/middleware.JWTAuthMiddleware` 也不要在 admin 路由里塞 `RequestMeta` 中间件。业务用户与管理员是两套体系。
- **凭证来源唯一**：管理员账号只来自环境变量。支持两种格式：
  - **推荐**：`ADMIN_ACCOUNTS=user1:bcryptHash1;user2:bcryptHash2`（多账号，`;` / `,` 分隔，首个 `:` 分割 username 与 hash）。
  - **兼容**：旧单账号 `ADMIN_USERNAME` / `ADMIN_PASSWORD_HASH`。两者同时存在时合并，同名用户以 `ADMIN_ACCOUNTS` 为准。
  - `deploy/docker-compose.yml` 在未提供覆盖值时使用 `admin` / `admin`（密码以 bcrypt hash 注入）；生产部署如需更换，通过环境变量覆盖默认值。
  - **禁止把管理员信息写入数据库**，避免和业务用户表交叉。
- **未配置即禁用**：上面两种来源解析出的账号集为空时，`NewModule` 返回的 `Module.Auth` 为 `nil`，`RegisterRoutes` 直接不挂载任何 `/admin/*` 路由——这是"后台未启用"的唯一信号，新增功能不要绕过这个开关。
- **会话仅内存**：`SessionStore` 使用 `sync.RWMutex` + `map`，进程重启即失效。**不要替换为持久化存储**；管理员重新登录是合理代价。
- **静态资源经 `go:embed`**：`static/` 目录通过 `//go:embed static` 内置进二进制；新增前端文件后**必须放到** `internal/admin/static/`，否则二进制找不到。`/admin/static/*filepath` 路由只读 `staticFS`，不读磁盘。
- **安装包本机上传**：管理后台安装包上传走 `POST /admin/api/releases/upload-package`，服务端校验 APK/IPA 并读取包内版本名、版本号/Build、应用标识，同时计算大小和 MD5；前端使用返回的下载地址自动填写 `package_url`。不要再把发布包上传链路改回浏览器 OSS 直传。
- **路径前缀固定 `/admin`**：所有路由都注册在 `h.Group("/admin")` 之下；前端 `fetch` 也写死 `/admin/api/...` 与 `/admin/static/...`。修改前缀需要同时改 `routes.go` 与 `static/*` 中的 URL。

## 目录与权威文件

```
internal/admin/
├── auth.go         # Authenticator + bcrypt 校验 + cookie + AuthMiddleware
├── session.go      # SessionStore (内存 + GC 协程，TTL 默认 12h)
├── handlers.go     # /admin/api/releases*、/admin/api/feedbacks*、/admin/api/route-groups*、/admin/api/track-submissions*、安装包上传、列表查询
├── routes.go       # NewModule / RegisterRoutes / go:embed static
└── static/
    ├── login.html  ├── login.js
    ├── index.html  ├── app.js
    └── style.css
```

| 主题 | 权威文件 |
| --- | --- |
| 路由清单 | `routes.go` 中 `RegisterRoutes` |
| 鉴权与 cookie | `auth.go`（`sessionCookieName = "admin_session"`） |
| 业务依赖注入 | `routes.go` 的 `NewModule(accounts, releaseSvc, stsSvc, staticRoot, userRepo, trackRepo, collectRepo, trackMapRepo, ..., restrictionSvc, routeGroupSvc)`；`cmd/server/main.go` 创建后通过 `Handler.SetScreenshotCache(screenshotCache)`、`Handler.SetTrackSubmissionService(trackSubmissionSvc)` 和 `Handler.SetDashboardStatisticsRepository(...)` 注入缓存、投稿与跨域只读统计能力 |
| 前端入口 | `static/login.html`、`static/index.html` |
| 响应式外壳与工作台 | `static/shell.js`、`static/ui.js`、`static/style.css`、`static/dashboard.html`、`static/dashboard.js`；组件库固定为内置 Tabler 1.7.1，图标为 Tabler Icons 3.49.0，图表为 ECharts 6.1.0，均位于 `static/vendor/`，不要改成运行时 CDN |
| 安装包上传 | `handlers.go` 中 `UploadPackage`、`static/app.js` 中 `/admin/api/releases/upload-package` |
| 用户与账号限制 | `handlers.go` 中 `ListUsers` / `CreateAccountRestriction` / `GetCurrentAccountRestriction` / `RevokeCurrentAccountRestriction`，`static/users.html`、`static/users.js` |
| 意见反馈管理 | `handlers.go` 中 `ListFeedbacks` / `GetFeedback` / `UpdateFeedbackStatus` / `GetFeedbackImage`，`static/feedbacks.html`、`static/feedbacks.js` |
| 纠错配置 | `feedback_correction.go` 中配置读写 handler；`static/feedback_correction_config.js` 编辑配置；校验、快照和默认值统一在 `service/feedback_correction.go` |
| 埋点同步摘要 | `handlers.go` 中 `ListAnalyticsSyncSummaries`，`static/analytics.html`、`static/analytics.js` |
| 聚合路线运营 | `handlers.go` 中 `ListRouteGroups` / `GetRouteGroup` / `RenameRouteGroup` / `MergeRouteGroup` / `RemoveRouteGroupMember` / `SetRouteGroupRepresentative`，`static/route_groups.html`、`static/route_groups.js` |
| 轨迹投稿审核 | `handlers.go` 中 `ListTrackSubmissions` / `GetTrackSubmission` / `ReviewTrackSubmission`，统计复用 `GetDashboard`；页面位于 `static/track_submissions.html`、`static/track_submissions.js` |
| 主景点目录 | `scenic_spot_handlers.go` + `ScenicSpotCatalogService`；页面 `static/scenic_spots.html`、`static/scenic_spots.js`；目录与调用统计由 `repository.ScenicSpotRepository` 三种实现提供；自动/人工记录均直接下发客户端，归档/合并记录隐藏；首次加载时分批同步存量供应商资料；新建时可点击地图 POI 热点或搜索附近 POI，将资料先放入备选预览，一键代入后写入供应商来源与资料/图片；详情以弹窗编辑并可拖动排序图片 |
| 轨迹编辑弹窗 | `static/track_editor.js` 提供共享编辑表单，轨迹管理页与景点相关轨迹列表共用；保存或删除后调用页刷新各自列表 |

## 路由清单（与 `routes.go` 对齐）

| 方法 | 路径 | 鉴权 | 说明 |
|------|------|------|------|
| GET | `/admin/`、`/admin` | 公开 | 302 跳转 `/admin/login.html` |
| GET | `/admin/login.html` | 公开 | 登录页 |
| GET | `/admin/index.html` | 会话 | 管理首页，服务端返回 HTML 前校验会话 |
| GET | `/admin/dashboard.html` | 会话 | 响应式运营工作台，服务端返回 HTML 前校验会话 |
| GET | `/admin/more.html` | 会话 | 移动端低频功能入口，服务端返回 HTML 前校验会话 |
| GET | `/admin/route_groups.html` | 会话 | 聚合路线运营页，服务端返回 HTML 前校验会话 |
| GET | `/admin/track_submissions.html` | 会话 | 轨迹投稿审核页，服务端返回 HTML 前校验会话 |
| GET | `/admin/static/*filepath` | 公开 | 内置静态资源 |
| POST | `/admin/api/login` | 公开 | bcrypt 校验，成功后下发 `admin_session` cookie |
| POST | `/admin/api/logout` | 公开 | 删除 session + 清 cookie |
| GET | `/admin/api/me` | 公开 | 返回当前会话；未登录返回 401 |
| GET | `/admin/api/releases` | 鉴权 | 列出发布记录，支持 `platform` / `status` 过滤 |
| GET | `/admin/api/dashboard` | 鉴权 | 业务库即时统计；`period=day|week|month`，返回同进度上周期对比、用户转化、审核/反馈效率、内容质量、互动、同行与日趋势 |
| GET | `/admin/api/users/statistics` | 鉴权 | 用户总数、性别/星座分布和已完成轨迹涉及城市的去重用户数（一个用户可计入多个城市） |
| GET | `/admin/api/companions/statistics` | 鉴权 | 同行会话当前活跃数、所选周期创建/结束/自动结束数、平均参与人数 |
| POST | `/admin/api/releases` | 鉴权 | 发布/覆盖一个版本（按 `(platform, version_code)` 幂等） |
| DELETE | `/admin/api/releases/:id` | 鉴权 | 删除一条发布记录 |
| POST | `/admin/api/releases/upload-package` | 鉴权 | multipart 上传安装包到 `<LogDir>/static/release/<platform>/` |
| GET | `/admin/api/releases/upload-token` | 鉴权 | 旧 OSS 直传 STS 凭证接口，前端不再使用 |
| GET | `/admin/api/static/*filepath` | 鉴权 | 后台静态资源代理，从 `<LogDir>/static/` 读取头像等资源；后台页面不要直接访问需业务 JWT 的 `/api/v1/static/*` |
| GET | `/admin/api/users` | 鉴权 | 用户列表，支持 `title` 按昵称模糊搜索及 cursor 翻页 |
| GET | `/admin/api/users/:user_id/restrictions/current` | 鉴权 | 查询当前账号限制；无生效限制返回 404 |
| POST | `/admin/api/users/:user_id/restrictions` | 鉴权 | 创建账号限制；`expires_at` 为空表示永久，操作人取 admin session 用户名 |
| DELETE | `/admin/api/users/:user_id/restrictions/current` | 鉴权 | 解除当前账号限制 |
| GET | `/admin/api/companions` | 鉴权 | 同行会话列表，支持 `title` 模糊搜索及 cursor 翻页 |
| GET | `/admin/api/tracks` | 鉴权 | 轨迹列表，支持 `title` / `user_name` 模糊搜索、`city_code` / `status` 筛选及 cursor 翻页 |
| GET | `/admin/api/cities` | 鉴权 | 返回内置城市/省份 Code、名称与省市层级，供轨迹和景点选择器使用 |
| GET | `/admin/api/tracks/:track_id` | 鉴权 | 返回轨迹详情用于后台编辑 |
| PUT | `/admin/api/tracks/:track_id` | 鉴权 | 更新轨迹业务字段（不改 ID、轨迹点与创建/更新时间）；城市/类型更新已有地图索引，状态/进行状态/原始文件变化时清理并按可索引条件重新排队 |
| DELETE | `/admin/api/tracks/:track_id` | 鉴权 | 删除轨迹：标记 `status=0`，并清理收藏、地图索引任务、geo index、路线组成员 |
| GET | `/admin/api/feedbacks` | 鉴权 | 意见反馈列表，补充用户名称，支持 `status` / `app_version` / `phone` / `cursor` / `limit` |
| GET/PUT | `/admin/api/feedback-correction-config` | 鉴权 | 读取含停用项的完整配置，按 version 条件保存；冲突返回 409 |
| GET | `/admin/api/feedbacks/:feedback_id` | 鉴权 | 意见反馈详情 |
| PUT | `/admin/api/feedbacks/:feedback_id/status` | 鉴权 | 更新反馈处理状态与用户可见 `reply`；`resolved` 时 `reply` 必填 |
| GET | `/admin/api/feedbacks/:feedback_id/images/:image_id` | 鉴权 | 读取反馈图片 |
| GET | `/admin/api/analytics/sync-summaries` | 鉴权 | 埋点 OSS 同步摘要列表，支持 `status` / `limit` / `offset` |
| GET | `/admin/api/route-groups` | 鉴权 | 路线组列表，支持 `track_type` / `city_code` / `limit`；返回 `area_id` 及目录区域摘要 |
| GET | `/admin/api/route-groups/:group_id` | 鉴权 | 路线组详情、`area_id` 对应区域信息与成员轨迹 |
| PUT | `/admin/api/route-groups/:group_id/name` | 鉴权 | 人工改名 |
| POST | `/admin/api/route-groups/:group_id/merge` | 鉴权 | 将 `source_group_id` 合并到当前路线组，并归档源路线组 |
| DELETE | `/admin/api/route-groups/:group_id/members/:track_id` | 鉴权 | 从路线组移除成员轨迹 |
| PUT | `/admin/api/route-groups/:group_id/representative` | 鉴权 | 指定代表轨迹 |
| GET/PUT | `/admin/api/route-groups/:group_id/introduction` | 鉴权 | 查询或保存结构化双语路线介绍 |
| POST | `/admin/api/route-groups/:group_id/introduction/publish` | 鉴权 | 发布路线介绍 |
| POST | `/admin/api/route-groups/:group_id/introduction/unpublish` | 鉴权 | 下线路线介绍 |
| GET | `/admin/api/track-submissions` | 鉴权 | 投稿审核列表，支持状态、难度、风险、运动类型、用户过滤 |
| GET | `/admin/api/track-submissions/:submission_id` | 鉴权 | 投稿、轨迹和审核流水详情 |
| POST | `/admin/api/track-submissions/:submission_id/review` | 鉴权 | 按 expected_revision 通过或驳回投稿 |

## 关键流程

**登录**：
```
POST /admin/api/login {username,password}
  → Authenticator.Verify (constant-time username + bcrypt password)
  → SessionStore.Create → Set-Cookie admin_session=<32B hex>; HttpOnly; SameSite=Lax
```

**前端发布 APK（Android）**：
```
[admin UI] 选 APK
  → POST /admin/api/releases/upload-package multipart(file, platform=android) (鉴权)
  → 服务端写入 <LogDir>/static/release/android/<ts>-<safe-file>.apk
  → 返回 /api/v1/static/release/android/<ts>-<safe-file>.apk 并填进 package_url
  → POST /admin/api/releases   (鉴权)
       → AppReleaseService.Publish → AppReleaseRepository.Upsert
```

**鉴权拦截**：
```
请求 → Authenticator.AuthMiddleware → SessionFromRequest(cookie) → 否则 401
```

**运营工作台统计**：
```
[admin UI] /admin/dashboard.html?period=day|week|month
  → GET /admin/api/dashboard?period=...
  → Handler 生成 current / previous / trend 时间窗
  → AdminDashboardStatisticsRepository 从当前仓储族只读聚合
       用户与活跃、完成轨迹、投稿审核、反馈、互动、地图索引、路线组、同行
  → 服务端补充运动类型、城市和状态展示名
  → ECharts 渲染增长/互动趋势、转化漏斗和内容分布
```
新增用户转化漏斗以同周期注册用户为 Cohort，后续阶段逐级限定为同周期完成轨迹、投稿和审核通过；收藏/导航/点赞/评论的周期指标按各自关系或记录的 `created_at` 统计，不使用轨迹当前累计数倒推历史趋势。工作台聚合只读，不得在加载页面时修正业务数据或触发外部服务。

**投稿审核统计**：
```
[admin UI] /admin/track_submissions.html
  → GET /admin/api/dashboard?period=month
  → 展示当前 pending 与超时数，以及当月通过/驳回/通过率/平均审核时长
  → 审核成功后重新读取统计和当前列表
```

**后台头像读取**：
```
GET /admin/api/users
  → UserService.DecorateAvatar 把头像改写为 /api/v1/static/...
  → AccountRestrictionService.FindActive 为每个用户补当前账号限制状态
  → admin handler 再改写为 /admin/api/static/...
  → 浏览器 <img> 请求携带 admin_session
  → GET /admin/api/static/*filepath 经 AuthMiddleware 后从 <LogDir>/static/ 读取
```
未落盘的内置默认头像（`default_avatars/girl_01.png` 等）由后台静态代理返回简单 SVG 兜底；正式视觉素材仍应放入 `<LogDir>/static/default_avatars/`。

**账号限制运营**：
```
[admin UI] /admin/users.html
  → GET /admin/api/users 拉取用户列表与当前 account_restriction
  → POST /admin/api/users/:user_id/restrictions {reason, expires_at?}
       → AccountRestrictionService.Create
       → 先撤销该用户现有未过期 active 限制，再写入 user_account_restrictions
  → DELETE /admin/api/users/:user_id/restrictions/current
       → AccountRestrictionService.RevokeActive
```
限制生效后由业务 `/api/v1` 侧 `AccountRestrictionMiddleware` 拦截；admin 页面只负责运营写入和解除。

**处理意见反馈**：
```
[admin UI] /admin/feedbacks.html
  → GET /admin/api/dashboard?period=month     展示状态分布与超时统计
  → GET /admin/api/feedbacks?status=pending&app_version=&phone=&cursor=  默认拉取待处理列表
  → 批量补充用户昵称，不在页面展示反馈 ID / 用户 ID
  → GET /admin/api/feedbacks/:feedback_id     在弹窗中查看内容与图片
  → 轨迹纠错展示问题类型、轨迹名称/ID、可选说明和图片；通过 /admin/tracks.html?track_id=... 新页打开既有轨迹编辑表单
  → 快捷回复仅预填状态和用户可见回复，管理员核查并保存后才更新反馈；反馈状态更新不会自动修改轨迹或景点/行政区归属
  → PUT /admin/api/feedbacks/:feedback_id/status {status, reply}
       → FeedbackService.UpdateStatus
       → resolved 状态要求 reply 必填，reply 会展示给提交用户
```

纠错展示以 `correction` 的提交时快照为准，不根据当前选项名称重建历史记录；旧文字格式仅保留兼容，普通反馈仅提到轨迹 ID 不应被误识别。选项通过后台配置维护稳定 ID，已有项只允许停用而非删除，新增 ID 由服务端生成；并发保存冲突时保留表单，由管理员显式重新加载，不覆盖他人配置。状态统计仍包含全部反馈。轨迹实际修改继续走既有鉴权与保存确认，读取关联轨迹不能自动提交改动。

**查看埋点同步摘要**：
```

**运营聚合路线**：
```
[admin UI] /admin/route_groups.html
  → GET /admin/api/route-groups?track_type=&city_code=
  → GET /admin/api/route-groups/:group_id 查看成员轨迹与 geo index
  → TrackRouteGroupService 按已存 area_id 补区域名称、类型、城市与介绍页链接
  → PUT /admin/api/route-groups/:group_id/name {name}
  → POST /admin/api/route-groups/:group_id/merge {source_group_id}
  → DELETE /admin/api/route-groups/:group_id/members/:track_id
  → PUT /admin/api/route-groups/:group_id/representative {track_id}
  → PUT /admin/api/route-groups/:group_id/introduction 保存中英文介绍
  → POST /admin/api/route-groups/:group_id/introduction/publish|unpublish 发布或下线
       → TrackRouteGroupService 校验运动类型、成员关系与代表轨迹
       → TrackMapRepository 更新 track_route_groups / track_route_group_members
```
路线组列表页展示路线组摘要及离线 `area_id` 对应的区域摘要，详情页展示同样的区域信息与介绍页入口；不得在 admin 请求链路重新做 Polygon/MultiPolygon 匹配。代表轨迹只用于封面/运营参考，RouteGroup 不再保存或下发代表折线，客户端用聚合中心点与 `radius_m` 画区域。
人工改名会把 RouteGroup 标记为 `mixed`，离线重聚合通过历史成员重合复用 GroupID 并保留该名称；不要把名称重新改为城市级默认值。离线任务先按 medoid 计算几何结果，再按“存量人工代表 > approved 投稿候选 > medoid”选择展示代表；人工代表只要仍属于重建后的组就必须保留。

**审核轨迹投稿**：
```
[admin UI] /admin/track_submissions.html
  → GET /admin/api/track-submissions?status=pending
  → GET /admin/api/track-submissions/:submission_id 查看投稿、轨迹与审核流水
  → POST /admin/api/track-submissions/:submission_id/review
       {decision, reason, expected_revision}
       → TrackSubmissionService 按 revision 原子审核并追加事件
```
投稿图片先经 `submission_images` AssetCache 缓存，再把 `/api/v1/static/submission_images/*` 改写为 `/admin/api/static/submission_images/*`；后台响应不得暴露原始 OSS URL。

**管理轨迹**：
```
[admin UI] /admin/tracks.html
  → GET /admin/api/tracks 拉取轨迹列表
       → 展示 preview_images.landscape_day_url（日间横版 4:3），并经后台静态代理访问
       → 用户名通过当前页 user_id 批量读取用户昵称，列表不展示 user_id
  → GET /admin/api/dashboard?period=month 展示本月轨迹类型与城市统计表
  → GET /admin/api/cities 将城市 Code 显示为名称并提供编辑选择器
  → 管理员点击编辑；删除操作位于共享编辑表单底部，并经确认后执行
  → GET /admin/api/tracks/:track_id 获取完整可编辑业务详情
  → PUT /admin/api/tracks/:track_id 更新所选业务字段与预览图；ID、轨迹点和创建/更新时间不由表单修改
  → DELETE /admin/api/tracks/:track_id
       → track_records.status=0，is_running=0，记录/保留 deleted_at
       → 清理 track_collects
       → 清理 track_map_index_jobs / track_geo_indexes / track_route_group_members
       → 若删除的是路线组代表轨迹，归档该路线组并移除其全部成员，后续由 track_route_group 任务重新聚合剩余轨迹
```

**主景点目录**：
```
[admin UI] /admin/scenic_spots.html
  → GET /admin/api/scenic-spots/statistics 汇总客户端可见景点覆盖、近 30 日供应商调用趋势与高调用景点
  → POST /admin/api/scenic-spots/provider-content/backfill 首次加载时游标分批同步供应商文字和图片；已同步的存量景点仅用轻量地点详情补缺失坐标，provider_coordinates_status 防止重复查询
  → GET /admin/api/scenic-spots/:spot_id/map 通过后台鉴权代理返回高德静态地图图片，供应商密钥不下发浏览器
  → GET /admin/api/scenic-spots/map-config 向已登录管理员下发高德 Web Key；AMAP_SECURITY_JS_CODE 仅由同源地图服务代理使用，不下发浏览器
  → /admin/api/scenic-spots 查询、创建/编辑草稿、发布/归档、查看审计
  → POST /admin/api/scenic-spots/:spot_id/provider-sources/verify 校验高德/Google 来源 ID
  → POST /admin/api/scenic-spots/:spot_id/provider-sources/preview 实时获取来源详情；景点目录的存量资料由 provider-content/backfill 分批同步并保存
  → POST /admin/api/scenic-spots/:spot_id/google-description 管理员主动查询 Google Places 简介，优先使用已关联 Google Place ID，否则按景点名、城市和坐标匹配；只返回简介给编辑框，需管理员确认并保存
  → POST /admin/api/scenic-spots/:spot_id/photos 上传后台自有图片
  → /admin/api/scenic-spots/:spot_id/tracks 搜索、关联或解除轨迹
  → POST /admin/api/scenic-spots/:spot_id/merge|split 合并目录或拆分轨迹归属
  → POST /admin/api/scenic-spots/backfill 分页将存量轨迹迁移到内部 SCP ID 草稿，并复用已有主景区匹配缓存中的名称；遇到已关联但名称为空的草稿时尝试恢复该名称。缺少可用来源 ID 时按地图服务配置执行有预算的真实查询
       → ScenicSpotCatalogService + ScenicSpotRepository (MySQL / Mongo / memory)
       → track_scenic_spot_links 关联轨迹，scenic_spot_audit_logs 记录运营操作
       → scenic_spot_provider_usage 只累计景点/provider/日期/操作的请求成功失败计数；同步后的供应商文字与图片引用保存在 scenic_spots 的资料和 photos 数组中
```
轨迹只保存内部景点关联；外部来源 ID 留在景点目录。存量回填对没有可用来源 ID 的合格轨迹强制刷新坐标查询缓存，调用地图 API；每日上限按配置执行（高德默认不设日上限），每轮最多 10 次真实请求。索引刷新失败时回退到已有索引继续匹配并单独计数；结果有明确重复命中才创建空白目录项。后台景点页先展示运营统计，调用榜只显示前三名；供应商调用明细不在页面展示。目录为紧凑整行，展示分类、简介和图片；“相关轨迹”操作通过景点轨迹列表接口，以轨迹管理页相同的表格字段（不含开始时间）展示关联轨迹；点击行或编辑按钮打开共享 `static/track_editor.js` 可编辑表单，保存或删除后刷新关联列表。景点详情在弹窗内编辑文字资料和图片，并显示供应商坐标、坐标系及位置地图。供应商文字同步到景点 JSON 资料，图片与人工上传图片共用 `photos` 有序数组；拖动排序和上传追加顺序都会随保存结果下发。Google 坐标按 WGS84 保存，高德坐标按 GCJ02 保存；来源同步只填补空坐标，不覆盖既有坐标，已同步记录单独请求坐标字段，避免再次请求照片媒体。城市信息只读展示，不作为编辑字段。`manual_edited` 表示资料由管理员创建或编辑保存；自动同步资料仍显示“自动”。页面隐藏来源 ID、轨迹关联编辑、合并拆分操作，但提供相关轨迹列表和编辑入口；其余数据/API仍由服务端保留，供客户端景点关联及维护流程使用。景点仍沿用内部 draft/published/archived/merged 状态机；页面展示的“自动/人工”是内容维护方式，不替代发布状态。未发布目录不会覆盖旧轨迹摘要。后台上传图像走受鉴权的 `/api/v1/static/scenic_spots/*` 资源路由，Android 详情页可读取。更新 MySQL/Mongo/memory 任一仓储时，保持三种实现、版本冲突控制和审计行为一致。
景点页统计开始于该计数能力上线，不回填旧请求；计入已关联内部景点的来源详情、照片请求和 ID 核验，存量轨迹需先运行回填，按坐标执行的识别搜索不计入单景点排行。发布覆盖率以关联轨迹计数为准，目标是帮助运营优先补齐高调用景点的自有内容。
高德来源景点简介旁的 Google Maps 查询按钮使用未缓存的 Places 查询，不受后台自动处理的每日预算限制，由 Google API 自身配额控制；有已关联 Google Place ID 时直接取该地点简介，否则用名称、城市和坐标匹配候选并校验名称与 5km 距离。请求仍记录到供应商调用统计。查询不会自动更新目录，管理员确认输入框内容后仍需手动保存。
[admin UI] /admin/analytics.html
  → GET /admin/api/analytics/sync-summaries?status=&limit=&offset=
  → AnalyticsRepository.ListSyncSummaries / CountSyncSummaries
  → 页面展示任务状态、扫描/上传/失败文件数、总字节数、OSS 前缀与 files_json 文件明细
```

## 改动导航

- **加一个管理后台 API**：在 `handlers.go` 或按业务域拆分的 handler 文件写 handler → `routes.go` 的 `api := g.Group("/api", m.Auth.AuthMiddleware())` 下挂载。需要操作者信息时用 `h.auth.SessionFromRequest(c)`。
- **展示业务静态资源**：后台页面不要直接使用 `/api/v1/static/*`，应由 handler 改写为 `/admin/api/static/*`，通过 admin session 鉴权后读取 `staticRoot`。
- **加一个公开页面/资源**：把文件放进 `static/`，在 `routes.go` 用 `serveEmbedded(...)` 或走 `/admin/static/*filepath`。**不要新增第二个 `go:embed`**。
- **加一个侧边/顶部入口**：统一修改 `static/shell.js` 的导航配置；页面只需在自身脚本前加载 `shell.js`，不要再复制一份导航。
- **后台组件样式**：页面先加载内置 `vendor/tabler.min.css`，再加载 `style.css`；通用 Card / Button / Form / Table / Badge 和移动端表格卡片化由 `static/ui.js` 统一增强，新增页面应加载 `vendor/tabler.min.js`、`shell.js`、`ui.js` 后再加载页面脚本，不要另写一套原生组件外观。
- **改 cookie 策略**：只在 `auth.go` 改；保持 `HttpOnly=true`，跨站调用前提下才考虑 `SameSite=None + Secure`。
- **调整 session TTL**：改 `routes.go` 中 `NewSessionStore(12 * time.Hour)`，不要在 cookie 与 store 两边写不一致的过期时间。

## 提交前最小检查

- 构建：`go build ./...`
- 测试遵循根 AGENTS 的“默认验证方式”：默认不新增或运行，由用户手动验证；仅在用户明确要求时执行 `go test ./internal/admin ./internal/service` 或其指定范围，不自动扩大为全量测试。
- 路由变更：核对 `routes.go` 与 `static/*.js` 中前端 fetch 的 URL 是否一致。
- 静态资源新增：确认文件位于 `internal/admin/static/` 下（`go:embed static` 才能打包）。
- 接口契约变更：同步更新 `docs/api/feedback.md` 与 `docs/api/route-index.md`；`track_api.md` 仅保留轻量入口。

## 常见误解

- **误解**：管理后台没访问到，往业务 `auth` 组里塞路由。
  **事实**：admin 完全独立挂在 `/admin`，不属于 `/api/v1/auth` 组；登录态也是 cookie，不是 JWT。
- **误解**：可以用 `OSSTokenService.GetUploadCredential` 上传 APK。
  **事实**：那是按 userID 隔离的用户目录，policy 不允许写入 `release/`，必须用 `GetReleaseUploadCredential`。
- **误解**：进程重启后会话还在。
  **事实**：`SessionStore` 是纯内存实现，重启即清空，必须重新登录。
- **误解**：管理员账号在数据库里。
  **事实**：账号只来自 `ADMIN_ACCOUNTS` / `ADMIN_USERNAME` / `ADMIN_PASSWORD_HASH` 环境变量，bcrypt 校验，不查任何表；Docker Compose 提供 `admin/admin` 默认值，可通过环境变量覆盖。
