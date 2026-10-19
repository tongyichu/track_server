# 工作交接：协议 H5 页面（2026-10-13）

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
