# Autonomous Solopreneur Blueprint — 5-Minute Quickstart & Video Script

This guide provides both a **step-by-step hands-on tutorial** and a **ready-to-record video script / storyboard** for demonstrating the Autonomous Solopreneur Blueprint in action.

---

## 🎬 Part 1: Quickstart Video Script & Storyboard (2-Minute Demo)

Use this script to record a crisp product demo video for X (Twitter), YouTube, or Bilibili.

### [Scene 1: The Problem — 0:00 - 0:25]
- **Visual**: Screen recording showing 10 open browser tabs (Bitly, Gmail, Cloudflare, Stripe, terminal). The founder looks overwhelmed.
- **Voiceover**: 
  > "Running a one-person business means you're constantly context-switching between SaaS dashboards: making short links, waiting for OTP verification emails, and debugging DNS records. What if your AI assistant could run all of this directly from your terminal or IDE?"

### [Scene 2: One-Command Octarq Setup — 0:25 - 0:45]
- **Visual**: Split screen. Left side: Terminal runs `docker run -p 8080:8080 -v octarq-data:/data ghcr.io/octarq-org/octarq`. Right side: Octarq dashboard boots instantly.
- **Voiceover**: 
  > "Meet Octarq: a self-hosted single Go binary providing short links, custom-domain email, and DNS automation. And with native Model Context Protocol (MCP) support, every capability is instantly drivable by AI agents."

### [Scene 3: Connecting the Agent — 0:45 - 1:05]
- **Visual**: Typing `claude mcp add octarq -- /usr/local/bin/octarq mcp` into Claude Code terminal.
- **Voiceover**: 
  > "With a single command, we link Octarq to Claude Code or Cursor over MCP. Zero API boilerplate. Zero custom scrapers. Full tenant safety."

### [Scene 4: Live Magic — OTP & Campaign Links — 1:05 - 1:40]
- **Visual**: 
  1. User asks: *"Create a Product Hunt launch link for my new app with full UTM parameters."* Claude Code calls `create_shortlink` and prints the branded short URL.
  2. User asks: *"I just signed up on Stripe, what's my verification code?"* Claude Code calls `list_emails` and extracts: **`Your Stripe verification code is 839-204`**.
- **Voiceover**: 
  > "Need a campaign link? Just ask your agent in plain English. Registering on a new platform? Stop opening your inbox—your agent fetches the OTP code in two seconds."

### [Scene 5: Daily Standup & Wrap-up — 1:40 - 2:00]
- **Visual**: User types: *"Run morning operations standup."* Claude Code outputs a clean markdown briefing of yesterday's top link clicks and domain health.
- **Voiceover**: 
  > "The Autonomous Solopreneur Blueprint is 100% open source under the MIT license. Check out the blueprint in the Octarq repository and automate your back office today."

---

## 💻 Part 2: Hands-On Interactive Walkthrough

Follow these 4 simple steps to run the blueprint locally:

### Step 1: Launch Octarq

```bash
docker run -d --name octarq \
  -p 8080:8080 \
  -v octarq-data:/data \
  ghcr.io/octarq-org/octarq:latest
```

Open `http://localhost:8080` in your browser. Grab your initial password from the Docker logs:
```bash
docker logs octarq | grep "Initial password"
```

### Step 2: Configure MCP in Claude Code or Cursor

#### Claude Code (Terminal CLI):
```bash
# Connect via local stdio
claude mcp add octarq -- /usr/local/bin/octarq mcp
```

#### Cursor IDE:
Add to your project's `.cursor/mcp.json`:
```json
{
  "mcpServers": {
    "octarq": {
      "command": "/usr/local/bin/octarq",
      "args": ["mcp"]
    }
  }
}
```

### Step 3: Run Interactive Prompts

Try running these real-world prompts in your agent conversation:

#### 1. Generate a Branded Campaign Link
```text
@Octarq Create a shortlink for our blog post at https://mybrand.com/blog/ai-ops with slug 'ai-ops' and UTM source 'twitter'.
```

**Expected Agent Action**:
- Calls `create_shortlink`
- Output:
```text
✅ Created short link: https://go.mybrand.com/ai-ops
Target: https://mybrand.com/blog/ai-ops?utm_source=twitter
```

#### 2. Extract Transactional Verification Code (OTP)
```text
@Octarq Extract the verification code from the latest email received on auth@mybrand.com.
```

**Expected Agent Action**:
- Calls `list_emails(limit=5)`
- Parses newest email and outputs:
```text
🔑 Verification code: 749201 (from: noreply@github.com)
```

#### 3. Run Daily Operations Standup
```text
@Octarq Run morning operations standup and list top performing links.
```

**Expected Agent Action**:
- Calls `list_links`, `list_mailboxes`, and `list_domains`
- Synthesizes a structured morning standup markdown report.

---

## 🎯 Verification Checklist

- [x] Octarq binary / container responds on `http://localhost:8080`
- [x] MCP server handshake succeeds over stdio or SSE
- [x] Agent successfully lists shortlinks and creates a new link
- [x] Agent extracts OTP code from inbound email
- [x] Agent executes morning standup briefing
