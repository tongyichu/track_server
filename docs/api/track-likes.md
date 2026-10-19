# 轨迹点赞与获赞

所有接口基于 `/api/v1`，需要业务 JWT；操作用户只取 token 身份。点赞与收藏独立。通用响应及错误格式见 [common.md](common.md)。

## 点赞状态

`GET /track/:track_id/like/status`

```json
{"code":0,"data":{"liked":false,"like_count":12,"can_like":true}}
```

- `liked`：当前浏览者是否已点赞；`like_count`：该轨迹的有效点赞关系数量。
- `can_like`：是否允许当前用户点赞。轨迹作者也可以点赞或取消点赞，自赞计入该轨迹 `like_count`，但不计入个人页获赞数与获赞记录。
- 只支持正常公开、已结束且 `raw_track_url` 非空的轨迹。不可用轨迹对他人返回 404；作者查询自己尚未公开或未上传的轨迹时返回数量 0、`can_like=false`。已删除轨迹返回 404。

## 点赞与取消点赞

`POST /track/:track_id/like`、`DELETE /track/:track_id/like`，无 body；响应均为上述状态结构。

- 同一用户与同一轨迹只有一条关系。重复 POST 不增加数量，不改变首次点赞时间；重复 DELETE 不重复扣减。取消后重新点赞创建新的时间。
- POST 支持给自己的轨迹点赞；不存在、私密、已删除、进行中、未上传轨迹返回 404；受限制账号返回 403（`账号已被限制，禁止点赞`）。
- DELETE 允许移除曾经点赞、目前已私密或删除的记录；目标已失效时幂等返回零状态。受限制账号仍可取消点赞。
- 客户端使用服务端返回的数量与状态，连续点击期间禁用按钮，请求失败保留上次成功状态。

## 获赞首页：按轨迹聚合

`GET /user/:user_id/likes/received/tracks?limit=20&cursor=<next_cursor>`

每条轨迹只返回一项，服务端先聚合再分页；按 `last_liked_at DESC, track_id DESC` 排序。获赞首页使用本接口，避免热门轨迹刷屏。

```json
{"code":0,"data":{"items":[{
  "track":{"id":"NO.00000001","user_id":1002,"title":"山间路线","liked":true,"like_count":129},
  "received_like_count":128,
  "recent_users":[{"id":1001,"nickname":"徒步者","avatar_url":"/api/v1/static/avatars/1001.jpg"}],
  "last_liked_at":"2026-10-20T08:00:00Z"
}],"total_count":12,"next_cursor":"opaque-cursor","has_more":true}}
```

- `track` 为完整 `TrackSummary`，示例省略其他字段；路线预览使用 `track_no_map_bg_screenshot_url` 中的独立 SVG 轨迹线，不依赖地图截图。
- `received_like_count` 为其他用户对该轨迹的有效点赞人数，排除作者自赞。`track.like_count` 含自赞，因此两者可以不同。
- `recent_users` 最多 3 人，按点赞时间、用户 ID 倒序，只含公开 ID、昵称、头像；`last_liked_at` 为最近一条他人点赞的时间。
- `total_count` 是有获赞的**轨迹数**，不是个人获赞总数。个人页 `received_like_count` 仍统计所有有效他人点赞。
- 首页只查询当前分页轨迹的最近 3 人，不加载爆款轨迹的完整名单；完整名单使用下方独立接口。有效点赞关系长期保存在 `track_likes`，不创建独立消息表，也不按显示期限自动删除。
- 默认每页 20 条，最大 50 条。非正/非整数 limit、非法游标返回 400，不存在用户返回 404。只返回正常公开、已结束、raw URL 非空的轨迹；没有他人点赞的轨迹不返回。
- 新增点赞或取消点赞可能改变实时排序，客户端按轨迹 ID 合并分页结果并支持下拉刷新，不能自行对逐条记录分页后再聚合。
- 游标由 `last_liked_at` 与 `track_id` 组成；空列表 `items=[]`，无后续时省略 `next_cursor`。

## 某条轨迹的获赞人列表

`GET /user/:user_id/likes/received/tracks/:track_id/users?limit=20&cursor=<next_cursor>`

```json
{"code":0,"data":{"items":[{"user":{"id":1001,"nickname":"徒步者","avatar_url":""},"liked_at":"2026-10-20T08:00:00Z"}],"total_count":128,"next_cursor":"opaque-cursor","has_more":true}}
```

- 点击聚合卡片的人数进入本列表；点击轨迹区域仍进入轨迹详情，点击头像进入用户主页。
- `total_count` 为该轨迹他人点赞人数，排除作者。按 `created_at DESC,user_id DESC` 排序，游标含固定 `track_id`；跨轨迹使用游标返回 400。
- 用户不存在、轨迹不属于路径用户、私密/已删除/进行中/未上传轨迹返回 404。需要业务 JWT，主客态均可查看。
- 默认每页 20 条，最大 50 条。取消点赞后记录删除，人数、最近点赞人与首页排序随有效关系更新。

## 获赞逐条记录（兼容旧入口）

`GET /user/:user_id/likes/received/list?limit=20&cursor=<next_cursor>`

保留用于旧客户端；新版获赞首页使用上面的聚合入口。主态和客态均可查看。每一项是一条有效点赞记录，**不按点赞人合并**。同一人赞了两条轨迹会返回两项。

```json
{
  "code":0,
  "data":{
    "items":[{
      "user":{"id":1001,"nickname":"徒步者","avatar_url":"/api/v1/static/avatars/1001.jpg"},
      "track":{"id":"NO.00000001","user_id":1002,"title":"山间路线","liked":true,"like_count":12},
      "liked_at":"2026-10-19T08:00:00Z"
    }],
    "total_count":12,
    "next_cursor":"opaque-cursor",
    "has_more":true
  }
}
```

- `track` 为完整的 `TrackSummary`；示例省略其他摘要字段。`track.liked` 按当前浏览者计算。`user` 为点赞人的公开资料，不包含手机号等私密字段。
- 默认 20 条，最大 50 条；非正数或非整数 limit、非法 cursor 返回 400。用户不存在返回 404。
- 按 `created_at DESC, track_id DESC, user_id DESC` 排序，游标由这三个值组成，不应自行构造。没有下一页时省略 `next_cursor`。
- `total_count` 与用户详情 `received_like_count` 使用相同可见性规则：只统计其他用户对该用户正常公开、已结束、已上传轨迹的现存点赞，排除作者自赞。私密、已删除、进行中、未上传轨迹不计入。恢复公开后其仍存在的点赞关系恢复可见。
- 取消点赞删除对应记录；空列表 `items=[]`、`total_count=0`、`has_more=false`。

## 现有响应新增字段

- 轨迹详情、推荐/搜索、本人列表、主页轨迹/收藏列表的摘要增加 `liked`（bool）、`like_count`（int64）。本人列表的 `liked` 也按浏览者计算，可以为 `true`。
- 用户详情增加 `received_like_count`（int64），见 [user.md](user.md)。
- MySQL 新表 `track_likes` 在启动建表时幂等创建，部署账号需要相应建表权限；也可先执行 `mysql.sql` 对应建表语句。Mongo 使用 `track_likes` collection，首次写入前创建 `(user_id,track_id)` 唯一索引。内存仓储遵循相同幂等与分页规则。

## 客户端数量展示

服务端始终下发原始整数 `like_count` 与 `received_like_count`，不发送本地化缩写。轨迹列表进入详情时透传已有 `liked` / `like_count` 供首屏显示，再由状态或操作响应更新，防止旧列表覆盖已确认的新状态。点赞数量为零或缺失时仅显示心形，不显示 0 或占位符。中文计数满 10000 使用“万”，英文满 1000 使用“K”，最多一位小数、向下截断并去掉 `.0`；个人页统计保留零并沿用同一格式。

## 查询索引

MySQL 启动时幂等补齐 `idx_track_likes_track_time(track_id,created_at,user_id)`，部署账号需拥有对应建索引权限；每页最近用户使用有界索引查询并批量返回。Mongo 新增相同字段的复合索引，聚合从目标作者的公开轨迹出发，在分组分页后查最多 3 位最近点赞人，不把热门轨迹的所有用户压入聚合数组。
