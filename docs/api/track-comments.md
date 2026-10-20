# 轨迹评论与回复

## 范围与规则

所有路径相对 `/api/v1`，需要业务 JWT，身份只取 token。只开放正常公开、已结束且 `raw_track_url` 非空的轨迹，与点赞一致。私密/删除/进行中/未上传轨迹不开放评论和图片读取；重新公开后现存评论恢复可见。本人仍可删除自己在失效轨迹上的评论。

- 首版支持文字、普通 Unicode emoji、单张图片；允许纯图。image_ids 数组长度最多为 1。正文去除首尾空白后最多 1000 个 Unicode code point，不支持编辑、主动 @、通知或分享评论。
- 主评论与回复分开分页，按 `created_at DESC,id DESC` 排序。回复平铺在一个主评论楼层内；回复某条回复时引用直接目标，不产生第三层列表。
- 轨迹详情和各类列表摘要直接返回 `comment_count`（未删除主评论与回复总数）和 `can_comment`；不可评论轨迹返回 false/0。详情入口使用这两个字段，不额外请求评论状态接口。评论主列表 `total_count` 与此计数一致；回复页 `total_count` 为该楼层未删除回复数。删除占位不计数。
- 本人可删除自己的评论/回复，轨迹作者没有删除他人评论的权限。主评论删除后有回复则保留占位，最后一条回复删除后该占位从主列表隐藏。被引用内容删除后不再返回原文或图片标记。
- 主评论的 `author_reply` 是当前轨迹作者最新一条未删除回复，客户端最多显示三行；作者以轨迹归属用户为准。
- 点赞为 `1`、点踩为 `-1`、无评价为 `0`，互斥、可切换和取消，重复操作幂等。评论与轨迹点赞相互独立，允许评价自己的评论。
- 受限账号不能发布评论、申请评论图片上传或新增/切换评价，仍可读列表、删除自己的评论和取消评价。

## 接口

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| GET | `/track/:track_id/comments` | 主评论游标分页，附每条评论的作者回复预览 |
| POST | `/track/:track_id/comments` | 发表主评论或回复 |
| GET | `/track/:track_id/comments/:comment_id` | 单条评论/楼层摘要，用于刷新数量及作者回复 |
| GET | `/track/:track_id/comments/:comment_id/replies` | 主评论楼层的回复分页，`root` 返回原评论 |
| GET | `/track/:track_id/comments/:comment_id/likes` | 当前点赞用户游标分页 |
| DELETE | `/track/:track_id/comments/:comment_id` | 删除自己的评论，重复删除成功 |
| PUT | `/track/:track_id/comments/:comment_id/reaction` | 设置赞踩目标状态 |
| POST | `/track/:track_id/comments/images/prepare` | 预约图片 ID、OSS 对象 key 和上传凭证 |
| GET | `/track/:track_id/comments/:comment_id/images/:image_id` | 登录态图片读取，校验轨迹、评论及图片绑定关系 |

分页参数为 `limit`（默认 20，最大 50）与不透明 `cursor`。显式非正 limit、非法或跨轨迹/跨楼层游标返回 400。使用 `limit+1` 判定 `has_more`，不以实时总数驱动翻页。客户端按 ID 合并分页结果。

### 发表与幂等

```json
{
  "client_comment_id": "bb7f015c-66e3-4515-bd23-187c57515de4",
  "content": "风景很好，建议早点出发。😀",
  "reply_to_id": "",
  "image_ids": []
}
```

`client_comment_id` 为 8～64 位 ASCII 字母、数字、下划线或连字符，服务端转为小写。每个用户全局唯一；同一 ID、轨迹和内容的重试返回同一评论，不刷新时间或重复计数；改用同一 ID 提交不同内容返回 409。修改草稿后应生成新的 ID。网络响应不确定时，必须用原 ID 和原图片 ID 重试。

`reply_to_id` 留空表示主评论。回复时传直接目标 ID，服务端推导所属主评论并校验目标有效且属于同一轨迹。不得自行拼接引用正文。图片 ID 必须来自当前用户在当前轨迹下的预约，只能绑定一次；预约有效期 24 小时。

### 评论结构

```json
{
  "id": "TC.0123456789abcdef0123456789abcdef",
  "track_id": "NO.00000001",
  "root_id": "",
  "user": {"id": 1001, "nickname": "路线朋友", "avatar_url": "/api/v1/static/avatars/1001.jpg"},
  "content": "路线很棒😀",
  "images": [],
  "deleted": false,
  "is_author": false,
  "can_delete": true,
  "reply_count": 2,
  "like_count": 3,
  "dislike_count": 0,
  "reaction": 1,
  "created_at": "2026-10-20T08:00:00Z",
  "author_reply": null
}
```

主评论 `root_id` 为空，回复为所属主评论 ID。回复某条回复时额外返回 `quote:{id,user,content,deleted,has_images}`；直接回复主评论不重复附引用。删除占位 `deleted=true` 且正文、图片及评价展示清空。`author_reply` 为同结构的单条回复，无递归预览。图片项为 `{id,url,width,height}`，不下发原始 OSS URL。

主评论列表响应 `data:{items,total_count,has_more,next_cursor?}`；回复页额外返回 `root`。所有响应沿用 `StandardResponse`。

发表、单条查询和赞踩的评论响应额外包含 `comment_count`，表示当前轨迹未删除主评论与回复总数；删除响应为 `data:{status:"ok",comment_count}`。客户端直接用发布/删除响应刷新详情入口和弹窗总数，无需再查询计数。分页仍以游标和 `has_more` 为准。

评价请求：

```json
{"reaction": -1}
```

返回更新后的评论，客户端用响应确认状态和计数；请求失败保留原状态。

### 点赞用户

`GET .../:comment_id/likes` 返回 `data:{items:[{id,nickname,avatar_url}],total_count,has_more,next_cursor?}`，同样支持 limit/cursor。仅列出当前 reaction=1 的登录用户，含自赞；取消点赞或改为点踩后移除。无游客行。按 user_id 倒序稳定分页，不表达点赞时间先后；游标绑定轨迹及评论，跨范围使用返回 400。评论已删除（含有回复的主评论占位）或轨迹不可见时返回 404；受限账号可以读取。

total_count 复用评论已维护的 like_count，分页取 limit+1；只批量加载当前页用户资料，头像沿用异步缓存，查询不逐个下载头像。客户端只在点击人数入口后请求，首屏不附带名单。回复页主评论 0 赞显示“暂无人赞过”，正数可进入名单并点击用户访问主页。

## 图片上传

1. 客户端将图片转为 JPEG，最长边建议不超过 2048 像素、单张不超过 5MB。
2. `POST .../images/prepare` 请求 `{width,height,byte_size}`，宽高上限 4096，返回 `{image_id,object_key,credential}`。credential 与现有 OSS STS 字段一致。
3. 使用该凭证直传到服务端给出的精确 key：`<dir>/comments/<user_id>/<image_id>.jpg`。
4. 发布时只传 `image_ids`。服务端检查预约归属、实际 OSS 大小及 JPEG 尺寸，事务绑定图片与评论。
5. 列表仅返回鉴权图片地址，不同步拉取图片。读取时通过 OSS 内网下载到 `<LogDir>/comments/images` 私有缓存，不挂到通用静态目录；下载限制 5MB 并验证 JPEG。头像仅复用本地命中或异步预热，不在评论查询中逐个同步下载。

需要现有 STS 与 `OSS_INTERNAL_ENDPOINT`。没有 OSS 配置时纯文字评论正常，图片预约返回 503。未完成的上传预约不计入评论数量。

## 持久化与部署

MySQL 新增 `track_comments`、`track_comment_stats`、`track_comment_reactions`、`track_comment_images`，基线见 `mysql.sql`，启动幂等建表。主列表索引为 `(track_id,root_id,created_at,id)`，作者预览索引为 `(root_id,user_id,deleted,created_at,id)`。总数、回复数和赞踩数随写入事务维护，不在每页实时全量聚合。轨迹列表按当前页 ID 批量读取统计表，不逐条查询评论数。评论主列表只对当前页执行有界作者预览查询，批量加载用户、引用与浏览者评价。

点赞人查询使用 `track_comment_reactions(comment_id,reaction,user_id)` 覆盖索引，MySQL 启动会为旧表幂等补齐 `idx_comment_likers`；Mongo 使用相同字段顺序、user_id 降序索引。没有新增业务表或名单副本。

Mongo 使用同名四个 collection，创建相应索引；**评论写入要求 Mongo 副本集/事务支持**。发布和删除通过轨迹统计文档建立事务冲突边界，赞踩只锁定对应评论文档，事务自动重试。`deploy/docker-compose.yml` 提供单节点副本集；已有独立 Mongo 部署需先配置副本集。Mongo 故障不允许将评论单独切到临时内存而丢失持久数据；只有服务启动的既有整仓储降级分支才使用内存实现。

可选真实仓储测试（仅连接临时测试库）：

```sh
go test ./internal/repository -run '^TestCommentPersistence$' -count=1 -v -args \
  -comment-mysql-dsn='root@tcp(127.0.0.1:13316)/comments_test?parseTime=true&charset=utf8mb4' \
  -comment-mongo-uri='mongodb://127.0.0.1:13317/?replicaSet=comments&directConnection=true'
```

常见状态：400 参数/游标/图片不合法；401 未登录；403 账号限制、无删除权限或图片归属不符；404 不可见轨迹、评论或图片；409 幂等 ID 内容冲突；503 图片服务未配置。
