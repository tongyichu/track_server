# API 通用模型

> 公共请求、认证和错误响应见 [common.md](common.md)。

## 轨迹对象（Track）字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | 轨迹 ID |
| `user_id` | int64 | 用户 ID |
| `session_id` | string | 关联的同行会话 ID；非同行轨迹为空字符串 |
| `city_code` | string | 城市 Code（用于标识轨迹所属城市；城市/省份映射关系维护在配置文件中） |
| `locate_addr` | string | 轨迹的具体位置信息 |
| `track_type` | string | 轨迹类型 code，例如 `hiking` / `running` / `climbing` / `riding` / `driving` |
| `source_tag` | string | 轨迹来源/运营标签；允许值为空字符串或 `manual_seed`（人工录入冷启动轨迹）；普通列表摘要不返回该字段 |
| `coordinate_system` | string | 坐标系，例如 `WGS84` / `GCJ02` / `BD09` |
| `title` | string | 轨迹标题 |
| `start_time` | string | 开始时间（RFC3339/ISO8601，服务端序列化时间格式） |
| `end_time` | string | 结束时间（RFC3339/ISO8601，服务端序列化时间格式） |
| `distance` | number | 距离（米） |
| `duration` | int | 时长（秒） |
| `avg_speed_kmh` | number | 平均速度（km/h） |
| `calories_burned` | number | 热量消耗（千卡） |
| `elevation_gain` | int | 累计爬升（米） |
| `raw_track_url` | string | 原始轨迹文件可下载链接（服务端本地缓存 URL，例如 `/api/v1/static/raw_tracks/<track_id>.dat`） |
| `track_screenshot_url` | string | 轨迹截图可下载链接（服务端本地缓存 URL，例如 `/api/v1/static/screenshots/<track_id>.jpg`） |
| `track_no_map_bg_screenshot_url` | string | 无地图背景的轨迹路线截图可下载链接（服务端本地缓存 URL，例如 `/api/v1/static/screenshots/<track_id>_no_map_bg.jpg`） |
| `is_running` | bool | 是否进行中 |
| `status` | int | 轨迹状态：`0` 删除，`1` 正常，`2` 私密 |
| `created_at` | string | 创建时间 |
| `updated_at` | string | 更新时间 |
| `deleted_at` | string | 删除时间（软删除；未删除时为空或不返回） |
| `earned_rewards` | array | 本次轨迹完成即时新获得的成就奖励；仅完成轨迹结算产生新奖励时返回，结构同 `/achievement/rewards` 单条奖励 |

---

