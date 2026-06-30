# 首页地图模式接口

> 公共请求、认证和错误响应见 [common.md](common.md)；通用轨迹对象字段见 [models.md](models.md)。

## 48. 查询地图视野数据

首页切到地图模式后调用。服务端会根据当前视野和缩放级别返回不同粒度的数据：

- `route`：具体路线组。
- `area`：区域聚合气泡。
- `city`：城市聚合气泡。

服务端已使用 `track_route_groups` 聚合表作为数据源，`group_id` 是路线组 ID，不等同于单条轨迹 ID。路线组由后台离线任务基于 `track_geo_indexes` 的轨迹中心点聚类生成；`route_group` 返回 `center` 与 `radius_m`，客户端可直接绘制聚合区域，不再依赖代表路线折线。

**需要认证**

### 请求

```http
GET /api/v1/track-map/view?bbox=114.1000,22.2500,114.3500,22.4500&zoom=12&track_type=hiking
Authorization: Bearer <token>
```

首次进入附近模式也可以传：

```http
GET /api/v1/track-map/view?latitude=22.3000&longitude=114.1700&radius_m=10000&track_type=hiking
```

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `bbox` | string | 地图视野模式必填 | `minLng,minLat,maxLng,maxLat` |
| `zoom` | number | 否 | 地图缩放级别；`<=9` 返回城市聚合，`<=11` 返回区域聚合，更大返回路线组 |
| `latitude` | number | 附近模式必填 | 用户当前位置纬度 |
| `longitude` | number | 附近模式必填 | 用户当前位置经度 |
| `radius_m` | int | 否 | 附近半径，默认 `10000`，最大 `50000` |
| `city_code` | string | 否 | 城市 Code |
| `track_type` | string | 否 | 运动类型 code；默认 `hiking`。兼容传中文名，如 `徒步` |
| `limit` | int | 否 | 默认 `100`，最大 `200` |

### 响应：route

```json
{
  "code": 0,
  "data": {
    "view_level": "route",
    "coordinate_system": "GCJ02",
    "items": [
      {
        "type": "route_group",
        "group_id": "RG.00000001",
        "name": "麦理浩径徒步路线",
        "city_code": "810000",
        "city_name": "香港",
        "track_type": "hiking",
        "coordinate_system": "GCJ02",
        "center": { "latitude": 22.3942, "longitude": 114.2781 },
        "radius_m": 6200,
        "bbox": {
          "min_latitude": 22.3541,
          "min_longitude": 114.2098,
          "max_latitude": 22.432,
          "max_longitude": 114.3652
        },
        "cover_track": {
          "track_id": "NO.00000001",
          "track_screenshot_url": "/api/v1/static/screenshots/NO.00000001.jpg"
        }
      }
    ]
  }
}
```

### 响应：area

```json
{
  "code": 0,
  "data": {
    "view_level": "area",
    "coordinate_system": "GCJ02",
    "items": [
      {
        "type": "area_cluster",
        "cluster_id": "cell_30.2_120.1",
        "name": "西湖景区",
        "area_type": "scenic_spot",
        "area": {
          "id": "scenic-west-lake",
          "introduction_url": "/api/v1/track-map/areas/scenic-west-lake/introduction.html?v=1"
        },
        "city_code": "330100",
        "city_name": "杭州市",
        "track_type": "hiking",
        "center": { "latitude": 30.25, "longitude": 120.14 },
        "bbox": {
          "min_latitude": 30.2,
          "min_longitude": 120.1,
          "max_latitude": 30.3,
          "max_longitude": 120.2
        },
        "route_count": 18
      }
    ]
  }
}
```

### 响应：city

```json
{
  "code": 0,
  "data": {
    "view_level": "city",
    "coordinate_system": "GCJ02",
    "items": [
      {
        "type": "city_cluster",
        "city_code": "810000",
        "city_name": "香港",
        "track_type": "hiking",
        "center": { "latitude": 22.3193, "longitude": 114.1694 },
        "bbox": {
          "min_latitude": 22.1435,
          "min_longitude": 113.8257,
          "max_latitude": 22.5619,
          "max_longitude": 114.4295
        },
        "route_count": 42
      }
    ]
  }
}
```

说明：

- `route_count` 是 RouteGroup 数量，不是用户数，也不是具体轨迹数。
- `area_cluster` 命中内置区域目录时会额外返回 `name`、`area_type`、`area`、`city_code`、`city_name`；`area_type` 当前为 `scenic_spot`（景区）或 `district`（区县）。
- `area.id` 是稳定区域 ID，不等同于随网格变化的 `cluster_id`；存在介绍内容时 `area.introduction_url` 指向公开 H5 页面。
- 区域目录使用 GCJ-02 边界框，并按“景区优先于区县、同优先级选择更小区域”的规则匹配聚合中心。当前区县基线覆盖 36 个重点城市的 194 个核心区县，完整清单见 `internal/maparea/README.md`；未命中时省略上述可选字段，客户端继续只展示路线数量。
- 点击 `city_cluster` / `area_cluster` 后，客户端放大地图并重新请求本接口。
- 点击 `route_group` 后，请求路线组详情或路线组轨迹列表。

## 49. 查询地图路线组列表

直接查询具体路线组。地图主流程优先使用 `/track-map/view`；该接口适合客户端已确定要展示 route 粒度时调用。

**需要认证**

```http
GET /api/v1/track-map/groups?bbox=114.1000,22.2500,114.3500,22.4500&track_type=hiking
Authorization: Bearer <token>
```

参数同 [查询地图视野数据](#48-查询地图视野数据)。

响应：

```json
{
  "code": 0,
  "data": {
    "items": [
      {
        "type": "route_group",
        "group_id": "NO.00000001",
        "name": "麦理浩径徒步路线",
        "city_code": "810000",
        "city_name": "香港",
        "track_type": "hiking",
        "coordinate_system": "GCJ02",
        "center": { "latitude": 22.3942, "longitude": 114.2781 },
        "radius_m": 6200,
        "bbox": {
          "min_latitude": 22.3541,
          "min_longitude": 114.2098,
          "max_latitude": 22.432,
          "max_longitude": 114.3652
        },
        "cover_track": {
          "track_id": "NO.00000001",
          "track_screenshot_url": "/api/v1/static/screenshots/NO.00000001.jpg"
        }
      }
    ]
  }
}
```

## 50. 查询路线组详情

**需要认证**

```http
GET /api/v1/track-map/groups/:group_id/detail
Authorization: Bearer <token>
```

`:group_id` 传路线组 ID，例如 `RG.00000001`。

响应 `data` 为单个 `route_group` 对象，字段同 [查询地图路线组列表](#49-查询地图路线组列表)。

## 51. 查询路线组下的具体轨迹列表

**需要认证**

```http
GET /api/v1/track-map/groups/:group_id/tracks?limit=20
Authorization: Bearer <token>
```

返回同一路线组下的具体用户轨迹列表。第一版不额外返回路线组人数或轨迹总数，避免冷启动阶段暴露人气不足。

响应：

```json
{
  "code": 0,
  "data": {
    "items": [
      {
        "id": "NO.00000001",
        "user_id": 1001,
        "city_code": "810000",
        "city_name": "香港",
        "track_type": "hiking",
        "nickname": "山野用户",
        "user_avatar_url": "/api/v1/static/avatars/1001.jpg",
        "title": "麦理浩径一段",
        "distance": 10200.5,
        "duration": 12600,
        "avg_speed_kmh": 2.9,
        "calories_burned": 600,
        "elevation_gain": 680,
        "collected": false,
        "collect_count": 0,
        "navigate_count": 0,
        "track_screenshot_url": "/api/v1/static/screenshots/NO.00000001.jpg",
        "track_no_map_bg_screenshot_url": "",
        "raw_track_url": "/api/v1/static/raw_tracks/NO.00000001.dat"
      }
    ],
    "has_more": false
  }
}
```

## 52. 查看地图区域介绍页

区县或景区的公开介绍 H5，用于客户端 WebView。页面内容来自随服务二进制发布的版本化区域目录，不包含用户或轨迹隐私数据。

**无需认证**

```http
GET /api/v1/track-map/areas/scenic-west-lake/introduction.html
```

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `area_id` | string | 是 | `area_cluster.area.id` 返回的稳定区域 ID |
| `lang` | string | 否 | 传 `english` 显示英文；默认中文 |
| `is_dark` | string | 否 | 传 `true` 使用深色主题 |

仅当区域目录中存在介绍内容时，`area_cluster.area.introduction_url` 才会返回。未知 `area_id` 返回 `404`。
