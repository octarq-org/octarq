# 一人公司自运行智能体样板间 (Autonomous Solopreneur Blueprint)

> 面向独立开发者、一人创业者与小型 AI 原生团队的开箱即用运营样板间 —— 基于 [Octarq](https://github.com/octarq-org/octarq) 与模型上下文协议 (MCP) 构建。

[English Documentation (README.md)](./README.md) · [5 分钟快速启动与视频脚本 (quickstart-walkthrough.md)](./quickstart-walkthrough.md) · [Claude Code 运营手册 (CLAUDE.md)](./CLAUDE.md) · [Cursor 规则手册 (.cursorrules)](./.cursorrules)

---

## 🎯 业务背景与架构定位

以“一人公司”（Company of One）模式运作数字业务，创始人往往需要同时兼顾产品研发、市场推广、基础设施运维和客户服务。

**一人公司自运行智能体样板间**将 Octarq 转化为创始人的专属智能运维副驾驶。通过标准 MCP 协议，将 **Claude Code**、**Cursor** 或 **Claude Desktop** 等 AI 编码与运营智能体连接至你自建的 Octarq 实例，智能体即可获得以下业务抓手能力：

1. 🔗 **营销短链智能分流**：无需打开网页控制台，智能体即可一键生成带有 UTM 推广参数、点击上限与设备/地理重定向的品牌短链接。
2. 📬 **验证码与事务邮件自动提取**：在注册各类 SaaS 工具与第三方 API 账号时，智能体自动从入站邮箱读取最新事务邮件并提取 4-8 位验证码（OTP），彻底告别手动查收邮箱。
3. 🌐 **域名解析与邮件可达性巡检**：通过 DNS 插件自动化巡检 Zone 解析状态，核验 MX、SPF、DKIM 及 DMARC 记录，保障企业商务邮件不进垃圾箱。
4. 📊 **自动化每日运营晨报**：一条自然语言指令即可聚合全网短链点击趋势、高频来源、邮箱待办与系统健康度，生成结构化运营晨报。

```
                  ┌───────────────────────────────────────────────────────────┐
                  │                 一人公司自运行智能体                      │
                  │             (Claude Code / Cursor / 自动化脚本)           │
                  └─────────────────────────────┬─────────────────────────────┘
                                                │
                                    MCP 协议 (stdio / SSE)
                                                │
                  ┌─────────────────────────────▼─────────────────────────────┐
                  │                    Octarq 业务中枢                        │
                  │   ┌───────────────────┬─────────────────┬─────────────┐   │
                  │   │ 🔗 短链插件       │ ✉️ 邮件插件     │ 🌐 DNS 域名 │   │
                  │   │  (创建/聚合分析)  │ (验证码提取/收信)│(SPF/DMARC) │   │
                  │   └───────────────────┴─────────────────┴─────────────┘   │
                  └───────────────────────────────────────────────────────────┘
```

---

## 🚀 核心能力与工作流清单

### 1. 每日自主运营晨报 (`workflows/daily-standup.md`)
对你的 AI 智能体提问：*“生成今日运营晨报”*
- 调用 `list_links` 统计昨日点击激增链接与失效短链。
- 调用 `list_emails` 汇总最新入站客户邮件与咨询未读数。
- 调用 `list_domains` 验证所有托管域名的解析与邮件认证状态。
- 为创始人呈现清晰、结构化的业务指标摘要。

### 2. 事务验证码（OTP）秒级提取 (`workflows/otp-retrieval.md`)
对你的 AI 智能体提问：*“我刚用 auth@mybrand.com 注册了 Stripe / GitHub，帮我提取验证码”*
- 通过 `list_emails` 过滤指定邮箱或最新邮件。
- 智能匹配并提取 4-8 位数字或字母验证码。
- 直接输出至终端或 IDE 窗口，无缝贴入注册流程。

### 3. 推广短链快速编排 (`workflows/campaign-link.md`)
对你的 AI 智能体提问：*“帮我为 Product Hunt 发布生成一个跳转到 https://mybrand.com/app 的专属短链，附带完整 UTM”*
- 自动生成带自定义 Slug 和精准 UTM 参数（`utm_source=producthunt`、`utm_medium=launch`）的高转化短链。
- 按需配置访问密码或自动过期规则。

### 4. 域名与邮件可达性审计 (`workflows/dns-health-audit.md`)
对你的 AI 智能体提问：*“全面检查我工作区的域名 DNS 解析与发信评级”*
- 遍历 Cloudflare / DNSPod 解析列表。
- 验证 MX、SPF、DKIM 及 DMARC 记录完整性。

---

## 📦 目录结构全览

```text
blueprints/autonomous-solopreneur/
├── README.md                   # 英文说明文档
├── README_ZH.md                # 中文说明文档
├── CLAUDE.md                   # Claude Code 智能体运维操作规范
├── .cursorrules                # Cursor IDE 提示词规则与快捷指令
├── mcp.json                    # 开箱即用的 MCP 配置文件（支持 stdio 与 SSE）
├── .env.example                # 样板间环境变量参考
├── quickstart-walkthrough.md   # 5 分钟快速上手与操作演示脚本
├── workflows/
│   ├── daily-standup.md        # 每日运营晨报工作流
│   ├── otp-retrieval.md        # 验证码快速提取工作流
│   ├── campaign-link.md        # 营销短链编排工作流
│   └── dns-health-audit.md     # DNS 与邮件合规巡检工作流
└── scripts/
    ├── daily_briefing.sh       # 独立自动化晨报提取脚本
    └── verify_otp.sh           # 验证码提取辅助脚本
```

---

## ⚡ 5 分钟快速接入指引

### 第 1 步：启动 Octarq 实例

确保本地或云端 Octarq 实例已正常运行（Docker 或单二进制）：

```bash
docker run -d --name octarq -p 8080:8080 -v octarq-data:/data ghcr.io/octarq-org/octarq:latest
```

登录 Web 控制台（`http://localhost:8080`），在 **设置 → API 令牌** 中生成一个专用的 API Token。

### 第 2 步：配置智能体连接

#### 方式 A：Claude Code（终端命令行）

在工作目录中直接使用 Claude Code CLI 添加 MCP 服务端：

```bash
# 本地进程 stdio 通道
claude mcp add octarq -- /path/to/octarq mcp

# 或远程云端 SSE 通道
claude mcp add --transport sse octarq https://your-octarq-host.com/api/mcp/sse --header "Authorization: Bearer oct_your_api_token"
```

#### 方式 B：Cursor IDE

将 `.cursorrules` 置于项目根目录，并在 `.cursor/mcp.json` 中配置：

```json
{
  "mcpServers": {
    "octarq": {
      "url": "https://your-octarq-host.com/api/mcp/sse",
      "headers": {
        "Authorization": "Bearer oct_your_api_token"
      }
    }
  }
}
```

### 第 3 步：运行首个自运行工作流

在终端或 Cursor 对话框中输入：

```text
@Octarq 执行每日运营晨报，统计昨日短链点击量前 5 名。
```

智能体将自动调用 `list_links` 聚合统计，并输出结构化晨报！

---

## 🛡️ 安全基线与操作护栏

本样板间严格遵循 SaaS 生产级安全标准：

- **工作区租户隔离**：所有 MCP 请求强制携带身份上下文，严格限定在对应工作区内，杜绝跨租户越权。
- **杜绝裸 SQL**：服务端坚决不暴露通用 SQL 工具，所有能力均封装为经安全审计的领域模型方法。
- **高危写操作人工确认机制**：`CLAUDE.md` 与 `.cursorrules` 中已预埋指令护栏，对批量删除链接或变更生产 DNS 等破坏性行为强制提示用户确认。
- **凭据最小化暴露**：禁止在交互输出与版本库中回显明文 API Token。
