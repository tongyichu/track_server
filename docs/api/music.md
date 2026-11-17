# 背景音乐乐库

基础鉴权和错误格式沿用[公共协议](common.md)。乐库用于客户端 3D 轨迹预览和本地视频导出，不是外部音乐检索或音乐版权采购服务；初始为空，不自动抓取第三方音乐。

## 客户端接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/v1/music` | JWT；只返回已上架曲目，按 sort_order、id 升序 |
| GET | `/api/v1/music/:music_id/audio?v=:sha256` | JWT；当前已上架音频，支持文件 Range |
| GET | `/api/v1/music/:music_id/cover?v=:sha256` | JWT；当前已上架封面 |

查询允许受账号限制的用户读取，沿用 AccountRestrictionMiddleware 的 GET 规则；无业务写接口。列表和资源均不放到通用 static，资源每次核验正式条目、上架状态和版本。不存在、已下架或版本不符返回404，存储故障503。资源 `Cache-Control: private, no-store`；客户端显式下载到私有文件供离线配乐，不依赖 HTTP 缓存。

列表结构：`{code:0,data:{items:[...]}}`。单项字段：

- `id`：32位随机十六进制字符串；`revision`：元数据修改版本。
- `name`、`artist`：名称及歌手/作者。
- `duration_ms`：服务端按 MPEG Layer III 帧解析的时长，毫秒；`audio_bytes`：实际文件大小。
- `audio_sha256`、`audio_url`：音频完整性及带 v 参数的本站相对地址。
- `cover_sha256`、`cover_url`：封面版本及地址，无封面时URL为空，客户端显示默认音符。
- `license`、`source`、`attribution`：授权类型、作品来源、署名文案；不承诺这些字段代表平台已代替运营者审核权利。
- `published`、`sort_order`、`updated_at`：上下架、排序、UTC更新时间。

乐库定位为小型运营精选列表，最多500首，不扫描无限集合；默认不提供分类、音乐裁剪、用户上传或云端合成。客户端按当前用户隔离目录与列表快照，版本或校验不一致的文件不能显示为已下载；保留最近列表供离线读取。已经下载的合法副本不因接口下架而强制远程删除，但停止新的试听和下载；如授权要求撤回本地副本，须另行设计撤销协议，不能声称现有机制具备DRM。

## 后台管理

页面 `/admin/music.html`，独立 `music` 模块的 none/read/write 权限。管理员继承所有权限，已有操作员不自动获得新模块权限。HTML/API分别校验；沿用独立admin_session、同源写校验和发起/结果审计。

| 方法 | 路径 | 权限 |
| --- | --- | --- |
| GET | `/admin/api/music` | music读；包括未上架条目 |
| POST | `/admin/api/music` | music写；新建 |
| PUT | `/admin/api/music/:music_id` | music写；修改、排序、上下架 |
| GET | `/admin/api/music/:music_id/:kind?v=:sha256` | music读；kind仅audio/cover，允许未上架预览 |

保存使用multipart/form-data：`metadata`为JSON；`audio`为可选音频文件，`cover`为可选封面。新建metadata的id为空、revision为0，必须上传音频；修改id与路径一致、revision等于最近读取版本，省略文件保留旧资源。禁止由客户端提供SHA、时长或大小冒充上传；这些字段均由服务端派生或保留原值。保存返回`{code:0,data:曲目}`，revision增加；版本冲突409，非法输入400，超大请求413，仓储不可用503。

音频首版只接受MP3，1秒～20分钟、最大30MiB，校验MPEG帧，不信文件扩展名；封面PNG/JPEG，最大5MiB、2048×2048，服务端转JPEG。请求总体38MiB上限，资料名称/作者各240字节、来源500字节、署名1200字节、授权类型100字节；sort_order为-9999～9999。上架前要求来源及授权类型，CC BY类型还要求署名。后台提示运营者确认作曲/录音、App分发、试听、剪辑及用户视频导出分享权限；不会自动购买、联系作者或保证第三方平台Content ID免申诉。

## 持久化与资源

新增 `background_music` 表/集合，MySQL每条为id主键、revision、payload JSON；Mongo使用相同字符串_id及revision条件替换；memory用锁和副本。三者同一CAS语义，无跨库切换。MySQL创建表失败时乐库拒绝请求，不把目录降级成易失内存；业务显式memory模式/既有业务连接降级分支才用memory目录。

媒体按SHA-256写入`<LogDir>/music/<hash>.mp3|.jpg`，临时文件原子改名后保存目录元信息；不置入公共static，生产需保留既有LogDir持久卷。更新、下架不立即物理删除旧文件，避免影响正在使用或已授权导出的副本；当前未做垃圾回收任务。无新配置/外部CDN/OSS上传权限。List/资产元信息查询5秒超时，后台保存45秒；失败不下发未保存资源链接。
