# 成就接口

> 公共请求、认证和错误响应见 [common.md](common.md)。

## 30. 成就中心摘要

获取当前用户成就首页数据，包括总 XP、等级、统计数据和最近获得的勋章奖励。当前 MVP 暂不返回里程碑奖励。

### 请求

```http
GET /api/v1/achievement/summary
Authorization: Bearer <token>
```

### 响应

```json
{
  "code": 0,
  "data": {
    "stats": {
      "total_xp": 120,
      "current_level": {"level": 1, "name": "初上路", "xp": 0},
      "next_level": {"level": 2, "name": "熟悉路线", "xp": 300},
      "level_progress": 0.4,
      "qualified_track_count": 1,
      "total_distance": 6000,
      "total_duration": 1800,
      "total_elevation_gain": 0,
      "companion_count": 0,
      "type_stats": {
        "running": {
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

### 字段说明

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `stats.total_xp` | int64 | 当前用户总成就 XP |
| `stats.current_level` | object | 当前等级 |
| `stats.next_level` | object/null | 下一等级；满级时为空 |
| `stats.level_progress` | number | 当前等级到下一等级的进度，范围 `0-1` |
| `stats.qualified_track_count` | int64 | 参与成就计算的有效轨迹数 |
| `stats.type_stats` | object | 按运动类型聚合的统计，key 为英文运动类型 code：`hiking` / `running` / `climbing` / `riding` / `driving` |
| `recent_rewards` | array | 最近获得的奖励，最多 10 条 |

---

## 31. 成就奖励列表

获取当前用户所有 MVP 勋章定义、进度和获得状态。当前 MVP 只返回 `type=badge`，里程碑体系暂不实现。

### 请求

```http
GET /api/v1/achievement/rewards
Authorization: Bearer <token>
```

### 响应

```json
{
  "code": 0,
  "data": {
    "stats": {
      "total_xp": 120,
      "current_level": {"level": 1, "name": "初上路", "xp": 0},
      "level_progress": 0.4,
      "qualified_track_count": 1,
      "type_stats": {}
    },
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

### 字段说明

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `rewards[].code` | string | 成就唯一编码，客户端可用于本地图标映射 |
| `rewards[].type` | string | 当前 MVP 固定为 `badge`；`milestone` 为后续里程碑体系预留 |
| `rewards[].category` | string | 成就分类，如 `新手` / `跑步` / `徒步` / `爬山` / `骑行` / `自驾` / `同行` |
| `rewards[].rarity` | string | `common` / `rare` / `epic` / `legendary` |
| `rewards[].target_value` | number | 达成目标值 |
| `rewards[].current_value` | number | 当前进度值；已获得奖励会返回目标值 |
| `rewards[].progress` | number | 进度比例，范围 `0-1` |
| `rewards[].earned` | bool | 是否已获得 |
| `rewards[].earned_at` | string/null | 获得时间，未获得时不返回 |

### 结算说明

- `track/create` 创建已完成轨迹（`is_running=false`）后会触发成就结算。
- `/track/:track_id/upload_cloud` 将进行中轨迹标记完成后会触发成就结算。
- `/track/:track_id/update` 更新已完成轨迹的补充字段后会尝试幂等结算。
- `/achievement/summary` 与 `/achievement/rewards` 会在查询前幂等补齐该用户历史有效轨迹的奖励记录，用于兼容历史轨迹或早期未结算数据。
- 同一用户同一成就只会发放一次。
- `404 Not Found`
  - `track_id` 对应轨迹不存在
- `500 Internal Server Error`
  - 服务端执行成就统计或奖励查询失败

### 示例（curl）

```bash
curl -X POST "http://<host>:<port>/api/v1/track_collect?track_id=trk2" \
  -H "Authorization: Bearer <token>"
```

---

## 32. 成长等级规则 H5

获取成长等级计算方式说明页。该接口返回 HTML，供客户端 WebView 嵌入到“等级规则”页面中。

### 请求

```http
GET /api/v1/achievement/level-rules.html
```

### Query 参数

| 参数 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `lang` | string | 否 | 中文 | 页面语言；传 `english` 时显示英文，其它值均按中文处理 |
| `is_dark` | string | 否 | `false` | 色调；传 `true` 时使用夜间模式，其它值均按日间模式处理 |

### 响应

- `200 OK`
- `Content-Type: text/html; charset=utf-8`

页面内容包括：

1. 有效轨迹口径。
2. 单次 XP 计算公式。
3. 不同运动类型的距离 XP 权重和单次上限。
4. 时长、爬升、内容完整度和同行奖励 XP。
5. Lv.1 至 Lv.10 的等级阈值。

### 客户端说明

- 该页面不需要登录态。
- 客户端可直接加载：`<Base URL>/achievement/level-rules.html`。
- 英文夜间模式示例：`<Base URL>/achievement/level-rules.html?lang=english&is_dark=true`。
- 页面为服务端内置静态 HTML，不依赖外部 JS/CSS 资源。

---

## 33. 运维刷新用户成就

按用户手机号手动触发成就系统刷新，用于历史轨迹因 `track_type` 口径不一致等原因未写入 `user_achievement_rewards` 时，运维侧对单个用户进行幂等补齐。

该接口不使用业务 JWT。服务端需配置环境变量 `OPS_INTERNAL_TOKEN`，调用方通过请求头 `X-Internal-Token` 传入相同 token；未配置时返回 `503 Service Unavailable`。

### 请求

```http
POST /api/v1/ops/achievement/refresh
Content-Type: application/json
X-Internal-Token: <ops-internal-token>
```

### 请求体

```json
{
  "phone": "13900002001"
}
```

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `phone` | string | 是 | 用户手机号，服务端会按该手机号查找用户并刷新该用户成就 |

### 响应

```json
{
  "code": 0,
  "data": {
    "user_id": 2001,
    "phone": "13900002001",
    "new_reward_count": 2,
    "earned_reward_count": 2,
    "qualified_track_count": 1,
    "total_xp": 105,
    "current_level": {"level": 1, "name": "初上路", "xp": 0},
    "next_level": {"level": 2, "name": "熟悉路线", "xp": 300},
    "new_rewards": [
      {
        "code": "first_track",
        "type": "badge",
        "category": "新手",
        "name": "第一条轨迹",
        "earned": true,
        "current_value": 1,
        "progress": 1
      }
    ]
  }
}
```

### 字段说明

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `new_reward_count` | int | 本次刷新新写入的奖励数量；重复调用应为 `0` |
| `earned_reward_count` | int | 刷新后该用户已获得的奖励总数 |
| `qualified_track_count` | int | 刷新后参与成就计算的有效轨迹数量 |
| `total_xp` | int | 刷新后按当前规则实时计算的总 XP |
| `new_rewards` | array | 本次新获得的奖励列表，结构与 `/achievement/rewards` 中单条奖励一致 |

### 错误码

| HTTP 状态码 | 场景 |
| --- | --- |
| `400` | 请求体非法或缺少 `phone` |
| `401` | 缺少 `X-Internal-Token` |
| `403` | `X-Internal-Token` 不匹配 |
| `404` | 手机号对应用户不存在 |
| `503` | 服务端未配置 `OPS_INTERNAL_TOKEN` |

---

