# 首页地图模式与路线发现能力方案

> 面向客户端、服务端和产品协作使用。
> 本文是功能设计与接口草案，具体字段以最终落地后的 `track_api.md` 和代码实现为准。

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
- 首页地图模式必须按运动类型分类，默认分类为 `徒步`。
- 点击路线组后展示相关轨迹列表。
- RouteGroup 列表先不展示 `user_count` 和 `track_count`，避免冷启动阶段暴露人气不足。

非第一版目标：

- 不做复杂社交热度外显。
- 不做实时轨迹展示。
- 不做路线人工运营后台。
- 不强依赖高精度地图匹配算法，先用可解释、可迭代的路线相似规则。

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
- 简化后的路线折线。
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

## 3. 运动类型分类约束

首页地图模式的一级过滤维度是运动类型。

默认运动类型：

```text
徒步
```

客户端进入首页地图模式时，如果用户没有主动选择运动类型，应默认请求 `track_type=徒步`。服务端在客户端未传 `track_type` 时也应按 `徒步` 处理，避免一次返回多种运动类型导致地图语义混乱。

客户端展示要求：

- 地图页需要有运动类型切换入口，例如顶部 segmented control、下拉筛选或横向类型 tab。
- 地图 marker / 路线卡片必须能体现运动类型，可以通过运动类型图标、颜色、标签或路线名称体现。
- 路线名称建议包含运动类型语义，例如“麦理浩径徒步路线”“西湖骑行环线”。
- 如果名称本身已经强表达运动类型，仍建议保留图标或标签，方便扫视。

服务端聚合要求：

- RouteGroup 聚合必须以 `track_type` 作为强约束。
- 查询候选 RouteGroup 时必须限定同一 `track_type`。
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
3. 如果拿到定位，默认以 `徒步` 分类查询当前位置附近 10 公里路线。
4. 如果没有定位权限，默认进入城市选择或使用上次城市。
5. 地图展示当前运动类型下的路线组 marker / 折线。
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
地图 + 路线组 marker / 折线
底部路线卡片
```

### 4.2 地图展示

地图上展示的是路线入口，不是单个用户轨迹。

建议展示形式：

- 低缩放级别：展示带运动类型的路线 marker，例如“麦理浩径徒步”。
- 中高缩放级别：展示代表路线折线 + 名称 marker。
- 点击 marker 或折线：打开路线详情半屏卡片。

RouteGroup 列表不展示人数与轨迹数量，文案使用路线视角：

- “麦理浩径徒步路线”
- “西湖环线”
- “龙井村徒步路线”

避免第一版出现：

- “1 人走过”
- “2 条轨迹”
- “热度 3”

### 4.3 城市模式

用户可以切换城市。

客户端传 `city_code` 给服务端，服务端返回该城市内的路线组。

城市切换不改变运动类型。比如当前是 `徒步`，从杭州切到香港后仍请求香港的徒步路线，除非用户主动切换运动类型。

城市模式下，如果服务端返回 bbox，客户端可自动缩放到结果范围；如果没有结果，客户端展示空态：

```text
这里还没有可展示的路线
```

### 4.4 路线详情与具体轨迹列表

用户点击 RouteGroup 后：

1. 客户端展示路线详情半屏卡片。
2. 卡片展示路线名称、运动类型、城市、代表路线预览。
3. 用户上滑或点击“查看相关轨迹”，请求具体轨迹列表。
4. 具体轨迹列表复用现有轨迹卡片样式，展示用户昵称、截图、距离、耗时、收藏状态等。

第一版详情也可以不展示轨迹数量，只展示“相关轨迹”。

## 5. 接口草案

所有接口建议放在 `/api/v1` 的 auth 分组下，需要 JWT。

### 5.1 查询地图路线组

```http
GET /api/v1/track-map/groups
```

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
| `track_type` | string | 否 | 运动类型，例如 `徒步` / `跑步`；不传时默认 `徒步` |
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
      "track_type": "徒步",
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
      "representative_polyline": [
        { "latitude": 22.3541, "longitude": 114.2098 },
        { "latitude": 22.3712, "longitude": 114.2481 },
        { "latitude": 22.3942, "longitude": 114.2781 }
      ],
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
| `center` | object | 路线中心点，用于 marker |
| `bbox` | object | 路线范围，用于地图缩放 |
| `representative_polyline` | array | 代表路线折线，已简化 |
| `cover_track` | object | 代表轨迹，用于封面图或兜底跳转 |

注意：第一版列表不返回 `user_count`、`track_count`、`hot_score`。

### 5.2 查询路线组详情

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
  "track_type": "徒步",
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
  "representative_polyline": [
    { "latitude": 22.3541, "longitude": 114.2098 },
    { "latitude": 22.3712, "longitude": 114.2481 },
    { "latitude": 22.3942, "longitude": 114.2781 }
  ],
  "cover_track": {
    "track_id": "NO.00000001",
    "track_screenshot_url": "/api/v1/static/screenshots/NO.00000001.jpg"
  }
}
```

详情第一版同样不展示人数和轨迹数量。如果客户端需要列表入口，直接展示“相关轨迹”。

### 5.3 查询路线组下的具体轨迹列表

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
      "track_type": "徒步",
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
  spatial_tokens JSON,
  created_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  PRIMARY KEY (track_id),
  KEY idx_geo_city_type (city_code, track_type),
  KEY idx_geo_center (center_lat, center_lng)
);
```

#### track_route_groups

用于存储路线组。

建议字段：

```sql
CREATE TABLE track_route_groups (
  id VARCHAR(64) NOT NULL,
  city_code VARCHAR(16) NOT NULL DEFAULT '',
  track_type VARCHAR(32) NOT NULL DEFAULT '',
  name VARCHAR(128) NOT NULL DEFAULT '',
  coordinate_system VARCHAR(32) NOT NULL DEFAULT 'WGS84',
  center_lat DOUBLE NOT NULL,
  center_lng DOUBLE NOT NULL,
  min_lat DOUBLE NOT NULL,
  min_lng DOUBLE NOT NULL,
  max_lat DOUBLE NOT NULL,
  max_lng DOUBLE NOT NULL,
  representative_track_id VARCHAR(64) NOT NULL DEFAULT '',
  representative_polyline_json MEDIUMTEXT,
  track_count BIGINT NOT NULL DEFAULT 0,
  user_count BIGINT NOT NULL DEFAULT 0,
  hot_score DOUBLE NOT NULL DEFAULT 0,
  source VARCHAR(16) NOT NULL DEFAULT 'auto',
  status TINYINT NOT NULL DEFAULT 1,
  created_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  PRIMARY KEY (id),
  KEY idx_route_group_city_type (city_code, track_type),
  KEY idx_route_group_center (center_lat, center_lng)
);
```

说明：

- `track_count` / `user_count` / `hot_score` 只作为服务端内部排序和后续运营能力使用。
- 第一版接口不返回这些字段。

#### track_route_group_members

用于存储路线组和具体轨迹的关系。

```sql
CREATE TABLE track_route_group_members (
  group_id VARCHAR(64) NOT NULL,
  track_id VARCHAR(64) NOT NULL,
  similarity_score DOUBLE NOT NULL DEFAULT 0,
  created_at DATETIME(6) NOT NULL,
  PRIMARY KEY (group_id, track_id),
  KEY idx_route_group_member_track (track_id)
);
```

### 6.2 索引构建流程

当轨迹完成或补齐原始轨迹文件后，服务端异步或同步触发索引构建。

```text
track/create(is_running=false)
或 /track/:track_id/upload_cloud
或 /track/:track_id/update 补齐 raw_track_url
  → 读取 raw_track_url 对应原始轨迹点
  → 解析轨迹点
  → 坐标统一到服务端内部坐标系
  → 计算起点、终点、中心点、bbox、简化折线
  → 写入 track_geo_indexes
  → 查找候选 RouteGroup
  → 判断是否可归入已有 RouteGroup
  → 写入 track_route_group_members
  → 更新 track_route_groups 的代表折线与内部统计
```

如果索引构建失败：

- 不影响轨迹创建/更新主流程。
- 记录日志。
- 后续可通过后台任务补偿重建。

### 6.3 路线聚合规则

第一版建议使用简单、可解释的规则。

候选范围：

- 同城市。
- 同运动类型。运动类型是硬性分组条件，不能跨类型聚合。
- bbox 有交集或中心点距离在一定范围内。

相似判断：

- 起点距离小于 500 米。
- 终点距离小于 500 米。
- 或允许反向路线：A 起点接近 B 终点，A 终点接近 B 起点。
- 总距离差小于 20%。
- 简化折线采样点平均距离小于 100-200 米。

满足阈值则归入已有 RouteGroup，否则创建新的 RouteGroup。

后续可升级为：

- Fréchet distance。
- Dynamic Time Warping。
- S2/H3 网格重合率。
- 地图匹配后的道路级相似度。
- 人工合并/拆分路线。

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
- 完成自动 RouteGroup 聚合。
- 提供附近/城市 RouteGroup 查询接口。
- 提供 RouteGroup 下具体轨迹列表接口。
- 客户端完成地图页、城市切换、路线详情、相关轨迹列表。

### 第二阶段

- 支持路线人工命名和人工合并。
- 支持更好的路线相似算法。
- 支持路线详情页展示难度、爬升、距离区间、推荐季节等。
- 支持路线搜索。

### 第三阶段

- 支持路线热度榜。
- 支持用户贡献路线纠错。
- 支持路线专题运营。
- 支持离线地图包或路线包。

## 11. 对现有工程的影响

真正落地实现时会涉及：

- 新增 HTTP 路由：`/track-map/groups`、`/track-map/groups/:group_id/detail`、`/track-map/groups/:group_id/tracks`。
- 新增模型：RouteGroup、TrackGeoIndex、RouteGroupMember。
- 新增 repository interface 与 MySQL / Mongo / memory 三套实现。
- 新增数据库表结构。
- 新增轨迹完成后的索引构建流程。
- 更新 `track_api.md`。
- 更新 `AGENTS.md`。

由于现有工程支持 MySQL / Mongo / in-memory 降级，新增 repository 方法时必须三套实现同步补齐。
