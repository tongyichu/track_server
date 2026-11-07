# 后台轨迹资料与景点候选

`POST /admin/api/tracks/:track_id/labels/generate` 返回 `data.shape_label`、`saved:false` 和 `diagnosis`。diagnosis 包含 category_checks（category/candidate/visual_basis，8类）、candidates（label/support/conflict，最多3项）、shape_label、recognition_status（recognized/ambiguous/no_candidate）、appropriateness（appropriate/inappropriate/undetermined）、decision_summary。只识别形象，不生成路线说明。GET 详情 `label_generation.shape_preview` 为 null 或最近同源成功结果 `{version,model,created_at,result}`，result 同 diagnosis；输入资料变化后不显示旧结果。只有点击保存才修改轨迹标签；失败不清空表单原值。

人工形象学习：PUT 保存 `shape_label` 实际变化（含清空）后记录最新反馈并由后台复核；保存成功但反馈记录失败或 AI 未配置时返回 `learning_warning`，不能按保存失败重复提交。GET 详情的 `label_generation.learning` 可为空，存在时含 status、attempts、label、review（supported/evidence/lesson）；状态为 pending/running/retry/reviewed/learned/disputed/stale/failed/unavailable。重新打开详情可查看进度。经验仅作后续识别参考，复核不改人工标签。协议未新增路由。

以下接口属于独立 `/admin/api`，使用后台 session。查看需轨迹管理页面 read/write 权限，确认、忽略和修改需轨迹管理 write 权限，不复用 App 用户 JWT。

## 轨迹类型与用户贡献统计

`GET /admin/api/tracks/statistics?period=day|week|month` 使用后台 session 和轨迹管理读取权限；默认 `month`，非法周期返回 400。

- 按北京时间从今天 00:00、本周一 00:00 或本月 1 日 00:00 统计至请求时刻，范围为 `[start,end)`。
- 以轨迹 `created_at`（服务端入库时间）归入周期，统计未下架、未软删除、已完成且有原始文件的云端轨迹，包含公开和私密；不受下方轨迹列表筛选影响，不按运动开始时间统计。
- `types` 为运动类型数量分布，包含全部类型，按数量降序、类型 code 升序；`key` 为稳定类型 code，`label` 为展示名称，未知类型不丢弃。
- `contributors` 为同口径的前 10 名轨迹所属用户，排除已不存在的账号；按数量降序、用户 ID 数值升序，少于十人时仅返回实际人数。`user_id` 始终为字符串，`nickname` 为当前昵称，空昵称由页面显示“未命名用户”。不逐用户或逐轨迹请求详情。
- 无数据时返回空数组。仅查询业务仓储，不新增表、埋点或供应商请求；MySQL/Mongo/内存实现保持一致。

响应示例（时间、ID、昵称和数量仅为示例）：

```json
{"code":0,"data":{"period":"month","start":"2026-11-01T00:00:00+08:00","end":"2026-11-04T01:00:00+08:00","types":[{"key":"hiking","label":"徒步","count":12}],"contributors":[{"user_id":"177920323643916288","nickname":"示例用户","count":12}]}}
```

轨迹管理页展示运动类型饼图和用户贡献前十横向柱状图（数量最多在顶部），两张图分别提供天/周/月切换，默认月。各卡片刷新与页面刷新保留选中周期；请求失败保留上次成功图表并回退到其周期，支持刷新重试。图表库异步加载，列表首屏不等待图表；本月城市分布从该页面移除。

## 列表与资料缺项

列表项增量返回 `city_name`，由服务端使用当前持久化地区目录与旧编号兼容映射解析，支持新 `region_id`、旧行政区码及旧目录键；不依赖城市选择器仅包含的新编号，不修改轨迹存储值或发起地图供应商请求。详情响应同级 `data.city_name` 与批量导入结果的 `track.city_name` 使用相同解析规则，编辑/删除后刷新导入结果仍保留正确城市显示。无法匹配时名称为空，前端区分“未填写城市”和“未匹配城市（原编号）”，不猜测或伪造地名。

`GET /admin/api/tracks` 保留 `title`、`user_name`、`city_code`、`status`、`limit`、`cursor_start_time`、`cursor_id`，新增可选 `province_code` 与 `missing`：

- `province_code`：省级 Code，服务端展开为该省全部城市 Code 后按 `city_code IN (...)` 筛选；与 `city_code` 同时传时 `city_code` 优先。
- `status`：`1` 公开、`2` 私密、`0` 已下架（软删除，页面文案为「已下架」，含客户端用户删除与后台删除）。默认（不传或传 1/2）排除已下架记录；传 `0` 改为只列已下架（`status=0`）的轨迹。

`POST /admin/api/tracks/:track_id/restore`（轨迹操作权限）恢复已下架轨迹：状态还原为公开（`status=1`）、清空 `deleted_at`，并重新排队地图索引任务（删除时 geo index 与索引任务已清理）；非已下架状态返回 400。删除时清理的收藏与路线组成员关系不可恢复。轨迹详情弹窗对已下架轨迹把底部「删除」按钮换成「恢复」，确认后调用该接口。

`POST /admin/api/tracks/:track_id/purge`（轨迹操作权限）彻底删除已下架轨迹，不可恢复：仅允许 `status=0` 的轨迹（否则 400），在事务中物理删除 `track_records` 及全部关联数据（轨迹点、收藏、导航记录、地图索引任务与 geo index、路线组成员、景点关联、景点识别记录、点赞、评论及评论反应/图片/统计、投稿及图片与审核流水），成功后尽力删除 OSS 原始轨迹文件与预览图（仅允许删除用户内容前缀下的对象，按精确 object key 授权；失败不阻断并在响应 `oss_warning` 中返回）。评论图片的 OSS 对象、用户已领取的成就奖励、历史同行会话与操作日志不在清理范围。轨迹详情弹窗对已下架轨迹把主按钮换成「彻底删除」，前端双重确认后调用。

`missing` 可选值：

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

入口常驻轨迹列表上方，以独立边框区域展示归属人、运动类型、坐标系、文件选择与开始导入按钮；结果以表格显示在该区域内，复用下方轨迹列表的 ID、城市、标题、类型、用户名（含头像）、轨迹图、状态和操作列，并增加文件名及导入结果。成功行的编辑按钮直接打开共享轨迹编辑弹窗，保存、下架、恢复或彻底删除后同步刷新本批结果及主列表；失败行保留文件名与错误，不提供编辑。窄屏表单自动改为单列，结果沿用主列表的响应式表格样式，溢出内容在区域内滚动，仅有轨迹操作权限的账号可见。

`POST /admin/api/tracks/import`（轨迹管理 write 权限，multipart/form-data）把轨迹文件批量导入并归属给指定内部人员：

| 字段 | 说明 |
| --- | --- |
| `files` | 轨迹文件（JSON / GeoJSON / KML / KMZ），可多选，单次最多 50 个 |
| `owner_user_id` | 归属人用户 ID，必须是 `is_internal` 内部人员 |
| `track_type` | 运动类型 code（hiking/running/climbing/riding/driving），默认 `hiking`，整批统一 |
| `coordinate_system` | 原始文件坐标系 `WGS84` / `GCJ02` / `BD09`，默认 `WGS84` |

响应 `{code:0,data:{results:[{file_name, track_id?, title?, track?, error?}]}}`，逐文件独立成功或失败，互不影响。成功项的 `track` 为 `{id, city_code, title, track_type, status}`，直接取自创建结果，避免浏览器逐条读取详情；不包含数值用户 ID。用户名和头像复用本次选择的归属人，初次导入不生成预览图，因此轨迹图列显示“无”。原有 `track_id`、`title` 字段保留兼容；失败项没有 `track`。

处理流程：服务端解析轨迹点（复用地图索引解析器，受原始文件大小上限约束）→ 计算距离/起止时间/耗时/爬升/均速 → 原始文件由服务端直传 OSS 归属人用户目录 → 走 `TrackService.CreateTrack` 建档（`source_tag=manual_seed`、已完成状态），自动排队地图索引并结算成就。解析器支持 KML/KMZ 中 `gx:Track` 的 `<when>` 时间戳（与 `<gx:coord>` 一一对应时按序回填），带真实时间的轨迹耗时/均速/热量按真实时间计算。源文件完全不带时间戳时，服务端按运动类型的估算速度（hiking 4、running 10、climbing 2.5、riding 15、driving 60 km/h）沿轨迹累计合成时间戳（起点为当前时间），并做拟真处理：逐段速度 ±25% 确定性抖动、按坡度爬坡减速/下坡加速、徒步与爬山每行进 12~20 分钟插入 1~4 分钟休息，使客户端配速/海拔图表接近真实记录；部分点有时间戳时保持原样。城市与 locate_addr 由内置行政区目录按轨迹中位点（转 GCJ02 后）离线解析，未命中留空；标题默认复用标题回填的生成规则；当文件名（去扩展名）含汉字时优先用文件名作为标题（最长 40 字符），因为运营文件命名通常即路线名。不生成预览四图与路线描述（按 auto/v0 口径留空），可后续用客户端 DevMode 补图/补齐。OSS 未配置时导入接口不可用。

上传文件格式：客户端详情/地图只解析 KMZ（zip 内 `gx:Track` KML）。已是 KMZ 的文件原样上传；KML / JSON / GeoJSON 会先按解析出的轨迹点转换为 `gx:Track` KMZ（全部点带时间戳时写入 `<when>`；无时间戳文件会先按估算速度合成）再上传，保证客户端可绘制。

## 轨迹详情、重新识别与人工候选放宽

`GET /admin/api/tracks/:track_id` 返回 `{code:0,data:{track, user_name, user_id_text, geo?}}`：

- `track`：轨迹持久化字段；`track.primary_scenic_spot={id,name}` 为当前主景点（自动归属或人工确认），无则省略。
- `user_name`：按持久化归属人查询的用户名（昵称），找不到或昵称为空时为“未知用户”；仅展示，不允许修改归属。
- `user_id_text`：字符串用户 ID，保留兼容；新版表单不展示或提交。
- `geo`：地图索引简化轨迹线 `{simplified_polyline, coordinate_system, start_lat/lng, end_lat/lng}`，坐标为 GCJ02，可直接在高德地图绘制；无索引时省略。
- `scenic_spot_id`：当前持久化主景点关联的内部 ID，无关联为空字符串；包括已归档景点的历史关联。读取失败时不返回此字段，前端禁用地图设置，不能把旧 `place-*` 标识当作内部关联 ID。

`POST /admin/api/tracks/:track_id/place-refresh`（轨迹操作权限）：同步对单条轨迹重新执行景点识别，复用后台识别任务同一互斥锁与供应商配额；任务忙返回 409，识别未启用 503，无可用地图索引 404。成功后前端重新读取候选。

`GET /admin/api/tracks/:track_id/place-candidates` 的人工候选集合放宽：除围住的景点（`enclosed_poi`）外，还包含采样途经的附近景点（`nearby` 且 `matched_samples≥1`，按 place_id 去重）；自动判定为 `unmatched` 的识别记录同样可读取候选并由人工确认。`confirm` 接受上述任一候选，主景点方法记录为对应 `kind`。`missing=place_candidate` 缺项筛选与全局候选队列仍只统计自动规则的 `candidate` 状态，不受放宽影响。

确认关联成功后，响应额外返回 `suggested_title`：服务端按客户端标题生成规则（`{景点名}周边{运动}[环线]`，环线判定用地图索引简化轨迹线）以景点名为地名生成的建议标题；与当前标题一致或无法生成时为空串。前端在标题输入框旁展示「代入建议标题」标签，点击仅代入输入框并随表单保存生效，不自动提交。

`GET /admin/api/tracks/map-config`（轨迹查看）：返回高德 Web(JS API) `key` 与 `service_host=/admin/api/tracks/amap-service`，供轨迹详情地图使用；`GET /admin/api/tracks/amap-service` 为同源高德服务代理（安全密钥不下发浏览器）。与景点页 `/scenic-spots/map-config` 同一配置，仅权限归属不同。

### 地图点选景点标签

`POST /admin/api/tracks/:track_id/scenic-spot`：同时要求轨迹与景点操作权限，沿用后台 session、同源检查和写审计。

```json
{
  "provider_id": "B000A00000",
  "name": "示例公园",
  "category": "风景名胜;公园广场",
  "address": "示例地址",
  "latitude": 39.9,
  "longitude": 116.4,
  "expected_spot_id": ""
}
```

字段来自高德底图 POI 点击后 `PlaceSearch.getDetails` 的结果，坐标固定 GCJ02；未触发热点的普通地图点击会展示附近景点候选（1500 米内最多 8 个），选中后同样读取 POI 详情。名称、资料状态和设置按钮展示在地图下方，不依赖地图气泡。POI ID、非空名称、有效坐标、`expected_spot_id` 必填，后者使用详情返回的 `scenic_spot_id`（无关联传空字符串）。仅支持高德 POI，不接收浏览器资源 URL。

优先按 `amap + provider_id` 复用景点目录，不覆盖已有名称、图片或人工内容；未收录时只创建所选 POI 基础资料，再通过既有景点关联仓储设为主景点（origin=manual），不触发 AI、等级采集或重新识别。仅点击 marker 或加载资料不写入；点“设为该轨迹景点标签”后立即生效，可替换已有归属，不提交标题、坐标系等未保存草稿。

返回 `{code:0,data:{scenic_spot_id,primary_scenic_spot,labels}}`；`labels` 只包含统一投影的景点标签。前端仅替换这一类标签并更新主景点和来源列表，保留其他标签、输入及地图视野；当前 POI 按钮显示“当前景点标签”。只读、下架或缺少任一操作权限不提供设置入口，服务端同样拒绝。

实际写关联时比较旧 ID，关联已变更、目录归档/合并或 CAS 冲突返回 409，刷新后重新选择；轨迹不存在/已下架返回 404，字段无效 400，无权限 403。建档与关联不是跨仓储事务，若建档后关联失败，目录保留供重试复用，不误报成功。关闭详情取消请求与定时工作、忽略迟到结果并销毁地图。

### user_id 精度约定

`GET /admin/api/tracks/:track_id` 额外返回 `user_id_text`（归属人 ID 的字符串形式）。轨迹归属人 ID 为 18 位 int64，经 JavaScript `Number` 往返会被精度截断（如 `…288` 变 `…300`），新版编辑弹窗只读展示 `user_name`，不展示或提交用户 ID；`PUT /admin/api/tracks/:track_id` 的 `user_id` 按 `json.Number` 解析，接受字符串或数字字面量。轨迹地图视野在弹窗可见后校准并限制最大缩放。

## 编辑字段与坐标预览

### 详情资源展示与下载

共享详情弹窗的预览资源只展示图片，可点击放大；图片字段为空显示“无”，请求失败显示“加载失败”并允许重试。原始轨迹文件展示下载图标，不展示 URL，字段为空显示“缺失”。URL 输入框从表单移除，保存资料不提交原始文件、截图或 `preview_images`，避免把展示用代理地址写回数据或覆盖并发补图。原 PUT 接口仍兼容资源字段。

`GET /admin/api/tracks/:track_id/assets/:asset` 使用后台 session 和轨迹查看权限，读取指定轨迹当前持久化资源；私密及已下架记录也可由有权限的后台账号查看。`asset` 仅接受：

| asset | 资源与响应 |
| --- | --- |
| `raw` | 原始轨迹文件，以 `Content-Disposition: attachment` 直接下载，文件名为轨迹 ID 加文件扩展名 |
| `square_day` / `square_night` | 方形日间 / 夜间预览图 |
| `landscape_day` / `landscape_night` | 横版日间 / 夜间预览图 |
| `legacy` / `no_map` | 旧版截图 / 无地图轨迹线；无地图 SVG 按运动类型主题色渲染 |

浏览器不传资源 URL。响应为文件内容，使用 `no-store`；不重定向到 OSS 或业务 JWT 路由。图片与原文件按请求异步读取，详情基础字段不等待缓存下载。服务端复用既有原文件、私有预览图及截图缓存和 OSS 内网下载器，缓存 key 包含来源摘要，预览四图沿用业务侧版本 key；读取不修改轨迹资源地址。资源字段为空/本地文件不存在返回 404，未知 asset 返回 400，历史本地路径无效返回 422，缓存未配置返回 503，远端读取失败返回 502。

无地图轨迹的客户端生成格式为透明背景 SVG（`TrackLineSvgGenerator`，原文件使用固定浅绿色）。后台 `no_map` 读取 SVG 时只复制安全的矢量几何与布局属性，用 `TrackDisplayLabels` 的 sport.theme_color（权威来源 `DefaultTrackTypeConfigs`）渲染线条，不修改原文件或源缓存。可选 `track_type` 参数仅用于预览：切换表单轨迹类型后即时换色，保存才修改类型；不传时按已存类型着色。未知运动类型无主题色时保留原图，历史位图仍按原图展示。SVG 限 8 MiB，移除脚本、样式及外部资源，无法解析返回 422；空资源仍显示“无”。

普通字段按弹窗宽度最多四列、逐级减少至单列；轨迹描述保留整行。标签/景点识别与地图在宽屏并排，窄屏顺排，样式限定 `.admin-track-editor`，不改变其他后台表单的密度。

共享轨迹编辑表单只读显示归属用户名 `user_name`，保存不提交 `user_id` 或 `user_name`。描述来源 `description_source`、描述规则版本 `description_rule_version`、轨迹进行状态 `is_running` 不展示且不随表单提交，原值保留。`PUT /admin/api/tracks/:track_id` 仅兼容旧页面回传原归属人；不同 ID 或非法 ID 返回 400，不允许通过后台转移归属。

坐标系使用下拉选项：未设置、WGS84、GCJ02、BD09；来源标签使用下拉选项：空值（用户录制/默认）、`manual_seed`（人工导入）。未知历史值保留在当前记录的选项中，避免保存其他字段时被静默替换。

`GET /admin/api/tracks/:track_id/map-preview` 需要轨迹查看权限，异步读取原始轨迹文件，返回 `{coordinate_system:"GCJ02", variants:{WGS84:[TrackPoint], GCJ02:[TrackPoint], BD09:[TrackPoint]}}`。每组均为对同一批原始坐标按对应源坐标系解释后转换到 GCJ02 的简化线；复用既有原始轨迹缓存、文件解析器、抽样上限及坐标转换（WGS84↔GCJ02 覆盖大陆、香港、澳门，使用独立于地图服务路由的转换范围），不写轨迹或地图索引，也不触发供应商地点查询。服务未配置返回 503、轨迹不存在返回 404、原始文件无法解析或读取返回 422。

预览数据独立加载，不阻塞表单；准备完成后，切换坐标系立即重绘线与起终点并保留地图视野。准备期间记住最新选择，失败提供重试；不把已转换的索引线再次当原始坐标转换。空坐标系沿用 GCJ02 历史默认。关闭弹窗取消请求并释放地图。选择仅改变预览，保存才写入；保存坐标系变更时清理旧空间索引与关联，并为符合公开已完成条件的轨迹重新排队索引。
# 标签管理补充

轨迹详情增加 `track.labels` 和 `label_generation`（status、reason、attempts、first_seen、next_attempt）。共享编辑器展示统一标签与任务状态。

`POST /admin/api/tracks/:track_id/labels/generate` 需要 tracks 写权限，使用已保存资料及一张方图生成形象建议，复用自动任务全局租约和日预算。允许历史轨迹手动生成；需公开、已结束、已有索引与方图。返回 `{code:0,data:{shape_label:"玫瑰花",saved:false}}`，不直接保存；没有明确且适宜的形象返回空串。忙碌、额度不足或识别失败返回 409，未配置返回 503。

既有 PUT 新增可选 `shape_label`：32 字符以内，禁止控制字符，空串清空、缺省保留。AI 建议填入输入框，点击保存才生效；后台不覆盖生成期间用户修改的输入。精品仍由审核状态决定，运动类型编辑 track_type，景点使用既有关联面板。人工形象应与轨迹相符且适宜公开展示。
