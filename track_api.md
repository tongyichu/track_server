# 轨迹 App API 文档入口

> 完整 API 文档已拆分到 [docs/api/](docs/api/)。
>
> 客户端 AI 不应再一次性读取历史大文档；请先读 [docs/api/README.md](docs/api/README.md)，再按接口读取对应分册。

## 快速入口

| 场景 | 文档 |
| --- | --- |
| 认证、公共请求头、公共错误 | [docs/api/common.md](docs/api/common.md) |
| 通用对象字段 | [docs/api/models.md](docs/api/models.md) |
| 按 method/path 查接口位置 | [docs/api/route-index.md](docs/api/route-index.md) |
| 轨迹创建、列表、详情、更新、删除、运动类型 | [docs/api/track.md](docs/api/track.md) |
| 收藏、取消收藏、导航上报 | [docs/api/collect-navigation.md](docs/api/collect-navigation.md) |
| OSS 上传临时凭证 | [docs/api/oss.md](docs/api/oss.md) |
| 用户资料接口 | [docs/api/user.md](docs/api/user.md) |
| App 升级检查 | [docs/api/upgrade.md](docs/api/upgrade.md) |
| 成就接口 | [docs/api/achievement.md](docs/api/achievement.md) |
| 同行接口 | [docs/api/companion.md](docs/api/companion.md) |
| 同行内部 MQTT/EMQX 接口 | [docs/api/companion-internal.md](docs/api/companion-internal.md) |
| 客户端埋点方案 | [track_analytics.md](track_analytics.md) |

## 维护规则

- 新增或修改接口时，同步更新对应分册和 [docs/api/route-index.md](docs/api/route-index.md)。
- 公共约定只维护在 [docs/api/common.md](docs/api/common.md)。
- 通用对象字段优先维护在 [docs/api/models.md](docs/api/models.md)。
