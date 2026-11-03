# 地区目录

地区目录是客户端选城和后台地区资料的唯一数据源。服务端首次启动且存储中没有目录时，从 `internal/regions/seed.json` 导入国内省市；这份初始数据来自 Android 端原有 JSON，保留 34 个省级编码、337 个城市编码、全部经纬度和已有坐标状态。此后只读取数据库中的目录。国内现有树节点 `id`、外部编码 `code`、层级与父级保持稳定。目录协议仍保留邮编与 `active` 字段，已有值在后台保存时保持不变；新建节点默认 `active: true`。

城市业务字段统一使用 `region_id`：国家根节点是两位国家缩写（如 `CN`），该国家下所有省、市和其他节点共用六位递增序号（如 `CN000003`），序号不按层级分段。已分配编号不可改、不可复用，新增节点只取该国家已有序号的最大值加一。旧目录首次读取时按既有节点存储顺序补齐编号并保存新版本。`id` 和 `parent_id` 仍是目录树的旧稳定键，供旧目录及管理页兼容，不作为新业务记录的城市编号。

第三方地区标识存在节点的 `external_ids` 数组，每项为 `{provider,type,value}`，当前管理页支持 `amap/adcode` 和 `google/place_id`；同一个来源、类型和值不能指向两个节点。旧 `code_system=cn_adcode` + `code` 无需批量迁移，服务端自动视为高德映射并继续保留作历史兼容。Google 地区 Place ID 与景点 Place ID 分开记录，不能互用；Google 标识可能变化，管理员可更新映射。外部标识只用于解析到 `region_id`，不替代内部编号。

## 客户端读取

`GET /api/v1/regions/catalog`，需要 Bearer token。首次读取返回完整目录；可传 `known_version=<本地版本>`。如果版本一致，响应只包含版本和 `unchanged: true`，否则返回完整节点列表。客户端先读取私有目录中的 `region_catalog.json`，再检查服务端版本；下载成功后原子写入本地文件。无网络时继续使用已有文件。后台保存后版本递增。

完整响应示例：

```json
{
  "code": 0,
  "data": {
    "version": 1,
    "nodes": [
      {"id":"country:CN","region_id":"CN","parent_id":"","name":"中国","level":"country","code":"CN","code_system":"iso3166-1","country_code":"CN","continent":"AS","selectable":false,"active":true,"sort_order":0},
      {"id":"cn:330100","region_id":"CN000118","parent_id":"cn:330000","name":"杭州市","level":"city","code":"330100","code_system":"cn_adcode","country_code":"CN","latitude":30.246026,"longitude":120.210792,"selectable":true,"active":true,"sort_order":83}
    ]
  }
}
```

`nodes` 是完整树。字段包括 `id`、`region_id`、`parent_id`、`name`、`name_en`、`level`、`code`、`code_system`、`external_ids`、`country_code`、`continent`、`postal_codes`（字符串数组）、`slogan`、`description`、`cover_url`、`latitude`、`longitude`、`coordinate_status`、`selectable`、`active`、`sort_order`。可选字段可能缺省，未设置的 `external_ids` 可省略。只有本身和祖先均启用、且 `selectable=true` 的节点进入客户端城市选择器。客户端以 `region_id` 写入用户和轨迹的 `city_code`，旧 adcode 和目录键仅在兼容读写时解析。国外层级由后台按国家下的任意树结构组织。

版本未变化时：

```json
{"code":0,"data":{"version":1,"unchanged":true}}
```

## 后台管理

`/admin/regions.html` 显示为“城市管理”，使用后台会话和独立的城市管理权限。国家以顶部标签切换，末尾的加号新增国家；每个国家内的下级节点以可折叠连线树呈现，有下级的节点默认折叠。地区资料提供高德 adcode 和 Google 地区 Place ID 输入；老国内节点的原 `code` 保留，修改高德字段可增加新的映射，不会抹掉旧编码。页面不提供邮编、启用状态、归档、层级、上级地区、编码体系、排序值和图片地址的直接编辑；所属洲只在国家节点显示。新增下级时固定父节点，国内按父级推断层级，其他地区默认为城市层级；排序值在同级末尾生成，上传图片后由响应写入地址。已有隐藏字段随整份目录保存保持原值；未保存的新节点仍可删除。

| 方法与路径 | 权限 | 用途 |
| --- | --- | --- |
| `GET /admin/api/regions/catalog` | 查看 | 读取完整目录和版本 |
| `PUT /admin/api/regions/catalog` | 操作 | 以 `{version,nodes}` 保存完整目录；版本不匹配返回 409 |
| `POST /admin/api/regions/images` | 操作 | multipart `image`，JPEG/PNG，最多 8 MiB；返回 `cover_url` |

国外可新增国家和任意下级节点。新国家须填写两位国家缩写，保存时由服务端分配不可变 `region_id`；新增节点的目录树 `id` 仍由管理页生成，仅用于父子关联。已保存节点不删除，以保留历史用户和轨迹编码；本次编辑中未保存的新节点可以直接移除。封面图保存在服务端静态目录，需随服务端静态文件一起持久化和备份。

MySQL 使用 `region_catalog_state` 单行 JSON 与版本号；Mongo 使用同名集合；本地内存模式只在当前进程有效。目录更新由版本号比较保护，避免多个管理员覆盖彼此的修改。
