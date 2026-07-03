# 账号限制

账号限制由管理中心或运维内部接口写入，服务端在业务 JWT 鉴权后通过中间件拦截受限动作。限制可以是永久的，也可以通过 `expires_at` 设置短期限制；过期限制不会继续拦截，历史记录仍保留。

创建或修改轨迹投稿属于内容发布动作，`POST/PUT /api/v1/track/:track_id/submission` 会被上传类账号限制拦截；查询和 `POST .../submission/withdraw` 不拦截。

## 账号限制错误

以下动作在账号处于限制状态时返回 `403`：

- 获取上传用 OSS STS：`GET /api/v1/oss/sts-token`
- 创建或上传/更新轨迹：`POST /api/v1/track/create`、`POST /api/v1/track/:track_id/upload_cloud`、`PUT /api/v1/track/:track_id/update`
- 发起同行：`POST /api/v1/companion/session/create`
- 关注用户：`POST /api/v1/user/:user_id/follow`
- 收藏轨迹：`POST /api/v1/track_collect`
- 修改个人信息：`PUT /api/v1/user/profile/update`、`PUT /api/v1/user/profile/phone`、`PUT /api/v1/user/profile/client_language`

取消收藏、取消关注、退出登录、浏览和查询类接口不受账号限制影响。

客户端可通过 `GET /api/v1/user/:user_id/detail` 查看自己的当前账号限制状态：当 `user_id` 等于当前登录用户且存在生效限制时，响应 `data.account_restriction` 返回限制记录；查看他人或未被限制时不返回该字段。

响应示例：

```json
{
  "error": "account restricted",
  "message": "账号已被限制，禁止上传内容",
  "data": {
    "id": 12,
    "user_id": 1001,
    "status": "active",
    "reason": "违规上传内容",
    "operator": "ops",
    "expires_at": "2026-06-25T10:00:00+08:00",
    "created_at": "2026-06-18T10:00:00+08:00",
    "updated_at": "2026-06-18T10:00:00+08:00"
  }
}
```

动作提示文案：

| 动作 | `message` |
| --- | --- |
| 上传内容 / 获取上传 STS | `账号已被限制，禁止上传内容` |
| 发起同行 | `账号已被限制，禁止发起同行` |
| 关注用户 | `账号已被限制，禁止关注用户` |
| 收藏轨迹 | `账号已被限制，禁止收藏轨迹` |
| 修改个人信息 | `账号已被限制，禁止修改个人信息` |

## 52. 创建账号限制

`POST /api/v1/ops/users/:user_id/restrictions`

运维内部接口，使用 `X-Internal-Token` 鉴权。创建新限制前会先解除该用户未过期的 active 限制，保证同一用户最多只有一条当前生效限制。

请求：

```json
{
  "reason": "违规上传内容",
  "operator": "admin",
  "expires_at": "2026-06-25T10:00:00+08:00"
}
```

字段说明：

| 字段 | 说明 |
| --- | --- |
| `reason` | 必填，限制原因，最多 255 字符 |
| `operator` | 可选，操作人，默认 `ops`，最多 64 字符 |
| `expires_at` | 可选，RFC3339 时间；不传表示永久限制 |

## 53. 查询当前账号限制

`GET /api/v1/ops/users/:user_id/restrictions/current`

运维内部接口，使用 `X-Internal-Token` 鉴权。没有当前生效限制时返回 `404`。

## 54. 查询账号限制历史

`GET /api/v1/ops/users/:user_id/restrictions/history?limit=50`

运维内部接口，使用 `X-Internal-Token` 鉴权。按 `created_at desc, id desc` 返回历史记录。

## 55. 解除当前账号限制

`DELETE /api/v1/ops/users/:user_id/restrictions/current?operator=admin`

运维内部接口，使用 `X-Internal-Token` 鉴权。只解除当前未过期的 active 限制，返回 `revoked_count`。
