# 本人探索足迹

两个接口均使用业务 JWT，只读当前用户，响应 `Cache-Control: no-store`，不接受目标用户 ID；个人私密轨迹只参与本人统计。不读取 GPS、下载预览、生成地图或请求供应商/AI。

## 全国探索统计

`GET /api/v1/track/activity/footprint?track_type=hiking`

- 省略 `track_type` 为全部运动；有值时先归一运动类型别名，再筛选后聚合。不使用 `all` 哨兵。全部历史、正常/私密、非进行中且原始文件 URL 非空的已上传轨迹参与。
- `data.provinces` / `data.cities` 是当前启用的国内省份与可选择城市，包含真实的零次数目录节点供地图和本机未上传记录合并；是否点亮只看 `track_count>0`，不将目录节点总数展示为探索数量。
- 节点字段：`id` 为稳定 `region_id`，`name/name_en`、`aliases`（兼容旧目录 ID/adcode）、`map_code`（目录明确提供的高德 adcode，关联矢量几何）、可选 `latitude/longitude`、`track_count`、`distance_meters`、`city_count`、`scenic_spot_count`。城市另有 `province_id`；省级直辖市兼具省与可选择城市身份。
- 目录及父子关系以持久化 `regions.Service` 为权威；不通过 `region_id` 前缀推断省份，不修改旧记录，不根据轨迹点或邻近城市猜测归属。城市中心缺失时保留统计，不伪造坐标。
- 景区来自 `track_scenic_spot_links` 明确关联且当前 `active` 的目录条目，按内部景区 ID 去重；归档/合并目录隐藏，不统计附近候选。同一景区多次运动只算一个，省份和全国分别再次去重。
- 顶层 `scenic_spot_count` 为国内全部去重景区数；`unlocated_track_count` 为无法解析至当前可选城市的轨迹数；`outside_china_track_count` 为明确解析到境外城市而未绘入全国图的次数。未知城市不算境外，跨省自驾只按轨迹归属城市计入，不自动点亮途经地区。
- MySQL/Mongo/memory 的 `ActivityFootprintRepository` 仅投影本人合格轨迹的 ID、城市、类型、距离、开始日期，不读取整份轨迹/图片/GPS。service 在筛选后内存聚合，景区关联及目录每批最多 200 个批量读取，无逐轨迹补详情、新表或 schema 迁移。仓储错误正常上抛，不返回伪造空统计。

## 对应轨迹列表

`GET /api/v1/track/activity/footprint/tracks?region_id=CN000118&track_type=hiking&limit=20&cursor=...`

- `region_id` 必填，为统计中返回的省份或城市稳定 ID。城市精确匹配；省份按目录下已解析城市匹配，包含直辖市自身。未知地区 400。
- 使用相同本人/完成状态/原始文件/类型口径，先筛选再分页，按 `start_time DESC,id DESC`，复用现有不透明游标。`limit` 默认 20、最大 50。
- 返回 `data.items`（现有 TrackSummary 形状）、`has_more`、有后续页时的 `next_cursor`。仅当前页 `FindByIDs` 一次批量读取，并再次检查归属、可见状态及地区；摘要直接投影已有鉴权静态预览，不调用旧同步资源缓存下载。
- 客户端为列表只展示标题、日期、距离、时长及预览，点 item 才进入完整详情；不将摘要中的未补充互动数据作为已核验社区统计。

## 客户端表现

数据页新增「探索足迹」，默认省份模式：全国本地矢量省界按运动次数四档对数着色，未探索保持浅色；城市模式只点亮有轨迹的城市，圆点面积随去重景区数增加并限幅，无景区仍显示基础圆点。地图范围包括既有全国矢量素材的省界、岛屿及南海元素，不新增地图 SDK 或定位请求。

卡片为全部历史，统一跟随页面右上运动类型，显示探索省份/城市/景区数；点击省份放大进入城市模式，顶部可返回全国，点击城市或省份查看统计与本人轨迹。模式切换 220ms 叠化，日夜使用既有语义色；支持触摸及虚拟无障碍节点。云端与本机未上传完成记录合并，已上传副本不重复计数，本机不推测景区；失败保留同账号/筛选已有云端结果，无成功结果时明确仅本机未上传且景区待同步。账号/筛选变化清缓存，异步响应核对账号、View、代次及类型。
