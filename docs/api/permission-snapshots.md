# 设备权限快照

客户端 `PUT /api/v1/user/permission-snapshot` 使用业务 JWT；用户归属只从 JWT 获取。后台 `GET /admin/api/users/:user_id/permission-snapshots` 使用后台会话及 users 读取权限，返回 `{code:0,data:{items:[],limit:100}}`，按服务端收到时间倒序展示最近 100 个安装实例。

请求字段：`installation_id`（16–64 位字母数字、下划线或短横线）、`schema_version`（1–100，当前 1）、`sequence`（正 int64）、`platform`、`device_model`、`os_version`、`app_version`、`version_code`（非负整数）、`collected_at`（RFC3339）、`items`（完整数组，可为空，最多 256 项）。设备描述字段各不超过 128 字符；请求最多 128 KiB。

每项含 `key`（非空唯一键，最多 200 字节）、`category` 和 `state`（非空，各最多 48 字节）。分类当前使用 runtime_permission、normal_permission、special_permission、system_setting、notification_channel、other_permission；状态为 granted/denied、enabled/disabled、unknown、not_applicable。服务端接受新的分类、键和状态，后台未知状态灰色显示，不推断为关闭。已开启绿色，未开启红色，并显示状态文字。自动发现清单权限；特殊能力仍需新增客户端读取逻辑。

成功响应 `{code:0,data:{status:"ok"}}`；400 参数错误、401 未登录、413 请求过大、503 快照仓储不可用。服务端覆盖 received_at，按账号及 installation_id 全量替换，只有 sequence 更大才更新；旧请求和重复请求成功但不覆盖。MySQL 表/Mongo 集合为 user_permission_snapshots；内存降级同样保持隔离语义。

Android 安装实例 UUID 存在 noBackupFilesDir，重装/清除数据后生成新实例，不采集 IMEI。每天本地日期首次进入前台上报一次，登录和升级触发一次；失败在后续前台重试，普通重试间隔至少一分钟。成功标记按账号、安装实例、日期和版本隔离；身份与 token 一起捕获，避免切账号串写。无后台轮询，不因权限弹窗/设置返回专门触发。时间展示为最近快照，不保证当前实时状态；旧版没有记录表示未上报。后台保留不同安装实例的最近状态，不提供历史版本列表。
