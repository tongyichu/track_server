# 地图区域目录维护

地图区域目录由两层数据组成：

- `districts.json`：按 DataV 全国行政区索引批量生成的行政区边界，不手工逐条编辑。
- `districts.manifest.json`：生成时的覆盖统计、特殊层级清单、源文件 URL / SHA-256 与产物 SHA-256，用于核对快照。
- `catalog.json`：人工维护的景区，以及需要精修边界或介绍文案的区县。

加载时先读取 `districts.json`，再用 `catalog.json` 中相同 `id` 的人工条目覆盖。因此区县 ID 必须保持 `district-{adcode}`，景区 ID 使用稳定的 `scenic-{slug}`。

## 当前覆盖

自动景区识别入口为 `internal/service/track_place_service.go`，完整流程和配置见 [自动景区识别](../../docs/track-place-discovery.md)。开启 `TRACK_PLACE_ENABLED` 后，`catalog.json` 中旧手工景区矩形仅保留介绍兼容，不再决定路线组的自动归属；这里的目录继续作为行政区兜底及人工文案覆盖。

`mainland.json` 由 DataV.GeoAtlas `areas_v3/bound/100000_full.json` 中大陆31个省级行政区域生成，保留 Polygon/MultiPolygon及内洞，仅用来选择地图服务和坐标转换；不能当作景区边界。更新时筛选整数 adcode、排除710000/810000/820000，合并各要素为MultiPolygon并保留来源信息。`mainland.go` 为每个面预计算外包框后再做点包含判断。

当前快照 `2026-10-27` 覆盖 DataV `areas_v3/bound/all.json` 索引中的全部 2,875 个末级行政区域，涉及 34 个省级区域、370 个城市/归属代码，原有 194 个区域 ID 全部保留：

- 2,840 个区县边界（包括香港 18 区、澳门 8 个源数据分区）。
- 34 个源索引没有下辖区县的城市或省直辖行政单位，例如东莞、中山、嘉峪关、济源、海南省直辖县市与新疆部分直辖市，使用自身多边形，不伪造下级区县。
- 台湾省的数据源没有提供下级区县，使用省级多边形兜底；不能据此宣称台湾区县精度完整覆盖。

`admin_level` 保存源索引的 `district` / `city` / `province`，仅用于内置数据维护。对客户端沿用 `area_type=district` 表示行政区域兜底，ID 均为 `district-{adcode}`；优先级分别为 100 / 50 / 10，旧人工景区优先级仍更高。无下辖区县的单位以自身 adcode 作为 `city_code`；城市选择表缺少该代码时，区域气泡使用该单位名称补齐 `city_name`。

覆盖完整性以这份 DataV 索引为准，`data_version` 是采集快照日期，不代表源边界的法定生效日期，也不保证已同步所有最新撤县设区或更名。索引没有更细边界的区域按上述粒度保留；不得用邻近区县的矩形扩张填补。

行政区边界基线来自 DataV.GeoAtlas `areas_v3` 与官方文档保留的 `areas/bound` 区县 GeoJSON，`districts.json` 保存 GCJ-02 `Polygon` / `MultiPolygon` 及其预计算边界框，并逐条记录实际来源；上线使用前应由数据负责人确认数据授权、版本与目标地图 SDK 坐标口径。

## 更新步骤

从仓库根目录运行维护工具（只读公开数据源、写本地文件，不访问业务数据库）：

```bash
go run ./cmd/update-map-districts -cache build/maparea-source-2026-10-27 -version 2026-10-27
```

工具以全国索引的末级节点为目标，批量获取父级 `_full.json`，先尝试 `areas_v3/bound`，不可用时尝试 `areas/bound`；父级集合缺少某个节点时再按该节点 adcode 单独获取。所有面、离散区域和内洞均原样保留，边界框由全部坐标重新计算。源缓存放在被 Git 忽略的 `build/` 中；更新上游快照时使用新的缓存目录，避免混用旧文件。复现同一快照可加 `-offline`，完全使用缓存。

写入前必须满足：索引层级有效、所有末级节点都有边界、34 个省级区域均被覆盖、没有静默删除既有 ID，并通过业务实际使用的 `ParseCatalogFiles` 校验多边形、闭环、坐标与人工覆盖。任何失败都会阻止生成，不允许以部分下载结果覆盖目录。生成时一并输出覆盖清单与数据摘要；逐区域 `content_version` 根据内容生成，兼容介绍页缓存。

1. 核对 `districts.manifest.json` 的覆盖数、特殊层级和来源摘要；源索引若删除/更换既有 adcode，先处理旧 ID 兼容再更新。
2. 对需要精修的区县，在 `catalog.json` 增加同 ID 条目覆盖生成内容；不要修改已经发布的 ID。人工条目未提供 `geometry` 时会继承同 ID 生成区县的 geometry。
3. 景区仅在人工确认边界和介绍内容后加入 `catalog.json`，优先级应高于区县；暂未提供 geometry 的人工景区按 `bounds` 兜底匹配。
4. 同步覆盖说明和已有数据快照断言，按仓库默认约定只执行构建：

   ```bash
   go build ./...
   ```

5. 功能由用户验证；只有明确要求时才运行测试。
6. 发布新二进制后，存量路线组需要重新执行 `track_route_group` 聚合才会使用新边界。默认调度为每天 04:00；仅替换二进制不会即时回填数据库中的 `area_id`。自动景区识别仍在行政区匹配后覆盖为有效主景点，不因扩充行政区目录改变优先级。

`track_route_group` 离线任务生成 RouteGroup 后，先用 `bounds` 快速预筛，再用 `Polygon` / `MultiPolygon` 对 RouteGroup 中心做最终点包含判断，并把稳定 ID 写入 `track_route_groups.area_id`；MultiPolygon 的离散区域和 Polygon 内洞均按 GeoJSON 语义处理。geometry 不存在时才按 `bounds` 兜底，未命中时 `area_id` 保持为空。`/track-map/view` 请求只按已存 `area_id` 查询目录详情，不再执行点包含计算；空 `area_id` 继续使用原网格数量气泡。
