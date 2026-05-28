# 同行文字弹幕客户端接入说明

> 适用于 `track_server` 的“与友同行”客户端研发。
>
> 本文是 [`emqx_client.md`](./emqx_client.md) 的补充，专门讲 **同行文字弹幕（danmaku）** 的客户端接入。
>
> 在阅读本文前，请先确认你已经按 `emqx_client.md` 完成了 MQTT 凭证申请、连接、订阅、上报位置等基础链路。本文 **复用同一条 MQTT 连接、同一组凭证**，不需要单独建立新的 broker 连接。

---

## 1. 总体原则

弹幕功能与位置同步使用 **同一条 MQTT 连接、同一组短期凭证**，但走独立的 topic：

1. **客户端只能 publish 自己的上行 topic**：`companion/{session_id}/member/{user_id}/danmaku`；
2. **客户端只能 subscribe 广播 topic**：`companion/{session_id}/danmaku`；
3. **客户端禁止 publish 广播 topic**（仅服务端可发，AuthZ 会返回 deny）；
4. **服务端确认 = 自己收到自己的广播**：客户端 publish 后启动 3s 计时器，在广播 topic 上收到 `user_id == 自己` 且 `content` 匹配的消息 → 视为成功；超时则视为失败；
5. **不要本地 echo**：发送方不要在 publish 完成后立刻把消息渲染到列表里，必须等服务端广播回来再渲染，否则会出现“假成功”体验；
6. **broker 不开 retained**：断线重连后只展示新到弹幕，历史不补发，历史弹幕请走业务接口（如有需要再扩展）。

相关接口与实现位置：

- MQTT 凭证返回（含 danmaku 两个 topic）：`track_api.md:1660`
- 弹幕 ingest 内部接口：`track_api.md:2054`
- topic 绑定生成逻辑：`internal/service/companion_service.go:777`
- 服务端 danmaku 业务逻辑：`internal/service/companion_service.go`（`PublishDanmaku` / `IngestDanmaku`）

---

## 2. 客户端与 App Server / EMQX 的职责边界

### 2.1 客户端负责

- 在弹幕输入框做基础校验（去首尾空白、长度 ≤ 200 个字符、空文本不允许发送）；
- 在已有的 MQTT 连接上 publish 上行 topic；
- 订阅广播 topic，按消息时序渲染到弹幕列表 / 弹幕滚屏区；
- 维护“发送成功 / 发送失败 / 超时”UI 状态；
- 对乱序、重复、自身回环消息做幂等处理。

### 2.2 App Server 负责

- 校验 `client_id / username` 与 `session_id / user_id` 是否匹配（principal 复核）；
- 校验内容长度（≤ 200 UTF-8 字符）与单成员限速（10 秒滚动窗口最多 5 条）；
- 落库（生成 `message_id`、`created_at`），并补全 `nickname / avatar_url`；
- 通过服务端 publisher 向广播 topic 发布最终弹幕消息。

### 2.3 EMQX 负责

- 转发上行消息到 App Server（Rule + HTTP Action）；
- 转发服务端发布的广播消息到所有订阅成员；
- 通过 HTTP AuthZ 限制客户端只能 publish 自己的上行 topic、不能 publish 广播 topic。

---

## 3. 推荐接入顺序

> 默认你已经走完 `emqx_client.md` 第 3 节的步骤，拿到了 MQTT 凭证、建好了 MQTT 连接、订阅了 `location_subscribe` 与 `control_subscribe`。

### 第 1 步：在凭证返回中取出 danmaku 两个 topic

`POST /api/v1/companion/session/:session_id/mqtt/credentials` 的返回中现在多了两个字段：

```json
{
  "topics": {
    "location_publish": "companion/sess_xxx/member/1001/location",
    "location_subscribe": "companion/sess_xxx/member/+/location",
    "presence_publish": "companion/sess_xxx/member/1001/presence",
    "presence_subscribe": "companion/sess_xxx/member/+/presence",
    "control_subscribe": "companion/sess_xxx/control",
    "danmaku_publish": "companion/sess_xxx/member/1001/danmaku",
    "danmaku_subscribe": "companion/sess_xxx/danmaku"
  }
}
```

- `danmaku_publish`：本人发送弹幕时使用的上行 topic；
- `danmaku_subscribe`：订阅同 session 的弹幕广播 topic。

**严禁硬编码这两个 topic，必须使用服务端返回值。**

### 第 2 步：连接成功后追加订阅 `danmaku_subscribe`

在已有 MQTT 连接上 subscribe：

- Topic：`topics.danmaku_subscribe`
- QoS：`1`

订阅成功后，客户端就能收到包括自己发送的弹幕在内的所有 session 弹幕广播。

### 第 3 步：发送弹幕

#### 3.1 客户端预校验（强烈建议）

在 publish 之前先在本地完成以下检查：

- 去除首尾空白字符；
- 文本不能为空；
- UTF-8 字符长度 ≤ 200（注意：是字符数，不是字节数）；
- 距离上一次发送间隔不要太短（建议本地节流 ≥ 1 秒，避免本地短时间堆积过多）。

服务端会对超长内容直接返回 `400 content exceeds 200 characters`，但客户端最好先拦截一次，提升用户体验。

#### 3.2 publish 上行消息

参数：

- Topic：`topics.danmaku_publish`
- QoS：`1`
- retained：`false`

推荐 payload：

```json
{
  "session_id": "sess_xxx",
  "user_id": 1001,
  "content": "加油！"
}
```

字段说明：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `session_id` | string | 是 | 当前 session ID，与 topic 第 2 段一致 |
| `user_id` | int64 | 是 | 当前登录用户 ID，与 topic 第 4 段一致 |
| `content` | string | 是 | 弹幕文本，UTF-8 长度 ≤ 200 |

> 服务端 Rule SQL 默认从 topic 抽取 `session_id / user_id`，因此即使 payload 没带这两个字段也不会出错；但建议客户端始终带上，便于排查。

#### 3.3 启动 3s 失败检测计时器

publish 成功（SDK 回调 publish ack）只表示消息进入 broker，**不代表服务端落库 / 广播成功**。

客户端必须为每条本地发送的弹幕维护一个 pending 记录，并启动一个 **3 秒** 的超时计时器，等待自己的广播回环：

```text
publish ok
  -> 记录 pending: { local_id, content, sent_at }
  -> 启动 3s 计时器
      -> 期间在 danmaku_subscribe 收到匹配消息：
            清理 pending，UI 显示“已发送”
      -> 超时仍未收到匹配消息：
            清理 pending，UI 显示“发送失败，点击重试”
```

匹配规则建议：

- `user_id == 当前登录 user_id`；
- 且 `content == 本次发送的 content`；
- 如果同一秒内连发多条相同文本，建议本地维护一个 FIFO pending 队列，按先进先出消费。

### 第 4 步：处理收到的广播消息

订阅 `danmaku_subscribe` 后，会按时序收到下面格式的 JSON：

```json
{
  "message_id": 12345,
  "session_id": "sess_xxx",
  "user_id": 1001,
  "nickname": "Tom",
  "avatar_url": "/api/v1/static/avatars/1001.png",
  "content": "加油！",
  "created_at": "2026-05-23T18:10:00Z"
}
```

字段含义：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `message_id` | int64 | 服务端自增 ID，**用于客户端幂等去重** |
| `session_id` | string | 同行会话 ID，可与本地 session 比对 |
| `user_id` | int64 | 发送者 user_id |
| `nickname` | string | 发送者昵称（已经服务端补齐） |
| `avatar_url` | string | 发送者头像 URL（按现有头像缓存策略改写） |
| `content` | string | 弹幕文本 |
| `created_at` | string(datetime) | 服务端落库时间，RFC3339 |

客户端处理建议：

1. 用 `message_id` 做去重：维护一个最近 N 条 `message_id` 的环形缓冲，重复直接丢弃；
2. 如果 `user_id == 自己`，**优先用于完成 pending 匹配**（见第 3.3 步），完成匹配后再渲染到列表；
3. 头像 / 昵称直接用广播里的字段渲染，不要再去拉用户资料接口；
4. 没有“在线滚动 + 历史回查”的设计，UI 上只展示连接期间收到的消息。

### 第 5 步：退出同行页面 / 切换 session

不需要单独处理弹幕：

- 退出同行页面时按 `emqx_client.md` 第 9 步走 HTTP 控制面 + 断 MQTT，弹幕订阅会随连接断开一起释放；
- 切换 session 时也需要重新申请凭证、重新订阅，包括 `danmaku_subscribe`。

---

## 4. 错误处理

### 4.1 publish 被 broker 拒绝（AuthZ deny）

最常见原因：

- 误用了 `companion/{session_id}/danmaku`（广播 topic）作为发送 topic；
- 误用了别的成员的 `danmaku_publish`；
- 凭证已过期或被同账号另一台设备的新凭证覆盖。

处理：

- 严格使用服务端返回的 `topics.danmaku_publish`；
- 若发现凭证被覆盖，重新拉取 credentials 并重连（流程同 `emqx_client.md` 第 4.2 节）。

### 4.2 服务端返回的错误状态码（间接表现为客户端超时）

EMQX → App Server 的 ingest 是后端链路，**客户端不会直接看到 HTTP 状态码**，只会表现为“3 秒内没收到自己的广播”。常见原因：

- `content` 超过 200 字符 → `400 content exceeds 200 characters`；
- 单成员 10 秒内发了超过 5 条 → `400 danmaku rate limit exceeded`；
- 上行 payload 与 principal（client_id / username）不匹配 → `403`；
- session 已结束 → `404`。

客户端策略：

- 不要尝试解析 ingest HTTP 错误，直接以 “3 秒未收到回环 = 失败” 作为唯一判定依据；
- UI 上的失败原因可以走经验文案：超长（本地校验已拦截）、发送过快、网络异常；
- 失败后允许用户点击重试；连续多次失败时建议禁用输入框 5~10 秒。

### 4.3 收到了别人的弹幕，但收不到自己的

优先检查：

- 是否真的订阅了广播 topic `topics.danmaku_subscribe`；
- 是否在 publish 之前就开始监听（**先订阅再发送**，否则可能错过自己的回环）；
- 服务端是否记录了限流 / principal 错误（看 App Server 日志）。

### 4.4 同一条弹幕收到了两次

由 QoS 1 + 重连导致的重复在 MQTT 中是正常现象。客户端必须用 `message_id` 做幂等去重，**不要用 `(user_id, content)` 去重**，否则会把短时间内重复发送的相同文字误判为重复。

---

## 5. 客户端最小联调清单

### 5.1 接入验证

- [ ] 凭证返回里能拿到 `danmaku_publish` 与 `danmaku_subscribe`
- [ ] 连接成功后能 subscribe `danmaku_subscribe`
- [ ] 客户端能在 `danmaku_publish` 上 publish 成功

### 5.2 业务联调验证

- [ ] A 设备发弹幕，B 设备能实时收到
- [ ] A 设备发弹幕，A 设备自己也能在 3 秒内收到（Plan A 成功路径）
- [ ] A 设备发送超长（> 200 字符）弹幕：本地拦截或 3 秒后超时显示失败
- [ ] A 设备 11 秒内连发 6 条：第 6 条超时显示失败
- [ ] A 设备 publish 广播 topic `companion/{sid}/danmaku` 被 broker 拒绝
- [ ] 收到 `session_ended` 后，弹幕订阅能随会话退出一起释放

---

## 6. UI / 交互建议

- **发送态机**：`idle` → `sending`（等待回环）→ `sent` / `failed`；`failed` 提供“点击重试”入口；
- **本地节流**：发送按钮在 `sending` 状态置灰；UI 层另加 1 秒最小间隔；
- **滚屏 / 列表去重**：以 `message_id` 为唯一键，保留最近 N 条（建议 N = 200）即可；
- **避免本地 echo**：千万不要在 publish ack 里就把消息渲染上屏，否则在“服务端限流 / 鉴权失败”场景会出现“界面看到了，其他人却没收到”的假象；
- **失败提示**：建议统一文案“发送失败，请稍后再试”，不要把后端错误细节透出；
- **断线提示**：MQTT 重连期间禁用发送按钮，重连成功后恢复。

---

## 7. 给客户端研发的最终约束

请严格遵循以下约束：

1. **复用 `emqx_client.md` 中已建好的 MQTT 连接，不要为弹幕单独建连**；
2. **topic 必须使用服务端返回的 `danmaku_publish` / `danmaku_subscribe`，禁止硬编码**；
3. **禁止 publish 广播 topic `companion/{session_id}/danmaku`**（仅服务端可发）；
4. **publish 后必须等 3 秒回环匹配，匹配成功才渲染到列表**；
5. **必须用 `message_id` 做幂等去重**；
6. **本地校验空文本与 200 字符上限**；
7. **断线重连不要尝试补拉历史弹幕**（服务端不维护、不补发）。

---

## 8. 参考文件

- 同行客户端基础接入：`emqx_client.md`
- MQTT 凭证接口（含 danmaku topic）：`track_api.md:1660`
- 弹幕 ingest 内部接口：`track_api.md:2054`
- 同行整体方案 + danmaku 设计章节：`track_companion.md`
- EMQX 服务端部署与 Dashboard 配置（含弹幕 Rule）：`emqx.md`
