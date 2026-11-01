# AGENTS.md — internal/admin

> 本文面向 AI Coding Agent。仅记录 admin 模块的稳定约束、改动导航与提交前最小检查。
> 通用约束、目录总览、降级链等见仓库根目录 `AGENTS.md`，本文不重复。

## 模块定位

`internal/admin` 是 **track_server 的内置管理后台**，提供：

- 一个独立登录的 Web 控制台（HTML/CSS/JS 经 `go:embed` 内置）；
- 围绕 `AppRelease` 的发布、列表、删除接口；
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
- 查看、修正和删除用户轨迹；列表按 ID、城市、标题、运动类型、用户名、轨迹图、状态和操作展示，支持标题及用户名模糊搜索、城市和状态筛选，并保留游标分页。用户名同时展示头像；景点详情中的相关轨迹列表复用相同列顺序。列表不单独放删除按钮；编辑轨迹使用共享 `static/track_editor.js` 表单，底部删除动作需二次确认，删除为软删除并同步清理关联数据。
- 反馈与纠错页默认筛选待处理，展示状态统计、反馈用户名称和完整内容；点击查看会在弹窗中加载详情并处理反馈。`static/feedback_correction.js` 优先读取服务端 `correction` 快照，旧记录兼容完整的六种中英文纠错正文；不识别的内容仍按普通反馈显示，不改写原始正文。页面「纠错选项」可新增、改名、排序、启停，由 API 下发客户端。
- 纠错选项支持 description/description_en 与 notes_hint/notes_hint_en（可空、单行、各最多 160 字），后台配置与客户端展示共用协议，不按显示名称推导类别。反馈列表的 kind/track_id/correction_option_id 在 MySQL/Mongo/内存仓储分页前过滤，仅按结构化 correction 识别类型，旧文本纠错保留兼容展示。
- 纠错处理结果与状态分离：corrected/unchanged 配 resolved，unverified 配 ignored；新版后台提交 revision 作条件更新，终态必须填写结果及用户可见回复，冲突返回 409 且保留草稿。处理人只从后台 session 取值。服务与仓储入口为 internal/service/feedback_resolution.go 和 internal/repository/feedback_resolution.go；复用 correction_json/BSON，不新增表列。
- 纠错详情按轨迹查看权限异步读取当前资料，并复用共享 track_editor.js；来源 source_feedback_id 的轨迹保存同时要求轨迹操作与反馈操作权限，核对纠错与轨迹一致后，只在实际保存成功且字段变化时记录最近关联修改的字段名、处理人和时间。轨迹写入与反馈关联非跨仓储事务，关联失败返回 correction_warning，页面必须明确提示轨迹已保存。仅修改反馈状态不视为轨迹已修正；历史无结构化快照时不能伪造关联。后台详情额外返回 handled_by/track_change，业务客户端不下发管理员身份及内部改动记录。

- 用户管理支持昵称模糊搜索，以及内部人员、资料所在城市、性别、生日星座和当前账号限制状态组合筛选；同行管理按会话标题、运动类型、发起人昵称和发起人资料省市筛选（昵称/省市先在用户仓储解析为发起人 ID 集合，再由 `AdminCompanionListRepository` 在仓储层参与总数与游标分页，列表批量补充发起人昵称/头像和成员记录数），轨迹管理按标题和用户名提供服务端模糊搜索，轨迹列表还支持城市、状态和资料缺项筛选，均保留游标分页。

模块通过 `admin.NewModule(...)`、`Module.ConfigureEmailRecovery(...)`、`Module.RegisterRoutes(h)` 注入和注册，不被业务 handler 引用；Close 同时停止会话 GC 与邮件 worker。

## 页面性能约束

修改后台页面、共享脚本或列表接口时，必须检查以下边界；不能仅靠隐藏加载提示改善体感。

- **首屏主链路**：列表、筛选、详情基础字段不得等待图表库、地图底图、统计或外部资料回填。统计独立请求，先显示已返回的数字，再绘图；ECharts 统一通过 `AdminAssets.loadCharts()` 按需加载，共用一个可失败重试的 Promise，不重新引入阻塞式 vendor script。外部回填在首轮列表和统计展示完成后启动。
- **公共资源缓存**：CSS、JS、图标、地图 JSON 等嵌入资源使用内容版本 URL；HTML 引用由 `assets.go` 统一注入，动态 URL 使用 `AdminAssets.url()`。版本匹配才允许 immutable 长缓存，未带版本或旧版本保留 ETag 重验证；修改资源必须自然改变版本，禁止手写长期不变版本号。不要新增外网 CDN 依赖。
- **缓存隔离**：公共资源缓存不包含用户上传图片、业务数据、会话或权限。受保护 HTML/API 继续 no-store，每次受保护请求实时验证账号状态和权限；不得为提速缓存鉴权结果。压缩含身份的 HTML 不得进入共享静态 gzip 缓存。
- **局部更新成本**：MutationObserver 只处理新增或受影响子树，同批去重；禁止每次图表、列表或提示变化都扫描整个 document。权限状态刷新仍做全量应用，新增表单、字段和业务脚本重新启用 disabled 时必须及时恢复只读锁；观察器自身写 DOM 要避免递归触发。
- **列表数据与资源**：优先批量查询、按唯一用户/资源去重，禁止在逐行循环里重复读取同一作者。图片缓存未命中等确需等待的 I/O 使用有上限并发（当前 `list_assets.go` 为 4），保留请求取消与单次超时；响应顺序稳定，不创建每行一个无限制 goroutine，不提前返回尚未落盘的本地资源地址。
- **异步与验收**：快速筛选/刷新忽略旧响应，失败可重试，局部统计失败不阻断列表操作。自查首次进入、再次切页、慢图表/图片和只读账号路径；遵守根文件的验证约定，默认仅构建与静态自查，功能由用户手动验证。未实测不得宣称首屏提速比例或耗时。

## 卡片布局与边距约束

- **内容留白默认存在**：卡片里的标题、说明、错误提示、筛选和按钮不得贴边。共享内容内边距默认桌面 18px、窄屏 14px，复用 style.css 的统一规则；不要只给当前截图某一行补 margin，留下其他状态或同类页面继续贴边。
- **检查增强后的结构**：`static/ui.js` 的 `enhanceCard` 有两种结构：独立 `.panel.card > .card-body`，以及没有直接 H2 时的同节点 `.panel.card.card-body`（包括标题放在复合工具栏内的情况）。两种都必须有内边距，不能让 `.panel.card { padding: 0 }` 覆盖同节点内容。改共享 Card/CSS 时同时检查这两条路径，不能仅凭 HTML 里有 card-body 就认为留白生效。
- **贴边必须显式且局部**：只有纯表格或其他明确设计为满宽的区域可用 `p-0` 等标记；账号列表纯表格和景点纯表格是现有例子。不得把包含标题、说明和筛选的整个业务卡片设为零内边距。表格贴边时仍保留单元格留白，文字/表单恢复正常内容边距。
- **工具栏和宽表格不突破留白**：普通筛选工具栏使用专用 flex/grid 类，不直接借用 Bootstrap `.row`；`.row` 的负边距必须和成对的列 gutter 一起使用，不能让标题、筛选和表格边界错位。反馈页复用 `admin-toolbar feedback-toolbar`：标签/控件分成固定行，三个控件高度 40px，按钮独立分组且与控件底部对齐，分页说明放在工具栏外；中等宽度按钮整组换行，窄屏字段独占一行。不能仅加 align-items 而忽略标签高度、控件高度与换行结构。宽表格包在独立 `.table-responsive` 内，包含弹窗内的配置表，不能让整个卡片或弹窗横向溢出。
- **提交前布局自查**：修改后台页面或共享增强逻辑时，对照桌面和窄屏规则，检查卡片四边、标题/说明/表格对齐、工具栏换行，以及加载/空态/报错/展开详情后的留白；长文本或宽表格只能在各自内容区换行或滚动。沿用根文件的人工验收约定，未实看页面不得声称视觉验收通过。
- **展开详情不重分配列宽**：含可展开详情或长请求路径的业务表格，先明确列宽比例并用 colgroup + table-layout:fixed 固定，不能依赖自动内容测宽。最近操作记录的时间/操作人/操作内容/操作对象/结果固定为 18%/12%/34%/22%/14%，长路径、ID 和结果标签在单元格内换行，展开只改变行高；桌面最小表宽 900px，由外围表格容器局部滚动。767px 以下继续使用移动卡片布局，隐藏 colgroup 并解除最小表宽，详情/结果说明保持在内容列，不把桌面列宽强加给卡片。

## 硬性约束

- **独立鉴权链路**：admin 走自有的 `Authenticator` + `SessionStore`（cookie 名 `admin_session`），**不要复用** `internal/middleware.JWTAuthMiddleware` 也不要在 admin 路由里塞 `RequestMeta` 中间件。业务用户与管理员是两套体系。
- **账号独立持久化**：后台使用 `AccountManager` + `AccountStorage`，由 `cmd/server/main.go` 注入当前 MySQL/Mongo 的 `admin_account_state` 和 `admin_audit_logs`。账号状态以单行/单文档 JSON 加整体 version CAS 保存，账号内 `revision` 防止管理员覆盖彼此编辑，`security_version` 使停用/重置密码跨进程失效。不得复用 App 用户表，也不得在数据库不可用或业务仓储降级时回退到环境变量鉴权或内存账号。
- **首次迁移**：仅账号状态尚未初始化时读取 `ADMIN_ACCOUNTS=user:bcryptHash;...` 与兼容的 `ADMIN_USERNAME` / `ADMIN_PASSWORD_HASH`；同名优先多账号配置。必须包含 `admin`：该账号迁移为管理员，其他既有账号迁移为操作员，默认所有当前业务页面只读，不含账号管理。保留既有密码。已迁移后数据库是唯一账号权威，环境变量不再新增、覆盖或恢复账号。新增账号与重置密码要求下次登录先改密码；新增操作员未授权页面默认无权限。
- **启用与故障关闭**：数据库未初始化且无初始账号时不挂载后台；已有持久化账号即使移除初始化环境变量仍启用。账号库不可用时登录/受保护请求拒绝（通常 503）；未挂载时保持 404。`USE_IN_MEMORY` 不提供后台账号降级。
- **身份与权限**：管理员拥有全部业务模块权限，只有管理员可访问 `/admin/accounts.html`、`/admin/api/accounts*` 与审计日志；操作员每个模块只有 `none/read/write` 三档，write 包含页面所有业务操作。HTML、API、受保护静态资源均校验；未知模块默认拒绝。`permissions.go` 是页面/接口权限映射权威，账号模块清单在 `accounts.go`。新增页面/API 时同步映射，不能只隐藏导航。
- **跨页面与汇总**：景点相关轨迹的查看/编辑同时校验轨迹模块权限。工作台按各业务模块过滤指标，其他页面通过 `/dashboard?scope=tracks|track_submissions|feedbacks` 只读取自身统计。只读景点页不得触发供应商回填或编辑；共享轨迹弹窗支持只读。`shell.js` 统一身份标签、导航、只读控件和退出，不再由各业务脚本覆盖账号展示。
- **轨迹资料与景点候选**：景点管理页不再展示全局候选面板；共享 `track_editor.js` 通过 `place_candidates.js` 把单条轨迹的识别结果放在详情弹窗顶部，所有加载共享编辑器的页面都要先加载该模块。读取 GET `/tracks/:track_id/place-candidates` 需轨迹查看权限，既有 `/place-candidates/:track_id/confirm|dismiss` 写请求归属轨迹操作权限。`AdminUI.form.beforeFields` 挂载独立候选区（`place_candidates.js`）与轨迹地图区（`track_map_preview.js`），候选确认/忽略立即生效并刷新来源列表，不提交未保存的轨迹资料。源摘要失效返回 409，禁止使用过期候选。后台人工候选放宽为「围住景点 + 采样途经附近（matched_samples≥1）」，自动判定 unmatched 的记录同样可由人工确认关联；`POST /tracks/:track_id/place-refresh`（轨迹操作权限）同步触发单条重新识别，复用后台任务同一锁与配额，任务忙返回 409。详情还返回主景点 `primary_scenic_spot`（点击跳 `/admin/scenic_spots.html?spot_id=` 深链打开详情）与地图索引简化轨迹线 `geo`（GCJ02）；轨迹地图复用高德 Web Key，经 `/tracks/map-config` 与 `/tracks/amap-service` 代理（轨迹模块权限），不依赖景点模块权限。
- **轨迹资料缺项口径**：`GET /tracks?missing=any|place_candidate|scenic_spot|description|city|previews` 在分页和总数统计前筛选；缺简介/城市按空文本，缺预览为四图任一缺失，未关联景点为无有效 draft/published 目录关联；任意缺项取各项并集。待确认 ID 由当前候选队列源摘要/规则/轨迹可索引校验取得，不触发供应商查询。MySQL 使用原生条件/关联，Mongo 使用 lookup + facet，内存由 main 注入景点仓储后使用关联快照；公共逻辑见 `repository/admin_track_missing.go`，新接口见 `docs/api/admin-tracks.md`。
- **账号设置入口**：`shell.js` 的共享账号区只常驻显示名称、身份标签与齿轮。齿轮复用内置 `tabler.Dropdown`，桌面侧栏向上展开、移动顶栏向下展开，支持点外部关闭和键盘操作；菜单读取当前 session 和 `/admin/api/me` 显示登录名、名称、身份、最近登录，并提供“账号安全”（保留 password.html URL，包含本人改密与邮箱验证）和退出入口。仅 GET 当前账号信息，不下发或展示密码/哈希。`ui.js` 不得给 dropdown-item 和下拉触发按钮自动套普通主按钮外观。退出请求失败保留当前页并在菜单提示，不把请求失败当成退出成功。
- **本人邮箱与密码找回**：`email_challenges.go` 的 8 位验证码使用随机盐与服务端 HMAC 摘要（密钥从 SMTP 专用密码独立派生，凭证轮换使旧码失效），在现有账号 JSON 中与错误次数/有效期/发信配额一起 CAS 持久化；不修改 AccountStorage 接口、不回退内存。10 分钟、一次有效、累计 5 次错误失效，每用途间隔 60 秒、每账号每小时共 10 封。原邮箱、新邮箱、密码找回三用途不能互换；错误尝试也必须提交，不能因返回业务错误回滚计数。改密、停用、绑定成功使全部旧会话/挑战失效。
- **邮箱归本人管理**：security API 只使用 session 的账号，不接受目标 username；绑定要当前密码和新邮箱验证，更换还要原邮箱验证。临时密码账号必须先改密，普通管理员编辑接口不得写入邮箱、验证码或现有密码。账号列表仅下发 recovery_email_bound/can_reset_password，完整邮箱只下发本人 GET security。已验证账号的管理员重置在服务端直接拒绝；全部启用账号绑定后持久化 legacy_password_reset_disabled，不因新建未绑定账号或 SMTP 暂不可用自动重开。未迁移账号暂保留标注“应急重置”的旧接口，不能提前让老账号失去恢复渠道。
- **公开找回与发信**：forgot-password.html 公开，必须先将申请放入有界队列再查账号，匹配/不匹配统一 202 提示；不能把 SMTP 耗时和错误同步回传来枚举邮箱。公开接口校验同源、限制 socket peer IP（不直接信任转发头），验证码重试有跨进程持久化上限。email_recovery.go 限 2 worker/16 队列，email_sender.go 总并发 2、连接与操作超时，只允许证书校验通过的 TLS 1.2+；不回退明文 SMTP/本地 sendmail，不记录 SMTP 响应、凭证、邮件正文、验证码或摘要。不在响应/邮件中发送登录密码，不自动签发登录会话。公开找回完成单独审计，日志不可用不改密码，actor=email-recovery 代表未登录流程，不能冒充账号本人发起。普通修改/邮箱验证复用受保护审计；后台安全通知为尽力投递，通知失败不能把已完成改密误报失败。
- **邮件配置与上线**：main 注入 ADMIN_SMTP_HOST/PORT/USERNAME/PASSWORD/FROM/TLS_MODE；未配置或非法时关闭邮件能力，登录/本人原密码改密继续工作，页面明确提示暂不可用。生产现有 OSS/STS 不是邮件服务；不要擅自开通云产品、复用 OSS AK/SK、发测试邮件或把真实 SMTP 凭证写入仓库。接入与迁移、公开响应/队列重启边界、HTTPS 要求见 `docs/admin-email-recovery.md`，示例配置 `deploy/admin-email.env.example`。
- **账号保护与日志**：整体 CAS 原子保护最后一个启用的管理员，停用/改密码使所有会话失效，调整权限由后续请求实时读取。后台写请求校验同源、记录发起和 HTTP 结果；日志不可用时拒绝发起写操作，不存正文、密码或哈希。后置结果写入失败时保留发起日志并记录 server log，不能声称业务与日志跨库事务原子。所有后台受保护数据响应使用 no-store；静态 HTML 禁止通过 `/admin/static/*` 访问。
- **日志账号筛选**：最近操作记录提供账号下拉选择（含停用账号）；`GET /admin/api/audit-logs?username=...` 按不可改的登录名精确匹配，MySQL/Mongo 必须先筛选再取最近 100 条。不传或空串表示全部账号。选项来自后台账号列表，刷新保留选择，快速切换忽略旧请求响应。
- **操作记录可读性**：账号页以“操作人、中文操作内容、对象、结果”展示日志，`static/audit_log.js` 统一维护方法/路径到业务行为的映射，新增被审计操作时同步更新；`accounts.html` 在 accounts.js 前加载该模块。原始方法、路径、状态和记录 ID 放在折叠详情，用 textContent 输出，保留账号筛选及同一 attempt/result 合并。状态 0 或 3xx 不能标成功，结果仅反映请求响应，批处理不声称逐项全成功；说明景点供应商回填可由页面自动分批触发，不能仅凭接口推定每条都由人工点击。旧记录没有正文/字段差异，不推测通过或驳回、启用或停用等具体改动，不为展示描述逐条查业务库；未知动作显示通用描述和原始详情。
- **会话仅内存**：`SessionStore` 使用 `sync.RWMutex` + `map`，进程重启即失效。**不要替换为持久化存储**；管理员重新登录是合理代价。
- **静态资源经 `go:embed`**：`static/` 目录通过 `//go:embed static` 内置进二进制；新增前端文件后**必须放到** `internal/admin/static/`，否则二进制找不到。`/admin/static/*filepath` 路由只读 `staticFS`，不读磁盘。
- **安装包本机上传**：管理后台安装包上传走 `POST /admin/api/releases/upload-package`，服务端校验 APK/IPA 并读取包内版本名、版本号/Build、应用标识，同时计算大小和 MD5；前端使用返回的下载地址自动填写 `package_url`。发布页面固定 Android/APK，版本名和版本号只由包解析结果写入隐藏字段；服务端接口仍保留 iOS 兼容性。不要再把发布包上传链路改回浏览器 OSS 直传。
- **路径前缀固定 `/admin`**：所有路由都注册在 `h.Group("/admin")` 之下；前端 `fetch` 也写死 `/admin/api/...` 与 `/admin/static/...`。修改前缀需要同时改 `routes.go` 与 `static/*` 中的 URL。

## 目录与权威文件

```
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
| 主景点目录 | `scenic_spot_handlers.go` + `ScenicSpotCatalogService`；页面 `static/scenic_spots.html`、`static/scenic_spots.js`，页面与导航显示名为「户外目的地」（目录涵盖景区/公园/山系/湖泊，内部 SCP ID、API 路径与模块名 scenic_spots 不变）；目录与调用统计由 `repository.ScenicSpotRepository` 三种实现提供；自动/人工记录均直接下发客户端，归档/合并记录隐藏；首次加载时分批同步存量供应商资料；新建时可点击地图 POI 热点或搜索附近 POI，将资料先放入备选预览，一键代入后写入供应商来源与资料/图片；详情以弹窗编辑并可拖动排序图片 |
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
| GET | `/admin/api/users/statistics` | 鉴权 | 用户总数、性别/星座/年龄分布、`track_output` 云端轨迹产出分层和 `user_cities` 用户资料所在城市分布；每个用户只计入当前设置的一个城市，未设置计入未知城市 |
| GET | `/admin/api/companions/statistics` | 鉴权 | 同行会话当前活跃数、所选周期创建/结束/自动结束数、平均参与人数 |
| GET | `/admin/api/companions/usage-statistics` | 鉴权 | days=1/7/30，创建与结束趋势、按本周期创建房间统计的实际参与人数分布 |
| GET | `/admin/api/route-groups/statistics` | 鉴权 | 全量 active 路线组中心点、覆盖城市和成员轨迹数分布，不受列表类型/城市筛选及 100 条上限影响 |
| POST | `/admin/api/releases` | 鉴权 | 发布/覆盖一个版本（按 `(platform, version_code)` 幂等） |
| DELETE | `/admin/api/releases/:id` | 鉴权 | 删除一条发布记录 |
| POST | `/admin/api/releases/upload-package` | 鉴权 | multipart 上传安装包到 `<LogDir>/static/release/<platform>/` |
| GET | `/admin/api/releases/upload-token` | 鉴权 | 旧 OSS 直传 STS 凭证接口，前端不再使用 |
| GET | `/admin/api/static/*filepath` | 鉴权 | 后台静态资源代理，从 `<LogDir>/static/` 读取头像等资源；后台页面不要直接访问需业务 JWT 的 `/api/v1/static/*` |
| GET | `/admin/api/users` | 鉴权 | 用户列表，支持 `title` 昵称模糊搜索、`is_internal` / `city_code` / `gender` / `zodiac_sign` / `restricted` 组合筛选及 cursor 翻页；所有条件参与总数统计，返回按当前页批量统计的 `last_login_at` |
| GET | `/admin/api/users/:user_id` | 鉴权 | 用户资料详情，包含头像、签名、性别、生日、手机号、内部人员标识和最近确认所在地 |
| PUT | `/admin/api/users/:user_id` | 鉴权 | 编辑用户昵称、签名、性别、生日、手机号和 `is_internal`；不修改所在地、头像、背景或账号关系 |
| PUT | `/admin/api/users/:user_id/internal` | 鉴权 | 仅更新 `is_internal` 内部人员标识；普通资料更新保留该字段 |
| GET | `/admin/api/users/:user_id/restrictions/current` | 鉴权 | 查询当前账号限制；无生效限制返回 404 |
| POST | `/admin/api/users/:user_id/restrictions` | 鉴权 | 创建账号限制；`expires_at` 为空表示永久，操作人取 admin session 用户名 |
| DELETE | `/admin/api/users/:user_id/restrictions/current` | 鉴权 | 解除当前账号限制 |
| GET | `/admin/api/place-candidates` | 轨迹查看 | 保留兼容的候选队列 API，页面改为详情单条处理 |
| GET | `/admin/api/tracks/:track_id/place-candidates` | 轨迹查看 | 单条轨迹当前有效候选，404 无候选，409 源变化 |
| POST | `/admin/api/place-candidates/:track_id/confirm` | 轨迹操作 | 确认指定候选为轨迹主景点并同步内部景点关联 |
| POST | `/admin/api/place-candidates/:track_id/dismiss` | 轨迹操作 | 忽略当前轨迹的景点候选 |
| GET | `/admin/api/companions` | 鉴权 | 同行会话列表，支持 `title` 模糊搜索、`owner_name` 发起人昵称模糊搜索、`track_type` 精确筛选、`province_code` / `city_code` 按发起人资料所在地筛选及 cursor 翻页；返回发起人昵称/头像与成员记录数 |
| GET | `/admin/api/tracks` | 鉴权 | 轨迹列表，支持 `title` / `user_name` 模糊搜索、`city_code` / `status` / `missing` 筛选及 cursor 翻页 |
| POST | `/admin/api/tracks/import` | 轨迹操作 | 批量导入轨迹文件归属内部人员，multipart `files`（≤50）+ `owner_user_id` + `track_type` + `coordinate_system`，逐文件返回结果 |
| GET | `/admin/api/cities` | 鉴权 | 返回内置城市/省份 Code、名称与省市层级，供轨迹和景点选择器使用 |
| GET | `/admin/api/tracks/:track_id` | 鉴权 | 返回 `{track, geo?}`：轨迹详情、主景点（track.primary_scenic_spot）与地图索引简化轨迹线（GCJ02） |
| POST | `/admin/api/tracks/:track_id/place-refresh` | 轨迹操作 | 同步触发单条轨迹重新景点识别；任务忙 409，无索引 404 |
| GET | `/admin/api/tracks/map-config` | 轨迹查看 | 轨迹详情地图的高德 Web Key 配置（service_host 指向 `/admin/api/tracks/amap-service`） |
| GET | `/admin/api/tracks/amap-service` | 轨迹查看 | 高德 JS API 服务代理（与景点地图同一实现） |
| PUT | `/admin/api/tracks/:track_id` | 鉴权 | 更新轨迹业务字段（不改 ID、轨迹点与创建/更新时间）；城市/类型更新已有地图索引，状态/进行状态/原始文件变化时清理并按可索引条件重新排队 |
| DELETE | `/admin/api/tracks/:track_id` | 鉴权 | 删除轨迹：标记 `status=0`，并清理收藏、地图索引任务、geo index、路线组成员 |
| GET | `/admin/api/feedbacks` | 鉴权 | 意见反馈列表，补充用户名称，支持 `status` / `app_version` / `phone` / `cursor` / `limit` |
| GET/PUT | `/admin/api/feedback-correction-config` | 鉴权 | 读取含停用项的完整配置，按 version 条件保存；冲突返回 409 |
| GET | `/admin/api/feedbacks/:feedback_id` | 鉴权 | 意见反馈详情 |
| PUT | `/admin/api/feedbacks/:feedback_id/status` | 鉴权 | 更新反馈处理状态与用户可见 `reply`；`resolved` 时 `reply` 必填 |
| GET | `/admin/api/feedbacks/:feedback_id/images/:image_id` | 鉴权 | 读取反馈图片 |
| GET | `/admin/api/analytics/sync-summaries` | 鉴权 | 埋点 OSS 同步摘要列表，支持 `status` / `limit` / `offset` |
| GET | `/admin/api/route-groups` | 鉴权 | 路线组列表，支持 `track_type` / `province_code`（服务端展开为省内城市集合）/ `city_code` 筛选与 `limit` / `offset` 分页；返回 `total`、`next_offset`、`has_more`、`area_id` 及目录区域摘要 |
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

### 后台账号新增路由

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/admin/accounts.html` | 管理员 | 账号与最近操作日志 |
| GET | `/admin/password.html` | 登录 | 本人账号安全，临时密码账号仅可先改密 |
| GET | `/admin/forgot-password.html` | 公开 | 邮箱找回密码页面 |
| GET | `/admin/access.html` | 登录 | 授权页面入口；无权限时联系管理员 |
| GET | `/admin/api/accounts` | 管理员 | 账号列表与模块目录，不下发密码哈希 |
| POST | `/admin/api/accounts` | 管理员 | 新建账号，临时密码必须修改 |
| PUT | `/admin/api/accounts/:username` | 管理员 | 按 revision 修改名称、身份、状态和权限 |
| POST | `/admin/api/accounts/:username/reset-password` | 管理员且迁移期 | 仅未绑定邮箱、旧入口未永久关闭时可应急重置 |
| POST | `/admin/api/password` | 登录 | 验证原密码、修改本人密码并使全部登录失效 |
| GET | `/admin/api/security` | 本人 session | 本人邮箱验证状态和邮件能力，不下发挑战摘要 |
| POST | `/admin/api/security/email/current-code` | 本人 session | password；发送原邮箱换绑证明码 |
| POST | `/admin/api/security/email/send-code` | 本人 session | password/email/current_code；新邮箱验证，换绑必须有原邮箱证明 |
| POST | `/admin/api/security/email/verify` | 本人 session | code；验证保存并退出全部登录 |
| GET | `/admin/api/password-recovery/status` | 公开 | 是否已配置邮件能力，不含账号信息 |
| POST | `/admin/api/password-recovery/request` | 公开 | username/email，通用 202，异步有界发送 |
| POST | `/admin/api/password-recovery/complete` | 公开 | username/email/code/password/confirmation；验证后设置密码，不自动登录 |
| GET | `/admin/api/audit-logs` | 管理员 | 可选 username 精确筛选，返回筛选后最近 100 条发起/结果日志 |

## 关键流程

**登录**：
```
POST /admin/api/login {username,password}
  → AccountManager 从持久化状态取账号 → bcrypt 校验 → 更新最近登录
  → admin=管理员，其余既有账号=只读操作员（仅首次导入）
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

用户管理概况位于用户列表上方，不单独展示用户总数卡片；桌面采用紧凑三列，左侧性别、星座两个 ECharts 饼图上下排列，中间为用户所在省份地图，右侧上下展示年龄分布和轨迹产出分层柱状图。页面使用 `/admin/api/users/statistics` 一次读取性别、生日星座、`age` 年龄、`track_output` 云端轨迹产出分层和 `user_cities` 用户资料所在城市分布；MySQL、Mongo、内存仓储均直接按 `users.city_code` 计数，不再使用轨迹记录，每个用户只计入当前设置的一个城市，未设置计入未知城市。城市按省份汇总后通过内置 `static/china_provinces.json` 和 ECharts 矢量地图展示，不加载高德 JS 或地图 Key，也不显示颜色深浅图例。地图采用白色背景与浅蓝到深蓝配色，图表和地图保留内侧留白；过滤 `100000_JD` 南海诸岛元素，不展示南海附图；主图使用 `boundingCoords=[[73,54],[136,17]]` 限制中国主体经纬度范围并垂直居中放大，避免顶部裁切和南部空白，不展示辅助说明文字。省份上显示人数，超过一万用 `w` 缩写；点击省份更新地图下方该省城市用户数 Top 10，海外城市和未知城市的小统计也位于地图下方。统计区右上角“刷新数据”统一重新读取全部顶部统计，保留仍有数据的已选省份；加载时禁用按钮，失败保留上次数据并提示。用户列表头像和昵称合并为“用户”列，头像为圆形；不展示创建时间，改展示性别、服务端按生日派生的星座和最近确认城市。内部人员开关移到详情弹窗，与其他资料一起通过 `PUT /admin/api/users/:user_id` 保存；列表单元格垂直居中，内部用户头像右下角显示 `inner` 标签，不再占用独立列。操作区使用“查看详情 + 限制开关”，开关取消或失败恢复原状态。详情弹窗通过 `GET /admin/api/users/:user_id` 读取资料及内部标识，所在地只读，昵称、签名、性别、生日、手机号和内部人员状态可编辑并保存，保存后刷新当前页。用户列表和详情接口的 `id` 一律以 JSON 字符串下发，避免 int64 用户 ID 被 JavaScript Number 截断后请求错误用户。用户列表的上次登录时间按当前页 user_id 批量查询 `login_log` 的最大 `created_at`，不得用用户资料 `updated_at` 代替；Mongo 登录日志未实现时返回无登录记录。

年龄以服务端当地日期计算周岁，分为 18 岁以下、18–24、25–34、35–44、45–54、55 岁及以上和未知；1900 年以前、未来或无法解析的生日归入未知。轨迹产出按全量现存用户的有效已完成云端轨迹数分为 0、1、2–5、6–20、21 条及以上，包含公开和私密轨迹，排除软删除、进行中、原始文件地址为空及已无所属用户的轨迹；本地未上传记录不计入。MySQL、Mongo、内存仓储保持同口径，生日使用完整日期分组后统一派生年龄和星座，轨迹在仓储层聚合，不逐用户查询；不新增表或客户端协议。顶部统计沿用全量用户口径，不随下方列表筛选变化。

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

后台地图模式只增加“路线空间覆盖”和“路线组规模分布”两张图；与友同行只增加“创建/结束趋势”和“参与人数分布”，使用近 1/7/30 天切换。`AdminDashboardStatisticsRepository` 的 `GetAdminMapModeStatistics` / `GetAdminCompanionUsageStatistics` 在 MySQL、Mongo、内存仓储中只读聚合；不新增表、客户端协议或业务埋点。实现位于 `internal/repository/admin_feature_statistics*.go`，模型为 `internal/models/admin_feature_statistics.go`，后台入口为 `feature_statistics.go`。地图按全量 active 路线组中心 GCJ02 坐标展示；规模按当前 member_count 分为 0、1、2–5、6–20、21 条及以上，城市去重。同行使用趋势按创建/结束各自发生时间独立计数，不以同周期结束数除以创建数作为完成率；人数分布以本周期创建的会话为 cohort，按服务端成员记录 user_id 去重，包含曾离开或移出的成员，不使用 max_members 或客户端 actual_participant_count。没有成员记录的历史会话单列未知，平均人数只计算有成员记录的会话。近 1 天为滚动 24 小时按小时分桶，7/30 天按北京时间自然日含当天补齐空桶。统一刷新保留周期，切换失败回退到上次成功周期；两页不再增加排行和重复数字卡片。

`static/admin_usage_charts.js` 共用空间聚合、折线与分布图；`static/feature_statistics.js` 是地图模式/同行页的轻量控制器，景点页通过 `scenic_spot_charts.js` 复用同一地图与趋势组件。组件均使用已内置 ECharts 和省界数据，不访问高德图表 API；地图保留视野并支持点选详情，页面按窄屏上下排列。地图模式统计刷新同时刷新列表，与友同行沿用页面顶部统一刷新按钮。

**运营聚合路线**：
```
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
  → 「批量导入」弹窗：选择归属人（内部人员下拉，头像+昵称，GET /admin/api/users?is_internal=true）、运动类型、坐标系与多个轨迹文件
  → POST /admin/api/tracks/import（multipart）
       → AdminTrackImportService 逐文件解析（复用地图索引解析器）→ 服务端直传 OSS 归属人目录
       → TrackService.CreateTrack 建档（source_tag=manual_seed，已完成），自动排队地图索引与成就结算
       → 城市/地址由内置行政区目录离线解析；不生成预览四图与路线描述，由客户端 DevMode 后续补齐
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
轨迹只保存内部景点关联；外部来源 ID 留在景点目录。存量回填对没有可用来源 ID 的合格轨迹强制刷新坐标查询缓存，调用地图 API；高德每日上限按配置执行（默认不设日上限），每轮最多 10 次真实请求；Google 不受服务端每日或单轮请求数上限约束。索引刷新失败时回退到已有索引继续匹配并单独计数；结果有明确重复命中才创建空白目录项。后台景点页先展示四张概况卡、景点空间分布图和可切换近 1/7/30 天的新增入库趋势，不再单独展示缺项图、关联轨迹 Top 10 或供应商调用量图表；底层供应商每日计数继续保留用于排查。统计范围排除 archived/merged；有效轨迹包含公开和私密、已完成且 raw_track_url 非空的未软删除轨迹，失效关联不计入，跨景点按每条轨迹唯一主景点关系统计。覆盖城市合并景点自身 city_code 与有效关联轨迹 city_code 去重，无法识别的城市不计入。缺项为缺简介、无图片、缺有效非零坐标或缺地址，一处景点可有多项；待完善数按景点去重。`repository.CountCompletedScenicSpotTracks` 在 MySQL/Mongo/内存三种仓储中批量关联轨迹计数与城市，`repository.ScenicSpotMissingFields` 定义统计/筛选口径。点击待完善卡片会清空其余列表条件并使用 `GET /admin/api/scenic-spots?missing=any|description|photos|coordinates|address` 在仓储层筛选，筛选参与 total 和 cursor 分页；默认目录只列有效景点，显式 status 查询保留维护用途。列表支持加载更多，不仅筛选首批记录。统计区刷新为只读操作，加载中禁用按钮，失败保留上次数据并提示；统计加载不读取供应商调用历史或触发供应商请求。`static/scenic_spot_charts.js` 通过共用 `admin_usage_charts.js` 复用内置 ECharts 和中国省界底图：全量有效景点按已有坐标转换为 GCJ02 后展示，像素网格聚合随缩放重算，点击聚合点放大并可选择景点详情，缺坐标单独显示；刷新/周期切换保留当前视野。趋势按北京时间及景点 created_at 统计入库数，含回填、后续归档/合并记录；近 1 天为滚动 24 小时按小时分桶，近 7/30 天含当天按自然日分桶，空桶补零，区间为 [start,end)，不将资料 updated_at 当新增。目录为紧凑整行，展示分类、简介和图片；“相关轨迹(N)”按钮展示完整关联轨迹数量，目录列表通过仅后台使用的 `related_track_count` 下发；计数与相关轨迹弹窗一致，只排除已不存在的轨迹，不套用顶部“有效已完成轨迹”的过滤口径。`RelatedTrackStatistics` / `repository.CountScenicSpotTracks` 在 MySQL/Mongo/内存仓储按当前页批量计数并补全关联城市，不逐景点查询，也不受原先每景点首 100 条的限制。弹窗接口返回 `total`，展示总数与已加载数量，编辑/删除后重新读取并回填按钮数量。“相关轨迹”操作通过景点轨迹列表接口，以轨迹管理页相同的表格字段（不含开始时间）展示关联轨迹；点击行或编辑按钮打开共享 `static/track_editor.js` 可编辑表单，保存或删除后刷新关联列表。景点详情在弹窗内编辑文字资料和图片，并显示供应商坐标、坐标系及位置地图。供应商文字同步到景点 JSON 资料，图片与人工上传图片共用 `photos` 有序数组；拖动排序和上传追加顺序都会随保存结果下发。Google 坐标按 WGS84 保存，高德坐标按 GCJ02 保存；来源同步只填补空坐标，不覆盖既有坐标，已同步记录单独请求坐标字段，避免再次请求照片媒体。城市信息只读展示，不作为编辑字段。`manual_edited` 表示资料由管理员创建或编辑保存；自动同步资料仍显示“自动”。页面隐藏来源 ID、轨迹关联编辑、合并拆分操作，但提供相关轨迹列表和编辑入口；其余数据/API仍由服务端保留，供客户端景点关联及维护流程使用。景点仍沿用内部 draft/published/archived/merged 状态机；页面展示的“自动/人工”是内容维护方式，不替代发布状态。未发布目录不会覆盖旧轨迹摘要。后台上传图像走受鉴权的 `/api/v1/static/scenic_spots/*` 资源路由，Android 详情页可读取。更新 MySQL/Mongo/memory 任一仓储时，保持三种实现、版本冲突控制和审计行为一致。
景点页统计开始于该计数能力上线，不回填旧请求；计入已关联内部景点的来源详情、照片请求和 ID 核验，存量轨迹需先运行回填，按坐标执行的识别搜索不计入单景点排行。发布覆盖率以关联轨迹计数为准，目标是帮助运营优先补齐高调用景点的自有内容。
所有 Google Places 查询均不受服务端每日预算或单轮请求数上限约束，由 Google API 自身配额控制。

Kimi 景点简介使用 `MOONSHOT_API_KEY` 和可选的 `KIMI_SCENIC_DESCRIPTION_MODEL` 环境变量，默认模型为 `kimi-k2.6`。存量景点详情弹窗的“AI联网查询”链接把当前名称、城市、分类和地址提交给 `/admin/api/scenic-spots/description/generate`，结果只填入简介输入框，点击既有保存按钮后才入库；该查询不自动批量触发。新建记录在创建前对空简介尝试生成，成功后随景点资料一起保存，失败时仍可创建并保留空简介。模型检索与摘要通过 Kimi 官方 `/v1/tools/search_pro` 和 `/v1/chat/completions` 接口完成，简介限制 200 字。
[admin UI] /admin/analytics.html
  → GET /admin/api/analytics/sync-summaries?status=&limit=&offset=
  → AnalyticsRepository.ListSyncSummaries / CountSyncSummaries
  → 页面展示任务状态、扫描/上传/失败文件数、总字节数、OSS 前缀与 files_json 文件明细
```

## 改动导航

- **加一个管理后台 API**：在业务 handler 写实现 → 在 `routes.go` 的鉴权 api 组挂载 → 同步 `permissions.go` 的模块/读写/跨页面规则。需要操作者信息时用 `h.auth.SessionFromRequest(c)`；未知模块不能默认放行。
- **展示业务静态资源**：后台页面不要直接使用 `/api/v1/static/*`，应由 handler 改写为 `/admin/api/static/*`，通过 admin session 鉴权后读取 `staticRoot`。
- **加一个公开页面/资源**：把文件放进 `static/`，在 `routes.go` 用 `serveEmbedded(...)` 或走 `/admin/static/*filepath`。**不要新增第二个 `go:embed`**。
- **加一个侧边/顶部入口**：统一修改 `static/shell.js` 的导航配置；页面只需在自身脚本前加载 `shell.js`，不要再复制一份导航。
- **后台组件样式**：页面先加载内置 `vendor/tabler.min.css`，再加载 `style.css`；通用 Card / Button / Form / Table / Badge、移动端表格卡片化及图片点击全屏预览由 `static/ui.js` 统一增强，新增页面应加载 `vendor/tabler.min.js`、`shell.js`、`ui.js` 后再加载页面脚本，不要另写一套原生组件外观。地图图片或其他不应预览的图片可标记 `data-no-preview`。
- **改 cookie 策略**：只在 `auth.go` 改；保持 `HttpOnly=true`，跨站调用前提下才考虑 `SameSite=None + Secure`。
- **调整 session TTL**：改 `routes.go` 中 `NewSessionStore(12 * time.Hour)`，不要在 cookie 与 store 两边写不一致的过期时间。

## 提交前最小检查

- 构建：`go build ./...`
- 测试遵循根 AGENTS 的“默认验证方式”：默认不新增或运行，由用户手动验证；仅在用户明确要求时执行 `go test ./internal/admin ./internal/service` 或其指定范围，不自动扩大为全量测试。
- 路由变更：核对 `routes.go` 与 `static/*.js` 中前端 fetch 的 URL 是否一致。
- 静态资源新增：确认文件位于 `internal/admin/static/` 下（`go:embed static` 才能打包）。
- 页面/共享脚本/列表改动：逐项自查本文“页面性能约束”，对改动的 JS 执行 `node --check`；特别检查是否重新引入同步图表依赖、无版本动态资源、全页观察器扫描或重复逐行 I/O。不得把这些静态检查当作实际性能测量。
- 页面/Card/样式改动：逐项检查“卡片布局与边距约束”，同节点与子节点 card-body 的内边距必须都覆盖桌面/窄屏；任何 p-0 都要确认只作用于有意满宽的区域，不能消除标题、说明或工具栏留白。
- 接口契约变更：同步更新 `docs/api/feedback.md` 与 `docs/api/route-index.md`；`track_api.md` 仅保留轻量入口。

## 常见误解

- **误解**：管理后台没访问到，往业务 `auth` 组里塞路由。
  **事实**：admin 完全独立挂在 `/admin`，不属于 `/api/v1/auth` 组；登录态也是 cookie，不是 JWT。
- **误解**：可以用 `OSSTokenService.GetUploadCredential` 上传 APK。
  **事实**：那是按 userID 隔离的用户目录，policy 不允许写入 `release/`，必须用 `GetReleaseUploadCredential`。
- **误解**：进程重启后会话还在。
  **事实**：`SessionStore` 是纯内存实现，重启即清空，必须重新登录。
- **误解**：改环境变量可以覆盖后台密码或恢复已停用账号。
  **事实**：环境变量仅首次导入；此后以独立持久化账号为准。数据库不可用时拒绝鉴权，不重建临时管理员。
