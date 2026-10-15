---
title: Dynamic DNS
description: Keep A and AAAA DNS records synchronized with dynamic IP addresses using standard Dyndns2 update endpoints.
category: infrastructure
order: 20
---

Octarq Dynamic DNS (DDNS) keeps your domain's A (IPv4) and AAAA (IPv6) records synchronized with changing IP addresses on routers, NAS devices, homelabs, and servers. Each DDNS token provides a standard HTTP update URL that works with popular DDNS clients and scheduled scripts.

## Create a DDNS Token

1. Go to **Infrastructure → Network → DNS** and open the **Dynamic DNS** tab.
2. Click **New DDNS Token**.
3. Select your managed domain from the dropdown.
4. Enter the subdomain record name (for example, `home` or `vpn`).
5. Choose the record type:
   - **A**: For IPv4 addresses.
   - **AAAA**: For IPv6 addresses.
6. (Optional) Enter a label to identify the device (for example, `Home Router`).
7. Click **Generate Token**.
8. Copy your secret key and update URL immediately.

:::caution[Secret Key Shown Once]
Octarq encrypts tokens and does not store the cleartext secret. You cannot view the secret key after closing the dialog. If you lose the key, revoke the token and create a new one.
:::

## Update Endpoint

Send an HTTP GET or POST request to the update endpoint to refresh the DNS record:

```
https://your-octarq-host.com/api/dns/ddns/update?token={token}
```

### Query Parameters

| Parameter | Type | Required | Description |
|---|---|---|---|
| `token` | String | Yes | Your secret DDNS token key. |
| `ip` | String | Optional | Explicit IPv4 or IPv6 address. If omitted, Octarq detects your client's public IP automatically. |

### Responses

The endpoint returns plain text responses:

- `good <ip>`: The DNS record was successfully updated to the new IP address.
- `nochg <ip>`: The submitted IP matches the existing DNS record. No provider update was needed.
- `badauth`: Authentication failed. The token is missing, revoked, or invalid.
- `dnserr`: An error occurred while resolving the domain or updating records at the DNS provider.

## Configure Clients

### Scheduled Cron with curl

Add a cron job to update your IP address automatically every 10 minutes:

```bash
*/10 * * * * curl -s "https://your-octarq-host.com/api/dns/ddns/update?token=YOUR_SECRET_TOKEN" >> /var/log/ddns.log 2>&1
```

To update an IPv6 record from a dual-stack host, use `curl -6`:

```bash
*/10 * * * * curl -6 -s "https://your-octarq-host.com/api/dns/ddns/update?token=YOUR_SECRET_TOKEN" >> /var/log/ddns-v6.log 2>&1
```

### OpenWrt Router

In the OpenWrt LuCI web interface:

1. Go to **Services → Dynamic DNS**.
2. Add a new DDNS service configuration:
   - **Lookup host**: `home.example.com`
   - **DDNS Service provider**: `-- custom --`
   - **Custom update-URL**: `https://your-octarq-host.com/api/dns/ddns/update?token=[PASSWORD]&ip=[IP]`
   - **Domain**: `home.example.com`
   - **Password**: `YOUR_SECRET_TOKEN`
   - **IP address source**: `network` or `web`
3. Click **Save & Apply**.

### Synology NAS

In Synology DiskStation Manager (DSM):

1. Go to **Control Panel → External Access → DDNS**.
2. Click **Customize** to register a new provider:
   - **Service Provider**: `Octarq`
   - **Query URL**: `https://your-octarq-host.com/api/dns/ddns/update?token=__PASSWORD__&ip=__MYIP__`
3. Click **Add**:
   - **Service Provider**: Select `Octarq`
   - **Hostname**: `home.example.com`
   - **Username/Email**: `token`
   - **Password/Key**: `YOUR_SECRET_TOKEN`
4. Click **Test Connection** to confirm that the status reports **Normal**.

### Linux ddclient

For systems running `ddclient`, configure `/etc/ddclient.conf` with a custom script or update URL:

```ini
daemon=300
syslog=yes
ssl=yes
protocol=dyndns2
use=web, web=checkip.dyndns.com
server=your-octarq-host.com
login=token
password=YOUR_SECRET_TOKEN
home.example.com
```

Restart `ddclient` to enable background updates.

## Revoke a Token

If a device is replaced or a token is exposed:

1. Go to **Infrastructure → Network → DNS** and open the **Dynamic DNS** tab.
2. Locate the token in the list and click **Revoke**.
3. Revocation takes effect immediately. Subsequent update requests using the revoked token receive `badauth`.

## Related Guides

- [DNS Management](/help/dns)
- [Quickstart](/help/quickstart)
