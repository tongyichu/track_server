# API 路由索引

> 只用于快速定位接口文档；完整请求/响应以对应分册为准。

| 接口 | 方法 | 路径 | 需要认证 | 文档 |
| --- | --- | --- | --- | --- |
| 创建轨迹 | POST | `/track/create` | 是 | [track.md](track.md#1-创建轨迹) |
| 推荐轨迹列表 | GET | `/track/recommend/list` | 是 | [track.md](track.md#2-推荐轨迹列表) |
| 轨迹详情 | GET | `/track/:track_id/detail` | 是 | [track.md](track.md#3-轨迹详情) |
| 获取 OSS STS 临时凭证 | GET | `/oss/sts-token` | 是 | [oss.md](oss.md#4-获取-oss-sts-临时凭证直传上传) |
| 收藏轨迹 | POST | `/track_collect` | 是 | [collect-navigation.md](collect-navigation.md#5-收藏轨迹) |
| 取消收藏轨迹 | DELETE | `/track_collect` | 是 | [collect-navigation.md](collect-navigation.md#6-取消收藏轨迹) |
| 轨迹搜索列表 | GET | `/track/search/list` | 是 | [track.md](track.md#7-轨迹搜索列表) |
| 导航使用上报 | POST | `/track/:track_id/navigation/report` | 是 | [collect-navigation.md](collect-navigation.md#8-导航使用上报) |
| 我的轨迹列表 | GET | `/track/my/list` | 是 | [track.md](track.md#9-我的轨迹列表) |
| 获取用户详情 | GET | `/user/:user_id/detail` | 是 | [user.md](user.md#10-获取用户详情) |
| 更新轨迹信息 | PUT | `/track/:track_id/update` | 是 | [track.md](track.md#11-更新轨迹信息) |
| 删除轨迹 | DELETE | `/track/:track_id` | 是 | [track.md](track.md#12-删除轨迹) |
| 用户已收藏轨迹列表 | GET | `/track/collected/list` | 是 | [collect-navigation.md](collect-navigation.md#13-用户已收藏轨迹列表) |
| 更新个人信息 | PUT | `/user/profile/update` | 是 | [user.md](user.md#14-更新个人信息) |
| 关注用户 | POST | `/user/:user_id/follow` | 是 | [user.md](user.md#42-关注用户) |
| 取消关注用户 | DELETE | `/user/:user_id/follow` | 是 | [user.md](user.md#43-取消关注用户) |
| 查询关注状态 | GET | `/user/:user_id/follow/status` | 是 | [user.md](user.md#44-查询关注状态) |
| 关注列表 | GET | `/user/:user_id/following/list` | 是 | [user.md](user.md#45-关注列表) |
| 粉丝列表 | GET | `/user/:user_id/follower/list` | 是 | [user.md](user.md#46-粉丝列表) |
| 批量上报埋点事件 | POST | `/analytics/events` | 否 | [analytics.md](analytics.md#47-批量上报埋点事件) |
| App 升级检查 | GET | `/upgrade/check` | 否 | [upgrade.md](upgrade.md#15-app-升级检查) |
| 获取运动类型 | GET | `/track/types` | 是 | [track.md](track.md#16-获取运动类型) |
| 查询地图视野数据 | GET | `/track-map/view` | 是 | [track-map.md](track-map.md#48-查询地图视野数据) |
| 查询地图路线组列表 | GET | `/track-map/groups` | 是 | [track-map.md](track-map.md#49-查询地图路线组列表) |
| 查询路线组详情 | GET | `/track-map/groups/:group_id/detail` | 是 | [track-map.md](track-map.md#50-查询路线组详情) |
| 查询路线组下的具体轨迹列表 | GET | `/track-map/groups/:group_id/tracks` | 是 | [track-map.md](track-map.md#51-查询路线组下的具体轨迹列表) |
| 查看地图区域介绍页 | GET | `/track-map/areas/:area_id/introduction.html` | 否 | [track-map.md](track-map.md#52-查看地图区域介绍页) |
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
| 我的反馈列表 | GET | `/feedback/list` | 是 | [feedback.md](feedback.md#35-我的反馈列表) |
| 我的反馈详情 | GET | `/feedback/:feedback_id` | 是 | [feedback.md](feedback.md#36-我的反馈详情) |
| 读取我的反馈图片 | GET | `/feedback/:feedback_id/images/:image_id` | 是 | [feedback.md](feedback.md#37-读取我的反馈图片) |
| 运维反馈列表 | GET | `/ops/feedback/list` | 运维内部 | [feedback.md](feedback.md#38-运维反馈列表) |
| 运维反馈详情 | GET | `/ops/feedback/:feedback_id` | 运维内部 | [feedback.md](feedback.md#39-运维反馈详情) |
| 运维更新反馈状态 | PUT | `/ops/feedback/:feedback_id/status` | 运维内部 | [feedback.md](feedback.md#40-运维更新反馈状态) |
| 运维读取反馈图片 | GET | `/ops/feedback/:feedback_id/images/:image_id` | 运维内部 | [feedback.md](feedback.md#41-运维读取反馈图片) |
