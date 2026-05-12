# AGENTS.md — internal/admin

> 本文面向 AI Coding Agent。仅记录 admin 模块的稳定约束、改动导航与提交前最小检查。
> 通用约束、目录总览、降级链等见仓库根目录 `AGENTS.md`，本文不重复。

## 模块定位

`internal/admin` 是 **track_server 的内置管理后台**，提供：

- 一个独立登录的 Web 控制台（HTML/CSS/JS 经 `go:embed` 内置）；
- 围绕 `AppRelease` 的发布、列表、删除接口；
- 上传安装包到服务端本地静态目录的接口。

模块对外只暴露 `admin.NewModule(...)` 与 `Module.RegisterRoutes(h)`，不被业务 handler 引用。

## 硬性约束

- **独立鉴权链路**：admin 走自有的 `Authenticator` + `SessionStore`（cookie 名 `admin_session`），**不要复用** `internal/middleware.JWTAuthMiddleware` 也不要在 admin 路由里塞 `RequestMeta` 中间件。业务用户与管理员是两套体系。
- **凭证来源唯一**：管理员账号只来自环境变量。支持两种格式：
  - **推荐**：`ADMIN_ACCOUNTS=user1:bcryptHash1;user2:bcryptHash2`（多账号，`;` / `,` 分隔，首个 `:` 分割 username 与 hash）。
  - **兼容**：旧单账号 `ADMIN_USERNAME` / `ADMIN_PASSWORD_HASH`。两者同时存在时合并，同名用户以 `ADMIN_ACCOUNTS` 为准。
  - **禁止把管理员信息写入数据库**，避免和业务用户表交叉。
- **未配置即禁用**：上面两种来源解析出的账号集为空时，`NewModule` 返回的 `Module.Auth` 为 `nil`，`RegisterRoutes` 直接不挂载任何 `/admin/*` 路由——这是"后台未启用"的唯一信号，新增功能不要绕过这个开关。
- **会话仅内存**：`SessionStore` 使用 `sync.RWMutex` + `map`，进程重启即失效。**不要替换为持久化存储**；管理员重新登录是合理代价。
- **静态资源经 `go:embed`**：`static/` 目录通过 `//go:embed static` 内置进二进制；新增前端文件后**必须放到** `internal/admin/static/`，否则二进制找不到。`/admin/static/*filepath` 路由只读 `staticFS`，不读磁盘。
- **安装包本机上传**：管理后台安装包上传走 `POST /admin/api/releases/upload-package`，服务端落盘到 `<LogDir>/static/release/<platform>/`，前端把返回的 `/api/v1/static/release/<platform>/<file>` 填入 `package_url`。不要再把发布包上传链路改回浏览器 OSS 直传。
- **路径前缀固定 `/admin`**：所有路由都注册在 `h.Group("/admin")` 之下；前端 `fetch` 也写死 `/admin/api/...` 与 `/admin/static/...`。修改前缀需要同时改 `routes.go` 与 `static/*` 中的 URL。

## 目录与权威文件

```
internal/admin/
├── auth.go         # Authenticator + bcrypt 校验 + cookie + AuthMiddleware
├── session.go      # SessionStore (内存 + GC 协程，TTL 默认 12h)
├── handlers.go     # /admin/api/releases* + /admin/api/releases/upload-package
├── routes.go       # NewModule / RegisterRoutes / go:embed static
└── static/
    ├── login.html  ├── login.js
    ├── index.html  ├── app.js
    └── style.css
```

| 主题 | 权威文件 |
| --- | --- |
| 路由清单 | `routes.go` 中 `RegisterRoutes` |
| 鉴权与 cookie | `auth.go`（`sessionCookieName = "admin_session"`） |
| 业务依赖注入 | `routes.go` 的 `NewModule(accounts, releaseSvc, stsSvc, staticRoot)` |
| 前端入口 | `static/login.html`、`static/index.html` |
| 安装包上传 | `handlers.go` 中 `UploadPackage`、`static/app.js` 中 `/admin/api/releases/upload-package` |

## 路由清单（与 `routes.go` 对齐）

| 方法 | 路径 | 鉴权 | 说明 |
|------|------|------|------|
| GET | `/admin/`、`/admin` | 公开 | 302 跳转 `/admin/login.html` |
| GET | `/admin/login.html` | 公开 | 登录页 |
| GET | `/admin/index.html` | 公开 | 管理首页（页面内会拉 `/admin/api/me` 校验） |
| GET | `/admin/static/*filepath` | 公开 | 内置静态资源 |
| POST | `/admin/api/login` | 公开 | bcrypt 校验，成功后下发 `admin_session` cookie |
| POST | `/admin/api/logout` | 公开 | 删除 session + 清 cookie |
| GET | `/admin/api/me` | 公开 | 返回当前会话；未登录返回 401 |
| GET | `/admin/api/releases` | 鉴权 | 列出发布记录，支持 `platform` / `status` 过滤 |
| POST | `/admin/api/releases` | 鉴权 | 发布/覆盖一个版本（按 `(platform, version_code)` 幂等） |
| DELETE | `/admin/api/releases/:id` | 鉴权 | 删除一条发布记录 |
| POST | `/admin/api/releases/upload-package` | 鉴权 | multipart 上传安装包到 `<LogDir>/static/release/<platform>/` |
| GET | `/admin/api/releases/upload-token` | 鉴权 | 旧 OSS 直传 STS 凭证接口，前端不再使用 |

## 关键流程

**登录**：
```
POST /admin/api/login {username,password}
  → Authenticator.Verify (constant-time username + bcrypt password)
  → SessionStore.Create → Set-Cookie admin_session=<32B hex>; HttpOnly; SameSite=Lax
```

**前端发布 APK（Android）**：
```
[admin UI] 选 APK
  → POST /admin/api/releases/upload-package multipart(file, platform=android) (鉴权)
  → 服务端写入 <LogDir>/static/release/android/<ts>-<safe-file>.apk
  → 返回 /api/v1/static/release/android/<ts>-<safe-file>.apk 并填进 package_url
  → POST /admin/api/releases   (鉴权)
       → AppReleaseService.Publish → AppReleaseRepository.Upsert
```

**鉴权拦截**：
```
请求 → Authenticator.AuthMiddleware → SessionFromRequest(cookie) → 否则 401
```

## 改动导航

- **加一个管理后台 API**：在 `handlers.go` 写 handler → `routes.go` 的 `api := g.Group("/api", m.Auth.AuthMiddleware())` 下挂载。需要操作者信息时用 `h.auth.SessionFromRequest(c)`。
- **加一个公开页面/资源**：把文件放进 `static/`，在 `routes.go` 用 `serveEmbedded(...)` 或走 `/admin/static/*filepath`。**不要新增第二个 `go:embed`**。
- **改 cookie 策略**：只在 `auth.go` 改；保持 `HttpOnly=true`，跨站调用前提下才考虑 `SameSite=None + Secure`。
- **调整 session TTL**：改 `routes.go` 中 `NewSessionStore(12 * time.Hour)`，不要在 cookie 与 store 两边写不一致的过期时间。

## 提交前最小检查

- 构建：`go build ./...`
- 测试：`go test ./...`（admin 模块当前暂无单测，至少保证业务包不被破坏）
- 路由变更：核对 `routes.go` 与 `static/*.js` 中前端 fetch 的 URL 是否一致。
- 静态资源新增：确认文件位于 `internal/admin/static/` 下（`go:embed static` 才能打包）。
- 接口契约变更：同步更新仓库根目录 `track_api.md`（如涉及客户端能感知的接口）。

## 常见误解

- **误解**：管理后台没访问到，往业务 `auth` 组里塞路由。
  **事实**：admin 完全独立挂在 `/admin`，不属于 `/api/v1/auth` 组；登录态也是 cookie，不是 JWT。
- **误解**：可以用 `OSSTokenService.GetUploadCredential` 上传 APK。
  **事实**：那是按 userID 隔离的用户目录，policy 不允许写入 `release/`，必须用 `GetReleaseUploadCredential`。
- **误解**：进程重启后会话还在。
  **事实**：`SessionStore` 是纯内存实现，重启即清空，必须重新登录。
- **误解**：管理员账号在数据库里。
  **事实**：账号只来自 `ADMIN_USERNAME` / `ADMIN_PASSWORD_HASH` 环境变量，bcrypt 校验，不查任何表。
