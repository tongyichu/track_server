# 后台实时通信

页面 `/admin/realtime.html`，API 前缀 `/admin/api/realtime`，使用独立后台 session 和 `realtime` 模块查看权限，响应 `no-store`。新模块不授予已有操作员，管理员可在账号管理中配置查看权限；沿用现有首页优先级。第一版仅查询，不提供踢连接、发消息或重试任务操作。

| 方法 | 路径 | 参数与响应 |
| --- | --- | --- |
| GET | `/summary` | `{runtime,backlog,at}`；业务数据库统计独立于 EMQX 管理 API |
| GET | `/clients` | `module=messages|companions`、`state=connected|disconnected`、`q`、`page`；每页30 |
| GET | `/clients/detail` | `client_id` 精确匹配 Broker 采样中的连接，返回连接信息与最近30条业务消息；同行内容另需 `companions` 查看权限 |
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

## 连接详情与消息内容

连接列表的“详情”先展示该行的基础信息，再独立加载内容；内容失败不影响已展示的连接信息，可重试。订阅数、排队消息、断开时间等辅助字段收进详情。顶部保留发布连接、在线连接、待补发提醒、发布失败四项，其余进程统计与口径说明放在“更多运行信息”。连接列表与近期异常为两个常驻区块，筛选、分页、错误状态独立。

`GET /clients/detail?client_id=...` 不接受浏览器指定用户或房间：先从 Broker 采样精确找到连接，再使用其 user_id / session_id 读取存储内容。client_id 必填、最多200字节；完整新采样中已无该连接返回404；未接入、未采样或部分采样中找不到时返回503。成功响应为 `{client,status,sampled_at,content_kind,items}`；client 与连接列表字段相同，采样失败沿用已有 stale 标记。items 按时间倒序，元素为 `{id,sender_id,receiver_id?,content,track_id?,created_at}`，所有身份和消息ID均为字符串。

- `content_kind=direct`：在 realtime 查看权限内，只读查询该连接用户参与的最近30条已存储、active 的私信，包含发送与接收、覆盖该账号全部设备。显示正文、收发用户ID及分享轨迹ID，不返回持久化 payload、请求指纹、临时凭证或用户状态。消息库不可用返回503，不回退内存。
- `content_kind=danmaku`：读取该连接所在房间最近30条已存储弹幕，包含其他成员内容；保留同行详情原有边界，额外校验 companions 查看权限，缺失返回403。定位、presence、控制事件和原始 MQTT payload 不在此列表中。

这些是账号/房间维度的业务存储记录，可能早于当前连接建立时间，不能作为此客户端实际收发、Broker 排队内容或用户已读的凭证。MQTT 消息提醒仅携带刷新事件和记录标识，私信正文不经过 MQTT；此功能不抓包、不新增 payload 日志，不修改已读、投递、房间或消息状态。详情内容查询数据库预算3秒，不增加仓储方法或逐条用户查询。

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

观测在 `internal/service/realtime_monitor.go`，Broker 只读适配在 `realtime_broker.go`，消息待处理统计和只读私信投影在 `realtime_backlog.go`；注入于 `cmd/server/main.go`，API 为 `internal/admin/realtime.go`、`realtime_detail.go`，页面复用现有 Tabler、shell/ui、AdminTime 及嵌入资源版本缓存。

页面每15秒独立刷新统计、两个列表和已打开的详情内容，可关闭自动刷新；详情另提供“刷新消息”。隐藏/离开/关闭详情取消对应请求，10秒请求期限，快筛忽略旧响应，失败保留列表和可恢复分页，相同内容不替换现有行。默认仅 Go 构建、JS语法和差量检查；实际 Broker、权限（包括同行内容的额外权限）、筛选分页、断线/恢复、详情收发方向/空态/失败重试、采样上限和窄屏视觉由用户人工验收。
