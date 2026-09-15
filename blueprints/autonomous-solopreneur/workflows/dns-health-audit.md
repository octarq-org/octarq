# Workflow: Domain & Deliverability Health Audit

## Objective
Proactively inspect DNS routing, SPF, DKIM, and DMARC configuration across all custom domains managed under Octarq to prevent email deliverability failures and broken link redirects.

## Trigger Prompt Examples
- *"Audit our domain DNS and email health"*
- *"Are all our custom domains properly verified?"*
- *"Check why my emails might be going to spam"*

## Execution Protocol

### Step 1: Query Hosted Domains
Call `list_domains` to retrieve the current registry of domains and their verification status.

### Step 2: Audit Core Records
For each domain:
1. **Shortlink Routing**: Confirm A / CNAME records point correctly to the Octarq server or reverse proxy.
2. **Inbound Mail**: Confirm MX records (e.g. Cloudflare Email Routing or direct MX) are verified.
3. **Outbound Deliverability**:
   - **SPF**: Verify `TXT` record includes `v=spf1 ... ~all`.
   - **DKIM**: Verify DKIM public key record exists.
   - **DMARC**: Verify `_dmarc` TXT record with appropriate policy (`p=none` or `p=quarantine`).

### Step 3: Deliver Audit Report

```markdown
### 🌐 Domain & Email Deliverability Audit Report

| Domain | DNS Status | MX (Inbound) | SPF (Sender Auth) | DKIM / DMARC | Action Needed |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `mybrand.com` | ✅ Active | ✅ Cloudflare | ✅ Configured | ✅ Verified | None |
| `go.mybrand.com` | ✅ Active | N/A (CNAME) | N/A | N/A | None |
| `altmail.org` | ⚠️ Pending | ❌ Missing MX | ❌ Missing SPF | ⚠️ No DMARC | Add DNS Records below |

#### 🔧 Required DNS Configurations:
If any records are missing, provide copy-pasteable BIND / DNS entries:
```text
Type: TXT
Name: @
Content: "v=spf1 include:_spf.mx.cloudflare.net ~all"

Type: TXT
Name: _dmarc
Content: "v=DMARC1; p=quarantine; rua=mailto:dmarc-reports@mybrand.com"
```
