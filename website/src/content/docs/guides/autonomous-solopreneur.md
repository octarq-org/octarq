---
title: Autonomous Solopreneur Blueprint
description: Turn Octarq into an autonomous operations co-pilot with Claude Code, Cursor, and MCP.
sidebar:
  order: 4
  group:
    label: "Guides"
---

The **Autonomous Solopreneur Blueprint** is an official operational recipe that turns Octarq into an automated back-office co-pilot for indie hackers, solo founders, and one-person digital businesses ("Company of One").

By pairing Octarq's built-in **Model Context Protocol (MCP)** server with AI agents like **Claude Code** and **Cursor**, your agent can autonomously orchestrate marketing short links, extract OTP verification codes from transactional emails, audit DNS health, and deliver daily business standup reports.

---

## Architecture

Instead of juggling multiple disjointed SaaS tools, your agent communicates with your self-hosted Octarq instance over standard MCP (stdio or SSE):

```
┌─────────────────────────────────────────────────────────────────┐
│                   Autonomous Solopreneur Agent                  │
│                (Claude Code / Cursor / CLI Runner)              │
└───────────────────────────────┬─────────────────────────────────┘
                                │
                    MCP Protocol (stdio / SSE)
                                │
┌───────────────────────────────▼─────────────────────────────────┐
│                      Octarq Operations Hub                      │
│   ┌─────────────────────┬───────────────────┬───────────────┐   │
│   │ 🔗 Links Plugin     │ ✉️ Mail Plugin    │ 🌐 DNS Zone   │   │
│   │  (create/analytics) │  (OTP/inbox)      │  (SPF/DMARC)  │   │
│   └─────────────────────┴───────────────────┴───────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

---

## Key Workflows

### 1. Daily Autonomous Standup
Ask your AI assistant:
> *"Run morning operations standup"*

The agent queries:
- `list_links`: Identifies traffic spikes, top performing campaigns, and dead links.
- `list_emails`: Summarizes unread transactional notifications and customer inquiries.
- `list_domains`: Verifies DNS zone health across all custom domains.

### 2. Transactional OTP & Verification Code Extraction
During onboarding or API credential setup across third-party services:
> *"I just requested a verification code from GitHub / Stripe, what is the code?"*

The agent retrieves the latest inbound transactional email via `list_emails` and extracts the 4-8 digit OTP code in seconds, keeping you in flow.

### 3. Campaign Link Orchestration
Quickly spin up branded shortlinks with full UTM tracking:
> *"Create a Product Hunt launch link pointing to https://mybrand.com/app with UTM parameters and slug 'ph-launch'"*

The agent validates the URL, appends canonical UTM tags, and invokes `create_shortlink`.

### 4. Domain & Deliverability Audit
Ensure your emails never hit spam:
> *"Audit our domain DNS and email deliverability health"*

The agent checks MX routing, SPF records (`v=spf1 ...`), DKIM, and DMARC policies.

---

## 5-Minute Setup

### 1. Ensure Octarq is Running

Run Octarq locally or in the cloud via Docker:

```bash
docker run -d --name octarq -p 8080:8080 -v octarq-data:/data ghcr.io/octarq-org/octarq:latest
```

### 2. Connect Your Agent

#### Claude Code (Terminal CLI)
```bash
# Connect via local stdio
claude mcp add octarq -- /usr/local/bin/octarq mcp

# Or connect via remote SSE
claude mcp add --transport sse octarq https://your-octarq-host.com/api/mcp/sse --header "Authorization: Bearer oct_your_api_token"
```

#### Cursor IDE
Add to your project's `.cursor/mcp.json`:
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

---

## Blueprint Assets & Templates

The complete blueprint starter kit is located in the GitHub repository at [`blueprints/autonomous-solopreneur/`](https://github.com/octarq-org/octarq/tree/main/blueprints/autonomous-solopreneur):

- **`CLAUDE.md`**: Pre-engineered SOPs and instructions for Claude Code.
- **`.cursorrules`**: Prompt macros and operational rules for Cursor.
- **`workflows/`**: Markdown prompt templates for daily standups, OTP retrieval, and campaigns.
- **`scripts/`**: Shell scripts for automated daily briefing extraction and verification helper.
- **`quickstart-walkthrough.md`**: Visual walkthrough and 2-minute video demo script.
