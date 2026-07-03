# 轨迹投稿功能方案

> 状态：已实现第一版投稿与审核闭环、图片缓存、分页窗口内优先展示和 RouteGroup 投稿代表候选。
>
> 本文描述轨迹投稿的产品规则、客户端交互、服务端模型、接口草案、审核流程、图片缓存、推荐排序与 RouteGroup 代表轨迹选择。正式实现时以代码、`internal/handler/router.go` 和 `docs/api/` 为准。

## 1. 背景

客户端目前生成轨迹标题时主要使用日期，标题信息量较少，不利于用户识别路线，也不利于服务端筛选优质轨迹。

新增“轨迹投稿”能力：用户可以为一条已完成轨迹补充结构化路线资料并提交审核。审核通过后：

- 在推荐轨迹列表中优先展示，并显示“优质投稿”标识；
- 可以成为所在 RouteGroup 的展示代表轨迹；
- 为后续路线筛选、路线介绍和运营内容提供结构化数据。

投稿不改变轨迹原始 GPS 数据，不改变 RouteGroup 的自动聚类关系，也不影响用户正常查看自己的未投稿轨迹。

## 2. 设计目标

### 2.1 目标

- 让用户用清晰标题、路线简介、难度、风险、月份、路面、交通和可选的沿途图片描述一条路线。
- 建立可追踪、可驳回、可重投、可撤回的审核流程。
- 让审核通过的轨迹获得稳定、可分页的推荐曝光。
- 让审核通过的投稿成为 RouteGroup 代表轨迹候选，同时保持地图聚类几何语义稳定。
- 投稿图片复用现有轨迹截图的 OSS 上传、本地缓存和静态地址改写机制。
- MySQL、Mongo 和 in-memory 降级实现保持一致，不引入启动时必须依赖的新外部服务。

### 2.2 非目标

- 第一版不自动审核，不使用 AI 判断路线质量。
- 第一版不根据投稿内容自动修改已发布的 RouteGroup 运营介绍。
- 第一版不改变 `track_records.title` 的原始含义；公开展示时可以使用已通过投稿的标题覆盖响应标题。
- 第一版不根据投稿难度或风险自动计算保险、救援或安全承诺。

## 3. 术语

| 术语 | 含义 |
| --- | --- |
| Track | 用户原始轨迹记录，外部 ID 形如 `NO.00000001` |
| Submission | 针对一条 Track 提交的结构化路线资料 |
| Featured Track | 审核通过、当前仍有效并获得推荐资格的投稿轨迹 |
| Geometry Anchor | RouteGroup 的几何锚点，即自动相似度算法选择的 medoid |
| Representative Track | RouteGroup 对用户展示的代表轨迹，可来自管理员指定或已审核投稿 |

## 4. 投稿资格

用户发起投稿时，轨迹必须同时满足：

- 属于当前 JWT 用户；
- `is_running=false`；
- `status=normal`，不能是私密或已删除轨迹；
- `raw_track_url` 非空；
- `track_screenshot_url` 非空；
- `track_type` 是服务端支持的标准英文 code；
- 标题、简介、难度、风险、适宜月份和交通信息满足校验规则；若提供沿途图片，图片也必须满足格式、数量和 OSS 地址校验规则；
- 当前账号不存在阻止内容发布的账号限制。

地图索引尚未完成时允许提交，但管理后台应显示“地图索引待就绪”。地图索引失败不应导致投稿数据丢失。审核人员可以等待索引完成后再审核；缺少地图索引的投稿不能被自动选为 RouteGroup 代表轨迹。

## 5. 投稿状态机

```text
未投稿
  → pending（首次投稿）
      → approved（审核通过）
      → rejected（审核驳回）
      → withdrawn（用户撤回）

rejected / withdrawn
  → pending（修改后重投，revision + 1）

approved
  → withdrawn（用户主动撤回）
  → invalidated（轨迹删除、转私密或关键内容发生变化）
```

状态说明：

| 状态 | 说明 | 推荐资格 | 代表轨迹候选资格 |
| --- | --- | --- | --- |
| `pending` | 等待审核 | 否 | 否 |
| `approved` | 审核通过且当前有效 | 是 | 是 |
| `rejected` | 审核驳回，可修改后重投 | 否 | 否 |
| `withdrawn` | 用户主动撤回 | 否 | 否 |
| `invalidated` | 轨迹关键内容变化导致审核结果失效 | 否 | 否 |

同一条 Track 只维护一个当前 Submission，通过 `revision` 区分重投版本；所有状态变化写入不可变事件流水。

## 6. 投稿字段

### 6.1 标题与简介

| 字段 | 规则 |
| --- | --- |
| `title` | 必填，去除首尾空白后 4～40 个 Unicode 字符 |
| `description` | 必填，去除首尾空白后 20～500 个 Unicode 字符 |

客户端可以根据“地点 + 运动类型 + 路线特征”生成初始标题，例如“西湖群山十里徒步环线”，但必须允许用户编辑，不再只使用日期。

### 6.2 路线难度

| code | 展示名 | 产品说明 |
| --- | --- | --- |
| `easy` | 轻松 | 路线平缓不陡，适合新手 |
| `standard` | 标准 | 有小幅起伏，需要轻度体力 |
| `hard` | 困难 | 爬升较多、路况复杂，需要一定经验 |
| `challenge` | 挑战 | 高海拔或长距离，部分路段险峻 |
| `extreme` | 极限 | 极端环境、超高强度，需要专业装备和经验 |

第一版由投稿用户选择、审核人员复核，不根据距离或爬升自动覆盖用户选择。

### 6.3 风险等级

| code | 展示名 | 产品说明 |
| --- | --- | --- |
| `none` | 无风险 | 路面平坦，未识别明显危险因素 |
| `low` | 低风险 | 存在小坡度或碎石，需要注意脚下 |
| `medium` | 中风险 | 存在陡坡险路，受天气影响较大 |
| `high` | 高风险 | 可能存在悬崖峭壁或恶劣天气等高风险因素 |

“无风险”只表示投稿和审核时未识别出明显危险因素，不构成绝对安全承诺。客户端详情页应提示用户结合实时天气、道路封闭和个人能力自行判断。

### 6.4 适宜月份

字段使用整数数组：

```json
"suitable_months": [3, 4, 5, 9, 10, 11]
```

规则：

- 必填，至少选择 1 个月；
- 每项取值为 `1`～`12`；
- 服务端去重并按升序保存；
- 全年适宜使用 `[1,2,3,4,5,6,7,8,9,10,11,12]`，不额外增加 `all_year` 特例。

### 6.5 路面与地形类型

用户提出的“乱世”在本文中按“乱石”处理。由于涉水、攀岩、灌木丛不完全属于路面，协议字段统一命名为 `surface_types`，客户端文案使用“路面与地形”。

| code | 展示名 |
| --- | --- |
| `road` | 公路 |
| `boardwalk` | 栈道 |
| `stone_slab` | 石板路 |
| `stairs` | 台阶 |
| `dirt` | 土路 |
| `wading` | 涉水 |
| `loose_rocks` | 乱石 |
| `climbing` | 攀岩 |
| `shrub` | 灌木丛 |
| `snow` | 雪地 |
| `desert` | 沙漠 |
| `reef` | 礁石 |

规则：必填，多选，去重后保存；第一版最多选择 12 项。

### 6.6 交通方式

交通采用“枚举多选 + 补充说明”，便于未来筛选，同时保留复杂到达方式的表达能力。

| code | 展示名 |
| --- | --- |
| `bus` | 公交 |
| `metro` | 地铁 |
| `train` | 火车 |
| `self_drive` | 自驾 |
| `taxi` | 出租车/网约车 |
| `chartered` | 包车 |
| `ferry` | 轮渡 |
| `cable_car` | 缆车 |
| `cycling` | 骑行 |
| `walking` | 步行 |
| `other` | 其他 |

字段：

```json
{
  "transport_modes": ["self_drive", "taxi"],
  "transport_description": "导航至云栖竹径停车场；节假日车位紧张，建议打车前往。"
}
```

规则：

- `transport_modes` 必填，至少一项；
- `transport_description` 必填，建议 10～500 个 Unicode 字符；
- 选择 `other` 时，说明中必须明确具体交通方式。

### 6.7 投稿图片

- 可选，0～9 张；不上传沿途图片也可以正常投稿；
- 单张建议不超过 10 MB，总计不超过 50 MB；
- 支持 JPEG、PNG、WebP；
- 客户端上传前建议压缩至长边不超过 2560 px，并移除 EXIF 中的精确 GPS 信息；
- 每张图支持可选 `caption`，最多 100 个 Unicode 字符；
- `sort_order` 从 1 开始，同一投稿中不能重复；第一张图作为列表封面图。

## 7. 数据模型草案

### 7.1 track_submissions

```sql
CREATE TABLE `track_submissions` (
  `submission_id` VARCHAR(64) NOT NULL COMMENT '投稿ID',
  `track_id` VARCHAR(64) NOT NULL COMMENT '轨迹ID',
  `user_id` BIGINT UNSIGNED NOT NULL COMMENT '投稿用户ID',
  `track_type` VARCHAR(32) NOT NULL DEFAULT '' COMMENT '投稿时的标准运动类型英文code',
  `title` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '投稿标题',
  `description` TEXT NOT NULL COMMENT '路线简介',
  `difficulty` VARCHAR(16) NOT NULL DEFAULT '' COMMENT 'easy/standard/hard/challenge/extreme',
  `risk_level` VARCHAR(16) NOT NULL DEFAULT '' COMMENT 'none/low/medium/high',
  `suitable_months_json` VARCHAR(128) NOT NULL COMMENT '适宜月份JSON数组',
  `surface_types_json` TEXT NOT NULL COMMENT '路面与地形类型JSON数组',
  `transport_modes_json` TEXT NOT NULL COMMENT '交通方式JSON数组',
  `transport_description` VARCHAR(500) NOT NULL DEFAULT '' COMMENT '交通补充说明',
  `status` VARCHAR(16) NOT NULL DEFAULT 'pending' COMMENT 'pending/approved/rejected/withdrawn/invalidated',
  `revision` BIGINT UNSIGNED NOT NULL DEFAULT 1 COMMENT '重投版本号',
  `submitted_at` DATETIME(6) NOT NULL,
  `approved_at` DATETIME(6) NULL,
  `reviewed_at` DATETIME(6) NULL,
  `reviewed_by` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '管理员账号',
  `review_reason` VARCHAR(500) NOT NULL DEFAULT '' COMMENT '驳回、撤销或失效原因',
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  PRIMARY KEY (`submission_id`),
  UNIQUE KEY `uk_track_submission_track` (`track_id`),
  KEY `idx_track_submission_status_submitted` (`status`, `submitted_at`),
  KEY `idx_track_submission_status_approved` (`status`, `approved_at`),
  KEY `idx_track_submission_user_updated` (`user_id`, `updated_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户轨迹投稿';
```

正式实现时需根据现有 MySQL 初始化/迁移风格调整建表方式，并为 Mongo 和 in-memory 实现等价约束。

### 7.2 track_submission_images

投稿图片处理方式与轨迹截图一致：客户端直传 OSS，服务端保存 OSS 地址；读取时缓存到本地静态目录并改写响应 URL。

```sql
CREATE TABLE `track_submission_images` (
  `image_id` VARCHAR(64) NOT NULL COMMENT '投稿图片ID',
  `submission_id` VARCHAR(64) NOT NULL COMMENT '投稿ID',
  `oss_url` VARCHAR(1024) NOT NULL COMMENT '客户端直传后的OSS地址',
  `caption` VARCHAR(200) NOT NULL DEFAULT '' COMMENT '图片说明',
  `sort_order` INT NOT NULL DEFAULT 0 COMMENT '展示顺序',
  `created_at` DATETIME(6) NOT NULL,
  `updated_at` DATETIME(6) NOT NULL,
  PRIMARY KEY (`image_id`),
  UNIQUE KEY `uk_track_submission_image_order` (`submission_id`, `sort_order`),
  KEY `idx_track_submission_image_submission` (`submission_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='轨迹投稿图片';
```

不在数据库中持久化本地缓存 URL。本地 URL 是读取时由 `AssetCacheService` 根据 `oss_url` 计算和改写的派生值。

### 7.3 track_submission_events

```sql
CREATE TABLE `track_submission_events` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `submission_id` VARCHAR(64) NOT NULL,
  `revision` BIGINT UNSIGNED NOT NULL,
  `event_type` VARCHAR(32) NOT NULL COMMENT 'submitted/resubmitted/approved/rejected/withdrawn/invalidated',
  `from_status` VARCHAR(16) NOT NULL DEFAULT '',
  `to_status` VARCHAR(16) NOT NULL DEFAULT '',
  `operator_type` VARCHAR(16) NOT NULL COMMENT 'user/admin/system',
  `operator` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '用户ID或管理员账号',
  `reason` VARCHAR(500) NOT NULL DEFAULT '',
  `snapshot_json` MEDIUMTEXT NOT NULL COMMENT '本次版本投稿内容快照',
  `created_at` DATETIME(6) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_track_submission_event_submission` (`submission_id`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='轨迹投稿审核流水';
```

事件流水只追加、不修改，用于审核追溯；当前状态仍以 `track_submissions.status` 为权威。

## 8. 客户端接口草案

所有接口默认挂在 `/api/v1` 的业务 JWT `auth` 分组中。

### 8.1 查询投稿选项

```http
GET /api/v1/track/submission/options
```

响应返回难度、风险、月份、路面、交通枚举及图片限制，避免不同客户端各自硬编码产品说明。服务端以 code 为协议值，客户端负责本地化展示。

```json
{
  "code": 0,
  "data": {
    "difficulty_options": [
      {
        "code": "easy",
        "name": "轻松",
        "description": "路线平缓不陡，适合新手"
      }
    ],
    "risk_level_options": [],
    "surface_type_options": [],
    "transport_mode_options": [],
    "image_limits": {
      "min_count": 0,
      "max_count": 9,
      "max_file_size": 10485760,
      "max_total_size": 52428800
    }
  }
}
```

### 8.2 创建或重新提交投稿

```http
POST /api/v1/track/:track_id/submission
```

```json
{
  "title": "西湖群山十里徒步环线",
  "description": "从云栖竹径出发，经五云山、九溪返回，林荫路段较多。",
  "difficulty": "standard",
  "risk_level": "low",
  "suitable_months": [3, 4, 5, 9, 10, 11],
  "surface_types": ["stone_slab", "stairs", "dirt", "loose_rocks"],
  "transport_modes": ["self_drive", "taxi"],
  "transport_description": "导航至云栖竹径停车场；节假日建议打车。",
  "images": [
    {
      "oss_url": "https://example-bucket.oss-cn-hangzhou.aliyuncs.com/user/1001/submission/1001/a.jpg",
      "caption": "五云山观景台",
      "sort_order": 1
    }
  ]
}
```

行为：

- 未投稿时创建 `revision=1,status=pending`；
- `rejected` 或 `withdrawn` 状态再次提交时 `revision+1` 并回到 `pending`；
- `pending` 状态重复 POST 返回 `409`，客户端应使用更新接口；
- `approved` 状态不允许直接覆盖，用户需先撤回；
- `images` 可以省略或传空数组；提供图片时才创建图片记录；
- 主记录、可选图片记录和事件流水应在同一存储事务/原子操作中完成。

### 8.3 修改待审核投稿

```http
PUT /api/v1/track/:track_id/submission
```

仅允许修改当前用户自己的 `pending` 投稿。修改后 `revision+1`、刷新 `submitted_at` 并追加 `resubmitted` 事件，使管理员不能审核旧版本。

### 8.4 查询投稿

```http
GET /api/v1/track/:track_id/submission
```

- 投稿人可以查看所有状态、驳回原因和当前版本；
- 其他用户只能查看有效的 `approved` 投稿；
- 非投稿人访问未通过投稿返回 `404`，避免泄露审核状态。

### 8.5 撤回投稿

```http
POST /api/v1/track/:track_id/submission/withdraw
```

- 撤回是投稿状态变化，不删除投稿主记录、图片或审核流水，因此不使用 `DELETE`；
- `pending` 撤回后变为 `withdrawn`；
- `approved` 撤回后立即取消推荐和代表候选资格；
- 若该轨迹正是 RouteGroup 的投稿代表轨迹，服务端应触发代表轨迹回退；下一次离线全量聚合再次校正。

### 8.6 我的轨迹列表

`GET /api/v1/track/my/list` 的每个 item 增加仅本人可见摘要：

```json
{
  "submission": {
    "status": "rejected",
    "revision": 2,
    "review_reason": "标题过于笼统，请补充路线起终点或主要途经点"
  }
}
```

服务端应批量查询投稿摘要，禁止对每条轨迹执行一次投稿查询。

### 8.7 公开列表与详情

审核通过的公开 TrackSummary 增加：

```json
{
  "is_featured": true,
  "title": "西湖群山十里徒步环线",
  "featured_description": "从云栖竹径出发，经五云山、九溪返回……",
  "featured_cover_url": "/api/v1/static/submission_images/TS_xxx_TSI_xxx.jpg"
}
```

兼容规则：

- `title` 直接使用审核通过的投稿标题，旧客户端无需新增字段即可获得更好的标题；
- 原始 `track_records.title` 不被覆盖；
- 投稿失效后恢复返回原始轨迹标题；
- 有投稿图片时，推荐列表只返回第一张投稿图片作为封面；未上传投稿图片时回退使用轨迹原有 `track_screenshot_url`；
- 投稿详情返回全部结构化字段，未上传投稿图片时 `images` 返回空数组。

## 9. 图片上传、缓存与地址改写

### 9.1 上传流程

```text
客户端 GET /api/v1/oss/sts-token
  → 使用临时凭证将图片直接上传到 <OSS_UPLOAD_PREFIX>/<bucket_id>/submission/<user_id>/
  → POST/PUT 投稿时提交图片 OSS URL
  → 服务端校验 URL 并持久化到 track_submission_images.oss_url
```

服务端必须校验：

- URL host、Bucket 和 OSS 配置匹配；
- Object Key 位于当前用户允许的 OSS 目录；
- URL 长度、协议和格式合法；
- 不接受 `file://`、内网 URL 或任意第三方图片 URL。

### 9.2 本地缓存目录

新增投稿图片缓存分类：

```text
<LogDir>/static/submission_images/
```

缓存 key 使用不可预测且不冲突的组合：

```text
<submission_id>_<image_id>
```

不得使用客户端原始文件名作为缓存 key，避免重名、路径穿越和特殊字符问题。

### 9.3 获取流程

```text
查询投稿列表/详情
  → 读取 track_submission_images.oss_url
  → submissionImageCache.EnsureCached(
       投稿用户 user_id,
       submission_id + "_" + image_id,
       oss_url
     )
  → 缓存到 <LogDir>/static/submission_images/
  → 响应 URL 改写为 /api/v1/static/submission_images/...
```

返回示例：

```json
{
  "image_id": "TSI.01JEXAMPLE",
  "url": "/api/v1/static/submission_images/TS.01J_TSI.01J.jpg",
  "caption": "五云山观景台",
  "sort_order": 1
}
```

响应不返回原始 `oss_url`。

### 9.4 业务端与管理后台地址

业务客户端使用现有 JWT 静态资源路由：

```text
/api/v1/static/submission_images/<cached-file>
```

管理后台把该地址继续改写为 admin session 静态代理：

```text
/admin/api/static/submission_images/<cached-file>
```

不为投稿图片新增一套文件下载协议。

### 9.5 缓存时机与性能

- 投稿包含图片时异步预热全部图片，预热失败不影响投稿主流程；未提供图片时不执行投稿图片缓存逻辑；
- 列表页只处理第一张封面图；
- 投稿详情页处理全部图片，但使用共享的请求级超时并限制并发，建议总超时 5 秒、并发数 3；
- 缓存未命中时同步兜底下载，失败的单张图片返回空 `url`，不影响其他字段和图片；
- 日志使用汇总计数，避免逐张图片刷屏；
- 投稿撤回或驳回后保留 OSS 地址和本地缓存，方便修改后重投；
- 投稿或轨迹永久删除时尽力清理本地缓存，OSS 对象仍由用户目录及其生命周期策略管理。

## 10. 管理后台方案

新增“轨迹投稿审核”页面：

```text
/admin/track_submissions.html
```

### 10.1 管理接口

```http
GET  /admin/api/track-submissions
GET  /admin/api/track-submissions/:submission_id
POST /admin/api/track-submissions/:submission_id/review
```

列表支持：

- `status`；
- `difficulty`；
- `risk_level`；
- `track_type`；
- `user_id`；
- `submitted_at` 游标分页。

审核请求：

```json
{
  "decision": "approved",
  "reason": "",
  "expected_revision": 2
}
```

规则：

- `decision` 只允许 `approved` 或 `rejected`；
- 驳回时 `reason` 必填；
- `expected_revision` 必须等于当前版本，否则返回 `409`，防止审核用户已经修改的旧版本；
- `reviewed_by` 从 admin session 获取，客户端不能传入；
- 审核操作与事件流水写入必须保持原子性。

### 10.2 审核详情展示

- 投稿标题、简介；
- 难度、风险等级及产品解释；
- 适宜月份；
- 路面与地形标签；
- 交通方式及补充说明；
- 投稿图片画廊和大图预览（有图片时展示）；
- 原始轨迹截图、路线地图、距离、时长、爬升、运动类型；
- 投稿用户和当前账号限制；
- 地图索引状态；
- 当前所属 RouteGroup、当前代表轨迹和相似度；
- 历次投稿版本、审核结果和原因。

审核人员默认只通过或驳回，不静默修改用户提交的难度、风险和文案。若内容不准确，应驳回并说明原因。

后台可以显示软提醒，但第一版不作为服务端硬性冲突规则：

- `difficulty=extreme` 且 `risk_level=none`；
- 包含 `climbing`、`wading`、`reef`，但风险为 `none`；
- 高海拔或雪地描述与适宜月份明显冲突。

## 11. 推荐列表优先展示

现有推荐列表按 `start_time DESC, id DESC` 使用游标分页。不能仅增加 `is_featured DESC` 排序，否则会破坏旧游标语义，并让历史优质投稿长期压住新轨迹。

### 11.1 当前实现：分页窗口内优先

- Repository 仍按 `start_time DESC, track_id DESC` 划分稳定分页窗口；
- 服务端批量查询当前窗口内的有效 `approved` 投稿，并在该窗口内稳定前置；
- 下一页游标继续使用 Repository 原始窗口最后一条轨迹，避免重排后的重复和遗漏；
- 搜索、收藏和“我的轨迹”保持原排序，只展示投稿状态或徽标；
- RouteGroup 的轨迹列表按“代表轨迹 → 其他已通过投稿 → 普通成员”排序。

全局双通道比例混排及独立投稿游标可作为后续推荐系统升级项，第一版不引入新游标协议。

### 11.2 当前游标

推荐列表继续使用现有 `(start_time,id)` base64url 不透明游标。客户端不能解析或自行构造。

## 12. RouteGroup 代表轨迹

### 12.1 几何锚点与展示代表解耦

离线任务先按 medoid 计算 RouteGroup 的 `center`、`radius_m`、`area_id`、bbox 和成员相似度，再独立选择展示用 `representative_track_id`。投稿代表轨迹只改变展示代表以及成员的 `role/source`，不改变已经计算完成的地图聚类几何结果。

### 12.2 代表轨迹选择顺序

每次全量路线组重建时：

1. 若历史管理员手动代表轨迹仍属于新组，保留该轨迹；
2. 否则，从当前组有效的 `approved` 投稿中选择与 medoid 相似度最高的轨迹；
3. 若没有合格投稿，使用 medoid；
4. 相似度相同则按 `track_id ASC` 稳定选择。

投稿审核通过只让轨迹获得候选资格，不同步改变聚类成员关系。若轨迹已经属于 RouteGroup，可以在审核后异步刷新该组代表轨迹；每日全量任务负责最终校正。

### 12.3 与路线介绍的关系

`track_route_introductions.anchor_track_id` 是运营介绍的稳定锚点，不能随着自动投稿代表轨迹切换而变化。

- 自动选中投稿代表轨迹：只更新 `representative_track_id`，并把对应成员标记为 `role=representative,source=submission`；
- 管理员通过现有“指定代表轨迹”操作时，沿用当前行为并将已有路线介绍锚点切换到该轨迹；投稿离线自动选择不调用该操作；
- 投稿内容可以在后台“一键复制为路线介绍草稿”，但不能自动覆盖已发布介绍；
- 已发布 RouteGroup 介绍仍是聚合路线内容的权威来源。

## 13. 轨迹变化与投稿失效

审核通过后，下列变化应将投稿标记为 `invalidated` 并立即取消推荐资格：

- 轨迹被删除；
- 轨迹转为私密；
- `raw_track_url` 变化；
- `track_type` 变化；
- 轨迹地图索引对应的路线几何发生实质变化；
- 管理员认定投稿内容不再有效并执行撤销。

只修改非关键展示字段时可以不自动失效。正式实现应把“更新 Track + 使投稿失效”做成同一仓储事务或等价原子操作，避免列表短暂展示未经审核的新内容。

若失效轨迹是当前 RouteGroup 投稿代表轨迹，应立即回退为手动代表或 medoid，并由下一次全量任务校正。

## 14. 客户端交互

### 14.1 入口

- 轨迹完成页增加“投稿优质轨迹”；
- 我的轨迹详情增加“投稿/查看投稿”；
- 已通过投稿显示“优质投稿”徽标；
- 待审核、驳回、撤回状态只对投稿人展示。

### 14.2 投稿表单

建议分为四个区块：

1. 路线介绍：标题、简介；
2. 路线属性：难度、风险、适宜月份、路面与地形；
3. 到达方式：交通多选、交通说明；
4. 沿途图片（可选）：上传、排序、说明、封面预览。

提交前显示完整预览和审核提示。客户端只做体验层校验，服务端必须重复全部校验。

### 14.3 状态反馈

- `pending`：显示“审核中”，允许修改或撤回；
- `approved`：显示“已通过”，可以查看公开效果或撤回；
- `rejected`：显示驳回原因，提供“修改后重投”；
- `withdrawn`：显示“已撤回”，提供“重新投稿”；
- `invalidated`：说明失效原因，满足条件后允许重新投稿。

## 15. 权限与安全

- 投稿、修改、撤回必须使用业务 JWT，并校验 Track 所有权；
- 投稿路由加入 `AccountRestrictionMiddleware` 的受限动作表；
- admin 审核使用独立 admin session，不复用业务 JWT；
- 公开接口不泄露 `reviewed_by`、内部审核备注和未通过内容；
- 非投稿人查询未通过投稿统一返回 `404`；
- OSS URL 必须校验归属和配置域名；
- 标题、简介、图片说明和交通说明需要执行长度、空白字符和敏感内容校验；
- 图片响应只返回本地静态缓存 URL，不返回 OSS 签名 URL或原始地址；
- 风险等级是信息提示，不替代实时天气、封路、救援和安全判断。

## 16. Repository 与服务划分

建议新增：

```go
type TrackSubmissionRepository interface {
    Create(...)
    UpdatePending(...)
    FindByTrackID(...)
    FindBySubmissionID(...)
    ListAdmin(...)
    Review(...)
    Withdraw(...)
    InvalidateByTrackID(...)
    ListApprovedByTrackIDs(...)
    ListApprovedFeed(...)
}
```

实际方法签名以实现时的模型和事务边界为准。MySQL、Mongo、in-memory 三种实现必须同时完成。

服务职责：

- `TrackSubmissionService`：资格校验、字段归一、状态机、审核、图片缓存、事件流水；
- `TrackService`：在推荐列表、我的轨迹和详情中批量补充投稿信息；
- `TrackRouteGroupService`：使用已通过投稿选择展示代表轨迹；
- `AssetCacheService`：新增 `submission_images` 缓存实例，不改变现有截图缓存语义。

## 17. 错误语义草案

| 场景 | HTTP 状态 |
| --- | --- |
| 参数或枚举非法 | `400` |
| 未登录 | `401` |
| 不是轨迹所有者 | `403` |
| 账号被限制 | `403` |
| 轨迹或投稿不存在 | `404` |
| pending 重复提交、审核版本冲突 | `409` |
| 未处理异常 | `500` |

客户端需要展示的业务错误应返回稳定 `message`，例如：

- `track is not eligible for submission`；
- `submission is already pending`；
- `submission revision conflict`；
- `at least one suitable month is required`；
- `submission image count must not exceed 9`；
- `submission image does not belong to current user`。

## 18. 监控与运营指标

建议记录：

- 投稿人数、投稿数、重投数；
- 审核通过率、驳回原因分布；
- 平均审核耗时和 pending 积压量；
- 投稿图片缓存命中率、下载失败率；
- 优质投稿在推荐列表中的曝光、点击、收藏和导航使用；
- 作为 RouteGroup 代表轨迹的投稿数量；
- 投稿撤回、失效及代表回退次数。

第一版可以使用现有日志和埋点体系，不要求新增外部监控依赖。

## 19. 测试与验收

### 19.1 服务端测试

- 各投稿状态转换及非法转换；
- Track 所有权、账号限制和公开可见性；
- 所有枚举、月份、数组去重和字符长度；
- 零张图片可以投稿，以及图片数量上限、排序、OSS URL 归属校验；
- 投稿图片缓存命中、未命中、失败和 admin URL 改写；
- MySQL、Mongo、in-memory 行为一致；
- 审核 `expected_revision` 冲突；
- 推荐双通道翻页无重复、无遗漏；
- 投稿撤回或失效后立即退出推荐；
- RouteGroup 手动代表、投稿代表和 medoid 的优先级；
- 自动代表切换不修改路线介绍锚点；
- Track 删除、转私密或关键更新时投稿失效。

### 19.2 客户端验收

- 投稿表单完整填写，无图片时可以提交；有图片时支持上传、排序和预览；
- 枚举与服务端 options 一致；
- 待审核、通过、驳回、撤回和失效状态正确展示；
- 驳回后可以修改并重投；
- 推荐列表正确显示投稿标题、徽标和封面；
- 投稿详情显示全部路线属性；有沿途图片时正确展示，无图片时返回空数组；
- 图片只使用服务端静态缓存地址。

### 19.3 管理后台验收

- 投稿列表筛选和分页；
- 投稿内容、轨迹地图、图片及历史版本完整展示；
- 审核通过、驳回和 revision 冲突处理；
- admin 图片 URL 使用 `/admin/api/static/`；
- 审核结果及时影响推荐和代表候选资格。

## 20. 实施阶段建议

### 第一阶段：投稿与审核闭环

- 数据表和三套 Repository；
- 客户端投稿接口和 options；
- OSS URL 保存、投稿图片缓存和地址改写；
- 我的轨迹投稿状态；
- 管理后台审核页面；
- 审核事件流水。

### 第二阶段：推荐优先展示

- 推荐双通道查询和版本化游标；
- 投稿标题、简介、徽标和封面；
- 推荐曝光、点击、收藏和导航埋点。

### 第三阶段：RouteGroup 代表轨迹

- Geometry Anchor 与 Representative Track 解耦；
- 投稿代表选择和手动代表持久化；
- 投稿撤回/失效时代表回退；
- 从投稿复制 RouteGroup 介绍草稿的运营能力。

## 21. 实现时必须同步的文件

正式落地时至少需要检查并更新：

- `internal/models/` 投稿模型和 Track/RouteGroup 响应模型；
- `internal/repository/interfaces.go`；
- `internal/repository/mysql.go`、`mongo.go`、`memory.go`；
- `internal/service/track_service.go`、`track_route_group_service.go`；
- `internal/handler/router.go` 和投稿 handler；
- `internal/middleware/account_restriction.go`；
- `internal/admin/routes.go`、`handlers.go`、`static/`；
- `cmd/server/main.go` 依赖注入；
- `mysql.sql`；
- `docs/api/track.md`、`docs/api/track-map.md`、`docs/api/route-index.md`；
- `track_map.md`；
- 根目录及 `internal/admin/AGENTS.md`。

提交前执行：

```bash
go build ./...
make test
```
