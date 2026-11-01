# 工作交接：协议 H5 页面（2026-10-13）

## 2026-11-01：本人邮箱验证、邮件找回密码与管理员重置迁移（本地实现，未部署）

- 用户认可“邮件用于验证身份后自行设置新密码，不发送登录密码”，明确要求开发；过程中表示尚无邮件准备，要求检查现有 Server 支持哪家。只读核查生产 /root/.env 的配置项名称/是否存在，以及 Postfix 状态：已有阿里云 OSS/STS，没有 SMTP/邮件配置，Postfix/sendmail 未启动且没有 relay。未输出密钥、未开通云产品、未修改生产配置或发送真实邮件。推荐兼容阿里云邮件推送的加密 SMTP，实际启用仍需发信域名、发信地址、SMTP 专用凭证。
- `email_challenges.go` 在现有后台账号 JSON 中保存邮箱、验证时间、三用途挑战、发信冷却/窗口计数，沿用 MySQL/Mongo CAS，无新表或 AccountStorage 接口变更。8 位验证码随机生成、10 分钟/一次有效/最多 5 次错误；用途隔离、随机盐和服务端 HMAC-SHA256 摘要，密钥由 SMTP 专用密码独立派生，不写账号库；轮换凭证会使旧码失效。错误次数与成功消耗同样提交 CAS；每用途间隔 60 秒、每账号每小时共 10 封，改密不重置限额。
- `email_sender.go` 仅证书校验通过的 TLS 1.2+ SMTP（465 隐式 TLS，或强制 STARTTLS），总并发 2、连接和操作超时，不回退明文/本地 sendmail，不记录 SMTP 响应、凭证、邮件正文或代码。`email_recovery.go` 通过 Module.ConfigureEmailRecovery 注入，公开申请 2 worker/16 队列并在查账号之前统一返回 202，隐藏匹配与 SMTP 耗时差异；socket peer IP 限制有界保存 4096 项，不信任客户端转发头。公开完成另有 2 路并发上限，防止无限并发 bcrypt/CAS；队列不持久化，挑战/配额持久化，具体边界已文档化。
- 自己绑定邮箱需当前密码及新邮箱验证码，更换需先验证原邮箱，再验证新邮箱；接口只取 session 本人，不接受目标账号。临时密码用户先改密才可绑定。邮箱必须唯一，不能被管理员账户编辑覆盖；完整邮箱只给本人 GET security，账号列表只新增 recovery_email_bound/can_reset_password。绑定/换绑成功、本人改密、找回和停用清理旧挑战并使会话失效；发送安全通知为尽力投递，不因通知失败误报已完成的变更失败。
- 新增公开 GET `/admin/forgot-password.html`，GET `/admin/api/password-recovery/status`、POST request/complete；新增本人 GET `/admin/api/security`、POST `/security/email/current-code|send-code|verify`。`permissions.go` 允许已完成首次改密的所有登录账号管理本人邮箱，包括无业务页权限/只读操作员；现有 `/admin/password.html` 改为“账号安全”，齿轮入口同步。公开完成校验同源、密码确认、验证码，并在变更前单独持久化审计，失败不能绕过日志；actor=email-recovery 显示为“邮箱找回（未登录）”，不能把失败申请认作用户本人操作。没有邮件能力时明确 503/页面不可用，不伪造发送成功，原登录/本人改密仍可用。
- 管理员重置分阶段退出：已验证邮箱的账号立即在 UI/服务端禁止代重置；未绑定账号在迁移期保留“应急重置”和提示。全部启用账号完成绑定后永久写入 legacy_password_reset_disabled，之后新建未绑定账号或 SMTP 暂不可用都不重开旧入口。新增账号仍需临时密码首次改密后及时验证邮箱；原邮箱丢失或未绑定账号的身份恢复不提供绕过入口，迁移安排与回滚旧 JSON 写回风险见文档。
- main/config 注入 ADMIN_SMTP_HOST/PORT/USERNAME/PASSWORD/FROM/TLS_MODE；默认无配置关闭邮件，非法配置告警不含凭证。新增 `deploy/admin-email.env.example`（无真实凭证）、`docs/admin-email-recovery.md`；API 分册、README/路由索引与 root/admin AGENTS 同步行为和约束。此前固定操作记录列宽的未部署改动完整保留，所有更改均尚未提交、推送或部署；线上仍为 7312801，旧成功回执 `/private/tmp/track-admin-audit-layout-deploy-md4tr31q/receipt.md`。
- 最终 `go build ./...` 通过；account_security/forgot_password/accounts/audit_log/shell/password 六个 JS 的 `node --check`、`git diff --check` 通过。按根约定未新增/运行单元、集成、接口或浏览器自动化，未使用真实账号发送邮件；收件箱投递、验证码并发/错误/过期、权限隔离、迁移关闭和界面由用户手动验收。此处不能声称实际邮件链路已可用。
- AGENTS 校验仍为 12 个历史结构/超长文件问题、169 条 warning（比原 161 新增的 8 条是已在 routes.go 注册的 HTTP URL 被规则器当文件路径检查，并非缺失文件），freshness 根元信息问题仍在。报告 `/private/tmp/track-admin-email-recovery-agents.json` 与 `/private/tmp/track-admin-email-recovery-freshness.json`，不宣称全绿；全局 ~/.codex/AGENTS.md 原本为空，未擅改。更新前 handoff 已完整归档到 `docs/handoff/history_handoff/HANDOFF-20261101-185341.md`。

## 2026-11-01：固定最近操作记录列宽，避免详情展开挤动表格（本地实现，未部署）

- 用户反馈最近操作记录展开/收起详情时表格抖动明显，明确要求从一开始定义各列宽度权重。本轮从干净的 main/7312801 工作树开始，只修改操作记录表结构、样式及对应规则文档。
- accounts.html 为操作记录增加语义 colgroup，style.css 使用 table-layout:fixed，时间/操作人/操作内容/操作对象/结果比例为 18%/12%/34%/22%/14%。比例由列定义确定，加载/空态 colspan 和展开后的长文本不再参与自动列宽分配；详情展开仍允许正常增加行高，不隐藏正文。
- 桌面表格最小宽度 900px，不足时在已有 table-responsive 区域内横向滚动。长方法路径、账号/对象 ID、结果标签在单元格内换行，移除详情原本 160px 最小宽度和 480px 固定上限，改为服从单元格宽度。767px 以下保留共享移动卡片布局，隐藏 colgroup、取消最小表宽，并让操作内容/结果说明的元素始终放在卡片内容列。
- admin scoped AGENTS 增加“展开详情不重分配列宽”约束，固定上述比例、局部滚动和移动卡片边界；无 API、日志存储、权限、筛选或请求逻辑变化，不修改根 AGENTS 或业务协议。
- `go build ./...` 与 `git diff --check` 通过。无 JS 修改，不重复无关语法检查；按项目约定未新增或运行自动化测试、浏览器或接口自动化，未操作生产，实际展开/收起效果待用户手动验收。AGENTS/freshness 校验仍有原有治理问题，报告 `/private/tmp/track-admin-audit-columns-agents.json` 和 `/private/tmp/track-admin-audit-columns-freshness.json`，未声称检查全绿。
- 更新前完整 handoff 已归档到 `docs/handoff/history_handoff/HANDOFF-20261101-181448.md`。本轮尚未提交、推送或部署；上一轮 main/7312801 已成功部署，内外健康 200、3 个账号加载正常，资源版本 1c3dc1b3de2850b6845d，成功回执 `/private/tmp/track-admin-audit-layout-deploy-md4tr31q/receipt.md`。

## 2026-11-01：部署中文操作日志与后台布局修复

- 用户明确要求部署 Server。按 android-track-release 只处理 track_server 的 Git 同步/提交/推送、镜像构建推送和生产更新，不发布 App。main fetch 后与 origin/main 一致，无合并变化，复用同源码已经通过的 Go 构建、JS 语法与差异检查，不扩大自动化测试。
- 发布范围为中文操作日志说明及折叠原始请求详情、共享卡片默认内容边距、账号列表显式满宽表格、反馈列表标题/筛选布局及表格局部滚动，删除反馈页待处理时效卡片并只保留四种处理状态数量；包含相应 AGENTS 布局约束和文档。没有数据表、配置或权限变更，统计接口的时效字段保持兼容。
- 生产旧更新脚本存在宽匹配历史容器/镜像风险，本轮继续使用已经核实的精确容器替换流程：先验证已推送镜像的 manifest/config/source revision，再仅停止并移除 track_server_go，以确切 image ID 启动；保留 .env、持久化目录、历史容器和旧镜像。远端脚本兼容 Python 3.6，ping 正文按纯文本 hello track app 核对。后台内存会话因重启失效，持久化账号和权限保留。
- 回执目录 `/private/tmp/track-admin-audit-layout-deploy-md4tr31q`；本节为部署前记录，最终 Git 提交/推送、镜像、容器和内外网健康结果以该目录回执及本轮最终回复为准，不能仅据本节认定上线成功。此前 handoff 已完整归档至 `docs/handoff/history_handoff/HANDOFF-20261101-123244.md`。

## 2026-11-01：反馈页边距、筛选对齐与移除待处理时效卡片（本地实现，未部署）

- 用户先要求反馈与纠错页也修正边距，随后明确取消“待处理时效”统计，并指出反馈列表筛选元素没有对齐。本轮将两项要求合并完成，保留账号中文日志和卡片统一内边距的先前未提交改动。
- 反馈列表标题与“纠错选项”入口复用可换行的 admin-section-heading；筛选栏移除 Bootstrap row（其负 gutter 会突破卡片留白），显式使用 admin-toolbar feedback-toolbar。最终采用 Grid 三个字段列与独立按钮组：标签和 40px 控件分成固定行，按钮最小 40px 并与输入控件底部对齐；1050px 以下按钮整组换行，767px 以下字段单列。分页提示移到工具栏外，空提示不占位，避免长提示挤歪按钮。
- 反馈列表及纠错选项弹窗内表格包独立 table-responsive，宽内容只在相应表格区域内滚动；沿用共享桌面 18px/窄屏 14px 内容留白和已有弹窗 body 留白。详情处理表单没有 row 负边距，未扩大重构。
- 删除“待处理时效”标题及 24/72 小时两张卡片、feedbacks.js 的对应 DOM 更新与错误恢复引用，以及失去用途的 aging/warning/danger CSS；顶部仅保留待处理、处理中、已处理、已关闭四种状态数量。服务端统计及工作台时效字段保持兼容，没有删除共享协议或改变业务查询、状态处理逻辑。
- admin AGENTS 的布局约束补充普通工具栏不能直接借 row、标签和控件行高度、按钮分组换行、分页说明独立及宽表格局部滚动；API feedback 分册同步页面范围。无新增 API、表结构、依赖、配置或权限变化。
- 最终 `go build ./...`、feedbacks.js 的 `node --check` 与 `git diff --check` 通过；静态搜索确认反馈页不存在 statOver24/statOver72/aging 卡片残留。按项目约定未新增/运行自动化测试、浏览器或接口自动化、生产操作；布局效果待用户手动验收，不宣称浏览器视觉检查通过。
- AGENTS/freshness 仍报告既有仓库治理问题，报告 `/private/tmp/track-admin-feedback-spacing-agents.json`、`/private/tmp/track-admin-feedback-spacing-freshness.json`；更新前 handoff 已归档至 `docs/handoff/history_handoff/HANDOFF-20261101-122306.md`。
- 本轮和此前中文日志/卡片边距修改均尚未提交、推送或部署。线上仍为 main/096c0c4，发布结果见 `/private/tmp/track-admin-ui-deploy-zpbd0l4a/receipt.md`。下次部署需一起包含这些未发布改动，生产更新仍须精确容器名操作，避免旧脚本宽匹配历史容器。

## 2026-11-01：修复账号操作记录卡片贴边并固化布局规则（本地实现，未部署）

- 用户提供截图 `/Users/foxcoder/Desktop/WX20261101-121541@2x.png`，指出“最近操作记录”标题、说明、账号筛选/刷新控件贴近卡片边缘，并要求把反复出现的边距问题写进后台 Agent 规则。本轮保留上一轮中文日志说明全部未提交修改。
- 原因为 ui.js/enhanceCard 对无直接 H2 的卡片加 `.card.card-body` 到同一节点，但 style.css 的 `.panel.card { padding:0 }` 覆盖了该路径；原有内边距仅覆盖 `.panel.card > .card-body` 子节点结构。截图标题在 admin-section-heading 内，正好走同节点分支。之前实现时未检查到此共享样式覆盖。
- 统一让子节点和同节点 card-body 都有默认桌面 18px、窄屏 14px 内边距，无需修改增强脚本或为截图标题单独添加 margin；同类卡片一并恢复正常留白。账号列表是纯表格，显式使用 p-0 保留原本满宽布局和单元格内边距；景点纯表格已有相同显式标记。没有直接 H2 的用户统计等区域已有更具体的布局规则，按原 CSS 优先级保留。
- `internal/admin/AGENTS.md` 新增“卡片布局与边距约束”及提交前检查：标题/提示/工具栏不得贴边，两种增强结构都要覆盖，p-0 仅限刻意满宽区域，同时检查桌面/窄屏、空态/加载/报错/展开详情和长内容。规则描述根因与排查步骤，不仅重复“注意边距”。
- `go build ./...` 和 `git diff --check` 通过。默认未新增/运行测试、浏览器或接口自动化、生产操作；截图与 CSS 级联为本轮定位依据，未声称实际浏览器视觉验收完成，功能界面由用户手动验收。
- AGENTS 与 freshness 检查仍报告仓库既有元信息/超长文件等问题，报告 `/private/tmp/track-admin-card-padding-agents.json`、`/private/tmp/track-admin-card-padding-freshness.json`；更新前 handoff 归档至 `docs/handoff/history_handoff/HANDOFF-20261101-121756.md`。
- 本轮边距修复及上轮中文行为日志都尚未提交、推送或部署；生产仍为 main/096c0c4，完整上次成功部署回执 `/private/tmp/track-admin-ui-deploy-zpbd0l4a/receipt.md`。下次部署须包含两轮未发布改动。

## 2026-11-01：最近操作记录改为中文业务行为说明（本地实现，未部署）

- 用户反馈账号管理的最近操作记录难懂，要求描述行为。本轮将原来的 method/path 主展示改成时间、操作人、中文操作内容、操作对象和结果，保留账号下拉筛选、请求竞态保护以及发起/结果合并，不改变存储或接口。
- 新增 `static/audit_log.js`：按当前所有被审计接口的方法/路径映射业务行为，包含账号设置/改密、版本上传与发布、景点目录及来源/图片/关联处理、候选确认忽略、用户限制、轨迹编辑删除、投稿审核、路线组和反馈配置/处理等。只根据现有事实解释，旧记录无需迁移；未知动作回退“执行后台操作”并可查看原始请求。
- `accounts.html` 在 accounts.js 前加载该模块；accounts.js 使用 DocumentFragment 一次性渲染，用 textContent 输出用户数据。原始方法、路径、HTTP 状态、记录编号、行为和结果说明放在“说明与请求详情”中；错误/待确认另显示常见原因。0 为结果待确认，2xx 仅表示请求成功返回，3xx 不再错误标为成功，4xx/5xx 为请求失败，不伪装成具体服务端错误正文。
- 景点供应商资料回填注明景点页可自动分批触发，因此连续日志不一定是人工重复点击；不推定每条来源。批处理单条请求成功不等于所有项目完成。现有日志不含正文/字段差异，无法还原审核通过或驳回、具体启停、字段前后值、回复内容等，说明明确此边界，不猜测、不逐条读取业务库、不保存密码。
- 更新 admin scoped AGENTS，要求新增审计动作同步中文映射并保持结果语义/历史信息边界，账号 API 分册同步展示说明。无新增 API、数据库字段、依赖或配置。
- `go build ./...` 通过；audit_log.js/accounts.js 的 `node --check` 和 `git diff --check` 通过。按项目约定未新增或运行自动化测试、接口或浏览器自动化，也未操作生产，具体界面由用户手动验收。
- AGENTS/freshness 检查仍有原有超长文件、根元信息等问题，未声称全绿；报告 `/private/tmp/track-admin-audit-description-agents.json` 和 `/private/tmp/track-admin-audit-description-freshness.json`。更新前 handoff 已完整归档至 `docs/handoff/history_handoff/HANDOFF-20261101-121429.md`。
- 本轮尚未提交、推送或部署。当前线上为已部署 main/096c0c453b029206abb9f26099fde24d07d2ac3a；上一轮发布内外健康 200，3 个持久化账号正常、静态资源版本 d9873391763537a5da93，完整成功回执 `/private/tmp/track-admin-ui-deploy-zpbd0l4a/receipt.md`，不能将本轮中文日志展示视为已上线。

## 2026-11-01：部署后台交互、轨迹候选与页面性能优化

- 用户明确要求部署 Server。按 android-track-release 仅同步、提交、推送 track_server，再构建并推送 linux/amd64 镜像、精确更新生产容器；不修改或发布 App。main fetch 后与 origin/main 一致，无合并源码变化，复用本轮已通过的 Go 构建、相关 JS 语法检查与差异自查，不新增或运行自动化测试。
- 发布包括最近操作记录按账号筛选、左下角名称/身份/齿轮设置菜单、景点候选迁入轨迹详情、轨迹资料缺项筛选，以及静态资源版本缓存、异步图表、局部 DOM 观察器、有界图片缓存并发和后台性能开发约束。无新配置或数据表，保留账号与权限持久化数据。
- 生产旧 update_track_server.sh 存在宽匹配历史容器/镜像风险，沿用上次已经确认的安全路径：先 pull 并核对源码标签、manifest 和 image config，仅停止/删除确切的 track_server_go 后用精确镜像 ID 启动；保留 .env、持久化目录、旧镜像和历史容器。ping 正文为纯文本 hello track app，不按 JSON 解析。会话仍为内存，服务重启后需重新登录。
- 发布回执目录 `/private/tmp/track-admin-ui-deploy-zpbd0l4a`。本节为部署前记录，最终提交、推送、镜像、容器和内外网健康结果以该目录回执和本轮最终回复为准，不能仅据本节认定部署成功。部署前 handoff 已归档到 `docs/handoff/history_handoff/HANDOFF-20261101-114047.md`。

## 2026-11-01：后台页面加载优化与性能规则沉淀（本地实现，未部署）

- 用户反馈部分后台页面点击打开偏慢，并要求把问题抽象记录到后台 AGENTS，避免后续重犯。本轮继续保留账号日志筛选、齿轮设置菜单、轨迹景点候选与资料缺项筛选等先前全部未提交改动；没有新增部署或提交授权。
- `assets.go` 对嵌入静态目录生成统一内容版本、缓存无账号信息的页面模板、给 HTML 静态引用注入版本与 `runtime.js`；动态图标/地图 JSON 使用 `AdminAssets.url()`。只有匹配当前内容版本的公开静态资源使用一年 immutable 缓存，无版本/旧版本继续 ETag 重验证；资源变化自动换 URL。公开登录页 no-cache，受保护页面/API 继续 no-store 和实时账号校验，未缓存权限。私有 HTML 支持 gzip，但不存入公共 gzip 缓存。
- 用户、地图模式、同行、景点页移除阻塞式 ECharts 标签，列表脚本无需等待图表库开始工作；共享 `AdminAssets.loadCharts()` 按需异步加载并在失败后允许重试，工作台也复用此入口。用户分布文字、景点核心数字先展示。工作台/统计与地图渲染保留或补充请求序号，避免加载库期间旧响应覆盖新选择。景点供应商回填推迟到首轮列表和统计完成后再启动。
- `shell.js` 权限观察器由每次 DOM 变化扫描整个 document，改为同批去重的局部子树处理；账号权限刷新仍全量应用，并观察 disabled/权限标记变化以恢复只读锁。`ui.js` 的新增节点和表格增强也在同批去重，减少列表与图表更新重复遍历。
- 列表头像/轨迹预览缓存未命中的逐条等待改为 `list_assets.go` 中最多 4 路并发，单页相同作者不再重复处理头像，处理后的列表顺序保持原序。继续等待缓存成功才使用本地 URL，保留原有 5 秒单次超时和请求 context；没有引入未落盘图片 URL 或无限制 goroutine。尚未重构工作台全量统计聚合、账号限制逐行查询等其他潜在热点，不宣称已覆盖所有慢接口。
- `internal/admin/AGENTS.md` 新增“页面性能约束”，提炼首屏依赖、内容版本缓存、私有数据隔离、局部观察器、批量去重/有界 I/O、异步竞态和验收边界；索引定位到 assets.go/runtime.js/list_assets.go，提交前清单要求检查是否重新引入相同问题。按 agents-md-generator 技能做针对性现有规则更新，未重建治理体系或触发无关目录整改。
- `go build ./...` 通过；runtime/admin_usage_charts/feature_statistics/scenic_spot_charts/scenic_spots/dashboard/users/shell/ui 共 9 个 JS 的 `node --check` 和 `git diff --check` 通过。遵循项目约定未新增/运行测试、接口自动化、测试容器或生产业务操作；未做浏览器实测或前后耗时对比，不声称具体提速幅度，功能和界面由用户手动验证。
- 已执行 AGENTS 校验与 freshness：仍有 12 个既有问题（根文件超 16KB、缺 precedence、若干旧源码超长）和 161 条 warning；根规则缺新鲜度元信息，全局 `/Users/foxcoder/.codex/AGENTS.md` 为空，即全局入口规则未落盘。报告 `/private/tmp/track-admin-performance-agents-check.json` 与 `/private/tmp/track-admin-performance-freshness.json`，不能宣称规则检查全绿。全局修复命令为 `python3 /Users/foxcoder/.codex/skills/agents-md-generator/scripts/manage_docs.py sync-global-codex-agents . --write`，未擅自修改工作区外全局规则。
- 当前仍未提交、推送或部署；线上仍为 afc8fdd。旧生产更新脚本宽匹配风险和安全更新路径见后续部署交接。本轮更新前完整 handoff 已归档到 `docs/handoff/history_handoff/HANDOFF-20261101-113918.md`。

## 2026-11-01：景点候选迁入轨迹详情与轨迹资料缺项筛选（本地实现，未部署）

- 用户明确同意按所建议范围开发：候选处理放入各自轨迹详情弹窗，轨迹列表增加任意缺项、待确认景点、未关联景点、缺简介、缺城市、缺预览图；保留之前账号日志下拉筛选和齿轮菜单的全部未提交改动。
- 景点页删除全局候选面板。`place_candidates.js` 改为共享单轨迹候选组件，`track_editor.js` 用 `AdminUI.form.beforeFields` 将其挂在弹窗顶部；tracks/scenic 页面均先加载该组件。只读可看，轨迹 write 才能确认/忽略，操作立即生效、刷新来源列表，不提交未保存的轨迹表单。处理后候选/缺项过滤可使对应行退出当前结果。
- 新增 GET `/admin/api/tracks/:track_id/place-candidates`，`TrackPlaceService.GetPlaceCandidate` 复用源摘要、规则、状态和可索引轨迹校验；404 无有效候选、409 源变化、503 未启用。读取仅查持久化缓存，不消耗供应商额度。保留全局 GET 队列作为兼容接口；现有候选确认/忽略 POST 改按轨迹模块授权并沿用写操作审计。
- `/admin/api/tracks` 新增 missing=any/place_candidate/scenic_spot/description/city/previews；`AdminTrackListFilter` 扩展 Missing 与当前有效候选 IDs。候选 IDs 经既有队列分页、源/规则和资料有效性校验取得，无人为截断。`repository/admin_track_missing.go` 统一缺项口径，MySQL 原生条件/关联、Mongo lookup+facet、内存关联快照均在总数和游标前过滤，支持与原条件组合。Mongo 受影响轨迹列表游标/排序统一使用真实 `_id` 字段。
- 未关联景点指无 draft/published 自有目录关联，包含正常的非景区轨迹；缺预览为四种日夜图任一缺失。页面增加必要口径提示，快速切换筛选忽略旧响应。main 为内存轨迹仓储注入内存景点仓储，读关联快照时不同时持有两仓储锁，数据库降级不新增外部依赖。没有新增数据表、迁移、配置或依赖。
- 文档 `docs/api/admin-tracks.md`、API 入口/索引、景点识别方案、账号权限说明以及 root/admin AGENTS 已同步。最终 `go build ./...`、place_candidates/tracks/track_editor/ui/shell/accounts 六个 JS 的 `node --check`、`git diff --check` 通过；按项目约定未新增或运行自动化测试，未启动测试数据库或操作生产，功能与界面由用户手动验收。
- 本轮所有改动尚未提交、推送或部署，生产仍为 afc8fdd；后续部署要包含先前两项待发布后台改动。旧生产更新脚本存在宽匹配历史容器风险，应按确切 track_server_go 更新并保留历史容器/镜像，详见后文部署交接与回执。

## 2026-11-01：精简账号区与齿轮设置菜单（本地实现，未部署）

- 用户要求直接设计并开发，不需要展示预览。共享账号区只常驻显示名称、身份标签与齿轮，移除常驻改密链接和退出按钮；此轮没有创建或交付预览文件。
- `static/shell.js` 使用现有内置 `tabler.Dropdown`：桌面侧栏向上展开，移动顶栏向下展开，支持点外部关闭、键盘导航和 Escape。菜单展示登录账号、名称、身份、最近登录时间，打开时通过既有 `/admin/api/me` 刷新信息；改密仍使用现有页面，退出仍调用现有接口。长名称在常驻区省略，菜单内完整换行；限高支持小屏滚动。
- 退出按钮请求期间禁用防止重复提交；失败留在当前页并显示重试提示，不因网络/服务错误误示已退出。所有账号信息使用 textContent，下发和展示的数据不含密码、哈希或内部版本。
- `static/ui.js` 跳过 dropdown-item 和下拉触发按钮的普通主按钮增强，`static/style.css` 增加紧凑账号区、齿轮与菜单样式。未增加 API、数据表、依赖或账号权限；相关 admin AGENTS 与账号 API 文档同步。
- `go build ./...`、shell.js/ui.js/accounts.js 的 `node --check`、`git diff --check` 通过。遵循项目约定，未新增或运行自动化测试、未操作生产；界面交互待用户手动验收。前一轮操作记录账号下拉筛选改动完整保留，两项变更均尚未提交、推送或部署。
- 生产当前仍是上轮已部署的 afc8fdd。部署历史及宽匹配更新脚本风险见下方交接；后续部署应精确替换 track_server_go，并保留历史容器和旧镜像。

## 2026-11-01：最近操作记录按账号下拉筛选（本地实现，未部署）

- 用户要求管理员按账号名称通过下拉菜单筛选最近操作记录。账号列表提供“全部账号”和各账号，显示名称不同于登录名时同时展示，停用账号也可选择；切换立即查询，刷新保留选择，无记录显示空状态，快速切换忽略旧请求响应。
- `GET /admin/api/audit-logs` 新增可选 `username` 查询参数，按固定登录名精确匹配；空值或不传保留原有全部账号行为。`AccountStorage.ListAudit` 的 MySQL/Mongo 实现先筛选再倒序 LIMIT 100，避免只筛全局最近 100 条造成记录缺失。接口仍固定仅管理员可读；不涉及表结构或配置变更。
- 修改 `account_storage.go`、`account_handlers.go`、`static/accounts.html`、`static/accounts.js`，既有路由测试 fixture 只适配接口参数；API 分册、索引与 admin scoped AGENTS 同步。`go build ./...`、`node --check internal/admin/static/accounts.js`、`git diff --check` 通过。按根 AGENTS 未新增或运行自动化测试，未启动数据库或操作生产，功能待用户手动验收。
- 本次筛选变更尚未提交、推送或部署。上轮账号系统已部署源码为 main/afc8fdd，3 个账号、MySQL 与内外网健康正常，回执 `/private/tmp/track-admin-accounts-deploy-QGhpG9/receipt.md`。生产旧更新脚本的宽匹配会命中 14 个历史容器，上轮未执行该脚本，采用精确容器名完成替换并保留历史容器与旧镜像；后续部署不要直接运行旧脚本误删历史容器。

## 2026-11-01：部署后台账号与页面权限

- 用户在本轮明确要求部署，授权按 android-track-release 完成 track_server 的同步、提交、推送、镜像构建和生产更新；不发布 App。
- main 已 fetch 并与 origin/main 对齐，发布源码沿用本轮通过的 go build、20 个 JS 语法检查和差异自查；不新增或运行自动化测试。
- 部署必须保留生产 .env 和全部既有后台初始化账号，核对 OPS_INTERNAL_TOKEN 与 admin 初始化账号存在但不输出凭证。首次启动自动导入为 admin 管理员及其余只读操作员；账号库不可用不能以内存鉴权兜底。
- 最终提交、镜像摘要、容器、内外网健康与账号初始化结果以本次部署回执和会话最终回复为准；本节为部署前记录，不能据此声称已上线。

## 2026-11-01：后台账号持久化、管理员与操作员页面权限（本地实现，未部署）

- 用户已确认：管理员全权限；操作员按页面 none/read/write 三档，不细分动作；跨页面检查实际所属模块。现有 `admin` 迁移为管理员，其他现有账号迁移为操作员，默认所有当前业务页面只读，账号管理固定禁止。账号/权限须持久化并可在后台新增和调整，左下角显示身份标签。
- `internal/admin/accounts.go` 提供独立账号状态、首次导入、身份/权限、编辑 revision、security_version 和最后启用管理员保护；`account_storage.go` 提供 MySQL/Mongo 单行/单文档 version CAS 和操作日志，main 显式注入。新增 `admin_account_state`、`admin_audit_logs`，MySQL 自动建表与 mysql.sql 同步。账号库故障不使用环境变量鉴权、不降级内存；会话仍只存在内存。
- 仅首次导入 ADMIN_ACCOUNTS 与兼容的 ADMIN_USERNAME/ADMIN_PASSWORD_HASH，必须包含 admin；保留现有密码。已初始化后以数据库为唯一权威，重启/环境变量不会覆盖后台修改。新增账号和重置密码下次登录先改密，新操作员默认未授权；新增页面不会自动授权。
- 新增 `/admin/accounts.html`、`password.html`、`access.html` 与 `/admin/api/accounts` GET/POST、`/accounts/:username` PUT、`/accounts/:username/reset-password` POST、`/password` POST、`/audit-logs` GET。管理员可创建/调整身份/权限/状态/重置密码；账号停用保留记录，不物理删除。停用/改密使各进程的旧会话失效，权限由后续请求实时读取；已在执行的请求不承诺撤回。
- `permissions.go` 在服务端校验 HTML/API/受保护资源及跨页面权限。静态资源目录不提供 HTML，不能绕过页面路由；景点关联轨迹需轨迹查看，轨迹编辑/删除需轨迹操作；头像、截图按使用模块放行。工作台按模块裁剪返回，其他页面复用统计加 scope 仅下发自身数据。读请求可以产生正常缓存/计数，但只读景点页不运行供应商回填。
- `shell.js` 统一身份标签、导航过滤、只读控件、退出和改密入口；移除各页面重复的身份/退出逻辑。共享轨迹弹窗支持只读查看，景点只读禁止照片拖动/上传/删除、AI 编辑与自动回填。写操作先记录发起，再保存 HTTP 结果；日志不可用时拒绝发起。后置日志失败保留发起记录并记 server log，不声称跨存储事务。日志不记录正文、密码或哈希。
- 最终 `go build ./...`、20 个变更/新增 JS 的 `node --check`、`git diff --check` 通过。按根 AGENTS 未新增测试用例、未运行单元/集成/接口/浏览器自动化，未启动数据库或操作生产；既有路由测试仅适配新增存储注入与会话账号版本。功能、数据库迁移和界面待用户手动验收。
- API 分册 `docs/api/admin-accounts.md`、索引、反馈统计说明和 root/admin AGENTS 已同步。规则检查报告 `/private/tmp/track-admin-accounts-agents-check.json`：已有 root 16KB 超长/precedence 声明、大型源码超长等仍未通过；全局入口规则未落盘（~/.codex/AGENTS.md 为 0 字节）。修复全局基线命令：`python3 /Users/foxcoder/.codex/skills/agents-md-generator/scripts/manage_docs.py sync-global-codex-agents . --write`。本次不重构无关历史治理文件、不改全局配置。
- 当前没有提交、推送或部署；生产尚未导入账号。部署时保留全部现有初始化账号配置，首次启动会自动持久化；此后新增/改密/权限调整都从后台处理。

## 2026-10-27：联合发布纠错配置

- 用户明确要求联合发布，服务端本轮范围为动态纠错选项、提交快照、后台配置与纠错处理；Android 同步发布当前工作树的详情/同行/地图 UI、坐标声明及纠错入口。两仓 main fetch 后均与 origin/main 一致，无合并变更；复用同源码已通过的 go build ./...、Android assembleDebug 和语法/差异检查，不新增或运行自动化业务测试。
- 服务端先提交推送发布快照，再构建推送 linux/amd64 镜像并更新生产；只有生产镜像、MySQL 与内外健康检查通过后才发布 Android。App 预计由 1.0.8（9）递增为 1.0.9（10），具体结果以发布回执为准。
- 发布记录目录 `/private/tmp/track-correction-joint-release-IBvpkD`；本节为发布前记录，生产状态、最终版本与提交哈希以该目录回执和本轮会话最终回复为准。

## 2026-10-27：可配置的轨迹纠错

- 用户要求纠错选项后台配置、API 下发，客户端不写死。业务新增 JWT `GET /api/v1/feedback/correction-options`，管理后台新增 session 鉴权 GET/PUT `/admin/api/feedback-correction-config`；后台「反馈与纠错 → 纠错选项」可新增、编辑中英文名称、排序和启停，保存使用 version 条件更新，409 保留未保存表单。
- `service/feedback_correction.go` 管理服务端默认六项、选项校验、稳定 ID 及提交快照。`POST /feedback` 接收 track_id/correction_option_id 和可空说明，校验选项启用及轨迹可访问；名称与真实标题由服务端读取，不信任客户端。停用项返回409；历史记录保存 correction 快照并保留兼容的可读 content，普通反馈行为不变。
- `repository/feedback_correction.go` 实现 MySQL/Mongo/内存配置读写；MySQL 启动自动补建 feedback_correction_config 和 user_feedbacks.correction_json，mysql.sql 同步。全停用返回空列表，不回退默认；内存降级保存不跨重启。反馈后台优先使用结构化快照，旧六项正文仅作兼容；支持关联轨迹编辑入口与快捷回复，保存反馈不会自动修改轨迹。
- `go build ./...` 通过；feedback_correction_config.js、feedback_correction.js、feedbacks.js、tracks.js 的 `node --check` 与 `git diff --check` 通过。Android assembleDebug 通过并安装启动三星；按约定未新增/运行自动化测试、数据库测试或业务操作。协议、路由索引与相关 AGENTS 已同步。
- 本轮尚未部署、提交或推送，线上生效须更新 Server；功能与界面待用户验收。AGENTS/新鲜度检查仍报告原有 root/源码超长、precedence/时间元信息及全局规则缺失，报告 `/private/tmp/track-correction-server-{agents,freshness}.log`，不宣称规则检查通过。

## 2026-10-26：部署放宽主景区与环线归属规则

- 用户明确要求部署Server，按android-track-release仅同步/提交/推送服务端，然后构建推送生产镜像；不提交或发布Android。main已fetch，与origin/main无分叉，复用同一源码通过的go build ./...及本轮差异自查，不新增或运行自动化测试。
- 范围为主归属2/5非连续采样、完整GPS环绕POI判断、证据复用与限额租约，以及原生KMZ坐标声明异步修复。保留生产配置、每日用量、历史已完成结果和原始GPS；历史迁移按后台既有预算续跑，不临时扩大额度或清空记录。
- 先核对已推送提交，再构建linux/amd64镜像、更新确切生产容器；成功以镜像配置ID/摘要、MySQL无内存降级以及内外网ping为准。本节为部署前记录，最终状态见本次部署回执与会话回复。

## 2026-10-26：放宽主景区与环线围合归属（尚未部署）

- 用户批准降低主归属门槛，补充要求环线围住景点也可归属。规则 v2 改为5个等距样本中2个 inside 即可、不要求连续，最高票并列不强选，源未变的旧 matched 保留。无采样主归属且无冲突时，普通高德 polygon 矩形召回最多4页×25个风景名胜候选，完整GPS折线而非bbox/简化折线做围合判断，父子候选合并后唯一才设 enclosed_poi 主归属。非闭合、圈外、边线3米内、大跨度/面积及候选截断均保守处理。国外仍只附近关联。
- 证据版本保持1，判定规则升级为2；旧源摘要/点查询key不变，完整 v1 Relations 可重判，无需重新请求5个点。evidence_complete 保留环内搜索遇额度/临时失败前的证据，新请求复用现有预算、租约、重试和清理；修复单轮调用上限提前占租约的问题。列表/详情迁移期接受源匹配的 v1 正例；地图聚合方法改为 track_relation，详情字段仍 primary_scenic_spot。
- 排查发现原生 KMZ 已归一化WGS84但声明会话GCJ02。Android 创建/重试/首次上传统一声明WGS84，KML增加显式标记。本次服务端索引从解析成功的同一份KML读标记；旧原生 source_tag 空且GCJ02声明、具有本项目导出特征才修复。三仓储CAS只改坐标声明和更新时间，不改原GPS。v1迁移一次检查原生GCJ02/WGS84并重建索引，早期空声明和无标记的人工导入不推断。旧已确定结果仅源不变时保留，坐标修复导致源变化时重新识别。
- 相关入口：track_place_decision.go / ring.go / search.go、track_raw_coordinates.go、repository/track_coordinate.go。无新HTTP路由/表/环境变量，协议、AGENTS及Android app规则同步。上线后既有后台游标自动迁移；受日预算限制，不清零用量，不必清空历史记录。路线组中心/范围由现有聚合任务刷新。
- 本轮 go build ./...、Android :app:assembleDebug 通过；最终Debug安装小米3e7cd913并启动成功。未新增/运行自动化测试、未调用供应商做新规则实测、未提交推送或部署。本轮前的真实诊断共15次，日计数600，报告见 /private/tmp/track-place-review-EeNnxhty/未命中复查与规则评估.md；旧证据模拟2/5规则可新增9条并保留2组并列，尚非新规则上线处理结果。
- 两仓diff检查通过。AGENTS verify/freshness实际执行，仍报既有超长文件、优先级/元信息/路径提示及freshness基线问题，未扩展治理整改；日志 /private/tmp/track-scenic-{server,android}-{agents,freshness}.log，不能声称静态规则全部通过。

## 2026-10-26：部署轨迹主景区响应字段

- 用户要求部署 Server，按 android-track-release 技能同步、提交、推送本仓库后构建推送生产镜像；只部署服务端，保留客户端尚未提交的标签代码，不发布 App。
- 部署范围为轨迹详情及各类列表的 `primary_scenic_spot={id,name}` 只读字段、批量索引查询、服务注入与协议/维护说明；无新配置、表结构或供应商外呼，不触发历史重识别。
- 复用同一业务源码已通过的 `go build ./...` 与差异检查，不运行自动化测试；生产更新后核对精确镜像 ID、MySQL 仓储与内外网 ping。成功状态以本轮最终部署回执为准。

## 2026-10-26：轨迹接口下发主景区标签

- 用户要求主景区随列表/轨迹信息下发，并在 Android 详情顶部标签末尾直接显示名字。新增只读可选 `primary_scenic_spot={id,name}`，覆盖 Track、通用摘要、本人上传与收藏摘要；已有推荐、搜索、客态主页和地图关联轨迹列表复用统一装饰。
- `track_scenic_summary.go` 按页批量读取完成的 matched 记录、当前空间索引和共享地点，校验源摘要/规则，三种仓储补齐 `FindPlaceGeoIndexes`。不调用供应商、不从 nearby 推测；缺失/可选读取失败省略字段，不阻塞原有内容；没有新 HTTP 路由或数据库结构变更。
- 主景区经 TrackService 注入，详情/创建/更新返回与列表字段一致。Android 使用 TrackInfo 和既有详情刷新，不写 Room 或上传请求；当前这轮只实现并编译，尚未提交推送或部署，新字段需服务端上线后客户端才有真实数据。`go build ./...`、Android `:app:assembleDebug` 与两仓差异检查通过；Debug 已安装并启动于小米 `3e7cd913`，未新增或运行自动化测试，界面待用户验证。
- 规则静态检查仍报告既有超长文件、根优先级/元信息及 freshness 基线问题；未扩大治理修复。报告位于 `/private/tmp/track-scenic-{android,server}-{agents,freshness}.log`，这些检查不算通过。

## 2026-10-26：景区识别一次完成与集中补齐

- 用户要求集中处理存量，并明确轨迹识别一次即可。高德 matched/unmatched 结果及共享景区资料长期复用，旧版已成功记录忽略旧到期时间，空候选也不按24小时重复识别；保留源摘要/规则变化保护与失败退避。请求缓存和 Google 内容仍保留 TTL，三仓储清理排除高德共享地点，并修正 Mongo 零时间长期记录的清理条件。
- 后台日志增加 reused/skipped，区分已完成复用与过短跳过；本轮计划同步提交推送和部署规则，再通过临时调度/预算集中补齐，完成后恢复日常配置，不清除已完成结果或请求缓存，不重置每日实际调用计数。
- 业务上保留多景区候选及 sampled_inside/nearby 区分，地图聚合仍仅一个主景区；本轮不增加客户端多景区展示。实际编译、部署及存量数量以本轮最终回执为准，不新增或运行自动化测试。

## 2026-10-26：部署自动景区识别

- 用户要求部署 Server，并确认服务器已登录；按发布技能只同步、提交、推送服务端，再构建推送 linux/amd64 镜像和更新生产，不发布 App。
- 本次包含普通 AOI 自动识别、限额缓存后台任务、独立持久化表及空间索引坐标修正。MySQL 启动幂等创建新增表，保留原轨迹与现有环境配置；Google 查询仍受独立 Key 和显式缓存配置控制。
- 复用本轮同一业务源码已通过的 `go build ./...`；不新增或运行自动化测试、不主动调用真实供应商验收接口。生产是否成功以部署回执、MySQL 仓储和内外网健康检查结果为准。

## 2026-10-26：普通 API 自动景区识别（尚未部署）

- 新增 `track_place` 异步扫描任务、MySQL/Mongo/内存持久化、供应商请求缓存及跨进程每日预算。国内使用高德普通逆地理编码AOI，5点至少3点命中且连续2点命中才建立主要景区；Google New适配器只产出附近地点，缺乏边界不强判归属。地图三档及原始轨迹业务结构保持兼容。
- 路线组持久化自动 `place-amap-*` 区域ID，地图气泡及管理端显示共享名称；重建后按稳定代表轨迹恢复关联。存量空间索引分页补齐，旧手工西湖矩形不参与新识别。源摘要、租约和代表轨迹条件更新保护并发，超额次日继续。
- 统一新空间索引坐标，源WGS84/GCJ02/BD09在入索引时转换；大陆服务范围用省级多边形掩码而非宽泛矩形，旧非GCJ02索引逐批从完整GPS修复。
- 配置及人工验收见 `docs/track-place-discovery.md`；Google需要服务端Key及适用的内容暂存配置，默认不持久化Google地点内容。`go build ./...` 与差异检查通过；未新增或运行自动化测试，也未调用真实供应商接口消耗额度。本次仅实现，未提交、推送或部署。

## 2026-10-23：DevMode 历史路线说明补齐

- 为 Android 统一历史数据补齐页新增路线说明待补分页、GPS 数据和完成接口；维持业务 JWT、全站 DevMode 既有调用边界，并对完成请求应用上传类账号限制。
- description source version 覆盖生成规则依赖的原始 GPS 地址、坐标系、运动类型、轨迹标题、位置、时间、距离及时长。MySQL、Mongo、内存仓储均只对未删除、已结束、说明为空且源字段仍匹配的记录条件写入，不覆盖已有说明；分页和缺原始轨迹计数在仓储层完成。API 文档与 route index 已同步。
- `go build ./...` 通过；未运行自动化测试。随后按发布流程同步提交、推送并部署，本节记录此次部署代码范围；最终生产结果以本轮部署回执和最终回复为准。

## 2026-10-22：联合发布精品标签

- 用户要求联合发布，授权两端同步、提交、推送及 Server 部署/App 发布。两仓 fetch origin 后 main 与 origin/main 一致；Server 仅包含本人已上传/收藏列表 is_featured 的 DTO、批量状态装饰和对应 API/规则/交接，无数据表或配置变更，复用同源码已通过的 go build ./...，差异检查通过，不新增或运行业务自动化测试。
- 先提交推送 Server，再构建推送 linux/amd64 latest 镜像，生产健康通过后发布 Android。用户已表示服务端本次已登录；仅验证并使用本轮新 SSH master，不复用历史认证。部署及 App 最终回执保存在 /private/tmp/track-featured-joint-release-95z8q7xc；本节为部署前记录，实际成功以回执和最终回复为准。

## 2026-10-22：本人已上传与收藏列表精品标识

- 用户要求 Android Me 和搜索 Item 的精品标签。搜索/客态主页复用 TrackSummary 原已返回 is_featured；本轮为 MyTrackSummary 和 CollectedTrackSummary 补齐同名 bool，无 omitempty，false 也明确返回，仅当前投稿 approved 时为 true。
- DecorateMySummaries 在既有 ListByTrackIDs 结果中填入标识，无新增查询；ListCollectedTracks 对当前页复用 TrackSubmissionService.ApprovedTrackIDs 批量取状态并拷贝到收藏 DTO，无逐条查询详情或投稿媒体缓存。不修改仓储接口、三种数据库实现、表/索引、标题或封面规则。API 通用模型、投稿/本人/收藏文档、路由索引和 AGENTS 已同步。
- go build ./... 与两仓 git diff --check 通过；Android assembleDebug（7s）通过，新 Debug 安装小米并启动，前台 MainActivity。未新增或运行自动化测试，用户手动验证分页、审核通过/未通过的标签及日夜配色；未提交推送或部署，本轮收藏字段上线需另行部署，当前生产仍是 0b9014d。
- AGENTS/freshness 检查保留既有文件超长、Server 根 precedence、全局入口为空及时间元信息告警，报告 /private/tmp/track-featured-{android,server}-{agents,freshness}.log；未扩展无关治理或修改全局规则。

## 2026-10-22：准备部署地图全部类型查询

- 用户明确要求“部署 server”，按 android-track-release 技能同时授权本仓库同步、提交、推送；仅处理 Server，不提交客户端或发布 App。已 fetch origin，main 与 origin/main 一致，无新远端代码；本次范围为地图查询 all、MySQL 区域聚合修正及 API/规则/交接文档（含此前 WebP 兼容说明），无数据表变更或新配置。
- 复用本轮同一源码已通过的 go build ./...，不重复运行业务自动化测试。先提交推送再构建 linux/amd64 latest 镜像，镜像推送成功后新建可见 SSH 密码终端，生产执行更新脚本、校对镜像摘要和配置 ID、精确清理已停止同名容器并启动，检查 MySQL/无降级/重启数及内外 ping。
- 部署脚本、构建元信息、健康结果与最终回执位于 /private/tmp/track-server-map-all-deploy-mjuqgc7_；本节为构建前准备记录，部署是否成功以最终回执和用户回复为准。SSH 密码由用户在当次终端自行输入，不沿用历史凭证或控制连接。

## 2026-10-22：地图查询支持全部运动类型

- 用户要求地图模式新增“全部类型”并默认选中。新增 /track-map/view、/track-map/groups 的显式 track_type=all 语义，在 TrackMapService.buildFilter 转为 TrackMapQueryFilter.TrackType 空串；参数匹配忽略大小写及首尾空白，未传/空值继续默认 hiking，不改变其他业务的 normalizeTrackTypeCode 或运动类型定义。
- 三种仓储原先都支持空类型不过滤。MySQL CountRouteGroupsByArea 原来另按 track_type 分组，现只按 area_id/网格汇总，返回类型取查询 filter.TrackType，防止全部查询生成相同 cluster_id 的多个气泡；单类型结果保持原语义，与 Mongo/内存的聚合方式一致。不改表、索引、路由、DTO 或离线 RouteGroup 严格按运动类型隔离的规则，具体路线组仍返回真实类型。
- Android 已为筛选列表前置本地“全部”，默认 raw=null 并在请求中显式传 all；元数据刷新不会把全部变回徒步。同步 docs/api/track-map.md、route-index.md、track_map.md 与两端 AGENTS 的查询约定。
- go build ./...、Android assembleDebug（2s）与两仓 git diff --check 通过，Android Debug 已安装小米并启动，前台 MainActivity。Go 初次被沙箱阻止访问系统构建缓存，经工具审批沙箱外构建通过。未新增或运行自动化业务测试、未操作生产数据、未提交推送/部署；需要部署本轮 Server 后再手动验证“全部”及单类型切换、城市/区域气泡和旧空参徒步语义。
- AGENTS/freshness 静态检查保留既有超长/precedence/全局空入口及时间元信息告警，报告 /private/tmp/track-map-all-{android,server}-{agents,freshness}.log；本轮未扩展无关治理。

## 2026-10-22：正常上传支持 WebP 的客户端调整

- 用户最终要求正常上传预览的原始长宽各缩至 1/4，并采用 WebP85。Android 已统一详情/批量入口，直接由 Bitmap 编码 WebP，上传后缀/MIME、缓存身份与重试清单对应格式/质量；旧 JPEG 继续可读，历史补图仍沿用固定尺寸 JPEG 校验，Me 本地缓存保持既有尺寸/格式。
- 核对 cmd/server/main.go 的 previewCache 已允许 .webp，AssetCacheService 按源 URL 保留扩展名，TrackPreview 的 c.File 直接读取缓存文件，无写死 JPEG 响应；普通创建/更新只接收四图 URL，不套用历史补图 JPEG/1024px 校验。本轮 Server 仅更新 AGENTS 与 docs/api/track.md，不修改接口、仓储或 Go 业务代码，不需要重新部署。
- 现有四图样本 363,170 字节，经桌面工具四分之一缩放及 WebP85 编码后合计 22,698 字节（约少93.8%），仅作预估，Android 实际编码与清晰度由用户验证；样本结果在 /private/tmp/track-preview-compression-4enm25id/quarter-webp-results.json。
- Android assembleDebug（7s）、两仓 git diff --check 通过，新 Debug 已安装并启动小米；未新增或运行业务自动化测试、未触发真实上传/生产数据修改、未提交推送或发布部署。Server 为纯文档改动，无需重复 go build；规则静态检查仍为此前已记录的超长/元信息等问题。

## 2026-10-22：联合发布本人列表收藏状态

- 用户要求联合发布，并确认已在本次交互中登录生产机。发布范围为 MyTrackSummary/CollectedTrackSummary 明确下发当前 JWT 用户的 collected、已上传列表复用现有批量关系查询，以及对应 API/规则/既有断言修正；四图接口本来已下发，不新增字段或数据表。
- 两仓 git fetch origin 后均与 main 上游一致；go build ./... 与差异自查通过，未运行自动化测试。先同步提交推送本仓库，随后从该快照构建推送 linux/amd64 镜像；生产健康通过后才继续 App 1.0.4 (5)。不执行补图、清空或其他生产数据变更。
- 已核对用户本次新启动的 SSH master 进程（2026-10-22 18:58:09），只复用这次已认证连接，不读取或保存密码。镜像摘要、容器状态、MySQL 与内外 ping 的最终结果以本轮部署回执及最终回复为准。

## 2026-10-22：Me/搜索的方形轨迹预览使用

- 用户要求 Me 页已上传、本地、收藏及搜索结果 Item 使用 1:1 日夜预览，若接口未下发则补齐。核对 MyTrackSummary、CollectedTrackSummary、TrackSummary 已携带完整 preview_images 并经 trackPreviewURLs 组装鉴权方形/横图地址，本轮无需修改 Server 业务代码、新接口或表；docs/api/track.md 补充 Me/搜索消费 square_*、首页/地图关联消费 landscape_* 的说明。
- 用户明确本地未上传轨迹按需生成并缓存两张 1:1 日夜图、不自动上传。Android 复用原生成器与 Bundle，按可见项串行渲染并在滚动/离开时取消；远端列表直接加载已有 URL，不逐条请求详情或生成地图。缺图保留原缩略图/占位，主题切换重选图，不将整张地图按运动颜色染色。
- 最终 Android assembleDebug（3s）及两仓 git diff --check 通过，已安装小米，启动结果 MainActivity，随后前台为同 App 的 SearchActivity；本轮 Server 只改文档，无需重复 Go 构建。未新增/运行自动化测试，用户手动验证效果；前一轮收藏状态 Server 代码仍待部署，本轮未提交推送或发布。

## 2026-10-22：本人轨迹列表明确下发收藏状态

- 用户追问 Me 页三个列表是否都缺收藏状态，并要求修改 track-server。核对：已上传走 /track/my/list，收藏走 /track/collected/list，旧协议均省略 collected；本地列表来自 Android Room，不是 Server 接口，已上传本地轨迹进入详情时复用已有详情请求补齐元信息，未上传轨迹隐藏收藏入口。客态主页两个接口原本已按 JWT 浏览者返回状态。
- MyTrackSummary 与 CollectedTrackSummary 新增 json:"collected"（无 omitempty），true/false 都明确下发。fillMyTrackSummaryExtras 复用既有 CollectRepository.ListCollectedByTrackIDs 按当前页一次批量读取，不按作者身份推断、不逐条调用 IsCollected；MySQL/Mongo/内存三实现无需新增接口或表。收藏列表在现有 fillTrackSummaryExtras 的批量查询后，将 Collected 带回专用响应模型，不增加查询。
- API 通用模型、track、collect-navigation、route-index 与两端 AGENTS/客户端字段注释已对齐。Android 撤去上一轮按收藏列表来源强制 collected=true 的临时推断，直接透传服务端值；保留个人列表收藏事件对嵌套 TrackInfo 状态/数量的同步修复。现有收藏列表 handler 用例仅将“字段省略”断言改为新协议的 collected=true，未新增测试或运行测试。
- 验证：go build ./...、Android ./gradlew :app:assembleDebug --console=plain（3s）及两仓 git diff --check 通过；Debug 已安装并启动小米 3e7cd913，Status: ok / MainActivity，前台检查通过。未提交、推送或部署，新状态下发须部署本轮 Server 后由用户手动验证首屏/翻页及收藏后再次进入详情。
- AGENTS 静态检查仍有既有根文件/业务文件超长、Server 根优先级声明及路径误报、缺少 freshness 元信息、全局 /Users/foxcoder/.codex/AGENTS.md 为空等问题；未扩大治理范围或修改全局文件。日志在 /private/tmp/track-server-collected-agents.log、track-server-collected-freshness.log 与 android-track-collected-agents.log、android-track-collected-freshness.log；这些检查不算通过。

## 2026-10-22：运动类型英文展示名

- /track/types 增加 name_en，DefaultTrackTypeConfigs 的 NameEN 为 Hiking/Running/Climbing/Cycling/Driving；保留原 name、type、图标、颜色与统计，不做按请求语言覆盖 name，避免账号缓存跨语言污染。
- Android 使用当前 App 语言选择双语字段，旧缓存缺英文字段时用本地英文资源兜底；未知自定义类型 name_en 为空，原配置名保留。不增加数据库表、列或环境配置项。
- 协议、通用模型说明、路由索引与 AGENTS 已同步；本轮为代码修复，尚未提交推送或部署，不执行自动化测试。
- 验证：go build ./... 与两仓 git diff --check 通过；Android assembleDebug 通过并安装启动小米。未运行自动化测试；AGENTS 检查仍报告既有超长与元信息问题，未扩大本轮修复范围。

### 英文名称的 Server 部署请求

- 用户要求部署 Server，并已完成本次交互式 SSH 登录；仅同步提交推送本仓库的 name_en 字段、内置英文名称及协议文档，不发布或提交客户端。
- 部署前 git fetch origin、go build ./... 与差异自查通过；从已推送快照构建 linux/amd64 镜像后更新生产，核对镜像、MySQL、容器与内外 ping；本次镜像/容器/提交及最终结果以部署回执为准。

## 2026-10-22：DevMode 清空历史四图

- POST /api/v1/track/preview-backfill/clear 使用现有业务 JWT，无新增管理员或作者验证；请求显式 all=true 或 track_id 二选一。空作用域/冲突返回 400，单条不存在或已删除 404；返回 affected_count（重置轨迹数，包含原本无图记录）。沿用上传账号限制。
- 清空所有未删除记录或指定记录的四图地址；仅改 preview_images/updated_at，保留轨迹与 GPS/旧截图/OSS 文件/服务端图片缓存。无图返回空对象，已结束且有原始 GPS 的记录重新进入待补列表。
- TrackRepository.ClearPreviewImages 补齐 MySQL/Mongo/内存三实现；随机清空版本放在现有 JSON/BSON 内，不对业务 JSON 暴露，不增表/列。source_version 和 FillMissingPreviewImages 的 CAS 比较包含清空版本，阻止清空前任务重新提交旧图；成功补图保留版本以支持幂等重试。
- Android DevMode 确认后调用，成功重置本机进度与专用缓存、重新查询数量并清除列表预览地址；详情位置标签上方可复制真实 track_id。不会自动补图。
- go build ./... 通过；默认不新增或运行自动化测试，不执行线上清空/补图。本轮尚未提交推送或部署，新清空接口需要后续部署。API、路由索引、账号限制及 AGENTS 已同步；治理检查保留既有超长/元信息告警。

### 清空接口的 Server 部署请求

- 用户要求部署 Server，发布范围为上述清空接口、内部清空版本、三仓储 CAS 与协议文档；不提交客户端，不执行清空或补图操作。
- 部署前 git fetch origin 确认 main 与 origin/main 一致，go build ./... 与 git diff --check 通过；按发布技能提交推送此快照后构建 linux/amd64 镜像，使用用户本次新登录的 SSH master 更新生产，最终镜像、容器与健康检查结果以部署回执为准。

## 2026-10-22：DevMode 历史轨迹四图补齐

- 用户决定由 Android DevMode 手动生成全站历史轨迹四图，沿用业务 JWT，不增加管理员、作者或二次验证。`/track/preview-backfill/list|:track_id/data|:track_id/prepare|:track_id/complete` 均在 auth 组；prepare/complete 沿用上传账号限制。
- `internal/service/track_preview_backfill.go` 编排筛选、完整 GPS 和幂等补齐；`track_preview_backfill_oss.go` 为对应作者目录的独立 upload_id 四个对象签发 PUT URL，并经 OSS 内网核对 JPEG、大小、1024×1024/1024×768。客户端生成图，不使用服务器绘图。
- TrackRepository 新增待补分页、待补/无原始文件计数、条件写四图，三实现位于 `internal/repository/track_preview_backfill.go`。CAS 比较源地址、坐标系、运动类型和旧四图，只更新 preview_images/updated_at；已有完整四图直接跳过。无新表或字段，分页以 start_time/id 倒序，完成状态直接以四图为准。
- 协议见 docs/api/track.md，路由和限制索引已同步。Android 有账号隔离的持久化队列、失败记录与逐图上传断点；后台暂停后手动继续。
- 本轮未提交/推送/部署，也未执行线上批处理；需部署服务端后由手机 DevMode 手动启动。默认只做构建与自查，不新增或运行自动化测试。
- 最终 `go build ./...` 通过，两仓 `git diff --check` 通过；Android assembleDebug 通过并安装、启动小米。AGENTS 验证仍有原有的 root 超长/缺优先级说明、既有 Go 源码超 1000 行及全局入口为空等问题，freshness 缺时间元数据；未扩大本轮范围，未运行自动化测试或生产数据写入。

### 随后的 Server 部署请求

- 用户要求单独部署 Server，按发布技能同步、提交并推送本仓库，然后构建 linux/amd64 镜像更新生产；不提交或发布客户端，也不自动执行补图任务。
- 发布前 `git fetch origin` 确认 main 与 origin/main 同步；本次发布包括上述完整历史预览补齐接口、三仓储实现、专用签名上传、条件提交及文档。自查修正省略 source_version 查询时的原始 GPS 缓存键，始终使用实际源版本，避免取回旧文件。
- 必要的 `go build ./...` 与 `git diff --check` 通过；沿用用户本次新登录的 SSH master（进程启动于 2026-10-22 15:22:18），不读取或保存密码。镜像推送、线上容器与内外 ping 的最终结果以本次部署回执为准。

用户要求为 android-track 关于我们页的隐私政策、用户服务协议提供 track_server HTML 页面，沿用成长等级规则页风格、支持日间和暗夜；用户明确“不清楚的地方就先用 {xxxx} 这种占位”。

已完成：

- 增加公开的 `/api/v1/legal/privacy-policy.html`、`/api/v1/legal/terms-of-service.html`，通过 `legal_handler.go` 与 `go:embed` 返回自包含 HTML。
- 中文 / 英文正文、章节目录、协议互链；`lang=english` 切英文，`is_dark=true/false` 指定主题，省略时跟随系统。互链保留参数。
- 内容按当前轨迹录制、云端摘要、公开轨迹、同行、第三方地图 / 登录 / 条码 / 存储和自建统计整理；运营主体等未知事实保留占位，页面明确为草稿。真实授权、注销、清理、安全及跨境流程的待落实项见 `docs/api/legal.md`，页面不代表这些功能已经实现或合规认证。
- android-track 的 `AboutActivity.kt` 两个按钮接入现有 WebView，复用 API 主机和实际 App 主题。未修改 Android AGENTS 文件。
- 服务端 AGENTS 只为本次新路由补充地址及 H5 维护说明。用户询问“你咋把 agents 里的内容删了”：已解释整行替换在 diff 中表现为删除/新增；撤回了顺手扩充的其他接口说明，原有规则内容保留。

已验证：

- `GOCACHE=/private/tmp/track-legal-go-cache go test ./...` 全量通过；新增公开路由测试覆盖无认证 HTML 返回及查询参数。
- `GOCACHE=/private/tmp/track-legal-go-cache go build ./...` 通过。首次受限构建有缓存写入警告，授权后重跑无错误。
- `GRADLE_USER_HOME=$PWD/.gradle-home ./gradlew :app:assembleDebug --console=plain` 通过；安装至小米 `3e7cd913` 成功，启动后前台为 `MainActivity`。
- 隔离 Chrome 验证两页 × 两种语言 × 两种主题 × 320/390/1024 宽度共 24 组：无横向溢出、缺失锚点或外部资源请求，互链保留语言 / 主题，系统主题实时切换通过。人工查看手机宽度日间 / 夜间截图。临时证据在 `/private/tmp/track-legal-qa/`。
- 两个仓库 `git diff --check` 通过。
- `verify_agents.py` 已运行，仍报告既有 AGENTS 超长、缺少优先级声明及八个既有 Go 文件超长问题；全局 `~/.codex/AGENTS.md` 为空，入口规则未落盘。未扩大本次修复范围；推荐修复命令为 `python /Users/foxcoder/.codex/skills/agents-md-generator/scripts/manage_docs.py sync-global-codex-agents . --write`。

未提交或推送，未部署服务端。因此安装包已指向新地址，但线上服务器需更新后才能展示这些页面；未声称已完成真机到线上新页面的端到端验证。

## 2026-10-14：阿里云镜像构建与推送尝试

用户明确要求执行 `docker buildx build --platform linux/amd64 -t crpi-p78v4agazv8zn80d.cn-beijing.personal.cr.aliyuncs.com/track_server/track_server_go:latest -f deploy/Dockerfile . --push`。Linux amd64 镜像构建成功；首次推送返回 `insufficient_scope: authorization failed`，用户重新登录后已完成推送，过程如下。未重启线上服务。

- 本地镜像索引摘要：`sha256:3851c8778a92511d6ebc21117c32c51d0abb88f20d4450278ca2003c9114a6a7`；本地 latest 标签已指向该镜像。
- 使用该镜像启动无外网、内存仓储、禁用定时任务和统计的临时容器。`/api/v1/ping` 正常；两个协议 URL 均返回 HTTP 200、`text/html; charset=utf-8`，HTML SHA-256 与本地源码完全一致。
- 临时容器 `track-legal-image-check-20261014` 已停止并自动移除。
- 用户在本机重新登录并回复“已登陆”后，普通 `docker push` 上传镜像层成功，但 registry 拒绝附加证明清单：`unknown manifest class for application/vnd.oci.empty.v1+json`。
- 随后执行 `docker push --platform linux/amd64 crpi-p78v4agazv8zn80d.cn-beijing.personal.cr.aliyuncs.com/track_server/track_server_go:latest` 成功，将此前已验证的同一平台镜像推至远端，未包含镜像索引中的附加证明。
- 远端 latest 摘要：`sha256:96533346bb70446572c5c3b2994a7e9ff9b5e56384cd9da94659df88554dc987`。`docker buildx imagetools inspect` 独立读取确认摘要一致，远端配置确认 `os=linux`、`architecture=amd64`。该摘要是单平台 manifest；与本地多平台格式索引摘要不同属预期。
- 本次仅上传镜像，服务器仍需另行拉取并重建容器，线上更新尚未执行。
- 本轮没有修改 AGENTS.md 或业务源码。页面仍保留草稿提示及占位信息。

---

# 历史工作交接：存量标题回填与本机 Go 环境

日期：2026-10-09。

## 用户要求与当前结果

用户先要求查看存量轨迹能否改为投稿同类自动标题，确认后台支持改名后明确要求“去改吧”。用户随后提供 `ssh root@47.93.239.24`，说明使用密码登录。通过本机隐藏密码输入框完成认证，未把密码写入聊天、命令参数或文件；已在该服务器真实数据库执行并核对回填。

用户随后明确要求“别临时环境了，你给本机把 go 环境配上”。已安装官方 Go 1.27.2 darwin/arm64 到用户目录，校验官方 SHA-256 和文件大小；`~/.local/share/go/current` 指向 SDK。已备份并更新 `~/.zprofile`、`~/.zshrc`，加入 SDK 和默认 `~/go/bin` 路径。新登录/交互终端均可直接使用 `go`、`gofmt`，Hello World 编译执行成功。

## 已完成代码

- `cmd/backfill-track-titles`：MySQL 一次性回填工具，先生成 JSON 计划，再显式应用计划。计划含新旧标题并绑定目标数据库指纹，应用时检查作者、旧标题、未删除和已结束状态，只改标题及更新时间。结果逐条保存，已有输出不覆盖。
- `internal/service/track_title_backfill.go`：默认只识别常见日期时间标题，已补齐旧版“• / ·”分隔符；优先复用同作者投稿标题。显式 `-include-dated-titles` 同时清理导入标题日期前后缀、上次日期兜底标题，保留已有路线描述。地点缺失时按距离/运动类型生成，不再用日期兜底。未知运动类型保留原值；原始轨迹缺失时不猜测环线。
- `docs/backfill-track-titles.md`：完整操作步骤和边界。`AGENTS.md` 只增加对应入口与稳定边界，未改 HTTP 协议、Repository interface 或表结构。
- 新增测试覆盖标题识别、人工标题保护、环线与原路折返区别、缓存轨迹解析、投稿复用、计划校验、错误数据库和输出文件保护。
- 预览后增加历史地址清理：忽略“已定位”、纯门牌号和 Plus Code，括号文本截断时保留前面完整地点；新标题优先选择前面仍有效的街道/地点。新增相应测试。

## 已运行验证

- `go version`：`go1.27.2 darwin/arm64`。
- 新登录/交互 zsh 中直接查找及执行 Go 成功。
- `go run /tmp/track-go-env-check.go`：`Go environment OK`。
- `go test ./...`：全量通过。
- `go build ./...`：通过。
- `go build -o build/backfill-track-titles ./cmd/backfill-track-titles`：通过，本地产物已生成。
- `gofmt -l`（本次四个 Go 文件）：无输出。
- `git diff --check`：通过。
- Linux amd64/arm64 初版交叉编译通过；含最终地址清理规则的 Linux amd64 工具重新构建通过，与服务器上传文件 SHA-256 一致（`7ac61ea621367f4361dbb3f6634948e9fecc6238dccefb65606aaa13c4fbbdae`）。最终清理规则改动后再次执行 `go test ./...`、`go build ./...`，均通过。
- `verify_agents.py`：运行过，仍报告仓库既有 AGENTS 大于 16KB、缺少优先级声明、八个既有源码文件超长等问题；全局 `~/.codex/AGENTS.md` 为空、入口规则未落盘。未为本次功能扩大治理修复范围。可用技能提供的 `manage_docs.py sync-global-codex-agents . --write` 修复全局基线，写入用户目录需遵守沙箱权限。

## 服务器实际执行结果

- 目标：`root@47.93.239.24`，x86_64，业务容器 `track_server_go`，复用其现有 MySQL 环境变量与 `/var/log/track_server` 持久化挂载。未重启/替换业务容器。
- 初始预览：扫描 281 条未删除轨迹，筛选出 112 条时间型标题；2 条复用投稿标题，110 条按已记录事实生成；26 条有可确认形态的完整原始轨迹。
- 最终清理历史地址后的候选 ID、作者与旧标题范围与初版一致；修正 11 个低质量地址生成标题。
- 实际应用：`updated=112`、`skipped=0`。
- 独立只读核对：修改前后总记录数均为 354；112 条新标题全匹配，242 条其余标题完全未变，所有记录其余 22 个字段摘要完全一致，无新增或缺失记录。
- 服务器留存目录：`/var/log/track_server/maintenance/title-backfill-20261009/`。包含 `initial-plan.json`、最终 `plan.json`（作者/新旧标题）、`baseline.json`（全部旧标题与其他字段摘要）、`result.jsonl`、`verification.json` 及已使用的运行工具。
- 本地只读执行记录在已忽略的 `build/title-backfill-20261009-*` 文件中，不提交这些业务数据文件。

## 第二轮日期标题清理（用户继续授权）

用户查看结果后要求“还有一些标题依然是年月日的，你要不也都改了”。重新通过本机隐藏密码窗口认证后，检查到 123 条未删除轨迹仍带日期：106 条旧版“日期 • 时间”，9 条“日期 + 地区”导入标题，1 条“路线名 + 日期”，7 条上次日期兜底标题。全部已结束。

- 补齐“•”格式识别，加 `-include-dated-titles` 扩展预览范围；保留“走马岗”等原路线名，使用已记录地点和运动生成其他标题，无有效地点时使用距离/运动。新增“未知位置”、纯坐标、连字符门牌号过滤，避免将坐标或 `250-2` 生成地点标题。
- 最终全量 `go test ./...`、`go build ./...` 通过；Linux amd64 v4 交叉编译通过，服务器上传 SHA-256 与本地一致：`660db671e554653274d0232359b0f57e6344b4192f435ae383b6818f20b9e1d1`。
- 实际应用：`updated=123`、`skipped=0`；独立核对前后均 354 条记录，全部 123 条新标题匹配，其余 231 条标题和所有记录其他 22 个字段均未变。
- 再次 SQL 查询：未删除轨迹带年月日标题为 0；64 条已删除轨迹保持原值。完整扩展预览再次扫描 281 条轨迹，`candidates=0`。
- 新一轮服务器留存目录：`/var/log/track_server/maintenance/title-date-cleanup-20261009/`，包含初始/最终计划、baseline、result、verification、空 remaining-plan 及 v3/v4 二进制；保留上一轮备份目录。
- 本地核对文件在忽略目录 `build/title-date-*`；没有修改 Android 上传标题工厂，此次已完成的是现有数据库标题清理，不能声称所有未来新上传均不会带日期。
- 未重启服务，未更改 HTTP 接口或表结构。
- 提交前补齐无效投稿标题兜底：投稿标题仍是日期、过短或过长时，清空无效候选后按记录事实重新生成，测试同时确认投稿本身未被修改。全量测试和构建再次通过。

## 剩余事项

本次存量回填已完成。本提交保存回填工具、测试及操作文档，没有修改业务 HTTP 协议或表结构。此前 Android 工作副本的导航端点优先规则和创建轨迹标题改动仍是未提交工作，勿误覆盖。回填工具当前仅支持 MySQL，不得声称支持 Mongo。


## 2026-10-19：轨迹点赞与个人主页获赞

用户要求将轨迹详情底部收藏位改为心形点赞，收藏移至分享右侧；个人页真实获赞数、关注/粉丝旁新增获赞 tab。用户选择按单条记录展示，同一人赞多条轨迹保留多条。客户端和服务端均已实现，尚未提交、推送、构建远端镜像或部署。

- JWT 接口：POST/DELETE /track/:track_id/like、GET /track/:track_id/like/status、GET /user/:user_id/likes/received/list。MySQL track_likes 表、Mongo 唯一索引、内存实现同步。只允许正常公开、已结束、raw_track_url 非空的轨迹；禁止自赞，写入幂等。受限账号不能新增但可取消。详见 docs/api/track-likes.md。
- 用户详情 received_like_count 与获赞记录可见性一致，轨迹详情及摘要 liked/like_count 按浏览者计算；资源缓存失败不清零点赞。游标按时间、轨迹、点赞用户三级倒序。
- Android TrackLikeViewModel 防止重复请求，失败保留成功状态；ReceivedLikesFragment 支持刷新分页、头像/昵称进入个人页、轨迹文案/缩略图进入详情。原删除和投稿管理保留。
- go test ./... 通过；补充缓存回归后 service/handler/repository 复测通过；并发点赞 -race 通过。assembleDebug、assembleDebugAndroidTest、4 项地图布局 JVM 测试通过；4 项点赞 instrumentation 与 3 项客态主页回归通过，追加真实获赞数断言再次通过。MySQL/Mongo 实例联调未运行。
- 已安装并启动小米 3e7cd913，前台 TrackDetailActivity；截图确认心形和收藏位置。线上仍旧服务，点赞数显示“—”，须部署新版后验证线上互动。测试未向真实服务写入点赞或关注。
- 保留 Android 原有未提交的背景缩略图 3dp 圆角、详情地图缩短与居中调整。两仓库 git diff --check 通过。
- 服务端 AGENTS 追加点赞约束；规范校验仍有既有超长文件、优先级与全局入口问题，未扩大治理范围。


### 2026-10-19 跟进：数量展示统一

用户补充：轨迹已有点赞数需下发，零或缺失隐藏数字；大数中文使用万、英文使用 K，个人页遵从，沉淀客户端 AGENTS。服务端现有 like_count 字段及详情/列表下发测试已复核通过，数据继续保持原始整数；docs/api/track-likes.md 补充展示约定。客户端新增 CountFormatter / Context.formatDisplayCount，中文 10000、英文 1000 起缩写，最多一位小数并向下截断、去掉 .0；接入个人页关注/粉丝/获赞、轨迹 tab 数量及首页卡片收藏/跟随数。详情透传列表 liked/like_count，服务器详情加载也同步已有数量；状态成功后旧列表数量不能覆盖，零或未知隐藏数字，心形居中。app/AGENTS.md 已追加该规则并明确精确物理量不机械缩写。

assembleDebug、assembleDebugAndroidTest、CountFormatterTest 三项 JVM 测试及点赞/客态主页共 10 项 instrumentation 通过；TestTrackLikesIdempotencyViewerStateAndCounts 通过。安装并启动小米 3e7cd913，前台 TrackDetailActivity，截图确认无数量时心形居中。AGENTS 校验新增规则无告警，仍有既有 root 超长及源码超长问题，全局入口为空。未提交、推送或部署。


### 2026-10-19 跟进：预览轨迹线与允许自赞

用户指出不再上传地图截图。获赞列表右侧现在直接使用独立轨迹线 SVG（track_no_map_bg_screenshot_url），复用 HomeRoutePreviewView 的轨迹线绘制及缓存，56dp、3dp 圆角、低饱和蓝灰背景，日夜颜色均定义；缺少或加载失败显示路线图标。不再依赖 track_screenshot_url，回收清空预览、异步回调按绑定代次隔离。build 和两项 SVG 解析单测通过，点赞/个人页 10 项设备测试通过，含没有截图但 SVG 能画线及缺少 SVG 的兜底；已安装小米并启动 MainActivity。

随后用户修正规则：允许给自己的轨迹点赞，但不算获赞列表。已实现自赞计入轨迹 like_count，同时从个人页 received_like_count 和获赞列表排除，保证两者口径一致。MySQL/Mongo/内存均在排序分页前过滤自赞；我的轨迹 liked 改按当前用户计算，能正确回显自赞。新增接口测试验证重复自赞、本人列表状态、他人点赞与自赞共同计入轨迹、主客态获赞数/明细排除自赞、limit/has_more 不被自赞占位、重复取消。go test ./... 全量通过。服务端 AGENTS 和 API 文档同步，新规则覆盖上文“禁止自赞”的历史记录。规范校验仍有既有文件超长/全局入口为空问题。尚未提交、推送或部署；线上自赞等新接口须部署新版后生效。


## 2026-10-20：获赞按轨迹聚合与二级点赞人列表

用户担心爆款刷屏和列表过长，接受：首页按轨迹聚合，每轨迹一张卡片，最多 3 位最近点赞人、人数与最新时间；点击人数进入该轨迹分页点赞人列表，轨迹区域进入详情、头像进入主页。有效关系继续长期存在 track_likes，不另存消息副本，自赞计入轨迹总数并排除个人获赞数/列表。

- 新 JWT 路由 GET /user/:user_id/likes/received/tracks 与 GET /user/:user_id/likes/received/tracks/:track_id/users。保留旧 /likes/received/list 逐条入口兼容。首页 total_count 为轨迹组数，卡片 received_like_count 和二级 total_count 为他人点赞人数，个人页统计仍使用有效他人点赞总数。
- 新服务 track_like_groups_service.go、三份 track_like_groups_* 仓储：SQL 按轨迹 GROUP BY 后 HAVING 游标再 LIMIT，分页内最近 3 人使用有界索引查询合并一次返回；Mongo 从作者公开轨迹出发查关系聚合，分页后 lookup 最多 3 人，避免把爆款所有用户放进数组；内存语义一致。补 track_id/created_at/user_id 复合索引，MySQL 启动幂等迁移已存在的表。没有新增业务表。
- Android ReceivedLikesFragment 复用为聚合首页与点赞人模式，新增 TrackLikersActivity（通用导航栏）和聚合卡片布局。复用 SVG 轨迹线及淡蓝灰底，大数复用 CountFormatter。两层列表刷新/游标分页，按轨迹/用户 ID 合并实时排序产生的重复项，游标不前进时终止；直接定位列表末尾也能触发翻页。仍不依赖地图截图。
- 服务端 go test ./... 全量通过；新回归覆盖 120 赞合成一组、每页 1 组、最近 3 人、点赞人 50/50/20 分页、同时间游标、自赞过滤、取消点赞、私密轨迹与非法参数。实例 MySQL/Mongo 联调未运行，不声称已做线上联调。
- assembleDebug、assembleDebugAndroidTest 通过；CountFormatter 和 SVG 解析共 5 项 JVM 单测通过。设备 TrackLikesTest 与 GuestProfileTest 共 12 项最终全通过（首次新增直接定位翻页测试发现零位移回调未触发下一页，修复后通过）。测试截图仅含造数，文件 /tmp/received-like-preview-test.png。
- 已安装到小米 3e7cd913，启动后前台检查为 FollowListActivity（App 已在前台）；未向真实服务写入点赞/关注。没有提交/推送/构建远端镜像/部署；线上须更新新服务才能显示聚合列表。
- API 分册、路由索引、用户详情指引和服务端 AGENTS 稳定业务规则已同步。verify_agents.py 仍报既有 root 超长/缺少优先级、多个既有源文件超长、全局入口为空等，未做无关治理调整。保留 Android 先前圆角/地图布局/数量格式及点赞的未提交改动。


### 2026-10-20：用户授权同步提交推送

用户明确要求 server 和客户端代码均同步提交推送。两个仓库均为 main，git fetch origin 后 HEAD 与 origin/main 没有分叉，无需合并。提交范围包含本轮完整点赞/聚合获赞、数量格式与先前未提交的小幅界面调整，排除 APK、缓存、日志和敏感配置。提交前服务端 go build ./... 通过；沿用刚完成的 go test ./...、客户端 Debug/测试包构建、5 项相关 JVM 单测和 12 项设备回归结果。仍未部署服务端，线上新接口需要单独更新服务。


## 2026-10-20：移除获赞列表，保留获赞总数

用户明确要求：“获赞列表页和获赞列表的相关接口下掉吧……获赞数还是得留着的。”本节覆盖上文有关保留获赞列表/旧列表兼容入口的历史描述。

- android-track 移除获赞页、点赞人名单页及其 Manifest、布局、字符串和 Retrofit/repository/模型；个人主页获赞数保留且不可点击，关注/粉丝仅保留两个标签。
- track_server 移除三个 GET 查询路由（逐条记录、按轨迹聚合、轨迹点赞人），删除 handler/service、三仓储列表查询和游标模型。保留点赞、取消点赞、状态查询、轨迹点赞数和用户详情 received_like_count。点赞关系、表结构和已有索引保留；MySQL 幂等索引初始化函数仅移动到 track_likes_mysql.go。
- 内存仓储 CountReceived 改为独立计数，无需依赖已删除列表方法或排序；仍排除自赞、私密、删除、进行中、未上传和他人轨迹。
- 同步 docs/api/README.md、models.md、route-index.md、track-likes.md、user.md、中英文隐私说明及服务端 AGENTS.md 中的点赞约定。
- 已验证 go build ./... 与 go test ./... 全量通过；Android ./gradlew :app:assembleDebug :app:assembleDebugAndroidTest :app:testDebugUnitTest 通过；两个仓库 git diff --check 通过。
- 模拟器 emulator-5554：TrackLikesTest 与 GuestProfileTest 合计 9 个 instrumentation 测试通过，包含获赞数不可点击、两标签及重建、点赞/取消/数量显示等。服务端测试覆盖移除的三个路由返回 404、自赞不计个人获赞、幂等和可见性计数。
- 真机小米 3e7cd913 已安装 Debug 与测试包，启动 LoginActivity 后确认前台为 MainActivity。首次真机自动化停在既有详情用例，主动停止该次 instrumentation 并在模拟器完成回归，不把真机中止结果算作通过。
- 已运行 check_freshness.py 和 verify_agents.py；仍有既有 root AGENTS 超过 16KB、优先级/元数据缺失和 Go 源码超长等治理问题。全局入口规则未落盘（~/.codex/AGENTS.md 为空），推荐修复命令：python /Users/foxcoder/.codex/skills/agents-md-generator/scripts/manage_docs.py sync-global-codex-agents . --write。本次未扩展治理重构。

未提交、推送或部署服务端；接口下线在部署新版后生效，旧客户端访问原列表路径将收到 404。

## 2026-10-20：轨迹评论、回复与图片

用户授权客户端与服务端完整实现评论，UI 尽量复刻其小米截图，颜色沿用 App 主题。最终口径：普通 emoji、最多三张图片且允许纯图；先不做主动 @、话题、评论分享、编辑和通知；回复列表平铺，回复某条回复只引用直接目标。作者最新回复须直接展示在主评论列表。沿用点赞的轨迹可见范围。

- Android 新增独立 `:comments` Library；详情页底部接入评论入口及总数。评论/回复同尺寸浮层、回复页从左进入、返回保留主列表位置；键盘上方另开发布窗口，提供引用、草稿、图片选择/移除/预览、普通 emoji、1000 Unicode code point 限制及失败重试。宿主提供语义色、MingCute 图标、通用 AppModalDialog、用户主页、图片查看与 OSS 上传。日间蓝色、夜间主题强调色均复用现有颜色。
- 服务端新增 `CommentService`、鉴权 handler、MySQL/Mongo/内存三种仓储与四张表/collection，接口见 `docs/api/track-comments.md`。主评论/回复分别倒序游标分页；仅当前页附最新未删除作者回复，批量装饰用户/引用/评价，头像不逐条同步下载。总数、楼层回复数、互斥赞踩随事务维护；按用户和 client_comment_id 幂等，冲突内容返回 409。
- 仅本人删除；仍有回复的已删主评论保留占位，最后一条回复删除后隐藏；删除引用目标后不下发原文/图片。账号限制覆盖发布、图片预约和非零赞踩，允许读、删本人及取消赞踩。私密/删除/进行中/未上传轨迹隐藏评论，恢复公开后恢复可见。
- 图片先预约后直传精确 OSS key，发布验证归属、实际 JPEG 尺寸和大小，最多三张、5MB/张。图片走评论鉴权地址，私有缓存位于 LogDir/comments/images；不挂通用 static、不下发原始 OSS 地址。需要现有 STS 和 OSS_INTERNAL_ENDPOINT；未配置时文本功能可用、图片预约 503。
- MySQL 发布/删除锁定轨迹统计行，赞踩仅锁对应评论；Read Committed + 有界死锁重试，避免并发首次创建统计行的死锁。Mongo 使用事务，发布/删除经统计文档建立冲突边界、赞踩按评论冲突重试；部署须为副本集，compose 已补单节点副本集配置。未部署或修改生产数据库。
- 验证：go test ./...、go build ./... 通过；评论 handler/repository 的 go test -race 通过；临时真实 MySQL 8.4、MongoDB 7 副本集及内存同一契约测试全通过，覆盖分页、同时间游标、并发重试、计数、赞踩、占位与图片绑定回滚。测试容器已停止并移除。
- Android Debug/测试 APK 构建、app 全量 JVM 测试、comments 三项单测通过；新增协议测试验证 URL 解析及发布字段。独立 emulator-5556 上三个 UI 用例在浅色与深色各通过一次，覆盖作者预览、引用回复、emoji 发布、赞踩切换/取消、分页和纯图选择/移除/发布。先前 emulator-5554 受其他应用自动化抢前台影响的失败不计为功能通过证据。
- 浅色/深色的主列表、回复页及发布窗口截图位于 android-track/build/outputs/comments-qa/，均为测试造数。已安装 Debug 到小米 3e7cd913，启动 LoginActivity 后确认前台 MainActivity。未向线上写入测试评论；真实 OSS 上传未做线上联调。
- 模块 README、API 分册、路由索引、数据模型、中英文隐私说明及两端 AGENTS 已同步。git diff --check 通过。check_freshness/verify_agents 已运行；既有 AGENTS 元数据缺失、文件超长、全局入口未落盘和旧源码超长仍存在，未扩展无关治理重构，修复入口见上一节。

尚未提交、推送或部署。手机上的新版客户端需服务端部署后才能使用线上评论；Mongo 独立实例需先启用副本集。保留本轮之前获赞列表移除、获赞数保留及海拔地址修复的未提交改动。

### 2026-10-20 跟进：评论元信息合并、空态、首次回复与单图

用户指出评论数应随轨迹信息下发，并连续提供小米参考：空评论列表的气泡插画/首评入口，零回复时先进入回复列表再自动打开输入窗口，以及图片只允许一张。本节覆盖上文“独立评论状态接口”和“最多三张图片”的旧实现。

- 已移除 GET /track/:track_id/comments/status、handler/service 方法与客户端 Retrofit/Gateway 调用。Track、TrackSummary、MyTrackSummary、CollectedTrackSummary 直接下发 comment_count 和 can_comment；创建/更新响应同步。详情在资源 URL 缓存处理前计算可用性，避免缓存失败误隐藏评论；列表按当前页轨迹 ID 从统计表批量读取，不逐条计数。MySQL、Mongo、内存均新增 Totals 实现。
- 客户端从列表透传评论元信息，在轨迹详情加载回调中同步；仅有本地缓存时复用现有轨迹详情接口补齐。发布/单条/赞踩响应中的 comment_count 为整条轨迹的有效评论+回复总数，删除返回 status/comment_count。发布与删除直接更新入口/弹窗数量，不额外查询计数。旧服务缺少字段时按既有轨迹信息兜底入口可用性，评论功能本身仍须部署后端才能使用。
- 空评论列表标题显示“评论”，使用主题色气泡插画、“暂无评论，点击抢首评”和固定底部“鸟过留鸣，人过留评”输入；点击空态直接打开发布。加载失败保持重试提示，不展示“暂无评论”。修复通用底部弹窗在空列表时收缩的问题，指定高度时按窗口可用高度完整测量。
- 主评论零回复时点击回复，先进入该条评论回复页，再自动打开发布窗口与软键盘；关闭发布后停留回复页并保留草稿。标题零回复显示“回复”；有内容时显示数量。首次显示输入窗口显式申请焦点及 IME，等待窗口焦点后弹出，避免需再点一次输入框。已删主评论仍有回复时保留查看回复入口。
- 评论图片改为客户端/服务端每条最多一张，允许纯图；系统单选，灰色输入区内显示缩略图，支持预览、移除、重选替换。服务端对 image_ids 长度大于 1 返回 400。API 文档、模型说明、路由索引、模块 README 与两端 AGENTS 已同步。
- 验证：go test ./...、go build ./... 通过，新增详情/推荐/搜索/我的轨迹/收藏/个人轨迹评论元信息、发布/删除总数、私密轨迹、资源缓存失败及多图拒绝回归。批量计数契约在临时真实 MySQL 8.4、Mongo 7 副本集及内存均通过，测试容器已停止并移除。Android Debug、测试 APK、app JVM 全量和 comments 单测通过；六项 UI 回归在浅色/深色各通过，覆盖评论数入口、错误重试与空态、纯图移除/替换/发布、作者预览与引用/赞踩、分页、首次回复与草稿。最后调整空态插画位置后，空态用例又在两种主题各补测通过；断言文字实际可见、列表高度完整、IME 自动可见。
- 浅色/深色最终截图为 android-track/build/outputs/comments-qa/{light,dark}-{empty,first-reply,single-image}.png，均为造数回归；已逐张检查。参考原图保存于 /private/tmp/track-comments-{empty,first-reply,single-image}-reference.png。Debug 已安装到小米 3e7cd913 并启动，前台 TrackDetailActivity；真实详情截图确认点赞右侧已显示评论入口（/private/tmp/track-comments-entry-final.png）。没有向线上写入测试评论或测试图片，真实 OSS 上传仍未线上联调。
- git diff --check 通过。AGENTS freshness/verify 已运行，仍有既有 root 超长、元数据/优先级、旧源码超长及全局入口未落盘问题；未扩展治理重构。代码仍未提交、推送或部署，新后端接口和真实计数须部署后生效。

### 2026-10-20 跟进：评论浮层、长按操作与时间展示

- 按小米截图调整 Android 评论界面：列表/操作菜单底部滑入并向下退出，关闭图标、回复按钮、赞踩图标缩小；保留赞踩点击区域。独立发布窗口外框直角，输入区保留小圆角，移除字数标识但仍限制 1000 Unicode code point。通过 CommentSheetKind 区分列表/发布/操作菜单，由通用 AppModalDialog 承载，未修改普通居中弹窗。
- 本人的未删除评论与回复在正文、引用、图片及行空白处支持长按，底部显示“删除、回复、复制、取消”；移除行内删除文字。复制仅复制正文，纯图复制项禁用；回复复用原有两层/引用逻辑，删除保留宿主确认。菜单随父评论窗口关闭，防止悬空。
- 用户明确时间只是示例，由实现决定完整规则：不足 1 分钟“刚刚”、不足 1 小时“几分钟前”、不足 24 小时“几小时前”、不足 7 天“几天前”；更早 MM-dd，跨年 yyyy-MM-dd，本地时区，英文单复数。时间由客户端派生，每分钟只更新可见行，不新增服务端请求、不重建列表；异常时间隐藏，轻微未来时间按“刚刚”兜底。
- 验证：Debug/测试 APK 构建、app 229 项与 comments 6 项 JVM 单测通过，时间回归含全部分界、跨年、本地时区及未来/非法时间。专用 emulator-5556 上七项 UI 用例在日间和夜间各全通过，新增长按取消/复制/回复/删除及无行内删除、无字数标识；首次新增用例匹配到隐藏回复页的同文节点，改为仅匹配可见节点后全量通过。运行时窗口属性确认使用新的底部位移动画。
- 最终截图 /private/tmp/track-comments-qa/{light,dark}-{main,composer,actions}.png 均为测试造数，已逐张检查。Debug 已安装到小米 3e7cd913，am start -W 确认前台 TrackDetailActivity。模块 README 已更新；本轮没有服务端代码/协议变化，未重复运行 Go 测试。没有线上测试写入、提交、推送或部署，服务端上线要求延续上一节。

### 2026-10-20 跟进：浮层单轴入场、右侧回复页与评论点赞人

本节覆盖此前“回复页从左侧进入”的旧实现。用户要求评论浮层从底部丝滑弹出、回复与点赞人页从右向左进入；回复页主评论 0 赞显示“暂无人赞过”，正数可打开参考截图中的头像昵称名单，不做游客行。

- AppModalDialog 先调用 Dialog.create 完成 AlertController 内容创建，再在窗口 attach 前固定宽高、底部居中、遮罩和导航栏。AlertController 会重置宽高，既不能在 create 前设置后任其覆盖，也不能等到 onShow 才拉成全宽；此前先升起再横移就是显示后尺寸重排。入场只用 280ms 纵向窗口动画。
- 独立 CommentPaneAnimator 管理同一浮层内的水平翻页：前页保留到入场完成，回复页及 CommentLikersPane 从右侧进入，返回向右退出；取消旧动画使用代次校验，防止迟到回调隐藏当前页。前一页及滚动状态保留。
- 主评论无赞占位不响应点击，有赞显示人数与箭头；名单只在点击后请求，分页按用户 ID 合并，失败明确显示重试且不会因滚动回调无限自动请求。支持点击用户访问主页，赞踩切换后人数入口跟随响应更新。
- 新 GET /track/:track_id/comments/:comment_id/likes，标准分页 items 为 id/nickname/avatar_url，total_count 复用 like_count，取 limit+1 判断 has_more。只列当前 reaction=1，含自赞，取消/点踩移除；按 user_id 倒序稳定分页，不表达点赞时间先后，游标绑定轨迹/评论。已删评论（含有回复的占位）、不可见轨迹返回 404，登录与受限账号读取规则沿用评论。没有恢复已下掉的轨迹点赞人或个人获赞列表接口。
- MySQL/Mongo/内存均实现查询。MySQL 启动为旧表幂等补 idx_comment_likers(comment_id,reaction,user_id)，基线 mysql.sql 同步；Mongo 同字段索引、降序 user_id，仅读索引字段并排除 _id。无新增业务表或名单副本；头像昵称按当前页批量装饰，复用异步头像缓存。
- API 分册、路由索引、入口 README、模块 README 与中英隐私说明已同步。go test ./...、go build ./...、评论点赞人/仓储契约 race 检查通过；临时真实 MySQL 8.4、Mongo 7 副本集和内存契约通过，覆盖稳定分页、正负评价过滤、自赞、取消/切换及旧表缺索引补齐与重复执行。临时容器已停止并清理。客户端 Debug/测试包构建、app 229 项与 comments 6 项单测通过。
- 九项 TrackCommentsTest 在日间/夜间各通过；专用 AVD 原有 window_animation_scale 为 0，恢复为 1 后再次完整运行日间九项也全部通过。测试显式匹配 Dialog，并在创建时同步登记动画等待时间、跟随异步 onShow 更新，避免点击移动中的窗口；不改产品交互来迎合测试。断言验证回复/点赞人页的初始 translationX 位于右边界之外，结束为 0；同时覆盖名单失败不自动重试、手动重试、25 人分页、用户点击、无赞占位与赞/取消、返回保留评论状态以及既有发布图片流程。
- /private/tmp/track-comments-qa/{light,dark}-{likers,no-likes}.png 为造数回归最终截图，均已检查；正常动画倍率的连续帧确认浮层保持全宽从底部进入。小米 3e7cd913 已安装最终 Debug，前台 TrackDetailActivity；真机已存在评论数据，本次未发送测试评论、图片或点赞。未提交、推送或部署服务端，新点赞人接口需随这次 track-server 更新上线。两仓库 git diff --check 通过。专用模拟器与临时数据库均已清理关闭；保留其他任务和此前的未提交修改。


### 2026-10-20 跟进：紧凑评论、全文展开与用户手动验证约定

- 用户要求停止在专门编写/执行测试上投入时间，由其本人验证。android-track 与 track_server 根 AGENTS 均增加“默认验证方式”：默认不专门新增或运行自动化测试、不启动模拟器或测试数据库；保留必要编译、自查，Android 沿用已授权安装/启动。仅用户明确要求时执行指定测试范围。同步移除 Android 原“优先补/跑测试”、服务端全量测试及 admin scope 的默认要求；已有测试保留。
- Android 赞踩统一：点赞为 track_like_active 的红色实心心形，点踩用官方 MingCute 实心裂心并着次要灰色，同时替换对应线性素材。回复按钮使用独立 day/night 背景与正文前景色。
- 评论/回复头像改为 28dp、正文 14sp、昵称 11sp，标题 15sp/header 52dp，行间距与操作图标收紧。按参考图比例调整，尚待用户在真机判断对齐效果。
- 新 CommentContentView 在正文超过六行时保留前五行，在第六行约半行处追加省略号及高亮“全文”，按 Unicode 字素边界截断；点击全部展开且无收起。展开 ID 由主/回复页共享，滚动、分页、赞踩与来回导航保留，实际重新刷新该列表后重置。模块 README 同步。
- 在用户改约定前曾新增一项全文 instrumentation 用例，测试 APK 编译遇到多余括号；现已修正语法，但按新约定不再构建或运行测试 APK，不声明新增用例通过。前一阶段的历史测试结果见上文，不能作为这次最终界面的验收证据。
- 本次最终仅运行 ./gradlew :app:assembleDebug（通过），Debug 已安装小米 3e7cd913，启动 LoginActivity 后返回 MainActivity。未自动操作评论业务、未在线上造数。专用 emulator-5556 已关闭，未动其他模拟器。两仓库 git diff --check 通过；无服务端业务代码变化，未重复构建或测试 Go。
- AGENTS verify/freshness 已运行；仍报告既有根文件超 16KB、旧源文件超长、server 缺优先级与 freshness 元数据等。全局入口规则未落盘（~/.codex/AGENTS.md 为空），修复入口仍为 python /Users/foxcoder/.codex/skills/agents-md-generator/scripts/manage_docs.py sync-global-codex-agents . --write；本次仅更新用户要求的验证偏好，未扩展治理重构。

四项新界面效果由用户手动验证，当前未做最终日夜间截图/自动化验收；未提交、推送或部署。


### 2026-10-20 跟进：登录页夜间按钮、移除光晕与协议入口

- 用户补充要求顶部及底部光晕全部去掉；activity_login.xml 已移除两个装饰 ImageView。
- 手机号登录按钮原 track_surface_deep 几乎融入夜间底色，现复用 bg_login_button_primary 与 track_primary_text：夜间浅黄绿底、深橄榄文字/图标，日间浅蓝底、主题蓝文字/图标，沿用现有语义色，无新增硬编码颜色。
- 登录协议通过中英文资源 annotation 标记两条链接，ClickableSpan 使用 track_highlight_text、无下划线；仅对应协议名可点击。新增 web/LegalPage.kt，将关于页既有协议 URL、语言和日夜参数封装为共用入口，登录页与关于页均打开既有 WebViewActivity，未改变登录态或协议正文。
- ./gradlew :app:assembleDebug 通过，git diff --check 通过；Debug 已安装小米 3e7cd913。遵循用户手动验证约定，未新增或运行测试，未启动模拟器或测试容器。界面与协议点击效果待用户验收。
- 本次无服务端业务代码/协议变化、无新的长期业务或开发规则，故未扩写 AGENTS。未提交、推送或部署；保留此前全部未提交改动。


### 2026-10-20 跟进：底部星星收藏与详情收藏数

- 用户要求收藏放到评论旁边、空心/实心星星状态，并由轨迹信息下发收藏次数。Android 已移除右上角收藏入口，底部顺序为点赞、评论、收藏；MingCute 官方 star regular/filled 素材转为 vector，收藏使用主题强调色，未收藏次要色。数量为 0/未知隐藏，使用 CountFormatter，点击区禁用覆盖请求中状态。
- 三个社交按钮及导航宽度收紧为 44dp、高度维持 52dp、间隔 8dp；上传/投稿文字单行自适应，给新增收藏留出空间。收藏数先用列表首屏值，再由已有详情回调补齐；自己的列表未下发 collected 时复用轨迹详情元信息刷新。无新增计数接口，CommentsController 的详情补齐回调同步提供收藏元信息。收藏成功后的权威状态不被迟到的初始详情覆盖，重建保存状态/数量；收藏事件继续分发到首页、搜索和个人列表。
- Track 新增临时 collected/collect_count，详情、创建/更新响应填充；列表沿用已有批量 CountByTrackIDs，不逐条新增查询。track_collect_summary.go 复用收藏仓储统计现存用户/轨迹唯一关系，包含作者自藏。详情 collected 取当前 JWT 浏览者。POST/DELETE track_collect 保留 status 并增加操作后读取的 collected/collect_count，客户端优先采用响应；旧服务器未带字段时保留原有本地加减兼容。
- MySQL 继续复用已有用户/轨迹唯一关系和 idx_collects_track，无新表或计数列。Mongo 补齐原有 AddCollect、RemoveCollect、RemoveByTrackID stub：setOnInsert upsert 保留首次收藏时间、唯一冲突按幂等成功处理；取消与轨迹删除清理关系，新增 track_id 索引支持现有聚合。内存重复收藏不再刷新时间，口径与 MySQL/Mongo 对齐。
- API 详情/模型/收藏分册与路由索引、两端 AGENTS 稳定约定已同步。go build ./... 与最终 ./gradlew :app:assembleDebug 均通过，两仓库 git diff --check 通过；最新 Debug 已安装小米 3e7cd913 并执行基本启动检查。遵循用户约定，未新增/运行测试、未启动模拟器或数据库容器，界面、收藏切换与数量由用户手动验收。
- AGENTS verify 已运行，仍有既有根文件超长、server 优先级/路径提示、旧源码超长及全局入口为空等；没有扩展治理重构。未提交、推送或部署服务端，详情收藏新字段与操作后权威计数需部署本次服务端版本才能线上生效。保留此前登录页、评论、获赞列表下线、海拔等全部未提交改动。


### 2026-10-20 跟进：Me 背景图固定比例与裁剪页沉浸栏

- 用户要求编辑资料选择 Me 背景图时固定展示宽高比，并修复裁剪页未沉浸状态栏。ProfileBackgroundActivity 开启 fixAspectRatio，使用当前窗口宽度与 120dp 身份区 + 64dp 统计区 + 16dp 重叠区 + 状态栏高度作为比例；三个区域尺寸提取到 dimens，ProfileFragment 的布局和 inset 更新共用，避免日后裁剪比例与实际背景区域失配。仍支持固定比例选取范围和多指缩放，JPEG 最长边限制继续 2048。
- 新 TrackImageCropActivity 继承现有 CanHub CropImageActivity，保留原裁剪视图、旋转/翻转菜单、状态恢复和结果返回；仅以应用 Toolbar 承载顶部操作，NoActionBar 主题去掉独立系统 ActionBar。TrackImageCropContract 委托原契约构造参数/解析结果后切换到新 Activity，背景与头像入口统一接入，头像仍固定 1:1。Manifest 移除未使用的原库 Activity，防止进入旧裁剪窗口。
- 状态栏/导航栏透明、关闭系统对比度遮罩；容器背景贯穿系统栏，按 systemBars/displayCutout 为按钮和内容补边距，系统图标明暗由裁剪页背景亮度决定。背景裁剪显式使用既有黑色 viewer/白色 foreground 语义色，头像沿用日夜主题。app/AGENTS 已记录共用入口与固定比例约束。
- ./gradlew :app:assembleDebug 通过，git diff --check 通过；Debug 已安装小米 3e7cd913，执行基本启动检查。遵循用户约定，未新增/运行测试、未启动模拟器，裁剪操作、比例和沉浸效果待用户手动验收。AGENTS 验证仍报既有根文件/旧源码超长等，无关治理未扩展。无服务端业务变化、未提交推送或部署。
- 上一小轮详情页导航文案已改为中文“跟随轨迹”/英文“Follow track”，单独编译安装通过，包含在当前包内。


### 2026-10-20：用户授权两端同步提交推送

- 用户明确要求“客户端和server的代码都同步/提交/推送”。已分别 fetch origin，两仓库 main 与 origin/main 均为 0 ahead / 0 behind，无需合并，按当前 main 提交推送。
- 提交本轮完整评论/回复/单图/赞踩及点赞人查询、获赞列表移除与总数保留、收藏计数与底部星星，以及 Android 海拔地址、登录协议/主题、固定比例背景裁剪与沉浸状态栏、导航文案和手动验证约定。保留已写的测试代码；遵循用户新约定，本次不再新增或执行测试。
- 最近最终 Android :app:assembleDebug 与服务端 go build ./... 已通过，Android 已安装小米；本次仅提交前检查改动及 git diff --check。客户端 .gitignore 的 build 规则改为覆盖全部模块，排除 comments/build 等生成目录；APK、缓存、本地签名和敏感配置均不纳入提交。
- 使用中文提交信息记录完整功能、协议/数据结构及验证范围；未部署服务端。新评论接口、收藏数及下线获赞列表需由部署新版服务端生效。提交/推送最终结果以本次对话输出的 commit 和远端一致性检查为准。


### 2026-10-22：静态轨迹预览与共享卡片联合发布准备

- 用户确认共享首页 Item 的复用页面跟随改版，并明确要求上传四图、列表/详情下发地址；随后要求“联合发布”。两端 main 已 fetch，均与 origin/main 一致。
- 新增 preview_images 四图值对象及 MySQL JSON 启动迁移，Mongo 更新同步；创建可选、更新整组替换，所有摘要与详情返回带源版本的鉴权图片 URL。
- 图片读取按轨迹可见范围检查，复用 AssetCacheService 和归属用户 OSS 凭证按需下载，缓存位于 LogDir/track_previews，未挂 static。列表不下载四图、不绘制地图。
- 新增入口：internal/models/track_preview.go、internal/service/track_preview.go、internal/handler/track_preview_handler.go、internal/repository/track_preview.go；同步 mysql.sql、main.go、相关协议和 AGENTS。此前 MingCute 线性图标及运动日夜色也纳入本次部署提交。
- go build ./... 已通过，git diff --check 通过；遵循用户约定未新增或运行自动化测试，未启动测试数据库。现有 AGENTS 校验仍报告旧的长文件、元信息和路径告警，未扩大整改范围。
- 发布遵循 Android 仓库 .agents/skills/android-track-release：先提交推送服务端，再构建推送镜像，用户当次 SSH 登录，部署健康后才发布 App。实际部署、App 版本及远端提交结果以本次会话最终输出为准。
