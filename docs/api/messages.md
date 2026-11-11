# 消息中心与私信协议

所有 App 路由前缀 `/api/v1`，使用业务 JWT，响应 `Cache-Control: no-store`；成功沿用 `{code:0,data:...}`。用户 ID 一律十进制字符串，序号/计数为整数，时间为 RFC3339 UTC。后台使用独立 admin session、同源校验、模块权限和审计。

## 目录

- [摘要与已读](#摘要与已读)
- [通知与互动](#通知与互动)
- [私信](#私信)
- [实时连接](#实时连接)
- [活动通知管理](#活动通知管理)
- [错误与存储](#错误与存储)

## 摘要与已读

`GET /messages/summary` 返回 `system/private/comments/reactions/interactions/total` 未读条数、`cutoff`、可选 `system_latest`（id/title/publish_at）和 `private_latest`（会话摘要）。`total=system+private+interactions`；comments/reactions 是 interactions 的子集，不能重复相加。

`POST /messages/read-all` 请求 `{"cutoff":"<摘要的cutoff>"}`，仅清除该时间及之前的未读，较晚新消息保留。服务端拒绝未来时间；游标单调不回退。清未读不删除历史。

所有分页列表返回 `items/has_more/next_cursor`，默认每页30，`limit` 支持1～100。普通列表的 cursor 是不透明 keyset，不能解析成页码。客户端实际看见条目或点击后才提交已读，进入列表不一次性清空所有分页未读。

## 通知与互动

| 方法 | 路径 | 参数或请求 |
| --- | --- | --- |
| GET | `/messages/notices` | `cursor/limit` |
| POST | `/messages/notices/read` | `{ids:["N.…"]}`，最多100个 |
| GET | `/messages/interactions` | `category=all|comments|reactions`、`cursor/limit` |
| POST | `/messages/interactions/read` | `{ids:["E.…"]}`，最多100个，只能标记本人事件 |

通知公开字段为 `id/kind/title/body/h5_url/publish_at/expires_at/unread`；`kind=system|activity`。不向 App 返回指定接收人 ID、管理员、草稿状态等后台字段。只有已发布、到时间且本人属于接收范围的通知可见；撤回即隐藏。注册前通知可在历史中查看，但不产生未读；编辑已发布内容不重新置未读。结束时间是活动提示，过期通知仍保留展示，客户端禁用 H5 跳转。

互动字段为 `id/kind/actor_id/actor/track_id/comment_id/text/created_at/unread/track`。actor 为 `id/nickname/avatar_url`；track 为 `id/title/preview_url/available`。失效或不可访问轨迹返回 available=false，不能借通知读取轨迹详情。全部包含 comment/reply/like/collect/comment_like/follow；评论包含 comment/reply；赞和收藏包含 like/collect/comment_like。自己的操作不通知自己；同一关系取消后隐藏，重新建立复用事件而不刷新增未读。此版本仅从上线后的业务操作生成，不追溯全量历史互动。

消息卡片的 `track.preview_url` 与轨迹列表共用带源版本的鉴权预览地址 `/api/v1/track/:track_id/previews/landscape_day?v=...`，不直接返回私有 OSS 原图。客户端补全相对地址并携带业务 JWT；图片读取沿用轨迹可见性、源版本校验和缓存填充。无预览时返回空字符串。该规则覆盖互动通知、私信发送响应和历史消息，历史消息在读取时重新组装，无需重发。

## 私信

| 方法 | 路径 | 参数或请求 |
| --- | --- | --- |
| GET | `/messages/conversations` | `cursor/limit`，最近消息倒序 |
| GET | `/messages/blacklist` | `cursor/limit`，仅本人拉黑记录 |
| GET | `/messages/conversations/:peer_id` | 会话元信息，可用于尚无消息的访客聊天 |
| GET | `/messages/conversations/:peer_id/messages` | `cursor=seq:N` 读取 N 之前的历史；不带游标取最近；`after_seq=N` 取增量 |
| POST | `/messages/conversations/:peer_id/messages` | `{client_id,text,track_id}` |
| GET | `/messages/conversations/:peer_id/messages/by-client/:client_id` | 查询本账号向该对方已提交的消息，用于结果不明恢复 |
| POST | `/messages/conversations/:peer_id/read` | `{through_seq:N}` |
| PUT | `/messages/conversations/:peer_id/block` | `{blocked:true|false}` |

会话字段：`peer/last/unread/blocked/blocked_at/can_send/send_hint/read_seq`。blocked/blocked_at 仅表示本人主动拉黑状态，不泄露对方黑名单详情。last 仅供预览，文字最多120字；完整正文从消息列表读取。read_seq 含全部已读的有效边界。

`peer` 与互动消息 `actor` 的 `avatar_url` 使用同一头像缓存：OSS 原图由服务端复用已有头像下载器同步，命中后返回 `/api/v1/static/avatars/...`，不把未签名的私有 OSS 地址交给客户端；空头像、同步失败或缓存未启用时回退本站默认头像。已有本站静态地址与非 OSS 外部头像保留，用户已注销时沿用空头像。覆盖摘要里的最近私信、会话列表、会话详情和黑名单；客户端统一补全相对地址，不需要逐项查询个人资料。此转换只影响响应，不修改用户资料或消息历史。

消息字段：`id/client_id/thread_id/seq/sender_id/receiver_id/text/track_id/track/created_at`。客户端生成8～80字节唯一 client_id（推荐 UUID），必须先本地落盘再发送；相同双方、发送者、client_id 和内容重复提交返回同一条，内容不同409。同一会话服务端 seq 单调递增，排序/去重以 seq/id 为准，不依赖设备时钟。

文字 trim 后最多1000个字符；文字和轨迹不能同时为空；轨迹须为已公开且已结束。文字加轨迹算一条消息。陌生人发起方在对方回复前最多一条；互相关注免此限制；对方回复后正常聊天。连续同发送者间隔至少1秒；受限账号禁止发送。两人任意一方拉黑即暂停双向发送，拒绝的消息不存储；历史保留，解除后继续原会话，不补发拉黑期间被拒内容。拉黑时间以服务端为准，可查看和解除已注销用户的既有拉黑记录。

消息列表无 after_seq 时返回最近一页（items 仍按 seq 正序）；向上翻页使用 next_cursor。增量 after_seq 可为0，items 正序，has_more 表示后方还有，继续使用本页最大 seq；增量不返回 next_cursor。不得同时混用两类游标。已读 through_seq 限制在已有序号内，并推进到不超过该序号的实际来信；只向前推进。

## 实时连接

`POST /messages/mqtt/credentials` 返回 `broker_url/websocket_url/client_id/username/password/expires_at/topics`。凭证15分钟有效，绑定业务用户、token_version 和 client_id；客户端只可订阅本人 inbox 及通知刷新主题，不可发布或订阅通配符/他人主题。沿用同行 EMQX HTTP auth/ACL 回调，msgv1 principal 单独识别，无需延长同行房间生命周期。

MQTT每20秒有不携带业务内容的 inbox_heartbeat；客户端超过45秒无服务端信号也视为提醒链路降级，恢复模块内12秒补拉。业务刷新使用MQTT QoS1，仅发送 `{event:"inbox_changed",id,kind,revision}`，不携带私信正文或接收范围；到达后 HTTP 拉取权威数据。私信双方设备均可收到刷新提示；读状态和黑名单变化通过本人状态刷新传播。重复/乱序提醒不能直接累加未读或插入消息。

Android 仅在消息模块可见时连接，内部页面切换共享连接；离开/后台500ms后关闭，回到模块先补拉。普通页面只在 App 前台、进入我的、退出模块时更新摘要，不常驻 MQTT、不轮询。MQTT 失败时消息模块内每12秒补拉；MQTT恢复订阅成功立即补拉。第一版没有系统通知栏或离线推送。

## 活动通知管理

页面 `/admin/announcements.html`，模块 `announcements`，读写权限独立。路由：GET/POST `/admin/api/announcements`、GET/PUT `/admin/api/announcements/:notice_id`。列表 cursor/limit 同上；保存请求 `{expected_version:0,notice:{…}}`，创建0、修改传最新 revision。

notice 字段：`kind/title/body/h5_url/status/target_user_ids/publish_at/expires_at`。标题1～100字、正文1～8000字、H5选填且只接受无账号密码的 HTTPS；接收范围为空数组表示全部，指定 ID 最多1000且服务端批量验证存在。操作者、创建/修改时间、ID及revision由服务端写入。

status 为 draft/published/withdrawn。草稿可编辑、预览；published+未来时间为定时发布，否则立即发布。第一次发布过去时间自动归当前时间。已发布内容可编辑，但发布时间与接收范围固定，不能退回草稿；可撤回，撤回不可直接重发，需新建。已有阅读标记不因编辑丢失。并发修改 CAS 冲突409后重开核对；审计沿用后台统一写请求流程。

后台通知列表/预览及发布时间、活动结束时间输入框统一使用北京时间，表单回填与提交显式双向转换；API 保持带时区的 RFC3339，未编辑时间保留原始精度，浏览器时区不影响定时发布时刻。统一约定见[后台时间与时区](../admin-ui.md#时间与时区)。

## 错误与存储

400 参数/陌生人配额/发送频率/拉黑限制，401 未登录，403 越权或账号限制，404 内容不存在，409 幂等内容冲突或并发状态变化，503 消息持久化/实时凭证不可用；其余500。失败不返回内部数据库或凭证错误。

MySQL `message_records`、Mongo 同名集合按版本 CAS 保存，内存实现保持协议便于开发；生产入口仅在持久化初始化成功后启用消息，失败503，不以易失内存确认私信成功。数据库结构见 mysql.sql，实现和恢复边界见 [消息存储与生命周期](../messages.md)。
