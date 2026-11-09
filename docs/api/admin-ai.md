# 后台 AI 使用管理

入口：`/admin/ai_usage.html`，独立 `admin_session`。数据来源、队列及计费边界见 [AI 使用运行说明](../ai-usage.md)。

## 权限与响应

- `ai_usage` 读取：页面、任务、统计、配额快照、策略和价格配置。既有普通账号不会自动获得新权限；管理员天然可用。
- `ai_tasks` 操作并同时具备 `ai_usage` 读取：中断、重试。重试还需源业务操作权限（轨迹类 `tracks`，景点类 `scenic_spots`）。
- `ai_settings` 操作并同时具备 `ai_usage` 读取：保存策略及估算价格。
- 写接口复用实时账号校验、同源检查和发起/结果审计；未知路由拒绝授权。GET 只读，不调用供应商、不修改任务状态。
- 成功 `{data: ...}`；错误 `{error: ...}`。400 参数错误，403 无权限，404 不存在，409 冲突，503 管理能力不可用。数据与 HTML 使用 no-store。

## 任务列表与统计

`GET /admin/api/ai/tasks` 与 `GET /admin/api/ai/statistics` 共用筛选：

| 参数 | 语义 |
| --- | --- |
| from / to | 北京时间 YYYY-MM-DD，均包含当天；默认今天，最大 366 天 |
| feature | track_auto / track_description / track_shape / shape_learning / scenic_description |
| provider / model | 实际调用供应商与模型，当前供应商 moonshot |
| mode | auto / manual（后台重试为 manual） |
| status | queued / running / cancelling / success / failed / cancelled / interrupted / paused / deferred |
| adoption | not_generated / pending / applied / cached / not_applied / no_result / stale |
| q | 不区分大小写，匹配任务编号、关联对象编号/名称、操作者；最长200字节 |
| active | true 时只查询未结束任务，忽略日期；前端实时区还主动去掉 status/adoption 参数 |
| cursor / limit | 仅列表分页，按时间顺序编号倒序，默认30、最大100 |

列表返回 `{items,total,next_cursor?}`。total 在 cursor 分页前计算；所有筛选在计数/分页前执行。列表不返回调用明细、输入摘要和生成结果，无逐条关联业务查询。轨迹/景点跳转由前端另查业务权限。

统计返回 tasks/requests/running/queued/success/failed/cancelled/adopted、usage_known/input_tokens/output_tokens、cost_known/estimated_cny、duration_ms/finished，及 `features/models/days` 同口径分组、全局 `first_recorded_at`。模型组 key 为 provider / model。失败包含进程中断；成功率只用 success/(success+failed)，人工取消、暂停和额度等待不充当失败。平均耗时为所有结束任务从创建到结束的时间，含排队。

统计按任务创建日期归属，跨日请求也属于该任务创建日；每日请求预算另按实际发起日计数。用量缺失不能补成0；费用只合计可估算部分，同时显示覆盖请求数。历史未采集数据不可追溯。

`GET /admin/api/ai/tasks/:task_id` 返回完整记录：

- 功能、对象、model/provider、mode/reason/actor、输入和规则版本、摘要、时间、状态、结果去向。
- calls：每次 search / generation / balance 的开始、耗时、HTTP状态、安全错误类别、可空 input_tokens/output_tokens/cached_tokens/estimated_cny 与 price_version / price_snapshot（当时单价快照）。not_sent 不计实际请求数，网络异常计请求但消耗可能未知。
- result：最多32KiB的业务结果 JSON 字符串；不保存隐藏推理、完整提示词、原始GPS、图片/base64、凭证或供应商原始错误。输出展示使用 textContent。
- retry_of / retry_task_id：原任务和新任务关联。自动与手动重试均关联原任务，保留原消耗及错误。

## 中断与重试

`POST /admin/api/ai/tasks/:task_id/cancel`：202 `{cancel_requested:true}` 仅表示已登记。执行端每2秒轮询并取消HTTP context，跨进程也生效；保存前再次校验。进入 saving 后409，不能承诺撤销已开始的数据库写入。供应商已发生的计费不撤回，无法取得的消耗仍未知。

用户中断的同一功能/对象/输入版本被自动巡检抑制，不立即重新付费；输入变化可重新纳管，显式手动重试可绕过抑制。暂停产生 paused，不作为永久人工中断，恢复后继续原巡检。

`POST /admin/api/ai/tasks/:task_id/retry`：仅 failed/cancelled/interrupted，202 `{task_id}`。每个原任务只创建一个直接后继，后继失败可继续重试；本机最多4个后台重试。重新读取当前业务资料，复用现有去重/租约/预算，不重放旧提示词、不自动重试供应商请求。未保存的景点草稿不能恢复重试。自动路线/学习重试复用全局扫描租约；人工说明/图案/景点重试只返回候选，采用仍需用户确认。

## 控制与估算价格

`GET /admin/api/ai/settings` 返回：features、已配置 models、policies（AIControl数组）、prices（PriceBook）、durable、today_usage；legacy_narrative_daily_budget仅为旧页面兼容保留，固定0，旧共享限制已停用。策略读取不重置预算，跨日旧计数由前端按day判断。today_usage 按今天创建任务统计，只用作提示；实时租约只显示未过期项。

`PUT /admin/api/ai/policies/:feature`：

```json
{"feature":"track_auto","revision":0,"pause_automatic":false,"daily_requests":0,"max_concurrent":1,"token_alert":0,"cost_alert_cny":0}
```

成功 `{saved:true}`；revision为所读策略版本，冲突409。每日请求上限0～100000，0表示不限且为默认值，最大并发1～16；Token提醒0～10亿、费用提醒0～10万元，0关闭提醒。本次解限部署会一次性将现有功能日预算迁移为0，之后后台设置的预算在重启时保留；迁移不改变计数或其他策略。操作者和更新时间由服务端写入。暂停仅影响自动任务，执行端将尚未进入保存的任务停止；人工操作和手动重试继续受请求预算/并发限制。减少并发不会中断已有任务，新任务等待可用槽位。自动路线和学习仍遵守原有全局叙述租约，旧共享日额度已移除。

`PUT /admin/api/ai/prices`：

```json
{"revision":0,"items":[{"provider":"moonshot","model":"kimi-k2.6","input_per_million":null,"cached_per_million":null,"output_per_million":null,"search_per_call":null}]}
```

最多30项，模型不可重复，单价0～1000000或null。null未知，0为明确免费；供应商当前moonshot。所有价格以元计，前3项按百万Token、搜索按次；按实际缓存命中拆分输入费用，缓存单价缺失时不能假定缓存免费。仅后续请求采用新版本，不追算历史。每次请求开始时读取价格版本；服务商未返回用量、请求失败/中断的费用不得猜测。余额诊断不伪装成模型生成费用。

## 原业务入口的采用追踪

- 路线说明生成返回新增 `task_id`；独立保存可携带 `{description,source_version,task_id}`，校验任务与对象/输入版本后保存，记为applied。业务保存成功但采用记录失败仅记日志，不误报保存失败。
- 图案生成返回 `task_id`，完整诊断也含task_id；详情保存支持 `shape_ai_task_id`，须同时提交shape_label并匹配当前输入版本。人工编辑后未采用候选不冒记。
- 景点介绍生成返回 `task_id`；创建景点可附 `ai_description_task_id`，更新在 `{expected_version,spot,ai_description_task_id}` 的顶层携带。验证功能/对象/操作者；未保存草稿在创建成功后绑定新景点ID。修改后的已确认候选仍可计为采用，结果日志保留原生成内容。
- 原调用方不携带task_id仍兼容保存，未记录来源的人工输入不推测为AI采用。

## 实时配额与独立调整

`GET /admin/api/ai/quotas`：AI使用读取权限，5秒超时，只查功能策略/计数和有效租约，不依赖历史统计或价格查询。返回 `{day,resets_at,as_of,durable,items}`，日期为北京时间；每项为 `{feature,policy,used,remaining,unlimited,active}`。

- used：当天配额计数，含失败及预留请求；跨日旧计数按0显示，不通过读取重置存储。
- remaining：正数日上限减used，最低0；不限时为null，unlimited=true，不伪造成0次剩余。
- active：尚未到期的功能任务槽位数；policy含当前版本、日配额、并发、暂停与提醒设置。
- 记录库不可用返回503，不把内存降级显示为真实0用量。GET不写数据、不调用AI。

`PUT /admin/api/ai/policies/:feature/quota`：AI配置操作及AI读取权限，同源校验并先审计。请求 `{revision,daily_requests}`，均必填，revision≥0，daily_requests为0～100000，0表示不限；参数错误400，版本冲突409。返回 `{saved:true,policy}`（服务端新版本）。

仅原子更新日配额、策略版本和操作者/时间，保留用量、租约、暂停、并发、提醒和迁移标记。调低到今日已用次数及以下时后续请求会被拦截，提高或设不限后后续步骤立即按新值执行；不撤回已发送请求，不清空计数。此独立接口避免调整配额时夹带旧的其他配置。

页面配额区不受历史筛选影响，默认每5秒更新；调整弹窗重新读当前版本，后台刷新不覆盖输入，失败保留待提交值。普通统计与当前页记录每15秒刷新，隐藏页面或关闭自动刷新时停用轮询。弹窗统一使用内置Tabler的tabler.Modal，不能引用未加载的bootstrap全局。
