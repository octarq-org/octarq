# R3-instance-api — 交付报告

分支：`feat/instance-api`（worktree `/Volumes/PHD/code/.worktrees/octarq-instance-api`）
只改 Go；`web/`、`openapi/` 未动（openapi 由 CI 刷新）。

---

## 1. 系统发件人（任务 1：修复自助注册验证邮件发不出去的 bug）

### 根因

`register.go` 在创建 org/OrgMember 的事务**之前**调 `sendVerificationEmail`，
`primaryOrgForUser` 查不到 membership 返回 0，`sendMail(0, …)` 按 `owner_id = 0`
查 sender 永远落空。上一轮的 `mail.ready`（全实例有没有 sender）绕不过这一点。

### 新增服务契约（plugin/services.go）

```go
const ServiceMailSendSystem = "mail.send.system"

type SystemMailSender func(to, subject, htmlBody, textBody string) error
```

签名不带 orgID——系统邮件不依赖收件人属于哪个 org（注册时还没有 org）。
编译期断言照 `plugin.MailReady` 写法（plugins/mail/plugin.go）：

```go
_ plugin.SystemMailSender = (*Plugin)(nil).sendSystemMail
```

### 实例设置

新 key：`mail_system_sender_id`（internal/api/settings.go `keySystemSenderID`），
存 SMTPSender 的全局 id：

- `getInstanceSettings` 返回 `systemSenderId`（未设置 = 0）
- `updateInstanceSettings` 接受 `systemSenderId`（0 清除）
- 仅 instance-admin 可读写（沿用端点现有门禁）

### mail 插件侧实现（plugins/mail/plugin.go）

- `systemSender()`：实例设置指定的 sender 存在 → 用它；未指定或指向已删除的
  sender（stale）→ 回落 **id 最小的 sender**（确定性）；一个都没有 → 明确错误
  `"no SMTP sender configured on this instance; system email cannot be sent. …"`
- `sendSystemMail(to, subject, htmlBody, textBody)`：经 `systemSender()` 发送；
  失败事件与用量按 sender 所属 org 记账（与 `sendMail` 一致）
- `Mount` 里注册 `plugin.ServiceMailSendSystem → plugin.SystemMailSender(p.sendSystemMail)`
- `mailReady()` 语义收紧为「系统发件人可用」：实现仍是全实例 sender 计数——
  与 `systemSender()` 的可解析性同一真源（有任一 sender 即能发系统邮件）

### core 侧三处系统邮件全部改走新契约（签名去掉 org/userID 依赖）

| 邮件 | 位置 | 变更 |
|---|---|---|
| 验证邮件 | internal/api/recovery.go `sendVerificationEmail(to, verifyURL)` | 原 (userID, to, url) |
| 重置邮件 | internal/api/recovery.go `sendPasswordResetEmail(to, resetURL)` | 原 (userID, to, url) |
| 邀请邮件 | internal/api/tenant_menu.go `sendInviteEmail(to, acceptURL)` | 原 (orgID, to, url) |

调用点同步更新：register.go、recovery.go（forgot / resend）、auth.go（changeEmail）、
tenant_menu.go（addOrgMember）。

`sendMail(orgID, …)`（`mail.send`）**保留**给业务邮件，签名未动。
`primaryOrgForUser` 保留（primary_org_test.go 仍钉住它），但不再出现在系统邮件路径上。

### 注册门禁

register.go 的 `requireEmailVerification() && !mailReady()` → 503 门禁不变，
`mailReady()` 现在就是「系统发件人可用」——注册门禁、启动日志、readiness API
走同一个 `mail.ready` 服务真源。

---

## 2. readiness 变成可查询的 API（任务 2）

### 单一真源重构

判定逻辑抽到新包 `internal/readiness`：

- `type Check struct { ID, Status, Title, Detail, FixPath }`（JSON 字段
  `id, status, title, detail, fixPath`）
- `func Evaluate(cfg, mailReady, domainsRegistered, requireEmailVerification bool) []Check`
- `app/readiness.go` 的 `readinessReport` 变成纯渲染层：`Evaluate` → `[]readinessLine`，
  日志文案/状态不变（`ok|degraded|dev`，新增 `blocked`）；`redactDSN` 随判定逻辑
  移入 internal/readiness

### 端点

`GET /api/instance/readiness`（internal/api/instance_readiness.go），要求
instance-admin：

- 响应：**裸数组**，每项 `{ id, status, title, detail, fixPath }`（与
  `/api/instance/plugins` 的裸数组风格一致）
- status 词汇：`ok | degraded | blocked`。日志侧独有 `dev` 收敛为 `ok`
  （dev =「非故障的信息性提示」，detail 里仍写明开发模式）——这是两套
  展示词汇的映射，不是判定逻辑的复制
- 检查项：`public-origin`、`outbound-mail`、`registration`、`database`、
  `hardening`（dev 时出现）、`secret-key`
- **自相矛盾检测**（必须项）：`requireEmailVerification` 为真且系统发件人
  不可用 → `registration` 为 `blocked`，detail 说明「注册功能当前是坏的：
  新用户会卡在验证邮件这一步」，fixPath `/mail?tab=settings`
- 绝不带任何密钥值（数据库行走 `redactDSN`；secret key 只报长度状态）

`fixPath` 契约（前端解释的字符串）：`/domains`、`/mail?tab=settings`、
`/settings/instance`。

### 启动日志

`app/app.go` 的 readiness 日志调用补上 `apiHandler.RequireEmailVerification()`
（settings.go 新增的公开包装，注册门禁/readiness API 读同一 setting）。

---

## 3. `/api/instance*` 门禁收紧（任务 3）

| 端点 | 之前 | 现在 |
|---|---|---|
| GET /api/instance/build | 仅登录（漏洞） | **instance-admin**（instance_build.go:43） |
| GET /api/instance/plugins | 已要求 instance-admin | 不变 |
| GET /api/instance/readiness | 新增 | **instance-admin** |
| GET/PUT /api/instance-settings | 已要求 instance-admin | 不变 |
| GET /api/admin/backup | 已要求 instance-admin | 不变 |

全仓核查（api.go 及插件路由，`/api/instance` 前缀穷举）：除上表外没有其他
`instance` 端点漏门禁。

---

## 4. `isInstanceAdmin` 的来源（任务 4，两步走）

`GET /api/auth/me` 响应新增 `isInstanceAdmin`（MeOutputBody，auth.go，
取自 `user.IsInstanceAdmin`——与 `settings()` 里 `isInstanceAdmin(r)` 读的是
同一列）。

**有意保留** `settings()` 里的 `isInstanceAdmin` 字段：前端
`web/src/App.tsx:464` 还在读它，删了会炸。前端切换 + 旧字段清理放后续批次。
这一步只新增来源。

---

## 5. 守卫测试 + 变异验证

### 守卫清单（全部通过，`go test ./... -race` 见第 7 节）

| # | 守卫 | 测试 |
|---|---|---|
| 1 | 无 sender：系统邮件返回明确错误 + 注册 503 | `TestSendSystemMailNoSenderReturnsClearError`（plugins/mail/system_mail_test.go）、`TestRegisterFailsLoudlyWhenMailUnavailable`（已有） |
| 2 | 有 sender：无任何 org membership 时验证邮件仍发出（钉住本次 bug） | `TestVerificationEmailDeliveredWithoutOrgMembership`（internal/api/system_mail_test.go，真实 SMTP 会话抓 DATA） |
| 3 | 非 instance-admin 的成员请求 /api/instance/build → 403 | `TestInstanceBuildRequiresInstanceAdmin`（instance_build_test.go） |
| 4 | readiness API 响应不含密钥值（照 `TestReadinessReportOmitsSecrets` 思路） | `TestInstanceReadinessOmitsSecrets`（instance_readiness_test.go，哨兵值） |
| 5 | requireEmailVerification 开 + 无系统发件人 → readiness `blocked` | `TestInstanceReadinessBlockedWithoutSystemSender` |

补充：`TestInstanceReadinessRequiresInstanceAdmin`（401/403/200 三角）、
`TestInstanceReadinessOKWithSystemSender`、`TestSystemSenderResolution`
（指定/回落/stale）、`TestMeReportsIsInstanceAdmin`、
`TestInstanceSettingsSystemSenderIDRoundTrip`。

### 变异验证（`if cond && false` 等仍可编译的短路改法，逐个确认变红后改回）

| 产品代码（短路点） | 变异 | 变红的用例 |
|---|---|---|
| plugins/mail/plugin.go:357 `systemSender()` 无 sender 错误分支 | `err != nil && false` | `TestSendSystemMailNoSenderReturnsClearError` |
| plugins/mail/plugin.go:337 `mailReady()` | `return n > 0 || true` | `TestRegisterFailsLoudlyWhenMailUnavailable` |
| plugins/mail/plugin.go:357 回落查询 | 加 `Where("owner_id = ?", 0)`（复刻原 bug） | `TestVerificationEmailDeliveredWithoutOrgMembership` |
| internal/api/instance_build.go:43 门禁 | `if !h.isInstanceAdmin(r) && false` | `TestInstanceBuildRequiresInstanceAdmin` |
| internal/readiness/readiness.go:84 数据库行 | 绕过 `redactDSN` 直接拼 `cfg.DBDSN` | `TestInstanceReadinessOmitsSecrets` |
| internal/readiness/readiness.go:119 blocked 分支 | `requireEmailVerification && false` | `TestInstanceReadinessBlockedWithoutSystemSender` |

每条确认变红后已还原（`git diff` 无残留）。测试都走生产代码路径，没有在测试里
重写判定逻辑。

---

## 6. 变更文件

- 新增：`internal/readiness/readiness.go`、`internal/api/instance_readiness.go`、
  `internal/api/instance_readiness_test.go`、`internal/api/system_mail_test.go`、
  `plugins/mail/system_mail_test.go`
- 修改：`plugin/services.go`、`plugins/mail/plugin.go`、`internal/api/settings.go`、
  `internal/api/recovery.go`、`internal/api/register.go`、`internal/api/auth.go`、
  `internal/api/tenant_menu.go`、`internal/api/instance_build.go`、`internal/api/api.go`、
  `app/readiness.go`、`app/app.go`、`app/readiness_test.go`（补第 4 参）、
  `internal/api/api_test.go`（抽出 `newTestHandlerRawCfg`）、
  `internal/api/instance_build_test.go`、`internal/api/origin_host_test.go`
  （抓包 fake 改挂 `mail.send.system`）

## 7. 验证

```bash
gofmt -l .          # 无输出
go build ./...      # OK
go vet ./...        # OK
go test ./... -race # 全绿（EXIT=0，35 个包 ok）
```

`go test ./... -race` 首次跑因与其他 worktree 的 agent 并行、机器过载触发
10m 默认包超时（仅 internal/api，无任何失败断言、无 DATA RACE）；单包
`go test ./internal/api/ -race -timeout 30m` 通过（686s），随后完整套件
`go test ./... -race -count=1 -timeout 30m` 全绿。

`openapi/` 未手动改（CI 负责刷新）。
