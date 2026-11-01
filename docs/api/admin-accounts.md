# 后台账号与页面权限

后台独立使用 `/admin` 路由和 `admin_session` cookie，不使用 App 用户 JWT，也不复用业务用户表。

## 身份与迁移

- 管理员拥有全部页面及操作权限，可以管理其他账号。必须保留至少一个启用的管理员。
- 操作员逐页面配置 `none`（无权限）、`read`（只读）、`write`（可操作）。可操作包含该页面的编辑、删除、审核、发布等业务动作，不再拆动作权限。账号管理和操作日志固定仅管理员可用。
- 首次部署保留当前全部 `ADMIN_ACCOUNTS`，以及兼容的 `ADMIN_USERNAME` / `ADMIN_PASSWORD_HASH`；配置必须包含登录名 `admin`。首次导入时仅 `admin` 为管理员，其余现有账号默认所有当前业务页面只读，保留原密码。
- 导入完成后以数据库为唯一权威。重启不会覆盖密码、身份、权限或启用状态；环境变量以后新增的账号也不会被自动导入。新账号应从后台创建，新操作员未分配的页面默认无权限。未来新增页面不会自动授权给已有操作员。
- 新增账号和重置密码使用临时密码，下次登录必须先修改；原有账号迁移不强制修改密码。登录名创建后固定，显示名称可改。密码至少 8 个字符、最多 72 字节，只保存 bcrypt 哈希。

## 存储与失效

`cmd/server/main.go` 注入当前 MySQL/Mongo 的独立 `AccountStorage`，自动创建 `admin_account_state` 与 `admin_audit_logs`；MySQL 基线同步在 `mysql.sql`。账号状态采用单行/单文档 JSON 和整体 `version` CAS，确保跨进程的首次导入、最后管理员保护和权限修改原子生效；每个账号的 `revision` 用于编辑冲突检测。

登录会话仍只存在内存，重启需要重新登录。每个后续受保护请求重新读取账号权限；停用、重置密码或本人改密码都会增加 `security_version`，让所有进程中的旧会话失效。已经开始执行的请求按请求开始时的权限处理。

账号库不可用或状态损坏时拒绝登录与受保护操作，不使用环境变量直接鉴权，不回退到内存账号。`USE_IN_MEMORY` 不提供后台账号服务。无初始账号且数据库未初始化时不挂载后台；初始化后移除初始账号环境变量不会禁用已有账号。

## 授权规则

模块清单为工作台、投稿审核、轨迹管理、景点管理、用户管理、地图模式、与友同行、反馈与纠错、App 发布、埋点同步。前端导航、控件和身份标签由 `static/shell.js` 统一维护，服务端 `permissions.go` 是实际授权权威。

- 无权限：菜单隐藏，直接请求 HTML/API 也拒绝。
- 只读：允许列表、详情和查询，禁止写请求；获取发布上传凭证虽为 GET，也要求可操作。
- 可操作：允许页面全部业务操作。城市选项和共享头像按实际使用模块放行，业务截图按所属模块校验。
- 景点页的相关轨迹列表也要求轨迹查看权限；轨迹编辑/删除仍检查轨迹可操作权限。景点轨迹归属写接口同时检查景点和轨迹的可操作权限。
- 工作台响应按各业务模块裁剪数据；轨迹、投稿和反馈页面复用 `/admin/api/dashboard?scope=tracks|track_submissions|feedbacks`，只返回自身统计，不能借此读取其他模块。
- 只读景点页不运行供应商资料回填。公共静态资源目录不提供 HTML，所有业务页面通过具名鉴权路由访问。
- 受保护响应不缓存，写请求校验同源。权限不足返回 403，登录失效返回 401，账号库不可用返回 503，编辑版本冲突或移除最后管理员返回 409。

## 页面与接口

页面 `/admin/accounts.html` 维护账号并展示最近操作记录；`/admin/password.html` 修改本人密码；无业务页面授权时 `/admin/access.html` 提示联系管理员。登录响应和 `GET /admin/api/me` 的 `data` 包含 `username`、`display_name`、`role`、`permissions`、`must_change_password`、`expires_at`、`home` 和不含密码哈希的 `account`。登录后按 `home` 跳转。

| 方法 | 路径 | 权限 | 请求/响应 |
| --- | --- | --- | --- |
| GET | `/admin/api/accounts` | 管理员 | `data.items` 账号列表、`data.modules` 页面目录；不返回密码哈希 |
| POST | `/admin/api/accounts` | 管理员 | `username, display_name, role, enabled, permissions, password` |
| PUT | `/admin/api/accounts/:username` | 管理员 | `display_name, role, enabled, permissions, revision`；整体替换页面权限，未包含项默认无权限 |
| POST | `/admin/api/accounts/:username/reset-password` | 管理员 | `password, revision`，使所有旧登录失效并要求改密 |
| POST | `/admin/api/password` | 已登录，包括待改密账号 | `old_password, password`；成功后重新登录 |
| GET | `/admin/api/audit-logs` | 管理员 | 最近 100 条操作发起/HTTP 结果记录 |

身份枚举为 `admin` / `operator`；权限映射的 key 为 `data.modules[].id`，value 为 `none` / `read` / `write`，例如 `{"tracks":"read","scenic_spots":"write"}`。不接受账号管理模块授权给操作员。新增接口默认按页面模块授权，未知模块拒绝。

账号修改保留操作者未保存的表单；409 时应重新读取账号并核对改动。账号停用保留历史身份与操作记录，不提供物理删除。

## 操作记录

写操作在执行前持久化发起记录，结束后保存 HTTP 结果，记录稳定账号 ID、登录名、方法、对象路径和 UTC 时间，不保存请求正文、密码或哈希。日志存储不可用时拒绝发起写操作。后置结果写入失败会保留发起记录并记录 server log；不能据此承诺业务写入与日志之间的跨存储事务。记录状态 0 表示已发起但结果待确认，其他值为实际 HTTP 状态。

## 验证范围

遵循项目约定，默认只进行 `go build ./...`、前端 `node --check` 和差异自查；不自动运行测试、启动数据库或操作生产。功能和迁移需在部署后由用户手动验收。
