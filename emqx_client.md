# EMQX 客户端接入说明

> 适用于 `track_server` 的“与友同行”客户端研发。
>
> 本文面向 iOS / Android / HarmonyOS / Flutter / React Native / UniApp / H5 等客户端实现，重点说明：**客户端如何通过 App Server 获取短期 MQTT 凭证，并正确连接 EMQX 完成位置同步与控制消息处理。**

---

## 1. 总体原则

客户端接入“与友同行”时，必须遵循以下原则：

1. **先走 App Server 控制面，再连 EMQX 数据面**；
2. **不要在客户端硬编码 EMQX 用户名/密码**；
3. **不要在客户端硬编码 topic**，应优先使用服务端返回的 topic bindings；
4. **新成员进入同行页面时，先拉 HTTP snapshot，再建立 MQTT 订阅**；
5. **MQTT 凭证是短期凭证**，过期或被新的凭证覆盖后，需要重新向 App Server 申请；
6. **收到 `session_ended` 后，客户端应立即停止上报位置并退出当前同行会话。**

相关接口与实现位置：

- 获取当前同行会话：`track_api.md:1549`
- 获取会话 snapshot：`track_api.md:1569`
- 获取 MQTT 凭证：`track_api.md:1660`
- 路由注册：`internal/handler/router.go:125`
- MQTT 凭证返回结构：`internal/service/companion_service.go:168`
- topic 绑定生成逻辑：`internal/service/companion_service.go:777`

---

## 2. 客户端与 App Server / EMQX 的职责边界

### 2.1 App Server 负责

- 创建 / 加入 / 离开 / 结束同行 session；
- 返回当前 session 的 snapshot；
- 为客户端签发短期 MQTT 凭证；
- 通过 EMQX 的 HTTP AuthN / AuthZ 校验客户端 MQTT 权限；
- 接收 EMQX 回写的位置 / presence 数据并更新服务端快照；
- 主动向 control topic 发布 `member_left` / `session_ended`。

### 2.2 EMQX 负责

- 实时转发位置消息；
- 转发 control 消息；
- 通过 HTTP 回调向 App Server 做动态鉴权；
- 通过 Rule Engine / Connector 把位置与 presence 写回 App Server。

### 2.3 客户端负责

- 在进入同行页面前，先完成登录与 session 获取；
- 拉取 snapshot 作为地图初始状态；
- 请求短期 MQTT 凭证；
- 建立 MQTT 连接并订阅服务端允许的 topic；
- 按约定 topic 上报本人的位置；
- 正确处理 `member_left` / `session_ended` 控制消息；
- 在离开页面、账号切换、凭证过期时主动断开 MQTT。

---

## 3. 推荐接入顺序

客户端进入“同行”页面后，推荐严格按下面顺序执行。

### 第 1 步：通过 HTTP 获取当前 session

有两种常见入口：

1. 创建同行：`POST /api/v1/companion/session/create`
2. 加入同行：`POST /api/v1/companion/session/join`

如果客户端是“恢复进入同行页面”，可直接调：

- `GET /api/v1/companion/session/current`：`track_api.md:1549`

拿到 `session_id` 后，客户端即可认为“控制面已就绪”。

### 第 2 步：获取 snapshot 作为初始地图状态

可用以下两种方式之一：

- 创建 / 加入接口本身返回的 `snapshot`
- 或单独调用 `GET /api/v1/companion/session/:session_id/snapshot`：`track_api.md:1569`

**这一阶段先渲染地图成员和最新位置，不要等 MQTT 首条消息。**

### 第 3 步：向 App Server 请求 MQTT 短期凭证

接口：

- `POST /api/v1/companion/session/:session_id/mqtt/credentials`：`track_api.md:1669`

返回示例：

```json
{
  "code": 0,
  "data": {
    "session_id": "sess_xxx",
    "broker_url": "mqtt://172.30.212.160:1883",
    "websocket_url": "ws://172.30.212.160:8083/mqtt",
    "client_id": "cmp-sess_xxx-1001-abc123",
    "username": "cmpv1:sess_xxx:1001:1770000000:abc123",
    "password": "signed_password",
    "expires_at": "2026-05-25T18:00:00Z",
    "topics": {
      "location_publish": "companion/sess_xxx/member/1001/location",
      "location_subscribe": "companion/sess_xxx/member/+/location",
      "presence_publish": "companion/sess_xxx/member/1001/presence",
      "presence_subscribe": "companion/sess_xxx/member/+/presence",
      "control_subscribe": "companion/sess_xxx/control"
    }
  }
}
```

字段定义对应：`track_api.md:1675`、`internal/service/companion_service.go:343`

### 第 4 步：选择连接协议

建议：

- **原生 App（iOS / Android / HarmonyOS）优先使用 `broker_url`**；
- **H5 / 小程序 / 某些跨端框架优先使用 `websocket_url`**；
- 如果当前平台不支持 TCP MQTT，则退回 WebSocket MQTT；
- 如果返回的某个 URL 为空，说明服务端没有对外暴露该协议，客户端应使用另一个非空地址。

### 第 5 步：建立 MQTT 连接

连接参数全部使用服务端返回值：

- `client_id`
- `username`
- `password`
- `broker_url` 或 `websocket_url`

**不要自己拼 username/password，不要复用上一次缓存的凭证。**

### 第 6 步：连接成功后立即订阅 topic

至少订阅：

- `topics.location_subscribe`
- `topics.control_subscribe`

可选订阅：

- `topics.presence_subscribe`

说明：

- 当前服务端鉴权允许订阅整个 session 的 location / presence 通配 topic，以及 control topic：`internal/service/companion_service.go:383`
- `location_subscribe` 会收到当前 session 全部成员的位置消息，客户端要按 `user_id` 聚合；
- 如果收到自己的位置回环消息，可用 `user_id + seq` 做幂等去重。

### 第 7 步：开始上报位置

客户端向 `topics.location_publish` 发布消息。

推荐 payload：

```json
{
  "session_id": "sess_xxx",
  "user_id": 1001,
  "track_id": "NO.00000001",
  "latitude": 30.123,
  "longitude": 120.456,
  "coordinate_system": "GCJ02",
  "speed_kmh": 5.2,
  "heading": 90,
  "accuracy_m": 8,
  "altitude": 100,
  "recorded_at": "2026-05-25T10:00:00Z",
  "seq": 1024
}
```

推荐使用：

- Topic：`companion/{session_id}/member/{user_id}/location`
- QoS：`0` 或 `1`
- retained：`false`

建议：

- 高频位置上报优先考虑 `QoS 0`，减少堆积；
- 如果业务更重视可靠性，可以切到 `QoS 1`；
- `seq` 应单调递增，用于去重与丢弃旧消息；
- `recorded_at` 使用 UTC ISO8601 时间字符串。

### 第 8 步：接收并处理 control 消息

客户端必须订阅：

- `companion/{session_id}/control`

服务端会发布两类消息：

#### 8.1 成员离开

```json
{
  "event": "member_left",
  "session_id": "sess_xxx",
  "member_user_id": 1002,
  "operator_user_id": 1002,
  "reason": "member_left",
  "at": "2026-05-25T18:00:00Z"
}
```

来源：`internal/service/companion_service.go:627`

客户端处理建议：

- 从成员列表中移除 `member_user_id`；
- 从地图上移除该成员当前位置；
- 如果本地正展示该成员头像气泡，也同步移除。

#### 8.2 会话结束

```json
{
  "event": "session_ended",
  "session_id": "sess_xxx",
  "operator_user_id": 1001,
  "reason": "owner_ended",
  "at": "2026-05-25T18:05:00Z"
}
```

或：

```json
{
  "event": "session_ended",
  "session_id": "sess_xxx",
  "operator_user_id": 0,
  "reason": "all_members_left",
  "at": "2026-05-25T18:06:00Z"
}
```

来源：`internal/service/companion_service.go:997`

客户端处理建议：

- 立即停止位置上报；
- 取消所有同行 topic 订阅；
- 主动断开 MQTT；
- 退出同行页面并提示“同行已结束”；
- 如果收到滞后的位置消息，与 `session_ended` 冲突时，以 `session_ended` 为准。

### 第 9 步：退出同行页面时走 HTTP 控制面接口

不要只断 MQTT，不走业务接口。

成员主动离开：

- `POST /api/v1/companion/session/:session_id/leave`

owner 主动结束：

- `POST /api/v1/companion/session/:session_id/end`

然后客户端再：

1. 停止位置采集；
2. 取消订阅；
3. 断开 MQTT；
4. 清理本地同行状态。

---

## 4. 连接与重连策略

### 4.1 短期凭证的使用规则

MQTT 凭证具有两个重要特性：

1. **有过期时间**：`expires_at`
2. **同一成员同一时刻仅最后一次签发的凭证有效**

第二点来自校验逻辑：服务端会把最近一次签发的 `mqtt_principal + mqtt_client_id` 记录到 member 上，后续鉴权时要求完全一致：`internal/service/companion_service.go:812`

因此客户端应遵循：

- 每次进入同行页面都重新请求一次 MQTT credentials；
- 不要长期缓存历史凭证；
- 如果同一账号在另一台设备重新申请了凭证，旧连接后续 publish / subscribe 可能被拒绝；
- 凭证即将过期时，应主动重新拉取 credentials 并重连。

### 4.2 推荐重连机制

建议：

1. 网络闪断时，先尝试 SDK 自带自动重连；
2. 如果重连失败，或服务端返回 auth/acl deny，则重新拉取 MQTT 凭证；
3. 如果发现当前 session 已结束，则不再重连，直接退出同行页面。

可按下面顺序处理：

```text
MQTT 断开
  -> 先做 1~2 次短重连
  -> 若失败 / 被拒绝 / 凭证过期
      -> GET current session 或拉 snapshot 校验 session 是否仍 active
      -> POST mqtt/credentials 重新取凭证
      -> 用新凭证重连并重新订阅
```

### 4.3 是否需要主动发布 presence

当前推荐方案中，presence 主要由 EMQX 的 `client connected / client disconnected` 事件回写 App Server。

因此：

- **v1 客户端可以不主动 publish presence topic**；
- 如果未来业务有“前后台切换 / 手动隐身 / 心跳态”的增强需求，再扩展 `presence_publish` 的使用。

---

## 5. 客户端最小联调清单

建议按下面顺序验证。

### 5.1 控制面验证

- [ ] 创建同行成功，可拿到 `session_id`
- [ ] 加入同行成功，可拿到 `snapshot`
- [ ] `GET /companion/session/current` 能返回当前 active session
- [ ] `POST /companion/session/:session_id/mqtt/credentials` 可返回 MQTT 凭证

### 5.2 MQTT 建连验证

- [ ] 客户端使用返回的 `broker_url` 或 `websocket_url` 建连成功
- [ ] 客户端 subscribe `location_subscribe` 成功
- [ ] 客户端 subscribe `control_subscribe` 成功
- [ ] 客户端 publish `location_publish` 不被拒绝

### 5.3 数据联调验证

- [ ] A 设备发位置后，B 设备实时收到
- [ ] 新成员进入后，先看到 snapshot，再继续收到后续 MQTT 位置流
- [ ] 成员离开时，其他成员能收到 `member_left`
- [ ] owner 结束时，所有成员能收到 `session_ended`
- [ ] 收到 `session_ended` 后客户端能停止上报并退出页面

---

## 6. 常见错误与排查

### 6.1 HTTP 能拿到 session，但拿不到 MQTT 凭证

常见原因：

- App Server 未配置 `COMPANION_MQTT_CREDENTIAL_SECRET`
- 当前用户不是该 session 的 joined 成员

相关错误：`track_api.md:1697`

### 6.2 MQTT CONNECT 失败

优先检查：

- 是否使用了服务端刚返回的 `client_id / username / password`
- `username` 是否被客户端 SDK 截断或转义
- `broker_url` / `websocket_url` 是否与当前平台协议兼容
- 凭证是否已过期

### 6.3 CONNECT 成功，但 publish / subscribe 被拒绝

优先检查：

- 是否误用了硬编码 topic
- 发布 topic 是否是自己的 `location_publish`
- 是否订阅了未授权的 topic
- 是否在另一个设备重新申请了新凭证，导致旧 `client_id / principal` 失效

鉴权逻辑见：`internal/service/companion_service.go:369`

### 6.4 新成员看不到历史位置

这是客户端接入顺序错误。

正确做法：

1. 先拿 HTTP snapshot；
2. 再连接 MQTT；
3. 再用实时消息增量更新地图。

### 6.5 收到 `session_ended` 后页面还在继续上报位置

这是客户端状态机没有收敛。

收到 `session_ended` 后必须立即：

1. 停止定位上报；
2. 取消订阅；
3. 断开 MQTT；
4. 清空本地同行状态。

---

## 7. 建议的客户端页面状态机

推荐把同行页面分成以下状态：

- `idle`：未进入同行
- `loading_snapshot`：控制面加载中
- `connecting_mqtt`：申请凭证并建连中
- `active`：控制面 + 数据面均正常
- `reconnecting`：网络抖动后的短时重连
- `ended`：收到 `session_ended` 或 HTTP 控制面确认 session 已结束

推荐状态流转：

```text
idle
  -> loading_snapshot
  -> connecting_mqtt
  -> active
  -> reconnecting
  -> active

active
  -> ended
```

不要把“是否有 MQTT 连接”直接等同于“会话是否 active”，会话状态应以 App Server 控制面与 control topic 综合判断。

---

## 8. 给客户端研发的最终约束

请严格遵循以下约束：

1. **不要硬编码 EMQX 地址、用户名、密码、topic**；
2. **必须先拉 snapshot，再订阅 MQTT**；
3. **必须订阅 `control_subscribe`**；
4. **收到 `session_ended` 必须立即退出同行链路**；
5. **每次进入同行页面重新申请 MQTT 凭证**；
6. **同一账号多端并发时，以最后一次签发的凭证为准。**

---

## 9. 参考文件

- 客户端业务接口：`track_api.md:1450`
- MQTT 凭证接口：`track_api.md:1660`
- 同行整体方案：`track_companion.md:1`
- EMQX 服务端部署与 Dashboard 配置：`emqx.md:1`
- 服务端路由：`internal/handler/router.go:125`
- 服务端 MQTT topic / ACL 逻辑：`internal/service/companion_service.go:777`

