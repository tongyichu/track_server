# AGENTS.md — internal/admin

**Scope:** internal/admin/；继承根规则，用户当前明确指令优先。内置 HTML/CSS/JS 控制台，独立挂在 /admin，不被业务 handler 引用。
本文保留跨页面必守约束；修改具体功能前读取下表对应参考章节。

## 鉴权与数据边界

- 使用 Authenticator + 内存 SessionStore（admin_session），不复用业务 JWT/RequestMeta，不把会话持久化；cookie 保持 HttpOnly，TTL 与 store 一致，重启需重新登录。
- 账号独立持久化于 admin_account_state，日志位于 admin_audit_logs；账号/权限以数据库为权威，环境变量只首次导入，故障拒绝鉴权，不回退内存或环境变量账号。保留最后管理员保护及 version/revision/security_version CAS。
- permissions.go 是 HTML/API/资源与跨模块权限权威，新增路由同步映射，未知模块拒绝；只隐藏导航不足以授权。每个受保护请求实时查状态/权限，不缓存鉴权；只读页面不得触发外部回填或写操作。景点删除按 archived 归档，等级采集保护人工值，具体见 docs/track-place-discovery.md 与功能流程参考。
- 写请求校验同源、从 session 取操作者、先审计再写；审计不可用拒绝发起，结果审计失败不得误报业务失败或声称跨库原子。响应 no-store，不通过公共 static 暴露受保护 HTML。
- int64 用户 ID 等用 JSON 字符串往返，禁止 JS Number；轨迹归属人只读、不提交，服务端拒绝变更归属。
- 本人邮箱/改密只认 session，验证码/配额持久化 CAS；已验证邮箱禁止管理员重置，旧重置入口永久关闭后不重开。公开找回防枚举、限流、TLS 邮件、审计和迁移策略必须按安全参考执行；无 SMTP 不伪造成功，不复用 OSS 密钥或擅自发测试邮件。
- 轨迹编辑、候选处理、地图/反馈联动使用共享 track_editor；跨模块读取/保存同时验证相关权限。候选确认、AI 填表和实际保存分清，不自动提交草稿；异步旧结果不得覆盖后来输入。
- 软删除/恢复/彻底删除保持各自前置条件和关联清理；purge 仅 status=0、双重确认，OSS 清理失败仅返回 warning。轨迹已保存但反馈关联/学习失败时明确已保存，不误报整项失败。

## 页面与资源边界

- 前端文件放 internal/admin/static/，复用现有 go:embed，不加第二套；路由和 fetch 固定 /admin，变更必须两端同步。业务静态资源由后台 session 代理，不用 App JWT 地址或暴露原始投稿 OSS URL。
- APK 经后台 upload-package 本机上传并解析包版本，发布表单固定 Android；不改回浏览器 OSS 直传。发布记录删除不删除独立 changelog。
- 组件使用内置 Tabler/ECharts、shell.js/ui.js/style.css；不新增 CDN 或复制导航/组件外观。新增页面加载 vendor/tabler.min.css → style.css，脚本按 vendor/tabler.min.js → shell.js → ui.js → 页面脚本。
- 主列表/筛选/详情基础信息不得等待图表、地图或外部回填；图表用 AdminAssets.loadCharts() 异步加载。动态资源用 AdminAssets.url()，保留内容版本、ETag、公共静态缓存与私有 no-store 隔离。
- 列表批量/去重查询，资源 I/O 限并发、超时和取消；不逐行无限 goroutine。MutationObserver 只处理受影响子树且避免递归；只读锁在新增/更新控件时恢复。快筛选忽略旧响应、失败保留上次数据。
- Card 默认桌面 18px/窄屏 14px 内容留白，同时覆盖子节点 card-body 和同节点 panel.card.card-body；p-0 仅用于明确满宽区域。工具栏响应式换行，宽表格局部滚动；展开详情不重分配列宽。
- 未可信内容用 textContent；设备快照非实时，未知/未上报不视为关闭。所有异步弹窗工作在关闭/切换时取消并释放资源。
- 城市管理页只读显示内部 `region_id`，高德 adcode 与 Google 地区 Place ID 写入目录节点的 `external_ids`；旧国内 `code` 保留供兼容，详见 docs/api/regions.md。

## 修改前查阅

| 范围 | 必读参考与权威入口 |
|---|---|
| 账号、权限、审计、cookie、邮件找回、int64、写操作和跨页面校验 | [安全与操作边界](../../docs/admin-security.md)；routes.go、permissions.go、accounts.go、auth.go；[邮件接入](../../docs/admin-email-recovery.md) |
| 页面/共享脚本/组件/性能/表格布局 | [页面与性能规范](../../docs/admin-ui.md)，包括性能、两种 Card 结构、工具栏和窄屏检查 |
| 新增/修改 API、页面或资源 | routes.go、permissions.go；[路由参考](../../docs/admin-routes.md)，实际路由以源码为准 |
| 用户/限制、纠错、工作台、统计、发布、埋点、轨迹/候选/导入、路线组、投稿、景点、设备快照和标签管理 | [功能流程与代码入口](../../docs/admin-workflows.md) 中对应章节；再查 docs/api/ 对应分册 |

## 验证

- 遵循根 AGENTS：代码修改 go build ./...，纯文档无需构建；默认不新增或运行业务测试，功能/UI 由用户手动验证。
- 修改 JS 执行 node --check；检查 fetch 与 routes.go、权限映射、静态打包、性能边界及桌面/窄屏布局规则。
- 协议变化同步对应 API 分册和路由索引。只报告实际构建/静态检查，不将静态自查称为视觉验收或性能实测。
