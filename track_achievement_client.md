# 客户端成就系统对接文档

> Base URL: `http://<host>:<port>/api/v1`
>
> 所有成就接口均需要登录态：`Authorization: Bearer <token>`。

## 1. MVP 能力范围

第一期成就系统支持：

1. 总 XP 与总等级。
2. 有效轨迹统计。
3. 跑步、徒步、爬山、骑行、自驾五类基础勋章。
4. 新手基础勋章：第一条轨迹。
5. 同行基础勋章：首次完成关联同行的有效轨迹。
6. 成就中心摘要接口。
7. 成就奖励列表接口。

第一期不包含：

1. 全站排行榜。
2. 运营后台动态配置成就。
3. 节日挑战。
4. 复杂路线相似度和地点级探索。
5. 会员专属荣誉结算。
6. 里程碑体系；服务端一期不会下发 `type=milestone` 的奖励定义或新增记录。

## 2. 成就结算时机

服务端会在以下时机结算成就：

| 场景 | 说明 |
| --- | --- |
| `POST /track/create` 且 `is_running=false` | 创建已完成轨迹后立即结算 |
| `POST /track/:track_id/upload_cloud` | 进行中轨迹上传云端并标记完成后结算 |
| `PUT /track/:track_id/update` | 已完成轨迹补充距离、爬升、截图等字段后幂等结算 |

同一用户同一个成就只会发放一次。客户端可以在完成轨迹后重新请求成就摘要或奖励列表，判断是否有新奖励。

## 3. 有效轨迹口径

只有有效轨迹参与 XP 和成就计算。

| 类型 | 最短距离 | 最短时长 |
| --- | --- | --- |
| 跑步 | 500m | 3 分钟 |
| 徒步 | 500m | 5 分钟 |
| 爬山 | 300m | 5 分钟 |
| 骑行 | 1000m | 5 分钟 |
| 自驾 | 3000m | 5 分钟 |

通用要求：

1. `is_running=false`。
2. `status=1`，即正常轨迹。
3. `track_type` 必须是 `跑步` / `徒步` / `爬山` / `骑行` / `自驾` 之一。

## 4. 接口一：成就中心摘要

### 请求

```http
GET /api/v1/achievement/summary
Authorization: Bearer <token>
```

### 用途

用于成就首页、个人页成就模块、轨迹完成后的奖励摘要刷新。

### 响应示例

```json
{
  "code": 0,
  "data": {
    "stats": {
      "total_xp": 120,
      "current_level": {
        "level": 1,
        "name": "初上路",
        "xp": 0
      },
      "next_level": {
        "level": 2,
        "name": "熟悉路线",
        "xp": 300
      },
      "level_progress": 0.4,
      "qualified_track_count": 1,
      "total_distance": 6000,
      "total_duration": 1800,
      "total_elevation_gain": 0,
      "companion_count": 0,
      "type_stats": {
        "跑步": {
          "distance": 6000,
          "duration": 1800,
          "elevation_gain": 0,
          "track_count": 1,
          "xp": 120,
          "max_distance": 6000,
          "max_elevation_gain": 0
        }
      }
    },
    "recent_rewards": [
      {
        "code": "first_track",
        "type": "badge",
        "category": "新手",
        "name": "第一条轨迹",
        "description": "完成首条有效轨迹",
        "rarity": "common",
        "icon_url": "",
        "target_value": 1,
        "earned": true,
        "earned_at": "2026-05-31T12:00:00Z",
        "current_value": 1,
        "progress": 1
      }
    ]
  }
}
```

## 5. 接口二：成就奖励列表

### 请求

```http
GET /api/v1/achievement/rewards
Authorization: Bearer <token>
```

### 用途

用于完整成就中心：勋章墙、分类列表、未获得成就进度。

### 响应结构

```json
{
  "code": 0,
  "data": {
    "stats": {},
    "rewards": [
      {
        "code": "run_5k",
        "type": "badge",
        "category": "跑步",
        "name": "5K 完成",
        "description": "单次跑步距离达到 5km",
        "rarity": "common",
        "icon_url": "",
        "target_value": 5,
        "earned": true,
        "earned_at": "2026-05-31T12:00:00Z",
        "current_value": 5,
        "progress": 1
      }
    ]
  }
}
```

## 6. 客户端展示建议

### 6.1 成就首页

建议展示：

1. 当前等级：`stats.current_level.name`。
2. 总 XP：`stats.total_xp`。
3. 升级进度：`stats.level_progress`。
4. 最近获得：`recent_rewards` 前 3-5 个。
5. 分类入口：新手、跑步、徒步、爬山、骑行、自驾、同行。

### 6.2 勋章墙

使用 `GET /achievement/rewards`：

1. 按 `category` 分组。
2. 同组内可按 `earned desc`、`rarity`、`target_value` 排序。
3. `earned=false` 时展示灰态和进度条。
4. `progress` 可直接用于进度条，取值 `0-1`。

### 6.3 轨迹完成页

轨迹完成后建议：

1. 调用 `/achievement/summary`。
2. 对比本地缓存的最近奖励 `code`。
3. 若出现新的 `recent_rewards[].code`，展示获得勋章弹窗。

服务端 MVP 暂未在 `track/create` 响应里直接返回 `earned_rewards`，客户端通过成就摘要刷新实现。

## 7. 字段说明

### 7.1 等级字段

| 字段 | 说明 |
| --- | --- |
| `current_level.level` | 当前等级数字 |
| `current_level.name` | 当前等级名称 |
| `current_level.xp` | 当前等级起始 XP |
| `next_level` | 下一等级；满级时为空 |
| `level_progress` | 当前等级内进度，范围 `0-1` |

### 7.2 统计字段

| 字段 | 单位 | 说明 |
| --- | --- | --- |
| `total_distance` | 米 | 所有有效轨迹累计距离 |
| `total_duration` | 秒 | 所有有效轨迹累计时长 |
| `total_elevation_gain` | 米 | 所有有效轨迹累计爬升 |
| `qualified_track_count` | 次 | 有效轨迹数量 |
| `companion_count` | 次 | 关联同行 session 的有效轨迹数量 |
| `type_stats.*.distance` | 米 | 某运动类型累计距离 |
| `type_stats.*.max_distance` | 米 | 某运动类型单次最大距离 |
| `type_stats.*.elevation_gain` | 米 | 某运动类型累计爬升 |
| `type_stats.*.max_elevation_gain` | 米 | 某运动类型单次最大爬升 |

### 7.3 奖励字段

| 字段 | 说明 |
| --- | --- |
| `code` | 成就唯一编码，客户端可用来映射本地图标 |
| `type` | 一期固定为 `badge`；`milestone` 为后续里程碑体系预留 |
| `category` | 分类 |
| `rarity` | `common` / `rare` / `epic` / `legendary` |
| `target_value` | 目标值 |
| `current_value` | 当前值 |
| `progress` | 进度，范围 `0-1` |
| `earned` | 是否已获得 |
| `earned_at` | 获得时间，未获得时不返回 |

## 8. MVP 成就编码

| code | 分类 | 名称 |
| --- | --- | --- |
| `first_track` | 新手 | 第一条轨迹 |
| `first_companion` | 同行 | 首次同行 |
| `run_5k` | 跑步 | 5K 完成 |
| `run_10k` | 跑步 | 10K 完成 |
| `run_100k` | 跑步 | 跑步百公里 |
| `run_1000k` | 跑步 | 跑步千公里 |
| `hike_3k` | 徒步 | 徒步初体验 |
| `hike_15k` | 徒步 | 长线徒步 |
| `hike_100k` | 徒步 | 徒步百公里 |
| `hike_1000k` | 徒步 | 徒步千公里 |
| `hike_3000k` | 徒步 | 徒步三千里 |
| `climb_100m` | 爬山 | 小坡热身 |
| `climb_1000m` | 爬山 | 登高挑战 |
| `climb_8848m` | 爬山 | 珠峰累计 |
| `climb_100000m` | 爬山 | 十万米爬升 |
| `ride_10k` | 骑行 | 骑行起步 |
| `ride_50k` | 骑行 | 半百骑行 |
| `ride_1000k` | 骑行 | 骑行千公里 |
| `ride_5000k` | 骑行 | 骑行五千公里 |
| `drive_30k` | 自驾 | 城郊兜风 |
| `drive_300k` | 自驾 | 长途驾驶 |
| `drive_10000k` | 自驾 | 公路万里 |
| `drive_30000k` | 自驾 | 山河三万里 |

## 9. 注意事项

1. 客户端不要硬编码 `target_value`，以服务端返回为准。
2. 客户端可以硬编码 `code` 对应图标，但需要对未知 `code` 做兜底展示。
3. 距离字段单位是米，部分成就进度的 `current_value` 是 km 或 m，按 `description` 和 `category` 展示即可。
4. `recent_rewards` 是最近获得记录，不等于“本次轨迹获得记录”；完成页如需精确本次奖励，先用本地缓存做差异比较。
