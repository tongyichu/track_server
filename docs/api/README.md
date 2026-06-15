# API 文档入口

> Base URL: `http://<host>:<port>/api/v1`
>
> 客户端 AI 优先读取本入口和相关分册，避免一次性加载完整协议。

## 读取建议

| 场景 | 建议读取 |
| --- | --- |
| 只需要认证、请求头、错误码 | [common.md](common.md) |
| 只需要对象字段定义 | [models.md](models.md) |
| 轨迹创建、详情、列表、搜索、更新、删除、运动类型 | [track.md](track.md) |
| 首页地图模式、路线发现、地图聚合 | [track-map.md](track-map.md) |
| 收藏、取消收藏、收藏列表、导航上报 | [collect-navigation.md](collect-navigation.md) |
| OSS 直传临时凭证 | [oss.md](oss.md) |
| 用户详情、个人信息更新、关注/粉丝关系 | [user.md](user.md) |
| App 升级检查 | [upgrade.md](upgrade.md) |
| 意见反馈提交、历史列表、图片读取、运维处理 | [feedback.md](feedback.md) |
| 成就摘要、奖励、等级规则 H5、运维刷新 | [achievement.md](achievement.md) |
| 同行控制面、会话、成员、事件、附近房间 | [companion.md](companion.md) |
| EMQX 回调、MQTT 数据面写回等内部接口 | [companion-internal.md](companion-internal.md) |
| 客户端埋点批量上报 | [analytics.md](analytics.md) |
| 客户端埋点事件、属性、隐私与验收 | [../../track_analytics.md](../../track_analytics.md) |
| 按 method/path 查找接口归属 | [route-index.md](route-index.md) |

## 维护约定

- 新增或修改接口时，同步更新对应分册和 [route-index.md](route-index.md)。
- 公共认证、请求头、错误响应只维护在 [common.md](common.md)。
- 通用对象字段优先维护在 [models.md](models.md)，分册内不要重复大段字段表。
- `/internal/*` 与 `/ops/*` 接口应明确标注内部/运维鉴权方式。
