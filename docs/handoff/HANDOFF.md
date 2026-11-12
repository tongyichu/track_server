# 后台轨迹筛选与共用省市选择器（2026-11-12）

用户要求轨迹管理增加运动类型筛选；资料缺项显示数量并移除任意缺项；省市合并为两级列表并复用到其他后台页面。

已实现track_type服务端校验与三仓储过滤，列表返回DefaultTrackTypeConfigs权威选项。切类型回第一页，与标题/用户名/状态/地区/缺项组合，统计卡片口径保持独立。missing_counts按其他条件统计全量、忽略当前missing与游标，MySQL条件聚合/Mongo facet/内存单次遍历，与列表共用基础过滤和缺项条件。旧API missing=any保留兼容，后台移除选项；计数失败显示未知，候选服务失败不阻断非候选列表。

共用region_picker.js已接入tracks/users/companions/route_groups/scenic_spots的列表地区筛选，以及景点新建表单；省份→城市两级，全部省份下有全部/全部城市，具体省份有全省/该省城市。选择完成后只派发一次原城市change，保留各页面查询/保存语义；本机不操作业务页面。原控件保留隐藏为数据源，权限禁用及程序回填同步，弹层支持键盘/外点关闭/窄屏限宽，目录数据继续由原页面读取。

最终go build ./...通过；所有修改JS均通过node --check，git diff --check通过，五个页面的HTML控件ID与共用脚本加载顺序静态核对通过。构建曾被沙箱缓存写入限制阻止，获准在沙箱外重跑后通过。用户随后要求推送，本轮纳入提交并合并远端d850da1的在线通信关系图与通知布局更新；最终推送状态以Git记录为准，未部署，不运行自动化业务测试。现有AGENTS的后台组件/路由/仓储入口与权限政策适用，共用组件约定已记入admin-ui.md，无需修改规则文件。

原交接如下：

# 活动与通知覆盖、重发及定时发布（2026-11-12）

用户确认实现：撤回可编辑/存草稿再发布；一次性通知面向发布时已有用户，持续公告在有效期覆盖新注册用户、每人每轮一次；定时发布可改期/取消，重启补执行，不补发已过完整有效窗口的内容。单次定时，不含循环任务。用户随后要求同步最新代码及推送，本轮提交上述服务端改动，未部署。

已快进本机main至已存在origin/main的0f6d252，保留远端实时监控功能。新代码复用活动通知既有权限/同源/审计路由，无新业务接口；专用notice_mode/notice_round/notice_until查询字段及MySQL补列/mysql.sql、Mongo、memory已同步。新调度notice_schedule与消息MQTT投递解耦，revision CAS互斥激活/编辑/取消。后台编辑ID稳定，App使用发布轮次不透明ID，receipt隔离旧轮次，原通知ID与阅读规则兼容保留；Android按已有不透明ID消费，无需改包。

后台新增覆盖/发布方式、独立停止发放时间、取消定时、撤回后编辑重发、实际发布时间与最近50条操作记录；同一记录CAS保存全部操作元数据，列表不返回历史，编辑请求不回传只读历史。列表复用未变行，维持只读权限、取消旧请求及北京时间显式转换。

go build ./...、node --check、git diff --check及后台HTML控件引用静态核对通过。未运行单元/集成/接口/UI测试或自动发消息；本轮提交推送状态以Git记录为准，未部署。人工验收重点：新老用户资格、持续公告到期、同轮编辑不重提醒、撤回重发的新轮次、旧页面已读不清新轮次、改期/取消与调度竞争、重启过期任务、只读账号及窄屏。AGENTS入口及验证政策仍适用，无需改规则文件。

原交接保留如下：

# 跨天个人运动与恢复交接（2026-11-12）

## Original Plan And Steps

用户要求个人运动支持暂停、跨天长途、杀 App 后继续同一场运动，暂停可选择计入或不计入时长；讨论方案后明确“好，开干”。后续用户报告安装包启动崩溃，已定位为恢复卡片新增 LinearLayout 后 MainActivity 底栏适配仍强转 FrameLayout.LayoutParams。

## Current Step

实现位于 android-track 与同级 track_server 的 main 工作区，用户已明确授权提交推送，提交与远端状态以Git记录为准；未正式发布或部署。客户端 Debug 构建、服务端 go build ./... 通过；功能/UI由用户手动验收，未新增或运行自动化测试。

## Resolved Problems

- 录制草稿复用 Room，按账号隔离元数据及增量原始点/候选点，事务提交、5秒心跳，失败重试未提交点。冷启动恢复暂停状态，异常中断与主动暂停分别标记；继续沿用 UUID/目标，重建定位与硬件步数基线。
- 同时保存总历时、记录时长、暂停/中断段；默认主时长不含暂停，可在设置或面板切换。真实结束时刻独立保存，热量只算 recording；详情显示计时口径和每日记录明细。
- segmentId贯穿计算、交通校正、实时/详情地图、回放、公里标、SVG、静态预览、Room、GPX和KMZ；禁止暂停或中断跨段连线补里程/爬升。云端补图及KML解析保留 segment_id。
- 完成先冻结快照并停止采集，本机outbox保存成功后才清草稿；失败保留重试，相同UUID去重。主页恢复卡片保持入口；底栏Insets已改为对 mainContentColumn 整体避让，使用 MarginLayoutParams，修复用户报告的启动 ClassCastException。
- 服务端可选 recording_timeline 保存、校验和回传；MySQL启动补JSON列与mysql.sql同步，Mongo/内存一致。日历metrics_version=2按记录段分日，纯暂停日无活动，窗口总数按轨迹去重；历史无timeline保持旧语义。

## Verification Evidence

- Android：GRADLE_USER_HOME=$PWD/.gradle-home ./gradlew :app:assembleDebug，通过；最后一次构建包含启动崩溃修复，日志 /private/tmp/track-multiday-android-build.log。
- Server：go build ./... 通过；两仓 git diff --check 通过。没有执行单元、UI、接口、集成或race测试，没有新建测试环境。
- 小米3e7cd913覆盖安装修复包成功；am start -W 从 LoginActivity 冷启动自动进入 MainActivity，Status: ok。随后只读检查进程21411存活、前台为MapActivity；crash buffer最新本包异常仍为00:19:53/00:19:54两次旧布局强转异常，无修复后新增同类崩溃。检查日志 /private/tmp/track-multiday-crash-log.txt。没有自动操作录制、暂停、杀进程或其他业务流程。

## Remaining Problems And Next Work

- 先部署服务端再发布新客户端；旧服务端会忽略未知timeline字段，当前不能声称云端联通验收通过。
- 人工验收跨午夜录制、暂停过夜/重启后继续、录制中强停恢复、异地续录无连接线、两种计时热量不变、暂停计步、保存失败重试、上传再下载分段一致、日历归属、账号及权限/provider切换。
- 未采集的中断轨迹不能恢复，突然终止前未提交的少量尾部数据不保证恢复。新日历按本机定位时间切片；不同查询时区进一步按时间比例分配区间指标。
- 现有根/app AGENTS 的录制、Room、接口与验证入口仍适用，详细行为已写入功能参考/API文档，没有变更授权或测试政策。

## References

- [时间线协议](../api/track.md#跨天录制时间线)
- [上一份交接](history_handoff/HANDOFF-20261112-002137-before-multiday.md)

## 本轮推送整合

已合并远端9989c97，保留消息轨迹卡片、清空记录、手机号唯一性及鉴权更新，合并后go build ./...通过。本轮仅提交推送，未部署。

## 实时通信后台精简与连接详情（2026-11-12）

- 用户请求：`android-track-backup` 与 `track-server-backup` 的后台实时通信继续精简，近期异常与连接列表分成两块，并能从连接进入详情查看消息内容。
- 本轮改动仅位于 `track-server-backup`；Android 工作副本保持干净。顶部保留四个核心指标，其余运行信息折叠；连接与异常两个常驻区块各自筛选、分页、保留失败前的数据，未变化的行不替换。订阅数、排队消息、断开时间移至详情。
- 新增 GET `/admin/api/realtime/clients/detail?client_id=...`，继承 realtime 查看权限，从 Broker 采样连接确定用户/房间，不接受浏览器指定身份。消息连接展示该账号跨设备最近30条存储私信，含收发方向、正文和分享轨迹ID；同行连接展示房间最近30条存储弹幕，并额外校验 companions 查看权限。身份与消息ID均用字符串。
- 详情立即展示行内基础信息，正文独立加载；支持手动与15秒自动刷新、失败保留、关闭/隐藏取消、请求超时。内容为业务存储记录，不能证明特定连接实际收发；不展示定位/控制事件，不抓取 MQTT payload，不改变已读或投递状态。协议、路由索引和消息方案已同步。
- 验证：`go build ./...` 通过（默认缓存受沙箱限制后，经提权使用系统 Go 缓存完成）；`node --check internal/admin/static/realtime.js` 与 `git diff --check` 通过。遵守默认验证约定，未新增或运行单元/集成/UI/接口测试，未启动额外环境；实际 Broker、权限、窄屏与内容展示由用户手动验收。
- 未提交、推送或部署。现有根/admin AGENTS 的后台权限、只读、资源、时间和验证边界继续适用，无需新增规则。


## 消息连接详情聊天面板（2026-11-12）

- 用户请求：消息提醒连接详情右半部分采用客户端对话页的效果，默认显示底部最新消息，往前翻阅历史，并展示对话双方头像与名字。
- 改动位于 `track-server-backup`。桌面左侧保留连接信息并增加联系人分页，右侧按所选对象显示头像、昵称、左右气泡与时间；默认打开最近对话并定位底部。窄屏改为上下布局与横向联系人列表。参照 `android-track-backup` 的 DirectMessageAdapter/item_direct_message 实现样式，客户端没有改动。
- 新增 `internal/service/realtime_chat.go` 的只读对话、私信与用户投影；复用三种仓储现有线程筛选、游标和 seq 排序，不调用会补日志的 App 读取接口。每页31取30；历史严格 before seq，增量从 after seq 后按正序逐批追赶，序号全程字符串。消息详情改为 conversations/messages/people 结构，新增 realtime clients/conversations、clients/messages 分页路由。
- `static/realtime_chat.js` 单独负责聊天视图。消息行与联系人按稳定ID复用，历史补到顶部后保持可见锚点；自动增量刷新在底部跟随，在阅读历史时保留位置并提示回到最新。支持顶部下拉、向上翻阅和历史按钮；联系人切换隔离旧响应，隐藏/关闭取消请求，空态与失败可刷新重试。
- 用户资料按页去重、批量查询；头像只读取已有本站缓存或默认头像，缺失时占位，不触发远程缓存补齐。permissions.go 的头像资源模块映射增加 realtime，其他资源权限不变；同行内容继续额外检查 companions 查看权限。API、路由、后台流程和消息方案已同步。
- 验证：`go build ./...`、`node --check internal/admin/static/realtime.js`、`node --check internal/admin/static/realtime_chat.js`、`git diff --check` 均通过。遵守现有约定，未新增或运行测试，未进行 UI 自动化；联系人分页、历史边界、消息追赶、切换/取消、头像空态与窄屏手感由用户手动验收。
- 本轮未提交、推送或部署；生产仍为上轮部署的 `2d81348`（该版本内外网健康检查已通过，容器 `8cd44bf19da6`）。当前根/admin AGENTS 的只读、身份、资源与验证边界仍适用，无需修改规则。


## 消息连接改为账号私信流（2026-11-12，后续修正）

- 用户指出消息提醒连接面向账号，可能同时与第三个或更多用户沟通，单段对话的呈现不合适。当前实现已替代上一节的默认最近对话/左右气泡方案：默认“全部私信”，统一时间流的每条记录显示发送方→接收方的头像、昵称、ID、发出/收到标签和北京时间；联系人只作可选筛选，刷新不会自动切换到某一人。
- 账号消息跨会话，不能复用单个 thread 的 seq。新增 RealtimeAccountMessages 按 `(created_at,id)` 做历史和增量复合游标，空流用 origin 作为增量起点；MessageFilter 新增 AfterID，memory/MySQL/Mongo 三实现同步，未传 AfterID 保持既有 After 行为。无新增仓储方法或表结构变化。单联系人筛选沿用已有 seq 游标，两种游标混用返回400。
- 初始详情始终加载账号全部私信，消息页实际涉及的双方用户按页批量读取资料。保留底部最新定位、历史锚点、增量追赶、关闭/隐藏取消与权限边界。页面明确为存储“私信”流，不宣称覆盖通知、互动提醒、心跳或全部 MQTT 收发。
- Go 构建与 JS 语法、差量检查通过；未新增或运行测试，实际多联系人混合记录、分页、筛选和滚动视觉由用户手动验收。相关协议、路由和消息流程文档已改为最终方案。
- 仍仅修改 track-server-backup，未提交、推送或部署；现有根/admin AGENTS 的只读、三仓储和验证规则适用，无需修改规则。
