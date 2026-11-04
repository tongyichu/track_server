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
