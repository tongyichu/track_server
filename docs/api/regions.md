# 地区目录

地区目录是客户端选城和后台地区资料的唯一数据源。服务端首次启动且存储中没有目录时，从 `internal/regions/seed.json` 导入国内省市；这份初始数据来自 Android 端原有 JSON，保留 34 个省级编码、337 个城市编码、全部经纬度和已有坐标状态。此后只读取数据库中的目录。国内现有 ID、编码、层级与父级不可更改；名称、坐标、宣传口号、简介、封面及启用状态可编辑。

## 客户端读取

`GET /api/v1/regions/catalog`，需要 Bearer token。首次读取返回完整目录；可传 `known_version=<本地版本>`。如果版本一致，响应只包含版本和 `unchanged: true`，否则返回完整节点列表。客户端先读取私有目录中的 `region_catalog.json`，再检查服务端版本；下载成功后原子写入本地文件。无网络时继续使用已有文件。后台保存后版本递增。

完整响应示例：

```json
{
  "code": 0,
  "data": {
    "version": 1,
    "nodes": [
      {"id":"country:CN","parent_id":"","name":"中国","level":"country","code":"CN","code_system":"iso3166-1","country_code":"CN","continent":"AS","selectable":false,"active":true,"sort_order":0},
      {"id":"cn:330100","parent_id":"cn:330000","name":"杭州市","level":"city","code":"330100","code_system":"cn_adcode","country_code":"CN","latitude":30.246026,"longitude":120.210792,"selectable":true,"active":true,"sort_order":83}
    ]
  }
}
```

`nodes` 是完整树。字段包括 `id`、`parent_id`、`name`、`name_en`、`level`、`code`、`code_system`、`country_code`、`continent`、`postal_codes`（字符串数组）、`slogan`、`description`、`cover_url`、`latitude`、`longitude`、`coordinate_status`、`selectable`、`active`、`sort_order`。可选字段可能缺省。只有本身和祖先均启用、且 `selectable=true` 的节点进入客户端城市选择器。国内使用 `code` 作为城市编码；国外使用六位 `id`。国外层级由后台按国家下的任意树结构组织。

版本未变化时：

```json
{"code":0,"data":{"version":1,"unchanged":true}}
```

## 后台管理

`/admin/regions.html` 显示为“城市管理”，使用后台会话和独立的城市管理权限。国家以顶部标签切换，末尾的加号新增国家；每个国家内的下级节点以可折叠连线树呈现，有下级的节点默认折叠。

| 方法与路径 | 权限 | 用途 |
| --- | --- | --- |
| `GET /admin/api/regions/catalog` | 查看 | 读取完整目录和版本 |
| `PUT /admin/api/regions/catalog` | 操作 | 以 `{version,nodes}` 保存完整目录；版本不匹配返回 409 |
| `POST /admin/api/regions/images` | 操作 | multipart `image`，JPEG/PNG，最多 8 MiB；返回 `cover_url` |

国外可新增国家和任意下级节点。新节点采用六位内部 ID，已保存节点用归档代替删除，以保留历史用户和轨迹编码；本次编辑中未保存的新节点可以直接移除。封面图保存在服务端静态目录，需随服务端静态文件一起持久化和备份。

MySQL 使用 `region_catalog_state` 单行 JSON 与版本号；Mongo 使用同名集合；本地内存模式只在当前进程有效。目录更新由版本号比较保护，避免多个管理员覆盖彼此的修改。
