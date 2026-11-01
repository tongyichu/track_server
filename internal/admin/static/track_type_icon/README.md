# MingCute 运动类型图标

来源：[MingCute 官方仓库](https://github.com/mingcute-design/mingcute-icons/tree/ca98bb55513a22ff0dab713aaf7870b512944653/assets/svg/core/regular)，Apache-2.0，许可证见同目录 `LICENSE`。

| 运动类型 | 上游原始文件 | 本地文件 |
| --- | --- | --- |
| 徒步 | `sports/hiking.svg` | `mingcute-hiking-line-v1.svg` |
| 跑步 | `sports/run.svg` | `mingcute-run-line-v1.svg` |
| 爬山 | `nature/mountain_2.svg` | `mingcute-mountain-2-line-v1.svg` |
| 骑行 | `transportation/bike.svg` | `mingcute-bike-line-v1.svg` |
| 自驾 | `transportation/car.svg` | `mingcute-car-line-v1.svg` |

SVG 保持上游原样，使用 24×24 viewBox、透明背景，客户端按场景染色。文件通过 `track_type_icon_handler.go` 内置进二进制，由登录鉴权后的 `/api/v1/track/types/icons/:icon_file` 返回，不依赖服务器磁盘静态目录。

更换图形时必须修改文件名版本，并同步 `config.DefaultTrackTypeConfigs` 的 `IconFile`，避免命中客户端旧图片缓存。自定义运动类型仍沿用原静态资源路径。

旧的 `fill-v1` 文件继续保留，供尚未刷新运动类型元数据的客户端读取；当前配置统一下发 `line-v1` 文件。
