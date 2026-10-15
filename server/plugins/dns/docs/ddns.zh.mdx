---
title: 动态 DNS
description: 通过标准 Dyndns2 更新端点，自动将动态 IP 地址同步到 A 与 AAAA 解析记录。
category: infrastructure
order: 20
---

Octarq 动态 DNS (DDNS) 能将路由器、NAS 设备、家庭实验室及服务器上变动的公网 IP，自动同步到域名的 A（IPv4）与 AAAA（IPv6）解析记录中。每个 DDNS Token 都提供标准的 HTTP 更新地址，可无缝配合主流 DDNS 客户端与定时脚本使用。

## 创建 DDNS Token

1. 进入 **基础设施 → 网络 → DNS**，切换到 **动态 DNS** 标签页。
2. 点击 **新建 DDNS Token**。
3. 从下拉列表中选择已托管的目标域名。
4. 输入待解析的子域名称（例如 `home` 或 `vpn`）。
5. 选择记录解析类型：
   - **A**：针对 IPv4 地址。
   - **AAAA**：针对 IPv6 地址。
6. （可选）输入设备备注标签（例如 `家庭路由器`）。
7. 点击 **生成 Token**。
8. 立即复制保存生成的密钥与更新地址。

:::caution[密钥仅展示一次]
Octarq 会加密存储 Token，不会保留明文密钥。关闭弹窗后将无法再次查看该密钥。如果密钥丢失，请吊销该 Token 并创建新 Token。
:::

## 更新端点规范

向更新端点发送 HTTP GET 或 POST 请求，即可刷新 DNS 记录：

```
https://your-octarq-host.com/api/dns/ddns/update?token={token}
```

### 请求参数

| 参数名 | 类型 | 是否必选 | 描述 |
|---|---|---|---|
| `token` | String | 是 | 创建时生成的 DDNS 密钥 Token。 |
| `ip` | String | 可选 | 显式指定的 IPv4 或 IPv6 地址。留空时，Octarq 会自动获取客户端公网 IP。 |

### 响应结果

端点返回纯文本响应：

- `good <ip>`：DNS 记录已成功更新为新 IP 地址。
- `nochg <ip>`：提交的 IP 与现有记录一致，无需向服务商发送更新请求。
- `badauth`：鉴权失败。Token 缺失、已吊销或无效。
- `dnserr`：获取域名信息或在 DNS 服务商处更新记录时出错。

## 客户端配置

### 使用 curl 定时任务

在 Linux 或服务器中添加 Crontab 任务，每 10 分钟自动更新一次 IP 地址：

```bash
*/10 * * * * curl -s "https://your-octarq-host.com/api/dns/ddns/update?token=YOUR_SECRET_TOKEN" >> /var/log/ddns.log 2>&1
```

若需在双栈网络中更新 IPv6 记录，可使用 `curl -6`：

```bash
*/10 * * * * curl -6 -s "https://your-octarq-host.com/api/dns/ddns/update?token=YOUR_SECRET_TOKEN" >> /var/log/ddns-v6.log 2>&1
```

### OpenWrt 路由器

在 OpenWrt 的 LuCI 管理界面中：

1. 进入 **服务 → 动态 DNS**。
2. 添加一条新的 DDNS 配置：
   - **查询主机名 (Lookup host)**：`home.example.com`
   - **DDNS 服务提供商**：选择 `-- 自定义 --`
   - **自定义更新 URL**：`https://your-octarq-host.com/api/dns/ddns/update?token=[PASSWORD]&ip=[IP]`
   - **域名 (Domain)**：`home.example.com`
   - **密码 (Password)**：填入你的 `YOUR_SECRET_TOKEN`
   - **IP 地址来源**：选择 `网络接口` 或 `网络访问`
3. 点击 **保存并应用**。

### 群晖 Synology NAS

在群晖 DiskStation Manager (DSM) 控制面板中：

1. 进入 **控制面板 → 外部访问 → DDNS**。
2. 点击 **自定义** 登记新的服务供应商：
   - **服务供应商**：`Octarq`
   - **Query URL**：`https://your-octarq-host.com/api/dns/ddns/update?token=__PASSWORD__&ip=__MYIP__`
3. 点击 **新增** 添加配置：
   - **服务供应商**：选择刚才添加的 `Octarq`
   - **主机名称**：`home.example.com`
   - **使用者名称/电子邮件**：`token`
   - **密码/密钥**：填入你的 `YOUR_SECRET_TOKEN`
4. 点击 **测试联机**，确认状态显示为 **正常**。

### Linux ddclient

在运行 `ddclient` 的系统中，可在 `/etc/ddclient.conf` 中配置更新参数：

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

重启 `ddclient` 服务即可在后台持续自动同步。

## 吊销 Token

当设备更换或密钥不慎泄露时：

1. 进入 **基础设施 → 网络 → DNS**，切换到 **动态 DNS** 标签页。
2. 在列表中找到对应的 Token，点击 **吊销**。
3. 吊销操作即时生效。后续所有使用已吊销 Token 的更新请求均会返回 `badauth`。

## 相关指南

- [DNS 域名解析](/help/dns)
- [快速开始](/help/quickstart)
