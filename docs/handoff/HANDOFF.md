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
- 本轮未提交推送或部署生产；先前已部署的统计接口不因本轮本地改动而自动上线个人纪录。用户后续明确部署时按server-only流程同步远端、提交推送、构建镜像并健康复核。
