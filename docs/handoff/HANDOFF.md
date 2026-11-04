# 开发交接：轨迹管理统计图表（2026-11-04）

## 请求与完成范围

- 用户要求将轨迹管理页“本月轨迹类型”改为天/周/月可切换饼图，将“本月城市分布”替换为同期贡献轨迹量前十用户横向柱状图，最多者位于顶部。
- 工作副本为 track-server-bakcup3。新增 track_statistics.js，由两卡片各自控制周期、默认 month，初始同周期请求合并；图表异步加载，不阻塞列表，移除旧月度表格及整份 dashboard 请求。
- 新增 GET /admin/api/tracks/statistics?period=day|week|month，默认 month，非法周期 400。已注册在后台鉴权 API 组；permissions.go 现有 tracks 前缀映射直接覆盖该 GET 读取权限，不扩大模块权限。
- 统计按北京时间今天/本周一/本月一日零点到请求时刻，created_at 入库时间为口径，包含公开/私密的未下架、未软删除、已完成且有原始文件的轨迹，不跟随列表筛选。
- MySQL/Mongo/内存分别实现独立类型聚合与贡献用户榜，不读取整份工作台统计、不逐用户查询；用户榜排除不存在账号，按数量降序、数值 ID 升序取十人，ID 字符串输出，空昵称展示未命名用户。
- 图表支持空态、刷新、加载失败保留旧图与恢复原周期、快速切换忽略旧响应；宽度变化 resize，离页取消请求/释放图表。共享 ui.js 跳过周期按钮增强，避免普通按钮样式盖掉切换样式。
- 已同步 docs/api/admin-tracks.md、route-index.md、docs/admin-routes.md 和 admin-workflows.md。既有 scoped AGENTS 已指向统计/API/页面参考，无需新增重复规则或扩大 AGENTS。

## 验证

- go build ./... 通过。最终构建首次受沙箱缓存写入权限阻止后，按权限流程重新执行并通过；不是业务编译错误。
- node --check：track_statistics.js、tracks.js、ui.js 均通过；gofmt -l 无输出，git diff --check 通过。
- 按项目约定未新增或运行单元/UI/集成测试，未启动测试服务或数据库；未浏览页面，不声称视觉验收通过，图表交互与布局待用户手动验证。
- 功能改动完成后用户明确要求部署：按 android-track-release 流程同步、提交、推送服务端并构建镜像更新生产。此前未部署的说明仅对应开发阶段；最终状态以本轮部署回执为准。Android 不参与此次发布。

## 历史与后续

- [上一轮交接](history_handoff/HANDOFF-20261104-113631.md) 包含 AGENTS 精简、双仓推送及合入远端批量导入更新的记录。
- 若后续要求提交/部署，先检查当前工作树及远端；发布使用同级 Android 仓库的 android-track-release 技能。

## 部署前同步

- 用户授权部署后，合入 origin/main 的 86acba4（含此前 20 个提交），保留最新详情资源、地区管理、景区等级及年度运动指标/步数改动。路由注释合并保留统计与资源两项；上游交接完整保存至 [同步前上游交接](history_handoff/HANDOFF-20261104-113807.md)。本次最终部署提交、镜像和健康结果以部署回执为准。

## 2026-11-04：年度多指标接口部署与两端推送

- 用户先要求部署Server，随后追加推送两端代码；继续完成部署。服务端main/86acba4e9227f327b9754089213d01b76cbd08ea已推送，远端无额外更新；go build ./...与git diff --check通过，从该干净源码构建linux/amd64镜像。
- 镜像manifest sha256:8539cc17a149651eb8b492d036131ce376c3bb7fd39f516a6b6276fe4d0baf3f，配置ID sha256:f32e3ec94d61d35d4bc16db2fcd3c100fdb4e9e619825982463c57dfe508885e。核对registry清单、revision和架构后运行生产既有update_track_server.sh，容器d6abffafd1f72062e89e164daa0eb0f3ced9bf3140c8d0a74566381385c2fd15使用预期配置镜像。
- 内外ping均200/hello track app，容器running且重启0，MySQL正常、无内存降级，3个后台账号加载正常，无step_count未知列错误；日历及类型分布未登录401、后台API401/页面302/登录200。环境文件及持久化挂载保留，旧a51459e6镜像保留，未创建测试业务数据或调用私有指标接口自动化。
- 回执/private/tmp/track-data-metrics-deploy-20261104.json；镜像元数据与部署日志为同前缀image-20261104.json、20261104.log。本文档回执不改变已部署86acba4源码快照。
- Android合入远端至72cda62，保留数据页整页刷新、横滑锁定与失败保留缓存，合并后assembleDebug通过（9s）；两端最终Git提交/推送及Debug安装以会话回复为准。本轮不递增App版本或上传Release。


## 2026-11-04：部署回执Git同步

- 补推部署回执时远端新增814d29e/4629622后台轨迹统计提交，普通merge保留新后台代码及本次部署回执，前序完整交接沿用113807历史归档。此合并用于Git收尾，不重建或覆盖本轮已部署的86acba4镜像；新后台功能的部署状态由对应部署回执记录。
- 客户端main/676bc213c10e7c4cb692037b3ad9bf066d11a4a8已推送并再次fetch确认同步；合并后的Debug1.0.18（19）已安装小米3e7cd913，前台MainActivity确认。
