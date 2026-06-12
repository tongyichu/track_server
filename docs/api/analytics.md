# 埋点采集接口

> Base URL: `http://<host>:<port>/api/v1`
>
> 本接口用于客户端批量上报埋点事件。服务端只做基础校验、敏感字段清理和本地 JSONL 落盘；OSS 同步由定时任务异步完成。
>
> 事件命名、公共属性、业务事件清单、隐私约束与存储方案见 [../../track_analytics.md](../../track_analytics.md)；本文只定义上报接口契约。

## 47. 批量上报埋点事件

`POST /analytics/events`

需要认证：否。

说明：

- 未登录状态也可以上报，用于启动、登录页、权限弹窗等场景。
- 已登录客户端建议继续携带 `Authorization` 和 `X-User-ID`，服务端会补充 `server_user_id`。
- 单次默认最多 50 条事件，body 默认不超过 256 KiB。
- 写入成功表示服务端本地磁盘已落盘，不代表已同步到 OSS。

### 请求头

| Header | 必填 | 说明 |
| --- | --- | --- |
| `Content-Type` | 是 | 固定 `application/json` |
| `Authorization` | 否 | `Bearer <token>`；已登录时建议携带 |
| `X-User-ID` | 否 | 已登录用户 ID |
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
| `events[].anonymous_id` | string | 建议 | 匿名设备 ID；若为空，服务端尝试使用 `X-Device-ID` 补充 |
| `events[].session_id` | string | 建议 | App 前台会话 ID |

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

### 错误

| HTTP 状态码 | 说明 |
| --- | --- |
| 400 | JSON 格式错误、事件为空、超过单批条数、缺少 `event_id` 或 `event_name` |
| 413 | body 超过 `ANALYTICS_MAX_BODY_BYTES` |
| 500 | 本地落盘失败 |
| 503 | 埋点服务未配置或 `ANALYTICS_ENABLED=false` |

## 服务端存储与同步

- 本地目录默认 `<LogDir>/analytics/events/`，可通过 `ANALYTICS_LOCAL_DIR` 覆盖。
- 活跃写入文件后缀为 `.writing`，轮转后改为 `.jsonl`。
- 定时任务 `analytics_sync` 默认每天 03:00 执行，由 `ANALYTICS_SYNC_CRON` 覆盖。
- OSS 归档前缀默认 `analytics/ods/`，可通过 `ANALYTICS_OSS_PREFIX` 覆盖。
- OSS 同步强制使用 `OSS_INTERNAL_ENDPOINT` 内网域名；未配置时同步失败并保留本地文件等待重试，不会回退公网 Endpoint。
- 上传后的 OSS key 形如 `analytics/ods/event_date=2026-06-12/hour=15/<instance>-000001.jsonl`。
- 上传成功后，服务端会删除对应本地 JSONL 文件，并尽力清理空的小时/日期目录。
- 每次同步任务都会向 `analytics_sync_summaries` 写入一条摘要，记录开始/结束时间、耗时、扫描/上传/失败文件数、成功上传字节数、OSS 前缀、文件明细 JSON 和错误摘要。
- 摘要写入失败不会影响本地文件上传结果；服务端只记录日志，避免摘要表故障阻断埋点归档。
