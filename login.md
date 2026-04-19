# 用户登录接口文档

> Base URL: `http://<host>:<port>/api/v1`
>
> 所有请求和响应均使用 **JSON** 格式，Content-Type 统一为 `application/json`。

---

## 目录

| 序号 | 接口 | 方法 | 路径 |
|------|------|------|------|
| 1 | [获取图形验证码](#1-获取图形验证码) | GET | `/captcha` |
| 2 | [发送短信验证码](#2-发送短信验证码) | POST | `/sms/send` |
| 3 | [短信验证码登录](#3-短信验证码登录) | POST | `/login/sms` |
| 4 | [微信登录](#4-微信登录) | POST | `/login/wechat` |
| 5 | [Apple 登录](#5-apple-登录) | POST | `/login/apple` |
| 6 | [查询登录日志](#6-查询登录日志) | GET | `/login/log` |

---

## 整体登录流程

### 短信登录流程

```
客户端                                          服务端
  │                                               │
  │  1. GET /api/v1/captcha                        │
  │ ────────────────────────────────────────────►   │
  │   ◄──── 返回 code=0, data={captcha_id, captcha_img} │
  │                                               │
  │  用户输入图形验证码                              │
  │                                               │
  │  2. POST /api/v1/sms/send                      │
  │     { phone, captcha_id, captcha_code }        │
  │ ────────────────────────────────────────────►   │
  │   ◄──── 返回 code=0, data={code}（仅开发环境返回） │
  │                                               │
  │  用户收到短信并输入验证码                         │
  │                                               │
  │  3. POST /api/v1/login/sms                     │
  │     { phone, code }                            │
  │ ────────────────────────────────────────────►   │
  │   ◄──── 返回 code=0, data={user_id, user}      │
  │                                               │
```

### 微信 / Apple 登录流程

```
客户端                                          服务端
  │                                               │
  │  客户端调用微信/Apple SDK 获取授权凭证            │
  │                                               │
  │  POST /api/v1/login/wechat 或 /login/apple     │
  │     { code } 或 { apple_user_id, ... }         │
  │ ────────────────────────────────────────────►   │
  │   ◄──── 返回 user_id + user 信息               │
  │                                               │
```

---

## 公共请求头

以下请求头建议在所有登录类请求中携带（非必填），服务端会将其记录到登录日志中用于审计和风控：

| Header 名称 | 类型 | 必填 | 说明 |
|-------------|------|------|------|
| `X-Device-ID` | string | 否 | 设备唯一标识，用于判断新设备登录、设备维度限流和风控 |
| `X-Platform` | string | 否 | 客户端平台，取值：`ios` / `android` |
| `X-Client-Version` | string | 否 | 客户端版本号，如 `1.0.0` |
| `X-Client-Language` | string | 否 | 客户端语言，如 `zh-CN`、`en-US` |

---

## 公共错误响应格式

所有接口在发生错误时统一返回以下 JSON 格式：

```json
{
  "error": "错误描述信息"
}
```

| HTTP 状态码 | 含义 |
|------------|------|
| 400 | 请求参数缺失或格式错误 |
| 401 | 认证失败（验证码错误、过期等） |
| 500 | 服务端内部错误 |

---

## 1. 获取图形验证码

获取一张图形验证码图片，用于在发送短信验证码前进行人机校验。

### 请求

```
GET /api/v1/captcha
```

**请求参数：** 无

### 响应

**状态码：** `200 OK`

```json
{
  "code": 0,
  "data": {
    "captcha_id": "cap_1713520800123456789_42135",
    "captcha_img": "data:image/svg+xml;base64,PHN2ZyB4bWxucz0i..."
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `code` | int | 业务状态码，`0` 表示成功 |
| `data.captcha_id` | string | 验证码唯一标识，发送短信时需回传此值 |
| `data.captcha_img` | string | 验证码图片，base64 编码的 SVG Data URI，可直接赋值给 `<img src="">` 或 Image 组件 |

### 注意事项

- 图形验证码有效期为 **5 分钟**
- 每个 `captcha_id` **仅可使用一次**，校验成功后立即失效
- 验证码为 **4 位数字**

### 客户端使用示例

```swift
// iOS - 将 captcha_img 直接用于显示
if let data = Data(base64Encoded: captchaImgBase64Part) {
    imageView.image = UIImage(data: data)
}
```

```kotlin
// Android - 使用 Glide 加载 Data URI
Glide.with(context).load(captchaImg).into(imageView)
```

---

## 2. 发送短信验证码

校验图形验证码后，向指定手机号发送短信验证码。

### 请求

```
POST /api/v1/sms/send
Content-Type: application/json
```

**请求体：**

```json
{
  "phone": "13800001111",
  "captcha_id": "cap_1713520800123456789_42135",
  "captcha_code": "5829"
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `phone` | string | 是 | 手机号码 |
| `captcha_id` | string | 是 | 获取图形验证码接口返回的 `captcha_id` |
| `captcha_code` | string | 是 | 用户输入的图形验证码 |

### 响应

**状态码：** `200 OK`

```json
{
  "code": 0,
  "data": {
    "code": "385021"
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `code` | int | 业务状态码，`0` 表示成功 |
| `data.code` | string | 6 位短信验证码。**注意：生产环境中此字段不应返回给客户端，验证码应通过短信通道下发；当前为开发/测试环境便于调试直接返回** |

### 错误响应

| 状态码 | error 示例 | 原因 |
|--------|-----------|------|
| 400 | `"invalid phone payload"` | 请求体格式错误或 `phone` 为空 |
| 400 | `"captcha_id and captcha_code are required"` | 未传图形验证码信息 |
| 400 | `"invalid or expired captcha"` | 图形验证码错误或已过期 |

### 注意事项

- 短信验证码有效期为 **5 分钟**
- 同一手机号重复发送会覆盖旧验证码（以最新一次为准）
- 图形验证码一次性使用，发送失败后需重新获取图形验证码

---

## 3. 短信验证码登录

使用手机号和短信验证码完成登录。若用户不存在将自动注册。

### 请求

```
POST /api/v1/login/sms
Content-Type: application/json
```

**请求头（建议携带）：**

| Header | 说明 |
|--------|------|
| `X-Device-ID` | 设备唯一标识 |
| `X-Platform` | `ios` 或 `android` |

**请求体：**

```json
{
  "phone": "13800001111",
  "code": "385021"
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `phone` | string | 是 | 手机号码，需与发送验证码时一致 |
| `code` | string | 是 | 6 位短信验证码 |

### 响应

**状态码：** `200 OK`

```json
{
  "code": 0,
  "data": {
    "user_id": 7264917263840,
    "user": {
      "id": 7264917263840,
      "nickname": "",
      "avatar_url": "",
      "signature": "",
      "phone": "13800001111",
      "client_language": "",
      "created_at": "2026-04-19T10:30:00.000000Z",
      "updated_at": "2026-04-19T10:30:00.000000Z"
    }
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `code` | int | 业务状态码，`0` 表示成功 |
| `data.user_id` | int64 | 用户唯一 ID |
| `data.user` | object | 用户详情对象，结构见 [User 对象](#user-对象) |

### 错误响应

| 状态码 | error 示例 | 原因 |
|--------|-----------|------|
| 400 | `"phone and code are required"` | 缺少必填字段 |
| 401 | `"invalid or expired verification code"` | 验证码错误或已过期 |

---

## 4. 微信登录

使用微信授权 code 完成登录。服务端会调用微信 `jscode2session` 接口换取 OpenID，若用户不存在将自动注册。

### 请求

```
POST /api/v1/login/wechat
Content-Type: application/json
```

**请求头（建议携带）：**

| Header | 说明 |
|--------|------|
| `X-Device-ID` | 设备唯一标识 |
| `X-Platform` | `ios` 或 `android` |

**请求体：**

```json
{
  "code": "0b1Zpa000XXXXXXXXX"
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `code` | string | 是 | 微信客户端 SDK 授权获得的临时 code |

### 响应

**状态码：** `200 OK`

```json
{
  "user_id": 5839201746382,
  "user": {
    "id": 5839201746382,
    "nickname": "",
    "avatar_url": "",
    "signature": "",
    "phone": "",
    "client_language": "",
    "created_at": "2026-04-19T10:30:00.000000Z",
    "updated_at": "2026-04-19T10:30:00.000000Z"
  }
}
```

响应结构同短信登录，字段说明见 [User 对象](#user-对象)。

### 错误响应

| 状态码 | error 示例 | 原因 |
|--------|-----------|------|
| 400 | `"code is required"` | 未提供微信授权 code |
| 401 | `"wechat login failed: ..."` | 微信接口调用失败或返回错误 |

### 客户端接入说明

1. 调用微信 SDK 拉起授权（`wx.login()` 或原生 SDK）
2. 获取返回的临时 `code`
3. 将 `code` 传给本接口完成登录

---

## 5. Apple 登录

使用 Apple Sign In 的用户标识完成登录。若用户不存在将自动注册。

### 请求

```
POST /api/v1/login/apple
Content-Type: application/json
```

**请求头（建议携带）：**

| Header | 说明 |
|--------|------|
| `X-Device-ID` | 设备唯一标识 |
| `X-Platform` | `ios` 或 `android` |

**请求体：**

```json
{
  "apple_user_id": "001234.abcdef1234567890.1234",
  "identity_token": "eyJraWQiOiJXNldjT0..."
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `apple_user_id` | string | 是 | Apple 返回的用户唯一标识（`user` 字段） |
| `identity_token` | string | 否 | Apple 返回的 JWT identity token，用于服务端验证 |

### 响应

**状态码：** `200 OK`

```json
{
  "user_id": 4120398571629,
  "user": {
    "id": 4120398571629,
    "nickname": "",
    "avatar_url": "",
    "signature": "",
    "phone": "",
    "client_language": "",
    "created_at": "2026-04-19T10:30:00.000000Z",
    "updated_at": "2026-04-19T10:30:00.000000Z"
  }
}
```

响应结构同短信登录，字段说明见 [User 对象](#user-对象)。

### 错误响应

| 状态码 | error 示例 | 原因 |
|--------|-----------|------|
| 400 | `"apple_user_id is required"` | 未提供 Apple 用户标识 |
| 401 | `"..."` | 认证失败 |

### 客户端接入说明（iOS）

1. 使用 `ASAuthorizationAppleIDProvider` 发起 Apple 登录
2. 在回调中获取 `appleIDCredential.user`（即 `apple_user_id`）和 `identityToken`
3. 将两者传给本接口完成登录

---

## 6. 查询登录日志

查询指定用户的登录历史记录，按时间倒序排列，最多返回 20 条。

### 请求

```
GET /api/v1/login/log?user_id=7264917263840
```

**Query 参数：**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `user_id` | int64 | 是 | 用户 ID |

### 响应

**状态码：** `200 OK`

```json
[
  {
    "id": 1,
    "user_id": 7264917263840,
    "login_type": "sms",
    "ip": "223.104.3.56",
    "device_id": "A1B2C3D4-E5F6-7890-ABCD-EF1234567890",
    "platform": "ios",
    "created_at": "2026-04-19T10:30:00.000000Z"
  },
  {
    "id": 2,
    "user_id": 7264917263840,
    "login_type": "apple",
    "ip": "223.104.3.56",
    "device_id": "A1B2C3D4-E5F6-7890-ABCD-EF1234567890",
    "platform": "ios",
    "created_at": "2026-04-18T08:15:00.000000Z"
  }
]
```

**LoginLog 对象字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | int64 | 日志记录唯一 ID |
| `user_id` | int64 | 用户 ID |
| `login_type` | string | 登录方式：`sms` / `wechat` / `apple` |
| `ip` | string | 登录时的客户端 IP 地址（可能为空） |
| `device_id` | string | 客户端上报的设备标识（可能为空） |
| `platform` | string | 客户端平台：`ios` / `android`（可能为空） |
| `created_at` | string | 登录时间，ISO 8601 格式 |

### 错误响应

| 状态码 | error 示例 | 原因 |
|--------|-----------|------|
| 400 | `"user_id is required"` | 未提供 user_id |
| 400 | `"user_id must be int64"` | user_id 格式不正确 |

---

## 数据模型

### User 对象

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | int64 | 用户唯一 ID |
| `nickname` | string | 用户昵称，新注册用户为空字符串 |
| `avatar_url` | string | 头像 URL，新注册用户为空字符串 |
| `signature` | string | 个性签名，新注册用户为空字符串 |
| `phone` | string | 手机号码，短信登录用户会自动填充 |
| `client_language` | string | 客户端语言偏好 |
| `created_at` | string | 注册时间，ISO 8601 格式 |
| `updated_at` | string | 最后更新时间，ISO 8601 格式 |

---

## 附录：cURL 调用示例

### 获取图形验证码

```bash
curl -X GET http://localhost:8080/api/v1/captcha
```

### 发送短信验证码

```bash
curl -X POST http://localhost:8080/api/v1/sms/send \
  -H "Content-Type: application/json" \
  -d '{
    "phone": "13800001111",
    "captcha_id": "cap_1713520800123456789_42135",
    "captcha_code": "5829"
  }'
```

### 短信登录

```bash
curl -X POST http://localhost:8080/api/v1/login/sms \
  -H "Content-Type: application/json" \
  -H "X-Device-ID: device-001" \
  -H "X-Platform: ios" \
  -d '{
    "phone": "13800001111",
    "code": "385021"
  }'
```

### 微信登录

```bash
curl -X POST http://localhost:8080/api/v1/login/wechat \
  -H "Content-Type: application/json" \
  -H "X-Device-ID: device-001" \
  -H "X-Platform: android" \
  -d '{
    "code": "0b1Zpa000XXXXXXXXX"
  }'
```

### Apple 登录

```bash
curl -X POST http://localhost:8080/api/v1/login/apple \
  -H "Content-Type: application/json" \
  -H "X-Device-ID: device-001" \
  -H "X-Platform: ios" \
  -d '{
    "apple_user_id": "001234.abcdef1234567890.1234",
    "identity_token": "eyJraWQiOiJXNldjT0..."
  }'
```

### 查询登录日志

```bash
curl -X GET "http://localhost:8080/api/v1/login/log?user_id=7264917263840"
```
