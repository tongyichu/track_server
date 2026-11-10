# 后台时间与时区修复交接（2026-11-10）

## Original Plan And Steps

用户反馈 foxcoder 刚操作后的最近活跃显示为12:50:43，并要求排查后台所有同类问题、记录到AGENTS，避免再漏时区转换。

## Current Step

本轮代码、协议和项目规则已修改，用户随后已明确授权部署；本次从main/3b3fbf3（最近活跃功能）继续发布该功能显示问题及同类调用点修复。最终提交、镜像和生产健康状态以本次部署回执为准，不能将实现交接时的未部署状态当作最终生产状态。原消息模块交接完整保留在history_handoff/HANDOFF-20261110-210838.md，本文不重新认定其独立验收状态。

## Resolved Problems

- 已只读核对线上foxcoder：last_active_at=2026-11-10 12:50:43.784747 UTC，对应北京时间20:50:43.784747。MySQL驱动loc为默认UTC，数据正常，旧users.js截断RFC3339导致显示少8小时；未改数据库记录。
- runtime.js新增共享AdminTime，assets.go原有注入链保证其先于页面脚本执行。后台用户/权限快照、账号/审计/登录菜单、发布/更新日志、反馈、埋点、路线组、同行/位置/弹幕、投稿、AI使用/价格、工作台、景点/候选及图表统一显示北京时间。
- 通知定时发布/到期和轨迹起止时间按北京时间双向转换，未编辑时间保留原始值和精度。AI日期筛选、北京今日、未来生日校验及后台日周月窗口明确时区，纯日期/游标/时长保留原语义。
- MySQL工作台每日趋势及兼容用户/上传趋势改用admin_daily_mysql.go按自然日边界批量UNION ALL聚合，不再直接DATE_FORMAT原始DATETIME；所有边界按time.Time交驱动转换，与连接实际存储时区一致。Mongo/内存兼容聚合按传入窗口时区归日，无表结构/接口方法变更。
- 根及internal/admin/AGENTS.md已记录不变量、共享入口和列表/详情/输入/保存/跨日/统计分桶静态核对项；细则见docs/admin-ui.md#时间与时区，相关API文档和route-index已同步。

## Verification Evidence

- go build ./...通过（最后一次包含仓储按日聚合修复）。21个变更JS的node --check通过，后续改动的runtime.js及announcements.js已重新通过语法检查；git diff --check通过。
- 未新增或运行单元/集成/race/API/UI测试，未启动额外环境。列表、输入保存、跨日及统计的功能/性能验收由用户手动完成，未声称已实测。
- agents-md-generator检查无规则内容/引用警告，仍报10个HEAD中已存在的超长源文件；全局/Users/foxcoder/.codex/AGENTS.md存在但为空。检查记录：/private/tmp/track-admin-time-audit.md；完整输出：/private/tmp/track-admin-time-agents-check.json。本轮不扩大为全仓拆分或修改全局规则。

## Remaining Problems And Next Work

- 用户已要求部署，沿用android-track-release服务端流程：同步、Go构建、明确路径提交推送、确认远端提交，再构建镜像/生产更新并核对内外网健康。本次仅部署服务端，不操作客户端发布或安装。
- 功能/UI手动核对重点：UTC/偏移显示同一北京时刻、跨北京零点、非北京时间浏览器、空/零/非法值、通知与轨迹输入往返、未编辑秒/小数秒保留、凌晨统计归日。
