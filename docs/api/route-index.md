# API 路由索引

> 只用于快速定位接口文档；完整请求/响应以对应分册为准。轨迹详情及摘要的 `labels` 景区文案包含可选等级，见 [统一轨迹标签](track.md#统一轨迹标签-labels)。

| 接口 | 方法 | 路径 | 需要认证 | 文档 |
| --- | --- | --- | --- | --- |
| 本人全部历史运动类型分布 | GET | `/track/activity/type-distribution` | 是 | [运动类型分布](track.md#运动类型分布) |
| 本人年度活动指标（可按运动类型筛选） | GET | `/track/activity/calendar` | 是 | [年度活动指标](track.md#年度活动指标) |
| 本人全部历史个人纪录（含轨迹预览） | GET | `/track/activity/records` | 是 | [个人纪录](track.md#个人纪录) |
| 本人全部历史探索足迹 | GET | `/track/activity/footprint` | 是 | [探索统计](track-footprint.md#全国探索统计) |
| 本人省份/城市足迹对应轨迹 | GET | `/track/activity/footprint/tracks` | 是 | [对应轨迹](track-footprint.md#对应轨迹列表) |
| 后台景点状态调整（详情删除按 archived 归档） | POST | `/admin/api/scenic-spots/:spot_id/status` | 后台景点写权限 | [track-place-discovery.md](../track-place-discovery.md#后台景点删除归档) |
| 后台景点等级编辑与详情 | GET / PUT | `/admin/api/scenic-spots/:spot_id` | 后台景点读/写权限 | [景区等级采集与下发](../track-place-discovery.md#景区等级采集与下发) |
| 存量景点资料及等级补采 | POST | `/admin/api/scenic-spots/provider-content/backfill` | 后台景点写权限 | [景区等级采集与下发](../track-place-discovery.md#景区等级采集与下发) |
| 发现景点列表（含 photos 相册、scenic_grade） | GET | `/track/discover/scenic-spots` | 是 | [track.md](track.md#发现页与年度活动图表) |
| 上报设备权限快照 | PUT | `/user/permission-snapshot` | 是 | [permission-snapshots.md](permission-snapshots.md) |
| 后台查看用户设备权限 | GET | `/admin/api/users/:user_id/permission-snapshots` | 后台 users 读取权限 | [permission-snapshots.md](permission-snapshots.md) |
| 创建轨迹（含可选默认路线说明） | POST | `/track/create` | 是 | [track.md](track.md#1-创建轨迹) |
| 推荐轨迹列表（个性化 Feed Session，支持 `city_code` 筛选） | GET | `/track/recommend/list` | 是 | [track.md](track.md#2-推荐轨迹列表) |
| 轨迹详情（含默认路线说明、收藏状态及可选主景区） | GET | `/track/:track_id/detail` | 是 | [track.md](track.md#3-轨迹详情)、[主景区标签](track.md#主景区标签) |
| 静态轨迹预览图片 | GET | `/track/:track_id/previews/:variant?v=<source_version>` | 是 | [track.md](track.md#静态轨迹预览图) |
| 历史预览待补列表 | GET | `/track/preview-backfill/list` | 是 | [track.md](track.md#历史轨迹预览补齐devmode) |
| 历史路线说明待补列表 / GPS | GET | `/track/description-backfill/list`, `/track/description-backfill/:track_id/data` | 是 | [track.md](track.md#历史路线说明补齐devmode) |
| 历史路线说明补齐 | POST | `/track/description-backfill/:track_id/complete` | 是 | [track.md](track.md#历史路线说明补齐devmode) |
| 历史预览原始 GPS | GET | `/track/preview-backfill/:track_id/data` | 是 | [track.md](track.md#历史轨迹预览补齐devmode) |
| 历史预览四图上传地址 | POST | `/track/preview-backfill/:track_id/prepare` | 是 | [track.md](track.md#历史轨迹预览补齐devmode) |
| 历史预览四图提交 | POST | `/track/preview-backfill/:track_id/complete` | 是 | [track.md](track.md#历史轨迹预览补齐devmode) |
| 清空历史预览图（全站或按轨迹 ID） | POST | `/track/preview-backfill/clear` | 是 | [track.md](track.md#历史轨迹预览补齐devmode) |
| 获取 OSS STS 临时凭证 | GET | `/oss/sts-token` | 是 | [oss.md](oss.md#4-获取-oss-sts-临时凭证直传上传) |
| 轨迹评论列表 / 发表 | GET / POST | `/track/:track_id/comments` | 是 | [track-comments.md](track-comments.md) |
| 单条评论 / 删除 | GET / DELETE | `/track/:track_id/comments/:comment_id` | 是 | [track-comments.md](track-comments.md) |
| 评论回复列表 | GET | `/track/:track_id/comments/:comment_id/replies` | 是 | [track-comments.md](track-comments.md) |
| 评论点赞人列表 | GET | `/track/:track_id/comments/:comment_id/likes` | 是 | [track-comments.md](track-comments.md) |
| 评论赞踩状态 | PUT | `/track/:track_id/comments/:comment_id/reaction` | 是 | [track-comments.md](track-comments.md) |
| 评论图片预约 | POST | `/track/:track_id/comments/images/prepare` | 是 | [track-comments.md](track-comments.md) |
| 评论图片读取 | GET | `/track/:track_id/comments/:comment_id/images/:image_id` | 是 | [track-comments.md](track-comments.md) |
| 点赞轨迹 | POST | `/track/:track_id/like` | 是 | [track-likes.md](track-likes.md) |
| 取消点赞 | DELETE | `/track/:track_id/like` | 是 | [track-likes.md](track-likes.md) |
| 轨迹点赞状态 | GET | `/track/:track_id/like/status` | 是 | [track-likes.md](track-likes.md) |
| 当前用户点赞过的轨迹列表（与收藏列表同构） | GET | `/track/liked/list` | 是 | [track-likes.md](track-likes.md) |
| 收藏轨迹（返回收藏状态及总数） | POST | `/track_collect` | 是 | [collect-navigation.md](collect-navigation.md#5-收藏轨迹) |
| 取消收藏轨迹（返回收藏状态及总数） | DELETE | `/track_collect` | 是 | [collect-navigation.md](collect-navigation.md#6-取消收藏轨迹) |
| 轨迹搜索列表（支持城市 / 运动类型 / 路线里程筛选） | GET | `/track/search/list` | 是 | [track.md](track.md#7-轨迹搜索列表) |
| 搜索筛选能力及按运动类型的里程分段 | GET | `/track/search/options` | 是 | [搜索筛选配置](track.md#搜索筛选配置) |
| 导航使用上报 | POST | `/track/:track_id/navigation/report` | 是 | [collect-navigation.md](collect-navigation.md#8-导航使用上报) |
| 我的轨迹列表（含精品标识、当前用户收藏状态及总数） | GET | `/track/my/list` | 是 | [track.md](track.md#9-我的轨迹列表) |
| 用户主页已传轨迹 | GET | `/user/:user_id/tracks` | 是 | [user.md](user.md#个人主页轨迹列表) |
| 用户主页收藏轨迹 | GET | `/user/:user_id/collections` | 是 | [user.md](user.md#个人主页轨迹列表) |
| 获取用户详情 | GET | `/user/:user_id/detail` | 是 | [user.md](user.md#10-获取用户详情) |
| 更新轨迹信息（可补充默认路线说明） | PUT | `/track/:track_id/update` | 是 | [track.md](track.md#11-更新轨迹信息) |
| 删除轨迹 | DELETE | `/track/:track_id` | 是 | [track.md](track.md#12-删除轨迹) |
| 用户已收藏轨迹列表（含精品标识、当前用户收藏状态及总数） | GET | `/track/collected/list` | 是 | [collect-navigation.md](collect-navigation.md#13-用户已收藏轨迹列表) |
| 更新个人信息（含背景图、性别、生日） | PUT | `/user/profile/update` | 是 | [user.md](user.md#14-更新个人信息) |
| 更新最近确认所在地 | PUT | `/user/profile/location` | 是 | [user.md](user.md#更新最近确认所在地) |
| 获取地区目录（支持版本检查） | GET | `/regions/catalog` | 是 | [regions.md](regions.md) |
| 关注用户 | POST | `/user/:user_id/follow` | 是 | [user.md](user.md#42-关注用户) |
| 取消关注用户 | DELETE | `/user/:user_id/follow` | 是 | [user.md](user.md#43-取消关注用户) |
| 查询关注状态 | GET | `/user/:user_id/follow/status` | 是 | [user.md](user.md#44-查询关注状态) |
| 关注列表 | GET | `/user/:user_id/following/list` | 是 | [user.md](user.md#45-关注列表) |
| 粉丝列表 | GET | `/user/:user_id/follower/list` | 是 | [user.md](user.md#46-粉丝列表) |
| 批量上报埋点事件 | POST | `/analytics/events` | 条件（登录用户事件） | [analytics.md](analytics.md#47-批量上报埋点事件) |
| App 升级检查 | GET | `/upgrade/check` | 否 | [upgrade.md](upgrade.md#15-app-升级检查) |
| 运维发布 Android App | POST | `/ops/releases/android` | 运维内部 | [upgrade.md](upgrade.md#16-运维发布-android-app) |
| 隐私政策 H5 | GET | `/legal/privacy-policy.html` | 否 | [legal.md](legal.md) |
| 用户服务协议 H5 | GET | `/legal/terms-of-service.html` | 否 | [legal.md](legal.md) |
| 获取运动类型（含中英文名称及日间/夜间主题色） | GET | `/track/types` | 是 | [track.md](track.md#16-获取运动类型) |
| 获取内置 MingCute 运动图标 | GET / HEAD | `/track/types/icons/:icon_file` | 是 | [track.md](track.md#16-获取运动类型) |
| 获取轨迹投稿选项 | GET | `/track/submission/options` | 是 | [track-submission.md](track-submission.md#1-投稿选项) |
| 创建或重新投稿 | POST | `/track/:track_id/submission` | 是 | [track-submission.md](track-submission.md#2-创建或重新投稿) |
| 修改待审核投稿 | PUT | `/track/:track_id/submission` | 是 | [track-submission.md](track-submission.md#3-修改待审核投稿) |
| 查询轨迹投稿 | GET | `/track/:track_id/submission` | 是 | [track-submission.md](track-submission.md#4-查询投稿) |
| 撤回轨迹投稿 | POST | `/track/:track_id/submission/withdraw` | 是 | [track-submission.md](track-submission.md#5-撤回投稿) |
| 查询地图视野数据（含离线区域语义，支持 `track_type=all`） | GET | `/track-map/view` | 是 | [track-map.md](track-map.md#48-查询地图视野数据) |
| 查询地图路线组列表（支持 `track_type=all`） | GET | `/track-map/groups` | 是 | [track-map.md](track-map.md#49-查询地图路线组列表) |
| 查询路线组详情 | GET | `/track-map/groups/:group_id/detail` | 是 | [track-map.md](track-map.md#50-查询路线组详情) |
| 查询路线组下的具体轨迹列表 | GET | `/track-map/groups/:group_id/tracks` | 是 | [track-map.md](track-map.md#51-查询路线组下的具体轨迹列表) |
| 查看地图区域介绍页 | GET | `/track-map/areas/:area_id/introduction.html` | 否 | [track-map.md](track-map.md#52-查看地图区域介绍页) |
| 查看聚合路线介绍页 | GET | `/track-map/groups/:group_id/introduction.html` | 否 | [track-map.md](track-map.md#53-查看聚合路线介绍页) |
| 创建同行会话 | POST | `/companion/session/create` | 是 | [companion.md](companion.md#17-创建同行会话) |
| 加入同行会话 | POST | `/companion/session/join` | 是 | [companion.md](companion.md#18-加入同行会话) |
| 预览同行会话 | GET | `/companion/session/preview` | 是 | [companion.md](companion.md#181-预览同行会话) |
| 获取当前同行会话 | GET | `/companion/session/current` | 是 | [companion.md](companion.md#19-获取当前同行会话) |
| 获取同行快照 | GET | `/companion/session/:session_id/snapshot` | 是 | [companion.md](companion.md#20-获取同行快照) |
| 离开同行会话 | POST | `/companion/session/:session_id/leave` | 是 | [companion.md](companion.md#21-离开同行会话) |
| 结束同行会话 | POST | `/companion/session/:session_id/end` | 是 | [companion.md](companion.md#22-结束同行会话) |
| 踢出同行成员 | POST | `/companion/session/:session_id/members/:user_id/kick` | 是 | [companion.md](companion.md#221-踢出同行成员) |
| 更新已结束同行摘要 | PUT | `/companion/session/:session_id/update` | 是 | [companion.md](companion.md#222-更新已结束同行摘要owner) |
| 上报同行关键事件 | POST | `/companion/session/:session_id/events` | 是 | [companion.md](companion.md#223-上报同行关键事件owner) |
| 查询同行关键事件时间线 | GET | `/companion/session/:session_id/events` | 是 | [companion.md](companion.md#224-查询同行关键事件时间线owner) |
| 当前用户参与过的同行记录列表 | GET | `/companion/session/history` | 是 | [companion.md](companion.md#23-当前用户参与过的同行记录列表) |
| 获取同行 MQTT 凭证 | POST | `/companion/session/:session_id/mqtt/credentials` | 是 | [companion.md](companion.md#24-获取同行-mqtt-凭证) |
| EMQX HTTP AuthN 回调 | POST | `/internal/mqtt/auth` | 内部 | [companion-internal.md](companion-internal.md#25-emqx-http-authn-回调) |
| EMQX HTTP AuthZ 回调 | POST | `/internal/mqtt/acl` | 内部 | [companion-internal.md](companion-internal.md#26-emqx-http-authz-回调) |
| EMQX 数据面写回接口 | POST | `/internal/companion/mqtt/*` | 内部 | [companion-internal.md](companion-internal.md#27-emqx-数据面写回接口) |
| 同行弹幕开关切换 | POST | `/companion/session/:session_id/danmaku/toggle` | 是 | [companion.md](companion.md#274-弹幕开关切换owner) |
| 附近 active 同行房间列表 | GET | `/companion/session/nearby` | 是 | [companion.md](companion.md#29-附近-active-同行房间列表) |
| 成就中心摘要 | GET | `/achievement/summary` | 是 | [achievement.md](achievement.md#30-成就中心摘要) |
| 成就奖励列表 | GET | `/achievement/rewards` | 是 | [achievement.md](achievement.md#31-成就奖励列表) |
| 成长等级规则 H5 | GET | `/achievement/level-rules.html` | 否 | [achievement.md](achievement.md#32-成长等级规则-h5) |
| 运维刷新用户成就 | POST | `/ops/achievement/refresh` | 运维内部 | [achievement.md](achievement.md#33-运维刷新用户成就) |
| 创建账号限制 | POST | `/ops/users/:user_id/restrictions` | 运维内部 | [account-restriction.md](account-restriction.md#52-创建账号限制) |
| 查询当前账号限制 | GET | `/ops/users/:user_id/restrictions/current` | 运维内部 | [account-restriction.md](account-restriction.md#53-查询当前账号限制) |
| 查询账号限制历史 | GET | `/ops/users/:user_id/restrictions/history` | 运维内部 | [account-restriction.md](account-restriction.md#54-查询账号限制历史) |
| 解除当前账号限制 | DELETE | `/ops/users/:user_id/restrictions/current` | 运维内部 | [account-restriction.md](account-restriction.md#55-解除当前账号限制) |
| 提交意见反馈 | POST | `/feedback` | 是 | [feedback.md](feedback.md#34-提交意见反馈) |
| 获取纠错选项 | GET | `/feedback/correction-options` | 是 | [feedback.md](feedback.md#纠错选项) |
| 我的反馈列表 | GET | `/feedback/list` | 是 | [feedback.md](feedback.md#35-我的反馈列表) |
| 我的反馈详情 | GET | `/feedback/:feedback_id` | 是 | [feedback.md](feedback.md#36-我的反馈详情) |
| 读取我的反馈图片 | GET | `/feedback/:feedback_id/images/:image_id` | 是 | [feedback.md](feedback.md#37-读取我的反馈图片) |
| 运维反馈列表 | GET | `/ops/feedback/list` | 运维内部 | [feedback.md](feedback.md#38-运维反馈列表) |
| 运维反馈详情 | GET | `/ops/feedback/:feedback_id` | 运维内部 | [feedback.md](feedback.md#39-运维反馈详情) |
| 运维更新反馈状态 | PUT | `/ops/feedback/:feedback_id/status` | 运维内部 | [feedback.md](feedback.md#40-运维更新反馈状态) |
| 运维读取反馈图片 | GET | `/ops/feedback/:feedback_id/images/:image_id` | 运维内部 | [feedback.md](feedback.md#41-运维读取反馈图片) |

个人信息字段扩展：`GET /user/:user_id/detail` 与 `PUT /user/profile/update` 支持背景图、性别、生日；性别、生日和服务端派生的 `zodiac_sign` 仅本人详情返回。登录 `user` 与资料更新响应也带星座，无新路由或星座存储字段，详见 [用户接口](user.md)。路由和鉴权分组保持不变。


## 管理后台（独立 `/admin` 路径）

以下使用后台 session，不属于 `/api/v1`。

| 接口 | 方法 | 完整路径 | 需要认证 | 文档 |
| --- | --- | --- | --- | --- |
| 后台账号列表与页面目录 | GET | `/admin/api/accounts` | 管理员 session | [admin-accounts.md](admin-accounts.md) |
| 后台读取/保存地区目录 | GET / PUT | `/admin/api/regions/catalog` | 地区查看 / 操作 | [regions.md](regions.md) |
| 后台保存单个地区节点 | PUT | `/admin/api/regions/node` | 地区操作 | [regions.md](regions.md) |
| 联网查询单个地区资料 | POST | `/admin/api/regions/research` | 地区操作 | [regions.md](regions.md) |
| 上传省份/城市图片文件（保存节点时关联） | POST | `/admin/api/regions/:node_id/photos` | 地区操作 | [regions.md](regions.md) |
| 人工查询3张Pixabay图片候选 | POST | `/admin/api/regions/photos/pixabay/search` | 地区操作 | [regions.md](regions.md#pixabay-人工选图) |
| 导入Pixabay图片文件（保存节点时关联） | POST | `/admin/api/regions/:node_id/photos/pixabay/import` | 地区操作 | [regions.md](regions.md#pixabay-人工选图) |
| 上传景点图片文件 | POST | `/admin/api/scenic-spots/:spot_id/photos` | 景点操作 | 可选 `draft=1` 仅暂存；省略时追加并保存，见 [后台路由](../admin-routes.md) |
| 新增后台账号 | POST | `/admin/api/accounts` | 管理员 session | [admin-accounts.md](admin-accounts.md) |
| 调整身份、状态与页面权限 | PUT | `/admin/api/accounts/:username` | 管理员 session | [admin-accounts.md](admin-accounts.md) |
| 迁移期应急重置后台密码 | POST | `/admin/api/accounts/:username/reset-password` | 管理员 session，且目标未验证邮箱/旧入口未关闭 | [admin-accounts.md](admin-accounts.md) |
| 修改本人后台密码 | POST | `/admin/api/password` | 后台 session | [admin-accounts.md](admin-accounts.md) |
| 本人找回邮箱状态 | GET | `/admin/api/security` | 本人后台 session | [admin-accounts.md](admin-accounts.md) |
| 原邮箱验证码 | POST | `/admin/api/security/email/current-code` | 本人后台 session | [admin-accounts.md](admin-accounts.md) |
| 新邮箱验证码 | POST | `/admin/api/security/email/send-code` | 本人后台 session | [admin-accounts.md](admin-accounts.md) |
| 验证并保存本人邮箱 | POST | `/admin/api/security/email/verify` | 本人后台 session | [admin-accounts.md](admin-accounts.md) |
| 邮件找回能力 | GET | `/admin/api/password-recovery/status` | 否 | [admin-accounts.md](admin-accounts.md) |
| 申请邮件找回 | POST | `/admin/api/password-recovery/request` | 否 | [admin-accounts.md](admin-accounts.md) |
| 验证邮件并重新设置密码 | POST | `/admin/api/password-recovery/complete` | 否 | [admin-accounts.md](admin-accounts.md) |
| 最近后台操作日志（可选 username 精确筛选） | GET | `/admin/api/audit-logs` | 管理员 session | [admin-accounts.md](admin-accounts.md) |
| 后台轨迹列表（含 missing 缺项筛选） | GET | `/admin/api/tracks` | 轨迹查看 | [admin-tracks.md](admin-tracks.md) |
| 后台轨迹类型与贡献用户前十（天/周/月） | GET | `/admin/api/tracks/statistics` | 轨迹查看 | [admin-tracks.md](admin-tracks.md#轨迹类型与用户贡献统计) |
| 后台轨迹坐标系预览 | GET | `/admin/api/tracks/:track_id/map-preview` | 轨迹查看 | [admin-tracks.md](admin-tracks.md#编辑字段与坐标预览) |
| 后台批量导入轨迹（逐文件结果含轨迹表格摘要） | POST | `/admin/api/tracks/import` | 轨迹操作 | [admin-tracks.md](admin-tracks.md) |
| 后台轨迹详情（含只读归属用户名） | GET | `/admin/api/tracks/:track_id` | 轨迹查看 | [admin-tracks.md](admin-tracks.md) |
| 后台轨迹图片与原始文件下载 | GET | `/admin/api/tracks/:track_id/assets/:asset` | 轨迹查看 | [admin-tracks.md](admin-tracks.md#详情资源展示与下载) |
| 单条轨迹重新识别 | POST | `/admin/api/tracks/:track_id/place-refresh` | 轨迹操作 | [admin-tracks.md](admin-tracks.md) |
| 轨迹地图高德配置 / 服务代理 | GET | `/admin/api/tracks/map-config`、`/admin/api/tracks/amap-service` | 轨迹查看 | [admin-tracks.md](admin-tracks.md) |
| 单条轨迹景点候选 | GET | `/admin/api/tracks/:track_id/place-candidates` | 轨迹查看 | [admin-tracks.md](admin-tracks.md) |
| 轨迹景点候选队列（兼容） | GET | `/admin/api/place-candidates` | 轨迹查看 | [admin-tracks.md](admin-tracks.md) |
| 确认轨迹景点候选 | POST | `/admin/api/place-candidates/:track_id/confirm` | 轨迹操作 | [admin-tracks.md](admin-tracks.md) |
| 忽略轨迹景点候选 | POST | `/admin/api/place-candidates/:track_id/dismiss` | 轨迹操作 | [admin-tracks.md](admin-tracks.md) |

纠错协议扩展（现有路由）：`GET /feedback/correction-options` 与后台配置接口支持双语问题解释/填写提示；`GET /admin/api/feedbacks` 支持 kind/track_id/correction_option_id 仓储筛选；后台反馈详情支持 handled_by/track_change，状态更新支持 outcome/revision；`PUT /admin/api/tracks/:track_id` 可选 source_feedback_id。详见 [feedback.md](feedback.md#纠错体验与处理结果扩展)，权限仍由既有路由映射及跨模块校验执行。

后台地域筛选：`GET /admin/api/users`（用户查看）支持 `province_code`；`GET /admin/api/scenic-spots`（景点查看）支持 `province_code` / `city_code`，见 [admin-regions.md](admin-regions.md)。
- `POST /admin/api/tracks/:track_id/labels/generate`：轨迹写权限，AI 形象建议，不直接保存，见 admin-tracks.md。
