# 地图区域目录维护

地图区域目录由两层数据组成：

- `districts.json`：批量生成的重点城市核心区县基线数据，不手工逐条编辑。
- `catalog.json`：人工维护的景区，以及需要精修边界或介绍文案的区县。

加载时先读取 `districts.json`，再用 `catalog.json` 中相同 `id` 的人工条目覆盖。因此区县 ID 必须保持 `district-{adcode}`，景区 ID 使用稳定的 `scenic-{slug}`。

## 当前覆盖

区县基线当前覆盖 16 个重点城市、90 个核心区县：北京、天津、上海、重庆、广州、深圳、杭州、南京、成都、武汉、西安、苏州、厦门、青岛、长沙、昆明。

行政区边界基线来自 DataV.GeoAtlas `areas_v3` 区县 GeoJSON，`districts.json` 保存根据 GeoJSON 计算出的 GCJ-02 边界框；上线使用前应由数据负责人确认数据授权、版本与目标地图 SDK 坐标口径。

## 更新步骤

1. 从已确认的数据源获取城市下属区县 GeoJSON。
2. 根据产品覆盖范围选择区县 adcode。
3. 从每个 Polygon/MultiPolygon 的全部坐标计算 `bounds`，生成 `districts.json`，同步更新 `data_version`。
4. 对需要精修的区县，在 `catalog.json` 增加同 ID 条目覆盖生成内容；不要修改已经发布的 ID。
5. 景区仅在人工确认边界和介绍内容后加入 `catalog.json`，优先级应高于区县。
6. 执行：

   ```bash
   go test ./internal/maparea ./internal/service ./internal/handler
   go build ./...
   ```

7. 使用区县内部、相邻区县边界、景区内部和景区外部坐标做人工抽查。

当前匹配依据是区域边界框，重点城市继续扩容前应优先升级为 Polygon/MultiPolygon 点包含判断，减少相邻区县边界框重叠产生的误标。
