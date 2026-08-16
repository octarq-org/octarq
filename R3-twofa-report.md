# R3-twofa Report — 堵住 OAuth 绕过 2FA 的洞

分支：`feat/twofa`（基于 `origin/main` @ 8019c4e）

## 漏洞与修复概览

`TOTPEnabled` 原先只在密码登录路径（`internal/api/auth.go` 的 `loginHuma` / `verify2FA`）被检查。
三条外部登录路径直接 `SetSessionFromRequest`、零检查：`internal/auth/oauth.go` 的 OAuth 回调、
`internal/auth/identity.go` 的 `LoginByIdentity`（per-org SSO）、`internal/auth/provision.go` 的
`LoginByEmail`。攻击者拿到受害者在 Google/GitHub 的会话即可绕过已启用的 2FA 建立完整会话。

## 1. OAuth 复用了哪套待验证机制（为什么不是新造一套）

密码登录的「待验证」机制是**无状态重认证**，不是临时 token：`loginHuma` 返回
`twoFactorRequired: true` 后，前端把 `email + password + code` 重新发给
`POST /api/auth/2fa/verify`（`verify2FA`），服务端重验密码证明「请求方确实持有该账号凭据」，
再走 `verifyTOTPOrRecovery` 校验 TOTP/恢复码，最后 `SetSessionFromRequest` + 审计 `user.login`。

OAuth 账号没有本地密码可重发（`PasswordHash == ""`），所以复用时用**签名挑战凭据**代替密码：

- `internal/auth/twofa_challenge.go`（新增）：`Manager.NewTwoFAChallenge(uid)` 铸造
  HMAC-SHA256 签名、10 分钟 TTL、带随机 nonce 的挑战令牌；`VerifyTwoFAChallenge` 校验签名与过期。
  故意不建任何「待验证状态表/缓存」——没有第二套 pending 状态可漂移。
- `internal/auth/oauth.go` Callback：用户 `TOTPEnabled` 时不 `SetSessionFromRequest`，
  重定向到 `/<SPA>/admin/?twofa=<challenge>`，复用密码登录同一个 2FA 输入页。
- `internal/api/auth.go` `verify2FA`：新增可选字段 `challengeToken`。两个分支（email+password /
  challengeToken）共享**同一端点、同一 `verifyTOTPOrRecovery`、同一会话签发、同一审计**。
  challenge 分支额外要求用户当前仍 `TOTPEnabled`（铸造后禁用 2FA 的账号必须重开登录）。

即：机制唯一，只是「证明待验证主体」的凭据不同（密码 vs 签名挑战）。挑战必须配有效
TOTP/恢复码才能换会话，且受 `loginLimiter` 限速。

## 2. LoginByEmail 的调用方清单

grep 全仓（octarq + octarq-pro，排除测试）：

| 调用方 | 位置 | 性质 |
|---|---|---|
| `app.App.loginByEmail` | `app/app.go:292` | 唯一接线：`plugin.Context.LoginByEmail`（`plugin/plugin.go:281`） |
| 测试 | `internal/auth/provision_test.go` | — |

- **octarq-pro 内没有任何 `LoginByEmail` 调用**（Pro 的 SSO 模块 `modules/sso/sso.go` 只用
  `LoginByIdentity`，第 109、414 行）。
- 本仓 `plugins/`、`cmd/`、`cli_plugin.go` 无调用。
- 结论：它只服务「已由外部身份源验证 email 的插件登录」（JIT/SSO 性质），**不存在管理员代登录
  之类需要强制 2FA 的调用方**。按已决策方案与 SSO 同处理：显式注释 + 审计，不强制 TOTP。

## 3. 审计日志事件名

`auth.sso_login_bypassed_totp`（`internal/auth/identity.go` 与 `internal/auth/provision.go`）。

- 触发条件：`LoginByIdentity` / `LoginByEmail` 登录了一个 `TOTPEnabled` 用户。
- 写入方式：`Manager.auditSSOTOTPBypass`——同步插入 `models.AuditLog`（Actor=用户本人、
  Target=用户，Meta 含 `method` 及可用的 `provider`/`issuer`，IP 来自 `reporterIP`）。
  同步是有意的：审计行是登录决策的一部分，丢了就跟没写一样。
- 语义：SSO 的第二因子由 IdP 承担（决策 2），该行让运维者能区分「SSO 后有 IdP MFA」与
  「SSO 后无 MFA」。

## 4. 变异验证结果（file:line → 变红用例）

每条守卫都用「仍能编译」的短路改法验证，确认变红后已改回：

| 变异 | 变红用例 | 结果 |
|---|---|---|
| `internal/auth/oauth.go:263` `if user.TOTPEnabled` → `&& false` | `TestOAuthCallbackRequiresTwoFactorForTOTPUsers`（回调直接 `/admin/` 发会话，不再去 2FA 页） | 变红 ✓ |
| `internal/auth/identity.go:58` `if m.userHasTOTP(uid)` → `&& false` | `TestLoginByIdentityWithTOTPUserWritesBypassAudit`（无审计行） | 变红 ✓ |
| `internal/auth/oauth.go:263` `if user.TOTPEnabled` → `if true`（反向变异） | `TestOAuthCallbackWithoutTwoFactorStillSignsIn`（非 TOTP 用户被误送去 2FA 页） | 变红 ✓ |

守卫恢复后三条测试全部转绿。

## 5. 新增的五语言 key（web/src/i18n/）

- `app.twoFactorOAuthDesc`（登录页 OAuth 2FA 提示）→ `en.ts` / `zh.ts` / `ja.ts` / `es.ts` / `pt.ts`
- `settings.twoFACoverageBase` / `settings.twoFACoverageOauth` / `settings.twoFACoverageSso`
  （安全页 2FA 覆盖范围）→ `pages/settings.ts` 的 en/zh/es/pt/ja 五段

安全页（`web/src/pages/settings/security.tsx`）在 2FA 启用时按实例实际配置显示覆盖范围：
base 恒显；`authConfig()` 检测 Google/GitHub OAuth；`GET /api/auth/methods`（public，
plugin-sso 仅在配置后注册 `sso` 方法）检测企业 SSO——明确告知「SSO 由 IdP 自身 MFA 承担」。

## 6. 改动文件清单（均为本次新增/修改）

```
internal/auth/twofa_challenge.go        (新增) 签名挑战凭据
internal/auth/oauth.go                   OAuth 回调 TOTP 门
internal/auth/identity.go                LoginByIdentity 注释 + 审计
internal/auth/provision.go               LoginByEmail 注释 + 审计
internal/api/auth.go                     verify2FA challengeToken 分支
internal/api/oauth_twofa_test.go        (新增) 守卫测试 1、2（走真实 /auth/callback/google 路由）
internal/auth/twofa_test.go             (新增) 守卫测试 3 + 挑战凭据往返 + LoginByEmail 审计
web/src/api.ts                           verify2FAChallenge
web/src/shell/Login.tsx                  OAuth 2FA 表单模式
web/src/pages/settings/security.tsx      2FA 覆盖范围文案
web/src/i18n/{en,zh,ja,es,pt}.ts         app.twoFactorOAuthDesc
web/src/i18n/pages/settings.ts           settings.twoFACoverage*
```

未触碰：`internal/api/register.go`、`recovery.go`、`settings.go`、`internal/api/api.go`、
`plugins/mail/`、`app/readiness.go`（范围边界外）。

## 7. 验证结果

```bash
unset http_proxy
gofmt -l .            # 无输出 ✓
go build ./...        # ✓
go vet ./...          # ✓
go test ./... -race   # 35 包全绿 ✓（本机 OOM，用 GOMAXPROCS=4 复跑；默认并发下 internal/api -race 在干净树同样 OOM kill，见下）
cd web
pnpm exec tsc --noEmit  # ✓
pnpm test             # 96/96 全绿 ✓（--testTimeout=30000；默认 5s 超时在本机高负载下 flaky——干净树同样失败，单独跑该文件通过）
pnpm i18n:audit       # 全部通过 ✓（硬门禁）
```

环境备注：本机（外置盘 + -race 内存翻倍）下 `internal/api -race` 全量默认并发会
`signal: killed`（OOM）；在 `git stash` 干净树上同样复现（602s killed，ulule/limiter 栈），
属 pre-existing 资源问题而非本改动引入。`GOMAXPROCS=4` 后全量 -race 通过。
