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
