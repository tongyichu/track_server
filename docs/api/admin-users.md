# 后台用户资料与级别

下列接口使用独立后台 session，权限归属 `users`；GET 需查看权限，PUT/限制操作需操作权限，并保留同源检查与审计。用户 ID 在所有 JSON 响应中使用字符串。

## 列表

`GET /admin/api/users` 支持昵称、内部人员、省市、性别、用户等级、限制状态和游标筛选。页面以“用户等级”替换星座筛选，传入精确 `level`；不传表示全部等级，非法等级返回400。旧 `zodiac_sign` 查询参数继续兼容。列表行展示用户、手机号、性别、所在城市、粉丝数、最近活跃和详情入口；头像保留内部人员角标，昵称旁只显示 `Lv.N`。

行响应包含 `level`（整数）及 `follower_count`（整数）。未按等级筛选时，级别读取失败为 `null`，页面隐藏标签，不伪造 Lv.1；粉丝数读取失败为 `null`，页面显示“—”，无粉丝时为0。响应 `data.level_options` 取权威等级定义（`level/name/xp`），供筛选项使用，级别名不进入列表行。列表不下发 `zodiac_sign` 和 `account_restriction`，这两项在详情展示。`next_cursor.id` 使用字符串，保持完整 int64 精度。

未选择等级时，`UserService.GetUserLevels` 对当前页 user_id 一次批量读取，`AchievementTrackReader.ScanAchievementTracks` 的 MySQL/Mongo/内存实现流式投影所需轨迹字段，复用 `isQualifiedAchievementTrack`、`achievementTrackXP` 和 `resolveAchievementLevel`。选择等级时，通过 `AchievementTrackScanner.ScanAllAchievementTracks` 对全量已完成有效状态轨迹做一次流式读取，再按同一资格及 XP 规则计算用户等级；匹配 ID 集合进入三种用户仓储的查询条件，先筛选再统计 total、应用游标和分页。初始等级通过排除更高等级用户匹配，包含没有轨迹的用户；无匹配的非初始等级返回空列表。等级依赖不可用时筛选返回503，不忽略该条件。显式等级筛选的查询成本随轨迹规模增长；未筛选时仍保持当前页读取。规则与客户端一致，不逐用户读全部轨迹详情、不结算奖励、不触发后台回填或供应商调用。

粉丝数通过 `FollowRepository.CountFollowersByUserIDs` 按当前页批量聚合，MySQL 使用 GROUP BY，Mongo 使用 match/group，内存实现遍历关注关系并统计目标用户；不按行请求详情。用户列表“刷新”位于“用户列表 共 xx 个用户”标题行右侧；分页在表格下方，与同行房间复用布局：左侧“第 N 页 · 本页 X 个用户”，右侧上一页/下一页，窄屏纵向排列。加载时提示“正在加载…”并禁用翻页，失败明确提示，快速筛选忽略旧响应。

## 最近活跃时间

列表与详情返回 `last_active_at`（时间或 `null`），列表用它替换旧 `last_login_at`。含义是服务端最近观察到该用户已认证的 App 业务请求，包含登录成功及持有效业务 JWT 的非静态资源请求（响应小于500且非401）；身份只取认证结果，不信任 `X-User-ID`。后台 session、内部 token、匿名升级检查、静态图片下载及无效认证不会更新它。后台读取/保存用户资料也不计入活跃。

接口保留带时区的时间值；用户管理页复用 `runtime.js` 的 `AdminTime` 按 `Asia/Shanghai` 显示最近活跃、注册、资料更新和限制到期时间，格式为 `YYYY-MM-DD HH:mm:ss`，不受浏览器本地时区影响。不得直接截断 RFC3339 的 `Z`/偏移，也不调整数据库历史时间来补偿显示。空值或无效值显示“—”。通用约定见[后台时间与时区](../admin-ui.md#时间与时区)。

`UserService.RecordActivity` 在单进程内按用户约一分钟限频（有界缓存），每次写入最多等待500ms，失败只记日志、不影响业务响应，下一限频窗口可重试；不新增请求、心跳或无限后台任务。多设备按同一用户合并，三种仓储只接受更晚时间。该值可能包含后台业务请求，不能用于判断用户此刻在线、前台停留时长或离线阅读；缓存淘汰或服务重启可提前再次写入。

MySQL 启动幂等补充 `users.last_active_at DATETIME(6) NULL`；Mongo 同名字段、内存独立映射，实现 `UserActivityRepository`。当前页用一次批量查询，详情用相同读取口径；读取失败或尚未采集为 `null`，页面显示“—”，不阻断基础资料。不用登录日志、设备绑定或 `updated_at` 回填历史活跃时间。字段仅在后台接口中下发，不进入 App 用户/他人主页响应。

## 资料读取与保存

`GET /admin/api/users/:user_id` 返回 `id/nickname/avatar_url/signature/background_url/gender/birthday/zodiac_sign/phone/province_code/province_name/city_code/city_name/is_internal/created_at/updated_at/last_active_at`。

- `background_url` 非空时固定为该用户的 `/admin/api/users/:user_id/background`，不暴露原始 OSS 地址。
- `zodiac_sign` 由服务端根据生日派生；未填写/无效日期为空串。后台选择生日时即时预览标签，保存只提交生日。边界规则见 [用户星座](user.md#星座派生规则)，图标和明暗配色复用 Android 客户端 MingCute 资源，许可随静态资源分发。
- 手机号、所在地、头像、背景图、互动数据、级别与最近活跃只读。详情分别显示“注册”“最近活跃”“资料更新”；`updated_at` 是用户资料/账号记录更新时间，修改资料、所在地或令牌版本均可能改变，不能当作最近活跃。

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
