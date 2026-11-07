# 后台用户资料与级别

下列接口使用独立后台 session，权限归属 `users`；GET 需查看权限，PUT/限制操作需操作权限，并保留同源检查与审计。用户 ID 在所有 JSON 响应中使用字符串。

## 列表

`GET /admin/api/users` 保留昵称、内部人员、省市、性别、星座、限制状态和游标筛选。列表行展示用户、手机号、性别、所在城市、上次登录和详情入口；头像保留内部人员角标，昵称旁只显示 `Lv.N`。

行响应新增 `level`（整数）；级别读取失败为 `null`，页面隐藏级别标签，不伪造 Lv.1。不再下发列表用的 `zodiac_sign` 和 `account_restriction`，这两项在详情展示；对应筛选仍在仓储层参与 total/分页。级别名不进入列表行响应。

级别查询由 `UserService.GetUserLevels` 对当前页 user_id 一次批量读取，`AchievementTrackReader.ScanAchievementTracks` 的 MySQL/Mongo/内存实现流式投影所需轨迹字段，再复用现有 `isQualifiedAchievementTrack`、`achievementTrackXP` 和 `resolveAchievementLevel`。规则与客户端一致，不逐用户读全部轨迹详情，不结算奖励、不触发后台回填或供应商调用。

## 资料读取与保存

`GET /admin/api/users/:user_id` 返回 `id/nickname/avatar_url/signature/background_url/gender/birthday/zodiac_sign/phone/province_code/province_name/city_code/city_name/is_internal/created_at/updated_at`。

- `background_url` 非空时固定为该用户的 `/admin/api/users/:user_id/background`，不暴露原始 OSS 地址。
- `zodiac_sign` 由服务端根据生日派生；未填写/无效日期为空串。后台选择生日时即时预览标签，保存只提交生日。边界规则见 [用户星座](user.md#星座派生规则)，图标和明暗配色复用 Android 客户端 MingCute 资源，许可随静态资源分发。
- 手机号、所在地、头像、背景图、互动数据与级别只读。

`PUT /admin/api/users/:user_id` 只保存 `nickname/signature/gender/birthday/is_internal`。昵称 1–20 字、签名最多100字；生日格式、范围与原实现相同。任何显式 `phone` 字段（含空串或 null）均返回400，数据库中的手机号保留原值。其余只读展示信息不随表单提交。

## 互动数据与级别

`GET /admin/api/users/:user_id/overview` 返回：

```json
{"code":0,"data":{"following_count":12,"follower_count":8,"received_like_count":25,"level":{"level":3,"name":"周末行者","xp":1000}}}
```

关注/粉丝复用当前关系计数；获赞复用客户端 `CountReceived` 口径（有效公开已完成且有原始文件的轨迹，排除自己的点赞）。级别使用同一 XP 规则，`level.xp` 是该级别门槛而非用户总经验值。查询固定并发、只读，不调用会结算成就的个人主页汇总方法。失败返回503，详情保留基本资料，指标显示“—”，提供独立重试。

## 背景图

`GET /admin/api/users/:user_id/background` 需 users 查看权限，只能读取该用户资料指向、经过既有媒体缓存解析的背景图片。响应 `no-store`、`nosniff` 并限制 SVG 等图像文档的脚本能力；不接受客户端源 URL、不把私有图像混入公共静态资源。没有背景或读取失败时页面展示空态和重试，不阻断表单。每次加载使用独立图片节点与当前详情身份判断，直接按图片加载结果切换展示，不依赖隐藏图片的 `currentSrc` 后缀判断；30秒未完成则停止加载并提供重试，关闭、切换用户和重试时取消旧图片及计时器。

## 账号限制与弹窗加载

账号限制集中在详情右侧账号管理区：读取既有 `GET /users/:user_id/restrictions/current`（404表示无生效限制），创建和解除沿用原 POST/DELETE 接口。在当前详情内展开限制原因/天数或解除确认，显式确认后立即生效，独立于“保存资料”；失败保留资料草稿，关闭不自动重试写请求。

弹窗顶部显示背景图、头像、昵称、完整级别和三项互动数据；正文为基本资料与账号管理/设备权限两列，窄屏上下排列。基本资料、互动数据、限制状态独立加载，背景在图片请求中解析；设备权限首次展开时才查询。沿用详情入口去重、关闭取消和旧响应忽略，辅助数据失败不覆盖正在编辑的字段。
