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

### 2.4 用户身份与账号切换

- `user_id` 表示事件发生时的登录用户，事件创建并进入本地队列后必须保持不变；发送时不得使用当前登录用户覆盖历史值。
- 客户端本地队列按身份域隔离为匿名队列和 `user:<user_id>` 队列。退出登录或切换账号前应尽力 flush 原用户队列，但 flush 失败不能阻塞正常退出。
- A 用户的事件未发出而 B 用户已经登录时，不得携带 B 的 Authorization 补发 A 的队列，也不得把事件改写成 B；应保留 A 队列，等 A 再次完成鉴权后补发，超过本地保留周期后按队列淘汰规则处理。
- 未登录时产生的事件始终保持 `user_id` 为空；即使登录后才补发，也不能追溯改写为新登录用户。
- 同一个上报批次只包含同一身份域的事件。登录用户事件必须携带与事件 `user_id` 一致的有效 JWT；匿名事件可不带 JWT。

## 3. 公共属性

所有事件必须携带以下公共属性。

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `event_id` | string | 是 | 单条事件唯一 ID，重试补发保持不变 |
| `event_name` | string | 是 | 事件名 |
| `client_time` | string | 是 | 事件发生时间，ISO 8601 |
| `send_time` | string | 是 | 事件发送时间，ISO 8601 |
| `user_id` | string | 否 | 事件发生时的登录用户 ID；未登录为空，进入队列后不可改写 |
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

### 4.1 推荐归因属性

第一版推荐功能只对支持新协议的首发客户端开放。服务端正式启用推荐开关后，当 `GET /api/v1/track/recommend/list` 响应包含 `recommendation` 以及 item 级推荐字段时，客户端必须在相关事件的 `properties` 中携带推荐上下文；旧内部测试包不纳入兼容和埋点验收范围。直接从搜索、个人主页、收藏列表等非推荐入口进入时不携带这些字段。若服务端因极端故障返回不含推荐字段的兼容 Legacy 响应，客户端继续上报原有通用事件，但不得伪造推荐上下文。

| 字段 | 类型 | 条件必填 | 说明 |
| --- | --- | --- | --- |
| `recommend_request_id` | string | 是 | 推荐响应中的 `recommendation.request_id`，用于串联同一次 Feed 的曝光、点击和后续转化 |
| `recommend_strategy` | string | 是 | 推荐响应中的 `recommendation.strategy`：`personalized` / `hybrid` / `legacy` |
| `candidate_source` | string | 单条内容事件必填 | item 中的主要候选来源，客户端原样回传，不自行推断 |
| `rank_index` | integer | 单条内容事件必填 | item 在整个 Feed Session 原始有序列表中从 1 开始的全局位置，不按页重排；前序内容失效被服务端过滤时允许跳号 |
| `city_code` | string | 是 | 本次推荐请求使用的城市 Code；未限定城市时传空字符串 |

`candidate_source` 第一版使用以下稳定值：

| 值 | 含义 |
| --- | --- |
| `content_affinity` | 用户内容偏好召回 |
| `city_hot` | 城市近期热门召回 |
| `followed_author` | 关注作者召回 |
| `quality` | 优质内容召回 |
| `exploration` | 新内容探索召回 |
| `legacy_fill` | 个性化候选不足时由 Legacy Recommend 补位 |
| `legacy` | 整次请求使用 Legacy Recommend |

同一轨迹命中多个召回源时，由服务端确定一个主要 `candidate_source` 并随 item 返回。客户端只能复制服务端返回值，不得根据页面位置、内容类型或本地规则自行生成。第一版推荐不包含实验分桶字段。

响应 item 的 `recommend_reason` 用于客户端直接展示，不作为埋点属性重复上报；数据分析使用稳定的 `candidate_source`，避免按可能调整或国际化的展示文案分组。

## 5. 页面曝光事件

| 事件名 | 页面 | 关键属性 |
| --- | --- | --- |
| `app_launch` | App 启动 | `launch_type`、`from_push` |
| `login_page_view` | 登录页 | `login_entry` |
| `home_map_view` | 首页地图 | `map_mode`、`city_code`、`locate_status` |
| `track_recommend_view` | 推荐路线列表 | `recommend_request_id`、`recommend_strategy`、`city_code`、`track_type`、`result_count`、`is_empty` |
| `track_detail_view` | 轨迹详情 | `track_id`、`track_type`、`source_page`；从推荐页进入时增加推荐归因属性 |
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
| `track_recommend_impression` | 推荐卡片达到真实曝光条件 | `track_id`、推荐归因属性 |
| `track_card_click` | 点击路线卡片 | `track_id`、`track_type`、`rank_index`、`source_page`；点击推荐卡片时增加推荐归因属性 |

#### 6.2.1 推荐曝光与归因规则

推荐事件按以下口径上报：

1. `track_recommend_view` 在推荐接口成功返回且推荐页首次实际可见时上报；同一个 `recommend_request_id` 只上报一次，继续翻页不重复上报。成功响应为 `items=[]` 时也要上报，并携带 `result_count=0`、`is_empty=true`；接口失败不报该成功曝光，改报 `api_request_fail`。
2. `track_recommend_impression` 只有在 App 位于前台、推荐页实际可见、卡片未被其他页面或弹层遮挡，并且卡片可见面积达到 50% 且连续持续至少 500 ms 时上报。卡片离屏、可见面积降到 50% 以下、切换页面、打开覆盖页面或 App 进入后台时立即取消计时，恢复后必须重新连续计满 500 ms；不同可见片段不能累计。
3. 同一个 `recommend_request_id + track_id` 在整个 Feed Session 中最多上报一次曝光。去重集合属于 Feed 上下文，不属于页面或列表组件生命周期；页面重建、横竖屏切换以及进入详情后返回都不能重置。App 进程重启后，客户端要么恢复该 Feed 对应的去重集合，要么丢弃旧 Feed 并请求新的 `recommend_request_id`，不能恢复旧 Feed 却清空去重集合。用户主动刷新获得新的 `recommend_request_id` 后，可以再次上报同一轨迹的曝光。
4. `rank_index` 使用服务端 item 返回的 `recommend_rank`，表示整个 Feed Session 的全局位置；客户端不能使用当前页内 RecyclerView 下标替代。
5. 点击推荐卡片进入详情页时，客户端应把该 item 的完整推荐上下文随页面跳转传递。由该详情页触发的收藏、取消收藏、导航和分享事件继续携带同一上下文。
6. 通过搜索、个人主页、收藏列表、外部链接等入口进入详情页时，不伪造推荐上下文。
7. 离线缓存和补发必须保留原始 `event_id`、`client_time`、事件发生时的 `user_id` 和推荐上下文，不能在发送时重新读取当前页面、当前账号或新 Feed 状态。
8. Feed Session 过期并静默刷新第一页后，以服务端新返回的 `recommend_request_id` 作为新的 Feed 上下文，只有新 Feed 内产生的曝光、点击和转化使用新 ID。过期前已进入本地队列的事件不改写；从旧 Feed 打开的详情页即使在 Session 过期后才发生收藏、取消收藏、导航或分享，也继续使用打开详情时保存的旧推荐上下文。Session 过期只影响分页，不使历史归因失效。

示例：

```json
{
  "event_id": "018f7d4a-2b6f-7f3f-9f3d-2a4fb8fdc002",
  "event_name": "track_recommend_impression",
  "client_time": "2026-10-04T10:00:00+08:00",
  "send_time": "2026-10-04T10:00:01+08:00",
  "anonymous_id": "device-uuid",
  "session_id": "session-uuid",
  "platform": "android",
  "app_version": "1.0.0",
  "properties": {
    "track_id": "NO.00001234",
    "recommend_request_id": "rec_01J...",
    "recommend_strategy": "hybrid",
    "candidate_source": "city_hot",
    "rank_index": 3,
    "city_code": "330100"
  }
}
```

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
| `track_collect_click` | 点击收藏 | `track_id`、`source_page`；存在推荐来源时增加推荐归因属性 |
| `track_collect_success` | 收藏成功 | `track_id`；存在推荐来源时增加推荐归因属性 |
| `track_collect_fail` | 收藏失败 | `track_id`、`error_code`；存在推荐来源时增加推荐归因属性 |
| `track_uncollect_success` | 取消收藏成功 | `track_id`；存在推荐来源时增加推荐归因属性 |
| `track_navigation_click` | 点击使用路线导航；仅表示意图，不代表导航已开始或完成 | `track_id`、`source_page`；存在推荐来源时增加推荐归因属性 |
| `track_navigation_report_success` | 用户已开始导航后主动结束，导航业务上报接口明确返回 200 | `track_id`；存在推荐来源时增加推荐归因属性 |
| `track_share_click` | 点击分享 | `track_id`、`share_channel`；存在推荐来源时增加推荐归因属性 |

导航统计按以下口径执行：

1. 点击导航只上报 `track_navigation_click`，不调用导航业务上报接口，也不计推荐强反馈。
2. “正常结束”限定为用户已经成功开始导航，随后主动结束本次导航；取消、导航初始化失败和开始前退出均不属于正常结束。
3. 正常结束时，客户端先持久化本地导航会话的“已尝试上报”标记，再调用一次 `POST /api/v1/track/:track_id/navigation/report`；同一个本地导航会话最多调用一次。
4. 只有明确收到 200 才生成 `track_navigation_report_success`。超时、断网、5xx 或其他响应不确定场景不自动重试导航业务接口，避免其非幂等写入重复增加 `navigate_count`。
5. App 崩溃或被强杀而未进入正常结束流程时不补报，第一版接受少计。
6. 上一条限制针对导航业务接口；已经生成的 `track_navigation_report_success` 是普通埋点事件，可以使用固定 `event_id` 进入埋点本地队列，并按批量采集接口规则重试和去重。

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
- 已进入本地队列的历史事件仍保留事件发生时的 `user_id`；上一条仅约束退出登录后新产生的事件，不能用于清空或改写历史身份。
- 若接入第三方 SDK，应在隐私政策和权限弹窗中说明数据用途，并支持用户撤回授权。

## 9. 数据验收

上线前至少完成以下检查：

- 核对核心漏斗事件是否覆盖：`app_launch`、`login_success`、`home_map_view`、`track_recommend_view`、`track_recommend_impression`、`track_card_click`、`track_record_start_success`、`track_create_success`、`track_publish_success`、`track_detail_view`、`track_collect_success`、`track_navigation_report_success`。
- 抽样检查公共属性完整率，`event_id`、`anonymous_id`、`session_id`、`platform`、`app_version` 不得为空。
- 推荐页面验收至少覆盖 personalized、hybrid、legacy 三种策略；`recommend_request_id`、`recommend_strategy`、`candidate_source`、`rank_index` 和 `city_code` 必须符合第 4.1 节定义。
- 验证曝光口径：App 后台、推荐页不可见、被覆盖、未进入可视区域、可见不足 50% 和快速滑过均不产生 `track_recommend_impression`；计时中断后重新计算连续 500 ms，页面重建和详情返回不重置 Feed 级去重。
- 验证空结果口径：推荐接口成功返回空列表且页面实际展示时，上报一次 `track_recommend_view`，其中 `result_count=0`、`is_empty=true`；接口失败不报该成功曝光。
- 验证归因上下文：推荐卡片点击进入详情后，详情、收藏、导航和分享事件携带与曝光事件一致的推荐上下文；从非推荐入口进入时不携带。
- 验证分页位置：第二页及后续页的 `rank_index` 使用 Feed 全局位置，不能从 1 重新计数。
- 验证 Session 过期：客户端静默刷新第一页且只自动重试一次；新 Feed 事件使用新的 `recommend_request_id`，已入队历史事件及旧详情页后续转化保持原推荐上下文。
- 验证 Session 仓储故障：`recommend_session_unavailable` 不触发新 Feed 或 Legacy 切换，保留原列表和游标退避重试。
- 验证失败场景：定位拒绝、网络断开、上传失败、接口 401/429/500。
- 验证离线补发和账号切换：断网事件恢复联网后 `event_id`、`client_time`、`user_id` 和推荐上下文均不变化；A 用户历史事件不能在 B 用户身份下发送或归属到 B。
- 验证批量确认：成功响应 `accepted` 必须等于发送条数；400/401/403 整批不重试原请求，413 拆包，429/500/503/网络超时使用原 `event_id` 整批退避重试。
- 验证导航口径：点击、取消、初始化失败和开始前退出不调用导航业务上报；正常结束时先写“已尝试”标记再调用且每个本地导航会话最多一次；只有明确 200 才生成 `track_navigation_report_success`，业务接口响应不确定不重试，但成功埋点本身可以按埋点队列规则重试。
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
- 队列必须按匿名身份和登录 `user_id` 分区，事件进入队列后不随登录态变化迁移或改写。
- 触发上报条件：事件数达到批量阈值、App 进入后台、网络从离线恢复、定时 flush。
- 建议批量大小：20 到 50 条；单批 payload 控制在 256 KB 以内。
- 本地保留上限：最多 1000 条或 7 天，超过后优先丢弃最旧的非关键事件。

第一版关键事件名单如下，队列达到容量上限时优先保留：

- 登录与推荐主漏斗：`login_success`、`track_recommend_view`、`track_recommend_impression`、`track_card_click`、`track_detail_view`；
- 推荐强转化：`track_collect_success`、`track_uncollect_success`、`track_navigation_report_success`、`track_share_click`；
- 内容生产：`track_record_start_success`、`track_create_success`、`track_publish_success`；
- 同行核心转化：`companion_create_success`、`companion_join_success`、`companion_end_success`。

如果队列中只剩关键事件且仍达到硬上限，允许淘汰最旧关键事件以保护 App 可用性，但必须记录本地丢弃计数；埋点队列不得阻塞登录、记录轨迹、收藏、导航等业务操作。

### 11.3 采集入口存储

服务端已提供 `POST /api/v1/analytics/events` 批量接收事件，并满足：

- 匿名事件默认可不携带业务 JWT；`user_id` 非空的登录用户事件必须携带同一用户的有效 JWT。`X-User-ID` 只能作为诊断字段，不能建立可信用户归属。
- 服务端从有效 JWT 生成 `server_user_id` 并校验批次内非空 `user_id`；身份不一致时整批拒绝，不能把客户端声明的 A 用户事件归属给当前 B 用户。
- 服务端不得用请求发生时的登录用户回填或覆盖事件的空 `user_id`；匿名事件即使登录后补发也保持匿名。推荐画像只消费经过服务端校验的登录用户事件。
- 服务端只做基础校验、限流、脱敏和本地顺序落盘，不在请求链路里调用 OSS 或做复杂聚合。
- 批量协议只支持整批确认，不返回逐条部分成功。`200 OK` 时 `accepted` 必须等于请求事件数；任一事件校验失败时整批拒绝。若响应缺失、`accepted` 小于发送数或发生 5xx，客户端保留整批并使用同一批 `event_id` 重试，数据侧按 `event_id` 去重。
- 429、500、503、网络中断和超时采用指数退避加抖动重试，建议 1、2、4、8、16、32、60 秒后封顶为 60 秒；响应带 `Retry-After` 时优先遵循。400/401/403 不原样自动重试，413 拆分批次后重试。
- 采集接口协议见 `docs/api/analytics.md`；调整字段、上限、认证策略或错误码时必须同步更新该文档、`docs/api/route-index.md` 和 `AGENTS.md`。

### 11.4 服务端本地落盘与 OSS 同步

客户端上报后，服务端可以先写入本地磁盘，再由进程内定时任务同步到 OSS。该方案可行，适合作为早期自建埋点链路，优点是接收接口不依赖 OSS 实时可用，写入延迟低，失败后可本地重试。

推荐落盘方式：

- 本地目录：`<LogDir>/analytics/events/`，按日期和小时分区，例如 `2026-06-12/15/events-000001.jsonl`。
- 文件格式：JSON Lines，一行一条事件；每行包含公共属性、业务属性、`server_time`、`schema_version`。
- 写入策略：接口完成校验和脱敏后 append 到当前活跃文件；写入成功即可向客户端返回成功。
- 文件轮转：按大小或时间轮转，当前服务端本地活跃文件按 64 MB 或 5 分钟轮转。
- 完成标记：活跃文件使用 `.writing` 后缀，轮转完成后 rename 为 `.jsonl`，只同步已关闭文件。
- 同步时间：沿用 `ANALYTICS_SYNC_CRON=0 3 * * *`，每天 03:00 扫描和上传已关闭文件。
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

推荐第一版的数据时效约定为：收藏、导航、关注等强行为直接从业务数据库聚合；`track_recommend_impression`、`track_card_click`、`track_detail_view` 等弱行为随每天 03:00 的批次进入 OSS 和下游清洗链路，按 T+1、24 小时级可用，不承诺实时进入用户画像或内容统计。同步或清洗失败时保留上一批可用统计，不能影响在线推荐接口。

### 11.5 原始明细层

原始明细层用于审计、回放、重新清洗，不直接服务产品看板。

推荐字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `event_id` | string | 事件唯一 ID，用于去重 |
| `event_name` | string | 事件名 |
| `user_id` | string | 事件发生时的登录用户 ID，未登录为空；离线补发时不可改写 |
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
- 推荐曝光、点击、详情、收藏和导航漏斗，以及按 `recommend_request_id`、策略、候选来源和全局排名的归因统计。
- 同行创建/加入/结束漏斗。
- 成就中心曝光、奖励点击、等级规则页访问。
- 接口失败率、上传失败率、地图渲染慢请求、定位失败率。

### 11.7 保留周期

第一版按以下周期保留：

| 数据层 | 保留周期 | 说明 |
| --- | --- | --- |
| 客户端本地队列 | 7 天 | 超期未上报丢弃 |
| 原始明细 ODS | 180 天 | 用于回溯与重新清洗；推荐原始事件采用相同周期 |
| 清洗明细 DWD | 180 天 | 用于推荐明细分析、归因和画像重建 |
| 汇总 DWS/ADS | 长期 | 用于趋势看板 |

如涉及合规要求，应支持按用户维度删除或匿名化历史埋点数据。
