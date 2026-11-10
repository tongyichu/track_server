# 消息模块开发交接（2026-11-10）

## Original Plan And Steps

- 用户批准消息中心、系统/活动通知运营、陌生人回复前首条限制、文字+轨迹私信、拉黑/解除和时间、分页未读，以及仅消息模块内MQTT。

## Current Step

- 完成track-server-backup首版代码和协议/表结构文档；尚未提交、推送或部署，不修改canonical track_server并发工作。Android首版在android-track-backup，Debug已生成，但小米3e7cd913未连接。
- 新入口 internal/service/messages.go/message_direct.go/message_announcements.go/message_realtime.go、handler/messages.go、models/messages.go、repository/messages*.go。完整API docs/api/messages.md、架构/恢复/上线边界docs/messages.md。

## Resolved Problems

- JWT私信/未读/通知/互动接口、发送账号限制、MySQL/Mongo/memory一致CAS仓储及索引；生产消息持久化初始化失败503，不用内存假确认成功。
- 私信thread内保存完整Pending日志后按确定ID写dm并清日志；崩溃由worker/读/重试修复。同client_id内容指纹校验，服务端seq单调，来信已读游标与全局cutoff不回退。双方拉黑阻止收发但保留历史，已注销对象可解除原拉黑。
- MQTT沿用EMQX回调/发布客户端、签名secret，独立msgv1凭证15分钟，仅本人及通知主题订阅，禁止发布/通配符。QoS1只推刷新信号，持久化pending重投；私信双端同步。20秒广播心跳，App45秒无信号降级；每小时清过期凭证，不清聊天。
- 后台新增announcements模块及活动&通知页：独立权限/同源/审计、字符串用户ID、CAS、草稿/预览/定时/即时/撤回、选填HTTPS H5、全部/指定受众。使用Tabler对话框，重复详情/保存确认去重，取消旧请求、关闭忽略旧响应，textContent防注入。
- 通知App公开DTO不带受众ID/操作者；注册前历史无未读，编辑不重新置未读，撤回隐藏。评论/赞/收藏/关注在原操作成功后幂等追加并支持取消隐藏。

## Problems

- 互动事件与旧业务操作为两次仓储写入，并非跨库原子，进程在两次写入间退出有漏通知窗口；已在docs/messages.md明确，不把它描述成私信事务保证。不上线前历史互动回填。
- agents-md-generator校验仍报告原仓库超长文件及空全局入口规则；本轮新增messages已拆分，不再有新增超长文件。仅更新根索引，不扩大规则授权。

## Remaining Problems

- 尚未部署，不能声称生产消息联通或业务流程验收通过。两账号首条限制/回复/互关、轨迹失效、拉黑及时间、重启/重试、MQTT生命周期、分页已读、后台受众/定时/编辑/撤回/权限均待人工检查。

## Next Work

- 明确部署授权后沿用android-track-release：同步实际发布仓库、提交推送、构建部署，再确认生产存储/EMQX发布链路及后台模块权限。当前不自动发布。

## Verification Evidence

- go build ./...通过，announcements.js/shell.js/audit_log.js的node --check通过。
- 公告HTML 28个ID无重复且JS直接引用齐全；mysql.sql与运行时MessageRecordsSchema一致，git diff --check通过。
- Android :app:assembleDebug通过，未运行单元/集成/race/API/UI测试或启动数据库/容器，未写入线上业务数据。

## Conversation Summary

- 用户讨论消息页完整形态并批准开工，要求消息模块内共享连接、离开关闭，普通页红点非实时；杀App后应恢复完整服务端历史、未读和聊天。实现保存与提醒分离，以HTTP存储和序号补拉保障历史，MQTT仅作刷新提醒，不把同行延长到整个App。
