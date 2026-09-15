# Autonomous Solopreneur Blueprint (一人公司自运行智能体)

> Turnkey AI-native operations blueprint for indie makers, solo founders, and one-person digital businesses — powered by [Octarq](https://github.com/octarq-org/octarq) and the Model Context Protocol (MCP).

[简体中文文档 (README_ZH.md)](./README_ZH.md) · [Quickstart Walkthrough & Script](./quickstart-walkthrough.md) · [Claude Code Guide (CLAUDE.md)](./CLAUDE.md) · [Cursor Rules (.cursorrules)](./.cursorrules)

---

## 🎯 Overview

Running a digital business as a solo founder ("Company of One") requires wearing ten hats: developer, marketer, sysadmin, and support agent. 

The **Autonomous Solopreneur Blueprint** transforms Octarq into an autonomous operations co-pilot. By connecting your favorite AI coding and operational assistants (**Claude Code**, **Cursor**, or **Claude Desktop**) to your self-hosted Octarq instance via MCP, your AI assistant can directly:

1. 🔗 **Generate High-Converting Short Links**: Provision branded shortlinks with UTM campaign parameters, expiration rules, and device/geo targeting without touching the dashboard.
2. 📬 **Extract OTP & Verification Codes**: Automatically retrieve login verification codes and OTP tokens from inbound transaction emails during SaaS onboarding and API registrations.
3. 🌐 **Audit DNS & Mail Deliverability**: Inspect DNS zones, MX records, SPF, DKIM, and DMARC health status to ensure business emails never land in spam.
4. 📊 **Deliver Daily Operations Standups**: Summarize daily traffic trends, top-performing marketing links, mailbox activity, and system health in a single natural language command.

```
                  ┌───────────────────────────────────────────────────────────┐
                  │                 Autonomous Solopreneur Agent              │
                  │              (Claude Code / Cursor / CLI Bot)             │
                  └─────────────────────────────┬─────────────────────────────┘
                                                │
                                    MCP Protocol (stdio / SSE)
                                                │
                  ┌─────────────────────────────▼─────────────────────────────┐
                  │                 Octarq Operations Hub                     │
                  │   ┌───────────────────┬─────────────────┬─────────────┐   │
                  │   │ 🔗 Links Plugin   │ ✉️ Mail Plugin  │ 🌐 DNS Zone │   │
                  │   │  (create/search)  │  (OTP/inbox)    │ (SPF/DMARC) │   │
                  │   └───────────────────┴─────────────────┴─────────────┘   │
                  └───────────────────────────────────────────────────────────┘
```

---

## 🚀 Key Capabilities & Workflows

### 1. Daily Autonomous Standup (`workflows/daily-standup.md`)
Ask your AI agent: *"Run morning operations standup"*
- Queries `list_links` to identify yesterday's click spikes and dead links.
- Queries `list_emails` to check unread incoming customer inquiries.
- Queries `list_domains` to ensure no DNS degradation has occurred.
- Formats an actionable briefing for the founder.

### 2. Instant OTP / Verification Code Retrieval (`workflows/otp-retrieval.md`)
Ask your AI agent: *"I just signed up on Stripe / GitHub using auth@mybrand.com. What is the verification code?"*
- Reads recent inbound emails via `list_emails`.
- Identifies the newest verification email and extracts the 4-8 digit numeric or alphanumeric OTP token.
- Returns the exact code directly into your terminal or IDE prompt.

### 3. Campaign Link Orchestration (`workflows/campaign-link.md`)
Ask your AI agent: *"Create a launch link for our Product Hunt post pointing to https://mybrand.com/app with UTM tags"*
- Generates a branded shortlink with custom slug and UTM parameters (`utm_source=producthunt`, `utm_medium=launch`).
- Sets click expiry or password protection if requested.

### 4. DNS & Deliverability Audit (`workflows/dns-health-audit.md`)
Ask your AI agent: *"Check our domain DNS and email deliverability health"*
- Inspects Cloudflare / DNSPod domain configuration via `list_domains`.
- Verifies MX, SPF, DKIM, and DMARC DNS records.

---

## 📦 Directory Structure

```text
blueprints/autonomous-solopreneur/
├── README.md                   # English blueprint manual
├── README_ZH.md                # Chinese translation (遵循 i18n 规范)
├── CLAUDE.md                   # Claude Code agent operations handbook
├── .cursorrules                # Cursor IDE configuration & quick prompt actions
├── mcp.json                    # Ready-to-copy MCP configuration (stdio & SSE)
├── .env.example                # Blueprint environment variables
├── quickstart-walkthrough.md   # 5-minute visual & video-script walkthrough
├── workflows/
│   ├── daily-standup.md        # Morning business briefing workflow
│   ├── otp-retrieval.md        # Automated OTP code extraction workflow
│   ├── campaign-link.md        # Marketing shortlink campaign workflow
│   └── dns-health-audit.md     # DNS & mail deliverability audit workflow
└── scripts/
    ├── daily_briefing.sh       # Automated standalone briefing script
    └── verify_otp.sh           # CLI helper to fetch latest OTP
```

---

## ⚡ 5-Minute Quick Setup

### Step 1: Start Octarq

Ensure your Octarq instance is running (via Docker or native binary):

```bash
docker run -d --name octarq -p 8080:8080 -v octarq-data:/data ghcr.io/octarq-org/octarq:latest
```

Log in at `http://localhost:8080` and grab an **API Token** from **Settings → API Tokens** (or use local stdio if running on the same host).

### Step 2: Configure Your Agent

#### Option A: Claude Code (Terminal CLI)

In your working directory or project root, copy `CLAUDE.md` and link the MCP server:

```bash
# Add stdio MCP (local instance)
claude mcp add octarq -- /path/to/octarq mcp

# OR add remote SSE MCP (hosted instance)
claude mcp add --transport sse octarq https://your-octarq-host.com/api/mcp/sse --header "Authorization: Bearer oct_your_api_token"
```

#### Option B: Cursor IDE

Copy `.cursorrules` to your project root. In `.cursor/mcp.json`:

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

### Step 3: Run Your First Autonomous Workflow

Open your terminal or IDE prompt and trigger:

```text
@Octarq Run morning operations standup and list yesterday's top link traffic.
```

Your agent will invoke `list_links`, compute metrics, and present your business status report!

---

## 🛡️ Security & Operational Guardrails

This blueprint enforces strict SaaS and operational security standards:

- **Strict Tenant Isolation**: All MCP operations are scoped to your API token's workspace.
- **Zero Raw SQL**: The MCP interface contains no raw query tools, preventing data leaks or accidental database corruption.
- **Human-in-the-Loop for Destructive Actions**: The agent is configured in `CLAUDE.md` and `.cursorrules` to request human confirmation before modifying production DNS or deleting critical short links.
- **Credential Protection**: Secrets and API tokens are never exposed in terminal outputs or commit logs.
