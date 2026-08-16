# FIX-deps 报告 —— Go 工具链与依赖漏洞修复

分支: `fix/ship-deps`
工作目录: `/Volumes/PHD/code/.worktrees/octarq-ship-deps`(仓库相对路径,下同)

## 变更摘要

| 项 | 修复前 | 修复后 |
|---|---|---|
| `go.mod` go 指令 | `go 1.25.11` | `go 1.25.13`(当前可用的最新 1.25.x patch,已从 go.dev/dl 确认) |
| `google.golang.org/grpc` | `v1.77.0` (indirect) | `v1.82.1` (indirect) |
| 连带升级(tidy 后 grpc 依赖链要求) | | `go.opentelemetry.io/otel` v1.41.0 → v1.43.0、`otel/metric`、`otel/trace`、`golang.org/x/oauth2` v0.35.0 → v0.36.0、`genproto/googleapis/api`、`genproto/googleapis/rpc` 同步更新;`golang.org/x/net` 由 indirect 转为 direct(仓库直接 import) |

改动文件: `go.mod`、`go.sum`。未改任何 Go 源码、未碰 `web/`。

## 1. go.mod 与 grpc 升级

```bash
# go 指令升到 1.25.13(go.mod 手工修改)
go get google.golang.org/grpc@v1.82.1   # v1.77.0 => v1.82.1
go mod tidy
```

`go mod tidy` 后依赖图干净,无多余/缺失模块。

## 2. govulncheck 修复前后对比

### 修复前(go1.25.11 + grpc v1.77.0,退出码 3)

```text
=== Symbol Results ===

Vulnerability #1: GO-2026-6218
    Avoid quadratic complexity in resolvePath in net/url
  More info: https://pkg.go.dev/vuln/GO-2026-6218
  Standard library
    Found in: net/url@go1.25.11
    Fixed in: net/url@go1.25.13
    Example traces found:
      #1: internal/safehttp/safehttp.go:154:18: safehttp.Get calls http.Client.Do, which eventually calls url.URL.Parse
      #2: plugins/mail/mcp.go:45:13: mail.Plugin.RegisterMCP calls mcp.AddTool[...], which eventually calls url.URL.ResolveReference

Vulnerability #2: GO-2026-6091
    Fix Javascript regexp context tracking in html/template
  More info: https://pkg.go.dev/vuln/GO-2026-6091
  Standard library
    Found in: html/template@go1.25.11
    Fixed in: html/template@go1.25.13
    Example traces found:
      #1: internal/server/server.go:106:18: server.Server.route calls http.HandlerFunc.ServeHTTP, which eventually calls template.Template.Execute
      #2: internal/server/server.go:106:18: server.Server.route calls http.HandlerFunc.ServeHTTP, which eventually calls template.Template.ExecuteTemplate

Vulnerability #3: GO-2026-6090
    Limit handshake messages we are willing to accept post-handshake in crypto/tls
  More info: https://pkg.go.dev/vuln/GO-2026-6090
  Standard library
    Found in: crypto/tls@go1.25.11
    Fixed in: crypto/tls@go1.25.13
    Example traces found:
      #1: plugins/links/shortlink.go:122:16: links.Engine.Close calls sync.Once.Do, which eventually calls tls.Conn.Handshake
      #2: app/app.go:827:35: app.Run calls http.Server.ListenAndServe, which eventually calls tls.Conn.HandshakeContext
      #3: internal/crypto/crypto.go:65:26: crypto.Cipher.EnableEnvelope calls io.ReadFull, which eventually calls tls.Conn.Read
      #4: internal/mail/send.go:60:13: mail.SMTPSender.Send calls fmt.Fprintf, which calls tls.Conn.Write
      #5: internal/cache/cache.go:34:27: cache.New calls redis.NewClient, which eventually calls tls.DialWithDialer
      #6: internal/safehttp/safehttp.go:154:18: safehttp.Get calls http.Client.Do, which eventually calls tls.Dialer.DialContext

Vulnerability #4: GO-2026-6089
    Apply ReadHeaderTimeout when doing unencrypted HTTP/2 check in net/http
  More info: https://pkg.go.dev/vuln/GO-2026-6089
  Standard library
    Found in: net/http@go1.25.11
    Fixed in: net/http@go1.25.13
    Example traces found:
      #1: app/app.go:827:35: app.Run calls http.Server.ListenAndServe

Vulnerability #5: GO-2026-6088
    Add recursion depth guard during decode in encoding/xml
  More info: https://pkg.go.dev/vuln/GO-2026-6088
  Standard library
    Found in: encoding/xml@go1.25.11
    Fixed in: encoding/xml@go1.25.13
    Example traces found:
      #1: internal/db/backup.go:214:20: db.VerifySQLiteIntegrity calls sql.Row.Scan, which eventually calls xml.Unmarshal

Vulnerability #6: GO-2026-6061
    Vulnerabilities in the xDS RBAC authorization engine and the HTTP/2
    transport server implementation in google.golang.org/grpc
  More info: https://pkg.go.dev/vuln/GO-2026-6061
  Module: google.golang.org/grpc
    Found in: google.golang.org/grpc@v1.77.0
    Fixed in: google.golang.org/grpc@v1.82.1
    Example traces found:
      #1-#6, #8: internal/crypto/crypto.go:65:26: crypto.Cipher.EnableEnvelope calls io.ReadFull, which eventually calls transport.ClientStream.* (Close/Header/Read/RecvCompress/TrailersOnly/Write/Stream.ReadMessageHeader)
      #7: plugins/links/shortlink.go:122:16: links.Engine.Close calls sync.Once.Do, which eventually calls transport.NewHTTP2Client
      #9-#10: llmprovider/langchain.go:169:24: llmprovider.makeGemini calls googleai.New, which eventually calls transport.http2Client.Close / GracefulClose
      #11: internal/crypto/crypto.go:65:26: crypto.Cipher.EnableEnvelope calls io.ReadFull, which eventually calls transport.http2Client.NewStream

Vulnerability #7: GO-2026-5972
    Enforce maximum recursion depth in encoding/asn1
  More info: https://pkg.go.dev/vuln/GO-2026-5972
  Standard library
    Found in: encoding/asn1@go1.25.11
    Fixed in: encoding/asn1@go1.25.13
    Example traces found:
      #1: plugins/links/shortlink.go:122:16: links.Engine.Close calls sync.Once.Do, which eventually calls asn1.Unmarshal

Vulnerability #8: GO-2026-5856
    Invoking Encrypted Client Hello privacy leak in crypto/tls
  More info: https://pkg.go.dev/vuln/GO-2026-5856
  Standard library
    Found in: crypto/tls@go1.25.11
    Fixed in: crypto/tls@go1.25.12
    Example traces found:
      #1-#6: 同 GO-2026-6090 的 tls.Conn.* 调用链(shortlink / app.ListenAndServe / crypto.EnableEnvelope / mail.SMTPSender / cache / safehttp)

Vulnerability #9: GO-2026-5026
    Invoking failure to reject ASCII-only Punycode-encoded labels in golang.org/x/net/idna
  More info: https://pkg.go.dev/vuln/GO-2026-5026
  Standard library
    Found in: net/http@go1.25.11
    Fixed in: net/http@go1.25.13
    Example traces found:
      #1: internal/safehttp/safehttp.go:154:18: safehttp.Get calls http.Client.Do
      #2: internal/auth/oauth.go:227:42: auth.OAuthHandler.Callback calls gothic.init, which eventually calls http.Client.Get
      #3: internal/notify/notify.go:173:25: notify.sendTelegram calls telegram.New, which eventually calls http.Client.PostForm
      #4-#5: internal/dnsprovider/cloudflare.go:192:32: dnsprovider.Cloudflare.VerifyZone calls cloudflare.API.ZoneDetails, which eventually calls http.Transport.CloseIdleConnections / RoundTrip

Your code is affected by 9 vulnerabilities from 1 module and the Go standard library.
This scan also found 3 vulnerabilities in packages you import and 5
vulnerabilities in modules you require, but your code doesn't appear to call
these vulnerabilities.
Use '-show verbose' for more details.
```

### 修复后(go1.25.13 + grpc v1.82.1,退出码 0)

```text
=== Symbol Results ===

No vulnerabilities found.

Your code is affected by 0 vulnerabilities.
This scan also found 1 vulnerability in packages you import and 5
vulnerabilities in modules you require, but your code doesn't appear to call
these vulnerabilities.
Use '-show verbose' for more details.
```

**可达漏洞: 9 → 0。目标达成。**

### 剩余 6 个不可达漏洞(逐条说明为什么留着)

`govulncheck -show verbose` 确认以下 6 个均**不可达**(无调用链,`your code doesn't appear to call these`),修复它们需要动本规格范围之外的直接依赖:

| 漏洞 | 模块 | 现状 | 修复版本 | 为什么留着 |
|---|---|---|---|---|
| GO-2026-5158 | go.opentelemetry.io/otel v1.43.0 | 不可达(otel baggage 解析,仓库未直接调用) | v1.44.0 | 不可达;且 v1.43.0 是 grpc v1.82.1 依赖链解析出的版本,升级超出本线范围(只动 go.mod/CI,不引入无关依赖变更) |
| GO-2026-5942 | golang.org/x/net v0.55.0 | 不可达(dns/dnsmessage 解析 panic,仓库未用 dnsmessage) | v0.56.0 | 不可达;x/net 为多模块直接依赖,升级会牵动整个依赖图,超出本线范围 |
| GO-2026-5932 | golang.org/x/crypto v0.53.0 | 不可达(openpgp 包,仓库未使用) | **N/A(官方判定 unmaintained,无修复)** | 无修复版本,唯一出路是移除该包依赖,超出本线范围 |
| GO-2026-5777 / 5775 / 5774 | github.com/go-chi/chi/v5 v5.2.5 | 不可达(RealIP 中间件 IP 伪造,仓库为 indirect 依赖,未调用 RealIP) | v5.3.0 | 不可达;chi 为间接依赖且仓库自带 safehttp 层,升级超出本线范围 |

## 3. CI workflow 检查结果

**无需修改** —— 所有安装 Go 的 job 均已使用 `go-version-file: go.mod`,无硬编码版本号:

- `.github/workflows/ci.yml`
  - `go` job (Go test & lint): `actions/setup-go@v5` + `go-version-file: go.mod`
  - `web` job: `actions/setup-go@v5` + `go-version-file: go.mod`
  - `spec` job: `actions/setup-go@v5` + `go-version-file: go.mod`(有 `if: steps.tip.outputs.current == 'true'` 条件,但版本来源同样是 go.mod)
- `.github/workflows/govulncheck.yml`: `golang/govulncheck-action@v1` + `go-version-file: go.mod`
- `.github/workflows/release.yml`、`publish-sdk.yml`、`deploy-website.yml`:不安装 Go,无版本来源问题

升 go.mod 后 CI 自动跟随 1.25.13。

## 4. 本机工具链与验证情况

- 本机初始工具链: `go1.25.11 darwin/arm64`
- `GOTOOLCHAIN=auto`,go.mod 升到 1.25.13 后 go 命令自动下载并使用 `go1.25.13 darwin/arm64` 工具链(`golang.org/dl` 机制)。**未**降级 go.mod,未谎称用低版本验证。
- 环境变量: `GOFLAGS=-mod=mod`、`GOPROXY=https://proxy.golang.org,direct`(规格要求,避免 GOPROXY=direct 挂死),`http_proxy` 已 unset。

**全部验证均在本地完成**(使用 go1.25.13 工具链):

```bash
go build ./...        # 通过
go vet ./...          # 通过
go test ./... -race   # 通过(注意:默认 -timeout 10m 不够,internal/api 包需 ~7-12 分钟
                      # 且本机负载高(8 核 load 30+,其他 agent 并行),已用 -timeout 20m 验证)
gofmt -l .            # 无输出
govulncheck ./...     # 可达漏洞 0
```

### 关于 `go test ./... -race` 的一次超时说明(与本改动无关)

首次以默认 `-timeout 10m` 跑 `go test ./... -race`,`internal/api` 包超时(601s)。排查过程:

1. 超时点是 `TestMintedTokenDefaultsToMember`,单独跑该用例 **0.6s 通过**(`go test ./internal/api/ -run '^TestMintedTokenDefaultsToMember$' -race`)。
2. 对照实验:stash 掉本分支改动(回到 go1.25.11 + grpc v1.77.0)后,`go test ./internal/api/ -race` 全包需 **728.6s** 才通过 —— 本身就超过 10 分钟默认超时。
3. 恢复改动后,同一命令 **388.3s 通过**。

结论:超时是该包全包耗时(负载高峰时 ~12 分钟)+ 默认 10m 超时的既有情况,与依赖/工具链升级无关;升级后状态在 `-timeout 20m` 下全绿。

## 5. 提交

- 仅提交 `go.mod`、`go.sum`、`FIX-deps-report.md`(本文件),未使用 `git add -A`,无临时脚本(`.omo/` 未动)。
- 分支: `fix/ship-deps`
