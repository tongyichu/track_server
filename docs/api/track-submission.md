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

只有本人已结束、正常、具有 `raw_track_url` 和轨迹截图的轨迹可以投稿。首次投稿状态为 `pending`；`rejected`、`withdrawn`、`invalidated` 可以重新投稿并增加 `revision`。

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

`GET /api/v1/track/my/list` 的 item 通过 `submission` 返回本人投稿状态摘要。审核通过的推荐/搜索 TrackSummary 返回 `is_featured=true`、`featured_description` 和 `featured_cover_url`，并使用投稿标题作为 `title`。

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
