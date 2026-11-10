# 管理后台功能流程与代码入口

从原 AGENTS.md 迁出的开发参考；修改对应功能前按 AGENTS 索引读取相关章节，无需每次通读。验证、授权和通用行为以当前 AGENTS.md 为准；实现细节以源码和最新接口文档为准。

代码短路径相对 internal/admin/；service/、repository/、models/ 位于 internal/；docs/ 路径相对仓库根目录。

## 目录

- [模块定位](#模块定位)
- [目录与权威文件](#目录与权威文件)
- [关键流程](#关键流程)
- [登录](#登录)
- [前端发布 APK（Android）](#前端发布-apkandroid)
- [鉴权拦截](#鉴权拦截)
- [运营工作台统计](#运营工作台统计)
- [投稿审核统计](#投稿审核统计)
- [后台头像读取](#后台头像读取)
- [账号限制运营](#账号限制运营)
- [处理意见反馈](#处理意见反馈)
- [地图与同行统计](#地图与同行统计)
- [运营聚合路线](#运营聚合路线)
- [审核轨迹投稿](#审核轨迹投稿)
- [管理轨迹](#管理轨迹)
- [主景点目录](#主景点目录)
- [查看埋点同步摘要](#查看埋点同步摘要)
- [用户设备权限](#用户设备权限)
- [标签管理入口补充](#标签管理入口补充)

## 模块定位
后台形象生成使用 `TrackShapeGenerator.GenerateShape`，经 `PreviewShape` 返回诊断而非复用路线说明生成；关闭思考模式，max_tokens=8192。`track_labels.go` 返回 `diagnosis`，共享编辑器用 textContent 展示分类/候选及结论。详情只展示同输入版本的最近诊断 `label_generation.shape_preview`，识别结果仅填表，保存才生效。接口契约见 `docs/api/admin-tracks.md`，验收不能用独立提示词脚本代替实际后台接口。

轨迹形象保存成功且值变化时调用 `RecordShapeCorrection`，复核在后台执行；反馈失败或 AI 未配置返回 `learning_warning`，前端明确“轨迹已保存”。详情 `label_generation.learning` 展示状态、依据和经验；不在 HTTP 保存请求内等待模型。入口 `service/track_shape_learning.go`。


`internal/admin` 是 **track_server 的内置管理后台**，提供：

- 一个独立登录的 Web 控制台（HTML/CSS/JS 经 `go:embed` 内置）；
- 围绕 `AppRelease` 的发布、列表、删除接口；
- 发布管理页顶部的客户端活跃统计区（紧凑布局）：活跃设备数、最新版本渗透、低于强升线设备数三张概况卡，以及版本活跃趋势（堆叠面积）、版本分布（饼图）、机型/系统版本 Top 10（条形图），支持近 7/30 天切换与统一刷新；版本标签只展示版本名，同名的不同构建在饼图与逐日趋势中合并计数，缺失版本名显示“未知”；概况区也不展示内部版本号，强升线判定仍使用原有数值规则。数据来自升级检查与登录上报的 `app_client_devices` / `app_client_active_daily`，经 `AdminAssets.loadCharts()` 异步加载 ECharts，不阻塞发布列表首屏；
- 上传 Android APK 到服务端本地静态目录的接口；发布表单固定 Android，版本名和版本号由包解析结果自动填充；
- 通过后台会话代理读取服务端静态资源（如用户头像），避免复用业务 JWT；
- 查看和处理用户意见反馈（含图片预览、状态更新、用户可见反馈意见）。
- 查看埋点 OSS 同步摘要（任务状态、文件明细弹窗、OSS key、字节数、耗时与错误）。
- 在用户管理页查看、创建和解除账号限制；账号限制会由业务侧中间件拦截上传、发起同行、关注、收藏、修改资料等动作。
- 查看和人工运营路线发现 RouteGroup（展示离线 `area_id` 对应的区域名称、类型与介绍页，支持改名、合并、移除成员、指定代表轨迹，以及编辑、发布和下线双语路线介绍）；景点页还提供轨迹围住多个独立景点的 candidate 队列，可确认其中一个或忽略。
- 通过响应式工作台查看业务库即时统计：日/周/月核心增长、同周期新增用户转化、投稿与反馈处理效率、轨迹内容质量、互动趋势、热门轨迹和同行会话；该口径不依赖 T+1 埋点同步。
- 审核用户轨迹投稿，查看结构化路线资料和经本地缓存改写的投稿图片，按 revision 通过或驳回。
- 管理后台内置静态资源支持 gzip、ETag 与内容版本缓存；公开登录页 no-cache，含账号身份的受保护 HTML 和 API no-store。工作台及列表页先加载主体数据，再异步加载 ECharts 绘图。
- 投稿审核页顶部展示当前待审核与超时队列，以及本月通过、驳回、通过率和平均审核时长；复用 `/admin/api/dashboard?period=month` 的只读统计。
- 查看、修正和删除用户轨迹；列表按 ID、城市、标题、运动类型、用户名、轨迹图、状态和操作展示，支持标题及用户名模糊搜索、省→市级联筛选（省份由服务端展开为城市集合，city_code 优先）、状态筛选（公开/私密/已下架，默认排除已下架，选已下架则只列已下架记录），并保留游标分页。`status=0` 统一称「下架」（软删除，含客户端用户删除与后台删除），下架轨迹可恢复或彻底删除（purge）。用户名同时展示头像；景点详情中的相关轨迹列表复用相同列顺序。列表不单独放删除按钮；编辑轨迹使用共享 `static/track_editor.js` 表单，底部删除动作需二次确认，删除为软删除并同步清理关联数据。
- 反馈与纠错页默认筛选待处理，展示状态统计、反馈用户名称和完整内容；点击查看会在弹窗中加载详情并处理反馈。`static/feedback_correction.js` 优先读取服务端 `correction` 快照，旧记录兼容完整的六种中英文纠错正文；不识别的内容仍按普通反馈显示，不改写原始正文。页面「纠错选项」可新增、改名、排序、启停，由 API 下发客户端。
- 纠错选项支持 description/description_en 与 notes_hint/notes_hint_en（可空、单行、各最多 160 字），后台配置与客户端展示共用协议，不按显示名称推导类别。反馈列表的 kind/track_id/correction_option_id 在 MySQL/Mongo/内存仓储分页前过滤，仅按结构化 correction 识别类型，旧文本纠错保留兼容展示。
- 纠错处理结果与状态分离：corrected/unchanged 配 resolved，unverified 配 ignored；新版后台提交 revision 作条件更新，终态必须填写结果及用户可见回复，冲突返回 409 且保留草稿。处理人只从后台 session 取值。服务与仓储入口为 internal/service/feedback_resolution.go 和 internal/repository/feedback_resolution.go；复用 correction_json/BSON，不新增表列。
- 纠错详情按轨迹查看权限异步读取当前资料，并复用共享 track_editor.js；来源 source_feedback_id 的轨迹保存同时要求轨迹操作与反馈操作权限，核对纠错与轨迹一致后，只在实际保存成功且字段变化时记录最近关联修改的字段名、处理人和时间。轨迹写入与反馈关联非跨仓储事务，关联失败返回 correction_warning，页面必须明确提示轨迹已保存。仅修改反馈状态不视为轨迹已修正；历史无结构化快照时不能伪造关联。后台详情额外返回 handled_by/track_change，业务客户端不下发管理员身份及内部改动记录。

- 用户管理支持昵称模糊搜索，以及内部人员、资料所在城市、性别、生日星座和当前账号限制状态组合筛选；同行管理按会话标题、运动类型、发起人昵称和发起人资料省市筛选（昵称/省市先在用户仓储解析为发起人 ID 集合，再由 `AdminCompanionListRepository` 在仓储层参与总数与游标分页，列表批量补充发起人昵称/头像和成员记录数），轨迹管理按标题和用户名提供服务端模糊搜索，轨迹列表还支持城市、状态和资料缺项筛选，均保留游标分页。

模块通过 `admin.NewModule(...)`、`Module.ConfigureEmailRecovery(...)`、`Module.RegisterRoutes(h)` 注入和注册，不被业务 handler 引用；Close 同时停止会话 GC 与邮件 worker。

省市筛选：用户管理 `GET /admin/api/users` 接收 `province_code`，与 `city_code` 按用户资料取交集；景点管理 `GET /admin/api/scenic-spots` 接收 `province_code` / `city_code`，城市优先，匹配景点自身城市或任一仍存在的关联轨迹城市。`repository.ScenicSpotIDsByTrackCities` 为 MySQL/Mongo/内存提供关联城市匹配，`ScenicSpotListFilter` 将地域条件用于分页前筛选和 total；不得退回仅过滤浏览器已加载记录。省市下拉共用 `/admin/api/cities`，直辖市省级码保留可选。协议见 `docs/api/admin-regions.md`。

共享轨迹编辑器只读展示详情接口的 `user_name`，`user_id_text` 仅保留协议兼容，保存不传 `user_id`/`user_name`，服务端仅兼容回传原 ID 并拒绝更换归属。坐标系和来源标签使用下拉选项，未知历史值保留。`GET /tracks/:track_id/map-preview` 归属轨迹查看权限，由 `TrackMapIndexService.PreviewCoordinateSystems` 读取原始轨迹并返回按 WGS84/GCJ02/BD09 解释的三组 GCJ02 预览线；不得将既有 GCJ02 索引线二次转换。预览独立异步加载，准备后切换立即重绘且保持视野，关闭弹窗释放地图及取消请求。保存坐标系变更纳入旧空间索引清理及重新排队流程；预览自身不写业务数据。`ui.js` 支持字段 `readOnly` / `onChange` 及表单 `onClosed` 生命周期。接口细节见 `docs/api/admin-tracks.md`。

描述来源、描述规则版本、轨迹进行状态不展示且不随表单保存；未修改说明时保留原值，修改说明时由服务端标记 manual、版本0（含清空）。无地图 SVG 由 `track_asset_svg.go` 复制安全矢量几何并按服务端运动主题色渲染，切换表单运动类型即时预览，不改写原文件；未知类型与历史位图保留原图。

详情资源由 `track_assets.go` 的 `GET /tracks/:track_id/assets/:asset` 按轨迹查看权限读取，asset 白名单为 raw、square_day/night、landscape_day/night、legacy、no_map。`main` 经 `SetTrackAssetCaches` 注入既有 raw/preview 缓存，截图沿用 `SetScreenshotCache`；按请求异步缓存，不阻塞详情字段，不接受浏览器 URL，不把资源代理地址写回轨迹。原始文件使用 attachment 下载，空字段显示“缺失”；图片只显示缩略图（点击放大），空字段显示“无”，读取失败可重试。共享编辑器保存不提交原文件/截图/preview_images；加载共享编辑器的页面同时引用 `static/track_editor.css`（由嵌入资源机制自动加内容版本）；`ui.js` 的 `className` 与 `afterFields` 将最多四列表单及资源区限定在 `.admin-track-editor`。接口见 [详情资源展示与下载](api/admin-tracks.md#详情资源展示与下载)。

## 目录与权威文件

```text
internal/admin/
├── auth.go         # 持久化账号鉴权 + bcrypt + cookie + AuthMiddleware
├── accounts.go     # 账号、身份、模块权限、首次导入和账号版本
├── account_storage.go # MySQL/Mongo 状态 CAS + 操作日志
├── account_handlers.go # 账号管理、改密与日志接口
├── email_challenges.go # 邮箱/找回挑战与配额，复用账号状态 CAS
├── email_handlers.go   # 本人邮箱与公开密码找回 HTTP 接口
├── email_recovery.go   # 有界邮件 worker、IP 限制与迁移配置入口
├── email_sender.go     # TLS SMTP，不记录邮件或认证内容
├── permissions.go  # HTML/API/资源与跨模块授权
├── dashboard_permissions.go # 工作台统计响应裁剪
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
| 公共资源版本与页面传输 | `assets.go` 生成嵌入资源内容版本、预处理页面引用并压缩私有 HTML；`routes.go` 区分 immutable 静态资源与 no-store 业务响应；`static/runtime.js` 提供版本 URL 和异步图表加载 |
| 列表图片等待 | `list_assets.go` 限制并发与头像去重；`handlers.go` 的 `decorateAdminTrackAssetURLs` 处理轨迹预览资源，仍等待缓存成功后才使用本地 URL |
| 业务依赖注入 | `routes.go` 的 `NewModule(accounts, releaseSvc, stsSvc, staticRoot, userRepo, trackRepo, collectRepo, trackMapRepo, ..., restrictionSvc, routeGroupSvc, accountStorage)`；`cmd/server/main.go` 创建后通过 `Handler.SetScreenshotCache(screenshotCache)`、`Handler.SetTrackSubmissionService(trackSubmissionSvc)` 和 `Handler.SetDashboardStatisticsRepository(...)` 注入缓存、投稿与跨域只读统计能力 |
| 前端入口 | `static/login.html`；登录后按授权页面跳转；账号管理 `static/accounts.html`、账号安全 `static/password.html`、无业务权限 `static/access.html` |
| 响应式外壳与工作台 | `static/shell.js`、`static/ui.js`、`static/style.css`、`static/dashboard.html`、`static/dashboard.js`；组件库固定为内置 Tabler 1.7.1，图标为 Tabler Icons 3.49.0，图表为 ECharts 6.1.0，均位于 `static/vendor/`，不要改成运行时 CDN |
| 安装包上传 | `handlers.go` 中 `UploadPackage`、`static/app.js` 中 `/admin/api/releases/upload-package` |
| 用户与账号限制 | `handlers.go` 中 `ListUsers` / `CreateAccountRestriction` / `GetCurrentAccountRestriction` / `RevokeCurrentAccountRestriction`，`static/users.html`、`static/users.js` |
| 意见反馈管理 | `handlers.go` 中 `ListFeedbacks` / `GetFeedback` / `UpdateFeedbackStatus` / `GetFeedbackImage`，`static/feedbacks.html`、`static/feedbacks.js` |
| 纠错配置 | `feedback_correction.go` 中配置读写 handler；`static/feedback_correction_config.js` 编辑配置；校验、快照和默认值统一在 `service/feedback_correction.go` |
| 埋点同步摘要 | `handlers.go` 中 `ListAnalyticsSyncSummaries`，`static/analytics.html`、`static/analytics.js` |
| 聚合路线运营 | `handlers.go` 中 `ListRouteGroups` / `GetRouteGroup` / `RenameRouteGroup` / `MergeRouteGroup` / `RemoveRouteGroupMember` / `SetRouteGroupRepresentative`，`static/route_groups.html`、`static/route_groups.js` |
| 轨迹投稿审核 | `handlers.go` 中 `ListTrackSubmissions` / `GetTrackSubmission` / `ReviewTrackSubmission`，统计复用 `GetDashboard`；页面位于 `static/track_submissions.html`、`static/track_submissions.js` |
| 主景点目录 | `scenic_spot_handlers.go` + `ScenicSpotCatalogService`；页面 `static/scenic_spots.html`、`static/scenic_spots.js`，页面与导航显示名为「景点管理」（目录涵盖景区/公园/山系/湖泊，内部 SCP ID、API 路径与模块名 scenic_spots 不变）；目录与调用统计由 `repository.ScenicSpotRepository` 三种实现提供；自动/人工记录均直接下发客户端，归档/合并记录隐藏；首次加载时分批同步存量供应商资料；新建时可点击地图 POI 热点或搜索附近 POI，将资料先放入备选预览，一键代入后写入供应商来源与资料/图片；详情以弹窗编辑并可拖动排序图片 |
| 轨迹编辑弹窗 | `static/track_editor.js` 提供共享编辑表单，轨迹管理页与景点相关轨迹列表共用；保存或删除后调用页刷新各自列表 |

## 关键流程

### 登录

```text
POST /admin/api/login {username,password}
  → AccountManager 从持久化状态取账号 → bcrypt 校验 → 更新最近登录
  → admin=管理员，其余既有账号=只读操作员（仅首次导入）
  → SessionStore.Create → Set-Cookie admin_session=<32B hex>; HttpOnly; SameSite=Lax
```

### 前端发布 APK（Android）

```text
[admin UI] 选 APK
  → POST /admin/api/releases/upload-package multipart(file, platform=android) (鉴权)
  → 服务端写入 <LogDir>/static/release/android/<ts>-<safe-file>.apk
  → 返回 /api/v1/static/release/android/<ts>-<safe-file>.apk 并填进 package_url
  → POST /admin/api/releases   (鉴权)
       → AppReleaseService.Publish → AppReleaseRepository.Upsert
```

### 鉴权拦截

```text
请求 → Authenticator.AuthMiddleware → SessionFromRequest(cookie) → 否则 401
```

### 运营工作台统计

```text
[admin UI] /admin/dashboard.html?period=day|week|month
  → GET /admin/api/dashboard?period=...
  → Handler 生成 current / previous / trend 时间窗
  → AdminDashboardStatisticsRepository 从当前仓储族只读聚合
       用户与活跃、完成轨迹、投稿审核、反馈、互动、地图索引、路线组、同行
  → 服务端补充运动类型、城市和状态展示名
  → ECharts 渲染增长/互动趋势、转化漏斗和内容分布
```
新增用户转化漏斗以同周期注册用户为 Cohort，后续阶段逐级限定为同周期完成轨迹、投稿和审核通过；收藏/导航/点赞/评论的周期指标按各自关系或记录的 `created_at` 统计，不使用轨迹当前累计数倒推历史趋势。工作台聚合只读，不得在加载页面时修正业务数据或触发外部服务。

### 投稿审核统计

```text
[admin UI] /admin/track_submissions.html
  → GET /admin/api/dashboard?period=month
  → 展示当前 pending 与超时数，以及当月通过/驳回/通过率/平均审核时长
  → 审核成功后重新读取统计和当前列表
```

### 后台头像读取

```text
GET /admin/api/users
  → UserService.DecorateAvatar 把头像改写为 /api/v1/static/...
  → AccountRestrictionService.FindActive 为每个用户补当前账号限制状态
  → admin handler 再改写为 /admin/api/static/...
  → 浏览器 <img> 请求携带 admin_session
  → GET /admin/api/static/*filepath 经 AuthMiddleware 后从 <LogDir>/static/ 读取
```
未落盘的内置默认头像（`default_avatars/girl_01.png` 等）由后台静态代理返回简单 SVG 兜底；正式视觉素材仍应放入 `<LogDir>/static/default_avatars/`。

用户管理概况位于用户列表上方，不单独展示用户总数卡片；桌面采用紧凑三列，左侧性别、星座两个 ECharts 饼图上下排列，中间为“用户位置分布”地图，右侧上下展示年龄分布饼图和轨迹产出分层柱状图。年龄图复用性别、星座的环形饼图及侧边图例样式，显示各年龄段人数，悬浮提示人数和占比。页面使用 `/admin/api/users/statistics` 一次读取性别、生日星座、`age` 年龄、`track_output` 云端轨迹产出分层和 `user_cities` 用户资料所在城市分布；MySQL、Mongo、内存仓储均直接按 `users.city_code` 计数，不再使用轨迹记录，每个用户只计入当前设置的一个城市，未设置计入未知城市。后台 `handlers.go` 经 `user_city_statistics.go` 读取一次地区目录，将新旧城市编码合并为统一 `region_id`，`user_cities` 返回专用城市项（内部编号、名称、国家、省份内部编号/名称及省份地图编号），空值或无法识别的编码合并为未知；前端只将明确非 CN 的地区计为海外，不根据编码格式推断。城市按服务端给出的省份地图编号汇总后通过内置 `static/china_provinces.json` 和 ECharts 矢量地图展示，不加载高德 JS 或地图 Key，也不显示颜色深浅图例。地图采用白色背景与浅蓝到深蓝配色，图表和地图保留内侧留白；过滤 `100000_JD` 南海诸岛元素，不展示南海附图；主图使用 `boundingCoords=[[73,54],[136,17]]` 限制中国主体经纬度范围并垂直居中放大，避免顶部裁切和南部空白，不展示辅助说明文字。省份上显示人数，超过一万用 `w` 缩写；初次加载不自动选中省份，地图下方默认按用户数降序展示省份 Top 10；点击省份后显示该省城市用户数 Top 10，点击地图省份外的空白区域清除省份选中并恢复省份 Top 10。海外城市和未知城市的小统计也位于地图下方。统计区右上角“刷新数据”统一重新读取全部顶部统计，保留仍有数据的已选省份；加载时禁用按钮，失败保留上次数据并提示。用户列表展示用户、手机号、性别、城市、粉丝数、上次登录与详情入口，昵称旁显示不带级别名的 `Lv.N`，内部人员角标保留；星座和限制操作集中在详情。列表筛选用用户等级替换星座，等级选项来自服务端权威定义；显式等级筛选一次流式读取全量已完成有效状态轨迹，复用资格与 XP 规则计算等级，再将匹配用户集合放入三种仓储的 total/游标分页条件，初始等级包含无轨迹用户，依赖失败返回503。未筛选等级时，列表按当前页批量流式读取级别所需轨迹字段（`repository/achievement_tracks.go` 三实现），通过 `service/user_profile_overview.go` 复用原 XP/资格/等级规则，不逐用户查轨迹、不结算奖励。粉丝数通过三种关注仓储按当前页批量聚合，失败显示“—”。列表刷新按钮位于用户列表与总数标题行右侧；表格底部左侧提示“第 N 页 · 本页 X 个用户”，右侧上一页/下一页，与同行房间复用分页样式；加载禁用翻页并提示，快速筛选忽略旧响应。详情采用顶部背景/身份/互动数据、左侧基本资料、右侧账号管理和折叠设备权限的布局；昵称、签名、性别、生日与内部人员可保存，手机号和所在地只读，后台拒绝手机号字段。生日输入框内即时显示星座图标标签，使用客户端 MingCute 图形与日夜配色。互动数和级别通过 `/users/:user_id/overview` 独立加载，背景经 `/users/:user_id/background` 鉴权代理，设备权限首次展开才读取；辅助失败可重试，不阻断基础资料。限制区使用既有 GET/POST/DELETE，在当前详情内展开并显式确认，独立于资料保存。协议见 [后台用户资料与级别](api/admin-users.md)。用户列表和详情接口的 `id` 一律以 JSON 字符串下发，避免 int64 用户 ID 被 JavaScript Number 截断后请求错误用户。用户列表的上次登录时间按当前页 user_id 批量查询 `login_log` 的最大 `created_at`，不得用用户资料 `updated_at` 代替；Mongo 登录日志未实现时返回无登录记录。

年龄以服务端当地日期计算周岁，分为 18 岁以下、18–24、25–34、35–44、45–54、55 岁及以上和未知；1900 年以前、未来或无法解析的生日归入未知。轨迹产出按全量现存用户的有效已完成云端轨迹数分为 0、1、2–5、6–20、21 条及以上，包含公开和私密轨迹，排除软删除、进行中、原始文件地址为空及已无所属用户的轨迹；本地未上传记录不计入。MySQL、Mongo、内存仓储保持同口径，生日使用完整日期分组后统一派生年龄和星座，轨迹在仓储层聚合，不逐用户查询；不新增表或客户端协议。顶部统计沿用全量用户口径，不随下方列表筛选变化。

### 账号限制运营

```text
[admin UI] /admin/users.html
  → 打开用户详情后 GET /admin/api/users/:user_id/restrictions/current 读取生效限制（404表示无）
  → POST /admin/api/users/:user_id/restrictions {reason, expires_at?}
       → AccountRestrictionService.Create
       → 先撤销该用户现有未过期 active 限制，再写入 user_account_restrictions
  → DELETE /admin/api/users/:user_id/restrictions/current
       → AccountRestrictionService.RevokeActive
```
限制创建/解除在用户详情账号管理区内展开并显式确认，不另弹资料框，也不提交基本资料草稿。限制生效后由业务 `/api/v1` 侧 `AccountRestrictionMiddleware` 拦截；admin 页面只负责运营写入和解除。

### 处理意见反馈

```text
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

### 地图与同行统计

后台地图模式只增加“路线空间覆盖”和“路线组规模分布”两张图；与友同行只增加“创建/结束趋势”和“参与人数分布”，使用近 1/7/30 天切换。`AdminDashboardStatisticsRepository` 的 `GetAdminMapModeStatistics` / `GetAdminCompanionUsageStatistics` 在 MySQL、Mongo、内存仓储中只读聚合；不新增表、客户端协议或业务埋点。实现位于 `internal/repository/admin_feature_statistics*.go`，模型为 `internal/models/admin_feature_statistics.go`，后台入口为 `feature_statistics.go`。地图按全量 active 路线组中心 GCJ02 坐标展示；规模按当前 member_count 分为 0、1、2–5、6–20、21 条及以上，城市去重。同行使用趋势按创建/结束各自发生时间独立计数，不以同周期结束数除以创建数作为完成率；人数分布以本周期创建的会话为 cohort，按服务端成员记录 user_id 去重，包含曾离开或移出的成员，不使用 max_members 或客户端 actual_participant_count。没有成员记录的历史会话单列未知，平均人数只计算有成员记录的会话。近 1 天为滚动 24 小时按小时分桶，7/30 天按北京时间自然日含当天补齐空桶。统一刷新保留周期，切换失败回退到上次成功周期；两页不再增加排行和重复数字卡片。

`static/admin_usage_charts.js` 共用空间聚合、折线与分布图；`static/feature_statistics.js` 是地图模式/同行页的轻量控制器，景点页通过 `scenic_spot_charts.js` 复用同一地图与趋势组件。组件均使用已内置 ECharts 和省界数据，不访问高德图表 API；地图保留视野并支持点选详情，页面按窄屏上下排列。地图模式统计刷新同时刷新列表，与友同行沿用页面顶部统一刷新按钮。

### 运营聚合路线

```text
[admin UI] /admin/route_groups.html
  → GET /admin/api/route-groups?track_type=&province_code=&city_code=&limit=&offset=
       → 运动类型为 icon+文字下拉，省→市级联；列表按关联区域分组成可展开的两级结构（区域 → 路线组），路线组行点「管理」打开详情弹窗
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

### 审核轨迹投稿

```text
[admin UI] /admin/track_submissions.html
  → GET /admin/api/track-submissions?status=pending
  → GET /admin/api/track-submissions/:submission_id 查看投稿、轨迹与审核流水
  → POST /admin/api/track-submissions/:submission_id/review
       {decision, reason, expected_revision}
       → TrackSubmissionService 按 revision 原子审核并追加事件
```
投稿图片先经 `submission_images` AssetCache 缓存，再把 `/api/v1/static/submission_images/*` 改写为 `/admin/api/static/submission_images/*`；后台响应不得暴露原始 OSS URL。

投稿列表不显示投稿编号；类型复用内置运动图标和主题色标签，难度、风险、状态显示中文，展示元数据由列表及详情响应提供。点击任意状态的“查看”立即打开当前页详情弹窗，不再展开页面底部面板。弹窗将路线指标、介绍与图片、出行信息、审核结果和折叠审核记录分区，桌面双列、窄屏单列，操作固定在底部；地形、交通、月份与审核事件均中文展示。固定弹窗复用单实例，关闭取消读取、切换忽略旧结果、失败可重试。只有待审核且具备操作权限时允许审核，已通过等其他状态只读；保留版本检查与驳回原因校验，提交期间防止重复操作。

### 管理轨迹

```text
[admin UI] /admin/tracks.html
  → GET /admin/api/tracks 拉取轨迹列表
  → 列表上方常驻的边框「批量导入」区域（紧凑栅格，窄屏单列，无弹窗）：选择归属人（内部人员下拉，头像+昵称，GET /admin/api/users?is_internal=true）、运动类型、坐标系与多个轨迹文件
  → POST /admin/api/tracks/import（multipart）
       → AdminTrackImportService 逐文件解析（复用地图索引解析器）→ 服务端直传 OSS 归属人目录
       → TrackService.CreateTrack 建档（source_tag=manual_seed，已完成），自动排队地图索引与成就结算
       → 城市/地址由内置行政区目录离线解析；不生成预览四图；预览由客户端 DevMode 补齐，路线说明由服务端 AI 或后台单条联网填写，不再依赖客户端算法
       → 源文件无时间戳时按运动类型估算速度沿轨迹合成时间戳（见 docs/api/admin-tracks.md），保证耗时/热量/均速与客户端图表可用
       → 标题默认自动生成；文件名（去扩展名）含汉字时优先用文件名做标题（≤40 字符）
       → 成功结果附带 track 摘要（id/city_code/title/track_type/status），结果表复用主列表单元格并增加文件名/导入结果；用户名与头像复用本批归属人，不逐文件查详情。成功行直接打开共享编辑器，保存/删除后刷新对应导入行与主列表；失败行保留错误且无编辑入口
       → 展示 preview_images.landscape_day_url（日间横版 4:3），并经后台静态代理访问
       → 用户名通过当前页 user_id 批量读取用户昵称，列表不展示 user_id
  → GET /admin/api/tracks/statistics?period=day|week|month 展示轨迹类型环形图与贡献用户前十横向柱状图，两卡片独立切换周期、默认本月；与用户管理共用 admin_chart_style.js，内容区压缩到220px，不再展示城市分布
  → GET /admin/api/cities 将城市 Code 显示为名称并提供编辑选择器
  → 管理员点击编辑；删除操作位于共享编辑表单底部，并经确认后执行
  → GET /admin/api/tracks/:track_id 获取完整可编辑业务详情
  → PUT /admin/api/tracks/:track_id 更新所选业务字段与预览图；ID、轨迹点和创建/更新时间不由表单修改
  → DELETE /admin/api/tracks/:track_id
       → track_records.status=0，is_running=0，记录/保留 deleted_at
       → 清理 track_collects
       → 清理 track_map_index_jobs / track_geo_indexes / track_route_group_members
       → 若删除的是路线组代表轨迹，归档该路线组并移除其全部成员，后续由 track_route_group 任务重新聚合剩余轨迹
  → POST /admin/api/tracks/:track_id/restore（已下架轨迹详情弹窗底部按钮变为「恢复」）
       → status 还原为公开、清空 deleted_at，重新排队地图索引任务；收藏与路线组成员关系不可恢复
  → POST /admin/api/tracks/:track_id/purge（已下架轨迹详情弹窗主按钮为「彻底删除」，双重确认）
       → 仅限 status=0；事务物理删除主表与全部关联数据，成功后尽力删 OSS 原始文件与预览图（oss_warning 不阻断）
```

轨迹编辑入口通过共享 `AdminTrackEditor` 对整个工作流去重，点击后立即打开加载弹窗，详情返回后在原弹窗内填充；城市选项复用列表已有数据或独立加载，不再作为打开详情的前置等待。关闭加载框取消请求，失败原地重试，保存/关闭收尾前不会重复创建编辑器。详情 handler 在轨迹主记录读取成功后，将景点、归属用户名、投稿、形象生成状态、地图索引五组只读查询固定并发执行，汇总后再组装响应；目录主景点有效时省去旧景点识别记录查询，仍保留目录不可用时的旧回退与缺失字段语义，不新增供应商调用或协议字段。其他页面入口和辅助加载边界见 [详情与编辑入口](admin-ui.md#详情与编辑入口)。

### 主景点目录

共享轨迹详情地图支持高德热点与普通地图点击：点中 POI 时在地图下方展示名称、详情和设置按钮，不依赖高德气泡的挂载/关闭时机；普通点击则查询周围 1500 米内最多 8 个景点，由管理员先选目标再设置，同一次点击优先采用精确热点。通过“设为该轨迹景点标签”显式立即写入 `/tracks/:track_id/scenic-spot`，需轨迹与景点双操作权限。已有目录按来源 ID 复用，未收录时只创建基础资料，不触发 AI/采集，不覆盖已有内容。沿用内部主景点关联与统一 labels，比较详情读取的旧关联 ID，冲突刷新后重新选择。成功局部刷新景点标签/主景点及来源列表，保留表单草稿与地图视野；只读/下架不注册景点点击入口，关闭详情取消定时工作、忽略迟到查询并销毁地图。协议见 [地图点选景点标签](api/admin-tracks.md#地图点选景点标签)。

```text
[admin UI] /admin/scenic_spots.html
  → GET /admin/api/scenic-spots/statistics 汇总有效景点数、覆盖城市数、关联有效轨迹数、待完善景点数、空间分布和新增入库趋势；days=1|7|30，默认 30
  → POST /admin/api/scenic-spots/provider-content/backfill 首次加载时游标分批同步供应商文字和图片；已同步的存量景点仅用轻量地点详情补缺失坐标，provider_coordinates_status 防止重复查询
  → POST /admin/api/scenic-spots/description/generate 使用详情弹窗当前表单内容查询 Kimi，将结果返回并填入简介编辑框；存量记录须由管理员逐条点击，保存按钮负责入库
  → GET /admin/api/scenic-spots/:spot_id/map 通过后台鉴权代理返回高德静态地图图片，供应商密钥不下发浏览器
  → GET /admin/api/scenic-spots/map-config 向已登录管理员下发高德 Web Key；AMAP_SECURITY_JS_CODE 仅由同源地图服务代理使用，不下发浏览器
  → /admin/api/scenic-spots 查询、创建/编辑草稿、发布/归档、查看审计
  → POST /admin/api/scenic-spots/:spot_id/provider-sources/verify 校验高德/Google 来源 ID
  → POST /admin/api/scenic-spots/:spot_id/provider-sources/preview 实时获取来源详情；景点目录的存量资料由 provider-content/backfill 分批同步并保存
  → POST /admin/api/scenic-spots/:spot_id/photos 上传后台自有图片
  → /admin/api/scenic-spots/:spot_id/tracks 搜索、关联或解除轨迹
  → POST /admin/api/scenic-spots/:spot_id/merge|split 合并目录或拆分轨迹归属
  → POST /admin/api/scenic-spots/backfill 分页将存量轨迹迁移到内部 SCP ID 草稿，并复用已有主景区匹配缓存中的名称；遇到已关联但名称为空的草稿时尝试恢复该名称。缺少可用来源 ID 时按地图服务配置执行有预算的真实查询
  → 共享轨迹详情 GET /admin/api/tracks/:track_id/place-candidates 查看当前候选；POST confirm 指定一个后更新 match 为 matched 并创建/关联内部景点，POST dismiss 仅标记候选不成立
       → ScenicSpotCatalogService + ScenicSpotRepository (MySQL / Mongo / memory)
       → track_scenic_spot_links 关联轨迹，scenic_spot_audit_logs 记录运营操作
       → scenic_spot_provider_usage 只累计景点/provider/日期/操作的请求成功失败计数；同步后的供应商文字与图片引用保存在 scenic_spots 的资料和 photos 数组中
```
轨迹只保存内部景点关联；外部来源 ID 留在景点目录。存量回填对没有可用来源 ID 的合格轨迹强制刷新坐标查询缓存，调用地图 API；高德每日上限按配置执行（默认不设日上限），每轮最多 10 次真实请求；Google 不受服务端每日或单轮请求数上限约束。索引刷新失败时回退到已有索引继续匹配并单独计数；结果有明确重复命中才创建空白目录项。后台景点页先展示四张概况卡、景点空间分布图和可切换近 1/7/30 天的新增入库趋势，不再单独展示缺项图、关联轨迹 Top 10 或供应商调用量图表；底层供应商每日计数继续保留用于排查。统计范围排除 archived/merged；有效轨迹包含公开和私密、已完成且 raw_track_url 非空的未软删除轨迹，失效关联不计入，跨景点按每条轨迹唯一主景点关系统计。覆盖城市合并景点自身 city_code 与有效关联轨迹 city_code 去重，无法识别的城市不计入。缺项为缺简介、无图片、缺有效非零坐标或缺地址，一处景点可有多项；待完善数按景点去重。`repository.CountCompletedScenicSpotTracks` 在 MySQL/Mongo/内存三种仓储中批量关联轨迹计数与城市，`repository.ScenicSpotMissingFields` 定义统计/筛选口径。点击待完善卡片会清空其余列表条件并使用 `GET /admin/api/scenic-spots?missing=any|description|photos|coordinates|address` 在仓储层筛选，筛选参与 total 和 cursor 分页；默认目录只列有效景点，显式 status 查询保留维护用途。列表支持加载更多，不仅筛选首批记录。统计区刷新为只读操作，加载中禁用按钮，失败保留上次数据并提示；统计加载不读取供应商调用历史或触发供应商请求。`static/scenic_spot_charts.js` 通过共用 `admin_usage_charts.js` 复用内置 ECharts 和中国省界底图：全量有效景点按已有坐标转换为 GCJ02 后展示，像素网格聚合随缩放重算，点击聚合点放大并可选择景点详情，缺坐标单独显示；刷新/周期切换保留当前视野。趋势按北京时间及景点 created_at 统计入库数，含回填、后续归档/合并记录；近 1 天为滚动 24 小时按小时分桶，近 7/30 天含当天按自然日分桶，空桶补零，区间为 [start,end)，不将资料 updated_at 当新增。目录为紧凑整行，展示省市、景点名、景区等级、资料类型、图片数和操作，不展示分类、简介或缩略图；景区等级为空显示“-”，图片数按photos数组计数，无图显示0。目录请求view=summary轻量摘要，不携带简介、分类、图片URL或来源详情；打开详情再读取完整资料。省市按权威目录解析，直辖市不重复省市名，多城市保留关联轨迹覆盖；“相关轨迹(N)”按钮展示完整关联轨迹数量，目录列表通过仅后台使用的 `related_track_count` 下发；计数与相关轨迹弹窗一致，只排除已不存在的轨迹，不套用顶部“有效已完成轨迹”的过滤口径。`RelatedTrackStatistics` / `repository.CountScenicSpotTracks` 在 MySQL/Mongo/内存仓储按当前页批量计数并补全关联城市，不逐景点查询，也不受原先每景点首 100 条的限制。弹窗接口返回 `total`，展示总数与已加载数量，编辑/删除后重新读取并回填按钮数量。“相关轨迹”操作通过景点轨迹列表接口，以轨迹管理页相同的表格字段（不含开始时间）展示关联轨迹；点击行或编辑按钮打开共享 `static/track_editor.js` 可编辑表单，保存或删除后刷新关联列表。景点详情在弹窗内编辑文字资料和图片，并显示供应商坐标、坐标系及位置地图。供应商文字同步到景点 JSON 资料，图片与人工上传图片共用 `photos` 有序数组；拖动排序和上传追加顺序都会随保存结果下发。Google 坐标按 WGS84 保存，高德坐标按 GCJ02 保存；来源同步只填补空坐标，不覆盖既有坐标，已同步记录单独请求坐标字段，避免再次请求照片媒体。城市信息只读展示，不作为编辑字段。`manual_edited` 表示资料由管理员创建或编辑保存；自动同步资料仍显示“自动”。页面隐藏来源 ID、轨迹关联编辑、合并拆分操作，但提供相关轨迹列表和编辑入口；其余数据/API仍由服务端保留，供客户端景点关联及维护流程使用。景点使用 active/archived/merged 状态，存量 draft/published 在仓储初始化时迁为 active；页面展示的“自动/人工”是内容维护方式。active 目录可覆盖旧轨迹摘要，归档和合并记录不下发。后台上传图像走受鉴权的 `/api/v1/static/scenic_spots/*` 资源路由，Android 详情页可读取。更新 MySQL/Mongo/memory 任一仓储时，保持三种实现、版本冲突控制和审计行为一致。
景点页统计开始于该计数能力上线，不回填旧请求；计入已关联内部景点的来源详情、照片请求和 ID 核验，存量轨迹需先运行回填，按坐标执行的识别搜索不计入单景点排行。目录覆盖率以关联轨迹计数为准，目标是帮助运营优先补齐高调用景点的自有内容。
所有 Google Places 查询均不受服务端每日预算或单轮请求数上限约束，由 Google API 自身配额控制。

Kimi 景点简介使用 `MOONSHOT_API_KEY` 和可选的 `KIMI_SCENIC_DESCRIPTION_MODEL` 环境变量，默认模型为 `kimi-k2.6`。存量景点详情弹窗的“AI联网查询”链接把当前名称、城市、分类和地址提交给 `/admin/api/scenic-spots/description/generate`，结果只填入简介输入框，点击既有保存按钮后才入库；该查询不自动批量触发。新建记录在创建前对空简介尝试生成，成功后随景点资料一起保存，失败时仍可创建并保留空简介。模型检索与摘要通过 Kimi 官方 `/v1/tools/search_pro` 和 `/v1/chat/completions` 接口完成，景点简介不再额外限制200字，按已有景点字段上限8000字符接收完整结果；提示词要求简洁，但不机械卡字数。
### 查看埋点同步摘要

```text
[admin UI] /admin/analytics.html
  → GET /admin/api/analytics/sync-summaries?status=&limit=&offset=
  → AnalyticsRepository.ListSyncSummaries / CountSyncSummaries
  → 页面展示任务状态、扫描/上传/失败文件数、总字节数、OSS 前缀与 files_json 文件明细
```

## 用户设备权限

- `permission_snapshot.go` 提供用户详情的设备快照读取，路由 `/users/:user_id/permission-snapshots` 归属 users 读取权限。`static/user_permissions.js` 在设备权限首次展开时独立异步加载，关闭/切用户取消旧请求；按安装实例显示机型、版本、采集和收到时间，非实时状态。已开启绿色、未开启红色、未知/不适用灰色，普通系统权限折叠；未知项用 textContent 安全显示，禁止把无上报视为关闭。
## 标签管理入口补充

共享 track_editor.js 展示统一 labels 和 label_generation，提供 AI 形象建议及人工 shape_label 输入。POST /admin/api/tracks/:track_id/labels/generate 属 tracks 写权限，复用 TrackNarrativeService 租约和日预算，只填表不落库，保存仍用原 PUT。关闭弹窗取消请求，异步结果不覆盖后来输入。精品、运动、景点沿用原权威来源，不接受独立 labels 写入。实现见 track_labels.go 和 service/track_narrative_admin.go。


### 景点详情删除与景区等级

省市和景点图片项统一取消左右移动按钮，保留拖动排序；缩略图下、说明框上显示`宽 × 高 px · KB/MB`。共享编辑器将文件信息与照片编辑状态分离：新上传/导入响应提供只读`image_info`，旧本站图片通过原静态资源代理的`photo_info=1`读取，最多3并发、单请求10秒超时，切换/关闭/移除时取消并忽略旧结果。读取失败或外链不能可靠取得体积时显示未知，不影响上传、自动保存或文字资料，不自动写回数据库或抓取第三方文件。

省市宣传图片与景点图片上传统一支持JPG、PNG、WebP，单张≤8MiB、尺寸≤12000×12000。共享选择器接受WebP MIME及浏览器未识别MIME时的扩展名，后端通过图片内容解码识别真实格式，WebP原样保存为`.webp`，后台预览返回`image/webp`；复用已有`golang.org/x/image/webp`解码器，不新增依赖。上传后的自动关联保存、图片元数据、排序及权限流程保持原协议。

高德景点外链另由 `service/scenic_spot_photo_store.go` 在写流程及既有资料回填中下载到本站，复用城市图的WebP质量75及编码参数，并区分横图宽1024/竖图高720、先校正EXIF。存量回填不因文字已同步而跳过图片，后台每批1处显示成功/失败张数；失败保留外链，下轮可重试，CAS冲突不覆盖人工编辑。共享图片编辑器接纳保存响应中的新地址并刷新文件信息，保留在途说明编辑。图片信息GET只读取最终文件，不发起下载；手动上传图片及其他供应商不在本轮转码范围。完整规则见[高德景点图片本地存储](track-place-discovery.md#高德景点图片本地存储)。

共享图片卡片的灰圈叉号移除按钮位于整张卡片右上角；已上传和已有图片在图片右下角提供下载原图图标，只读账号也可使用。下载通过现有后台鉴权资源地址读取图片后交给浏览器保存，保留文件名及格式，不下载裁剪缩略图、不写照片资料；失败提示并可重试，移除、切换或关闭编辑器取消在途下载，未完成上传/导入的图片不显示下载入口。外链图片沿用浏览器跨域限制，不新增服务端外链代理。

详情底部“删除景点”仅已有 active 景点且具有 scenic_spots 写权限时展示。复用 POST /admin/api/scenic-spots/:spot_id/status，status=archived、expected_version CAS 与审计；从默认目录和客户端隐藏，保留轨迹、图片、来源与历史记录，不物理删除。共享 AdminUI.confirm 前关闭原生 dialog，取消后恢复原草稿；409 不自动重试，成功清理深链并刷新目录与统计。

省市宣传图片标题旁的“从 Pixabay 查询”由 `static/region_pixabay.js` 提供：管理员点击后用当前英文/中文地名查询3张横版候选，可改关键词、手动换组、勾选导入或全部导入。`internal/regions/pixabay.go` 只由后台人工POST调用，配置 `PIXABAY_API_KEY` 后使用私有磁盘24小时查询缓存，不扫描省市或定时下载。导入由服务端核对缓存候选ID、使用API原始1280地址（旧缓存缺失时用原始640地址），再等比缩至960×720以内并复用地区静态资源存储；不改写签名URL后缀。`photo_upload_editor.js` 复用队列、取消和重试，用不确定进度遮罩展示服务端处理，不伪造下载百分比。`region_photos.js` 接合现有节点草稿和首次保存，最终由独立图片接口自动CAS保存关联；来源/作者/许可自动保留。两个接口均为地区操作权限并审计，详见 `docs/api/regions.md#pixabay-人工选图`。

景点详情弹窗的内容独立滚动，保存与删除操作固定在底部并共同右对齐；删除使用实心危险色按钮。图片卡片隐藏来源文案，保存时仍保留来源和归属元数据。城市管理的省份/城市详情参照景点图片提供上传、说明、移除和拖动排序；上传只暂存受保护资源，随后自动通过独立图片接口保存关联，沿用目录JSON、图片基线和版本CAS。协议及上传限制见 `docs/api/regions.md`。

城市管理页不再注册 `beforeunload` 离站拦截，切换页面、刷新或关闭标签页时不弹浏览器的“离开此网站”确认；基本资料仍手动保存，宣传图片仍按原流程自动保存。

城市管理完整目录读取与保存响应按请求协商gzip并保留no-store；前端维护节点/父子索引，目录加载、保存合并草稿、新增和删除后重建索引，重用同级排序。已展开节点之间切换只更新树选中状态，非树字段输入不刷新树；搜索、名称变化、展开和国家切换仍更新可见树。地区文字由表单手动编辑并保存，不再提供 AI 联网填写及检索接口；批量素材与文案维护使用项目内 `.agents/skills/track-region-content/SKILL.md`。图片自动保存继续仅更新该节点缺项标记。详见 `docs/api/regions.md#后台管理`。

城市树名称旁按已保存节点资料展示“缺口号 / 缺简介 / 缺图片”小标记，空白文字视为缺失，图片至少有一条非空URL才算已配置；图片标记仅用于支持宣传图片维护的省份和城市节点。未保存的文字编辑不提前清除缺项，节点保存成功后重新计算；图片自动保存成功后仅更新对应可见树节点的标记，保留展开、选择和滚动状态，删除最后一张图片保存后恢复“缺图片”。

非中国国家的详情右上角提供实心“删除国家”按钮，二次确认会连同下级地区一起移除。`regions.Service.DeleteCountry` 经独立DELETE与目录版本CAS将整棵子树置为inactive，保持存量编号、用户/轨迹引用及文件；国家标签只展示active国家。中国由前后端共同保护。前端删除成功后清理对应草稿和图片队列，其他国家草稿保留；服务端沿用共享Store与现有审计、地区写权限，不新增表或物理删除文件。

Pixabay大图下载失败时最多回退同一缓存中的原始640图一次，父请求已取消时停止；读取失败按超时、传输中断、空内容和实际超限分别提示并记录不含URL/密钥的诊断信息，不再统一报“下载图片大小无效”。每次传输30秒上限、整个任务最多2次传输且占用同一个并发槽，不额外查询供应商。

景点图片与省市“宣传图片”共用 `static/photo_upload_editor.js/css`：多选后立即生成有序图片项，已保存记录自动上传，每个编辑器最多并行两张；遮罩只覆盖图片，显示真实发送进度，100%后等待服务端确认再隐藏。网络/服务端失败原项重试，格式/大小错误提示重新选择；右上角圆圈叉移除并取消请求，取消或关闭后的旧回调不得复活图片；正常完成不重建其他图片项。说明输入及拖动排序保留，取消左右移动按钮，图片顺序由本地项顺序决定。每张完成后由独立PUT自动关联，删除/排序自动保存、说明失焦保存；保存队列串行，状态显示保存中/已保存/失败并可重试。新记录先保存基本信息再开放上传，新建景点保存后保留详情供继续添加图片。图片编辑不标记文字表单dirty，基本资料保存只携带已确认的图片，未完成图片修改保留在原详情。景点上传使用可选 `POST /admin/api/scenic-spots/:spot_id/photos?draft=1`，只返回图片资源、不改变景点版本，随后调用独立 `PUT /admin/api/scenic-spots/:spot_id/photos`，通过expected_version只替换照片，不触发等级采集、不提交文字；未传draft的旧调用保持原有追加并保存行为。省市通过 `PUT /admin/api/regions/:node_id/photos` 比较base_photos，并在最新目录CAS保存图片字段。未关联文件不因移除立即物理删除。城市表单隐藏“可在客户端选为城市”和已有节点的国家代码，保留值；新增国家仍需填写两位缩写。

等级表单可选“暂无等级/1A/2A/3A/4A/5A”，来源/状态/时间服务端托管，人工改值或清空不被自动覆盖；旧 PUT 省略保留，空串清空，null/非法值400。新建或来源变更在相同保存内尝试采集，以保持返回版本供后续图片上传使用；自动识别新建异步采集。provider-content/backfill 在原文字图片已同步时也独立补采等级，grades 为本批补齐数；查询失败不阻止原有资料同步。完整协议见 [景点等级](track-place-discovery.md#景区等级采集与下发) 和 [删除归档](track-place-discovery.md#后台景点删除归档)。

共享轨迹信息弹窗将 description 显示为“路线说明”，旁边的“AI 联网填写”调用 POST `/admin/api/tracks/:track_id/description/generate`，结果先在独立可编辑候选区展示原文与来源，原说明不变。确认后点击“一键代入并保存”，PUT `/admin/api/tracks/:track_id/description` 携带说明与source_version，三仓储CAS仅保存说明、manual/v0和更新时间，其他草稿保持原样，无需详情页保存。tracks写权限、同源与审计沿用既有链路，生成期间编辑/切换不覆盖，网络保存结果不确定时重新打开核实。说明规则v5为第一人称向朋友介绍，用数据提炼定性感受，不罗列运动数字或虚构见闻。历史轨迹无需进入自动队列，自动开关不关闭手动入口，仍共享租约与日预算。入口 `service/track_description_admin.go`、`kimi_track_description.go` 与 `repository/track_description.go`，协议见 `docs/api/admin-tracks.md`。

## AI 使用管理

新增 `/admin/ai_usage.html`，统计、筛选记录、实时中断、手动重试及功能暂停/预算/并发/价格配置。复用既有图表与后台权限审计；候选和实际采用独立记录。日请求上限0表示不限、默认为不限；旧路线共享日额度已停用。页面默认展示四项用量概览、正在执行和精简任务记录；更多筛选、统计图表及技术明细按需展开。“额度与设置”集中提供今日已用/剩余和日上限调整，保留当前草稿；统计及任务记录自动刷新。入口：internal/admin/ai_usage.go、internal/ai/、internal/repository/ai_usage*.go。详见 [运行说明](ai-usage.md) 与 [接口协议](api/admin-ai.md)。

### 景点相册的自动补齐

景点图片独立后台任务scenic_photos负责新景点与存量缺图、供应商外链到本站文件的迁移，不再依赖浏览器批量同步。目录、景点详情及轨迹详情只返回已保存本站图；后台仍可查看原外链、人工同步和编辑。photos_edited由服务端在实际相册编辑时设置，禁止自动补回人工清空的相册，人工文字编辑不误标记。不会覆盖名称/简介等资料。任务游标、负缓存、退避和跨进程租约见[图片独立巡检](track-place-discovery.md#景点图片独立巡检)。
