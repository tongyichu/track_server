# AGENTS.md — track_server

> 本文面向 AI Coding Agent（Codex / Claude Code / Trae / Cursor）。
> 仅保留稳定约束、改动导航与提交前最小检查；短期数据、压测结论等不写入本文。

## 第一层（必读）

### 使用规则
- 从当前工作目录向上逐级查找 `AGENTS.md`，以最近的一份为准。
- 用 `AGENTS.md` 定位相关文件、约束、流程与验证步骤；实现细节以代码为准。
- 若 `AGENTS.md` 与代码不一致，以代码为准，并在回复中明确指出不一致点。

### 维护规则
仅当代码变更导致以下信息失准时，才更新最近的相关 `AGENTS.md`：
- 目录总览 / 模块职责
- 关键流程或调用路径
- 硬性约束、不变量、权威数据源
- 提交前验证命令

不要因"润色措辞、格式、统一风格"去修改 `AGENTS.md`。

### 硬性约束
- **数据库依赖降级**：服务启动时若连接 MySQL/Mongo 失败，会自动降级为 in-memory 仓储（见 `cmd/server/main.go:46`）。禁止新增任何在降级分支里"必须调用外部服务"的逻辑。
- **轨迹 ID 编码不可变**：`"NO." + 8 位 base36` 是全链路外部 ID（见 `internal/repository/interfaces.go:14`）。不得修改 `trackIDPrefix` / `trackIDLength` / `trackIDBase`，否则旧 ID 将无法解析。
- **OSS 文件哈希桶数**：`config.OSSFileBucketSize = 2000` 参与用户轨迹文件的目录 hash，**不可修改**（见 `internal/config/config.go:17`）。
- **静态文件路由不能直接用 `Static`**：必须用 `StaticFS + PathRewrite`，否则静态资源会 404（原因见 `internal/handler/router.go:57-91`）。
- **鉴权分组**：所有业务接口都在 `api := h.Group("/api/v1")` 下的 `auth` 子组中；公开接口只有 `/ping`、`/captcha`、`/sms/send`、`/login/*`。新增业务接口默认加到 `auth` 组，除非有明确登录豁免需求。
- **Repository 降级链约束**：MySQL / Mongo / in-memory 三种实现都必须实现 `internal/repository/interfaces.go` 里的 interface 全集；新增接口方法时，三份实现都要补齐，否则启动会编译失败或运行时 panic。
- **生成文件不手改**：`internal/config/china_city_raw.json`、`internal/config/china_province_raw.json` 为外部数据，不要逐行手工编辑。

### 目录总览
```
track_server/
├── cmd/server/             # main 入口：加载配置 → 选仓储 → 注入 service → 注册路由
├── internal/
│   ├── config/             # 环境变量加载；内置省市数据 + 昵称字典 + 同行弹幕敏感词词库
│   ├── handler/            # Hertz HTTP handler + router.go 路由表（权威）
│   ├── middleware/         # JWT 鉴权、请求元信息、Token 黑名单
│   ├── models/             # 领域模型（Track / User / Companion / Achievement / 相关光标/子结构）
│   ├── repository/         # 持久化接口 + mysql / mongo / memory 三实现
│   ├── scheduler/          # 进程内定时任务（基于 robfig/cron/v3，按 SCHEDULER_ENABLED 启停）
│   └── service/            # 业务编排：登录、轨迹、用户、同行控制面、成就、OSS STS、资源缓存
├── deploy/                 # Dockerfile / docker-compose / nginx / systemd
├── Makefile                # run / test / docker-build / compose-up/down
├── go.mod / go.sum
├── mysql.sql               # MySQL 初始化 SQL（表结构权威之一）
├── track_api.md            # 业务接口文档（接口契约权威，含同行控制面 API）
├── track_companion.md      # 同行能力技术方案设计（控制面 / MQTT 数据面规划）
├── track_achievement.md    # 轨迹成就产品/规则方案（等级、XP、勋章、会员边界）
├── track_achievement_client.md # 成就系统客户端对接文档
└── login.md                # 登录流程与协议说明
```

### 权威数据源
| 主题 | 权威文件 |
| --- | --- |
| HTTP 路由清单 | `internal/handler/router.go` |
| 配置项与默认值 | `internal/config/config.go` |
| Repository 接口契约 | `internal/repository/interfaces.go` |
| 领域模型 | `internal/models/track.go`、`internal/models/user.go`、`internal/models/achievement.go` |
| MySQL 表结构 | `mysql.sql` |
| 接口协议 | `track_api.md`、`login.md`、`track_companion.md`、`track_achievement_client.md` |
| 成就规则方案 | `track_achievement.md` |

### 关键流程

**启动流程**（`cmd/server/main.go`）：
```
Load Config → 选择 Repository（Memory/MySQL/Mongo，失败降级为 Memory）
→ 构造 Service（Track / User / Login / OSSToken / AssetCache×3）
→ 将 OSSTokenService 作为 downloader 注入 AssetCache
→ （可选）加载 TLS 证书
→ RegisterRoutes(Hertz, Deps)
→ （可选）SCHEDULER_ENABLED=true 时启动 Scheduler（注册 danmaku_cleanup 等任务）
→ h.Spin()
```

**成就结算流程**：
```
track/create(is_running=false) 或 track upload/update 完成轨迹
  → TrackService 调用 AchievementService.SettleTrackCompleted
  → AchievementService 基于有效轨迹实时聚合 XP / 进度
  → AchievementRepository 幂等写入 user_achievement_rewards
  → 客户端通过 /achievement/summary 或 /achievement/rewards 拉取展示
```

**典型业务调用链**：
```
HTTP Request
  → middleware.RequestMeta → middleware.JWTAuth（auth 组）
  → handler.<Xxx>Handler（参数校验 + 响应封装）
  → service.<Xxx>Service（业务编排 + 资源缓存下发）
  → repository.<Xxx>Repository（数据持久化）
```

**资源缓存与静态发布包**：客户端经 OSS 直传（头像 / 轨迹截图 / 原始轨迹文件），列表/详情接口返回时由 `AssetCacheService` 按需从 OSS 拉回本地 `<LogDir>/static/<category>/`，对外走 `GET /api/v1/static/<category>/<file>`（需登录）。管理后台上传的 App 发布包直接写入 `<LogDir>/static/release/<platform>/`，对外走公开的 `GET /api/v1/static/release/<platform>/<file>`，供升级下载使用。

### 提交前最小检查
- 构建可通过：`go build ./...`
- 全量测试：`make test`（等价 `go test ./...`）
- 新增/修改路由：核对 `internal/handler/router.go` 是否挂载在正确的 `auth` 分组。
- 新增 Repository 方法：`mysql.go` / `mongo.go` / `memory.go` 三实现必须同步。
- 改动与协议相关（字段增删、登录流程、错误码、同行控制面、成就系统）：同步更新 `track_api.md`、`login.md`、`track_companion.md` 或 `track_achievement_client.md`。

---

## 第二层（附录）

### 不要轻易改动的文件
| 文件 | 原因 |
| --- | --- |
| `internal/config/china_city_raw.json` / `china_province_raw.json` | 外部数据源，批量导入 |
| `internal/config/nickname.go` | 昵称词库，顺序与索引参与随机分配 |
| `internal/config/sensitive_words.json` | 同行弹幕敏感词词库，由 `go:embed` 加载，运维维护，不要逐行手改 |
| `mysql.sql` | 线上表结构基线，仅在正式变更表结构时同步修改 |

### 工程惯例
- **包命名**：所有业务代码位于 `internal/`，外部仓库不可 import。
- **错误语义**：`repository` 层只返回 `ErrNotFound` / `ErrAlreadyExists` / `ErrForbidden`（见 `interfaces.go:28`）；上层 service / handler 根据这三种语义翻译为 HTTP 状态码。
- **Context 约定**：全链路传递 `context.Context`；Handler 从 Hertz `RequestContext` 取值后传下去，不要在 service 层重新从 `app.RequestContext` 取上下文。
- **日志**：统一使用标准库 `log` + Hertz `hlog`，并通过 `cmd/server/main.go:setupLogging` 把输出同时落到 `LogDir/server.log`。
- **配置读取**：只在 `internal/config/config.go` 里读 `os.Getenv`；其他包通过 `*config.Config` 注入，不自己读环境变量。

### 常见误解
- **误解**：MySQL 启用后就不用 in-memory 了。
  **事实**：启动时若 MySQL `Ping` 失败会降级为 in-memory，所以线上必须通过日志确认实际仓储类型（搜 `using mysql repositories` / `using in-memory repositories`）。
- **误解**：可以用 `auth.Static("/static", root)` 挂载静态资源。
  **事实**：Hertz 的 `Static` 会把完整 URL 拼到 Root 下，造成 404；必须用 `StaticFS + PathRewrite`（见 `router.go:57`）。
- **误解**：STS 不配置就无法启动。
  **事实**：OSS STS 缺配置时会降级为"禁用 STS"，对应接口返回 `"oss sts not configured"`，服务仍可启动。

### 术语
| 术语 | 含义 |
| --- | --- |
| Track | 一条用户轨迹记录，外部 ID 形如 `NO.00000001` |
| Waypoint | 轨迹上的多媒体打点 |
| Collect | 用户对他人轨迹的收藏 |
| Navigation | "使用他人轨迹导航"的使用次数统计（与 Collect 独立） |
| AssetCache | 把 OSS 私有对象按需缓存到服务器本地并通过 `/api/v1/static/*` 下发的服务 |
| STS | 阿里云临时凭证，用于客户端直传 OSS |
