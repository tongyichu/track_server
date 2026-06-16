# 轨迹 App 埋点方案

> 本文定义客户端埋点口径、事件清单、属性规范与验收规则。
> 当前服务端已提供 `POST /api/v1/analytics/events` 批量采集接口；完整协议见 `docs/api/analytics.md`。

## 1. 目标

- 衡量核心漏斗：注册登录 → 浏览路线 → 创建轨迹 → 完成轨迹 → 分享/收藏/导航 → 复访。
- 定位体验问题：定位权限、GPS 质量、上传失败、OSS STS 获取失败、地图加载与轨迹渲染性能。
- 支撑业务增长：运动类型偏好、热门路线、城市分布、同行转化、成就激励效果。
- 保持隐私最小化：默认不上报原始经纬度、手机号、昵称、图片 URL、完整轨迹文件地址等可识别信息。

## 2. 命名与上报原则

### 2.1 事件命名

- 使用小写蛇形：`module_action_result`，例如 `track_create_success`。
- 页面曝光统一用 `_view`，点击统一用 `_click`，提交统一用 `_submit`，结果统一用 `_success` / `_fail`。
- 事件名一旦上线不得复用为不同语义；需要废弃时保留兼容周期，并新增替代事件。

### 2.2 上报时机

- 页面曝光：页面首次可见时上报；同页面内刷新数据不重复上报，除非切换核心 tab 或模式。
- 点击事件：用户主动触发时上报，不等待接口结果。
- 结果事件：接口返回、上传完成、定位状态变化或业务状态落定后上报。
- 长耗时任务：同时上报开始、成功/失败，并带 `duration_ms`。

### 2.3 离线与幂等

- 客户端允许本地缓存事件，网络恢复后批量补发。
- 每条事件必须有 `event_id`，建议 UUID；补发时保持不变，供数据侧去重。
- `client_time` 使用客户端本地时间，`send_time` 使用实际发送时间；服务端或数据平台应补充接收时间。

## 3. 公共属性

所有事件必须携带以下公共属性。

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `event_id` | string | 是 | 单条事件唯一 ID，重试补发保持不变 |
| `event_name` | string | 是 | 事件名 |
| `client_time` | string | 是 | 事件发生时间，ISO 8601 |
| `send_time` | string | 是 | 事件发送时间，ISO 8601 |
| `user_id` | string | 否 | 登录后用户 ID；未登录为空 |
| `anonymous_id` | string | 是 | 设备级匿名 ID，卸载重装可重置 |
| `session_id` | string | 是 | App 前台会话 ID |
| `platform` | string | 是 | `ios` / `android` / `web` |
| `app_version` | string | 是 | App 版本 |
| `build_number` | string | 否 | 构建号 |
| `os_version` | string | 否 | 系统版本 |
| `device_model` | string | 否 | 设备型号 |
| `network_type` | string | 否 | `wifi` / `cellular` / `offline` / `unknown` |
| `locale` | string | 否 | 语言地区 |
| `source_page` | string | 否 | 来源页面 |
| `ab_bucket` | string | 否 | 实验桶，多个实验用逗号分隔 |

## 4. 业务公共属性

| 字段 | 类型 | 示例            | 说明 |
| --- | --- |---------------| --- |
| `track_id` | string | `NO.00000001` | 轨迹外部 ID；仅在轨迹已创建后上报 |
| `track_type` | string | `hiking`      | 使用 `/track/types` 返回的英文 code |
| `is_running` | bool | `false`       | 轨迹是否仍在进行中 |
| `distance_m` | number | `5230`        | 距离，单位米，可按区间脱敏后上报 |
| `duration_s` | number | `3600`        | 时长，单位秒 |
| `city_code` | string | `110000`      | 城市编码；避免上报精确经纬度 |
| `locate_status` | string | `authorized`  | `authorized` / `denied` / `restricted` / `unknown` |
| `gps_quality` | string | `good`        | `good` / `weak` / `lost` / `unknown` |
| `companion_session_id` | string | -             | 同行会话 ID |
| `achievement_reward_id` | string | -             | 成就奖励 ID |
| `error_code` | string | -             | 业务或 SDK 错误码 |
| `error_message` | string | -             | 错误摘要，不带手机号、token、URL 签名等敏感信息 |

## 5. 页面曝光事件

| 事件名 | 页面 | 关键属性 |
| --- | --- | --- |
| `app_launch` | App 启动 | `launch_type`、`from_push` |
| `login_page_view` | 登录页 | `login_entry` |
| `home_map_view` | 首页地图 | `map_mode`、`city_code`、`locate_status` |
| `track_recommend_view` | 推荐路线列表 | `city_code`、`track_type`、`sort_type` |
| `track_detail_view` | 轨迹详情 | `track_id`、`track_type`、`source_page` |
| `track_record_view` | 轨迹记录页 | `track_type`、`locate_status`、`gps_quality` |
| `track_publish_view` | 轨迹发布/补全页 | `track_id`、`track_type` |
| `profile_view` | 我的页 | `achievement_level` |
| `achievement_center_view` | 成就中心 | `achievement_level`、`reward_count` |
| `companion_home_view` | 同行入口/附近房间 | `city_code`、`track_type` |
| `companion_session_view` | 同行房间 | `companion_session_id`、`member_count`、`role` |
| `feedback_page_view` | 意见反馈页 | `source_page` |

## 6. 核心漏斗事件

### 6.1 登录与账号

| 事件名 | 时机 | 关键属性 |
| --- | --- | --- |
| `login_sms_code_click` | 点击获取验证码 | `source_page` |
| `login_sms_code_success` | 验证码发送成功 | `duration_ms` |
| `login_sms_code_fail` | 验证码发送失败 | `error_code`、`error_message` |
| `login_submit` | 提交短信登录 | `source_page` |
| `login_success` | 登录成功 | `is_new_user`、`achievement_level`、`duration_ms` |
| `login_fail` | 登录失败 | `error_code`、`error_message` |
| `logout_click` | 点击退出登录 | `source_page` |

### 6.2 首页、地图与路线发现

| 事件名 | 时机 | 关键属性 |
| --- | --- | --- |
| `home_locate_click` | 点击定位 | `locate_status` |
| `home_locate_success` | 定位成功 | `city_code`、`duration_ms`、`gps_quality` |
| `home_locate_fail` | 定位失败 | `locate_status`、`error_code` |
| `home_map_mode_change` | 切换地图模式 | `from_mode`、`to_mode` |
| `track_filter_change` | 修改路线筛选 | `track_type`、`city_code`、`sort_type` |
| `track_search_submit` | 提交搜索 | `keyword_length`、`city_code` |
| `track_card_click` | 点击路线卡片 | `track_id`、`track_type`、`rank_index`、`source_page` |

### 6.3 轨迹记录与发布

| 事件名 | 时机 | 关键属性 |
| --- | --- | --- |
| `track_record_start_click` | 点击开始记录 | `track_type`、`locate_status` |
| `track_record_start_success` | 记录开始成功 | `track_type`、`gps_quality`、`duration_ms` |
| `track_record_pause_click` | 点击暂停 | `track_id`、`distance_m`、`duration_s` |
| `track_record_resume_click` | 点击继续 | `track_id` |
| `track_record_finish_click` | 点击结束 | `track_id`、`distance_m`、`duration_s` |
| `track_create_success` | 轨迹创建成功 | `track_id`、`track_type`、`is_running`、`earned_reward_count` |
| `track_create_fail` | 轨迹创建失败 | `track_type`、`error_code`、`error_message` |
| `track_upload_start` | 开始上传原始轨迹/截图 | `track_id`、`asset_type` |
| `track_upload_success` | 上传成功 | `track_id`、`asset_type`、`duration_ms`、`file_size_kb` |
| `track_upload_fail` | 上传失败 | `track_id`、`asset_type`、`error_code` |
| `track_publish_submit` | 提交发布/补全 | `track_id`、`track_type`、`has_screenshot`、`waypoint_count` |
| `track_publish_success` | 发布/补全成功 | `track_id`、`earned_reward_count` |
| `track_publish_fail` | 发布/补全失败 | `track_id`、`error_code`、`error_message` |
| `track_delete_success` | 删除轨迹成功 | `track_id`、`source_page` |

### 6.4 轨迹详情、收藏与导航

| 事件名 | 时机 | 关键属性 |
| --- | --- | --- |
| `track_collect_click` | 点击收藏 | `track_id`、`source_page` |
| `track_collect_success` | 收藏成功 | `track_id` |
| `track_collect_fail` | 收藏失败 | `track_id`、`error_code` |
| `track_uncollect_success` | 取消收藏成功 | `track_id` |
| `track_navigation_click` | 点击使用路线导航 | `track_id`、`source_page` |
| `track_navigation_report_success` | 导航使用上报成功 | `track_id` |
| `track_share_click` | 点击分享 | `track_id`、`share_channel` |

### 6.5 同行

| 事件名 | 时机 | 关键属性 |
| --- | --- | --- |
| `companion_create_click` | 点击创建同行 | `track_type`、`city_code` |
| `companion_create_success` | 创建成功 | `companion_session_id`、`track_type`、`max_members` |
| `companion_create_fail` | 创建失败 | `track_type`、`error_code` |
| `companion_join_click` | 点击加入同行 | `companion_session_id`、`source_page` |
| `companion_join_success` | 加入成功 | `companion_session_id`、`member_count` |
| `companion_join_fail` | 加入失败 | `companion_session_id`、`error_code` |
| `companion_mqtt_connect_success` | MQTT 连接成功 | `companion_session_id`、`duration_ms` |
| `companion_mqtt_connect_fail` | MQTT 连接失败 | `companion_session_id`、`error_code` |
| `companion_leave_success` | 离开成功 | `companion_session_id`、`role` |
| `companion_end_success` | 结束成功 | `companion_session_id`、`end_reason`、`role` |
| `companion_event_submit_success` | 关键事件上报成功 | `companion_session_id`、`event_type` |
| `companion_danmaku_toggle` | 弹幕开关切换 | `companion_session_id`、`enabled` |

### 6.6 成就

| 事件名 | 时机 | 关键属性 |
| --- | --- | --- |
| `achievement_reward_expose` | 奖励在页面可见 | `achievement_reward_id`、`reward_type` |
| `achievement_reward_click` | 点击奖励 | `achievement_reward_id`、`reward_type` |
| `achievement_level_rules_click` | 点击等级规则 | `achievement_level`、`source_page` |
| `achievement_level_rules_view` | 打开等级规则 H5 | `achievement_level`、`lang`、`is_dark` |
| `achievement_level_up_popup_view` | 等级提升弹窗曝光 | `from_level`、`to_level` |

### 6.7 意见反馈

| 事件名 | 时机 | 关键属性 |
| --- | --- | --- |
| `feedback_submit_click` | 点击提交反馈 | `image_count`、`content_length` |
| `feedback_submit_success` | 提交成功 | `image_count`、`duration_ms` |
| `feedback_submit_fail` | 提交失败 | `image_count`、`error_code`、`error_message` |
| `feedback_image_add_fail` | 添加图片失败 | `image_count`、`error_code` |

## 7. 性能与错误事件

| 事件名 | 时机 | 关键属性 |
| --- | --- | --- |
| `api_request_fail` | 关键接口失败 | `api_path`、`http_status`、`error_code`、`duration_ms` |
| `api_request_slow` | 关键接口慢请求 | `api_path`、`duration_ms`、`network_type` |
| `map_render_slow` | 地图渲染超过阈值 | `duration_ms`、`track_point_count` |
| `track_polyline_render_fail` | 轨迹线渲染失败 | `track_id`、`track_point_count`、`error_code` |
| `oss_sts_token_fail` | 获取 OSS STS 失败 | `asset_type`、`error_code` |
| `static_asset_load_fail` | 静态资源加载失败 | `asset_type`、`http_status` |

建议阈值：

- `api_request_slow`：移动网络超过 3000 ms，Wi-Fi 超过 1500 ms。
- `map_render_slow`：轨迹线首次可见超过 1000 ms。
- 批量列表类接口可单独设置 5000 ms 阈值。

## 8. 隐私与合规约束

- 不上报手机号、短信验证码、JWT、内部 token、OSS 签名 URL、图片原始 URL。
- 不默认上报原始经纬度、完整轨迹点、精确住址；分析城市分布使用 `city_code` 或行政区编码。
- `error_message` 必须做摘要化处理，不能直接透传服务端完整响应体。
- 用户退出登录后，后续事件只保留 `anonymous_id`，不继续带 `user_id`。
- 若接入第三方 SDK，应在隐私政策和权限弹窗中说明数据用途，并支持用户撤回授权。

## 9. 数据验收

上线前至少完成以下检查：

- 核对核心漏斗事件是否覆盖：`app_launch`、`login_success`、`home_map_view`、`track_record_start_success`、`track_create_success`、`track_publish_success`、`track_detail_view`、`track_collect_success`、`track_navigation_report_success`。
- 抽样检查公共属性完整率，`event_id`、`anonymous_id`、`session_id`、`platform`、`app_version` 不得为空。
- 验证失败场景：定位拒绝、网络断开、上传失败、接口 401/429/500。
- 验证离线补发：断网产生的事件恢复联网后补发，且 `event_id` 不变化。
- 验证隐私字段：日志和数据平台中不得出现手机号、token、验证码、OSS 签名 URL、原始经纬度数组。

## 10. 版本管理

- 事件新增：先更新本文，再由客户端实现；数据平台按本文建表或更新 schema。
- 属性新增：允许向后兼容；必填属性变更需给出灰度周期。
- 事件废弃：至少保留一个客户端版本周期，数据看板迁移完成后再下线。
- 服务端新增埋点接收接口或数据表时，同步更新 `docs/api/`、`mysql.sql`、repository 三实现和 `AGENTS.md`。

## 11. 数据存储方案

### 11.1 推荐架构

埋点数据不建议直接写入轨迹业务库的核心表，避免高频写入影响用户、轨迹、同行等在线业务。推荐按以下链路存储：

```
App SDK 本地队列
  → 埋点采集入口（POST /api/v1/analytics/events）
  → 服务端本地明细文件缓冲（JSONL / WAL）
  → 定时任务同步到 OSS 原始明细层 ODS
  → 可选导入 ClickHouse / Doris 明细表
  → 清洗明细层 DWD（字段标准化、去重、脱敏）
  → 汇总分析层 DWS/ADS（漏斗、留存、路径、性能看板）
```

### 11.2 客户端本地存储

- 使用本地轻量队列保存待上报事件，建议 SQLite 或 SDK 内置持久化队列。
- 每条事件以 JSON 保存，必须包含 `event_id`、`event_name`、`client_time`、`anonymous_id`、`session_id`。
- 触发上报条件：事件数达到批量阈值、App 进入后台、网络从离线恢复、定时 flush。
- 建议批量大小：20 到 50 条；单批 payload 控制在 256 KB 以内。
- 本地保留上限：最多 1000 条或 7 天，超过后优先丢弃最旧的非关键事件。

### 11.3 采集入口存储

服务端已提供 `POST /api/v1/analytics/events` 批量接收事件，并满足：

- 默认可接收未登录事件，因此不能强依赖业务 JWT；已登录时可带 `user_id` 或业务 token 做身份补充。
- 服务端只做基础校验、限流、脱敏和本地顺序落盘，不在请求链路里调用 OSS 或做复杂聚合。
- 写入失败时返回可重试错误，客户端用同一批 `event_id` 重试。
- 采集接口协议见 `docs/api/analytics.md`；调整字段、上限、认证策略或错误码时必须同步更新该文档、`docs/api/route-index.md` 和 `AGENTS.md`。

### 11.4 服务端本地落盘与 OSS 同步

客户端上报后，服务端可以先写入本地磁盘，再由进程内定时任务同步到 OSS。该方案可行，适合作为早期自建埋点链路，优点是接收接口不依赖 OSS 实时可用，写入延迟低，失败后可本地重试。

推荐落盘方式：

- 本地目录：`<LogDir>/analytics/events/`，按日期和小时分区，例如 `2026-06-12/15/events-000001.jsonl`。
- 文件格式：JSON Lines，一行一条事件；每行包含公共属性、业务属性、`server_time`、`schema_version`。
- 写入策略：接口完成校验和脱敏后 append 到当前活跃文件；写入成功即可向客户端返回成功。
- 文件轮转：按大小或时间轮转，当前服务端本地活跃文件按 64 MB 或 5 分钟轮转。
- 完成标记：活跃文件使用 `.writing` 后缀，轮转完成后 rename 为 `.jsonl`，只同步已关闭文件。
- 上传合并：同步任务先按 `event_date/hour` 时间分区合并小 JSONL 文件，单个 OSS part 目标上限为 128 MB，减少 OSS 小文件数量。
- 上传路径：`analytics/ods/event_date=yyyy-mm-dd/hour=HH/part-<instance_id>-yyyy-mm-dd-HH-*.jsonl`。
- 上传 Endpoint：服务端强制使用 `OSS_INTERNAL_ENDPOINT` 内网域名，未配置时同步任务失败并保留本地文件等待重试，不回退公网 Endpoint。
- 上传成功后：服务端删除参与该 part 合并的本地 JSONL 文件和临时 part，并尽力清理空的小时/日期目录；同步审计信息以 `analytics_sync_summaries` 为准。
- 同步摘要：每次 `analytics_sync` 执行都会写入 `analytics_sync_summaries`，记录扫描了哪些本地源文件、合并上传到哪个 OSS part、成功上传字节数、任务耗时和错误摘要；摘要写入失败只记录日志，不阻断文件上传结果。
- 上传失败后：保留原文件，定时任务按退避策略重试；连续失败时记录日志并触发告警。

服务端本地落盘必须满足以下约束：

- 不把埋点写入轨迹业务 MySQL 主库。
- 不在 HTTP 请求链路里同步上传 OSS，避免 OSS 抖动拖慢客户端请求。
- 本地磁盘必须设置容量上限；超过阈值时优先拒绝非关键埋点或返回可重试错误，避免挤占业务日志和静态资源空间。
- 多实例部署时，每个实例独立落盘和上传，OSS key 必须包含 `instance_id` 或 hostname，避免覆盖。
- 定时任务上传应具备幂等性；重复上传同一文件时，后续清洗层仍以 `event_id` 去重。
- 如果服务启动时 OSS 不可用，埋点接收仍可继续本地落盘；但磁盘到达水位线后必须降级。

该链路中的 OSS 原始文件就是 ODS 的低成本归档层；如果后续接入 ClickHouse / Doris，可由离线任务或流式任务从 OSS ODS 导入明细表。

### 11.5 原始明细层

原始明细层用于审计、回放、重新清洗，不直接服务产品看板。

推荐字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `event_id` | string | 事件唯一 ID，用于去重 |
| `event_name` | string | 事件名 |
| `user_id` | string | 登录用户 ID，未登录为空 |
| `anonymous_id` | string | 匿名设备 ID |
| `session_id` | string | App 前台会话 ID |
| `client_time` | datetime | 客户端事件发生时间 |
| `server_time` | datetime | 服务端接收时间 |
| `platform` | string | `ios` / `android` / `web` |
| `app_version` | string | App 版本 |
| `properties_json` | json/string | 业务属性 JSON |
| `ip_region` | string | IP 粗粒度地区，避免保存完整 IP |
| `ingest_date` | date | 分区日期 |

存储选择：

- 低成本归档：按天写入对象存储，路径如 `analytics/ods/event_date=2026-06-12/part-*.jsonl`。
- 高频查询：写入 ClickHouse / Doris 等列式库，按 `ingest_date` 分区，按 `event_name`、`user_id`、`anonymous_id` 建常用排序键或索引。
- 不推荐直接使用 MySQL 承载全量原始事件；MySQL 只适合保存少量配置、字典或低频管理数据。

### 11.6 清洗与汇总层

清洗层处理：

- 基于 `event_id` 去重。
- 校正客户端时间，保留 `client_time` 和 `server_time`，异常时间用 `server_time` 参与统计。
- 标准化 `track_type`、`platform`、`network_type` 等枚举。
- 移除或掩码手机号、token、OSS 签名 URL、原始经纬度等敏感信息。

汇总层产出：

- 日活、周活、月活。
- 登录转化、轨迹创建转化、轨迹发布转化、收藏/导航转化。
- 运动类型分布、城市分布、热门路线。
- 同行创建/加入/结束漏斗。
- 成就中心曝光、奖励点击、等级规则页访问。
- 接口失败率、上传失败率、地图渲染慢请求、定位失败率。

### 11.7 保留周期

建议默认保留：

| 数据层 | 保留周期 | 说明 |
| --- | --- | --- |
| 客户端本地队列 | 7 天 | 超期未上报丢弃 |
| 原始明细 ODS | 90 到 180 天 | 用于回溯与重新清洗 |
| 清洗明细 DWD | 180 到 365 天 | 用于明细分析 |
| 汇总 DWS/ADS | 长期 | 用于趋势看板 |

如涉及合规要求，应支持按用户维度删除或匿名化历史埋点数据。
