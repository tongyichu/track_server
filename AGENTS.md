# AGENTS.md — track_server

<!-- Last updated: 2026-11-10T21:08:38+08:00 -->
<!-- Last verified: 2026-11-04T16:28:34+08:00 -->

**Precedence:** 用户当前明确指令 > 最近的 AGENTS.md > 父级 AGENTS.md > README/参考文档。
默认中文回复。用本文件定位约束与入口，实现细节以源码为准；发现文档与代码不一致时指出并校正。
Go/Hertz 轨迹服务，调用链为 middleware → handler → service → repository，支持 MySQL/Mongo/in-memory。

## 必守边界

- 业务仓储启动连接失败可降级内存；降级分支不得新增强制外部调用。线上通过启动日志确认实际仓储。后台账号、权限快照等独立初始化失败策略按对应文档执行，不套用业务降级。
- 新增 Repository 方法同步 MySQL/Mongo/memory 三实现，保持持久化、游标、总数和错误语义一致；筛选在分页/总数统计前执行，列表关联按页批量读取，禁止逐项补详情。
- 外部轨迹 ID 固定 NO. + 8 位 base36；不得更改 trackIDPrefix/Length/Base。OSSFileBucketSize=2000 不可修改。
- 新业务接口默认加入业务 JWT auth 组；公开豁免以 internal/handler/router.go 为准。内部/运维接口用内部 token，admin 使用独立 session。
- 新增写内容/关系/资料接口时检查 AccountRestrictionMiddleware；查询及取消关系的放行语义见账号限制协议。业务用户 API 的私有字段只向本人返回，is_internal 不进入 App 用户响应；后台管理查询按独立权限执行。
- 静态资源用 StaticFS + PathRewrite，禁止直接 Static。反馈/评论/预览等私有资源保留各自归属、可见性与源版本鉴权，不混入通用静态目录。
- 环境变量只在 internal/config/config.go 读取后注入；全链路传 context.Context。日志复用 log/hlog；仓储错误使用已有 ErrNotFound/ErrAlreadyExists/ErrForbidden，由上层翻译 HTTP 状态。
- 时间先区分时刻、日历日期和时长：时刻按带时区 RFC3339 或明确单位的时间戳传输，展示显式选择业务时区，禁止截断时区或改数据库历史值补偿显示；生日/日期键和时长不套时刻转换。后台固定北京时间，见 [后台规则](internal/admin/AGENTS.md) 与 [时间约定](docs/admin-ui.md#时间与时区)。
- 原始 GPS 声明、索引 GCJ02 与地图坐标分清，复用 maparea 转换；不改原始文件。自动空间索引/景点识别由后台任务执行，业务地图/列表查询不调用供应商；后台人工同步刷新、来源预览与核验沿用专用接口及其锁/配额规则。后台 raw track 与埋点 OSS 同步必须用内网 Endpoint，不回退公网。
- 地区目录以 region_catalog_state 为权威，seed 仅首次导入；业务城市编号使用稳定 `region_id`，旧树键/高德 adcode 按目录兼容，外部标识按来源和类型映射，不当作通用编号，细节见 docs/api/regions.md。运动类型以 DefaultTrackTypeConfigs 和稳定英文 code 为权威。all 仅为地图查询哨兵，不写存储或跨类型合并路线组。
- 保持 RouteGroup 历史 ID、人工名称/代表及路线介绍 anchor_track_id；重聚合不得覆盖人工资料。labels 是既有权威字段的只读投影，不新建标签表或触发 AI/供应商请求。景区等级仅取明确 A 级来源，scenic_spot.name 由 API 组装“景点名·5A”，客户端不得重复拼接；采集边界见景点识别文档。
- 自动说明/形象写回须保护人工内容，复用去重、租约和预算。埋点脱敏、事件发生时身份与整批确认语义不可改变；导航非幂等上报不得自动重试。
- 敏感配置/密钥不入库；地区边界、词库按既有生成/维护流程更新，不逐条手改；mysql.sql 随正式表结构变化同步。

## 入口与按需参考

修改业务前先读下表相关文档的对应章节，无需通读全部参考。

| 范围 | 权威入口与开发参考 |
|---|---|
| 路由、配置、接口、表结构 | internal/handler/router.go、internal/config/config.go、internal/repository/interfaces.go、mysql.sql；[API 索引](docs/api/README.md)、[路由索引](docs/api/route-index.md) |
| 本人运动指标、类型分布及步数保存 | [日历实现](internal/service/track_activity_calendar.go)、[指标仓储](internal/repository/track_activity_calendar.go)、[步数schema](internal/repository/track_step_count.go)；[年度活动指标](docs/api/track.md#年度活动指标)、[运动类型分布](docs/api/track.md#运动类型分布) |
| 本人全部历史个人纪录 | [纪录实现](internal/service/track_personal_records.go)、[三种仓储](internal/repository/track_personal_records.go)；[个人纪录协议](docs/api/track.md#个人纪录) |
| 本人全国探索足迹、地区/景区去重及轨迹下钻 | [足迹协议](docs/api/track-footprint.md)、[实现](internal/service/track_footprint.go)、[轻量仓储](internal/repository/track_footprint.go) |
| 轨迹/用户/发现/日历/互动、元数据、各业务默认值和数据边界 | [服务端契约](docs/server-contracts.md)，再查 docs/api/ 对应业务分册 |
| 登录、成就、投稿、同行、限制、反馈、静态资源、四图补齐、H5、纠错及后台导入调用链 | [服务端流程](docs/server-flows.md) |
| 地图索引/路线组/景点/坐标 | [契约](docs/server-contracts.md)、[索引流程](docs/server-flows.md#首页地图索引流程)、[地图方案](track_map.md)、[景点识别](docs/track-place-discovery.md)、[边界维护](internal/maparea/README.md) |
| 推荐/归因/埋点/离线任务 | [推荐方案](track_recommendation.md)、[埋点方案](track_analytics.md)、[契约](docs/server-contracts.md)、[流程](docs/server-flows.md) |
| AI 路线说明与形象反馈 | [运行边界](docs/track-narrative.md)、[形象技能](skills/track-shape-label/SKILL.md)；共享规则调整同步版本，不用独立提示词脚本替代真实接口验收 |
| 消息中心、私信、未读、活动通知与独立MQTT | [消息协议](docs/api/messages.md)、[持久化与生命周期](docs/messages.md)；消息存储不可用时拒绝确认发送，不套用易失内存降级 |
| 管理后台 | 先读 [internal/admin/AGENTS.md](internal/admin/AGENTS.md) |
| 存量标题回填 | [操作说明](docs/backfill-track-titles.md)，默认只生成计划，应用需原值条件匹配，不自动建表或降级内存 |
| 历史任务恢复 | [HANDOFF](docs/handoff/HANDOFF.md) |

## 验证与交付

- 默认由用户手动验证功能；不主动新增或运行单元/集成/race/接口/全量测试，不为验证启动数据库、容器或额外环境。仅用户明确要求时执行指定范围，不因旧规则或技能建议扩大。
- 代码改动做必要构建 go build ./... 与自查；纯文档/规则修改无需业务构建。保留已有测试，仅为本次接口/编译失配做必要修正。
- 路由变化检查鉴权与账号限制；仓储变化检查三实现；协议变化同步 docs/api/ 对应分册和 route-index，并更新相关业务方案。只报告实际运行结果，功能待用户验证。
- 用户要求提交时使用中文 commit message，说明做了什么、原因、影响范围及协议/数据/文档/测试变化。

## 发布与授权

- 发布 App/部署 Server 通过同级 Android 仓库 [android-track-release](../android-track/.agents/skills/android-track-release/SKILL.md)：可同步、提交、推送实际发布仓库，Server 先推送再构建镜像和生产更新；仅部署 Server 不提交客户端。
- 仅修改技能不触发发布、部署或 Git 写操作。敏感配置、签名、产物和缓存不入库；分别核对 Git 和生产结果，不把已推送等同部署成功。
- 发布范围外的 Git 写操作及不可逆破坏性操作需要当前明确授权，已授权步骤不重复询问。

## 规则维护

- 新增或改变模块、路由、仓储、数据结构、协议、任务、外部依赖或配置时，检查最近 AGENTS 的入口与边界是否失准；若无须改动，交付时简述理由。
- 根 AGENTS 只保留跨模块不变量、验证/授权与查阅索引；业务细节更新对应参考/API 文档，局部规则归入 scoped AGENTS，避免逐次追加完整功能描述。
- 普通任务不为润色改规则；用户明确要求的规则维护可执行。不得静默改变既有权限、默认验证或业务契约。
