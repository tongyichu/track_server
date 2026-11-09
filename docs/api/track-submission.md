# 轨迹投稿接口

轨迹投稿用于给已完成公开轨迹补充结构化路线资料。投稿审核通过后会在推荐列表当前分页窗口内优先展示，并成为 RouteGroup 展示代表轨迹候选。

除管理后台接口外，本文业务接口均需要业务 JWT。

## 1. 投稿选项

`GET /api/v1/track/submission/options`

返回难度、风险、路面与地形、交通方式，以及投稿图片限制。客户端应提交 option 的英文 `code`。

## 2. 创建或重新投稿

`POST /api/v1/track/:track_id/submission`

```json
{
  "title": "西湖群山十里徒步环线",
  "description": "从云栖竹径出发，经过五云山和九溪返回，沿途林荫路段较多。",
  "difficulty": "standard",
  "risk_level": "low",
  "suitable_months": [3, 4, 5, 9, 10, 11],
  "surface_types": ["stone_slab", "stairs", "dirt"],
  "transport_modes": ["taxi", "self_drive"],
  "transport_description": "导航至云栖竹径停车场，节假日建议打车。",
  "images": []
}
```

`images` 可省略或传空数组，最多 9 张。图片由客户端使用现有 OSS STS 直接上传到 `<OSS_UPLOAD_PREFIX>/<bucket_id>/submission/<user_id>/`，服务端保存 `oss_url`；返回时缓存到 `<LogDir>/static/submission_images/` 并改写为 `/api/v1/static/submission_images/...`。

`transport_description` 为选填，可省略或传空字符串，trim 后最多 500 个 Unicode 字符；无最少字数限制，选择其他交通方式时也可留空。`transport_modes` 仍须至少选择一项。

只有本人已结束、正常公开且具有 `raw_track_url` 的轨迹可以投稿。提交与审核通过使用同一轨迹资格校验，不校验 `preview_images`、`track_screenshot_url` 或无底图截图；这些字段属于轨迹上传流程自动生成的展示资料，不是投稿必填项。首次投稿状态为 `pending`；`rejected`、`withdrawn`、`invalidated` 可以重新投稿并增加 `revision`。

不符合资格时返回 HTTP 400，并按实际原因返回 `error_code`：`submission_track_running`（仍在录制）、`submission_track_status`（非正常公开）、`submission_track_raw_missing`（缺原始文件）。`submission_transport_length` 仅在交通说明超过 500 字符时返回。

## 3. 修改待审核投稿

`PUT /api/v1/track/:track_id/submission`

请求体同创建接口。仅允许修改 `pending` 投稿，修改后 `revision` 增加。

## 4. 查询投稿

`GET /api/v1/track/:track_id/submission`

投稿人可以查看所有状态及驳回原因；其他用户只能查看 `approved` 投稿。未通过投稿对其他用户返回 `404`。

## 5. 撤回投稿

`POST /api/v1/track/:track_id/submission/withdraw`

撤回只把 `pending` 或 `approved` 投稿更新为 `withdrawn`，不删除投稿内容、图片和审核流水。已通过投稿撤回后立即失去推荐与代表轨迹候选资格。

## 6. 状态

| 状态 | 说明 |
| --- | --- |
| `pending` | 等待审核 |
| `approved` | 审核通过 |
| `rejected` | 审核驳回，可修改后重投 |
| `withdrawn` | 用户撤回 |
| `invalidated` | 轨迹删除或关键内容变化导致审核失效 |

`GET /api/v1/track/my/list` 的 item 通过 `submission` 返回本人投稿状态摘要。我的轨迹、收藏列表 `/track/collected/list` 和推荐/搜索摘要均明确返回 `is_featured`，仅当前投稿状态为 `approved` 时为 true，其余为 false；按当前页批量读取投稿状态，不逐条补查详情。审核通过的推荐/搜索 TrackSummary 还返回 `featured_description` 和 `featured_cover_url`，并使用投稿标题作为 `title`。封面优先投稿首图，无投稿首图或缓存失败时使用新版日间横图，再回退旧截图。

## 7. 管理后台审核

以下接口使用 admin session：

```text
GET  /admin/api/track-submissions
GET  /admin/api/track-submissions/:submission_id
POST /admin/api/track-submissions/:submission_id/review
```

审核请求：

```json
{"decision":"approved","reason":"","expected_revision":2}
```

`decision` 支持 `approved`、`rejected`。驳回时 `reason` 必填；版本不一致返回 `409`。

后台列表与详情响应的 `data` 附带 `options`（复用投稿难度、风险、地形及交通选项）和 `track_types:[{code,name,color,icon}]`（来自 `DefaultTrackTypeConfigs`），供页面展示中文名称及内置类型图标，不改变投稿中持久化的英文 code。

列表隐藏投稿编号，保留轨迹编号；类型使用中文、主题色和内置运动图标标签，难度、风险、状态均为中文。所有状态的“查看”在当前页打开同一详情弹窗，包括已通过投稿；点击立即显示加载态，关闭取消查询，失败可原地重试，切换投稿忽略旧响应。弹窗按路线概况、路线介绍/投稿图片、出行信息、投稿与审核、可折叠审核记录分区；桌面双列，窄屏单列，底部关闭与审核操作固定。地形、交通、月份和审核事件使用中文展示，图片沿用后台受鉴权缓存代理。

仅待审核且有投稿操作权限时展示审核意见、通过和驳回操作；已通过等其他状态只读。驳回前校验原因，提交期间禁止重复提交，继续使用 `expected_revision` 防止旧详情覆盖新审核；成功后刷新列表及统计，失败不宣称审核成功。

当前 Android 首次投稿可读取已有服务端默认 `description` 作为可编辑初稿；尚未生成时留空，不再用客户端 GPS 算法补文案。已有投稿仍以投稿记录为准；投稿说明独立保存，不覆盖默认说明，新的投稿辅助生成规则另行定义。
