# 开发交接：后台图表统一、压缩及城市显示修复（2026-11-04）

## 用户要求

- 轨迹管理图表与用户管理统一风格，长期约定记入 AGENTS。
- 随后补充：整体缩小，减少统计区高度；轨迹列表大量未知城市也一并修复。
- 工作副本 track-server-bakcup3；Android 未改动。用户随后明确要求部署，已授权服务端同步、提交、推送、构建镜像及更新生产；最终结果见 /private/tmp/track-style-city-deploy-1H8gFP/receipt.md 与会话回复。

## 实现

- 新增 static/admin_chart_style.js，提供共用环形图、横向柱状图和安全 DOM 图例。users.js 保留原统计和生命周期，改为调用共享样式；track_statistics.js 同样复用。两页 HTML 在业务脚本前加载共享文件。
- 沿用用户画像的 Tabler 色板、57%–82% 环形半径、白色分隔、中心总数、侧边图例、12px 细柱、浅色底轨、圆角和字体。用户管理原百分比/总数/灰色空值语义保留，贡献榜显示条数，不伪造百分比。
- 轨迹图表内容区 360px → 220px，环形图 170px，标题14px，缩减控件/卡片留白；两卡片独立天/周/月切换保持。窄屏上下卡片、环形与图例保持两列，长图例内部滚动。
- 城市问题的代码证据：ListAdminCities 只返回新 region_id 选择项，轨迹可能仍存旧 adcode/目录键；tracks.js 原先仅在该选项数组按 code 查名称。改由服务端 CanonicalCityCode + CityNameByCode 解析增量 city_name，列表项、详情响应同级与导入摘要一致返回；编辑后的导入刷新同步该字段。没有改写存量轨迹或发起供应商请求。
- 前端优先 city_name，其次选择项映射；空城市显示未填写城市，非空无法解析显示未匹配城市（原编号），不假造地名。
- scoped internal/admin/AGENTS.md 已记录用户明确的同类图表统一/紧凑约定，细节放 docs/admin-ui.md。同步 admin-workflows 和 admin-tracks 协议说明，根 AGENTS 保持精简。

## 验证与边界

- go build ./...、相关四份 JS 的 node --check、git diff --check 通过。
- verify_agents.py 的 AGENTS 本身无错误/路径告警；仍有十份既有超1000行源码文件问题，本次未扩展无关拆分。
- 按项目约定未新增或运行单元/UI/集成测试，未浏览页面或操作生产；实际视觉、交互及真实记录显示待用户手动验证。
- 开发阶段尚未上线，后续用户已明确要求部署。上一轮已部署源码为4629622，镜像e8ee826e…，旧回执位于 /private/tmp/track-stats-deploy-XWXXNe/receipt.md；本轮最终运行版本以新的部署回执为准。

## 历史

- [上一轮交接](history_handoff/HANDOFF-20261104-150953.md) 保留图表统计接口及部署前同步信息。

## 本次部署前同步

- 合入origin/main 1251e27，远端两项新增仅为历史部署回执，未改变本次已通过编译的业务源码。以下保留上游部署记录。

## 2026-11-04：年度多指标接口部署与两端推送

- 用户先要求部署Server，随后追加推送两端代码；继续完成部署。服务端main/86acba4e9227f327b9754089213d01b76cbd08ea已推送，远端无额外更新；go build ./...与git diff --check通过，从该干净源码构建linux/amd64镜像。
- 镜像manifest sha256:8539cc17a149651eb8b492d036131ce376c3bb7fd39f516a6b6276fe4d0baf3f，配置ID sha256:f32e3ec94d61d35d4bc16db2fcd3c100fdb4e9e619825982463c57dfe508885e。核对registry清单、revision和架构后运行生产既有update_track_server.sh，容器d6abffafd1f72062e89e164daa0eb0f3ced9bf3140c8d0a74566381385c2fd15使用预期配置镜像。
- 内外ping均200/hello track app，容器running且重启0，MySQL正常、无内存降级，3个后台账号加载正常，无step_count未知列错误；日历及类型分布未登录401、后台API401/页面302/登录200。环境文件及持久化挂载保留，旧a51459e6镜像保留，未创建测试业务数据或调用私有指标接口自动化。
- 回执/private/tmp/track-data-metrics-deploy-20261104.json；镜像元数据与部署日志为同前缀image-20261104.json、20261104.log。本文档回执不改变已部署86acba4源码快照。
- Android合入远端至72cda62，保留数据页整页刷新、横滑锁定与失败保留缓存，合并后assembleDebug通过（9s）；两端最终Git提交/推送及Debug安装以会话回复为准。本轮不递增App版本或上传Release。


## 2026-11-04：部署回执Git同步

- 补推部署回执时远端新增814d29e/4629622后台轨迹统计提交，普通merge保留新后台代码及本次部署回执，前序完整交接沿用113807历史归档。此合并用于Git收尾，不重建或覆盖本轮已部署的86acba4镜像；新后台功能的部署状态由对应部署回执记录。
- 客户端main/676bc213c10e7c4cb692037b3ad9bf066d11a4a8已推送并再次fetch确认同步；合并后的Debug1.0.18（19）已安装小米3e7cd913，前台MainActivity确认。


## 2026-11-04：本人全部历史个人纪录接口

- 配合用户要求的紧凑纪录卡，新增JWT GET /track/activity/records，可选track_type、仅本人、no-store，返回按类型分组的distance/elevation/duration/steps纪录及获纪录轨迹摘要/鉴权预览地址。只统计正常/私密、已完成且有原始文件的全部历史；有限正数才产生纪录，按值、开始日期、ID比较，并列取较新。自驾仅距离/时长，骑行不含步数，真实步数缺失保持无纪录。
- PersonalRecordsRepository在MySQL/Mongo/memory三实现分别选各原始类型/指标候选ID，再一次FindByIDs取少量获纪录轨迹，service归一类型后比较。SQL使用分组MAX与日期+ID并列选择，兼容没有窗口函数的MySQL；Mongo排序分组，memory同规则。无新表/字段，不取GPS、不逐项详情、不拉取资源；toSummaries直接投影现有静态预览路由，避开BuildTrackSummaries的旧资源缓存下载。
- Android会将云端纪录与当前账号已完成且未上传本机轨迹合并比较；全局类型筛选生效，最多四格、带56×42dp预览，缺图占位，点item进入详情。未部署新接口时明确只有本机未上传纪录，不用假数据。
- gofmt、go build ./...、git diff --check通过，Android最终assembleDebug通过（8s）；未新增或运行单元/UI/接口自动化。API分册、路由索引和AGENTS入口已同步，规则仅剩既有10处超长源码，报告/private/tmp/track-personal-records-server-agents-20261104.json。
- 开发阶段尚未提交推送或部署；随后用户明确要求同步并部署Server，最终状态见以下部署回执。


## 2026-11-04：同步最新Server并部署个人纪录

- 用户明确要求“同步server代码，部署server”。提交个人纪录接口9e4da89，普通merge合入origin/main至5abfe3e的后台图表、轨迹城市与城市AI检索修复，发布源码main/289dfb369f0f170513410f20663a0f28195b36a2已成功推送，从干净工作树构建linux/amd64镜像。
- 合并后go build ./...通过；后台admin_chart_style.js、track_statistics.js、tracks.js、users.js的node --check及git diff --check通过。按仓库约定未运行业务自动化测试，个人纪录展示及交互待用户手动验收。
- 镜像manifest sha256:5d7c6a9fab9b7dd19e62629b84b779eb7441e438dcfbb0ece4df930ea1d6aa76，配置ID sha256:2d4fdf4f203a11ebf89c8aa713ac90df9208b45679225847cc4c681597a202fe，revision核对为上述源码。生产既有update_track_server.sh更新容器a97c3959a9dce9a290d1162780928af36c7e4b71383f48e42f0ade9d86e176f5，running、重启0。
- 2026-11-04 15:40:52+08:00生产内网ping及随后公网ping均200/hello track app。MySQL仓储正常、无内存降级，3个后台账号正常加载，个人纪录/日历/运动类型接口未登录401，后台API401/页面302/登录200，资源版本d215cb9a84c6b6804a0f。环境、持久化挂载保留，旧aa813b6e镜像保留。
- 完整回执/private/tmp/track-personal-records-deploy-20261104.json；对应镜像元数据track-personal-records-deploy-image-20261104.json和部署日志track-personal-records-deploy-20261104.log。此文档收尾不改变生产源码289dfb3。本轮仅部署Server，未改动、构建或发布客户端。


## 最终增量同步与部署（2026-11-04 15:44+08:00）

- 首次部署期间origin/main新增ac157bf“城市AI填写只检索文字资料”；回执推送拒绝后fetch并普通merge，发布源码461efef36cb76ffb74a0f3bf356a1815c10d9bf3已推送。合并后go build ./...及regions.js的node --check通过，再次构建并部署，最终生产含个人纪录与这项最新优化。
- 第二次预检发现旧更新脚本按docker images首项选镜像，会选到旧版本，在停止服务前安全中止。后续备份生产/root/update_track_server.sh（before-exact-image时间戳.bak，0600），仅将IMAGE_ID选择改为docker image inspect "$IMAGE"的精确ID；其余停止/删除确切容器、拉取和启动逻辑不变。镜像revision、manifest及config ID重新核对后部署成功。
- 最终manifest sha256:362a9fd464825d94e2ddd06888ad2ab257fc2f89b59241c909341d3365ac3aa0，配置ID sha256:2e567cb79b01f062f28df0921aae1f3914f365ddf5b40566a76213330da37271，容器90f5e6409d6e2a8616b0558ceeb005aa0858676aaf6c1386c2301c76322419cd，running/重启0。内外ping200；MySQL、3个后台账号、鉴权守卫及登录检查正常；资源版本75f2c587ceee30c15378，环境/挂载及前版镜像保留。
- 最终回执/private/tmp/track-personal-records-final-deploy-20261104.json，元数据track-personal-records-final-image-20261104.json，日志track-personal-records-final-deploy-fixed-20261104.log，runner为track-personal-records-final-deploy-20261104.py。最终运行源码461efef，后续纯回执提交不改生产快照。


## 2026-11-04：本人全国探索足迹接口（开发完成，尚未部署）

- 用户确认全国足迹图方案并要求实现。新增JWT/no-store GET /track/activity/footprint及/track/activity/footprint/tracks，全部历史本人正常/私密、完成且有原始文件的轨迹，支持可选track_type，省略为全部；不使用all哨兵、不允许指定其他用户。
- ActivityFootprintRepository在MySQL/Mongo/memory三实现仅读取ID/城市/类型/距离/开始时间轻量投影；service按类型/地区筛选后统计或分页，目录由持久化regions.Service注入，旧目录键/adcode按明确别名兼容，父省/坐标/省界对应码从目录取，不猜测途经城市或修改轨迹。返回国内全量启用目录节点（含真实零值）供本机记录合并，城市/省份运动次数、里程、城市数与明确关联active景区去重数；无归属与境外次数单列，不错误落点。
- 景区关联与目录每批最多200批量读取，按景区内部ID在城市/省份/全国去重，不统计附近候选或归档/合并景区，不调用供应商/AI、不读取GPS或下载资源。地区轨迹先按本人/类型/地区筛选，再按start_time/id降序游标分页，当前页FindByIDs一次批量取摘要并复核归属/状态/地区，直接投影鉴权静态预览。无新表/迁移/配置项。
- Android配套本地矢量地图、260dp卡片、类型全局筛选、未上传记录合并、失败保留与地区轨迹下钻，详见客户端功能参考。API索引/路由索引/track-footprint.md与AGENTS入口已同步。
- gofmt、go build ./...与git diff --check通过；Android最终assembleDebug通过（3s），已安装小米并启动MainActivity。按项目约定未新增或运行单元/集成/业务接口自动化，无生产业务数据或环境变更。规则报告/private/tmp/track-footprint-server-agents-20261104.json，仅既有10处超长源码，无路径告警，freshness stale=false。
- 本轮仅开发，未提交推送或部署；当前生产仍为前轮461efef，尚不包含探索足迹。后续用户要求部署时先同步两侧发布范围，Server按既有流程推送后构建镜像，生产脚本已使用精确latest镜像ID，不再依docker images列表首项。


## 2026-11-04：探索足迹生产部署

- 用户明确要求部署。提交bbdf9bd探索足迹接口，普通merge合入origin/main至5a7ec98的城市资料/AI超时重试修复，发布源码main/241ece40a036e97122a3fa23cfc70226c81cd41d已推送。从干净工作树构建linux/amd64镜像；合并后go build ./...、regions.js的node --check和git diff --check通过，未运行业务自动化测试。
- manifest sha256:225413a367a61ea64db3799b8c3ed5ea2c3e64fe0da640ddcaa9ef47656fdc04，配置ID sha256:c7e5608dc1d8a101413902706e31387edb526ea323cda6eff56a1df25e4d7282。生产先pull并核对摘要、revision和架构，再运行既有精确latest镜像ID选择的update_track_server.sh，无脚本改动；容器e8fb0935315eb67d26b326889f69602cdafde979b092d529c1650fa106c85099 running、重启0。
- 2026-11-04 16:39:41+08:00本机ping200，随后公网ping200，均hello track app；MySQL正常、无内存降级，3个后台账号，后台API401/页面302/登录200，静态资源版本b0d73504518c49676677。足迹及地区轨迹、个人纪录、日历、类型分布未登录均401；环境文件/持久化挂载保留，前版镜像保留。
- 完整回执/private/tmp/track-footprint-deploy-20261104.json；镜像元数据track-footprint-deploy-image-20261104.json，日志track-footprint-deploy-20261104.log，runner为track-footprint-deploy-20261104.py。后续纯文档收尾不改变生产源码241ece4。
- 部署期间用户追加客户端隐藏南海附属图形、放大主体；统计归属不变，仅更新客户端显示与对应协议说明。客户端构建/安装在另一仓进行，不在本次Server部署中提交客户端或发布App。

## 2026-11-05：控制台标题、城市缺项与图片操作部署

- 用户明确要求部署这三轮后台改动。发布源码main/af79620c1a9c331073b7ec590fd56a8d3d329bad已推送，正式track_server仓库快进同步后从干净工作树构建linux/amd64镜像；go build ./...、regions.js/photo_upload_editor.js/ui.js语法及git diff --check通过。未新增或运行自动化业务/UI测试。
- 后台18个页面标签统一为“时光轨迹 - 控制台”；城市树按已保存资料显示缺口号/缺简介/缺图片，保存后更新；城市和景点图片右下角增加下载原图，灰圈叉移到整张卡片右上角。沿用现有后台会话鉴权、上传及图片自动保存，无新API、表或权限规则；现有AGENTS边界继续适用。
- 镜像manifest为sha256:7af6d3e83ac0b647317892cc7e54782a09c090170698d40145bf3603dec6e965，配置ID为sha256:375abdbd6c74dc25ed2ddc5efcab26bfd16b0ce8ee4719292e59ecac2b777fd6。生产预拉取并核对摘要、revision和架构后运行既有精确镜像选择脚本，未修改部署脚本；容器b9697408f2f77398698cd6ef764faa689cac5e7e30244df84b2d8b76ba67e7b0，running、重启0。
- 2026-11-05 21:08:25+08:00内网及随后公网ping均200/hello track app。MySQL正常、无内存降级，3个后台账号正常加载，后台API未登录401、受保护页面302、登录页200。新标题已核对，资源版本533a1f73925303ed02a6，5份相关JS/CSS内外网SHA256均与发布源码一致。环境、持久化挂载和前版镜像保留；实际页面视觉、图片下载及保存交互由用户手动验收。
- 完整回执/private/tmp/track-console-deploy-20261105.json，镜像元数据track-console-deploy-image-20261105.json，日志track-console-deploy-20261105.log，runner为track-console-deploy-20261105.py。本文档收尾不改变已部署af79620快照；本轮不提交或发布客户端，Feed图标修改仍在客户端工作副本中。
