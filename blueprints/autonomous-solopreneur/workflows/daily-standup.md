# Workflow: Daily Autonomous Standup

## Objective
Provide the solo founder with an automated, high-signal operational briefing every morning without having to open multiple dashboards.

## Trigger Prompt Examples
- *"Run morning operations standup"*
- *"What happened in Octarq yesterday?"*
- *"Give me our daily business metrics briefing"*

## Execution Protocol

### Step 1: Query Marketing & Traffic Performance
Call `list_links` with `limit=50`:
- Sort links by click counts (`clicks`).
- Calculate total clicks across all active links.
- Identify the top 3 performing links and any recently created links with sudden traction.

### Step 2: Query Transactional Inboxes & Support
Call `list_mailboxes`:
- Note total mailboxes configured and unread counts.
Call `list_emails` with `limit=10, unreadOnly=true`:
- Identify any unread customer emails, payment notifications, or urgent transactional alerts.

### Step 3: Verify Infrastructure Status
Call `list_domains`:
- Check DNS validation status and SPF/DKIM records across active domains.

### Step 4: Output Synthesis Template

Generate a structured response following this template:

```markdown
### ☕ Autonomous Operations Standup — [Current Date]

#### 📈 Marketing & Traffic
- **Total Tracked Links**: [Count]
- **Top Performing Campaigns**:
  1. `[slug]` ([destination]) — **[clicks]** clicks
  2. `[slug]` ([destination]) — **[clicks]** clicks
  3. `[slug]` ([destination]) — **[clicks]** clicks
- **Growth Velocity**: [Brief observation on click trends]

#### 📬 Inbound Mail & Notifications
- **Active Mailboxes**: [Count] ([Total Unread] unread messages)
- **Priority Attention Items**:
  - [Sender / Subject] ([Time received])

#### 🌐 Domain & Deliverability
- **Monitored Domains**: [Count]
- **Status**: All zones healthy (SPF/DKIM/DMARC verified)

#### 🎯 Suggested Next Actions
- [Action 1: e.g. Extend campaign link expiry for launch promotion]
- [Action 2: e.g. Reply to customer inquiry on auth@yourbrand.com]
```
