# 后台账号与页面权限

后台独立使用 `/admin` 路由和 `admin_session` cookie，不使用 App 用户 JWT，也不复用业务用户表。

## 身份与迁移

- 管理员拥有全部业务页面及操作权限，可以管理账号身份、启停和页面权限。本人密码与邮箱归本人管理，管理员不能代改已验证邮箱账号的密码或邮箱。必须保留至少一个启用的管理员。
- 操作员逐页面配置 `none`（无权限）、`read`（只读）、`write`（可操作）。可操作包含该页面的编辑、删除、审核、发布等业务动作，不再拆动作权限。账号管理和操作日志固定仅管理员可用。
- 首次部署保留当前全部 `ADMIN_ACCOUNTS`，以及兼容的 `ADMIN_USERNAME` / `ADMIN_PASSWORD_HASH`；配置必须包含登录名 `admin`。首次导入时仅 `admin` 为管理员，其余现有账号默认所有当前业务页面只读，保留原密码。
- 导入完成后以数据库为唯一权威。重启不会覆盖密码、身份、权限或启用状态；环境变量以后新增的账号也不会被自动导入。新账号应从后台创建，新操作员未分配的页面默认无权限。未来新增页面不会自动授权给已有操作员。
- 新增账号和重置密码使用临时密码，下次登录必须先修改；原有账号迁移不强制修改密码。登录名创建后固定，显示名称可改。密码至少 8 个字符、最多 72 字节，只保存 bcrypt 哈希。

## 存储与失效

`cmd/server/main.go` 注入当前 MySQL/Mongo 的独立 `AccountStorage`，自动创建 `admin_account_state` 与 `admin_audit_logs`；MySQL 基线同步在 `mysql.sql`。账号状态采用单行/单文档 JSON 和整体 `version` CAS，确保跨进程的首次导入、最后管理员保护和权限修改原子生效；每个账号的 `revision` 用于编辑冲突检测。

登录会话仍只存在内存，重启需要重新登录。每个后续受保护请求重新读取账号权限；停用、重置密码或本人改密码都会增加 `security_version`，让所有进程中的旧会话失效。已经开始执行的请求按请求开始时的权限处理。

邮箱、验证时间、各用途验证码摘要/错误次数/有效期、发信配额与管理员重置迁移开关也保存在同一账号 JSON 中，使用相同 CAS，不新增表或内存验证码回退。绑定/更换邮箱和邮箱找回成功同样增加 security_version，并清理全部旧挑战。完整机制、SMTP 配置与人工验收边界见 [邮箱找回配置说明](../admin-email-recovery.md)。

账号库不可用或状态损坏时拒绝登录与受保护操作，不使用环境变量直接鉴权，不回退到内存账号。`USE_IN_MEMORY` 不提供后台账号服务。无初始账号且数据库未初始化时不挂载后台；初始化后移除初始账号环境变量不会禁用已有账号。

## 授权规则

模块清单为工作台、投稿审核、轨迹管理、景点管理、用户管理、地图模式、与友同行、反馈与纠错、App 发布、埋点同步。前端导航、控件和身份标签由 `static/shell.js` 统一维护，服务端 `permissions.go` 是实际授权权威。

- 无权限：菜单隐藏，直接请求 HTML/API 也拒绝。
- 只读：允许列表、详情和查询，禁止写请求；获取发布上传凭证虽为 GET，也要求可操作。
- 可操作：允许页面全部业务操作。城市选项和共享头像按实际使用模块放行，业务截图按所属模块校验。
- 景点页的相关轨迹列表也要求轨迹查看权限；轨迹编辑/删除仍检查轨迹可操作权限。景点轨迹归属写接口同时检查景点和轨迹的可操作权限。
- 景点识别候选放在共享轨迹详情弹窗中，读取需轨迹查看权限，确认/忽略需轨迹可操作权限；从景点相关轨迹入口打开弹窗也遵循该规则。说明见 [admin-tracks.md](admin-tracks.md)。
- 工作台响应按各业务模块裁剪数据；轨迹、投稿和反馈页面复用 `/admin/api/dashboard?scope=tracks|track_submissions|feedbacks`，只返回自身统计，不能借此读取其他模块。
- 只读景点页不运行供应商资料回填。公共静态资源目录不提供 HTML，所有业务页面通过具名鉴权路由访问。
- 受保护响应不缓存，写请求校验同源。权限不足返回 403，登录失效返回 401，账号库不可用返回 503，编辑版本冲突或移除最后管理员返回 409。

## 页面与接口

页面 `/admin/accounts.html` 维护账号并展示最近操作记录，记录区提供“全部账号”和各后台账号的下拉筛选（含停用账号，显示名称与登录名）；`/admin/password.html` 为本人的“账号安全”，包含改密与找回邮箱验证；登录页的“忘记密码”进入公开 `/admin/forgot-password.html`。无业务页面授权时 `/admin/access.html` 提示联系管理员。登录响应和 `GET /admin/api/me` 的 `data` 包含 `username`、`display_name`、`role`、`permissions`、`must_change_password`、`expires_at`、`home` 和不含密码哈希的 `account`。登录后按 `home` 跳转。

共享账号区只显示名称、身份标签与齿轮。桌面端位于侧栏左下方，齿轮菜单向上展开；移动端保留顶栏紧凑布局，菜单向下展开。菜单展示当前账号的登录名、名称、身份和最近登录时间（`account.last_login_at`），打开时通过现有 `/admin/api/me` 刷新信息；“账号安全”入口保留原改密页 URL，退出调用现有 `/admin/api/logout`，失败时保留当前页并提示重试。菜单中不展示密码、密码哈希或内部版本字段。

| 方法 | 路径 | 权限 | 请求/响应 |
| --- | --- | --- | --- |
| GET | `/admin/api/accounts` | 管理员 | `data.items` 账号列表、`data.modules` 页面目录；各账号增加 recovery_email_bound/can_reset_password，不返回完整邮箱、密码哈希或挑战信息 |
| POST | `/admin/api/accounts` | 管理员 | `username, display_name, role, enabled, permissions, password` |
| PUT | `/admin/api/accounts/:username` | 管理员 | `display_name, role, enabled, permissions, revision`；整体替换页面权限，未包含项默认无权限 |
| POST | `/admin/api/accounts/:username/reset-password` | 迁移期管理员 | `password, revision`；仅目标未验证邮箱且旧入口未永久关闭时可应急重置，否则 403；重置使旧登录/验证码失效并要求改密 |
| POST | `/admin/api/password` | 已登录，包括待改密账号 | `old_password, password`；成功后重新登录 |
| GET | `/admin/api/audit-logs` | 管理员 | 可选 `username` 查询参数，精确筛选该登录名的最近 100 条操作发起/HTTP 结果记录；不传或空串为全部账号 |
| GET | `/admin/api/security` | 本人 session | `data.email, email_verified, email_recovery_enabled`；存在有效新邮箱挑战时另含 pending_email/verification_expires_at，不含验证码 |
| POST | `/admin/api/security/email/current-code` | 本人 session，已完成首次改密 | `password`，向当前已验证邮箱发送换绑证明码 |
| POST | `/admin/api/security/email/send-code` | 本人 session，已完成首次改密 | `password, email, current_code`；首次绑定不需 current_code，换绑需原邮箱验证码 |
| POST | `/admin/api/security/email/verify` | 本人 session，已完成首次改密 | `code`；验证新邮箱后保存，退出全部旧登录；返回 status=ok/login_required=true |
| GET | `/admin/api/password-recovery/status` | 公开 | `data.enabled`，只表示配置能力，不保证邮件最终送达 |
| POST | `/admin/api/password-recovery/request` | 公开 | `username, email`，格式有效且队列接收时统一 202 + 通用 data.message；匹配与否/投递失败不通过响应泄露 |
| POST | `/admin/api/password-recovery/complete` | 公开 | `username, email, code, password, confirmation`；验证后保存新密码并退出全部旧登录，返回 status=ok/login_required=true，不自动登录 |

邮箱发送接口成功返回 `data.message, expires_in=600, resend_after=60`，表示 SMTP 已接受邮件，不等于收件箱最终送达。公开申请仅表示有界队列已接受，返回 resend_after=60；验证码是 8 位数字，10 分钟、一次有效、最多 5 次错误；实际发信另有每用途/每账号限制。公开完成接口无效账号、邮箱、验证码统一失败描述。限额返回 429，未配置邮件返回 503；客户端应保留登录返回入口和明确的不可用说明。

绑定邮箱必须使用本人 session 和当前密码，更换还需先证明原邮箱控制权，再验证新邮箱。完整邮箱只能通过本人 security 接口读取；管理员账号编辑请求不接受邮箱或现有密码修改。新增账号仍采用临时密码首次改密，随后应立即绑定邮箱。

管理员旧重置入口按账号逐步退出：已验证邮箱的账号立即禁止管理员代重置；全部启用账号完成绑定后，在账号状态持久化 `legacy_password_reset_disabled=true` 并永久关闭旧入口。之后新增未绑定账号或 SMTP 暂时不可用不会自动恢复旧入口。未绑定账号不具备邮件恢复能力，不应跳过迁移与现有账号恢复安排。

身份枚举为 `admin` / `operator`；权限映射的 key 为 `data.modules[].id`，value 为 `none` / `read` / `write`，例如 `{"tracks":"read","scenic_spots":"write"}`。不接受账号管理模块授权给操作员。新增接口默认按页面模块授权，未知模块拒绝。

账号修改保留操作者未保存的表单；409 时应重新读取账号并核对改动。账号停用保留历史身份与操作记录，不提供物理删除。

## 操作记录

写操作在执行前持久化发起记录，结束后保存 HTTP 结果，记录稳定账号 ID、登录名、方法、对象路径和 UTC 时间，不保存请求正文、密码或哈希。日志存储不可用时拒绝发起写操作。后置结果写入失败会保留发起记录并记录 server log；不能据此承诺业务写入与日志之间的跨存储事务。记录状态 0 表示已发起但结果待确认，其他值为实际 HTTP 状态。

账号筛选在数据库查询中先应用，再按时间和 ID 倒序取最近 100 条，不能只过滤全局最近 100 条的浏览器缓存。切换账号立即加载，刷新保持当前筛选；快速切换时忽略旧请求响应，无记录时显示空状态。

账号页按“时间、操作人、操作内容、操作对象、结果”展示，例如“修改轨迹资料 / 轨迹 NO.…”、“重置后台账号密码 / 后台账号 operator”。`static/audit_log.js` 根据已有方法/路径生成中文说明，适用于旧记录，不增加查询或变更日志存储。原始请求方法、路径、HTTP 状态、记录编号及行为说明收在“说明与请求详情”中。最近 100 条原始日志中，同一操作的发起与结果合并显示，因此页面操作行数可能少于 100。

状态 0 显示“结果待确认”，表示有发起记录但未取得最终响应，不能当作失败；2xx 显示“成功”，仅表示请求成功返回；3xx 显示“已跳转，结果待确认”；4xx/5xx 显示“请求失败”并提供基于状态码的常见原因说明，不冒充服务端实际错误正文。批量任务的逐项结果应到业务页面查看。景点供应商资料同步注明可由打开页面自动分批触发，连续记录不一定是人工重复点击。

现有日志未保存字段差异、请求/响应正文，无法还原新旧名称、具体启停、审核通过/驳回或回复内容；描述只说明请求的业务类型，不推测实际变化。未知动作保留通用描述与原始详情。普通浏览、登录和退出目前不属于这份受保护操作日志的记录范围。

本人邮箱操作复用受保护写操作审计；公开邮箱找回完成单独审计，actor_id=email-recovery，username 表示找回目标，页面操作人显示“邮箱找回（未登录）”，失败请求不能冒充账号本人行为。按账号筛选时也会包含针对该账号的找回完成请求。日志不可用时拒绝改密码，日志不包含验证码、密码或邮件正文。

## 验证范围

遵循项目约定，默认只进行 `go build ./...`、前端 `node --check` 和差异自查；不自动运行测试、启动数据库或操作生产。功能和迁移需在部署后由用户手动验收。
