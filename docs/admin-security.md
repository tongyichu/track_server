# 管理后台权限与操作边界

从原 AGENTS.md 迁出的开发参考；修改对应功能前按 AGENTS 索引读取相关章节，无需每次通读。验证、授权和通用行为以当前 AGENTS.md 为准；实现细节以源码和最新接口文档为准。

代码短路径相对 internal/admin/，docs/ 路径相对仓库根目录。修改鉴权、账号、邮件、审计或跨模块访问前读取对应条目。

## 目录

- [硬性约束](#硬性约束)
- [常见误解](#常见误解)

## 硬性约束

- **独立鉴权链路**：admin 走自有的 `Authenticator` + `SessionStore`（cookie 名 `admin_session`），**不要复用** `internal/middleware.JWTAuthMiddleware` 也不要在 admin 路由里塞 `RequestMeta` 中间件。业务用户与管理员是两套体系。
- **账号独立持久化**：后台使用 `AccountManager` + `AccountStorage`，由 `cmd/server/main.go` 注入当前 MySQL/Mongo 的 `admin_account_state` 和 `admin_audit_logs`。账号状态以单行/单文档 JSON 加整体 version CAS 保存，账号内 `revision` 防止管理员覆盖彼此编辑，`security_version` 使停用/重置密码跨进程失效。不得复用 App 用户表，也不得在数据库不可用或业务仓储降级时回退到环境变量鉴权或内存账号。
- **首次迁移**：仅账号状态尚未初始化时读取 `ADMIN_ACCOUNTS=user:bcryptHash;...` 与兼容的 `ADMIN_USERNAME` / `ADMIN_PASSWORD_HASH`；同名优先多账号配置。必须包含 `admin`：该账号迁移为管理员，其他既有账号迁移为操作员，默认所有当前业务页面只读，不含账号管理。保留既有密码。已迁移后数据库是唯一账号权威，环境变量不再新增、覆盖或恢复账号。新增账号与重置密码要求下次登录先改密码；新增操作员未授权页面默认无权限。
- **启用与故障关闭**：数据库未初始化且无初始账号时不挂载后台；已有持久化账号即使移除初始化环境变量仍启用。账号库不可用时登录/受保护请求拒绝（通常 503）；未挂载时保持 404。`USE_IN_MEMORY` 不提供后台账号降级。
- **身份与权限**：管理员拥有全部业务模块权限，只有管理员可访问 `/admin/accounts.html`、`/admin/api/accounts*` 与审计日志；操作员每个模块只有 `none/read/write` 三档，write 包含页面所有业务操作。HTML、API、受保护静态资源均校验；未知模块默认拒绝。`permissions.go` 是页面/接口权限映射权威，账号模块清单在 `accounts.go`。新增页面/API 时同步映射，不能只隐藏导航。
- **跨页面与汇总**：景点相关轨迹的查看/编辑同时校验轨迹模块权限。工作台按各业务模块过滤指标，其他页面通过 `/dashboard?scope=tracks|track_submissions|feedbacks` 只读取自身统计。只读景点页不得触发供应商回填或编辑；共享轨迹弹窗支持只读。`shell.js` 统一身份标签、导航、只读控件和退出，不再由各业务脚本覆盖账号展示。
- **轨迹资料与景点候选**：景点管理页不再展示全局候选面板；共享 `track_editor.js` 通过 `place_candidates.js` 把单条轨迹的识别结果放在详情弹窗顶部，所有加载共享编辑器的页面都要先加载该模块。读取 GET `/tracks/:track_id/place-candidates` 需轨迹查看权限，既有 `/place-candidates/:track_id/confirm|dismiss` 写请求归属轨迹操作权限。`AdminUI.form.beforeFields` 挂载独立候选区（`place_candidates.js`）与轨迹地图区（`track_map_preview.js`），候选确认/忽略立即生效并刷新来源列表，不提交未保存的轨迹资料。源摘要失效返回 409，禁止使用过期候选。后台人工候选放宽为「围住景点 + 采样途经附近（matched_samples≥1）」，自动判定 unmatched 的记录同样可由人工确认关联；`POST /tracks/:track_id/place-refresh`（轨迹操作权限）同步触发单条重新识别，复用后台任务同一锁与配额，任务忙返回 409。确认关联成功后响应返回 `suggested_title`（按客户端标题生成规则以景点名为地名生成，环线判定用简化轨迹线），前端在标题输入框旁展示可点击代入的标签，代入随表单保存生效，不自动提交。详情还返回主景点 `primary_scenic_spot`（点击跳 `/admin/scenic_spots.html?spot_id=` 深链打开详情）与地图索引简化轨迹线 `geo`（GCJ02）；轨迹地图复用高德 Web Key，经 `/tracks/map-config` 与 `/tracks/amap-service` 代理（轨迹模块权限），不依赖景点模块权限。
- **轨迹资料缺项口径**：`GET /tracks?missing=any|place_candidate|scenic_spot|description|city|previews` 在分页和总数统计前筛选；缺简介/城市按空文本，缺预览为四图任一缺失，未关联景点为无 active 目录关联；任意缺项取各项并集。待确认 ID 由当前候选队列源摘要/规则/轨迹可索引校验取得，不触发供应商查询。MySQL 使用原生条件/关联，Mongo 使用 lookup + facet，内存由 main 注入景点仓储后使用关联快照；公共逻辑见 `repository/admin_track_missing.go`，新接口见 `docs/api/admin-tracks.md`。
- **账号设置入口**：`shell.js` 的共享账号区只常驻显示名称、身份标签与齿轮。齿轮复用内置 `tabler.Dropdown`，桌面侧栏向上展开、移动顶栏向下展开，支持点外部关闭和键盘操作；菜单读取当前 session 和 `/admin/api/me` 显示登录名、名称、身份、最近登录，并提供“账号安全”（保留 password.html URL，包含本人改密与邮箱验证）和退出入口。仅 GET 当前账号信息，不下发或展示密码/哈希。`ui.js` 不得给 dropdown-item 和下拉触发按钮自动套普通主按钮外观。退出请求失败保留当前页并在菜单提示，不把请求失败当成退出成功。
- **本人邮箱与密码找回**：`email_challenges.go` 的 8 位验证码使用随机盐与服务端 HMAC 摘要（密钥从 SMTP 专用密码独立派生，凭证轮换使旧码失效），在现有账号 JSON 中与错误次数/有效期/发信配额一起 CAS 持久化；不修改 AccountStorage 接口、不回退内存。10 分钟、一次有效、累计 5 次错误失效，每用途间隔 60 秒、每账号每小时共 10 封。原邮箱、新邮箱、密码找回三用途不能互换；错误尝试也必须提交，不能因返回业务错误回滚计数。改密、停用、绑定成功使全部旧会话/挑战失效。
- **邮箱归本人管理**：security API 只使用 session 的账号，不接受目标 username；绑定要当前密码和新邮箱验证，更换还要原邮箱验证。临时密码账号必须先改密，普通管理员编辑接口不得写入邮箱、验证码或现有密码。账号列表仅下发 recovery_email_bound/can_reset_password，完整邮箱只下发本人 GET security。已验证账号的管理员重置在服务端直接拒绝；全部启用账号绑定后持久化 legacy_password_reset_disabled，不因新建未绑定账号或 SMTP 暂不可用自动重开。未迁移账号暂保留标注“应急重置”的旧接口，不能提前让老账号失去恢复渠道。
- **公开找回与发信**：forgot-password.html 公开，必须先将申请放入有界队列再查账号，匹配/不匹配统一 202 提示；不能把 SMTP 耗时和错误同步回传来枚举邮箱。公开接口校验同源、限制 socket peer IP（不直接信任转发头），验证码重试有跨进程持久化上限。email_recovery.go 限 2 worker/16 队列，email_sender.go 总并发 2、连接与操作超时，只允许证书校验通过的 TLS 1.2+；不回退明文 SMTP/本地 sendmail，不记录 SMTP 响应、凭证、邮件正文、验证码或摘要。不在响应/邮件中发送登录密码，不自动签发登录会话。公开找回完成单独审计，日志不可用不改密码，actor=email-recovery 代表未登录流程，不能冒充账号本人发起。普通修改/邮箱验证复用受保护审计；后台安全通知为尽力投递，通知失败不能把已完成改密误报失败。
- **邮件配置与上线**：main 注入 ADMIN_SMTP_HOST/PORT/USERNAME/PASSWORD/FROM/TLS_MODE；未配置或非法时关闭邮件能力，登录/本人原密码改密继续工作，页面明确提示暂不可用。生产现有 OSS/STS 不是邮件服务；不要擅自开通云产品、复用 OSS AK/SK、发测试邮件或把真实 SMTP 凭证写入仓库。接入与迁移、公开响应/队列重启边界、HTTPS 要求见 `docs/admin-email-recovery.md`，示例配置 `deploy/admin-email.env.example`。
- **账号保护与日志**：整体 CAS 原子保护最后一个启用的管理员，停用/改密码使所有会话失效，调整权限由后续请求实时读取。后台写请求校验同源、记录发起和 HTTP 结果；日志不可用时拒绝发起写操作，不存正文、密码或哈希。后置结果写入失败时保留发起日志并记录 server log，不能声称业务与日志跨库事务原子。所有后台受保护数据响应使用 no-store；静态 HTML 禁止通过 `/admin/static/*` 访问。
- **日志账号筛选**：最近操作记录提供账号下拉选择（含停用账号）；`GET /admin/api/audit-logs?username=...` 按不可改的登录名精确匹配，MySQL/Mongo 必须先筛选再取最近 100 条。不传或空串表示全部账号。选项来自后台账号列表，刷新保留选择，快速切换忽略旧请求响应。
- **操作记录可读性**：账号页以“操作人、中文操作内容、对象、结果”展示日志，`static/audit_log.js` 统一维护方法/路径到业务行为的映射，新增被审计操作时同步更新；`accounts.html` 在 accounts.js 前加载该模块。原始方法、路径、状态和记录 ID 放在折叠详情，用 textContent 输出，保留账号筛选及同一 attempt/result 合并。状态 0 或 3xx 不能标成功，结果仅反映请求响应，批处理不声称逐项全成功；说明景点供应商回填可由页面自动分批触发，不能仅凭接口推定每条都由人工点击。旧记录没有正文/字段差异，不推测通过或驳回、启用或停用等具体改动，不为展示描述逐条查业务库；未知动作显示通用描述和原始详情。
- **会话仅内存**：`SessionStore` 使用 `sync.RWMutex` + `map`，进程重启即失效。**不要替换为持久化存储**；管理员重新登录是合理代价。
- **int64 ID 禁止经 JS Number 往返**：用户 ID 等业务 ID 是 18 位 int64，超出 JS Number 安全整数范围；JS 的 `String(number)` 还会把大整数四舍五入打印（`177920323643916288` → `"177920323643916300"`），经表单保存即写坏数据（2026-11-01 轨迹归属人因此被写错过）。约定：接口下发用字符串字段（用户列表 `id`、轨迹详情 `user_id_text`），前端只做文本展示，需提交的 ID 按字符串回传；轨迹归属人只读且不提交，服务端按 `json.Number`/字符串校验旧页面回传值必须等于原 ID；新增涉及 int64 ID 的后台接口与表单时沿用该约定，URL 路径参数天然是字符串无需处理。
- **静态资源经 `go:embed`**：`static/` 目录通过 `//go:embed static` 内置进二进制；新增前端文件后**必须放到** `internal/admin/static/`，否则二进制找不到。`/admin/static/*filepath` 路由只读 `staticFS`，不读磁盘。
- **安装包本机上传**：管理后台安装包上传走 `POST /admin/api/releases/upload-package`，服务端校验 APK/IPA 并读取包内版本名、版本号/Build、应用标识，同时计算大小和 MD5；前端使用返回的下载地址自动填写 `package_url`。发布页面固定 Android/APK，版本名和版本号只由包解析结果写入隐藏字段；服务端接口仍保留 iOS 兼容性。不要再把发布包上传链路改回浏览器 OSS 直传。
- **路径前缀固定 `/admin`**：所有路由都注册在 `h.Group("/admin")` 之下；前端 `fetch` 也写死 `/admin/api/...` 与 `/admin/static/...`。修改前缀需要同时改 `routes.go` 与 `static/*` 中的 URL。

## 常见误解

轨迹详情地图点选 POI 的写入接口 `/admin/api/tracks/:track_id/scenic-spot` 同时检查轨迹与景点操作权限，不能只凭轨迹查看权限新建景点或改绑。点击热点只展示资料，设置需显式按钮操作，沿用同源校验与发起审计。客户端提交详情里的旧关联 ID，服务端实际关联复用三仓储 CAS，拒绝覆盖已变更关联。协议见 [地图点选景点标签](api/admin-tracks.md#地图点选景点标签)。

- **误解**：管理后台没访问到，往业务 `auth` 组里塞路由。
  **事实**：admin 完全独立挂在 `/admin`，不属于 `/api/v1/auth` 组；登录态也是 cookie，不是 JWT。
- **误解**：可以用 `OSSTokenService.GetUploadCredential` 上传 APK。
  **事实**：那是按 userID 隔离的用户目录，policy 不允许写入 `release/`，必须用 `GetReleaseUploadCredential`。
- **误解**：进程重启后会话还在。
  **事实**：`SessionStore` 是纯内存实现，重启即清空，必须重新登录。
- **误解**：改环境变量可以覆盖后台密码或恢复已停用账号。
  **事实**：环境变量仅首次导入；此后以独立持久化账号为准。数据库不可用时拒绝鉴权，不重建临时管理员。
- **误解**：后台表单里 int64 的用户 ID 用 number 输入框往返是安全的。
  **事实**：JS Number 存不下 18 位 ID 且 `String()` 会截断打印，保存即写坏归属；必须用字符串字段下发；轨迹归属人只读，不参与保存（见硬性约束「int64 ID 禁止经 JS Number 往返」）。

## AI 使用权限

AI 模块新增 `ai_usage`（页面/数据查看）、`ai_tasks`（中断与重试）和 `ai_settings`（预算/并发/价格配置），后两者的操作还要求AI查看权限；重试另查源业务写权限。操作权限项没有独立页面，home跳过空页面。既有普通账号不自动获得新权限。沿用写请求同源、会话实时校验和发起/结果审计，日志中文动作见audit_log.js。完整协议见 [admin-ai](api/admin-ai.md)。
