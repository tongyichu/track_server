# 首页地图模式接口

> 公共请求、认证和错误响应见 [common.md](common.md)；通用轨迹对象字段见 [models.md](models.md)。

## 48. 查询地图视野数据

首页切到地图模式后调用。服务端会根据当前视野和缩放级别返回不同粒度的数据：

- `route`：具体路线组。
- `area`：区域聚合气泡。
- `city`：城市聚合气泡。

服务端已使用 `track_route_groups` 聚合表作为数据源，`group_id` 是路线组 ID，不等同于单条轨迹 ID。路线组由后台离线任务对 `track_geo_indexes` 执行中心召回、距离/正反向起终点/折线相似关系图聚类并选择 medoid 计算几何结果；任务按历史成员重合复用 `group_id`，再离线写入 `area_id`。`route_group` 返回 `center` 与 `radius_m`，客户端可直接绘制聚合区域，不再依赖代表路线折线。

离线任务先使用 medoid 计算路线组几何属性，再按“管理员人工代表 > 已通过投稿中与 medoid 最相似者 > medoid”选择展示代表轨迹。投稿代表只影响 `representative_track_id` 和成员 role/source，不改变 center、radius、area_id 或路线介绍锚点。

**需要认证**

### 请求

```http
GET /api/v1/track-map/view?bbox=114.1000,22.2500,114.3500,22.4500&zoom=12&track_type=all
Authorization: Bearer <token>
```

首次进入附近模式也可以传：

```http
GET /api/v1/track-map/view?latitude=22.3000&longitude=114.1700&radius_m=10000&track_type=all
```

新版 Android 进入地图页默认选择“全部”，显式传 `track_type=all`；视野与路线组列表接口均支持该值。不传或传空值仍默认 `hiking`，保持旧客户端兼容。

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `bbox` | string | 地图视野模式必填 | `minLng,minLat,maxLng,maxLat` |
| `zoom` | number | 否 | 旧模式按城市、区域、路线组分层。按出发点浏览的缩放规则见下文 |
| `latitude` | number | 附近模式必填 | 用户当前位置纬度 |
| `longitude` | number | 附近模式必填 | 用户当前位置经度 |
| `radius_m` | int | 否 | 附近半径，默认 `10000`，最大 `50000` |
| `city_code` | string | 否 | 城市 Code |
| `browse_mode` | string | 否 | `departure` 启用按出发点浏览。省略时使用旧地图分层 |
| `district_id` | string | 否 | `departure` 模式下，区县气泡返回的 `area.id` |
| `scenic_id` | string | 否 | `departure` 模式下，景点气泡返回的 `area.id`，同时传出发区县或城市 |
| `track_type` | string | 否 | 运动类型 code；`all` 表示不过滤运动类型，不传或空值默认 `hiking`。兼容传中文名，如 `徒步` |
| `limit` | int | 否 | 默认 `100`，最大 `200` |

### 按出发点浏览

请求 `browse_mode=departure` 后，成功响应带 `browse_mode=departure`。查询使用已完成且公开的路线组成员，从已有空间索引读取起点。城市和区县由起点匹配内置边界，边界未覆盖时沿用轨迹的城市代码。附近和视野模式均按起点过滤。

- 仅传视野和缩放时，`zoom <= 9` 返回 `city`，`9 < zoom <= 11` 返回 `district`，`11 < zoom <= 13` 返回 `places`，`zoom > 13` 返回 `route`。景点层可以同时展示 `area_cluster`（景点）和 `route_group`（独立路线）。无下辖区县的城市可从城市气泡直接放大到景点和独立路线。
- 显式传 `district_id` 或 `scenic_id` 的调用仍可固定查看对应范围，分别返回景点层或路线层。Android 地图通过地图缩放切换层级。
- 区县内按公开成员的当前主景点关联分组。已保存的新增、改绑、移除和景点归档在下一次查询生效，无需等待路线组重算。一个出发范围内同一路线组只展示一次，多个成员景点不同时优先运营关联。同一组仍有有效关联时，移除单个成员的景点后可继续按其余关联展示。
- 在每个城市、区县或景点气泡内按 `group_id` 去重。已有公开轨迹实际从不同区县出发时，同一 `group_id` 可以在多个区县出现。路线标记用 `group_id` 与 `departure_scope_id` 联合识别。
- 路线标记的 `center` 和 `bbox` 表示出发点，`departure_track_id` 指向该方向的实际公开轨迹。另可返回 `district_name`、`distance_m` 及跨区终点 `arrival_name`。查询路线组详情仍用 `group_id`。

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
        "introduction_url": "/api/v1/track-map/groups/RG.00000001/introduction.html?v=3",
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
          "preview_images": {}
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
- `track_type=all` 时路线组仍返回各自真实的 `track_type`；城市、区域气泡按现有城市/区域范围汇总多种类型，气泡的 `track_type` 为空。`all` 只是查询条件，不会跨类型合并 RouteGroup，也不写入轨迹或地图索引。
- `area_cluster` 命中内置区域目录时会额外返回 `name`、`area_type`、`area`、`city_code`、`city_name`；`area_type` 当前为 `scenic_spot`（景区）或 `district`（区县）。
- 开启自动景区识别后，高德AOI采样命中或环绕 POI 点判定的主要景区使用稳定 `place-amap-*` 区域ID，下发 `name`、`area_type=scenic_spot`、`area.id`，并增加 `place_provider=amap`、`place_match_method=track_relation`。自动地点不保证有 `introduction_url`；气泡bbox是所属路线组的合并范围，不能作为景区真实边界。未匹配的路线组继续按区县或网格聚合，不会重复计数。流程、调用预算和配置见 [自动景区识别](../track-place-discovery.md)。
- `area.id` 是稳定区域 ID，不等同于随网格变化的 `cluster_id`；存在介绍内容时 `area.introduction_url` 指向公开 H5 页面。
- 路线组详情可选返回 `nearby_places`（最多3条），字段包括内部 `id`、`provider`、`provider_id`、`name`、`category`、`parent_id`、`latitude`、`longitude`、`coordinate_system`、`expires_at`。它只读取有效本地识别结果；附近地点不代表轨迹进入该地点，Google邻近结果不会被当成景区边界或归属依据。
- `area_id` 由 `track_route_group` 离线任务计算并写入 RouteGroup：使用 GCJ-02 `bounds` 预筛、Polygon/MultiPolygon 点包含最终判断，未提供 geometry 的人工区域按 `bounds` 兜底。接口不会实时执行空间匹配，只根据已存 `area_id` 查询名称和介绍信息；相同 `area_id` 的 RouteGroup 聚合成一个区域气泡，空 `area_id` 继续按原网格聚合。
- 开启自动识别时，采样或环绕规则判定的主要景区优先，其余使用路线组中心点匹配行政区。内置边界覆盖 DataV 全国索引的 2,875 个末级区域：2,840 个区县、34 个无下辖区县的城市/直辖单位及台湾省级兜底，涉及34个省级区域；快照日期不代表最新法定区划日期。行政区兜底统一沿用 `area_type=district` 和 `district-{adcode}`，未命中时继续网格聚合；无下辖区县的单位以自身代码作为 `city_code`，城市表缺失时使用区域名称补齐 `city_name`。关闭自动识别时沿用旧目录的优先级匹配。完整范围、更新工具与数据来源见 `internal/maparea/README.md`；发布后存量归属随 `track_route_group` 重聚合更新。
- 点击 `city_cluster` / `area_cluster` 后，客户端放大地图并重新请求本接口。
- 点击 `route_group` 后，请求路线组详情或路线组轨迹列表。
- `route_group.introduction_url` 仅在路线介绍已发布时返回，客户端可直接用 WebView 打开。

## 49. 查询地图路线组列表

直接查询具体路线组。地图主流程优先使用 `/track-map/view`；该接口适合客户端已确定要展示 route 粒度时调用。

**需要认证**

```http
GET /api/v1/track-map/groups?bbox=114.1000,22.2500,114.3500,22.4500&track_type=all
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
          "preview_images": {}
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

已发布介绍的路线组会在视野、列表和详情接口中返回相同的 `introduction_url`；未配置或草稿状态时省略该字段。

## 51. 查询路线组下的具体轨迹列表

**需要认证**

```http
GET /api/v1/track-map/groups/:group_id/tracks?limit=20
Authorization: Bearer <token>
```

返回同一路线组下的具体用户轨迹列表。第一版不额外返回路线组人数或轨迹总数，避免冷启动阶段暴露人气不足。

可选传 `departure_track_id`。该轨迹仍是当前路线组的公开、已完成成员时，会排在列表首位。客户端可使用地图标记返回的同名字段。

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

## 53. 查看聚合路线介绍页

聚合路线的公开介绍 H5，内容由管理中心维护。

**无需认证**

```http
GET /api/v1/track-map/groups/RG.00000001/introduction.html?lang=english&is_dark=true
```

仅已发布的路线介绍可访问。页面支持 `lang=english` 切换英文、`is_dark=true` 切换夜间模式；未知路线组、未配置介绍或草稿状态均返回 `404`。接口返回的 `introduction_url` 携带内容版本 `v`，用于缓存失效。

后台 `POST /admin/api/route-groups/:group_id/introduction/publish` 需要路线组操作权限。可提交与保存草稿相同的介绍结构，服务端校验并一次写入已发布版本。空请求体继续发布已保存的版本。

地图代表轨迹`cover_track`使用`preview_images`新版四图及可选`cover_url`（投稿封面），不再返回旧版`track_screenshot_url`。
