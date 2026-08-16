# TWOFA challenge 迁移到 HttpOnly cookie

## 动机

OAuth 回调在用户开启 TOTP 时签发的 2FA challenge 原本拼进重定向 URL
（`/admin/?twofa=<challenge>`）。challenge 是认证材料，出现在 query string 会被
反向代理访问日志和浏览器历史记录下来。Referrer-Policy 是
strict-origin-when-cross-origin，跨源不泄漏，但本地日志这条是实打实的。

本次把 challenge 的载体从 URL 换成 HttpOnly cookie，URL 只保留「要走二次验证」
这个事实（`/admin/?twofa=1`）。

## 改动

| 文件 | 改动 |
|---|---|
| `internal/auth/twofa_challenge.go` | 新增 `SetTwoFAChallengeCookie` / `TwoFAChallengeFromRequest` / `ClearTwoFAChallengeCookie`；cookie 名 `octarq_2fa_challenge`，Path 常量 `twofaChallengePath` |
| `internal/auth/oauth.go` | `Callback` 的 TOTP 分支改为 `SetTwoFAChallengeCookie(w, r, challenge)` + 重定向 `/admin/?twofa=1`（不再 `url.QueryEscape`，删除 `net/url` 导入） |
| `internal/api/auth.go` | `verify2FA`：删除 `ChallengeToken` 请求体字段，challenge 改从 cookie 读；成功签发会话后立即清 cookie；challenge 无效（`uid==0`）时也清 |
| `web/src/api.ts` | `verify2FAChallenge(code)` 只发 `{code}`，不再传 `challengeToken` |
| `web/src/shell/Login.tsx` | `oauthChallenge`（字符串）→ `oauthPending`（布尔），只从 query 读 `twofa=1` 事实；删除成功后 `history.replaceState` 清理逻辑（URL 里已无密钥材料） |
| `internal/api/oauth_twofa_test.go` | 守卫测试重写（见下） |

## Cookie 属性（照抄 session cookie 的派生方式）

对照 `internal/auth/auth.go` `setCookie`（SetSessionFromRequest 的落点）：

| 属性 | session cookie | 2FA challenge cookie |
|---|---|---|
| HttpOnly | true | true |
| SameSite | Lax | Lax |
| Secure | `origin.Secure(r, trustProxy)` | 同左（逐请求派生，信任代理时看 X-Forwarded-Proto） |
| Max-Age | `sessionTTL` | `int(twofaChallengeTTL.Seconds())` = 600，与 challenge 10 分钟 TTL 一致 |
| Path | `/` | `/api/auth/2fa/verify`（收窄） |

**Path 收窄依据**：challenge 值的唯一消费者是 `POST /api/auth/2fa/verify`；
2FA 页面本身只需要 URL 里的 `twofa=1` 事实，从不接触 challenge 值。把 Path
钉在唯一消费端点上，cookie 不会随 `/admin/` 页面或任何其他 `/api/` 请求兜圈。
设置与清除共用同一个 `twofaChallengePath` 常量，否则浏览器会把两者当成两个
不同 cookie。

## 清理策略（失败路径的选择）

- **成功签发会话后**：`ClearTwoFAChallengeCookie`（Max-Age=-1 过期），与
  session cookie 同响应返回。challenge 一旦换来会话就作废，不能重放。
- **challenge 无效**（`uid==0`：伪造/过期/畸形）：立即清除。credential 已死，
  没有重试能救活，留着只会让浏览器反复重交。
- **TOTP 码错误**（`verifyTOTPOrRecovery` 失败）：**不清除**。challenge 仍然
  有效，清除会把一次手误变成整个 OAuth 流程重来（用户得再去 provider 走一遍
  round-trip）；重试已被 `loginLimiter` 限流（默认 60/min），且 cookie 的
  Max-Age=600 与签名内嵌的 TTL 同时到期，不存在放大窗口。

## CSRF 路径确认

`POST /api/auth/2fa/verify` 仍过 `CSRFGuard`（`internal/api/csrf.go`，
在 `app/app.go` 用 `api.CSRFGuard(secret, mux)` 包住整棵路由）：

1. challenge 流程下请求带 cookie（challenge cookie）→ 触发「any cookie」分支的
   Origin/Referer 同源校验；SPA 同源 fetch 自带 Origin，通过；跨站伪造表单被拦。
2. 双提交 token 校验只对携带 `octarq_session` 的请求生效；challenge 流程没有
   session cookie（这正是 2FA 在拦的东西），所以不要求 `X-CSRF-Token`，
   `csrfFetch.ts` 也因没有 `octarq_csrf` cookie 而不加头——两边一致。
3. 纵深防御：challenge cookie 本身 `SameSite=Lax`，跨站 POST 根本不会带上它。
4. 测试环境（`h.Routes()`）不带 CSRFGuard 包装，与生产路径的差异不影响本修复
   的守卫语义；CSRF 守卫自身行为由 `csrf_test.go` 矩阵覆盖（未改动）。

## 守卫测试与变异验证

守卫测试在 `internal/api/oauth_twofa_test.go` 的
`TestOAuthCallbackRequiresTwoFactorForTOTPUsers`：

- `oauth_twofa_test.go:158` — cookie 是 HttpOnly（且 Max-Age=600）
- `oauth_twofa_test.go:166` — **重定向 Location 不含 challenge 值**（本次修复的直接钉住点）
- `oauth_twofa_test.go:175` — 无 code + 有 cookie → 401，且不签发 session
- `oauth_twofa_test.go:185` — 错误验证码不清 cookie（可重试策略）
- `oauth_twofa_test.go:196` — 有 code 无 cookie → 401（钉住「challenge 从 cookie 读」）
- `oauth_twofa_test.go:206` — 验证成功同响应清除 challenge cookie

变异验证（每条：产品代码 `&& false` / 等价改法 → 确认变红 → 还原）：

| 产品代码变异点 | 改法 | 变红的测试用例 |
|---|---|---|
| `internal/auth/oauth.go:277`（重定向） | `"/admin/?twofa=1&c="+challenge`（仍编译） | `oauth_twofa_test.go:166` "challenge value leaked into the redirect Location" |
| `internal/auth/twofa_challenge.go:80`（Set 的 HttpOnly） | `HttpOnly: true && false` | `oauth_twofa_test.go:158` "2FA challenge cookie is not HttpOnly" |
| `internal/api/auth.go:209`（成功后清除） | `if challenge && false` | `oauth_twofa_test.go:206` "spent challenge cookie was not cleared after successful verify" |

三处均确认变红后还原，`git diff` 恢复为仅含本次修复的改动。

## 验证（CI 命令原样）

```
gofmt -l .                 → 无输出
go build ./...             → ok
go vet ./...               → ok
go test ./... -race        → 全过，无 FAIL
cd web && pnpm exec tsc --noEmit  → 0
pnpm test                  → 26 files / 96 tests 全过
pnpm i18n:audit            → All i18n checks passed
```

## 未纳入本次范围

- challenge 仍是无状态签名设计（`twofa_challenge.go`），没有引入服务端 pending
  状态表——只是载体从 URL 换成 cookie。
- `R3-twofa-report.md` 描述的是上一提交的 URL 载体状态，未改动；本文档为本次
  修复的记录。
