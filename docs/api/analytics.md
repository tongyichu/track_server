# 埋点采集接口

> Base URL: `http://<host>:<port>/api/v1`
>
> 本接口用于客户端批量上报埋点事件。服务端只做基础校验、敏感字段清理和本地 JSONL 落盘；OSS 同步由定时任务异步完成。
>
> 事件命名、公共属性、业务事件清单、隐私约束与存储方案见 [../../track_analytics.md](../../track_analytics.md)；本文只定义上报接口契约。

## 47. 批量上报埋点事件

`POST /analytics/events`

需要认证：匿名事件否；`user_id` 非空的登录用户事件是。

说明：

- 未登录状态也可以上报，用于启动、登录页、权限弹窗等场景。
- 登录用户事件必须携带有效 `Authorization`，且事件发生时的 `user_id` 必须与 JWT 用户一致；`X-User-ID` 不能作为可信身份凭证。
- 单次默认最多 50 条事件，body 默认不超过 256 KiB。
- 写入成功表示服务端本地磁盘已落盘，不代表已同步到 OSS。
- 批次采用整批确认：成功时全部接受，失败时不返回逐条部分成功结果。
- `events[].properties` 承载事件特有属性；推荐曝光和转化事件必须遵循《轨迹 App 埋点方案》第 4.1 节与第 6.2.1 节的推荐归因协议。

### 请求头

| Header | 必填 | 说明 |
| --- | --- | --- |
| `Content-Type` | 是 | 固定 `application/json` |
| `Authorization` | 条件必填 | `Bearer <token>`；批次中存在非空 `user_id` 时必须携带，且 JWT 用户必须一致 |
| `X-User-ID` | 否 | 仅用于请求诊断，不参与可信身份归属，也不能覆盖事件内 `user_id` |
| `X-Device-ID` | 否 | 匿名设备 ID，服务端可补到 `anonymous_id` |
| `X-Platform` | 否 | `ios` / `android` / `web` |
| `X-Client-Version` | 否 | App 版本 |
| `X-Client-Language` | 否 | 客户端语言 |

### 请求体

```json
{
  "events": [
    {
      "event_id": "018f7d4a-2b6f-7f3f-9f3d-2a4fb8fdc001",
      "event_name": "app_launch",
      "client_time": "2026-06-12T10:00:00+08:00",
      "send_time": "2026-06-12T10:00:01+08:00",
      "anonymous_id": "device-uuid",
      "session_id": "session-uuid",
      "platform": "ios",
      "app_version": "1.0.0",
      "properties": {
        "launch_type": "cold"
      }
    }
  ]
}
```

### 字段约束

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `events` | array | 是 | 事件列表，默认 1 到 50 条 |
| `events[].event_id` | string | 是 | 单条事件唯一 ID，重试补发保持不变，最长 128 字符 |
| `events[].event_name` | string | 是 | 事件名，最长 128 字符 |
| `events[].client_time` | string | 建议 | 客户端事件发生时间 |
| `events[].send_time` | string | 建议 | 客户端实际发送时间 |
| `events[].user_id` | string | 条件必填 | 事件发生时的登录用户；未登录为空。进入本地队列后不可因登录、退出或切换账号而改写 |
| `events[].anonymous_id` | string | 建议 | 匿名设备 ID；若为空，服务端尝试使用 `X-Device-ID` 补充 |
| `events[].session_id` | string | 建议 | App 前台会话 ID |
| `events[].properties` | object | 按事件要求 | 事件特有属性；字段和条件必填规则见《轨迹 App 埋点方案》 |

### 用户身份归属

- 客户端按匿名身份和 `user:<user_id>` 隔离本地队列，同一个批次只能包含同一身份域的事件。
- A 用户产生但尚未发送的事件，在 B 用户登录后不得改写成 B，也不得携带 B 的 JWT 发送；应保留在 A 的队列中，等 A 再次鉴权后补发，或在超过本地保留周期后淘汰。
- 匿名事件的 `user_id` 必须保持为空，即使事件在登录后才补发也不能追溯绑定到当前用户。
- 服务端对携带 Authorization 的请求校验 JWT，并仅从有效 JWT 生成可信 `server_user_id`；`X-User-ID` 和客户端声明的 `user_id` 都不能单独建立可信身份。
- 批次内任何非空 `user_id` 与 JWT 用户不一致时，返回 `403 analytics_user_mismatch` 并整批拒绝；非空 `user_id` 未携带有效 JWT 时，返回 `401 analytics_auth_required` 并整批拒绝。
- 服务端不得使用当前请求用户回填空 `user_id`。推荐画像只消费 `user_id` 与有效 JWT 已核验一致的事件；匿名事件和未核验身份不得进入个人画像。

### 推荐曝光事件示例

```json
{
  "events": [
    {
      "event_id": "018f7d4a-2b6f-7f3f-9f3d-2a4fb8fdc002",
      "event_name": "track_recommend_impression",
      "client_time": "2026-10-04T10:00:00+08:00",
      "send_time": "2026-10-04T10:00:01+08:00",
      "user_id": "1001",
      "anonymous_id": "device-uuid",
      "session_id": "session-uuid",
      "platform": "android",
      "app_version": "1.0.0",
      "properties": {
        "track_id": "NO.00001234",
        "recommend_request_id": "rec_01J...",
        "recommend_strategy": "hybrid",
        "candidate_source": "city_hot",
        "rank_index": 3,
        "city_code": "330100"
      }
    }
  ]
}
```

当前采集接口在服务端强制校验身份域/JWT 一致性、`event_id`、`event_name`、批量条数和请求体大小；除身份字段外的事件特有业务必填属性由客户端联调和数据验收保证。服务端会原样保存通过脱敏后的 `properties`，客户端不能因为采集接口返回成功而认为推荐归因字段已经完整。

### 响应

```json
{
  "code": 0,
  "data": {
    "accepted": 1,
    "status": "ok"
  }
}
```

`200 OK` 表示整批事件均已通过校验并完成本地落盘，`accepted` 必须等于请求中 `events` 的数量。第一版不提供逐条结果，也不会主动返回 `0 < accepted < events.length` 的部分成功响应。

如果客户端收到 2xx 但 `accepted` 缺失或小于发送条数，应视为协议异常，保留整批并使用原 `event_id` 重试，同时记录客户端诊断日志。网络断开、超时或 5xx 时，服务端是否已写入部分数据可能无法确定，客户端仍需整批重试；下游以 `event_id` 去重。

错误响应必须提供稳定机器码，例如：

```json
{
  "error": "analytics user mismatch",
  "error_code": "analytics_user_mismatch"
}
```

### 错误

| HTTP 状态码 | `error_code` | 客户端处理 |
| --- | --- | --- |
| 400 | `analytics_invalid_payload` | JSON、条数或事件字段非法，整批拒绝；不得原样无限重试。客户端应先本地校验，必要时拆分批次定位并丢弃非法事件 |
| 401 | `analytics_auth_required` | 登录用户事件缺少有效 JWT，整批拒绝；等待对应用户重新鉴权后再发送，不能改成当前其他用户 |
| 403 | `analytics_user_mismatch` | 事件 `user_id` 与 JWT 用户不一致，整批拒绝；保留到匹配用户重新登录后发送 |
| 413 | `analytics_payload_too_large` | 按事件边界拆分为更小批次后立即重试，单条事件本身超限则丢弃并记录诊断 |
| 429 | `analytics_rate_limited` | 整批保留，优先遵循 `Retry-After`，否则指数退避重试 |
| 500 | `analytics_write_failed` | 本地落盘失败或结果不确定，使用相同 `event_id` 整批退避重试 |
| 503 | `analytics_unavailable` | 服务未配置、关闭或暂时不可用，整批退避重试；存在 `Retry-After` 时优先遵循 |

### 重试与本地队列

- 429、500、503、网络中断和超时属于可重试错误。建议退避间隔为 1、2、4、8、16、32、60 秒，之后封顶 60 秒，并增加 0～20% 随机抖动；App 重启后继续保留下次重试时间。
- 400、401、403 不得对同一请求做无条件自动重试；必须先修正数据或恢复匹配身份。413 需要拆分批次，而不是原样重试。
- 所有重试保持原始 `event_id`、`client_time`、`user_id` 和推荐上下文不变；`send_time` 更新为本次实际发送时间。
- 本地队列默认最多保留 1000 条或 7 天；达到上限时先淘汰非关键事件。关键事件完整名单和淘汰顺序见《轨迹 App 埋点方案》第 11.2 节。

## 服务端存储与同步

- 本地目录默认 `<LogDir>/analytics/events/`，可通过 `ANALYTICS_LOCAL_DIR` 覆盖。
- 活跃写入文件后缀为 `.writing`，轮转后改为 `.jsonl`。
- 定时任务 `analytics_sync` 默认每天 03:00 执行，由 `ANALYTICS_SYNC_CRON` 覆盖。
- 推荐曝光、点击和详情浏览按 T+1、24 小时级进入下游统计，不承诺采集成功后实时参与推荐；收藏、导航和关注等强行为由推荐任务从业务数据库聚合。
- OSS 归档前缀默认 `analytics/ods/`，可通过 `ANALYTICS_OSS_PREFIX` 覆盖。
- OSS 同步强制使用 `OSS_INTERNAL_ENDPOINT` 内网域名；未配置时同步失败并保留本地文件等待重试，不会回退公网 Endpoint。
- 同步时按 `event_date/hour` 时间分区合并小 JSONL 文件，单个 OSS part 目标上限为 128 MB。
- 上传后的 OSS key 形如 `analytics/ods/event_date=2026-06-12/hour=15/part-<instance>-2026-06-12-15-*.jsonl`。
- 上传成功后，服务端会删除参与该 part 合并的本地 JSONL 文件和临时 part，并尽力清理空的小时/日期目录。
- 每次同步任务都会向 `analytics_sync_summaries` 写入一条摘要，记录开始/结束时间、耗时、扫描源文件数、上传 part 数、失败 part 数、成功上传字节数、OSS 前缀、文件明细 JSON 和错误摘要；文件明细中包含每个 part 对应的源文件列表。
- 摘要写入失败不会影响本地文件上传结果；服务端只记录日志，避免摘要表故障阻断埋点归档。
- 推荐原始事件在 ODS/DWD 保留 180 天；生命周期清理由对象存储或数据仓库侧配置，不由本采集接口同步删除。
