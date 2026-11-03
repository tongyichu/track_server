# 开发交接：AGENTS 精简（2026-11-04）

## 2026-11-04：后台详情紧凑布局与资源展示

- 用户要求轨迹详情只展示预览图、原文件下载图标及明确空态，并压缩表单。共享编辑器已移除资源 URL 输入框及保存字段；最多四列，宽屏标签/景点与地图并排，图片可放大，空图片“无”、空原文件“缺失”、图片失败可重试。
- `track_assets.go` 新增受 tracks 查看权限保护的 `/tracks/:track_id/assets/:asset`，复用原文件/私有预览/截图缓存和 OSS 内网下载，raw 使用 attachment，资源读取不阻塞基础详情，不改写持久化 URL。预览四图复用业务缓存版本 key，支持后台查看私密及下架轨迹。
- 三个使用共享编辑器的页面均加载 `track_editor.css`；`AdminUI.form` 增量支持 className/afterFields，其他页面保持默认布局。景点页此前两项未发布布局调整仍保留：显示数量在筛选下面，新增按钮紧邻目录标题。
- Go 构建、两个修改脚本的 node --check 及 diff 检查通过；未新增或运行自动化测试，功能/界面待用户手动验证。尚未提交、推送或部署本轮改动，Android 未修改。

## 请求与范围

- 用户在克隆 android-track-bakcup3 和 track-server-bakcup3 后，确认将偏大的 AGENTS 精简为关键规则与按需文档索引。
- 仅修改这两个副本的 Markdown 规则/参考文档；未改业务源码，未提交、推送、部署、构建或运行功能测试。
- 原交接完整归档为 [历史交接](history_handoff/HANDOFF-20261104-003606.md)，需恢复旧业务任务时读取。

## 已完成

- 根与 scoped AGENTS 保留默认验证、授权、风险边界与常用入口；详细业务/接口/UI 说明迁到 docs 参考文件，相关改动前按主题读取。
- 本仓 AGENTS 总体积 139,701 → 11,860 字节，减少 91.5%。
- 新增参考：docs/admin-routes.md, docs/admin-security.md, docs/admin-ui.md, docs/admin-workflows.md, docs/server-contracts.md, docs/server-flows.md。
- 修正迁移后的形象技能相对链接、陈旧的 Android uploader 路径；明确业务私有字段限制不影响后台授权查询，业务查询不外呼也不禁止后台人工专用接口。
- 按 agents-md-generator 技能进行了独立只读审查，审查指出的链接与范围问题已修正；采用 internal-docs 的中文、目录和代码围栏规范。

## 验证与剩余问题

- git diff --check、所有新增/修改规则与参考文档的本地 Markdown 链接/锚点和代码围栏检查通过；四份 AGENTS 均小于 16 KB。
- verify_agents.py 已执行：AGENTS 本身无错误或路径告警；全仓校验仍有 10 个既有超 1000 行源码文件问题，本次未修改业务源码。
- 全局 ~/.codex/AGENTS.md 为空，全局入口规则未落盘；这是已有环境状态，不属于本次两个仓库的精简范围。修复命令为 python ~/.codex/skills/agents-md-generator/scripts/manage_docs.py sync-global-codex-agents . --write，本次未执行。
- 纯规则和文档维护无需构建；未新增或运行单元/UI/集成测试，未操作设备或生产。

## 后续维护

- 继续保持根文件精简；新增详细功能说明写对应参考/API 文档，仅在 AGENTS 更新入口或不变量。
- 用户随后明确授权“两个都推送代码”：将本次规则精简、参考文档和交接提交推送到本仓 origin/main；不发布 App、不部署 Server。最终提交与远端核验结果见本次会话回复及 Git 历史。


## 推送前同步远端

- 用户授权两仓提交推送后 fetch，Android 无新增提交，Server 合入 origin/main 的 a75e52d。保留远端业务改动，将新增导入结果约束并入 docs/admin-workflows.md，AGENTS 继续保持精简。未额外构建或运行测试。

## 2026-11-04：批量导入结果表格与编辑入口

- 用户要求新克隆的 backup2 工作副本中，后台批量导入结果按下方轨迹列表展示表格，并可直接编辑。
- `tracks.js` 提取共用轨迹单元格，导入结果增加文件名、逐文件成功/失败状态及错误原因；成功行打开共享编辑器，编辑、下架、恢复或彻底删除后刷新对应导入行及主列表。保留本批归属人快照，刷新结果忽略已被新批次替换的记录。
- `ImportTrackResult.track` 增量返回 id/city_code/title/track_type/status，直接使用创建结果，不逐文件查详情，不传数值用户 ID；初次导入无预览图。接口文档与后台 AGENTS 已同步，Android 无需改动。
- `go build ./...`、`node --check internal/admin/static/tracks.js`、`git diff --check` 通过。未运行自动化测试，界面及功能待用户手动验证；未提交、推送或部署。
- AGENTS 校验发现原有根文件超过 16KB、缺少优先级声明和多份源码超长；全局入口规则为空。仅记录已有问题，本次未扩展治理范围。


## 2026-11-03：后台景点详情删除按钮

- 用户要求在后台景点详情提供删除按钮，移除该景点。本轮在 track-server-backup 实现，不涉及 Android 代码。
- 详情底部新增红色“删除景点”，仅已有 active 景点展示；data-admin-write=scenic_spots 沿用后台写权限隐藏，点击时再次校验权限并防止重复提交。复用已有 POST /admin/api/scenic-spots/:spot_id/status，提交 status=archived 及 expected_version，不增加路由、表、仓储方法或物理删除。
- 删除语义为归档：后台默认目录、统计和客户端均隐藏；关联轨迹、图片文件、来源绑定、内部关联和历史审计保留。确认文案明确这一点及不提交当前未保存修改。新建、归档/合并详情及只读账号不显示删除。
- 共享 AdminUI.confirm 是 Bootstrap 弹窗，而详情为原生顶层 dialog，故确认前关闭详情，确认结束后等待过渡再恢复原表单与滚动位置。取消不发请求；成功关闭详情、清理对应深链和地图预览，并刷新目录与统计。失败使用详情内联错误，409 保留草稿且不自动刷新/重试。
- 同步 internal/admin/AGENTS.md、docs/track-place-discovery.md 和路由索引，状态调整接口沿用现有权限、版本CAS及审计。无需将历史审计的通用“调整景点状态”推断成物理删除。
- 已通过 go build ./...、node --check internal/admin/static/scenic_spots.js、git diff --check。按项目默认约定未新增或运行单元/UI/接口自动化，未在生产删除任何景点，交互视觉由用户手动验收。
- AGENTS 验证仍有既有根规则超过16KB、缺少校验器认可的precedence声明、10个既有超1000行源文件及历史路径引用告警；全局 ~/.codex/AGENTS.md 为空，未擅自修复用户配置。
- 本轮未提交、推送或部署；线上仍是刚发布的服务端5a3d64a，客户端1.0.17（18）。用户后续明确部署时按 android-track-release 的 server-only 流程处理当前服务端改动，不重复发布 App。


## 2026-11-04：景区等级采集、保存与下发

- 用户要求先接景区 A 级的完整数据链路。本轮仅修改 track-server-backup，保留上一轮未提交的后台删除按钮；未改 Android UI、提交、推送或部署，也未执行生产景点补采/写库。
- 新增 service/scenic_spot_grade.go，按目录 active 高德来源的精确 POI ID 查询 v5/place/detail 的 business.keytag，严格归一到 1A..5A；不使用评分、酒店星级、名称简介推断、父景点继承或 AI。只读外部采样确认故宫 v3 无等级、v5 搜索及相同 ID 详情返回“5A景区”，密钥仅远端内存读取，未输出。
- ScenicSpot 新增 scenic_grade/source/status/checked_at，MySQL JSON、Mongo BSON、内存深拷贝沿用现有存储，不增表/列/仓储接口。source 为 amap:<POI ID> 或 manual，status 为 synced/unavailable/failed/manual；未知省略，成功无等级或无来源持久化负结果，失败后续重试。复用既有日预算和景点调用统计 operation=grade，每次查询最多15秒。
- 新建/更新/核验来源/合并来源时在相同保存内采集，返回版本可供随后的图片上传使用；自动识别新建异步采集。存量复用 provider-content/backfill 独立补等级，文字图片已同步或 provider_data_status=no_source 也可检查，grades 返回本批补齐数。等级查询失败不中断原文字图片同步；并发补采仅合并等级字段并走版本 CAS，保护人工编辑及简介生成。
- 后台详情新增等级下拉及来源状态提示，沿用权限、dirty 状态与保存。人工改值/清空记 manual，自动不覆盖；旧 PUT 请求省略等级保留，显式空串清空，null/非法等级400，来源状态时间服务端托管。替换自动来源会重置自动等级，人工值保留。
- DiscoverSpot 与轨迹 primary_scenic_spot（详情/摘要）只读下发可选 scenic_grade，例如5A；未知省略，不应理解为官方未评级。客户端读取不发供应商请求；本轮不包含客户端等级展示。
- 同步根/后台 AGENTS、景点发现文档、track API 分册及路由索引。go build ./...、node --check internal/admin/static/scenic_spots.js、git diff --check 通过；按项目约定未新增或运行单元/UI/接口自动化，完整功能待用户手动验收。
- AGENTS 校验仍有已有12项错误（根规则超过16KB、precedence声明、10个既有超长源文件），freshness 无历史元数据；报告 /private/tmp/track-grade-agents-20261104.json 与 /private/tmp/track-grade-freshness-20261104.json，不宣称治理检查全绿。全局规则状态沿用上一轮记录，未擅自修复配置。
- 后续明确部署时仅需 server-only 发布，不需重复发 Android；部署后打开有写权限的景点管理页将自动分批补采存量等级。


## 2026-11-04：景区等级加入 labels 展示文案

- 用户要求轨迹标签由 API labels 直接提供“景点名·5A”，中间是上下居中的点；客户端景区详情在标题旁显示等级标签。
- buildTrackLabels 在景点名后追加已归一的 A 级，分隔符固定 U+00B7（·）；无等级保留原名。沿用 scenic_spot.type、value=true、稳定景点 ID code 及日夜配色，primary_scenic_spot.name 保持纯名称。所有经过统一投影的轨迹详情/摘要自然获得完整文案，不新增字段、路由、查询或持久化。
- Android backup 同轮接收 DiscoverSpot/TrackPrimaryScenicSpot.scenic_grade，透传详情 Intent，在标题旁显示可隐藏的等级标签；TrackDetailLabels 继续直接消费 API name/name_en，不在客户端拼接。没有等级的数据不展示等级标记。
- 已同步根 AGENTS、track API 分册、路由索引提示及景区等级文档；go build ./... 与两仓 git diff --check 通过，Android :app:assembleDebug 通过并安装三星。按约定不新增/运行业务自动化测试，功能由用户手动验收。
- 服务端 AGENTS 校验仍为既有12项错误（根规则体积/precedence及10个既有超长文件），报告 /private/tmp/track-grade-label-server-agents-20261104.json 与同前缀 freshness 文件。保留上一轮后台删除按钮、完整等级采集链路的待提交修改。
- 未提交、推送或部署。Android 已安装但线上 API 仍旧，真实等级展示依赖后续部署服务端并完成存量等级采集。


## 2026-11-04：本轮服务端部署准备

- 用户要求“部署”，执行 android-track-release 的 server-only 流程，仅提交推送和部署服务端，不发布或提交 Android。
- 本地景区等级、labels 文案和后台删除入口提交为62fcccd；fetch发现远端新增6个提交（至47aaa10），普通merge合入城市管理、批量导入表格和规则索引拆分。保留远端“景点管理”命名以及本地等级/删除控件。
- 规则冲突按远端精简结构解决，业务细节补入 docs/server-contracts.md、docs/admin-workflows.md；本地和远端历史交接归档均保留，新功能交接合并到本页。源码合并后重新执行Go构建和修改JS语法检查，部署结果以随后回执为准。


## 2026-11-04：服务端部署完成

- 用户要求“部署”，按 android-track-release 的 server-only 流程完成，仅处理 track-server-backup。源码提交62fcccd，合并远端6个更新后的实际镜像源码为 main/38a189fcf37e987771e4008abca8de9c354175b3，已推送并核对 origin/main；镜像构建时工作树干净。Android 未重新发版或提交。
- 发布景区等级采集/保存/下发、API labels.name 的“景点名·5A”文案和后台景点归档删除按钮，同时包含已合入的城市管理/批量导入更新。合并后 go build ./... 与景点/城市/导入 JS 的 node --check、git diff --check 通过，未新增或运行业务自动化测试。
- linux/amd64 镜像已推送。manifest digest 为 sha256:0a6c5cdc58d219d4389b1b280136375da925ef28c87a7fb1c5a2d9733d699c55；镜像配置ID为 sha256:4a79d930fb9257061dc8c7f17be850ee4ceb15e9cb4cc64ed36cd4499dbd7a08，revision label 与38a189f一致。
- 生产/root/update_track_server.sh 当前精确匹配同名容器并自行启动，先预拉取、核对镜像ID/digest/revision和脚本镜像选择结果，再执行原脚本，无第二次重复docker run。仅替换 track_server_go；新容器 b40316c5fe523a3f5067e2dfaa0e7d65f6dbc8c3bbc96295b09740638274ece9，restart_count=0。旧镜像98be969b7c7b保留，生产.env及持久化挂载不变，未修改更新脚本或输出密钥。
- 内网与公网 GET /api/v1/ping 均200、hello track app；启动确认使用MySQL、无内存降级、载入3个后台账号，无账号存储错误。后台未登录API401、页面302到登录，登录页200；资源版本2da6b5861ef5c46c415d。
- 部署回执 /private/tmp/track-grade-deploy-20261104.json，镜像元数据 /private/tmp/track-grade-deploy-image-20261104.json，安全日志 /private/tmp/track-grade-deploy-20261104.log。存量等级由打开有写权限的后台景点管理页后自动分批采集，本轮未额外调用业务回填、修改景点或伪造业务数据验收。
- 合入远端规则精简后，AGENTS校验仅剩10个既有超长源文件问题，无规则文件错误或路径告警，报告 /private/tmp/track-grade-deploy-agents-20261104.json。当前追加为部署结果文档记录，不改变已部署的38a189f源码快照，无需重新构建服务镜像。
