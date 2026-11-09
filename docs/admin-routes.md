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
| GET | `/admin/ai_usage.html` | AI 使用查看 | AI 统计、任务、控制与价格估算页面 |
| GET | `/admin/api/ai/tasks` | AI 使用查看 | 任务筛选/实时列表与游标分页 |
| GET | `/admin/api/ai/statistics` | AI 使用查看 | Token/费用/成功率及功能/模型/日期分组 |
| GET | `/admin/api/ai/quotas` | AI 使用查看 | 独立实时配额快照（今日已用、剩余、并发槽位） |
| PUT | `/admin/api/ai/policies/:feature/quota` | AI 配置操作 + AI 查看 | 只调整每日配额，保留计数与其他配置 |
| GET | `/admin/api/ai/settings` | AI 使用查看 | 持久化状态、功能策略、模型价格和今天用量 |
| GET | `/admin/api/ai/tasks/:task_id` | AI 使用查看 | 任务及请求明细、业务结果和重试关联 |
| POST | `/admin/api/ai/tasks/:task_id/cancel` | AI 任务操作 + AI 查看 | 登记中断，保存阶段拒绝；消耗保留 |
| POST | `/admin/api/ai/tasks/:task_id/retry` | AI 任务操作 + AI 查看 + 源业务操作 | 重读当前资料并创建关联重试任务 |
| PUT | `/admin/api/ai/policies/:feature` | AI 配置操作 + AI 查看 | CAS保存自动暂停、请求预算（0不限）/并发和提醒阈值 |
| PUT | `/admin/api/ai/prices` | AI 配置操作 + AI 查看 | CAS保存仅对未来请求生效的估算价格 |
| POST | `/admin/api/login` | 公开 | bcrypt 校验，成功后下发 `admin_session` cookie |
| POST | `/admin/api/logout` | 公开 | 删除 session + 清 cookie |
| GET | `/admin/api/me` | 公开 | 返回当前会话；未登录返回 401 |
| GET | `/admin/api/releases` | 鉴权 | 列出发布记录，支持 `platform` / `status` 过滤 |
| GET | `/admin/api/dashboard` | 鉴权 | 业务库即时统计；`period=day|week|month`，返回同进度上周期对比、用户转化、审核/反馈效率、内容质量、互动、同行与日趋势 |
| GET | `/admin/api/users/statistics` | 用户查看 | 用户总数、性别/星座/年龄分布、`track_output` 云端轨迹产出分层；`user_cities` 返回规范化 `region_id/name/country_code/province_id/province_name/province_map_code/count`，按目录合并新旧城市编码，每人只计入一个城市，未知编码归入未知；见 [地域统计](api/admin-regions.md#用户地域统计) |
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
| GET | `/admin/api/users` | 用户查看 | 用户列表，支持 `title` 昵称模糊搜索、`is_internal` / `province_code` / `city_code` / `gender` / `level` / `restricted` 组合筛选及 cursor 翻页；旧 `zodiac_sign` 参数兼容。等级按现有 XP 规则计算后在 total/分页前筛选，返回权威 `level_options`；当前页批量补 `last_login_at`、`level`、`follower_count`，行不返回星座/限制详情。见 [列表协议](api/admin-users.md#列表) |
| GET | `/admin/api/users/:user_id` | 用户查看 | 用户资料、生日派生星座、只读手机号/所在地及受保护背景图入口；见 [资料协议](api/admin-users.md) |
| PUT | `/admin/api/users/:user_id` | 用户操作 | 编辑昵称、签名、性别、生日与 `is_internal`；显式提交手机号返回400 |
| GET | `/admin/api/users/:user_id/overview` | 用户查看 | 独立读取关注数、粉丝数、获赞数与完整级别，使用原有规则，不结算奖励 |
| GET | `/admin/api/users/:user_id/background` | 用户查看 | 代理读取该用户已设置的背景图，不下发原始 OSS URL，不允许指定任意图片源 |
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
| POST | `/admin/api/regions/:node_id/photos` | 地区操作 | 上传已保存省份/城市的图片文件，返回图片元数据；前端自动PUT保存关联，限制见 [地区目录](api/regions.md) |
| PUT | `/admin/api/regions/:node_id/photos` | 地区操作 | `{base_photos,photos}` 独立保存图片数组，照片基线冲突返回409，不保存文字；返回 `{node,version}` |
| POST | `/admin/api/regions/photos/pixabay/search` | 地区操作 | 人工查询3张Pixabay候选，24小时缓存，不自动导入；需服务端配置 `PIXABAY_API_KEY` |
| POST | `/admin/api/regions/:node_id/photos/pixabay/import` | 地区操作 | 按缓存候选ID下载API原始地址，等比缩至960×720以内保存到本站，返回Photo；前端自动PUT保存关联 |
| POST | `/admin/api/scenic-spots/:spot_id/photos` | 景点操作 | 图片上传；可选 `draft=1` 仅暂存并返回图片，不更新景点版本；省略时保持追加图片并保存 |
| PUT | `/admin/api/scenic-spots/:spot_id/photos` | 景点操作 | `{expected_version,photos}` 独立保存图片数组，版本CAS，返回景点；高德外链先下载压缩为本站WebP，不重新查询POI或保存未提交文字 |
| GET | `/admin/api/tracks/:track_id` | 鉴权 | 返回 `{track, user_name, user_id_text, geo?}`：轨迹详情、只读归属用户名、主景点与 GCJ02 简化线；`user_id_text` 保留字符串兼容，新表单不展示或提交 ID/用户名，服务端拒绝变更归属人 |
| GET | `/admin/api/tracks/:track_id/assets/:asset` | 轨迹查看 | 只读图片与原始文件资源，raw 按附件下载；asset 白名单与错误码见 [资源协议](api/admin-tracks.md#详情资源展示与下载) |
| POST | `/admin/api/tracks/:track_id/description/generate` | 轨迹操作 | 单条 AI 联网生成候选说明、来源及 source_version，不写轨迹，见 [协议](api/admin-tracks.md#路线说明与-ai-联网填写) |
| PUT | `/admin/api/tracks/:track_id/description` | 轨迹操作 | 一键代入并保存候选说明，版本核对与CAS，仅写说明及人工来源，不提交其他草稿；见 [协议](api/admin-tracks.md#路线说明与-ai-联网填写) |
| POST | `/admin/api/tracks/:track_id/place-refresh` | 轨迹操作 | 同步触发单条轨迹重新景点识别；任务忙 409，无索引 404 |
| POST | `/admin/api/tracks/:track_id/scenic-spot` | 轨迹及景点操作 | 地图 POI 设置主景点，复用/创建目录并比较旧关联 ID；仅更新景点关系 |
| GET | `/admin/api/tracks/map-config` | 轨迹查看 | 轨迹详情地图的高德 Web Key 配置（service_host 指向 `/admin/api/tracks/amap-service`） |
| GET | `/admin/api/tracks/amap-service/*filepath` | 轨迹查看 | 高德 JS API 服务代理，捕获并转发 POI 查询等子路径（与景点地图同一实现） |
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
| GET | `/admin/api/track-submissions` | 鉴权 | 投稿审核列表，支持状态、难度、风险、运动类型、用户过滤；附带中文选项及内置类型图标/颜色元数据，列表隐藏投稿编号 |
| GET | `/admin/api/track-submissions/:submission_id` | 鉴权 | 投稿、轨迹、审核流水及展示元数据；所有状态由当前页详情弹窗展示，见 [投稿接口](api/track-submission.md#7-管理后台审核) |
| POST | `/admin/api/track-submissions/:submission_id/review` | 鉴权 | 按 expected_revision 通过或驳回投稿 |

### 图片文件信息

省市/景点本站图片资源可传 `?photo_info=1`，返回 `{code:0,data:{width,height,size_bytes}}`。沿用原静态资源会话和模块查看权限、路径校验及no-store，仅允许`regions`/`scenic_spots`下的JPG/PNG/WebP；只读本地文件头和实际文件长度，不访问第三方URL。缺文件404，信息不可用不会阻止图片区其他操作。普通资源请求仍返回图片字节。

高德景点图片在写入及存量同步时下载成本站WebP，横图宽1024、竖图高720，质量75；文件完成后才保存关联，图片信息读取压缩后的实际文件。`POST /admin/api/scenic-spots/provider-content/backfill` 增加 `photos_cached`、`photos_failed` 图片计数，已同步文字的记录仍检查外链图片，不在GET请求中迁移。详情见[高德景点图片本地存储](track-place-discovery.md#高德景点图片本地存储)。

### 国家删除

`DELETE /admin/api/regions/:node_id` 使用城市管理操作权限、同源校验与先审计后写，JSON为`{expected_version}`。仅非中国国家根节点可删除，实际将整棵国家子树归档并返回完整目录，保持历史编码与资源；中国和单个省市拒绝。目录版本不符409，详情见[地区后台管理](api/regions.md#后台管理)。

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

AI 使用完整协议见 [admin-ai](api/admin-ai.md)，运行与计费边界见 [AI 使用运行说明](ai-usage.md)。

### 景点列表摘要

`GET /admin/api/scenic-spots?view=summary`沿用景点查看权限，返回目录所需的编号、名称、等级、省市展示名、图片数、人工/自动类型及关联轨迹数，不返回分类、简介、照片URL或来源明细。不传view仍返回原完整条目；查看详情继续用既有单条接口。分页和筛选规则及字段见[省市筛选协议](api/admin-regions.md#景点目录轻量摘要)。
