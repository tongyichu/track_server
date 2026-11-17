# 背景音乐乐库

基础鉴权和错误格式沿用[公共协议](common.md)。乐库用于客户端 3D 轨迹预览和本地视频导出，不是外部音乐检索或音乐版权采购服务；初始为空，不自动检索第三方音乐。

## 客户端接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/v1/music` | JWT；只返回已上架曲目，按 sort_order、id 升序 |
| GET | `/api/v1/music/:music_id/audio?v=:sha256` | JWT；当前已上架音频，支持文件 Range |
| GET | `/api/v1/music/:music_id/cover?v=:sha256` | JWT；当前已上架封面 |

查询允许受账号限制的用户读取，沿用 AccountRestrictionMiddleware 的 GET 规则；无业务写接口。列表和资源均不放到通用 static，本站资源每次核验正式条目、上架状态和版本；外链通过乐库已上架条目下发，后续由浏览器/App直接读取，不提供服务器整首代理或跳转。不存在、已下架或版本不符返回404，存储故障503。外链条目的本站audio资源路径返回404，客户端应读取audio_url/audio_link。资源 `Cache-Control: private, no-store`；客户端显式下载到私有文件供离线配乐，不依赖 HTTP 缓存。

列表结构：`{code:0,data:{items:[...]}}`。单项字段：

- `id`：32位随机十六进制字符串；`revision`：元数据修改版本。
- `name`、`artist`：名称及歌手/作者。
- `duration_ms`：毫秒；上传文件由服务端解析，外链由网页播放器读取后提交，未知为0。`audio_bytes`：上传文件实际大小或外链声明大小，未知为0。
- `audio_sha256`：仅上传文件有真实SHA，外链为空。`audio_link`：外链原地址，仅外链条目有值。`audio_url`：上传为带v的本站资源地址，外链为原链接。
- `audio_version`：上传文件等于SHA；外链为URL、ETag、Last-Modified及声明大小的版本摘要，不代表完整文件SHA。客户端区分两种来源，外链不携带本站JWT。
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

保存使用multipart/form-data：`metadata`为JSON；`audio`为可选音频文件，`cover`为可选封面。新建metadata的id为空、revision为0，选择上传MP3或填写audio_link；修改id与路径一致、revision等于最近读取版本，省略文件保留旧资源。上传文件的SHA、时长和大小仍由服务端派生或保留；外链可提交duration_ms（0或1000～1200000），SHA固定为空、版本和声明大小由探测派生。文件与链接不能同时提供。切回上传方式时audio_link传空，若原条目为外链则必须提供新文件。保存返回`{code:0,data:曲目}`，revision增加；版本冲突409，非法输入400，超大请求413，仓储不可用503。

上传音频首版只接受MP3，1秒～20分钟、最大30MiB，校验MPEG帧，不信文件扩展名；封面PNG/JPEG，最大5MiB、2048×2048，服务端转JPEG。请求总体38MiB上限，资料名称/作者各240字节、来源500字节、署名1200字节、授权类型100字节；sort_order为-9999～9999。上架前要求来源及授权类型，CC BY类型还要求署名。后台提示运营者确认作曲/录音、App分发、试听、剪辑及用户视频导出分享权限；不会自动购买、联系作者或保证第三方平台Content ID免申诉。

## 持久化与资源

新增 `background_music` 表/集合，MySQL每条为id主键、revision、payload JSON；Mongo使用相同字符串_id及revision条件替换；memory用锁和副本。三者同一CAS语义，无跨库切换。MySQL创建表失败时乐库拒绝请求，不把目录降级成易失内存；业务显式memory模式/既有业务连接降级分支才用memory目录。

上传媒体按SHA-256写入`<LogDir>/music/<hash>.mp3|.jpg`，临时文件原子改名后保存目录元信息；不置入公共static，生产需保留既有LogDir持久卷。更新、下架不立即物理删除旧文件，避免影响正在使用或已授权导出的副本；当前未做垃圾回收任务。不增加配置或OSS上传权限；原链接通过目录JSON/BSON持久化，不下载整首或落盘。List/资产元信息查询5秒超时，后台保存45秒；失败不下发未保存资源链接。

## 外链轻量校验与网页试听

后台音频来源选择上传文件/使用链接。填写外链后显示原生audio controls、preload=none播放器，点击播放由浏览器直接访问；能读取有限时长时自动填写秒数，也可手填。更换链接会停止旧播放器并清除旧时长，换来源、关闭弹窗或离页释放播放器和本地Object URL，切后台停止声音。错误提示仅说明当前不能播放，不把页面检查等同于整首解码成功。页面Referrer-Policy=no-referrer，不向外链附加后台或业务Authorization。

新建外链、替换链接或保存上架外链时，服务端使用无登录态的独立HTTP客户端发送HEAD。HEAD不能确认MP3时（不支持HEAD、常见HEAD拒绝、空/未知类型）最多GET开头2048字节并检查ID3/MPEG头，服务端不保留这些字节；即使对方忽略Range也只读取限额后关闭。拒绝非2xx、空响应、明显非MP3及声明大于30MiB的资源；这是可达性/类型的轻量检查，不保证完整可播放或未来永不失效。已下架且链接未改的资料保存不重新探测，以便失效链接也能正常下架。

仅HTTP/HTTPS标准端口、无userinfo/fragment、最多两次重定向，不接受HTTPS降级；每跳检查URL，DNS解析后直接拨号到同一已验证公网IP，拒绝私网/回环/链路本地/元数据及保留网段。独立连接不使用环境代理、cookie或业务Token；并发最多2、单次含排队15秒、请求12秒/响应头6秒，所有请求随保存context取消，不输出原始URL或响应正文。只在管理写操作内检查，乐库GET不探测、不发外部请求。沿用music写权限、同源与审计，无新增路由、Repository方法或表结构；三实现已有JSON/BSON保存新增可选字段。

客户端先直接试听原链接（无App鉴权）；下载时独立无凭据客户端限制重定向及公网DNS地址，最大30MiB，下载完在本机读取MP3类型/时长并记录实际SHA和大小，成功才发布下载状态。未知时长展示--:--，未知长度显示“下载中”而非编造百分比；实际下载后的本机时长优先。上游无ETag/Last-Modified且同地址同大小替换音频时不能可靠感知版本，运营者应更换稳定版本地址。
