# 意见反馈接口

## 34. 提交意见反馈

`POST /api/v1/feedback`

需要登录。使用 `multipart/form-data`，服务端直接接收图片并私有落盘到 `<LogDir>/feedback/images`；图片不会通过 `/api/v1/static/*` 暴露。

### 表单字段

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `content` | 是 | 反馈文字，1-1000 字 |
| `images` | 否 | 图片文件字段，可重复传 0-3 个；兼容字段名 `image` |
| `contact` | 否 | 用户补充联系方式 |
| `app_version` | 否 | 客户端版本；不传时优先取 `X-Client-Version` |
| `platform` | 否 | `ios` / `android`；不传时优先取 `X-Platform` |
| `device_model` | 否 | 设备型号 |
| `system_version` | 否 | 系统版本 |

图片限制：

- 最多 3 张。
- 单张最大 5 MiB，总大小最大 15 MiB。
- 仅允许 JPEG / PNG / WebP；服务端按文件内容识别类型，不信任客户端文件名。

提交频控：

- 同一用户最多保留 5 条未处理反馈。
- 未处理反馈包含 `pending` 和 `processing` 状态。
- 达到上限后继续提交返回 `429`，需要等待运营将已有反馈更新为 `resolved` 或 `ignored` 后才能继续提交。

### 响应

```json
{
  "code": 0,
  "data": {
    "feedback_id": "FB202606111530001A2B3C4D",
    "user_id": 123,
    "content": "轨迹保存后封面偶现为空",
    "images": [
      {
        "image_id": "1",
        "url": "/api/v1/feedback/FB202606111530001A2B3C4D/images/1",
        "mime_type": "image/jpeg",
        "size": 102400
      }
    ],
    "status": "pending",
    "reply": "",
    "created_at": "2026-06-11T15:30:00+08:00",
    "updated_at": "2026-06-11T15:30:00+08:00"
  }
}
```

## 35. 我的反馈列表

`GET /api/v1/feedback/list?cursor=&limit=20`

需要登录。只返回当前登录用户自己的反馈，按 `created_at desc, feedback_id desc` 排序。

`next_cursor` 是不透明字符串，客户端翻页时原样传回 `cursor`。

运营填写的 `reply` 会在列表和详情中返回给用户；当 `status=resolved` 时，`reply` 表示处理完成后的反馈意见。

```json
{
  "code": 0,
  "data": {
    "items": [],
    "next_cursor": "",
    "has_more": false
  }
}
```

## 36. 我的反馈详情

`GET /api/v1/feedback/:feedback_id`

需要登录。只能查看当前登录用户自己的反馈；查看他人反馈返回 `403`。

## 37. 读取我的反馈图片

`GET /api/v1/feedback/:feedback_id/images/:image_id`

需要登录。服务端会校验反馈归属，只有提交者本人可以读取图片。

## 38. 运维反馈列表

`GET /api/v1/ops/feedback/list?status=&cursor=&limit=20`

运维内部接口，使用 `X-Internal-Token` 鉴权。`status` 可选：`pending`、`processing`、`resolved`、`ignored`。

## 39. 运维反馈详情

`GET /api/v1/ops/feedback/:feedback_id`

运维内部接口，使用 `X-Internal-Token` 鉴权。

## 40. 运维更新反馈状态

`PUT /api/v1/ops/feedback/:feedback_id/status`

运维内部接口，使用 `X-Internal-Token` 鉴权。

请求：

```json
{
  "status": "resolved",
  "reply": "问题已修复，请更新后重试"
}
```

规则：

- `status=resolved` 表示处理完成，`reply` 必填，且会展示给提交用户。
- `status=processing` / `ignored` 的 `reply` 可选；若填写，同样会展示给提交用户。
- `reply` 最长 2000 字符。

## 41. 运维读取反馈图片

`GET /api/v1/ops/feedback/:feedback_id/images/:image_id`

运维内部接口，使用 `X-Internal-Token` 鉴权，可读取任意反馈图片。
