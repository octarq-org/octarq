# R4-instance-console —— 实例运维台:独立入口 + 首启 wizard

分支:`feat/instance-ui`。只改了 `web/`,未动 Go,未动 `octarq-pro`(只读参考)。
后端地基(`feat/instance-api`,commit `57a735b`)与 `UIPlugin.replaces`(commit `00f1ca0`)
均已确认合入(开工前 `git log` 核对)。

## 一、新 basename 的接法

- `web/src/main.tsx`:第三个 basename case,顺着 `/status` 的模式写:
  - `/status`(精确)→ `"/"`
  - 路径以 `/instance` 开头 → `"/instance"`
  - 其余 → `"/admin"`
- `web/src/App.tsx`:在 `/status` 分支之后插入 `window.location.pathname.startsWith("/instance")`
  → `<Suspense><InstanceConsole/></Suspense>`。控制台自带认证门禁,不走租户 Shell 的
  `authed` 状态机;`/instance` 模式的会话根本不启动租户 Shell(无 org 切换器、
  无插件菜单管线)。
- 资产仍从 `/admin/assets/…` 加载(Vite base),与 `/status` 完全同机制。
- 开发期桥:`web/vite.config.ts` 新增一个 dev 中间件(`spa-basename-html`),在 base
  检查前把 `/instance…`、`/admin`、`/status` 重写到 `/admin/` 入口。**原因**:
  vite dev 对 base 之外的路径一律 404,而后端 ServeMux 已能服务 `/admin` 与 `/status`
  、唯独 `/instance` 还没有路由(见「阻塞项」)。`/admin`、`/status` 两分支只是对齐
  server.go 已有行为;`/instance` 分支在对应后端路由落地后(见 report 结论,删除
  vite 中间件里除 `/instance` 外的分支)应一并移除。

## 二、首启 wizard:readiness → 步骤派生

- 步骤**完全由 `GET /api/instance/readiness` 派生**,前端零本地完成态:
  - `steps = checks.filter(c => !!c.fixPath)` —— 只挑可修复项,顺序即服务端顺序
    (见 `web/src/pages/instance/shared.tsx` 的 `fixableChecks` / `useInstanceReadiness`)。
  - 完成与否 = `status === "ok"`;每步重新拉取,`Refresh` 按钮只做重新请求。
  - 排序/分组/视觉映射是前端唯一职责;blocked 与 degraded 视觉权重显著不同:
    - `blocked`:`border-danger-border bg-danger-bg/30` + `XCircle` 危险色图标 +
      `Blocked`(tone=danger)徽章 + 顶部红色横幅「N blocked item(s)…」
    - `degraded`:`border-warning-border/60 bg-warning-bg/20` + 琥珀色 `AlertTriangle` +
      `Degraded`(tone=warning)徽章
    - `ok`:`border-success-border/40` + 绿色 `CheckCircle2` + `Done`
- 入口行为:`/instance` 在存在任何未完成的可修复项时渲染 wizard(首启引导),
  **wizard 可跳过**(`/wizard` 页的 "Skip for now" → `/console`),home 在仍有问题时
  渲染同一健康摘要在顶部并保留「Open setup wizard」入口(`/instance` 与
  `/instance/wizard`、`/instance/console` 三条路由,全部由同一份 readiness 快照决定,
  无本地状态)。全部完成 → wizard 显示 `Setup complete`,入口变成平铺首页。
- 检查项标题按 `t(\`instance.check.${id}\`, title)` 做本地化(仅翻译已知 id,未知 id
  回退服务端 title)——这是对派生列表的**翻译**,不是第二份步骤清单。

## 三、迁移页面与 shared.tsx 归属

- `git mv` 三个页面到 `/instance` 下并改写:
  - `web/src/pages/settings/instance.tsx` → `web/src/pages/instance/settings.tsx`
  - `web/src/pages/settings/auth.tsx` → `web/src/pages/instance/auth.tsx`
  - `web/src/pages/settings/instance-plugins.tsx` → `web/src/pages/instance/plugins.tsx`
- 改写内容:去掉 `forbidden`/`InstanceAdminOnly`(Shell 已在门下挡住非管理员,
  页面不再需要自己的门禁);settings/auth 的数据钩子换成控制台本地的
  `useInstanceSettings()`(无 `settings()` isInstanceAdmin 舞蹈,门禁由 shell 保证,
  见 `web/src/pages/instance/shared.tsx`)。
- **`shared.tsx` 不迁移**:核验后发现 `web/src/plugins/dns|links|mail` 的多个租户页
  (`ProviderAccounts`、`LinkSettings`、`MailSettings`、`SMTPSenders`)通过完整相对路径
  `../../../pages/settings/shared` 使用 `SavedBadge`/`useInstanceSettingsData`,租户页
  也在用 → 按规格保留 `web/src/pages/settings/shared.tsx` 原样(四个导出全留);
  控制台页的 `SavedBadge` 从 `../settings/shared` 导入,`useInstanceSettings` 留在
  console 本地。
- `web/src/shell/areas.tsx` 的 "Instance" 分组删除三个页面项,改为**单个出口项**:
  `Instance Management`(`/instance`,`external: true`)。这是规格允许的「实例管理 →」
  出口,同时 `groups.Instance`(五语言)继续被 Pro license 菜单的 category 命中劫持
  (见下)。

## 四、旧路径重定向清单(不能 404)

`web/src/pages/Settings.tsx` 内:
- `/settings/instance` → `/instance`(跨 basename 全页跳转)
- `/settings/instance/*` → `/instance/*`(前缀剥离)
- `/settings/instance/auth` → `/instance/auth`
- `/settings/instance/plugins` → `/instance/plugins`
- `/settings/auth`(旧别名)→ `/instance/auth`(显式 to)

实现:`web/src/pages/instance/redirect.tsx` 的 `InstanceExitRedirect`。**必须全页跳转**:
控制台是独立 basename,router `<Navigate>` 会生成 `/admin/instance`(租户 404),所以
用 `useLocation()` 计算目标 + `window.location.replace`。另把 `plugins.tsx:142` 的
「See what this instance has loaded」链接从 `<Link to="/settings/instance/plugins">`
改为 `<a href="/instance/plugins">`(跨 basename)。

## 五、Pro license 页落位结论(查清 + 待决策)

- 现状:octarq-pro 前端 `packages/plugin-licensing/src/index.ts` 只有 `routes:
  [{ path: "/settings/license" }]`,**无 category/area**;落位完全由 Go 侧
  `modules/licensing/licensing.go:86` 的 MenuProvider 决定:`Category: "Instance"`。
  (octarq-pro 只读,本批次未动。)
- 删掉静态 Instance 分组后它会掉到**哪里**?答案是:**仍留在租户 Settings**,外观
  由 `mergeAreas`(App.tsx)在 `areaForCategory("instance") → "settings"` 后**动态重建
  一个名为 "Instance" 的分组**并只放 license 一项(mergeAreas 会对没有匹配分组的
  category 就地 push 新分组)。对非实例管理员,现有 `currentSettingsArea` 按
  label 过滤 `Instance` 分组,继续隐藏 license 入口。即:license 页不回退 404,也不进
  控制台。
- `UIPlugin.replaces` 是不是同类问题?**不是**。replaces 是「整个 UIPlugin 被替换」
  (routes/widgets/areas/i18n 全停,见 packages/plugin-sdk/src/contract/types.ts 与
  registry `effectivePlugins()`),而 license 的**落位**来自 Go 侧 MenuProvider 的
  category,前端契约碰不到后端菜单。要把它挪进实例控制台,缺的能力是「把某个
  **后端菜单项**(路径/分类)归属到另一条 Area/入口」——既不是替换插件也不是
  声明新 Area(控制台本来就不是通过 pluginAreas 实现的)。按规格不扩契约:
  **标为待决策**——建议由 octarq-pro 侧按同样思路把 licensing 菜单 category 改为
  控制台可识别(或后端增加 `/instance` 菜单分组),本批次不改 Pro。

## 六、i18n

- 新命名空间 `web/src/i18n/pages/instance.ts`:`instance.*` 全部五个词典
  (en/zh/es/pt/ja),值均与 en 不同(除非 allowlist),占位符完全一致。
- 新增 `nav.*`(五语言,id 均无点号):`console-overview`、`console-wizard`、
  `console-settings`、`console-auth`、`console-plugins`(控制台 rail)、
  `instance-console`(租户侧出口项);渲染全部走 `translateNavItemLabel`。
- 删除已死键(迁移后无引用):`nav.auth`、`nav.instance`、`nav.instance-plugins`
  (五语言同步删;已确认 Go 侧无同 id 菜单,`checkGoMenuI18n` 仍绿)。
- `groups.Instance` 保留(license 动态分组仍需它)。

## 七、测试

`web/src/pages/instance/console.test.tsx` + `redirect.test.tsx`,全部通过:

1. 非实例管理员访问 `/instance` → 看到中性提示("This page isn't available to your
   account"),无任何实例功能(`instanceReadiness`/`instanceBuild` 均未被调用,
   无 wizard、无健康页)。
2. wizard 步骤来自 readiness 响应:mock 含一条 `blocked` 项 → 该步渲染 `data-state=
   "blocked"`,`[data-state="blocked"]` 存在且其 Fix 链接为 `/admin/mail?tab=settings`,
   另有 `data-state="degraded"` 与 `data-state="ok"`;无 fixPath 的 database 项不进
   wizard(共 3 步);顶部冒 "1 blocked item" 横幅;`Blocked`/`Degraded` 徽章并存。
3. 旧路径重定向:`/settings/instance` → `replace("/instance")`;`/settings/instance/
   auth` → `replace("/instance/auth")`;`/settings/auth`(to 显式)→ `/instance/auth`。
   测试用父路由 `/settings/*` 包裹,镜像真实嵌套(后代路由按父剩余路径匹配)。

**变异验证**(短路改法,仍能编译;变红后全部改回):

| 变异 | 命中测试 | 结果 |
|---|---|---|
| `web/src/pages/instance/console.tsx:68` 门禁 `if (!isAdmin)` → `if (!isAdmin && false)` | 「shows a neutral notice to non-instance-admins…」 | 红(该 run 内第二用例级联红;用例单独跑仍绿,确认级联非本逻辑) |
| `web/src/pages/instance/shared.tsx:42` `stepState` blocked 判定 → `if (status === "blocked" && false)` | 「derives steps from the readiness report…blocked step as blocked」 | 红 |
| `web/src/pages/instance/redirect.tsx:10` 目标前缀 `/instance` → `/admin` | 「sends /settings/instance to the console root」「sends /settings/instance/auth to /instance/auth」 | 红(依赖 to 显式路径的用例仍绿,证明映射确实是前缀派生) |

## 八、验证门禁(全绿)

```
unset http_proxy
cd web && pnpm exec tsc --noEmit   # 通过
pnpm test                          # 28 files / 101 tests 通过
pnpm i18n:audit                    # 通过(1154 静态 key 全解析;Go 菜单 11 id / 6 分组全翻译)
pnpm build                         # 通过(console 为独立 15 kB 懒加载 chunk)
```

`pnpm build` 会重写 `webembed/dist`(新生哈希文件);按 CLAUDE.md 未提交,已
`git checkout` + `git clean -fdx` 还原为 tracked 原状。

## 九、截图证据(主题证明 + 结构断言)

截图方式:先 `pnpm build` 重启后端(:8080,DB 在 /tmp 重建、长密钥、admin 邮箱化)与
vite dev(:5173,代理 :8080),用 `.agy-specs/shot.mjs`,`OCTARQ_SHOT_BASE=
http://localhost:5173`。shot.mjs 自带**登录失败检测**与每张图的
`document.documentElement.className` 打印。

- stateA(全新实例:无域名、无 SMTP、验证强制 → registration=**blocked**):
  - `.agy-specs/R4-instance-console-shots/stateA/instance-light.png` / `instance-dark.png`
    —— `/instance` 首屏,**含 blocked 项**
  - `.agy-specs/R4-instance-console-shots/stateA/instance-auth-light.png`(+dark)
    —— `/instance/auth`,wizard 的「某一步」(认证步骤页)
  - `.agy-specs/R4-instance-console-shots/stateA/admin-overview-light.png`(+dark)
    —— `/admin/overview`,证明租户主面板未被改坏
- stateB(插入 domains+smtp_senders 行,readiness 全 ok → 完成态平铺首页):
  - `.agy-specs/R4-instance-console-shots/stateB/instance-light.png`(+dark)
    —— 完成后的实例设置首页(健康摘要全绿)
  - `.agy-specs/R4-instance-console-shots/stateB/instance-settings-light.png`(+dark)
    —— 迁移过来的 Instance Settings 页
  - `.agy-specs/R4-instance-console-shots/stateB/instance-wizard-light.png`(+dark)
    —— wizard 的 Setup complete 态
- stateC(非实例管理员 user@example.com → 拒绝页):
  - `.agy-specs/R4-instance-console-shots/stateC/instance-light.png` / `instance-dark.png`
    —— 中性提示 + 返回 /admin 链接,**无可泄漏的实例功能**

主题证明(shot.mjs 输出):
```
stateA: login-light=""; instance-light=""; instance-auth-light=""; admin-overview-light=""
        login-dark="dark"; instance-dark="dark"; instance-auth-dark="dark"; admin-overview-dark="dark"
stateB: instance-light=""; instance-settings-light=""; instance-wizard-light=""
        instance-dark="dark"; instance-settings-dark="dark"; instance-wizard-dark="dark"
stateC: login-light=""; instance-light=""; login-dark="dark"; instance-dark="dark"
```
(「""」=light,「dark」=dark;错误主题会直接暴露。)

截图是 PNG,像素级人工判读在本环境不可用(当前模型不支持读图、multimodal-looker
无 API key),因此对每个状态在同一会话里做了**浏览器 DOM 结构断言**代替像素判读:

- stateA `/instance`:`[data-state="blocked"]`=1、degraded=2、ok=0;blocked 行有
  `h3:Registration`、「1 blocked item」横幅、`a[href="/admin/mail?tab=settings"]`;
  header 有 `a[href="/admin"]`、有 nav rail → **PASS**
- `/admin/overview` 无 "Instance Console",正文非空 → **PASS**
- stateB `/instance`:"All systems operational",blocked=0、degraded=0、ok=6、
  "Done"=6、无 "Open setup wizard";`/instance/wizard`:"Setup complete" → **PASS**
- stateC `/instance`:"isn't available to your account",无 nav rail、无 blocked、
  `a[href="/admin"]` 存在 → **PASS**
- 旧路径实测:`/admin/settings/instance` → 浏览器 URL 变为 `http://localhost:5173/instance`
  且渲染 "Instance Console" → **PASS**

"如果这个功能是坏的,这张图会不会拍出来?":blocked 项若未显著区分,stateA 断言
`blocked=1 && degraded=2` 会失败;完成态若没有变平铺首页,stateB 断言
`blocked=0 && w/o "Open setup wizard"` 会失败;门禁若泄漏功能,stateC 断言的
「无 rail/无 blocked」会失败——所有失败路径都被真实断言覆盖。

## 十、阻塞项(不改 Go,按规格标注)

- **后端尚未在 `/instance` 返回同一份 index.html**。核验
  `internal/server/server.go` 的路由(1.`/api/*`、2.`/admin/*`、2.25 `/status`、2.5
  插件 mounts、2.75 marketing shortcuts、3.`/`→redirect、4.rootFallback):
  `/instance` 不在任何分支 → 落到 rootFallback/404。与 `/status` 的地基批次
  (`// 2.25 Public status page`)应为同一性质的工作——缺的正是这么一段:
  ```go
  // 2.75 Instance console: same SPA, gated like /admin (mirror the /status branch).
  if path == "/instance" || strings.HasPrefix(path, "/instance/") {
      if !s.dashboardAllowed(r.Host) { http.NotFound(w, r); return }
      s.serveIndex(w); return
  }
  ```
  位置插在 `/status` 分支(server.go:131-138)之后、插件 mounts 之前即可。前端已在
  `/instance` basename、资产经 `/admin/assets/` 全部就绪;此路由落地后删除
  `vite.config.ts` 中间件里的 `/instance` 分支。
- 因此阶段截图/实测对 `/instance` 走的是与生产同机制的 vite dev(build 产物已验)。

## 十一、待决策

1. **Pro license 页进不进实例控制台**(见第五节):需 octarq-pro 侧协同——要么把
   licensing 菜单 category 指向控制台能认的分组,要么后端提供「菜单归属另一 Area」
   的机制;`UIPlugin.replaces` 不适用,未扩契约。
2. 后端 `/instance` SPA 路由(见阻塞项)由后续 Go 批次补齐;合并本分支后 `pnpm dev`
   仍可用(中间件),生产形态需该路由。
