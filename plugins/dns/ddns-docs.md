Octarq includes a built-in Dynamic DNS (DDNS) server that allows home routers, server scripts, and edge devices on dynamic IP connections to automatically update domain A (IPv4) and AAAA (IPv6) DNS records using the standard Dyndns2 protocol.

## Key Capabilities

- **Dyndns2 Protocol Compliance**: Compatible with standard routers, NAS devices, and DDNS clients like `ddclient`, `inadyn`, OpenWrt, DD-WRT, and simple `curl` cron jobs.
- **Secure Token Management**: Each DDNS token generates a 48-character hex secret that is displayed once upon creation and stored securely as a SHA-256 hash.
- **Automatic Client IP Detection**: Updates automatically detect the client's public IP address via proxy headers (`X-Forwarded-For`, `X-Real-IP`) or socket address when the IP parameter is omitted.
- **IPv4 (A) & IPv6 (AAAA) Support**: Update single-stack IPv4 addresses, IPv6 addresses, or both.
- **Public Route Architecture**: The update endpoint (`/api/dns/ddns/update`) uses public route metadata, allowing automated background updates without maintaining an active user session cookie.

## How to Use

### 1. Creating a DDNS Token
1. Navigate to **Domains $\rightarrow$ Dynamic DNS (DDNS)** tab.
2. Click **New DDNS Token**.
3. Select your target domain, enter the FQDN record name (e.g., `home.example.com`), choose the record type (`A` or `AAAA`), and add an optional label (e.g., `Home Router`).
4. Click **Generate Token**.
5. **Copy your Token Secret and Update URL immediately.** The cleartext secret is displayed only once.

### 2. Updating via `curl` Cron Job
You can trigger DDNS updates from any Linux server, Raspberry Pi, or router using a simple `curl` command in a cron job:

```bash
# Automatic IP detection (uses caller's public IP)
curl -s "https://your-octarq.com/api/dns/ddns/update?hostname=home.example.com&key=YOUR_SECRET_TOKEN"

# Explicit IP specification
curl -s "https://your-octarq.com/api/dns/ddns/update?hostname=home.example.com&myip=1.2.3.4&key=YOUR_SECRET_TOKEN"
```

### 3. Dyndns2 Response Formats
The update endpoint returns standard text responses:
- `good 1.2.3.4`: DNS record was updated successfully to the new IP.
- `nochg 1.2.3.4`: The current IP matches the existing record; no update was required.
- `badauth`: Invalid or revoked DDNS token secret.
- `dnserr`: Target domain missing or upstream DNS provider error.

:::caution
For security, raw secret tokens are never stored in cleartext. If you lose your token secret, revoke the existing token and generate a new one.
:::

## Why It Matters

Octarq's DDNS engine turns any Cloudflare or DNSPod domain into a private dynamic DNS provider. You don't need third-party dynamic DNS services (like No-IP or DynDNS) or paid subscriptions to access self-hosted home servers and homelabs.

## Related Links

- [DNS Management](/help/dns) — Manage domain zones and DNS API keys.
- [API Tokens](/help/api-tokens) — Manage bearer tokens for administrative API access.
