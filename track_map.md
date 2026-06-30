# 首页地图模式与路线发现能力方案

> 面向客户端、服务端和产品协作使用。
> 本文是功能设计与实现说明；客户端 HTTP 接口以 `docs/api/track-map.md` 和代码实现为准。

## 1. 背景与目标

轨迹 App 当前首页是单个轨迹列表。第一版不新增独立的“路线发现”一级入口，而是在首页增加“地图模式”切换能力：用户在首页可以在列表模式和地图模式之间切换。

地图模式背后的功能能力可以理解为“路线发现”：用户像在地图 App 中搜索附近酒店一样，在地图上发现附近或指定城市的公共轨迹路线。

产品信息架构：

```text
首页
├── 列表模式：展示单条用户轨迹列表
└── 地图模式：展示路线组，也就是路线发现能力
```

典型场景：

- 用户在首页点击地图模式切换按钮，默认展示当前位置附近 10 公里内的路线。
- 用户切换城市为“香港”，地图展示香港范围内的热门/可推荐路线。
- 多个用户可能走过同一条经典路线，例如“麦理浩径徒步路线”。这些用户上传的是不同 `Track`，但路线主体相同，地图上应先展示为一个路线入口。
- 用户点击“麦理浩径徒步路线”后，再查看所有相关的具体用户轨迹列表。

第一版目标：

- 地图首页展示“路线组”，而不是直接展示所有用户轨迹。
- 客户端只需要在首页提供列表/地图模式切换，不需要新增一个可见的“路线发现”按钮。
- 支持当前位置附近 10 公里查询。
- 支持指定城市查询。
- 首页地图模式必须按运动类型分类，默认分类 code 为 `hiking`（展示名 `徒步`）。
- 点击路线组后展示相关轨迹列表。
- RouteGroup 列表先不展示 `user_count` 和 `track_count`，避免冷启动阶段暴露人气不足。

非第一版目标：

- 不做复杂社交热度外显。
- 不做实时轨迹展示。
- 不做路线人工运营后台。
- 不强依赖高精度地图匹配算法，先用可解释、可迭代的中心点空间聚类规则。

## 2. 核心概念

### Track

用户实际上传的一条轨迹，对应现有 `track_records`。

特点：

- 归属于某个用户。
- 有具体开始时间、距离、截图、原始轨迹文件。
- 可以被收藏、导航、展示详情。

### TrackGeoIndex

服务端从 `Track.raw_track_url` 的原始轨迹点中解析出来的地图索引数据。

用途：

- 支持附近检索。
- 支持城市地图展示。
- 支持路线聚合。
- 避免每次地图查询都临时下载并解析原始轨迹文件。

建议包含：

- 起点、终点、中心点。
- 轨迹 bbox。
- 简化后的路线折线（仅用于轨迹索引和后续分析，不作为 RouteGroup 客户端展示字段）。
- 轨迹点数量、距离、城市、运动类型。
- 空间索引字段，例如 geohash / S2 cell / H3 cell。

### RouteGroup

多条相似 `Track` 聚合后的路线入口。

例如：

- “麦理浩径徒步路线”是一个 `RouteGroup`。
- A 用户、B 用户、C 用户分别走过的麦理浩径轨迹是多个 `Track`。
- 地图首页先展示 `RouteGroup`。
- 点击 `RouteGroup` 后再展示相关 `Track` 列表。

第一版 RouteGroup 列表不返回 `user_count` 和 `track_count`。服务端可以内部保存这些统计，用于排序和后续产品能力，但不在列表接口中对客户端展示。

RouteGroup 必须归属于明确的运动类型。同一地理路线在不同运动类型下应拆成不同 RouteGroup，例如“西湖环线徒步路线”和“西湖环线骑行路线”不能合并。

当前实现已落地 `track_route_groups` / `track_route_group_members` 聚合表。客户端始终按 RouteGroup 处理，`group_id` 为路线组 ID，不再等同于单条轨迹 ID。

## 3. 运动类型分类约束

首页地图模式的一级过滤维度是运动类型。

默认运动类型：

```text
hiking
```

客户端进入首页地图模式时，如果用户没有主动选择运动类型，应默认请求 `track_type=hiking`。服务端在客户端未传 `track_type` 时也应按 `hiking` 处理，避免一次返回多种运动类型导致地图语义混乱。

客户端展示要求：

- 地图页需要有运动类型切换入口，例如顶部 segmented control、下拉筛选或横向类型 tab。
- 地图 marker / 路线卡片必须能体现运动类型，可以通过运动类型图标、颜色、标签或路线名称体现。
- 路线名称建议包含运动类型语义，例如“麦理浩径徒步路线”“西湖骑行环线”。
- 如果名称本身已经强表达运动类型，仍建议保留图标或标签，方便扫视。

服务端聚合要求：

- RouteGroup 聚合必须以 `track_type` 作为强约束。
- 聚类时必须限定同一 `track_type`。
- RouteGroup 列表返回的每一项必须包含 `track_type`。

## 4. 客户端体验

### 4.1 入口与命名

不新增独立的“路线发现”按钮或一级入口。客户端在现有首页增加一个模式切换按钮即可。

推荐命名口径：

| 位置 | 建议文案 | 说明 |
| --- | --- | --- |
| 首页现有入口 | 沿用当前首页名称 | 不新增一级入口 |
| 模式切换按钮 | 地图模式 / 地图图标 | 从轨迹列表切换到地图视图 |
| 地图模式返回按钮 | 列表模式 / 列表图标 | 从地图视图切回轨迹列表 |
| 内部能力名 | 路线发现 | 用于文档、需求和服务端能力描述 |
| 地图页筛选标题 | 徒步路线 / 附近路线 / 香港徒步路线 | 面向用户表达当前筛选结果 |

客户端可见的信息架构应是“首页的两种浏览模式”，而不是“首页 + 路线发现两个入口”。

进入页面后：

1. 用户在首页点击地图模式切换按钮。
2. 客户端请求定位权限。
3. 如果拿到定位，默认以 `hiking` 分类查询当前位置附近 10 公里路线。
4. 如果没有定位权限，默认进入城市选择或使用上次城市。
5. 地图展示当前运动类型下的路线组 marker / 聚合区域。
6. 底部半屏列表展示当前视野内路线。

示意：

```text
列表模式：
[徒步 v] [城市/附近 v]                         [地图图标]
---------------------------------------------------------
轨迹卡片
轨迹卡片
轨迹卡片

地图模式：
[徒步 v] [附近 10km v]                         [列表图标]
---------------------------------------------------------
地图 + 路线组 marker / 聚合区域
底部路线卡片
```

### 4.2 地图展示

地图上展示的是路线入口，不是单个用户轨迹。

地图展示应随缩放级别变化：

- 默认进入 / 高缩放级别：展示具体 RouteGroup，例如“麦理浩径徒步路线”。
- 中等缩放级别：展示区域聚合气泡，例如“18 条路线”。
- 低缩放级别：展示城市聚合气泡，例如“香港 42 条路线”。
- 点击城市气泡：地图放大到该城市。
- 点击区域气泡：地图放大到该区域。
- 点击 RouteGroup marker 或聚合区域：打开路线详情半屏卡片。

这里的“路线数量”指 RouteGroup 数量，不是用户数，也不是用户上传的 Track 数量。该口径表达的是“这个区域有多少条可发现路线”，不会暴露冷启动阶段具体有几个人走过。

具体 RouteGroup 展示建议：

- marker / 路线卡片需要体现运动类型，例如“麦理浩径徒步”。
- 地图放大后展示聚合区域 + 名称 marker。
- 聚合区域和 marker 可按运动类型使用不同图标或颜色。

RouteGroup 列表不展示人数与轨迹数量，文案使用路线视角：

- “麦理浩径徒步路线”
- “西湖环线”
- “龙井村徒步路线”

避免第一版出现：

- “1 人走过”
- “2 条轨迹”
- “热度 3”

### 4.3 缩放分层策略

客户端每次地图拖动或缩放后，建议以当前地图视野查询数据，而不是只按固定 10 公里半径查询。

推荐交互：

1. 首次进入地图模式：用用户当前位置查询附近 10 公里，展示具体 RouteGroup。
2. 用户拖动地图：传当前 `bbox + zoom + track_type` 给服务端。
3. 用户缩小地图：服务端根据 zoom 返回区域聚合或城市聚合。
4. 用户放大地图：服务端返回更具体的 RouteGroup。
5. 客户端对拖动/缩放请求做 300-500ms debounce，避免频繁请求。

推荐分层：

| 地图状态 | 服务端返回 | 客户端展示 |
| --- | --- | --- |
| 默认进入 / 放大 | 附近 10km 或当前 bbox 内的 RouteGroup | 具体路线 marker / 聚合区域 |
| 缩小到区域级 | 网格或区域聚合数据 | 聚合气泡，例如“18 条路线” |
| 继续缩小到城市级 | 城市路线数量 | 城市气泡，例如“香港 42 条路线” |

当前 zoom 阈值为：`<=9` 返回城市聚合，`<=11` 返回区域聚合，更大返回具体 RouteGroup。具体阈值由客户端地图 SDK 和产品体验调试决定。服务端也可以根据 `bbox` 尺寸兜底判断返回粒度，避免不同平台 zoom 语义不一致。

### 4.4 城市模式

用户可以切换城市。

客户端传 `city_code` 给服务端，服务端返回该城市内的路线组。

城市切换不改变运动类型。比如当前是 `徒步`，从杭州切到香港后仍请求香港的徒步路线，除非用户主动切换运动类型。

城市模式下，如果服务端返回 bbox，客户端可自动缩放到结果范围；如果没有结果，客户端展示空态：

```text
这里还没有可展示的路线
```

### 4.5 路线详情与具体轨迹列表

用户点击 RouteGroup 后：

1. 客户端展示路线详情半屏卡片。
2. 卡片展示路线名称、运动类型、城市、代表路线预览。
3. 用户上滑或点击“查看相关轨迹”，请求具体轨迹列表。
4. 具体轨迹列表复用现有轨迹卡片样式，展示用户昵称、截图、距离、耗时、收藏状态等。

第一版详情也可以不展示轨迹数量，只展示“相关轨迹”。

## 5. 客户端接口

所有接口放在 `/api/v1` 的 auth 分组下，需要 JWT；完整协议见 `docs/api/track-map.md`。

### 5.1 查询地图视野数据

```http
GET /api/v1/track-map/view
```

用于地图模式的主查询接口。客户端传当前地图视野和缩放级别，服务端根据视野大小返回不同粒度的数据。

首次进入时，客户端可以传用户当前位置：

```text
latitude=22.3000
longitude=114.1700
radius_m=10000
track_type=hiking
```

地图拖动或缩放后，客户端传当前视野：

```text
bbox=114.1000,22.2500,114.3500,22.4500
zoom=12
track_type=hiking
```

参数：

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `bbox` | string | 地图视野模式必填 | 当前地图视野，格式 `minLng,minLat,maxLng,maxLat` |
| `zoom` | number | 否 | 地图缩放级别，用于判断返回粒度 |
| `latitude` | number | 首次附近模式必填 | 用户当前位置纬度 |
| `longitude` | number | 首次附近模式必填 | 用户当前位置经度 |
| `radius_m` | int | 否 | 半径，默认 10000 |
| `city_code` | string | 否 | 城市 Code，可用于城市筛选或兜底 |
| `track_type` | string | 否 | 运动类型 code，不传时默认 `hiking`；地图索引与路线组只保存 `hiking` / `running` / `climbing` / `riding` / `driving`，兼容中文名如 `徒步` |
| `limit` | int | 否 | 默认 100 |

服务端返回的 `view_level` 决定客户端展示方式：

| `view_level` | 含义 | 客户端展示 |
| --- | --- | --- |
| `route` | 具体路线组 | RouteGroup marker / 聚合区域 |
| `area` | 区域聚合 | 区域聚合气泡 |
| `city` | 城市聚合 | 城市聚合气泡 |

具体路线组响应示例：

```json
{
  "view_level": "route",
  "coordinate_system": "GCJ02",
  "items": [
    {
      "type": "route_group",
      "group_id": "rg_810000_hiking_000001",
      "name": "麦理浩径徒步路线",
      "city_code": "810000",
      "city_name": "香港",
      "track_type": "hiking",
      "center": {
        "latitude": 22.3942,
        "longitude": 114.2781
      },
      "radius_m": 6200,
      "bbox": {
        "min_latitude": 22.3541,
        "min_longitude": 114.2098,
        "max_latitude": 22.4320,
        "max_longitude": 114.3652
      },
      "cover_track": {
        "track_id": "NO.00000001",
        "track_screenshot_url": "/api/v1/static/screenshots/NO.00000001.jpg"
      }
    }
  ]
}
```

区域聚合响应示例：

```json
{
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
      "center": {
        "latitude": 30.2500,
        "longitude": 120.1400
      },
      "bbox": {
        "min_latitude": 30.2000,
        "min_longitude": 120.1000,
        "max_latitude": 30.3000,
        "max_longitude": 120.2000
      },
      "route_count": 18
    }
  ]
}
```

城市聚合响应示例：

```json
{
  "view_level": "city",
  "coordinate_system": "GCJ02",
  "items": [
    {
      "type": "city_cluster",
      "city_code": "810000",
      "city_name": "香港",
      "track_type": "hiking",
      "center": {
        "latitude": 22.3193,
        "longitude": 114.1694
      },
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
```

说明：

- `route_count` 表示符合当前 `track_type` 的 RouteGroup 数量。
- `route_count` 不表示用户数，也不表示具体 Track 数量。
- 区域聚合中心命中内置 GCJ-02 区域目录时，返回可选的 `name`、`area_type`、`area`、`city_code`、`city_name`。生成区县基线当前覆盖 36 个重点城市的 194 个核心区县，`catalog.json` 中的人工景区/区县条目可按稳定 ID 覆盖基线；景区匹配优先于区县，同优先级选择范围更小的区域，未命中时保持原数量气泡响应。
- `area.id` 是稳定区域 ID；有介绍内容时，公开的 `area.introduction_url` 可由客户端 WebView 打开，支持 `lang=english` 和 `is_dark=true`。
- 客户端点击 `city_cluster` 或 `area_cluster` 后，只需要放大地图并重新请求 `/track-map/view`。
- 客户端点击 `route_group` 后，请求路线组详情或具体轨迹列表。

### 5.2 查询地图路线组

```http
GET /api/v1/track-map/groups
```

该接口可作为高缩放级别下直接查询 RouteGroup 的接口，也可作为 `/track-map/view` 返回 `view_level=route` 时的兼容拆分接口。若实现了 `/track-map/view`，客户端地图主流程优先使用 `/track-map/view`。

支持两种查询模式。

附近模式：

```text
latitude=22.3000
longitude=114.1700
radius_m=10000
```

城市模式：

```text
city_code=810000
```

通用参数：

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `latitude` | number | 附近模式必填 | 用户当前位置纬度 |
| `longitude` | number | 附近模式必填 | 用户当前位置经度 |
| `radius_m` | int | 否 | 半径，默认 10000，建议最大 50000 |
| `city_code` | string | 城市模式必填 | 城市 Code |
| `track_type` | string | 否 | 运动类型 code，例如 `hiking` / `running`；不传时默认 `hiking`，兼容中文名 |
| `bbox` | string | 否 | 当前地图视野，格式 `minLng,minLat,maxLng,maxLat` |
| `zoom` | number | 否 | 地图缩放级别 |
| `limit` | int | 否 | 默认 50，最大 100 |

附近模式与城市模式至少选择一种。若同时传 `latitude/longitude` 和 `city_code`，服务端可优先使用 `bbox` 或附近模式，具体以最终接口约定为准。

响应示例：

```json
{
  "items": [
    {
      "group_id": "rg_810000_hiking_000001",
      "name": "麦理浩径徒步路线",
      "city_code": "810000",
      "city_name": "香港",
      "track_type": "hiking",
      "coordinate_system": "GCJ02",
      "center": {
        "latitude": 22.3942,
        "longitude": 114.2781
      },
      "radius_m": 6200,
      "bbox": {
        "min_latitude": 22.3541,
        "min_longitude": 114.2098,
        "max_latitude": 22.4320,
        "max_longitude": 114.3652
      },
      "cover_track": {
        "track_id": "NO.00000001",
        "track_screenshot_url": "/api/v1/static/screenshots/NO.00000001.jpg"
      }
    }
  ]
}
```

字段说明：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `group_id` | string | 路线组 ID |
| `name` | string | 路线展示名称 |
| `city_code` | string | 城市 Code |
| `city_name` | string | 城市名称 |
| `track_type` | string | 运动类型 |
| `coordinate_system` | string | 返回坐标系，客户端按地图 SDK 使用 |
| `center` | object | 聚合中心点，用于 marker |
| `radius_m` | number | 聚合区域覆盖半径，单位米，客户端可按中心点直接画区域 |
| `bbox` | object | 聚合区域范围，用于地图缩放 |
| `cover_track` | object | 代表轨迹，用于封面图或兜底跳转 |

注意：第一版列表不返回 `user_count`、`track_count`、`hot_score`。

### 5.3 查询路线组详情

```http
GET /api/v1/track-map/groups/:group_id/detail
```

响应示例：

```json
{
  "group_id": "rg_810000_hiking_000001",
  "name": "麦理浩径徒步路线",
  "city_code": "810000",
  "city_name": "香港",
  "track_type": "hiking",
  "coordinate_system": "GCJ02",
  "center": {
    "latitude": 22.3942,
    "longitude": 114.2781
  },
  "bbox": {
    "min_latitude": 22.3541,
    "min_longitude": 114.2098,
    "max_latitude": 22.4320,
    "max_longitude": 114.3652
  },
  "radius_m": 6200,
  "cover_track": {
    "track_id": "NO.00000001",
    "track_screenshot_url": "/api/v1/static/screenshots/NO.00000001.jpg"
  }
}
```

详情第一版同样不展示人数和轨迹数量。如果客户端需要列表入口，直接展示“相关轨迹”。

### 5.4 查询路线组下的具体轨迹列表

```http
GET /api/v1/track-map/groups/:group_id/tracks?limit=20&cursor=
```

返回字段建议复用现有 `TrackSummaryPage` 结构：

```json
{
  "items": [
    {
      "id": "NO.00000001",
      "user_id": 1001,
      "city_code": "810000",
      "city_name": "香港",
      "track_type": "hiking",
      "nickname": "山野用户",
      "user_avatar_url": "/api/v1/static/avatar/1001.jpg",
      "title": "麦理浩径一段",
      "distance": 10200.5,
      "duration": 12600,
      "elevation_gain": 680,
      "collected": false,
      "collect_count": 0,
      "navigate_count": 0,
      "track_screenshot_url": "/api/v1/static/screenshots/NO.00000001.jpg",
      "raw_track_url": "/api/v1/static/raw_tracks/NO.00000001.dat"
    }
  ],
  "next_cursor": "",
  "has_more": false
}
```

这里可以保留单条轨迹的 `collect_count` / `navigate_count`，因为它们是现有轨迹卡片字段，不代表路线组总人气。

## 6. 服务端设计

### 6.1 数据表建议

#### track_map_index_jobs

用于记录轨迹地图索引异步任务。轨迹完成接口只负责写入 pending job，不在请求主链路里下载 OSS 文件、解析轨迹点或聚合 RouteGroup。

```sql
CREATE TABLE track_map_index_jobs (
  track_id VARCHAR(64) NOT NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'pending',
  attempts INT NOT NULL DEFAULT 0,
  last_error VARCHAR(512) NOT NULL DEFAULT '',
  next_run_at DATETIME(6) NOT NULL,
  locked_at DATETIME(6) NULL,
  locked_by VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  succeeded_at DATETIME(6) NULL,
  last_failed_at DATETIME(6) NULL,
  PRIMARY KEY (track_id),
  KEY idx_track_map_index_pending (status, next_run_at, created_at)
);
```

任务状态：

| 状态 | 说明 |
| --- | --- |
| `pending` | 等待后台 worker 处理 |
| `processing` | 已被某个 worker 抢占处理 |
| `succeeded` | 索引构建成功 |
| `failed` | 保留枚举；当前实现失败后会回到 `pending` 并设置 `next_run_at` 延迟重试 |

#### track_geo_indexes

用于存储单条轨迹的地图索引。

建议字段：

```sql
CREATE TABLE track_geo_indexes (
  track_id VARCHAR(64) NOT NULL,
  city_code VARCHAR(16) NOT NULL DEFAULT '',
  track_type VARCHAR(32) NOT NULL DEFAULT '',
  coordinate_system VARCHAR(32) NOT NULL DEFAULT 'WGS84',
  start_lat DOUBLE NOT NULL,
  start_lng DOUBLE NOT NULL,
  end_lat DOUBLE NOT NULL,
  end_lng DOUBLE NOT NULL,
  center_lat DOUBLE NOT NULL,
  center_lng DOUBLE NOT NULL,
  min_lat DOUBLE NOT NULL,
  min_lng DOUBLE NOT NULL,
  max_lat DOUBLE NOT NULL,
  max_lng DOUBLE NOT NULL,
  distance DOUBLE NOT NULL DEFAULT 0,
  point_count INT NOT NULL DEFAULT 0,
  simplified_polyline_json MEDIUMTEXT,
  created_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  PRIMARY KEY (track_id),
  KEY idx_geo_city_type (city_code, track_type),
  KEY idx_geo_center (center_lat, center_lng)
);
```

#### track_route_groups

用于存储路线组。自动聚合只处理公开、已完成、正常状态轨迹对应的 `track_geo_indexes`，并严格按 `track_type` 分组。

当前实现字段：

```sql
CREATE TABLE track_route_groups (
  group_id VARCHAR(64) NOT NULL,
  name VARCHAR(128) NOT NULL DEFAULT '',
  track_type VARCHAR(32) NOT NULL DEFAULT '',
  status VARCHAR(16) NOT NULL DEFAULT 'active',
  city_codes_json TEXT,
  coordinate_system VARCHAR(32) NOT NULL DEFAULT '',
  center_lat DOUBLE NOT NULL,
  center_lng DOUBLE NOT NULL,
  radius_m DOUBLE NOT NULL DEFAULT 0,
  min_lat DOUBLE NOT NULL,
  min_lng DOUBLE NOT NULL,
  max_lat DOUBLE NOT NULL,
  max_lng DOUBLE NOT NULL,
  distance DOUBLE NOT NULL DEFAULT 0,
  representative_track_id VARCHAR(64) NOT NULL DEFAULT '',
  member_count BIGINT NOT NULL DEFAULT 0,
  source VARCHAR(16) NOT NULL DEFAULT 'auto',
  created_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  PRIMARY KEY (group_id),
  KEY idx_track_route_group_type_status (track_type, status),
  KEY idx_track_route_group_center (center_lat, center_lng),
  KEY idx_track_route_group_rep (representative_track_id)
);
```

说明：

- `member_count` 是服务端内部字段，用于排序和运营观察；第一版接口不返回该字段。
- `city_codes_json` 支持跨城市路线归属于多个城市。当前自动聚合先继承轨迹 `city_code`，后续可通过轨迹点反查城市后扩展为多城市。
- `source=auto/manual/mixed` 预留人工运营能力。自动任务不会覆盖人工改名后的展示诉求，后续 ops 接口可基于该字段做合并、拆分、改名和指定代表轨迹。

#### track_route_group_members

用于存储路线组和具体轨迹的关系。

```sql
CREATE TABLE track_route_group_members (
  group_id VARCHAR(64) NOT NULL,
  track_id VARCHAR(64) NOT NULL,
  similarity_score DOUBLE NOT NULL DEFAULT 0,
  match_direction VARCHAR(16) NOT NULL DEFAULT 'forward',
  role VARCHAR(16) NOT NULL DEFAULT 'member',
  source VARCHAR(16) NOT NULL DEFAULT 'auto',
  created_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  PRIMARY KEY (group_id, track_id),
  UNIQUE KEY uk_track_route_member_track (track_id),
  KEY idx_track_route_member_group (group_id, role, created_at)
);
```

### 6.2 索引构建流程

当轨迹完成或补齐原始轨迹文件后，服务端只触发索引任务入队。索引构建由后台 scheduler 异步执行；定时任务同时负责漏建补偿和失败重试。

主链路禁止同步依赖以下重操作：

- 下载 OSS raw track 文件。
- 解析大量轨迹点。
- 坐标转换。
- 计算 bbox / 中心点 / 起终点 / 简化折线。
- RouteGroup 中心点空间聚类。

这样可以保证 `track/create`、`/track/:track_id/upload_cloud`、`/track/:track_id/update` 的响应速度主要受轨迹记录写入影响，不受 OSS 下载和轨迹点解析影响。

```text
track/create(is_running=false)
或 /track/:track_id/upload_cloud
或 /track/:track_id/update 补齐 raw_track_url
  → 校验轨迹已完成、公开、raw_track_url 非空
  → 写入 track_map_index_jobs(status=pending)
  → 主接口立即返回

Scheduler(track_map_index，默认每 1 分钟)
  → 补偿扫描：查找已完成但缺少 track_geo_indexes 的轨迹，并补写 pending job
  → Claim pending jobs（小批量，默认 10 条；processing 超过 30 分钟可被重新抢占）
  → 读取 raw_track_url 对应原始轨迹点
  → 解析轨迹点（支持 JSON/GeoJSON/KML/KMZ）
  → 坐标统一到服务端内部坐标系
  → 计算起点、终点、中心点、bbox、简化折线
  → 写入 track_geo_indexes
  → 标记索引任务 succeeded

Scheduler(track_route_group，默认每天 04:00)
  → 全量读取 track_geo_indexes
  → 排除过短、点数过少或缺少运动类型的轨迹
  → 按 track_type 严格隔离，基于轨迹中心点做空间聚类
  → 计算每个 RouteGroup 的 center_lat/center_lng、radius_m、bbox、member_count
  → 重建替换 track_route_groups / track_route_group_members（MySQL 实现使用事务）
```

如果索引构建失败：

- 不影响轨迹创建/更新主流程。
- 记录到 `track_map_index_jobs.last_error`。
- 按 `next_run_at` 延迟重试，避免失败任务高频重刷。
- worker 中途退出导致的 `processing` 僵尸任务，超过 30 分钟后可被后续 worker 重新抢占。
- 定时补偿任务会持续发现漏建索引的历史轨迹。

OSS 下载要求：

- 后台索引 worker 复用服务端 raw track 本地缓存能力。
- 下载必须通过 `OSS_INTERNAL_ENDPOINT` 内网域名；未配置内网 endpoint 时下载失败并重试，不回退公网 endpoint。

### 6.3 路线聚合规则

当前实现使用简单、可解释、偏保守的中心点空间聚类规则。

聚类范围：

- 不强制同城市；跨城市路线可以合并，RouteGroup 会记录多个 `city_code`，城市聚合时每个城市都计数。
- 同运动类型。运动类型是硬性分组条件，不能跨类型聚合。
- 中心点距离在运动类型对应阈值内：徒步/跑步/爬山默认 3km，骑行 8km，自驾 15km。

聚合区域计算：

- `center_lat` / `center_lng` 使用成员轨迹中心点均值。
- `radius_m` 取 “成员中心点到 group 中心距离 + 成员 bbox 半径” 的最大值，避免长轨迹覆盖区域被低估。
- `bbox` 使用所有成员轨迹 bbox 的并集。
- 聚合任务会重建替换 `track_route_groups` / `track_route_group_members`，存量结果可清空重算；MySQL 实现使用事务。

中心点距离满足运动类型阈值则归入已有 RouteGroup，否则创建新的 RouteGroup。

不合并的情况：

- 不同运动类型。
- 中心点距离超过当前运动类型阈值。
- 距离太短、点数太少、GPS 数据质量明显不足的轨迹。

后续可升级为：

- DBSCAN / HDBSCAN 等密度聚类。
- S2/H3 网格聚合。
- 基于城市、行政区或业务热区的动态阈值。
- 人工合并/拆分聚合区域。

## 7. 排序策略

第一版虽然不展示人气数据，但服务端仍需要排序。

建议综合：

- 距离用户当前位置近。
- 路线完整度高，有原始轨迹、有截图。
- 代表轨迹质量高。
- 收藏数、导航数作为内部加权。
- 最近有更新的路线适当加权。

城市模式下建议优先：

1. 城市内经典/高质量路线。
2. 与当前地图视野 bbox 相交的路线。
3. 有代表截图和完整轨迹点的路线。

## 8. 坐标系约定

服务端内部建议统一存 WGS84，便于空间计算和跨地图供应商兼容。

客户端如果使用高德地图，展示通常需要 GCJ-02。

推荐接口明确返回：

```json
{
  "coordinate_system": "GCJ02"
}
```

并在服务端返回前转换成客户端需要的坐标系。

如果第一版服务端暂不做转换，则必须和客户端约定清楚，由客户端转换后再绘制，否则地图上会出现偏移。

## 9. 隐私与展示口径

地图功能只展示满足以下条件的轨迹：

- `status = normal`
- `is_running = false`
- `raw_track_url` 非空
- 已成功生成 `TrackGeoIndex`
- 不包含私密轨迹
- 不包含删除轨迹
- 不包含进行中轨迹

RouteGroup 是路线聚合入口，不展示用户实时位置。

第一版不在 RouteGroup 列表和详情中展示人数、轨迹数、热度值。

## 10. 分期计划

### MVP

- 新增首页地图模式所需的数据模型。
- 完成 `TrackGeoIndex` 构建。
- 已完成自动 RouteGroup 聚合。
- 提供地图视野查询接口，支持具体 RouteGroup、区域聚合、城市聚合三种返回粒度。
- 提供附近/城市 RouteGroup 查询接口。
- 提供 RouteGroup 下具体轨迹列表接口。
- 客户端完成地图页、城市切换、路线详情、相关轨迹列表。

### 第二阶段

- 支持路线人工命名和人工合并的服务端数据结构，后续补 ops 接口或管理后台。
- 支持更好的空间聚类算法。
- 支持路线详情页展示难度、爬升、距离区间、推荐季节等。
- 支持路线搜索。

### 第三阶段

- 支持路线热度榜。
- 支持用户贡献路线纠错。
- 支持路线专题运营。
- 支持离线地图包或路线包。

## 11. 对现有工程的影响

当前落地实现已涉及：

- 新增 HTTP 路由：`/track-map/view`、`/track-map/groups`、`/track-map/groups/:group_id/detail`、`/track-map/groups/:group_id/tracks`、公开的 `/track-map/areas/:area_id/introduction.html`。
- 新增模型：TrackGeoIndex、TrackMapIndexJob、地图视野响应模型。
- 新增模型：TrackRouteGroup、TrackRouteGroupMember。
- 新增 repository interface 与 MySQL / Mongo / memory 三套实现。
- 新增数据库表结构。
- 新增轨迹完成后的索引构建流程。
- 新增路线组离线聚合任务 `track_route_group`，默认每天 04:00。
- 新增管理中心聚合路线运营页，支持查看 RouteGroup、改名、合并、移除成员、指定代表轨迹。
- 新增内置地图区域目录：按 GCJ-02 边界框为区域聚合补充景区/区县语义，并提供统一的双语、明暗主题介绍 H5。
- 更新 `track_api.md` 与 `docs/api/track-map.md`。
- 更新 `AGENTS.md`。

由于现有工程支持 MySQL / Mongo / in-memory 降级，新增 repository 方法时必须三套实现同步补齐。
