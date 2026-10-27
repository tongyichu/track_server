# 意见反馈接口

## 纠错选项

`GET /api/v1/feedback/correction-options`

需要登录。返回后台启用的纠错选项，按 `sort_order` 升序、同序按 `id` 排序；响应禁止缓存。客户端每次打开纠错页读取，不内置业务选项；失败展示重试，空列表展示暂无可用选项并禁用提交。

```json
{
  "code": 0,
  "data": {
    "version": 1,
    "options": [
      { "id": "scenic", "name": "景点有误", "name_en": "Wrong scenic spot", "enabled": true, "sort_order": 10 }
    ]
  }
}
```

`id` 是提交时使用的稳定标识，不能根据名称推导。中文展示 `name`，英文展示 `name_en`（为空时回退 `name`）。尚未保存配置时由服务端提供六个初始选项；后台全部停用后返回空列表，不恢复默认项。`version` 用于后台并发保护，客户端无需提交。

## 34. 提交意见反馈

`POST /api/v1/feedback`

需要登录。使用 `multipart/form-data`，服务端直接接收图片并私有落盘到 `<LogDir>/feedback/images`；图片不会通过 `/api/v1/static/*` 暴露。

### 表单字段

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `content` | 普通反馈必填 | 普通反馈 1-1000 字；纠错时为可选补充说明，最多 500 字 |
| `track_id` | 纠错必填 | 真实轨迹 ID，如 `NO.00000001`，与 `correction_option_id` 同时传入 |
| `correction_option_id` | 纠错必填 | 选项接口返回的 `id`；客户端不提交名称或自行拼装纠错正文 |
| `images` | 否 | 图片文件字段，可重复传 0-3 个；兼容字段名 `image` |
| `contact` | 否 | 用户补充联系方式 |
| `app_version` | 否 | 客户端版本；不传时优先取 `X-Client-Version` |
| `platform` | 否 | `ios` / `android`；不传时优先取 `X-Platform` |
| `device_model` | 否 | 设备型号 |
| `system_version` | 否 | 系统版本 |

纠错提交复用本接口、图片与未处理数量限制。服务端校验轨迹存在、已完成且有原始轨迹文件；非本人轨迹须为正常可见状态。不存在/已删除/未上传/仍在录制返回 `404`，无权访问返回 `403`。选项已停用或不存在返回 `409`，客户端重新拉取选项并让用户选择，保留说明及图片，不自动重提。

服务端保存 `correction` 快照，包含真实轨迹标题和提交时的中英文选项名称；后续配置改名/停用不修改历史记录。同时按 `X-Client-Language` 生成可读 `content`，旧版反馈列表与详情仍可展示。提交、历史列表/详情及管理后台返回相同结构；普通反馈及旧纠错记录可无此字段：

```json
{
  "correction": {
    "track_id": "NO.00000001",
    "track_title": "山间步道",
    "option_id": "scenic",
    "option_name": "景点有误",
    "option_name_en": "Wrong scenic spot",
    "notes": ""
  }
}
```

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

## 后台纠错配置

管理后台「反馈与纠错 → 纠错选项」，通过独立管理员 session 鉴权：

- `GET /admin/api/feedback-correction-config`：返回 `{code:0,data:{version,options}}`，含已停用项。
- `PUT /admin/api/feedback-correction-config`：提交完整 `{version,options}`，字段结构同选项接口；`version` 必须使用最近读取值。成功返回保存后的完整配置和递增版本；发生并发修改返回 `409`，页面保留未保存内容，由管理员显式重新加载后修改。

每项 `name` 必填且最多 24 个 Unicode 字符，`name_en` 可空且最多 64 字，名称不能换行；`sort_order` 为 0-9999 整数，`enabled` 为布尔值。最多 30 项；新增项省略/留空 `id`，由服务端生成。已有 ID 不可修改或删除，不需要的选项设 `enabled=false`。保存后客户端下次进入纠错页即可看到变更，无需发版。

配置由 `FeedbackRepository` 在 MySQL 的 `feedback_correction_config`、Mongo 同名 collection 或内存降级仓储保存；内存降级下进程重启后配置丢失。MySQL 启动自动补建配置表和 `user_feedbacks.correction_json`，Mongo 的反馈快照为 `correction` 子文档。
