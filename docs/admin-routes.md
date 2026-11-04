# 管理后台路由参考

从原 AGENTS.md 迁出的开发参考；修改对应功能前按 AGENTS 索引读取相关章节，无需每次通读。验证、授权和通用行为以当前 AGENTS.md 为准；实现细节以源码和最新接口文档为准。

表中路由迁自原规则；新增或修改接口以 internal/admin/routes.go 与 permissions.go 为权威，并更新相关 API 分册。

## 目录

- [路由清单（与 `routes.go` 对齐）](#路由清单与-routesgo-对齐)
- [后台账号新增路由](#后台账号新增路由)

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
| DELETE | `/admin/api/releases/:id` | 鉴权 | 删除一条发布记录，并尽力删除落在本机 `<LogDir>/static/release/` 的安装包文件；独立留存的 changelog 不受影响 |
| GET | `/admin/api/releases/changelogs` | 鉴权 | 版本更新日志（`platform` 过滤，默认 android），来自 `app_release_changelogs`，发布记录删除后仍留存 |
| POST | `/admin/api/releases/upload-package` | 鉴权 | multipart 上传安装包到 `<LogDir>/static/release/<platform>/` |
| GET | `/admin/api/releases/upload-token` | 鉴权 | 旧 OSS 直传 STS 凭证接口，前端不再使用 |
| GET | `/admin/api/releases/client-statistics` | 鉴权 | 客户端活跃统计：`days=7|30`，版本/机型/系统版本分布、按日版本活跃趋势与最新发布版本渗透概况（含低于强升线设备数） |
| GET | `/admin/api/static/*filepath` | 鉴权 | 后台静态资源代理，从 `<LogDir>/static/` 读取头像等资源；后台页面不要直接访问需业务 JWT 的 `/api/v1/static/*` |
| GET | `/admin/api/users` | 鉴权 | 用户列表，支持 `title` 昵称模糊搜索、`is_internal` / `province_code` / `city_code` / `gender` / `zodiac_sign` / `restricted` 组合筛选及 cursor 翻页；所有条件参与总数统计，返回按当前页批量统计的 `last_login_at` |
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
| GET | `/admin/api/tracks` | 鉴权 | 轨迹列表，支持 `title` / `user_name` 模糊搜索、`province_code`（展开为省内城市集合，`city_code` 优先）/ `city_code` / `status`（1 公开、2 私密、0 已删除，默认排除已删除）/ `missing` 筛选及 cursor 翻页 |
| POST | `/admin/api/tracks/import` | 轨迹操作 | 批量导入轨迹文件归属内部人员，multipart `files`（≤50）+ `owner_user_id` + `track_type` + `coordinate_system`，逐文件返回结果 |
| GET | `/admin/api/tracks/statistics` | 轨迹查看 | period=day/week/month，默认 month；已完成云端轨迹类型分布和贡献用户前十，按北京时间入库时间统计 |
| GET | `/admin/api/cities` | 鉴权 | 返回内置城市/省份 Code、名称与省市层级，供轨迹和景点选择器使用 |
| GET | `/admin/api/tracks/:track_id` | 鉴权 | 返回 `{track, user_id_text, geo?}`：轨迹详情、主景点（track.primary_scenic_spot）与地图索引简化轨迹线（GCJ02）；`user_id_text` 是字符串形式的归属人 ID，编辑表单只读显示该字符串且保存不提交 user_id，服务端拒绝变更归属人，避免 int64 经 JS Number 精度丢失 |
| POST | `/admin/api/tracks/:track_id/place-refresh` | 轨迹操作 | 同步触发单条轨迹重新景点识别；任务忙 409，无索引 404 |
| GET | `/admin/api/tracks/map-config` | 轨迹查看 | 轨迹详情地图的高德 Web Key 配置（service_host 指向 `/admin/api/tracks/amap-service`） |
| GET | `/admin/api/tracks/amap-service` | 轨迹查看 | 高德 JS API 服务代理（与景点地图同一实现） |
| PUT | `/admin/api/tracks/:track_id` | 鉴权 | 更新轨迹业务字段（不改 ID、轨迹点与创建/更新时间）；城市/类型更新已有地图索引，状态/进行状态/原始文件变化时清理并按可索引条件重新排队 |
| DELETE | `/admin/api/tracks/:track_id` | 鉴权 | 删除轨迹（页面文案为下架）：标记 `status=0`，并清理收藏、地图索引任务、geo index、路线组成员 |
| POST | `/admin/api/tracks/:track_id/restore` | 鉴权 | 恢复已下架轨迹：还原为公开、清空 `deleted_at` 并重新排队地图索引；收藏与路线组成员关系不可恢复。详情弹窗对已下架轨迹把删除按钮换成恢复 |
| POST | `/admin/api/tracks/:track_id/purge` | 鉴权 | 彻底删除已下架轨迹（仅限 `status=0`，不可恢复）：事务物理删除主表与全部关联数据（轨迹点/收藏/导航/索引/路线组成员/景点关联与识别记录/点赞/评论全套/投稿全套），成功后尽力删 OSS 原始文件与预览图（`OSSTokenService.DeleteUserObjects`，仅限用户内容前缀、按精确 key 授权，失败返回 `oss_warning` 不阻断）。详情弹窗对已下架轨迹主按钮为「彻底删除」，前端双重确认 |
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
