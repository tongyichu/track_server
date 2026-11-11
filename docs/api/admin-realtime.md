# 后台实时通信

页面 `/admin/realtime.html`，API 前缀 `/admin/api/realtime`，使用独立后台 session 和 `realtime` 模块查看权限，响应 `no-store`。新模块不授予已有操作员，管理员可在账号管理中配置查看权限；沿用现有首页优先级。第一版仅查询，不提供踢连接、发消息或重试任务操作。

| 方法 | 路径 | 参数与响应 |
| --- | --- | --- |
| GET | `/summary` | `{runtime,backlog,at}`；业务数据库统计独立于 EMQX 管理 API |
| GET | `/clients` | `module=messages|companions`、`state=connected|disconnected`、`q`、`page`；每页30 |
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

## 配置

在服务端环境中设置以下可选值，经 config.Config 读取后注入，不从浏览器传入：

```dotenv
EMQX_MANAGEMENT_URL=http://127.0.0.1:18083/api/v5
EMQX_MANAGEMENT_API_KEY=
EMQX_MANAGEMENT_API_SECRET=
```

使用 EMQX 单独创建的只读管理 API Key，优先使用内网或 HTTPS。URL 必须能从服务端进程或容器访问，容器内的127.0.0.1并不指向宿主机。不要复用 Dashboard 登录密码、App 临时 MQTT 凭证或 MQTT 发布账号。这些值不得提交到 Git，也不会下发到页面。缺任意配置时保留页面、进程观测和持久化统计，实际连接区域显示未接入；HTTP 401/403 标注管理接口鉴权失败，不影响原有 MQTT 收发。

对接依据：[EMQX REST API 鉴权与 v5 路径](https://docs.emqx.com/en/emqx/latest/guides/api.html)、[客户端查询字段](https://docs.emqx.com/en/cloud/latest/api/clients_v5.html)。当前实现面向自托管 EMQX v5 的 page/limit 分页接口，不自动修改 Broker 规则或申请管理凭证。

## 入口与验证

观测在 `internal/service/realtime_monitor.go`，Broker 只读适配在 `realtime_broker.go`，消息待处理统计在 `realtime_backlog.go`；注入于 `cmd/server/main.go`，API 为 `internal/admin/realtime.go`，页面复用现有 Tabler、shell/ui、AdminTime 及嵌入资源版本缓存。

页面每15秒独立刷新统计和当前列表，可关闭自动刷新；隐藏/离开取消请求，10秒请求期限，快筛忽略旧响应，失败保留列表和可恢复分页，相同内容不替换现有行。默认仅 Go 构建、JS语法和差量检查；实际 Broker、权限、筛选分页、断线/恢复、采样上限和窄屏视觉由用户人工验收。
