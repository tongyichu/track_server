# 后台轨迹资料与景点候选

以下接口属于独立 `/admin/api`，使用后台 session。查看需轨迹管理页面 read/write 权限，确认、忽略和修改需轨迹管理 write 权限，不复用 App 用户 JWT。

## 列表与资料缺项

`GET /admin/api/tracks` 保留 `title`、`user_name`、`city_code`、`status`、`limit`、`cursor_start_time`、`cursor_id`，新增可选 `missing`：

| 值 | 页面选项 | 口径 |
| --- | --- | --- |
| 不传/空串 | 全部 | 原有非删除轨迹列表 |
| `place_candidate` | 待确认景点 | 当前规则下有效的候选，且源摘要匹配当前空间索引、轨迹仍可用于识别、有有效围合候选资料 |
| `scenic_spot` | 未关联景点 | 无内部景点关联，或关联目录不存在/已归档/已合并；draft/published 关联均视为存在 |
| `description` | 缺简介 | 轨迹 description 为空或空白 |
| `city` | 缺城市 | 轨迹 city_code 为空或空白 |
| `previews` | 缺预览图 | square_day/night、landscape_day/night 四张预览任意一张缺失；旧截图不替代四图完整性 |
| `any` | 任意缺项 | 上述条件的并集 |

未知 missing 值返回 400。所有过滤条件在总数统计和游标分页前应用，不在浏览器中过滤当前页。候选 ID 由既有候选队列批量读取并做当前源/规则校验，读取不会触发地图供应商查询；景点归属查自有目录关联。MySQL 用关联条件，Mongo 用 lookup/facet，内存使用 main 注入的景点仓储快照。Mongo 轨迹列表排序及游标使用实际 `_id` 字段。

未关联景点也包含正常的非景区轨迹，是运营筛选条件。缺预览图表示四图不完整，即使列表当前使用的日间横图存在，也可能命中。

切换缺项清空分页位置，可以与标题、用户名、城市、状态组合。快速切换忽略旧请求响应。候选处理成功后刷新来源列表，筛选总数同步更新。

## 单条候选详情

`GET /admin/api/tracks/:track_id/place-candidates`：返回 `{code:0,data:TrackPlaceCandidateItem}`，包含 `track_id,title,city_code,track_type,start_time,relations,places,updated_at`。

- 只返回该轨迹当前有效的围合候选，复用 source_version、规则版本、amap/provider、candidate 状态与可识别轨迹校验。
- 404：没有当前可用候选；409：轨迹源数据已变化；503：识别服务不可用。前端展示相应提示，不阻断基础轨迹资料查看。
- 只读取持久化的候选和地点资料，不调用供应商、不运行识别任务。

## 确认与忽略

继续使用既有接口：

| 方法 | 路径 | 请求 | 结果 |
| --- | --- | --- | --- |
| POST | `/admin/api/place-candidates/:track_id/confirm` | `{"place_id":"候选地点 ID"}` | 确认该轨迹的候选，更新主要景点并同步内部景点关联 |
| POST | `/admin/api/place-candidates/:track_id/dismiss` | 空对象 | 将本次候选标记为 unmatched |

两者归属轨迹操作权限；不能仅凭景点页面 write 权限操作。仍由服务端验证地点确实属于该轨迹当前候选，源失效返回 409，已处理或无有效候选返回 404。操作经后台写请求审计记录。

## 页面入口

景点管理页移除原来的全局“景点识别候选”面板。候选区位于共享轨迹详情弹窗顶部，轨迹列表和景点相关轨迹均复用。

只读用户可以查看候选，操作按钮仅对轨迹 write 用户显示。确认关联直接生效，忽略需确认；处理期间锁定候选区按钮，完成后刷新来源列表。候选操作独立于基础资料表单，不自动保存未提交的轨迹字段。过期/已变化候选操作失败后收起旧选项，允许刷新读取当前状态。

`GET /admin/api/place-candidates` 保留为兼容和内部队列读取接口，归属轨迹查看权限，页面不再加载全局队列。

## 批量导入轨迹

`POST /admin/api/tracks/import`（轨迹管理 write 权限，multipart/form-data）把轨迹文件批量导入并归属给指定内部人员：

| 字段 | 说明 |
| --- | --- |
| `files` | 轨迹文件（JSON / GeoJSON / KML / KMZ），可多选，单次最多 50 个 |
| `owner_user_id` | 归属人用户 ID，必须是 `is_internal` 内部人员 |
| `track_type` | 运动类型 code（hiking/running/climbing/riding/driving），默认 `hiking`，整批统一 |
| `coordinate_system` | 原始文件坐标系 `WGS84` / `GCJ02` / `BD09`，默认 `WGS84` |

响应 `{code:0,data:{results:[{file_name, track_id?, title?, error?}]}}`，逐文件独立成功或失败，互不影响。

处理流程：服务端解析轨迹点（复用地图索引解析器，受原始文件大小上限约束）→ 计算距离/起止时间/耗时/爬升/均速 → 原始文件由服务端直传 OSS 归属人用户目录 → 走 `TrackService.CreateTrack` 建档（`source_tag=manual_seed`、已完成状态），自动排队地图索引并结算成就。城市与 locate_addr 由内置行政区目录按轨迹中位点（转 GCJ02 后）离线解析，未命中留空；标题复用标题回填的生成规则。不生成预览四图与路线描述（按 auto/v0 口径留空），可后续用客户端 DevMode 补图/补齐。OSS 未配置时导入接口不可用。
