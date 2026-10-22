# 工作交接：协议 H5 页面（2026-10-13）

## 2026-10-22：运动类型英文展示名

- /track/types 增加 name_en，DefaultTrackTypeConfigs 的 NameEN 为 Hiking/Running/Climbing/Cycling/Driving；保留原 name、type、图标、颜色与统计，不做按请求语言覆盖 name，避免账号缓存跨语言污染。
- Android 使用当前 App 语言选择双语字段，旧缓存缺英文字段时用本地英文资源兜底；未知自定义类型 name_en 为空，原配置名保留。不增加数据库表、列或环境配置项。
- 协议、通用模型说明、路由索引与 AGENTS 已同步；本轮为代码修复，尚未提交推送或部署，不执行自动化测试。
- 验证：go build ./... 与两仓 git diff --check 通过；Android assembleDebug 通过并安装启动小米。未运行自动化测试；AGENTS 检查仍报告既有超长与元信息问题，未扩大本轮修复范围。

### 英文名称的 Server 部署请求

- 用户要求部署 Server，并已完成本次交互式 SSH 登录；仅同步提交推送本仓库的 name_en 字段、内置英文名称及协议文档，不发布或提交客户端。
- 部署前 git fetch origin、go build ./... 与差异自查通过；从已推送快照构建 linux/amd64 镜像后更新生产，核对镜像、MySQL、容器与内外 ping；本次镜像/容器/提交及最终结果以部署回执为准。

## 2026-10-22：DevMode 清空历史四图

- POST /api/v1/track/preview-backfill/clear 使用现有业务 JWT，无新增管理员或作者验证；请求显式 all=true 或 track_id 二选一。空作用域/冲突返回 400，单条不存在或已删除 404；返回 affected_count（重置轨迹数，包含原本无图记录）。沿用上传账号限制。
- 清空所有未删除记录或指定记录的四图地址；仅改 preview_images/updated_at，保留轨迹与 GPS/旧截图/OSS 文件/服务端图片缓存。无图返回空对象，已结束且有原始 GPS 的记录重新进入待补列表。
- TrackRepository.ClearPreviewImages 补齐 MySQL/Mongo/内存三实现；随机清空版本放在现有 JSON/BSON 内，不对业务 JSON 暴露，不增表/列。source_version 和 FillMissingPreviewImages 的 CAS 比较包含清空版本，阻止清空前任务重新提交旧图；成功补图保留版本以支持幂等重试。
- Android DevMode 确认后调用，成功重置本机进度与专用缓存、重新查询数量并清除列表预览地址；详情位置标签上方可复制真实 track_id。不会自动补图。
- go build ./... 通过；默认不新增或运行自动化测试，不执行线上清空/补图。本轮尚未提交推送或部署，新清空接口需要后续部署。API、路由索引、账号限制及 AGENTS 已同步；治理检查保留既有超长/元信息告警。

### 清空接口的 Server 部署请求

- 用户要求部署 Server，发布范围为上述清空接口、内部清空版本、三仓储 CAS 与协议文档；不提交客户端，不执行清空或补图操作。
- 部署前 git fetch origin 确认 main 与 origin/main 一致，go build ./... 与 git diff --check 通过；按发布技能提交推送此快照后构建 linux/amd64 镜像，使用用户本次新登录的 SSH master 更新生产，最终镜像、容器与健康检查结果以部署回执为准。

## 2026-10-22：DevMode 历史轨迹四图补齐

- 用户决定由 Android DevMode 手动生成全站历史轨迹四图，沿用业务 JWT，不增加管理员、作者或二次验证。`/track/preview-backfill/list|:track_id/data|:track_id/prepare|:track_id/complete` 均在 auth 组；prepare/complete 沿用上传账号限制。
- `internal/service/track_preview_backfill.go` 编排筛选、完整 GPS 和幂等补齐；`track_preview_backfill_oss.go` 为对应作者目录的独立 upload_id 四个对象签发 PUT URL，并经 OSS 内网核对 JPEG、大小、1024×1024/1024×768。客户端生成图，不使用服务器绘图。
- TrackRepository 新增待补分页、待补/无原始文件计数、条件写四图，三实现位于 `internal/repository/track_preview_backfill.go`。CAS 比较源地址、坐标系、运动类型和旧四图，只更新 preview_images/updated_at；已有完整四图直接跳过。无新表或字段，分页以 start_time/id 倒序，完成状态直接以四图为准。
- 协议见 docs/api/track.md，路由和限制索引已同步。Android 有账号隔离的持久化队列、失败记录与逐图上传断点；后台暂停后手动继续。
- 本轮未提交/推送/部署，也未执行线上批处理；需部署服务端后由手机 DevMode 手动启动。默认只做构建与自查，不新增或运行自动化测试。
- 最终 `go build ./...` 通过，两仓 `git diff --check` 通过；Android assembleDebug 通过并安装、启动小米。AGENTS 验证仍有原有的 root 超长/缺优先级说明、既有 Go 源码超 1000 行及全局入口为空等问题，freshness 缺时间元数据；未扩大本轮范围，未运行自动化测试或生产数据写入。

### 随后的 Server 部署请求

- 用户要求单独部署 Server，按发布技能同步、提交并推送本仓库，然后构建 linux/amd64 镜像更新生产；不提交或发布客户端，也不自动执行补图任务。
- 发布前 `git fetch origin` 确认 main 与 origin/main 同步；本次发布包括上述完整历史预览补齐接口、三仓储实现、专用签名上传、条件提交及文档。自查修正省略 source_version 查询时的原始 GPS 缓存键，始终使用实际源版本，避免取回旧文件。
- 必要的 `go build ./...` 与 `git diff --check` 通过；沿用用户本次新登录的 SSH master（进程启动于 2026-10-22 15:22:18），不读取或保存密码。镜像推送、线上容器与内外 ping 的最终结果以本次部署回执为准。

用户要求为 android-track 关于我们页的隐私政策、用户服务协议提供 track_server HTML 页面，沿用成长等级规则页风格、支持日间和暗夜；用户明确“不清楚的地方就先用 {xxxx} 这种占位”。

已完成：

- 增加公开的 `/api/v1/legal/privacy-policy.html`、`/api/v1/legal/terms-of-service.html`，通过 `legal_handler.go` 与 `go:embed` 返回自包含 HTML。
- 中文 / 英文正文、章节目录、协议互链；`lang=english` 切英文，`is_dark=true/false` 指定主题，省略时跟随系统。互链保留参数。
- 内容按当前轨迹录制、云端摘要、公开轨迹、同行、第三方地图 / 登录 / 条码 / 存储和自建统计整理；运营主体等未知事实保留占位，页面明确为草稿。真实授权、注销、清理、安全及跨境流程的待落实项见 `docs/api/legal.md`，页面不代表这些功能已经实现或合规认证。
- android-track 的 `AboutActivity.kt` 两个按钮接入现有 WebView，复用 API 主机和实际 App 主题。未修改 Android AGENTS 文件。
- 服务端 AGENTS 只为本次新路由补充地址及 H5 维护说明。用户询问“你咋把 agents 里的内容删了”：已解释整行替换在 diff 中表现为删除/新增；撤回了顺手扩充的其他接口说明，原有规则内容保留。

已验证：

- `GOCACHE=/private/tmp/track-legal-go-cache go test ./...` 全量通过；新增公开路由测试覆盖无认证 HTML 返回及查询参数。
- `GOCACHE=/private/tmp/track-legal-go-cache go build ./...` 通过。首次受限构建有缓存写入警告，授权后重跑无错误。
- `GRADLE_USER_HOME=$PWD/.gradle-home ./gradlew :app:assembleDebug --console=plain` 通过；安装至小米 `3e7cd913` 成功，启动后前台为 `MainActivity`。
- 隔离 Chrome 验证两页 × 两种语言 × 两种主题 × 320/390/1024 宽度共 24 组：无横向溢出、缺失锚点或外部资源请求，互链保留语言 / 主题，系统主题实时切换通过。人工查看手机宽度日间 / 夜间截图。临时证据在 `/private/tmp/track-legal-qa/`。
- 两个仓库 `git diff --check` 通过。
- `verify_agents.py` 已运行，仍报告既有 AGENTS 超长、缺少优先级声明及八个既有 Go 文件超长问题；全局 `~/.codex/AGENTS.md` 为空，入口规则未落盘。未扩大本次修复范围；推荐修复命令为 `python /Users/foxcoder/.codex/skills/agents-md-generator/scripts/manage_docs.py sync-global-codex-agents . --write`。

未提交或推送，未部署服务端。因此安装包已指向新地址，但线上服务器需更新后才能展示这些页面；未声称已完成真机到线上新页面的端到端验证。

## 2026-10-14：阿里云镜像构建与推送尝试

用户明确要求执行 `docker buildx build --platform linux/amd64 -t crpi-p78v4agazv8zn80d.cn-beijing.personal.cr.aliyuncs.com/track_server/track_server_go:latest -f deploy/Dockerfile . --push`。Linux amd64 镜像构建成功；首次推送返回 `insufficient_scope: authorization failed`，用户重新登录后已完成推送，过程如下。未重启线上服务。

- 本地镜像索引摘要：`sha256:3851c8778a92511d6ebc21117c32c51d0abb88f20d4450278ca2003c9114a6a7`；本地 latest 标签已指向该镜像。
- 使用该镜像启动无外网、内存仓储、禁用定时任务和统计的临时容器。`/api/v1/ping` 正常；两个协议 URL 均返回 HTTP 200、`text/html; charset=utf-8`，HTML SHA-256 与本地源码完全一致。
- 临时容器 `track-legal-image-check-20261014` 已停止并自动移除。
- 用户在本机重新登录并回复“已登陆”后，普通 `docker push` 上传镜像层成功，但 registry 拒绝附加证明清单：`unknown manifest class for application/vnd.oci.empty.v1+json`。
- 随后执行 `docker push --platform linux/amd64 crpi-p78v4agazv8zn80d.cn-beijing.personal.cr.aliyuncs.com/track_server/track_server_go:latest` 成功，将此前已验证的同一平台镜像推至远端，未包含镜像索引中的附加证明。
- 远端 latest 摘要：`sha256:96533346bb70446572c5c3b2994a7e9ff9b5e56384cd9da94659df88554dc987`。`docker buildx imagetools inspect` 独立读取确认摘要一致，远端配置确认 `os=linux`、`architecture=amd64`。该摘要是单平台 manifest；与本地多平台格式索引摘要不同属预期。
- 本次仅上传镜像，服务器仍需另行拉取并重建容器，线上更新尚未执行。
- 本轮没有修改 AGENTS.md 或业务源码。页面仍保留草稿提示及占位信息。

---

# 历史工作交接：存量标题回填与本机 Go 环境

日期：2026-10-09。

## 用户要求与当前结果

用户先要求查看存量轨迹能否改为投稿同类自动标题，确认后台支持改名后明确要求“去改吧”。用户随后提供 `ssh root@47.93.239.24`，说明使用密码登录。通过本机隐藏密码输入框完成认证，未把密码写入聊天、命令参数或文件；已在该服务器真实数据库执行并核对回填。

用户随后明确要求“别临时环境了，你给本机把 go 环境配上”。已安装官方 Go 1.27.2 darwin/arm64 到用户目录，校验官方 SHA-256 和文件大小；`~/.local/share/go/current` 指向 SDK。已备份并更新 `~/.zprofile`、`~/.zshrc`，加入 SDK 和默认 `~/go/bin` 路径。新登录/交互终端均可直接使用 `go`、`gofmt`，Hello World 编译执行成功。

## 已完成代码

- `cmd/backfill-track-titles`：MySQL 一次性回填工具，先生成 JSON 计划，再显式应用计划。计划含新旧标题并绑定目标数据库指纹，应用时检查作者、旧标题、未删除和已结束状态，只改标题及更新时间。结果逐条保存，已有输出不覆盖。
- `internal/service/track_title_backfill.go`：默认只识别常见日期时间标题，已补齐旧版“• / ·”分隔符；优先复用同作者投稿标题。显式 `-include-dated-titles` 同时清理导入标题日期前后缀、上次日期兜底标题，保留已有路线描述。地点缺失时按距离/运动类型生成，不再用日期兜底。未知运动类型保留原值；原始轨迹缺失时不猜测环线。
- `docs/backfill-track-titles.md`：完整操作步骤和边界。`AGENTS.md` 只增加对应入口与稳定边界，未改 HTTP 协议、Repository interface 或表结构。
- 新增测试覆盖标题识别、人工标题保护、环线与原路折返区别、缓存轨迹解析、投稿复用、计划校验、错误数据库和输出文件保护。
- 预览后增加历史地址清理：忽略“已定位”、纯门牌号和 Plus Code，括号文本截断时保留前面完整地点；新标题优先选择前面仍有效的街道/地点。新增相应测试。

## 已运行验证

- `go version`：`go1.27.2 darwin/arm64`。
- 新登录/交互 zsh 中直接查找及执行 Go 成功。
- `go run /tmp/track-go-env-check.go`：`Go environment OK`。
- `go test ./...`：全量通过。
- `go build ./...`：通过。
- `go build -o build/backfill-track-titles ./cmd/backfill-track-titles`：通过，本地产物已生成。
- `gofmt -l`（本次四个 Go 文件）：无输出。
- `git diff --check`：通过。
- Linux amd64/arm64 初版交叉编译通过；含最终地址清理规则的 Linux amd64 工具重新构建通过，与服务器上传文件 SHA-256 一致（`7ac61ea621367f4361dbb3f6634948e9fecc6238dccefb65606aaa13c4fbbdae`）。最终清理规则改动后再次执行 `go test ./...`、`go build ./...`，均通过。
- `verify_agents.py`：运行过，仍报告仓库既有 AGENTS 大于 16KB、缺少优先级声明、八个既有源码文件超长等问题；全局 `~/.codex/AGENTS.md` 为空、入口规则未落盘。未为本次功能扩大治理修复范围。可用技能提供的 `manage_docs.py sync-global-codex-agents . --write` 修复全局基线，写入用户目录需遵守沙箱权限。

## 服务器实际执行结果

- 目标：`root@47.93.239.24`，x86_64，业务容器 `track_server_go`，复用其现有 MySQL 环境变量与 `/var/log/track_server` 持久化挂载。未重启/替换业务容器。
- 初始预览：扫描 281 条未删除轨迹，筛选出 112 条时间型标题；2 条复用投稿标题，110 条按已记录事实生成；26 条有可确认形态的完整原始轨迹。
- 最终清理历史地址后的候选 ID、作者与旧标题范围与初版一致；修正 11 个低质量地址生成标题。
- 实际应用：`updated=112`、`skipped=0`。
- 独立只读核对：修改前后总记录数均为 354；112 条新标题全匹配，242 条其余标题完全未变，所有记录其余 22 个字段摘要完全一致，无新增或缺失记录。
- 服务器留存目录：`/var/log/track_server/maintenance/title-backfill-20261009/`。包含 `initial-plan.json`、最终 `plan.json`（作者/新旧标题）、`baseline.json`（全部旧标题与其他字段摘要）、`result.jsonl`、`verification.json` 及已使用的运行工具。
- 本地只读执行记录在已忽略的 `build/title-backfill-20261009-*` 文件中，不提交这些业务数据文件。

## 第二轮日期标题清理（用户继续授权）

用户查看结果后要求“还有一些标题依然是年月日的，你要不也都改了”。重新通过本机隐藏密码窗口认证后，检查到 123 条未删除轨迹仍带日期：106 条旧版“日期 • 时间”，9 条“日期 + 地区”导入标题，1 条“路线名 + 日期”，7 条上次日期兜底标题。全部已结束。

- 补齐“•”格式识别，加 `-include-dated-titles` 扩展预览范围；保留“走马岗”等原路线名，使用已记录地点和运动生成其他标题，无有效地点时使用距离/运动。新增“未知位置”、纯坐标、连字符门牌号过滤，避免将坐标或 `250-2` 生成地点标题。
- 最终全量 `go test ./...`、`go build ./...` 通过；Linux amd64 v4 交叉编译通过，服务器上传 SHA-256 与本地一致：`660db671e554653274d0232359b0f57e6344b4192f435ae383b6818f20b9e1d1`。
- 实际应用：`updated=123`、`skipped=0`；独立核对前后均 354 条记录，全部 123 条新标题匹配，其余 231 条标题和所有记录其他 22 个字段均未变。
- 再次 SQL 查询：未删除轨迹带年月日标题为 0；64 条已删除轨迹保持原值。完整扩展预览再次扫描 281 条轨迹，`candidates=0`。
- 新一轮服务器留存目录：`/var/log/track_server/maintenance/title-date-cleanup-20261009/`，包含初始/最终计划、baseline、result、verification、空 remaining-plan 及 v3/v4 二进制；保留上一轮备份目录。
- 本地核对文件在忽略目录 `build/title-date-*`；没有修改 Android 上传标题工厂，此次已完成的是现有数据库标题清理，不能声称所有未来新上传均不会带日期。
- 未重启服务，未更改 HTTP 接口或表结构。
- 提交前补齐无效投稿标题兜底：投稿标题仍是日期、过短或过长时，清空无效候选后按记录事实重新生成，测试同时确认投稿本身未被修改。全量测试和构建再次通过。

## 剩余事项

本次存量回填已完成。本提交保存回填工具、测试及操作文档，没有修改业务 HTTP 协议或表结构。此前 Android 工作副本的导航端点优先规则和创建轨迹标题改动仍是未提交工作，勿误覆盖。回填工具当前仅支持 MySQL，不得声称支持 Mongo。


## 2026-10-19：轨迹点赞与个人主页获赞

用户要求将轨迹详情底部收藏位改为心形点赞，收藏移至分享右侧；个人页真实获赞数、关注/粉丝旁新增获赞 tab。用户选择按单条记录展示，同一人赞多条轨迹保留多条。客户端和服务端均已实现，尚未提交、推送、构建远端镜像或部署。

- JWT 接口：POST/DELETE /track/:track_id/like、GET /track/:track_id/like/status、GET /user/:user_id/likes/received/list。MySQL track_likes 表、Mongo 唯一索引、内存实现同步。只允许正常公开、已结束、raw_track_url 非空的轨迹；禁止自赞，写入幂等。受限账号不能新增但可取消。详见 docs/api/track-likes.md。
- 用户详情 received_like_count 与获赞记录可见性一致，轨迹详情及摘要 liked/like_count 按浏览者计算；资源缓存失败不清零点赞。游标按时间、轨迹、点赞用户三级倒序。
- Android TrackLikeViewModel 防止重复请求，失败保留成功状态；ReceivedLikesFragment 支持刷新分页、头像/昵称进入个人页、轨迹文案/缩略图进入详情。原删除和投稿管理保留。
- go test ./... 通过；补充缓存回归后 service/handler/repository 复测通过；并发点赞 -race 通过。assembleDebug、assembleDebugAndroidTest、4 项地图布局 JVM 测试通过；4 项点赞 instrumentation 与 3 项客态主页回归通过，追加真实获赞数断言再次通过。MySQL/Mongo 实例联调未运行。
- 已安装并启动小米 3e7cd913，前台 TrackDetailActivity；截图确认心形和收藏位置。线上仍旧服务，点赞数显示“—”，须部署新版后验证线上互动。测试未向真实服务写入点赞或关注。
- 保留 Android 原有未提交的背景缩略图 3dp 圆角、详情地图缩短与居中调整。两仓库 git diff --check 通过。
- 服务端 AGENTS 追加点赞约束；规范校验仍有既有超长文件、优先级与全局入口问题，未扩大治理范围。


### 2026-10-19 跟进：数量展示统一

用户补充：轨迹已有点赞数需下发，零或缺失隐藏数字；大数中文使用万、英文使用 K，个人页遵从，沉淀客户端 AGENTS。服务端现有 like_count 字段及详情/列表下发测试已复核通过，数据继续保持原始整数；docs/api/track-likes.md 补充展示约定。客户端新增 CountFormatter / Context.formatDisplayCount，中文 10000、英文 1000 起缩写，最多一位小数并向下截断、去掉 .0；接入个人页关注/粉丝/获赞、轨迹 tab 数量及首页卡片收藏/跟随数。详情透传列表 liked/like_count，服务器详情加载也同步已有数量；状态成功后旧列表数量不能覆盖，零或未知隐藏数字，心形居中。app/AGENTS.md 已追加该规则并明确精确物理量不机械缩写。

assembleDebug、assembleDebugAndroidTest、CountFormatterTest 三项 JVM 测试及点赞/客态主页共 10 项 instrumentation 通过；TestTrackLikesIdempotencyViewerStateAndCounts 通过。安装并启动小米 3e7cd913，前台 TrackDetailActivity，截图确认无数量时心形居中。AGENTS 校验新增规则无告警，仍有既有 root 超长及源码超长问题，全局入口为空。未提交、推送或部署。


### 2026-10-19 跟进：预览轨迹线与允许自赞

用户指出不再上传地图截图。获赞列表右侧现在直接使用独立轨迹线 SVG（track_no_map_bg_screenshot_url），复用 HomeRoutePreviewView 的轨迹线绘制及缓存，56dp、3dp 圆角、低饱和蓝灰背景，日夜颜色均定义；缺少或加载失败显示路线图标。不再依赖 track_screenshot_url，回收清空预览、异步回调按绑定代次隔离。build 和两项 SVG 解析单测通过，点赞/个人页 10 项设备测试通过，含没有截图但 SVG 能画线及缺少 SVG 的兜底；已安装小米并启动 MainActivity。

随后用户修正规则：允许给自己的轨迹点赞，但不算获赞列表。已实现自赞计入轨迹 like_count，同时从个人页 received_like_count 和获赞列表排除，保证两者口径一致。MySQL/Mongo/内存均在排序分页前过滤自赞；我的轨迹 liked 改按当前用户计算，能正确回显自赞。新增接口测试验证重复自赞、本人列表状态、他人点赞与自赞共同计入轨迹、主客态获赞数/明细排除自赞、limit/has_more 不被自赞占位、重复取消。go test ./... 全量通过。服务端 AGENTS 和 API 文档同步，新规则覆盖上文“禁止自赞”的历史记录。规范校验仍有既有文件超长/全局入口为空问题。尚未提交、推送或部署；线上自赞等新接口须部署新版后生效。


## 2026-10-20：获赞按轨迹聚合与二级点赞人列表

用户担心爆款刷屏和列表过长，接受：首页按轨迹聚合，每轨迹一张卡片，最多 3 位最近点赞人、人数与最新时间；点击人数进入该轨迹分页点赞人列表，轨迹区域进入详情、头像进入主页。有效关系继续长期存在 track_likes，不另存消息副本，自赞计入轨迹总数并排除个人获赞数/列表。

- 新 JWT 路由 GET /user/:user_id/likes/received/tracks 与 GET /user/:user_id/likes/received/tracks/:track_id/users。保留旧 /likes/received/list 逐条入口兼容。首页 total_count 为轨迹组数，卡片 received_like_count 和二级 total_count 为他人点赞人数，个人页统计仍使用有效他人点赞总数。
- 新服务 track_like_groups_service.go、三份 track_like_groups_* 仓储：SQL 按轨迹 GROUP BY 后 HAVING 游标再 LIMIT，分页内最近 3 人使用有界索引查询合并一次返回；Mongo 从作者公开轨迹出发查关系聚合，分页后 lookup 最多 3 人，避免把爆款所有用户放进数组；内存语义一致。补 track_id/created_at/user_id 复合索引，MySQL 启动幂等迁移已存在的表。没有新增业务表。
- Android ReceivedLikesFragment 复用为聚合首页与点赞人模式，新增 TrackLikersActivity（通用导航栏）和聚合卡片布局。复用 SVG 轨迹线及淡蓝灰底，大数复用 CountFormatter。两层列表刷新/游标分页，按轨迹/用户 ID 合并实时排序产生的重复项，游标不前进时终止；直接定位列表末尾也能触发翻页。仍不依赖地图截图。
- 服务端 go test ./... 全量通过；新回归覆盖 120 赞合成一组、每页 1 组、最近 3 人、点赞人 50/50/20 分页、同时间游标、自赞过滤、取消点赞、私密轨迹与非法参数。实例 MySQL/Mongo 联调未运行，不声称已做线上联调。
- assembleDebug、assembleDebugAndroidTest 通过；CountFormatter 和 SVG 解析共 5 项 JVM 单测通过。设备 TrackLikesTest 与 GuestProfileTest 共 12 项最终全通过（首次新增直接定位翻页测试发现零位移回调未触发下一页，修复后通过）。测试截图仅含造数，文件 /tmp/received-like-preview-test.png。
- 已安装到小米 3e7cd913，启动后前台检查为 FollowListActivity（App 已在前台）；未向真实服务写入点赞/关注。没有提交/推送/构建远端镜像/部署；线上须更新新服务才能显示聚合列表。
- API 分册、路由索引、用户详情指引和服务端 AGENTS 稳定业务规则已同步。verify_agents.py 仍报既有 root 超长/缺少优先级、多个既有源文件超长、全局入口为空等，未做无关治理调整。保留 Android 先前圆角/地图布局/数量格式及点赞的未提交改动。


### 2026-10-20：用户授权同步提交推送

用户明确要求 server 和客户端代码均同步提交推送。两个仓库均为 main，git fetch origin 后 HEAD 与 origin/main 没有分叉，无需合并。提交范围包含本轮完整点赞/聚合获赞、数量格式与先前未提交的小幅界面调整，排除 APK、缓存、日志和敏感配置。提交前服务端 go build ./... 通过；沿用刚完成的 go test ./...、客户端 Debug/测试包构建、5 项相关 JVM 单测和 12 项设备回归结果。仍未部署服务端，线上新接口需要单独更新服务。


## 2026-10-20：移除获赞列表，保留获赞总数

用户明确要求：“获赞列表页和获赞列表的相关接口下掉吧……获赞数还是得留着的。”本节覆盖上文有关保留获赞列表/旧列表兼容入口的历史描述。

- android-track 移除获赞页、点赞人名单页及其 Manifest、布局、字符串和 Retrofit/repository/模型；个人主页获赞数保留且不可点击，关注/粉丝仅保留两个标签。
- track_server 移除三个 GET 查询路由（逐条记录、按轨迹聚合、轨迹点赞人），删除 handler/service、三仓储列表查询和游标模型。保留点赞、取消点赞、状态查询、轨迹点赞数和用户详情 received_like_count。点赞关系、表结构和已有索引保留；MySQL 幂等索引初始化函数仅移动到 track_likes_mysql.go。
- 内存仓储 CountReceived 改为独立计数，无需依赖已删除列表方法或排序；仍排除自赞、私密、删除、进行中、未上传和他人轨迹。
- 同步 docs/api/README.md、models.md、route-index.md、track-likes.md、user.md、中英文隐私说明及服务端 AGENTS.md 中的点赞约定。
- 已验证 go build ./... 与 go test ./... 全量通过；Android ./gradlew :app:assembleDebug :app:assembleDebugAndroidTest :app:testDebugUnitTest 通过；两个仓库 git diff --check 通过。
- 模拟器 emulator-5554：TrackLikesTest 与 GuestProfileTest 合计 9 个 instrumentation 测试通过，包含获赞数不可点击、两标签及重建、点赞/取消/数量显示等。服务端测试覆盖移除的三个路由返回 404、自赞不计个人获赞、幂等和可见性计数。
- 真机小米 3e7cd913 已安装 Debug 与测试包，启动 LoginActivity 后确认前台为 MainActivity。首次真机自动化停在既有详情用例，主动停止该次 instrumentation 并在模拟器完成回归，不把真机中止结果算作通过。
- 已运行 check_freshness.py 和 verify_agents.py；仍有既有 root AGENTS 超过 16KB、优先级/元数据缺失和 Go 源码超长等治理问题。全局入口规则未落盘（~/.codex/AGENTS.md 为空），推荐修复命令：python /Users/foxcoder/.codex/skills/agents-md-generator/scripts/manage_docs.py sync-global-codex-agents . --write。本次未扩展治理重构。

未提交、推送或部署服务端；接口下线在部署新版后生效，旧客户端访问原列表路径将收到 404。

## 2026-10-20：轨迹评论、回复与图片

用户授权客户端与服务端完整实现评论，UI 尽量复刻其小米截图，颜色沿用 App 主题。最终口径：普通 emoji、最多三张图片且允许纯图；先不做主动 @、话题、评论分享、编辑和通知；回复列表平铺，回复某条回复只引用直接目标。作者最新回复须直接展示在主评论列表。沿用点赞的轨迹可见范围。

- Android 新增独立 `:comments` Library；详情页底部接入评论入口及总数。评论/回复同尺寸浮层、回复页从左进入、返回保留主列表位置；键盘上方另开发布窗口，提供引用、草稿、图片选择/移除/预览、普通 emoji、1000 Unicode code point 限制及失败重试。宿主提供语义色、MingCute 图标、通用 AppModalDialog、用户主页、图片查看与 OSS 上传。日间蓝色、夜间主题强调色均复用现有颜色。
- 服务端新增 `CommentService`、鉴权 handler、MySQL/Mongo/内存三种仓储与四张表/collection，接口见 `docs/api/track-comments.md`。主评论/回复分别倒序游标分页；仅当前页附最新未删除作者回复，批量装饰用户/引用/评价，头像不逐条同步下载。总数、楼层回复数、互斥赞踩随事务维护；按用户和 client_comment_id 幂等，冲突内容返回 409。
- 仅本人删除；仍有回复的已删主评论保留占位，最后一条回复删除后隐藏；删除引用目标后不下发原文/图片。账号限制覆盖发布、图片预约和非零赞踩，允许读、删本人及取消赞踩。私密/删除/进行中/未上传轨迹隐藏评论，恢复公开后恢复可见。
- 图片先预约后直传精确 OSS key，发布验证归属、实际 JPEG 尺寸和大小，最多三张、5MB/张。图片走评论鉴权地址，私有缓存位于 LogDir/comments/images；不挂通用 static、不下发原始 OSS 地址。需要现有 STS 和 OSS_INTERNAL_ENDPOINT；未配置时文本功能可用、图片预约 503。
- MySQL 发布/删除锁定轨迹统计行，赞踩仅锁对应评论；Read Committed + 有界死锁重试，避免并发首次创建统计行的死锁。Mongo 使用事务，发布/删除经统计文档建立冲突边界、赞踩按评论冲突重试；部署须为副本集，compose 已补单节点副本集配置。未部署或修改生产数据库。
- 验证：go test ./...、go build ./... 通过；评论 handler/repository 的 go test -race 通过；临时真实 MySQL 8.4、MongoDB 7 副本集及内存同一契约测试全通过，覆盖分页、同时间游标、并发重试、计数、赞踩、占位与图片绑定回滚。测试容器已停止并移除。
- Android Debug/测试 APK 构建、app 全量 JVM 测试、comments 三项单测通过；新增协议测试验证 URL 解析及发布字段。独立 emulator-5556 上三个 UI 用例在浅色与深色各通过一次，覆盖作者预览、引用回复、emoji 发布、赞踩切换/取消、分页和纯图选择/移除/发布。先前 emulator-5554 受其他应用自动化抢前台影响的失败不计为功能通过证据。
- 浅色/深色的主列表、回复页及发布窗口截图位于 android-track/build/outputs/comments-qa/，均为测试造数。已安装 Debug 到小米 3e7cd913，启动 LoginActivity 后确认前台 MainActivity。未向线上写入测试评论；真实 OSS 上传未做线上联调。
- 模块 README、API 分册、路由索引、数据模型、中英文隐私说明及两端 AGENTS 已同步。git diff --check 通过。check_freshness/verify_agents 已运行；既有 AGENTS 元数据缺失、文件超长、全局入口未落盘和旧源码超长仍存在，未扩展无关治理重构，修复入口见上一节。

尚未提交、推送或部署。手机上的新版客户端需服务端部署后才能使用线上评论；Mongo 独立实例需先启用副本集。保留本轮之前获赞列表移除、获赞数保留及海拔地址修复的未提交改动。

### 2026-10-20 跟进：评论元信息合并、空态、首次回复与单图

用户指出评论数应随轨迹信息下发，并连续提供小米参考：空评论列表的气泡插画/首评入口，零回复时先进入回复列表再自动打开输入窗口，以及图片只允许一张。本节覆盖上文“独立评论状态接口”和“最多三张图片”的旧实现。

- 已移除 GET /track/:track_id/comments/status、handler/service 方法与客户端 Retrofit/Gateway 调用。Track、TrackSummary、MyTrackSummary、CollectedTrackSummary 直接下发 comment_count 和 can_comment；创建/更新响应同步。详情在资源 URL 缓存处理前计算可用性，避免缓存失败误隐藏评论；列表按当前页轨迹 ID 从统计表批量读取，不逐条计数。MySQL、Mongo、内存均新增 Totals 实现。
- 客户端从列表透传评论元信息，在轨迹详情加载回调中同步；仅有本地缓存时复用现有轨迹详情接口补齐。发布/单条/赞踩响应中的 comment_count 为整条轨迹的有效评论+回复总数，删除返回 status/comment_count。发布与删除直接更新入口/弹窗数量，不额外查询计数。旧服务缺少字段时按既有轨迹信息兜底入口可用性，评论功能本身仍须部署后端才能使用。
- 空评论列表标题显示“评论”，使用主题色气泡插画、“暂无评论，点击抢首评”和固定底部“鸟过留鸣，人过留评”输入；点击空态直接打开发布。加载失败保持重试提示，不展示“暂无评论”。修复通用底部弹窗在空列表时收缩的问题，指定高度时按窗口可用高度完整测量。
- 主评论零回复时点击回复，先进入该条评论回复页，再自动打开发布窗口与软键盘；关闭发布后停留回复页并保留草稿。标题零回复显示“回复”；有内容时显示数量。首次显示输入窗口显式申请焦点及 IME，等待窗口焦点后弹出，避免需再点一次输入框。已删主评论仍有回复时保留查看回复入口。
- 评论图片改为客户端/服务端每条最多一张，允许纯图；系统单选，灰色输入区内显示缩略图，支持预览、移除、重选替换。服务端对 image_ids 长度大于 1 返回 400。API 文档、模型说明、路由索引、模块 README 与两端 AGENTS 已同步。
- 验证：go test ./...、go build ./... 通过，新增详情/推荐/搜索/我的轨迹/收藏/个人轨迹评论元信息、发布/删除总数、私密轨迹、资源缓存失败及多图拒绝回归。批量计数契约在临时真实 MySQL 8.4、Mongo 7 副本集及内存均通过，测试容器已停止并移除。Android Debug、测试 APK、app JVM 全量和 comments 单测通过；六项 UI 回归在浅色/深色各通过，覆盖评论数入口、错误重试与空态、纯图移除/替换/发布、作者预览与引用/赞踩、分页、首次回复与草稿。最后调整空态插画位置后，空态用例又在两种主题各补测通过；断言文字实际可见、列表高度完整、IME 自动可见。
- 浅色/深色最终截图为 android-track/build/outputs/comments-qa/{light,dark}-{empty,first-reply,single-image}.png，均为造数回归；已逐张检查。参考原图保存于 /private/tmp/track-comments-{empty,first-reply,single-image}-reference.png。Debug 已安装到小米 3e7cd913 并启动，前台 TrackDetailActivity；真实详情截图确认点赞右侧已显示评论入口（/private/tmp/track-comments-entry-final.png）。没有向线上写入测试评论或测试图片，真实 OSS 上传仍未线上联调。
- git diff --check 通过。AGENTS freshness/verify 已运行，仍有既有 root 超长、元数据/优先级、旧源码超长及全局入口未落盘问题；未扩展治理重构。代码仍未提交、推送或部署，新后端接口和真实计数须部署后生效。

### 2026-10-20 跟进：评论浮层、长按操作与时间展示

- 按小米截图调整 Android 评论界面：列表/操作菜单底部滑入并向下退出，关闭图标、回复按钮、赞踩图标缩小；保留赞踩点击区域。独立发布窗口外框直角，输入区保留小圆角，移除字数标识但仍限制 1000 Unicode code point。通过 CommentSheetKind 区分列表/发布/操作菜单，由通用 AppModalDialog 承载，未修改普通居中弹窗。
- 本人的未删除评论与回复在正文、引用、图片及行空白处支持长按，底部显示“删除、回复、复制、取消”；移除行内删除文字。复制仅复制正文，纯图复制项禁用；回复复用原有两层/引用逻辑，删除保留宿主确认。菜单随父评论窗口关闭，防止悬空。
- 用户明确时间只是示例，由实现决定完整规则：不足 1 分钟“刚刚”、不足 1 小时“几分钟前”、不足 24 小时“几小时前”、不足 7 天“几天前”；更早 MM-dd，跨年 yyyy-MM-dd，本地时区，英文单复数。时间由客户端派生，每分钟只更新可见行，不新增服务端请求、不重建列表；异常时间隐藏，轻微未来时间按“刚刚”兜底。
- 验证：Debug/测试 APK 构建、app 229 项与 comments 6 项 JVM 单测通过，时间回归含全部分界、跨年、本地时区及未来/非法时间。专用 emulator-5556 上七项 UI 用例在日间和夜间各全通过，新增长按取消/复制/回复/删除及无行内删除、无字数标识；首次新增用例匹配到隐藏回复页的同文节点，改为仅匹配可见节点后全量通过。运行时窗口属性确认使用新的底部位移动画。
- 最终截图 /private/tmp/track-comments-qa/{light,dark}-{main,composer,actions}.png 均为测试造数，已逐张检查。Debug 已安装到小米 3e7cd913，am start -W 确认前台 TrackDetailActivity。模块 README 已更新；本轮没有服务端代码/协议变化，未重复运行 Go 测试。没有线上测试写入、提交、推送或部署，服务端上线要求延续上一节。

### 2026-10-20 跟进：浮层单轴入场、右侧回复页与评论点赞人

本节覆盖此前“回复页从左侧进入”的旧实现。用户要求评论浮层从底部丝滑弹出、回复与点赞人页从右向左进入；回复页主评论 0 赞显示“暂无人赞过”，正数可打开参考截图中的头像昵称名单，不做游客行。

- AppModalDialog 先调用 Dialog.create 完成 AlertController 内容创建，再在窗口 attach 前固定宽高、底部居中、遮罩和导航栏。AlertController 会重置宽高，既不能在 create 前设置后任其覆盖，也不能等到 onShow 才拉成全宽；此前先升起再横移就是显示后尺寸重排。入场只用 280ms 纵向窗口动画。
- 独立 CommentPaneAnimator 管理同一浮层内的水平翻页：前页保留到入场完成，回复页及 CommentLikersPane 从右侧进入，返回向右退出；取消旧动画使用代次校验，防止迟到回调隐藏当前页。前一页及滚动状态保留。
- 主评论无赞占位不响应点击，有赞显示人数与箭头；名单只在点击后请求，分页按用户 ID 合并，失败明确显示重试且不会因滚动回调无限自动请求。支持点击用户访问主页，赞踩切换后人数入口跟随响应更新。
- 新 GET /track/:track_id/comments/:comment_id/likes，标准分页 items 为 id/nickname/avatar_url，total_count 复用 like_count，取 limit+1 判断 has_more。只列当前 reaction=1，含自赞，取消/点踩移除；按 user_id 倒序稳定分页，不表达点赞时间先后，游标绑定轨迹/评论。已删评论（含有回复的占位）、不可见轨迹返回 404，登录与受限账号读取规则沿用评论。没有恢复已下掉的轨迹点赞人或个人获赞列表接口。
- MySQL/Mongo/内存均实现查询。MySQL 启动为旧表幂等补 idx_comment_likers(comment_id,reaction,user_id)，基线 mysql.sql 同步；Mongo 同字段索引、降序 user_id，仅读索引字段并排除 _id。无新增业务表或名单副本；头像昵称按当前页批量装饰，复用异步头像缓存。
- API 分册、路由索引、入口 README、模块 README 与中英隐私说明已同步。go test ./...、go build ./...、评论点赞人/仓储契约 race 检查通过；临时真实 MySQL 8.4、Mongo 7 副本集和内存契约通过，覆盖稳定分页、正负评价过滤、自赞、取消/切换及旧表缺索引补齐与重复执行。临时容器已停止并清理。客户端 Debug/测试包构建、app 229 项与 comments 6 项单测通过。
- 九项 TrackCommentsTest 在日间/夜间各通过；专用 AVD 原有 window_animation_scale 为 0，恢复为 1 后再次完整运行日间九项也全部通过。测试显式匹配 Dialog，并在创建时同步登记动画等待时间、跟随异步 onShow 更新，避免点击移动中的窗口；不改产品交互来迎合测试。断言验证回复/点赞人页的初始 translationX 位于右边界之外，结束为 0；同时覆盖名单失败不自动重试、手动重试、25 人分页、用户点击、无赞占位与赞/取消、返回保留评论状态以及既有发布图片流程。
- /private/tmp/track-comments-qa/{light,dark}-{likers,no-likes}.png 为造数回归最终截图，均已检查；正常动画倍率的连续帧确认浮层保持全宽从底部进入。小米 3e7cd913 已安装最终 Debug，前台 TrackDetailActivity；真机已存在评论数据，本次未发送测试评论、图片或点赞。未提交、推送或部署服务端，新点赞人接口需随这次 track-server 更新上线。两仓库 git diff --check 通过。专用模拟器与临时数据库均已清理关闭；保留其他任务和此前的未提交修改。


### 2026-10-20 跟进：紧凑评论、全文展开与用户手动验证约定

- 用户要求停止在专门编写/执行测试上投入时间，由其本人验证。android-track 与 track_server 根 AGENTS 均增加“默认验证方式”：默认不专门新增或运行自动化测试、不启动模拟器或测试数据库；保留必要编译、自查，Android 沿用已授权安装/启动。仅用户明确要求时执行指定测试范围。同步移除 Android 原“优先补/跑测试”、服务端全量测试及 admin scope 的默认要求；已有测试保留。
- Android 赞踩统一：点赞为 track_like_active 的红色实心心形，点踩用官方 MingCute 实心裂心并着次要灰色，同时替换对应线性素材。回复按钮使用独立 day/night 背景与正文前景色。
- 评论/回复头像改为 28dp、正文 14sp、昵称 11sp，标题 15sp/header 52dp，行间距与操作图标收紧。按参考图比例调整，尚待用户在真机判断对齐效果。
- 新 CommentContentView 在正文超过六行时保留前五行，在第六行约半行处追加省略号及高亮“全文”，按 Unicode 字素边界截断；点击全部展开且无收起。展开 ID 由主/回复页共享，滚动、分页、赞踩与来回导航保留，实际重新刷新该列表后重置。模块 README 同步。
- 在用户改约定前曾新增一项全文 instrumentation 用例，测试 APK 编译遇到多余括号；现已修正语法，但按新约定不再构建或运行测试 APK，不声明新增用例通过。前一阶段的历史测试结果见上文，不能作为这次最终界面的验收证据。
- 本次最终仅运行 ./gradlew :app:assembleDebug（通过），Debug 已安装小米 3e7cd913，启动 LoginActivity 后返回 MainActivity。未自动操作评论业务、未在线上造数。专用 emulator-5556 已关闭，未动其他模拟器。两仓库 git diff --check 通过；无服务端业务代码变化，未重复构建或测试 Go。
- AGENTS verify/freshness 已运行；仍报告既有根文件超 16KB、旧源文件超长、server 缺优先级与 freshness 元数据等。全局入口规则未落盘（~/.codex/AGENTS.md 为空），修复入口仍为 python /Users/foxcoder/.codex/skills/agents-md-generator/scripts/manage_docs.py sync-global-codex-agents . --write；本次仅更新用户要求的验证偏好，未扩展治理重构。

四项新界面效果由用户手动验证，当前未做最终日夜间截图/自动化验收；未提交、推送或部署。


### 2026-10-20 跟进：登录页夜间按钮、移除光晕与协议入口

- 用户补充要求顶部及底部光晕全部去掉；activity_login.xml 已移除两个装饰 ImageView。
- 手机号登录按钮原 track_surface_deep 几乎融入夜间底色，现复用 bg_login_button_primary 与 track_primary_text：夜间浅黄绿底、深橄榄文字/图标，日间浅蓝底、主题蓝文字/图标，沿用现有语义色，无新增硬编码颜色。
- 登录协议通过中英文资源 annotation 标记两条链接，ClickableSpan 使用 track_highlight_text、无下划线；仅对应协议名可点击。新增 web/LegalPage.kt，将关于页既有协议 URL、语言和日夜参数封装为共用入口，登录页与关于页均打开既有 WebViewActivity，未改变登录态或协议正文。
- ./gradlew :app:assembleDebug 通过，git diff --check 通过；Debug 已安装小米 3e7cd913。遵循用户手动验证约定，未新增或运行测试，未启动模拟器或测试容器。界面与协议点击效果待用户验收。
- 本次无服务端业务代码/协议变化、无新的长期业务或开发规则，故未扩写 AGENTS。未提交、推送或部署；保留此前全部未提交改动。


### 2026-10-20 跟进：底部星星收藏与详情收藏数

- 用户要求收藏放到评论旁边、空心/实心星星状态，并由轨迹信息下发收藏次数。Android 已移除右上角收藏入口，底部顺序为点赞、评论、收藏；MingCute 官方 star regular/filled 素材转为 vector，收藏使用主题强调色，未收藏次要色。数量为 0/未知隐藏，使用 CountFormatter，点击区禁用覆盖请求中状态。
- 三个社交按钮及导航宽度收紧为 44dp、高度维持 52dp、间隔 8dp；上传/投稿文字单行自适应，给新增收藏留出空间。收藏数先用列表首屏值，再由已有详情回调补齐；自己的列表未下发 collected 时复用轨迹详情元信息刷新。无新增计数接口，CommentsController 的详情补齐回调同步提供收藏元信息。收藏成功后的权威状态不被迟到的初始详情覆盖，重建保存状态/数量；收藏事件继续分发到首页、搜索和个人列表。
- Track 新增临时 collected/collect_count，详情、创建/更新响应填充；列表沿用已有批量 CountByTrackIDs，不逐条新增查询。track_collect_summary.go 复用收藏仓储统计现存用户/轨迹唯一关系，包含作者自藏。详情 collected 取当前 JWT 浏览者。POST/DELETE track_collect 保留 status 并增加操作后读取的 collected/collect_count，客户端优先采用响应；旧服务器未带字段时保留原有本地加减兼容。
- MySQL 继续复用已有用户/轨迹唯一关系和 idx_collects_track，无新表或计数列。Mongo 补齐原有 AddCollect、RemoveCollect、RemoveByTrackID stub：setOnInsert upsert 保留首次收藏时间、唯一冲突按幂等成功处理；取消与轨迹删除清理关系，新增 track_id 索引支持现有聚合。内存重复收藏不再刷新时间，口径与 MySQL/Mongo 对齐。
- API 详情/模型/收藏分册与路由索引、两端 AGENTS 稳定约定已同步。go build ./... 与最终 ./gradlew :app:assembleDebug 均通过，两仓库 git diff --check 通过；最新 Debug 已安装小米 3e7cd913 并执行基本启动检查。遵循用户约定，未新增/运行测试、未启动模拟器或数据库容器，界面、收藏切换与数量由用户手动验收。
- AGENTS verify 已运行，仍有既有根文件超长、server 优先级/路径提示、旧源码超长及全局入口为空等；没有扩展治理重构。未提交、推送或部署服务端，详情收藏新字段与操作后权威计数需部署本次服务端版本才能线上生效。保留此前登录页、评论、获赞列表下线、海拔等全部未提交改动。


### 2026-10-20 跟进：Me 背景图固定比例与裁剪页沉浸栏

- 用户要求编辑资料选择 Me 背景图时固定展示宽高比，并修复裁剪页未沉浸状态栏。ProfileBackgroundActivity 开启 fixAspectRatio，使用当前窗口宽度与 120dp 身份区 + 64dp 统计区 + 16dp 重叠区 + 状态栏高度作为比例；三个区域尺寸提取到 dimens，ProfileFragment 的布局和 inset 更新共用，避免日后裁剪比例与实际背景区域失配。仍支持固定比例选取范围和多指缩放，JPEG 最长边限制继续 2048。
- 新 TrackImageCropActivity 继承现有 CanHub CropImageActivity，保留原裁剪视图、旋转/翻转菜单、状态恢复和结果返回；仅以应用 Toolbar 承载顶部操作，NoActionBar 主题去掉独立系统 ActionBar。TrackImageCropContract 委托原契约构造参数/解析结果后切换到新 Activity，背景与头像入口统一接入，头像仍固定 1:1。Manifest 移除未使用的原库 Activity，防止进入旧裁剪窗口。
- 状态栏/导航栏透明、关闭系统对比度遮罩；容器背景贯穿系统栏，按 systemBars/displayCutout 为按钮和内容补边距，系统图标明暗由裁剪页背景亮度决定。背景裁剪显式使用既有黑色 viewer/白色 foreground 语义色，头像沿用日夜主题。app/AGENTS 已记录共用入口与固定比例约束。
- ./gradlew :app:assembleDebug 通过，git diff --check 通过；Debug 已安装小米 3e7cd913，执行基本启动检查。遵循用户约定，未新增/运行测试、未启动模拟器，裁剪操作、比例和沉浸效果待用户手动验收。AGENTS 验证仍报既有根文件/旧源码超长等，无关治理未扩展。无服务端业务变化、未提交推送或部署。
- 上一小轮详情页导航文案已改为中文“跟随轨迹”/英文“Follow track”，单独编译安装通过，包含在当前包内。


### 2026-10-20：用户授权两端同步提交推送

- 用户明确要求“客户端和server的代码都同步/提交/推送”。已分别 fetch origin，两仓库 main 与 origin/main 均为 0 ahead / 0 behind，无需合并，按当前 main 提交推送。
- 提交本轮完整评论/回复/单图/赞踩及点赞人查询、获赞列表移除与总数保留、收藏计数与底部星星，以及 Android 海拔地址、登录协议/主题、固定比例背景裁剪与沉浸状态栏、导航文案和手动验证约定。保留已写的测试代码；遵循用户新约定，本次不再新增或执行测试。
- 最近最终 Android :app:assembleDebug 与服务端 go build ./... 已通过，Android 已安装小米；本次仅提交前检查改动及 git diff --check。客户端 .gitignore 的 build 规则改为覆盖全部模块，排除 comments/build 等生成目录；APK、缓存、本地签名和敏感配置均不纳入提交。
- 使用中文提交信息记录完整功能、协议/数据结构及验证范围；未部署服务端。新评论接口、收藏数及下线获赞列表需由部署新版服务端生效。提交/推送最终结果以本次对话输出的 commit 和远端一致性检查为准。


### 2026-10-22：静态轨迹预览与共享卡片联合发布准备

- 用户确认共享首页 Item 的复用页面跟随改版，并明确要求上传四图、列表/详情下发地址；随后要求“联合发布”。两端 main 已 fetch，均与 origin/main 一致。
- 新增 preview_images 四图值对象及 MySQL JSON 启动迁移，Mongo 更新同步；创建可选、更新整组替换，所有摘要与详情返回带源版本的鉴权图片 URL。
- 图片读取按轨迹可见范围检查，复用 AssetCacheService 和归属用户 OSS 凭证按需下载，缓存位于 LogDir/track_previews，未挂 static。列表不下载四图、不绘制地图。
- 新增入口：internal/models/track_preview.go、internal/service/track_preview.go、internal/handler/track_preview_handler.go、internal/repository/track_preview.go；同步 mysql.sql、main.go、相关协议和 AGENTS。此前 MingCute 线性图标及运动日夜色也纳入本次部署提交。
- go build ./... 已通过，git diff --check 通过；遵循用户约定未新增或运行自动化测试，未启动测试数据库。现有 AGENTS 校验仍报告旧的长文件、元信息和路径告警，未扩大整改范围。
- 发布遵循 Android 仓库 .agents/skills/android-track-release：先提交推送服务端，再构建推送镜像，用户当次 SSH 登录，部署健康后才发布 App。实际部署、App 版本及远端提交结果以本次会话最终输出为准。
