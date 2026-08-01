Octarq provides full-lifecycle DNS management across your connected domain names. It pairs DNS provider API accounts (such as Cloudflare and DNSPod) with automated health verification for email (SPF, DMARC, DKIM) and short link host CNAME records.

## Key Capabilities

- **Multi-Provider API Integration**: Store credentials for DNS providers (Cloudflare, DNSPod) encrypted at rest using AES-GCM.
- **One-Click Domain Sync**: Synchronize hosted domains and zones directly from provider accounts into Octarq.
- **Full-Featured DNS Record CRUD**: Create, update, and delete A, AAAA, CNAME, TXT, MX, and NS records with custom TTLs and proxy settings (e.g., Cloudflare CDN proxy).
- **Preset Configurator**: Apply standard DNS presets with one click (such as Short Link CNAME targets, Mail MX records, or SPF policies).
- **Automated Health Verification**: Perform real-time posture checks for apex and subdomain records to verify SPF, DMARC, DKIM setup as well as link host CNAME target resolution.

## How to Use

### 1. Adding a DNS Provider Account
1. Go to **Domains -> Provider Accounts** (or **Settings -> Provider Accounts**).
2. Click **Add Provider Account**.
3. Select your provider type (e.g., `Cloudflare` or `DNSPod`) and enter your API Token.
4. Click **Save**.

### 2. Syncing & Managing Domains
1. Click **Sync Domains** to fetch all hosted zones from your configured DNS provider accounts.
2. Select any domain to inspect its active DNS records.
3. Add or edit records directly from the Octarq table. For Cloudflare domains, toggle the **Proxied** badge to enable or disable HTTP caching and SSL proxying.

### 3. Verifying Email & Link DNS Health
1. Open a domain from the **Domains** list and click **Verify DNS**.
2. Octarq performs live DNS queries for:
   - **SPF**: Checks for valid `v=spf1` TXT records.
   - **DMARC**: Validates `_dmarc` policy TXT records.
   - **DKIM**: Inspects configured DKIM selectors.
   - **Link CNAME**: Resolves CNAME records for short link hostnames to verify they point to the correct apex target.

:::tip
When adding short link hostnames or mailboxes under new subdomains, use the **Preset Configurator** in the record editor to generate recommended DNS record values automatically.
:::

## Why It Matters

Centralizing DNS management inside Octarq removes the context-switching of logging into external DNS registrar portals whenever you launch new short link subdomains, configure transactional mail, or verify authentication postures.

## Related Links

- [Dynamic DNS (DDNS)](/help/ddns) — Update dynamic IP addresses automatically via Dyndns2 protocol.
- [Short Links](/help/short-links) — Route short link subdomains to your workspace.
- [Mailboxes & Email Routing](/help/mailboxes) — Verify SPF/DKIM/DMARC for incoming and outgoing email.
