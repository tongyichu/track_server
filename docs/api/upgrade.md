# App 升级接口

> 公共请求和错误响应见 [common.md](common.md)。

## 15. App 升级检查

> 客户端在「启动 / 切回前台 / 设置页手动检查」等场景调用，根据当前平台与本地 `version_code` 询问服务端是否有新版本。

### 请求

`GET /api/v1/upgrade/check`

**注意：本接口为公开接口，不需要 `Authorization` Header。**

### 请求头

| Header 名称 | 类型 | 必填 | 说明 |
|-------------|------|------|------|
| `X-Platform` | string | 是 | 客户端平台，固定为 `android` 或 `ios`（沿用公共请求头，不再单独通过 query 传递） |

### Query 参数

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `version_code` | int64 | 是 | 当前安装版本的 `version_code`（单调递增整数）。未知时传 `0` |

### 请求示例

```
GET /api/v1/upgrade/check?version_code=120
X-Platform: android
```

### 响应

**状态码：** `200 OK`

返回统一响应格式 `StandardResponse`，`data` 为 `UpgradeCheckResult`。

#### 字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `has_update` | bool | 是否存在可升级版本。`false` 时其它字段无意义 |
| `force_update` | bool | 是否强制升级。判定规则见下文 |
| `latest_version_name` | string | 服务端最新发布版本的版本名（如 `1.2.3`） |
| `latest_version_code` | int64 | 服务端最新发布版本的 `version_code` |
| `min_supported_version_code` | int64 | 最低支持版本号；客户端 `version_code < min_supported_version_code` 时必须强升 |
| `package_url` | string | 安装包下载地址。**Android** 为 OSS 直接下载链接；**iOS** 为 AppStore 跳转链接 |
| `package_size` | int64 | 安装包大小（字节）。iOS 通常为 `0` |
| `package_md5` | string | 安装包 MD5（可选）。iOS 通常为空 |
| `release_notes` | string | 版本说明文案，可包含换行 |

#### 强制升级判定逻辑

服务端会基于以下规则计算 `force_update`：

1. 若服务端没有任何 `published` 状态的版本，返回 `has_update=false`；
2. 若 `version_code >= latest_version_code`，返回 `has_update=false`；
3. 否则 `has_update=true`，并按以下任一条件触发强升（`force_update=true`）：
   - 客户端 `version_code < min_supported_version_code`
   - 服务端最新版本被标记为 `force_update=true`

客户端处理建议：
- `has_update=false`：不提示。
- `has_update=true && force_update=false`：可关闭的升级提示。
- `has_update=true && force_update=true`：不可关闭的升级提示，未升级前阻断核心功能。

### 响应示例

#### 1）有可升级版本，非强制

```json
{
  "code": 0,
  "data": {
    "has_update": true,
    "force_update": false,
    "latest_version_name": "1.3.0",
    "latest_version_code": 130,
    "min_supported_version_code": 100,
    "package_url": "https://track-resource.oss-cn-beijing.aliyuncs.com/release/android/1714378800-app-1.3.0.apk",
    "package_size": 28456321,
    "package_md5": "a1b2c3d4e5f60718293a4b5c6d7e8f90",
    "release_notes": "1. 新增轨迹分享\n2. 修复地图卡顿"
  }
}
```

#### 2）当前版本过低，必须强升

```json
{
  "code": 0,
  "data": {
    "has_update": true,
    "force_update": true,
    "latest_version_name": "1.3.0",
    "latest_version_code": 130,
    "min_supported_version_code": 120,
    "package_url": "https://apps.apple.com/cn/app/idXXXXXXXXX",
    "package_size": 0,
    "package_md5": "",
    "release_notes": "本次为安全更新，必须升级后才能继续使用。"
  }
}
```

#### 3）已是最新或服务端无发布版本

```json
{
  "code": 0,
  "data": {
    "has_update": false,
    "force_update": false,
    "latest_version_name": "",
    "latest_version_code": 0,
    "min_supported_version_code": 0,
    "package_url": "",
    "package_size": 0,
    "package_md5": "",
    "release_notes": ""
  }
}
```

### 错误响应

- `400 Bad Request`
  - `version_code` 非整数（返回 `{"error":"version_code must be an integer"}`）
  - `platform` 不是 `android` / `ios`（返回 `{"error":"platform must be android or ios"}`）
  - `version_code < 0`（返回 `{"error":"current_version_code must be >= 0"}`）
- `500 Internal Server Error`
  - 服务端查询发布信息失败

---

## 16. 运维发布 Android App

> 供可信发布脚本一次性上传 APK 并写入 `published` 发布记录。客户端随后调用升级检查接口即可发现该版本。

### 请求

`POST /api/v1/ops/releases/android`

请求使用 `multipart/form-data`，不使用业务 JWT。服务端必须配置 `OPS_INTERNAL_TOKEN`，调用方通过 `X-Internal-Token` 传入相同 token。

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `file` | file | 是 | Release APK，最大 500 MiB，文件名必须以 `.apk` 结尾 |
| `version_name` | string | 是 | 版本名，如 `1.0.1`，最长 32 字符 |
| `version_code` | int64 | 是 | 大于 0 的单调递增版本号 |
| `min_supported_version_code` | int64 | 否 | 最低支持版本号，默认 `0`，不得大于 `version_code` |
| `release_notes` | string | 否 | 用户可见的版本说明 |
| `force_update` | bool | 否 | 是否强制升级，默认 `false` |

请求示例：

```bash
curl --fail-with-body \
  -H "X-Internal-Token: <ops-internal-token>" \
  -F "file=@app-release.apk;type=application/vnd.android.package-archive" \
  -F "version_name=1.0.1" \
  -F "version_code=2" \
  -F "release_notes=修复已知问题" \
  http://server.example.com/api/v1/ops/releases/android
```

### 成功响应

服务端把 APK 写入 `<LogDir>/static/release/android/`，计算文件大小与 MD5，并以 `android-track-release` 记录操作人。返回的 `data` 为完整 `AppRelease`：

```json
{
  "code": 0,
  "data": {
    "id": 12,
    "platform": "android",
    "version_name": "1.0.1",
    "version_code": 2,
    "min_supported_version_code": 0,
    "package_url": "/api/v1/static/release/android/1792584000000-track-1.0.1-2.apk",
    "package_size": 95420416,
    "package_md5": "a1b2c3d4e5f60718293a4b5c6d7e8f90",
    "release_notes": "修复已知问题",
    "force_update": false,
    "status": "published",
    "operator_user_id": 0,
    "operator_name": "android-track-release"
  }
}
```

发布记录写入失败时，服务端会删除本次上传的文件，不留下未发布安装包。

### 错误响应

- `400 Bad Request`：字段格式无效、版本号不合法、文件为空或不是 APK。
- `401 Unauthorized`：缺少 `X-Internal-Token`。
- `403 Forbidden`：token 不匹配。
- `413 Request Entity Too Large`：APK 超过 500 MiB。
- `503 Service Unavailable`：未配置内部鉴权或发布包存储目录。
- `500 Internal Server Error`：文件落盘或发布记录写入失败。

---
