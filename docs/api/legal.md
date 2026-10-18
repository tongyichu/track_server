# 隐私政策与用户服务协议 H5

## 公开页面

| 内容 | 方法 | 路径 | 登录 |
| --- | --- | --- | --- |
| 隐私政策 | GET | `/api/v1/legal/privacy-policy.html` | 无需 |
| 用户服务协议 | GET | `/api/v1/legal/terms-of-service.html` | 无需 |

响应为 `text/html; charset=utf-8`。页面通过 `go:embed` 内置在二进制，无需部署额外静态目录，不依赖数据库或业务 JWT。

| 查询参数 | 规则 |
| --- | --- |
| `lang` | `english` 显示英文，其余默认简体中文 |
| `is_dark` | `true` 夜间、`false` 日间；未指定或其他值跟随浏览器系统主题 |

示例：`/api/v1/legal/privacy-policy.html?lang=english&is_dark=true`。正文中的协议互链保留语言与显式主题参数，目录锚点支持页内跳转。关于我们页通过已有 `WebViewActivity` 打开，按实际 App 主题传 `is_dark`，非中文语言传 `lang=english`；主机来源复用 `ApiClient.resolveUrl`。

## 内容维护

- HTML：`internal/handler/static/privacy_policy.html`、`terms_of_service.html`；路由处理：`internal/handler/legal_handler.go`。
- 风格沿用 `achievement_level_rules.html`，页面自身无外部脚本、字体或统计请求。
- 当前为可预览草稿，中英文都保留 `{xxxx}` 形式占位，页首有草稿提示；不能当作已正式生效、已通过合规审核的政策。
- 运营主体、地址、邮箱、版本及日期、保留与清理期限、部署区域、第三方准确主体/字段/联系方式、授权入口、儿童规则及注销流程应由运营方补齐。不能把代码里的建议保留天数当作已落实的删除周期。
- 当前文本按中国大陆用户场景拟定；若服务其他法域，应补充当地适用内容。
- 同步维护中英文版本；敏感信息、公开范围或数据流变化时同步核查正文、真实授权流程和第三方清单。

## 正式启用前与实现对齐

以下为代码核对结果，页面文案不能代替这些能力：

1. `TrackApplication` 启动时初始化并记录自建统计事件；当前未核实到首次隐私确认及统计选择入口。上线前需落实适用的同意机制、拒绝路径和初始化顺序。
2. 主 Android Manifest 未申请 `ACCESS_BACKGROUND_LOCATION`，录制与同行通过位置前台服务在后台持续运行。须在启用对应功能前落实敏感位置的单独告知/同意，并说明两个服务分别停止。
3. `TrackCreateUploader` 完成本地保存后尝试创建云端摘要；`TrackService.CreateTrack` 以 `TrackStatusNormal` 创建普通云端轨迹。推荐、搜索和地图中会使用符合条件的普通轨迹，不能表述为“默认仅自己可见”。公开与上传需要对应确认；隐私设置入口尚待核实，不虚构入口。
4. 云端轨迹删除是软删除；数据库、OSS 原始文件、截图、缓存和备份的完整清理与请求处理周期尚需落实。注销接口/自助入口未核实可用，反馈不能代替完整注销实现。
5. API 当前配置为 HTTP，不能声称“全程 HTTPS 加密”。安全措施为占位，需核实并落实正式环境传输与存储保护。
6. Google 服务可能涉及境外处理，实际部署区域、接收方和跨境手续尚未核实；不要承诺“所有数据只在境内”。
7. 当前短信发送服务仍有 TODO，不能虚构已经接入的短信服务商。微信、Google、高德和 ML Kit 的字段要按实际版本、启用模块和官方披露逐项核对。
8. 服务端具备个性化推荐，非个性化入口及统计控制需落实；未成年人服务范围、监护人验证及儿童专门规则需明确。

本次仅增加页面、公开路由和关于我们入口，未实现上述独立的授权、注销、清理或跨境处理能力，未部署线上服务。

## 起草参考（核查于 2026-10-13）

- [《个人信息保护法》](https://www.cac.gov.cn/2021-08/20/c_1631050028355286.htm)：告知、敏感信息、个人权利、第三方及出境处理框架。
- [《App违法违规收集使用个人信息行为认定方法》](https://www.cac.gov.cn/2019-12/27/c_1578986455686625.htm)：访问可读性、逐项披露、授权、注销和投诉处理。
- [《民法典》](https://www.cac.gov.cn/2020-06/01/c_15925617772683193.htm)：格式条款、责任及隐私权益框架。
- [高德开放平台隐私政策](https://lbs.amap.com/pages/privacy/)。
- [Google Maps SDK 数据披露](https://developers.google.com/maps/documentation/android-sdk/play-data-disclosure)、[Places SDK 政策](https://developers.google.com/maps/documentation/places/android-sdk/policies)、[Google 隐私政策](https://policies.google.com/privacy)。
- [ML Kit 条款与隐私说明](https://developers.google.com/ml-kit/terms)。

参考资料用于辅助起草，第三方处理范围仍须与实际接入版本一致，页面本身不能证明 App 合规。
