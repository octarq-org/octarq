Octarq includes a full-featured URL shortener and link management system built directly into your platform. It allows you to create branded short links, track real-time engagement analytics, apply dynamic targeting rules, and automatically wrap links in outbound emails.

## Key Capabilities

- **Custom Domains & Slugs**: Bind short links to custom domain names (e.g., `l.example.com/promo`) or default apex domains.
- **Smart Target Routing**: Route visitors dynamically based on Country (GeoIP), Device type (Mobile/Desktop), OS (iOS/Android/macOS/Windows), or browser Language.
- **Access Controls & Expiration**: Protect links with passwords, set expiration timestamps (`ExpiresAt`), enforce total click limits (`ClickLimit`), and configure fallback redirect URLs (`ExpiredURL`).
- **Privacy-First Analytics**: Track unique clicks, total engagement over 7-day and 30-day windows, top performing links, referrer sources, and device/geographic distributions without tracking user PII.
- **Bot Detection & Filtering**: Distinguish human clicks from automated web crawlers and social media preview bots.
- **Automated Email Link Wrapping**: Automatically shorten external URLs in outgoing emails sent via the [Mailboxes](/help/mailboxes) module.

## How to Use

### 1. Creating a Short Link
1. Navigate to **Links** from the main navigation panel.
2. Click **Create Link** (or press `⌘K` / `Ctrl+K` and search for "Create Link").
3. Enter your destination **Target URL** (e.g., `https://my-app.com/launch-discount`).
4. (Optional) Customize the domain host, custom slug (e.g., `launch`), title, tags, or expiration date.
5. Click **Create**.

### 2. Configuring Dynamic Routing Rules
For links targeting global or multi-platform audiences, you can add conditional routing rules:
- **Geo Matching**: Redirect visitors from specific countries (e.g., `US` -> US landing page).
- **Device & OS Matching**: Direct mobile users (iOS or Android) directly to app store download links.
- **Language Matching**: Match browser preferred languages (e.g., `zh-CN` -> Chinese documentation).

### 3. Automatic Email Link Wrapping
Enable workspace-wide email link wrapping under **Settings -> Workspace -> Auto-shorten Links in Outgoing Mail**. When enabled, any external URL included in outgoing emails sent via Octarq is automatically converted into a tracked short link.

:::tip
Built-in system routes (such as `/admin`, `/api`, `/assets`, `/portal`) are protected reserved slugs and cannot be overwritten by short links.
:::

## Why It Matters

Using an embedded shortener eliminates dependencies on third-party SaaS services like Bitly or Dub. You retain complete ownership of click data, protect customer privacy, and reinforce brand authority with custom domain short links.

## Related Links

- [DNS Management](/help/dns) — Connect custom CNAME domains for short link hosting.
- [Mailboxes & Email Routing](/help/mailboxes) — Configure outbound transactional mail.
- [API Tokens](/help/api-tokens) — Programmatically create and manage short links via REST API.
