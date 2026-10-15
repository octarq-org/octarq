---
title: DNS
description: Connect DNS providers, manage records, verify mail and link setup, and apply email blueprints.
category: infrastructure
order: 10
---

Octarq DNS connects to your DNS providers to manage domain zones and records. You can sync domains from Cloudflare and DNSPod, edit DNS records directly, verify email and link routing, and apply email setup blueprints with one click.

## Connect a DNS Provider

1. Go to **Infrastructure → Network → DNS** and click the **Settings** tab.
2. Click **Connect a DNS Provider** and select **Cloudflare** or **DNSPod**.
3. Enter your provider API credentials:
   - **Cloudflare**: Enter an API token with `Zone:Read` and `DNS:Edit` permissions.
   - **DNSPod**: Enter your API ID and Token pair.
4. Click **Save**. Octarq encrypts your credentials at rest.
5. Connected provider accounts are scoped to your workspace and cannot be accessed by other workspaces.

## Sync Domains and Manage Records

1. On the **DNS** tab, click **Sync Domains** to import hosted zones from your connected provider accounts.
2. Select a domain from the list to view and manage its DNS records.
3. Add or update records using standard types:
   - **A** and **AAAA**: Point hostnames to IPv4 or IPv6 addresses (for example, `192.0.2.1` or `2001:db8::1`).
   - **CNAME**: Alias a subdomain to another domain (for example, `go.example.com` to `octarq.example.com`).
   - **TXT**: Add verification tokens, SPF rules, or DKIM keys.
   - **MX**: Route incoming email with priority values (for example, priority `10` pointing to `mail.example.com`).
   - **NS**: Delegate subdomains to dedicated nameservers (for example, `ns1.example.com`).
4. (Optional) For Cloudflare domains, switch the **Proxied** toggle (orange cloud) to route traffic through Cloudflare's CDN and DDoS protection or directly to the origin.

:::tip[DNSSEC Management]
DNSSEC signing and DS records are managed directly in your DNS provider's dashboard (such as Cloudflare or DNSPod) or domain registrar.
:::

## Verify DNS Records

1. Select a domain and click **Verify DNS**.
2. Octarq queries public resolvers to check key records:
   - **SPF**: Checks for a valid `v=spf1` TXT record at the root domain.
   - **DMARC**: Checks for a policy record at `_dmarc.{domain}` (for example, `v=DMARC1; p=none;`).
   - **DKIM**: Probes standard selectors (`default`, `octarq`, `google`, `mail`, `k1`, `sig1`) for published public keys.
   - **Link CNAME**: Verifies that custom short-link subdomains resolve to your Octarq instance.

:::note[Cloudflare Proxying and CNAME Verification]
When proxying is enabled on a short-link CNAME, Cloudflare flattens the response to its edge IP addresses. The verification badge may show Unverified, but short-link redirection continues to work normally.
:::

## Apply an Email Blueprint

1. Select a domain and click **Email Blueprint**.
2. Review the recommended records and their live status:
   - **MX**: `route1.mx.cloudflare.net` (priority `10`) and `route2.mx.cloudflare.net` (priority `53`)
   - **SPF**: TXT record `v=spf1 include:_spf.mx.cloudflare.net ~all`
   - **DMARC**: TXT record `_dmarc` with `v=DMARC1; p=none; sp=none;`
3. Check the status of each record: **OK** (matches live record), **Missing** (absent), or **Mismatch** (content differs).
4. Click **Apply Blueprint**. Octarq creates missing records and updates mismatches in your provider account without changing healthy existing records.

## Protect Reserved Hostnames

To protect shared routing and avoid collisions between workspaces:

- Hostnames matching the instance's base domain or its subdomains are reserved for automatic workspace routing.
- Workspaces cannot claim or register reserved system hostnames.
- If you enter a hostname that is reserved, Octarq rejects the change and prompts you to use a different subdomain.

## Related Guides

- [Dynamic DNS (DDNS)](/help/ddns)
- [Short Links](/help/short-links)
- [Mailboxes](/help/mailboxes)
- [Quickstart](/help/quickstart)
