# 后台实时通信

页面 `/admin/realtime.html`，API 前缀 `/admin/api/realtime`，使用独立后台 session 和 `realtime` 模块查看权限，响应 `no-store`。新模块不授予已有操作员，管理员可在账号管理中配置查看权限；沿用现有首页优先级。第一版仅查询，不提供踢连接、发消息或重试任务操作。

| 方法 | 路径 | 参数与响应 |
| --- | --- | --- |
| GET | `/summary` | `{runtime,backlog,at}`；业务数据库统计独立于 EMQX 管理 API |
| GET | `/network` | `module=messages|companions`；在线用户的私信关系 / 按房间归组的在线成员；companions 另需同行查看权限 |
| GET | `/network/room` | `session_id`；当前采样在线房间的成员、位置快照和最近30条弹幕，另需同行查看权限 |
| GET | `/clients` | `module=messages|companions`、`state=connected|disconnected`、`q`、`page`；每页30 |
| GET | `/clients/detail` | `client_id` 精确匹配 Broker 采样连接；消息连接返回联系人与账号全部私信首屏；同行返回最近30条弹幕，另需 `companions` 查看权限 |
| GET | `/clients/conversations` | `client_id`、`cursor?`；按最近消息时间倒序分页对话，每页30 |
| GET | `/clients/messages` | `client_id`、`peer_id?`、`cursor?`、`after?` / `after_seq?`；默认账号全部私信，可按联系人筛选，每页30 |
| GET | `/events` | `module`、`q`、`page`；每页30，最新异常优先 |

成功返回 `{code:0,data:...}`；非法筛选400，未登录401，无权限403，依赖不可用503，内部错误不返回原始上游错误或凭证。用户ID、异常ID为字符串。q 最多200字节，用于用户ID精确匹配或客户端/同行房间ID包含匹配，不按昵称查询。

## 统计口径

runtime 为当前服务进程内的观测：started_at、publisher_configured、publisher_connected、last_connected_at、last_success_at、last_heartbeat_at、last_failure_at、publish_success、publish_failure、auth_denied、acl_denied、connection_failure、last_latency_ms。计数自进程启动累计，最近200条异常驻内存，重启重置，不宣称覆盖历史或所有 Broker 事件。发布成功为 QoS1 Broker 确认，不代表客户端收取或用户已读。

鉴权/权限拒绝只覆盖到达现有服务端回调的拒绝；user_id 为请求申报身份，未经验证，不作为已登录身份。异常不存储密码、完整 username、签名、API Key、正文、坐标或原始 topic，也不转发可能带敏感内容的上游错误。客户端断连原因历史暂未接入，连接表只显示 Broker 当前保留的连接/会话与时间。

backlog 为 `{status,pending,journals,oldest_record_at?}`。pending 按现有投递 worker 的待处理种类和到期范围计数，journals 单独统计待补齐的 thread 日志，不能与未读消息数混用。oldest_record_at 沿用 message_records 的排序时间，通知使用发布时间，其他记录可能复用创建时间，因此仅作参考，不是实际入队时间或精确积压时长。消息持久化不可用时 status=unavailable，pending/journals 为 null，页面显示未知；不回退易失消息存储，也不为了查看而触发补发。

## 实际连接与缓存

clients 为 `{status,message,sampled_at,complete,online,messages_online,companions_online,total,page,has_more,items}`。只呈现能解析为消息/同行 principal 且客户端前缀匹配的业务连接，排除发布器和其他 Broker 客户端；有效凭证数量不当作在线数量。连接昵称只对当前页批量读取，不改变用户/房间状态。

查询 EMQX v5 `/clients` 时，每页100，最多50页、总预算4秒，管理客户端禁止跟随重定向、限制单页4MiB，成功采样缓存5秒。连接列表对业务、状态、用户/客户端/房间筛选后再分页、统计；顶部 online 分类计数始终覆盖全部已采样业务连接，不受列表筛选影响。达到上限时 complete=false，计数仅代表已采样值，页面标注“至少”；非原子 Broker 分页下连接可能在采样期间变化。

status 为 ok / partial / stale / unavailable / unconfigured。未配置或从未获取数据时 sampled_at 为空，online/分类计数/total 为 null，页面不伪造0；查询失败时保留上次成功采样并标注 stale，不能当作当前精确在线数。时间仅解析 RFC3339（带时区）或 Unix 毫秒，无时区文本不猜时区；后台使用 AdminTime 显示北京时间。

## 在线关系图

`/admin/realtime.html` 的主区为可拖动、缩放和搜索的关系图，消息提醒和与友同行独立切换；原连接列表、筛选分页与近期异常收进“连接诊断与近期异常”。原统计与诊断请求不等待关系图。前端使用本地 `realtime_graph.js` 的原生 SVG，无 CDN 或第二套图表依赖；未变化的节点/连线复用现有元素、保留布局及选中状态，头像只用已有本站缓存或默认头像。

`GET /network` 返回 `{module,status,message,sampled_at,complete,limited?,total_users,total_rooms,nodes,edges}`。未采样时总数为 null，不伪造0；状态和时间口径继承 Broker 快照。采样失败沿用旧快照，页面明确显示上次采样；网络查询失败保留上一张图。节点/连线只基于样本，不把凭证数量或 presence 声明当作实际 Broker 在线连接。正常情况15秒刷新；下线移除以随后一次成功采样为准，不宣称即时断连推送。

- **消息提醒**：只取 `connected=true,module=messages` 的连接；同一用户按 user_id 合并为一个节点，节点ID为字符串用户ID，connections 为该用户采样在线连接数，client_id 只作为诊断/详情查询的当前代表连接。用户最后一条连接断开后，从实时图移除节点与相关边；重连换 client_id 不改变节点身份。边来自双方均在本图用户集合中的 active thread 且 seq>0，表示双方已有私信往来；同时在线不自动连线，也不把每条消息画成一条边。边ID为稳定会话ID，不代表设备直连或特定客户端已收消息。
- **与友同行**：按 Broker principal 的 session_id 分组，每个房间是中心节点 `room:<session>`；用户节点为 `member:<session>:<user>`，同一用户在不同房间不会把房间串起来。连接线表示采样连接所属房间，图中只绘制在线连接成员；成员业务资格以房间详情的 member_status 为准。房间名称按当前图的 session_id 集合批量查询，查不到名称显示“同行房间”，可点击查看完整房间ID。
- nodes 字段为 `{id,kind,user_id?,label,avatar_url?,connections,client_id?,session_id?}`。edges 为 `{id,source,target,kind,last_at?}`，kind 为 direct / membership；last_at 为会话最近记录时间。全部身份/连接/房间ID保持字符串。

每图最多200个用户/成员节点、50个房间中心节点和400条私信关系，超过上限明确标注局部展示，不能把局部图当作全站完整关系；搜索只定位当前已展示的昵称/用户ID/房间ID。所有符合采样条件的用户/房间总数在截断前去重计算；图内的用户资料按页批量读取，私信关系在仓储通过 Participants 同时约束双方后再取401条判断上限，禁止逐个用户读取会话。房间名称通过 AdminCompanionListFilter.SessionIDs 在仓储过滤和总数/分页前限定，nil 保持原有不限制语义，空集合返回空；MySQL/Mongo/memory 三实现同步，无新增仓储方法或表结构。

点击私信连线在右侧显示双方的只读存储记录，初始传 peer_id 并锁定对象，不展示其他联系人的过滤控件；保留最新定位、历史分页和增量刷新。右侧打开期间若代表连接变化但用户仍在线，更新代表 client_id；所选节点/边消失时关闭并取消详情请求。点击用户可查看身份、在线连接数及独立连接诊断。

点击同行房间/连线/成员在右侧打开房间详情。`GET /network/room` 先检验当前采样中该房间至少一条在线同行连接，再查询房间及成员；有效完整样本中房间已不在线返回409，未采样/部分采样/旧样本无法确认时返回503。成功响应 `{room,members,positions,messages,people,sampled_at,status}`：room 仅含 session_id/title/owner_id/status/started_at，成员仅含 user_id/role/member_status/connections/last_seen_at；positions 按用户字符串ID索引，包含坐标、明确坐标系和 recorded_at，表示位置快照，不宣称实时定位。messages 为最近30条存储弹幕，people 为批量读取的头像昵称；不返回 MQTT principal、凭证或房间加入 token。复用既有同行仓储，底层能力不可用明确返回503。

图及其详情使用 realtime 查看权限，同行图与房间详情额外要求 companions 查看权限；服务端 permissions.go 和 handler 均校验，不能只隐藏按钮。每次业务读取预算3秒，前端请求10秒超时；关闭/切换/页面隐藏取消工作，失败保留已加载内容，不修改已读、投递、房间或用户状态。

## 连接详情与消息内容

顶部保留发布连接、在线连接、待补发提醒、发布失败四项，其余运行信息折叠。连接列表、近期异常为两个独立区块，各自筛选与分页。详情先展示基础连接信息，再独立加载正文。

消息连接对应整个账号，默认展示“账号私信消息流”，同时包含与不同联系人的收发记录。桌面左侧为连接信息、“全部私信”与联系人筛选，右侧为统一时间流；每条记录明确显示发送方 → 接收方的头像、昵称、字符串用户ID、发出/收到标签和北京时间，不用单段对话的左右气泡暗示连接归属。点击联系人只是筛选该账号与此人的往来，可随时返回全部；自动刷新不会切换筛选对象。

默认打开账号最新30条并定位底部；向上翻阅、在顶部向下拉或点击历史按钮加载更早消息，补到顶部时保留阅读锚点。自动刷新只追加增量，阅读历史时保留位置并显示“回到最新消息”，不替换无变化的记录行。窄屏改为上下布局，联系人横向排列。

所有详情接口先根据 `client_id` 精确匹配 Broker 采样，再确定连接用户；不接受浏览器指定连接所属用户或房间。client_id 必填、最多200字节。完整新采样中已无连接返回404；未接入、未采样或部分采样中找不到返回503。消息分页接口仅接受 messages 连接，同行连接访问分页接口返回400。每次请求重查后台 session 的 realtime 查看权限，响应 no-store；所有用户ID、消息ID和序号均为字符串。

### 消息连接响应与分页

三个消息接口均包含 `{client,status,sampled_at,content_kind:"direct",people}`。`people` 按字符串用户ID索引，值仅含 `{id,nickname,avatar_url}`；当前响应涉及的用户按页去重并批量读取。头像只使用已存在的本站缓存或默认头像，不触发远程补齐、不返回原始 OSS URL；通过 `/admin/api/static/avatars/*` 或 `default_avatars/*` 的 realtime 查看权限读取，缺失或加载失败回退占位头像。

- `/clients/detail` 额外返回 `conversations`、`peer_id`、`messages`。conversations 是联系人首页；默认 peer_id 为空、messages 为账号全部私信首屏。关系图点击连线时可传 peer_id，返回该用户对所选联系人的首屏消息。
- `/clients/conversations?client_id=...&cursor=...` 额外返回 `conversations:{items,has_more,next_cursor?}`。每项 `{peer_id,content,created_at}`，content 是最多80字的最新消息摘要。沿用 thread 的 `(created_at,id)` 严格游标，筛选后取31条并返回30条，包含该账号仍在业务库中的对话记录。
- `/clients/messages?client_id=...` 默认返回该账号所有联系人的 active 存储私信：`peer_id` 为空，`messages:{items,has_more,next_cursor?,latest_cursor}`。首次按 `(created_at,id)` 倒序取最近30条，再正序返回。items 每项 `{id,seq,sender_id,receiver_id,content,track_id?,created_at}`；seq 仅为所属会话内序号，不能跨联系人排序。所有返回消息的双方均纳入本页用户资料批量查询。
- 全部私信的历史请求传不透明 `cursor`，严格查询更早的 `(created_at,id)`；增量请求传 `after=latest_cursor`，严格按更大的复合边界正序读取，避免同微秒不同联系人记录被时间单字段筛掉。空账号的 latest_cursor 为 `origin`，后续增量从最早记录开始。MySQL、Mongo、memory 的 MessageFilter.AfterID 保持一致，未传 AfterID 的既有 After 语义不变；没有表结构改动。不能同时传历史与增量游标。
- 只有指定 `peer_id` 时才筛选该账号与某联系人的会话，验证 thread 的双方归属，找不到返回404；此模式按会话 seq 正序返回，增量边界为 `latest_seq`。
- 联系人筛选的历史请求传 `cursor=seq:<最早序号>`，严格查询更早的30条；has_more 和 next_cursor 表示是否还有更早记录。非法、非正数或溢出游标返回400。
- 联系人筛选的增量请求传 `after_seq=<最新已加载序号>`，从该序号之后按 seq 正序返回最早30条；has_more 表示本批后仍有新记录，latest_seq 是本批已返回的最大序号。客户端连续追赶最多5批，更多内容留到后续刷新；不会直接跳到最新30条而遗漏中间消息。after_seq 须为非负 int64 文本，不能同时传历史游标；全部私信模式拒绝 after_seq，联系人筛选模式拒绝 after，避免混用游标。全部私信和联系人筛选两种模式均最多连续追赶5批。

不调用 App 的 ListConversations/ListDirect，因为这些接口可能补齐发送日志或读取用户已读状态；后台始终通过独立只读投影查询，不刷新 pending 日志、不清除历史、不修改已读、投递或房间状态。当前消息库不可用返回503，不回退内存。每次业务查询预算3秒，复用三种仓储筛选与排序接口，新增向后复合边界字段 AfterID，不新增仓储方法，也不逐条补用户详情。

### 同行内容与观测范围

同行详情保持 `{client,status,sampled_at,content_kind:"danmaku",items}`，按时间倒序返回房间最近30条弹幕，items 为 `{id,sender_id,content,created_at}`。房间内容沿用同行详情边界，额外校验 companions 查看权限；缺失返回403。

展示内容是账号/房间维度的业务存储记录，可能早于当前连接建立时间，也可能包含用户在客户端清除显示边界前的存储记录，不能证明某个客户端实际收发、Broker 排队内容或用户已读。MQTT 消息提醒仅携带刷新事件和记录标识，正文不经过 MQTT；后台不抓包、不新增 payload 日志。定位、presence 和控制事件不在聊天面板中。

## 配置

在服务端环境中设置以下可选值，经 config.Config 读取后注入，不从浏览器传入：

```dotenv
EMQX_MANAGEMENT_URL=http://127.0.0.1:18083/api/v5
EMQX_MANAGEMENT_API_KEY=
EMQX_MANAGEMENT_API_SECRET=
```

使用 EMQX 单独创建的后台专用 API Key，优先使用内网或 HTTPS；发行版支持 API Key 角色时选择 viewer。EMQX 5.8 社区版的 Key API 未提供可核验的只读角色，按普通专用 Key 管理，由本服务只调用 GET /clients；应用仅查询并不等于 Broker 原生权限受限。URL 必须能从服务端进程或容器访问，容器内的127.0.0.1并不指向宿主机。不要复用 Dashboard 登录密码、App 临时 MQTT 凭证或 MQTT 发布账号。这些值不得提交到 Git，也不会下发到页面。缺任意配置时保留页面、进程观测和持久化统计，实际连接区域显示未接入；HTTP 401/403 标注管理接口鉴权失败，不影响原有 MQTT 收发。

对接依据：[EMQX REST API 鉴权与 v5 路径](https://docs.emqx.com/en/emqx/latest/guides/api.html)、[客户端查询字段](https://docs.emqx.com/en/cloud/latest/api/clients_v5.html)。5.8.6 的角色字段仅企业版提供，见 [对应版本 API 源码](https://github.com/emqx/emqx/blob/v5.8.6/apps/emqx_management/src/emqx_mgmt_api_api_keys.erl)。当前实现面向自托管 EMQX v5 的 page/limit 分页接口，不自动修改 Broker 规则或申请管理凭证。

## 入口与验证

观测在 `internal/service/realtime_monitor.go`，Broker 只读适配在 `realtime_broker.go`，消息待处理统计在 `realtime_backlog.go`、只读联系人/账号私信流/头像投影在 `realtime_chat.go`；注入于 `cmd/server/main.go`，API 为 `internal/admin/realtime.go`、`realtime_detail.go`、`realtime_chat.go`，页面复用现有 Tabler、shell/ui、AdminTime 及嵌入资源版本缓存。

页面每15秒独立刷新统计、两个列表和已打开的详情内容，可关闭自动刷新；详情另提供“刷新消息”。隐藏/离开/关闭详情取消对应请求，10秒请求期限，快筛忽略旧响应，失败保留列表和可恢复分页，相同内容不替换现有行。默认仅 Go 构建、JS语法和差量检查；实际 Broker、权限（包括同行内容的额外权限）、筛选分页、断线/恢复、全部联系人混合收发、联系人分页与筛选切换、双方头像/昵称、历史锚点、增量追赶、收发方向/空态/失败重试、采样上限和窄屏视觉由用户人工验收。
