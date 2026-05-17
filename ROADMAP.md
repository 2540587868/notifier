# Roadmap: Notifier

> 最后更新: 2026-05-17 | 版本: v0.1.0

## 项目现状

- 代码文件: 16 个 Go 源文件 (含 cmd 入口)
- 测试覆盖率: 0% (无任何 `_test.go` 文件)
- 已实现渠道: 企业微信机器人、钉钉机器人(HMAC签名)、Email(SMTP/TLS)、Telegram Bot、Webhook、Qmsg(QQ)、Server酱、PushPlus、NapCatQQ(OneBot11)
- 未实现渠道: WxPusher
- 已知问题: 0 (P0/P1 全部修复)
- 技术债: 静默忽略错误已修复；0 测试、store 重复 scan 代码、缺少配置校验
- 基础设施: Dockerfile ✅ | CI ✅ | Makefile ✅ | golangci-lint ✅ | 云服务器部署 ✅ | Email推送 ✅

## 🔴 P0 — 紧急（立即处理）

| # | 类别 | 描述 | 影响 | 工作量 | 关联文件 |
|---|------|------|------|--------|----------|
| 1 | 🐛 bug ✅ | ~~SIGHUP 热加载后新 Registry 被丢弃~~ → 已修复：添加 `ReplaceAll` 方法替换内部 channels map | 热加载后所有渠道仍使用旧实例，发送行为不变，生产环境严重隐患 | <1h | [main.go:169](file:///d:/ccswitch/notifier/cmd/notifier/main.go#L169) |
| 2 | 🔒 security ✅ | ~~Admin Token 默认值 "changeme"~~ → 已修复：移除默认值，启动时强制校验非空且非默认值 | 默认密码可被猜测，管理 API 完全暴露 | <1h | [config.go:104](file:///d:/ccswitch/notifier/internal/config/config.go#L104) |
| 3 | 🐛 bug ✅ | ~~通知记录状态永不更新~~ → 已修复：添加 `UpdateNotificationStatus`，队列 handler 发送后回写 "sent"/"failed"；修复多渠道共享 ID 导致只插入首条记录的问题 | 无法通过 API 查询通知是否真正送达，监控统计完全失真 | 1-4h | [router.go:179](file:///d:/ccswitch/notifier/internal/api/router.go#L179), [main.go:90-110](file:///d:/ccswitch/notifier/cmd/notifier/main.go#L90) |
| 4 | 🔒 security ✅ | ~~Notify API 认证使用 admin_token~~ → 已修复：`/api/v1/notify` 使用 api_tokens 表认证，管理 API 保留 admin_token 认证 | 违反最小权限原则，服务间共享 admin_token 等于共享 root 权限 | 4h | [router.go:513-527](file:///d:/ccswitch/notifier/internal/api/router.go#L513) |

## 🟠 P1 — 高优先级（本版本）

| # | 类别 | 描述 | 影响 | 工作量 | 关联文件 |
|---|------|------|------|--------|----------|
| 5 | ✨ feature ✅ | ~~新增 Server酱 渠道~~ → 已实现：`serverchan.go` + 模板 + 工厂注册 | 个人用户无法收到微信通知，核心需求缺失 | 1-4h | `internal/channel/serverchan.go` (新建) |
| 6 | ✨ feature ✅ | ~~新增 PushPlus 渠道~~ → 已实现：`pushplus.go` + HTML模板 + 工厂注册 | PushPlus 免费额度 200 条/天，是 Server酱 的最佳补充 | 1-4h | `internal/channel/pushplus.go` (新建) |
| 7 | 🔧 techdebt ✅ | ~~修复静默忽略错误~~ → 已修复：`json.Unmarshal` 错误检查 + `rand.Read` 返回值检查 | 隐藏的数据损坏和弱 ID 风险 | <1h | [store.go:306,322,338](file:///d:/ccswitch/notifier/internal/store/store.go#L306), [router.go:556,562](file:///d:/ccswitch/notifier/internal/api/router.go#L556) |

## 🟡 P2 — 中优先级（下个版本）

| # | 类别 | 描述 | 影响 | 工作量 | 关联文件 |
|---|------|------|------|--------|----------|
| 8 | ✨ feature | 新增 WxPusher 渠道 — 微信+App 双通道推送 | WxPusher 有独立 App，推送到达率更高，免费 2000 条/天/UID | 1-4h | `internal/channel/wxpusher.go` (新建) |
| 9 | ✨ feature | 渠道熔断器 — 渠道连续失败 N 次后自动熔断，超时后半开探测 | 外部 API 不可用时浪费重试资源，拖慢队列消费速度 | 半天 | `internal/channel/circuitbreaker.go` (新建) |
| 10 | 📊 observability | 健康检查增强 — 检查 DB 连通性 + 队列积压量 | 当前 /health 只返回 {"status":"ok"}，DB 挂了也显示健康 | 1-4h | [router.go:83-85](file:///d:/ccswitch/notifier/internal/api/router.go#L83) |
| 11 | 🔧 techdebt | 配置加载校验 — 启动时校验必要字段 (listen 地址、duration 格式、渠道配置完整性) | 非法配置导致运行时才报错，启动失败原因不明确 | 1-4h | [config.go:130-142](file:///d:/ccswitch/notifier/internal/config/config.go#L130) |
| 12 | 📊 observability | Prometheus 自定义指标 — 队列深度/发送延迟/渠道成功率/熔断状态/丢弃计数 | /metrics 仅暴露 Go 默认运行时指标，无法监控通知系统自身健康 | 半天 | [router.go:116](file:///d:/ccswitch/notifier/internal/api/router.go#L116) |
| 13 | 🔧 techdebt | 补充核心模块单元测试 (channel/router/ratelimit/dedup/silence/queue/retrier) | 0 测试覆盖率，任何改动都可能引入回归，重构无安全网 | 2-3天 | `internal/**/*_test.go` (新建) |
| 14 | 🔧 techdebt | 消除 store.go 重复 scan 代码 — `scanChannel`/`scanChannelFromRows` 等函数几乎完全相同 | 违反 DRY 原则，新增字段需改 4 处 | <1h | [store.go:297-373](file:///d:/ccswitch/notifier/internal/store/store.go#L297) |
| 15 | ✨ feature | 补全 PUT /api/v1/channels/:id — 当前只能删除不能更新渠道配置 | 渠道配置变更需先删后建，丢失历史记录 | 1-4h | [router.go:269-293](file:///d:/ccswitch/notifier/internal/api/router.go#L269) |

## 🔵 P3 — 低优先级（排期待定）

| # | 类别 | 描述 | 影响 | 工作量 | 关联文件 |
|---|------|------|------|--------|----------|
| 16 | ✨ feature ✅ | ~~新增 NapCatQQ 渠道~~ → 已实现：`napcat.go` + OneBot11 协议 + 群/私聊支持 | NapCat 比 Qmsg 更强大，支持富文本/图片/群消息 | 半天 | `internal/channel/napcat.go` |
| 17 | ✨ feature | 多渠道故障转移 — 渠道 A 发送失败自动切到渠道 B | 单渠道故障时通知丢失 | 半天 | [router.go](file:///d:/ccswitch/notifier/internal/router/router.go) |
| 18 | 🚀 ops | DB 迁移系统 — 版本化 Schema 变更，支持升级回滚 | 当前 migrate() 用 CREATE IF NOT EXISTS，无法演进表结构 | 半天 | [store.go:24-73](file:///d:/ccswitch/notifier/internal/store/store.go#L24) |
| 19 | 🔧 techdebt | Email 渠道支持 STARTTLS — 当前只支持直接 TLS 连接 | 部分 SMTP 服务器 (如 Gmail) 不支持直接 TLS，需先明文再升级 | 1-4h | [email.go:84-86](file:///d:/ccswitch/notifier/internal/channel/email.go#L84) |
| 20 | 📊 observability | 请求日志补充响应状态码和耗时 — 当前 loggingMiddleware 只记录请求开始 | 无法通过日志排查 API 错误和性能瓶颈 | 1-4h | [router.go:529-544](file:///d:/ccswitch/notifier/internal/api/router.go#L529) |
| 21 | ✨ feature | 单条消息重发 — 管理员可手动触发重发失败的通知 | 失败通知无法补救 | 4h | [router.go](file:///d:/ccswitch/notifier/internal/api/router.go), [store.go](file:///d:/ccswitch/notifier/internal/store/store.go) |
| 22 | 🚀 ops | 持久化消息队列 — DB 持久化队列，重启不丢消息 | 当前内存队列重启丢失所有未发送消息 | 1天 | [queue.go](file:///d:/ccswitch/notifier/internal/queue/queue.go) |
| 23 | 🔧 techdebt | 发送记录降采样 — 高流量时只存储摘要，定期归档 | 大量通知记录导致 SQLite 膨胀 | 4h | [store.go](file:///d:/ccswitch/notifier/internal/store/store.go) |
| 24 | 📝 docs | API 文档 (OpenAPI/Swagger) — 自动生成接口文档 | 第三方集成无参考，需读源码 | 半天 | `docs/openapi.yaml` (新建) |

## ⚪ P4 — 可选（有空再做）

| # | 类别 | 描述 | 影响 | 工作量 | 关联文件 |
|---|------|------|------|--------|----------|
| 25 | 🚀 ops | Admin Web 管理界面 — 渠道管理/通知历史/统计面板 | 只能通过 API 管理，不方便 | 2-3天 | `web/static/` (新建) |
| 26 | ✨ feature | 自定义脚本渠道 — 执行外部脚本发送通知 | 完全灵活，可对接任意系统 | 半天 | `internal/channel/script.go` (新建) |
| 27 | ✨ feature | 通知确认机制 — critical 级别消息要求接收方确认 | 无法知道告警是否被看到 | 1天 | [router.go](file:///d:/ccswitch/notifier/internal/api/router.go) |
| 28 | ✨ feature | 浏览器推送 — Web Push API 推送 | 不依赖微信/QQ/邮件也能收到通知 | 1天 | `internal/channel/webpush.go` (新建) |
| 29 | ✨ feature | 消息模板市场 — 用户可分享/导入通知模板 | 重复造轮子 | 2-3天 | [engine.go](file:///d:/ccswitch/notifier/internal/template/engine.go) |

## 版本规划

| 版本 | 目标 | 包含项目 | 状态 |
|------|------|----------|------|
| v0.2.0 | 安全修复 + 核心缺陷 + 个人微信推送 | #1, #2, #3, #4, #5, #6, #7 | ✅ 已完成 |
| v0.3.0 | 质量保障 + 可观测性 + 高级特性 | #8, #9, #10, #11, #12, #13, #14, #15 | ⬜ 计划中 |
| v0.4.0 | 渠道扩展 + 运维体验 | #16 ✅, #17, #18, #19, #20, #21, #24 | 🔄 进行中 |
| v0.5.0 | 持久化 + 管理界面 | #22, #23, #25 | ⬜ 计划中 |

## 变更记录

| 日期 | 变更 |
|------|------|
| 2026-05-17 | 初始创建 — 基于五维分析模型全面扫描生成 |
| 2026-05-17 | ✅ 完成 #1 SIGHUP 热加载 Registry 修复 |
| 2026-05-17 | ✅ 完成 #2 Admin Token 默认弱密码修复 |
| 2026-05-17 | ✅ 完成 #3 通知记录状态更新修复 |
| 2026-05-17 | ✅ 完成 #4 Notify API 认证模型修复 |
| 2026-05-17 | ✅ 完成 #5 Server酱渠道实现 |
| 2026-05-17 | ✅ 完成 #6 PushPlus渠道实现 |
| 2026-05-17 | ✅ 完成 #7 静默忽略错误修复 |
| 2026-05-17 | ✅ 完成 #16 NapCatQQ 渠道实现 + 端到端测试通过 (Notifier → NapCat → QQ) |
| 2026-05-17 | ✅ Notifier 云服务器部署完成 (systemd + 二进制) |
| 2026-05-17 | ✅ Email 渠道端到端测试通过 (QQ邮箱 SMTP → 邮件收到) |
| 2026-05-17 | ✅ CI/CD 部署脚本 + GitHub Actions 配置完成 |

---

## 附录：五维分析详情

### 维度 1：代码质量

| 检查项 | 状态 | 说明 |
|--------|------|------|
| 测试覆盖率 | ❌ 0% | 无任何 `_test.go` 文件 |
| TODO/FIXME/HACK | ✅ 无 | 代码中无遗留标记 |
| 重复代码 | ⚠️ 存在 | `store.go` 中 `scanChannel`/`scanChannelFromRows`、`scanToken`/`scanTokenFromRows` 几乎完全相同 |
| 静默忽略错误 | ⚠️ 3处 | `json.Unmarshal` 错误被忽略 (store.go L306, L322, L338)；`rand.Read` 返回值未检查 (router.go L556, L562) |
| 过长函数 | ⚠️ 1处 | `handleNotify` ~100 行，可拆分为子函数 |
| 魔法数字 | ⚠️ 存在 | `5*time.Minute, 10000` (dedup)、`4096` (queue cap)、`5` (workers) |

### 维度 2：功能完整性

| 检查项 | 状态 | 说明 |
|--------|------|------|
| Notify API 认证 | ❌ 错误 | 使用 admin_token 而非 api_tokens 表，违反最小权限 |
| SIGHUP 热加载 | ❌ 失效 | 新 Registry 被丢弃 (`_ = newRegistry`) |
| 通知状态更新 | ❌ 缺失 | 入队后状态永不从 "queued" 更新为 "sent"/"failed" |
| 渠道更新 API | ❌ 缺失 | 无 PUT /api/v1/channels/:id |
| 个人微信渠道 | ❌ 缺失 | Server酱/PushPlus/WxPusher 均未实现 |
| NapCatQQ 渠道 | ❌ 缺失 | 完整 QQ Bot 未实现 |
| 渠道熔断器 | ❌ 缺失 | 外部 API 故障时无保护 |
| 多渠道故障转移 | ❌ 缺失 | 单渠道失败时无备选 |

### 维度 3：可观测性

| 检查项 | 状态 | 说明 |
|--------|------|------|
| 结构化日志 | ✅ slog JSON | 使用 `slog.NewJSONHandler` |
| Prometheus 指标 | ⚠️ 仅默认 | `/metrics` 端点存在，但仅暴露 Go 运行时指标，无业务指标 |
| 健康检查 | ⚠️ 过于简单 | `/health` 只返回 `{"status":"ok"}`，不检查 DB/队列 |
| 链路追踪 | ❌ 缺失 | 无 OpenTelemetry |
| 审计日志 | ❌ 缺失 | Token 创建/删除、渠道变更无审计记录 |
| 请求日志 | ⚠️ 不完整 | 不记录响应状态码和耗时 |

### 维度 4：运维就绪度

| 检查项 | 状态 | 说明 |
|--------|------|------|
| 优雅关闭 | ✅ 已实现 | SIGINT/SIGTERM 处理 + 队列排空 |
| 配置热更新 | ⚠️ 部分实现 | SIGHUP 支持，但 Registry 未更新 (P0 #1) |
| Dockerfile | ✅ 完善 | 多阶段构建、非 root 用户、HEALTHCHECK |
| CI/CD | ✅ 完善 | GitHub Actions (build/test/lint/docker push to GHCR) |
| DB 迁移 | ⚠️ 基础 | 仅 `CREATE IF NOT EXISTS`，无版本化迁移 |
| 备份/恢复 | ❌ 缺失 | 无 SQLite 备份机制 |
| 多实例部署 | ❌ 不支持 | SQLite 单写限制，无法水平扩展 |

### 维度 5：用户体验

| 检查项 | 状态 | 说明 |
|--------|------|------|
| 错误信息 | ✅ 较好 | API 返回具体错误描述 |
| 输入校验 | ✅ 已实现 | `NotifyRequest.Validate()` 校验必填字段 |
| API 文档 | ❌ 缺失 | 无 OpenAPI/Swagger 规范 |
| Admin UI | ❌ 缺失 | 仅 API 访问 |
| 默认安全 | ❌ 不足 | admin_token 默认 "changeme" |

---

## 附录：个人微信/QQ 推送方案对比

### 个人微信推送方案

| 方案 | 免费额度 | 推送方式 | 接入难度 | 推荐度 |
|------|---------|---------|---------|--------|
| **Server酱** | 5条/天 | 企业微信应用消息→微信 | ⭐ 极简 | ⭐⭐⭐⭐⭐ |
| **PushPlus** | 200条/天 | 微信公众号→微信 | ⭐ 极简 | ⭐⭐⭐⭐⭐ |
| **WxPusher** | 2000条/天/UID | 微信+独立App | ⭐⭐ 简单 | ⭐⭐⭐⭐ |
| 企业微信机器人 | 无限制 | 企业微信群机器人 | ⭐⭐ 需企业微信 | ⭐⭐⭐ (已实现) |

**推荐方案**：
- **个人用户首选 PushPlus**：免费额度充足(200条/天)，API 最简单，一个 token 搞定
- **备选 Server酱**：生态最成熟，但免费版每天只有 5 条
- **追求到达率 WxPusher**：有独立 App，不依赖微信模板消息

### QQ 推送方案

| 方案 | 推送方式 | 接入难度 | 稳定性 | 推荐度 |
|------|---------|---------|--------|--------|
| **Qmsg** | QQ机器人转发 | ⭐ 极简 | ⭐⭐⭐ 依赖第三方 | ⭐⭐⭐⭐ (已实现) |
| **NapCatQQ** | OneBot协议 | ⭐⭐⭐ 需部署 | ⭐⭐⭐⭐ 自建可控 | ⭐⭐⭐⭐⭐ |

### 快速接入指南

#### PushPlus (推荐个人微信推送)

```yaml
channels:
  - name: "pushplus-wechat"
    type: "pushplus"
    config:
      token: "你的PushPlus token"
      template: "html"
    filter:
      levels: ["critical", "error", "warning"]
    enabled: true
```

#### Server酱 (备选个人微信推送)

```yaml
channels:
  - name: "serverchan-wechat"
    type: "serverchan"
    config:
      sendkey: "你的SendKey"
    filter:
      levels: ["critical", "error"]
    enabled: true
```

#### WxPusher (追求到达率)

```yaml
channels:
  - name: "wxpusher"
    type: "wxpusher"
    config:
      app_token: "你的app_token"
      uids: "UID_xxx"
      content_type: 2
    filter:
      levels: ["critical", "error", "warning"]
    enabled: true
```

#### NapCatQQ (完整 QQ Bot)

```yaml
channels:
  - name: "napcat-qq"
    type: "napcat"
    config:
      api_url: "http://localhost:3000"
      qq: "接收者QQ号"
      message_type: "private"
    filter:
      levels: ["critical", "error", "warning"]
    enabled: true
```

### Watchdog → Notifier 完整链路

```
服务挂了 → Watchdog 探测到 → 调用 Notifier API → 路由到 PushPlus/Server酱 → 微信收到通知
                       → 同时路由到 Qmsg → QQ 收到通知
                       → 同时路由到 Email → 邮箱收到通知
```
