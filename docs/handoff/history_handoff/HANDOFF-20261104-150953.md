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
