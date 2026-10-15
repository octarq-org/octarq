---
title: DNS 域名解析
description: 绑定 DNS 服务商、管理解析记录、验证邮件与短链配置，并一键应用邮件蓝图。
category: infrastructure
order: 10
---

Octarq DNS 连接你的 DNS 服务商，统一管理域名与解析记录。你可以从 Cloudflare 和 DNSPod 同步域名、直接编辑 DNS 记录、验证邮件与短链解析，并一键应用邮件配置蓝图。

## 绑定 DNS 服务商

1. 进入 **基础设施 → 网络 → DNS**，切换到 **设置** 标签页。
2. 点击 **连接 DNS 服务商**，选择 **Cloudflare** 或 **DNSPod**。
3. 输入服务商 API 凭证：
   - **Cloudflare**：输入具有 `Zone:Read` 和 `DNS:Edit` 权限的 API Token。
   - **DNSPod**：输入控制台颁发的 API ID 与 Token 密钥对。
4. 点击 **保存**。Octarq 会在存储时对凭据进行加密。
5. 已绑定的服务商账户仅归当前工作区所有，其他工作区无法访问。

## 同步域名与管理解析记录

1. 在 **DNS** 标签页中点击 **同步域名**，从已绑定的服务商账户中导入托管域名。
2. 在左侧列表中选择域名，即可查看并管理其全部 DNS 记录。
3. 添加或更新常用类型的记录：
   - **A 与 AAAA**：将主机名指向 IPv4 或 IPv6 地址（例如 `192.0.2.1` 或 `2001:db8::1`）。
   - **CNAME**：将子域名别名指向另一个域名（例如将 `go.example.com` 指向 `octarq.example.com`）。
   - **TXT**：保存站点验证、SPF 策略或 DKIM 公钥。
   - **MX**：配置带有优先级的邮件路由地址（例如优先级 `10` 指向 `mail.example.com`）。
   - **NS**：将子域委派给专门的域名服务器（例如 `ns1.example.com`）。
4. （可选）针对 Cloudflare 托管的域名，切换 **Proxied** 代理开关（橙色云朵）控制流量是否经过 Cloudflare CDN 与 DDoS 防护，或直接导向源站。

:::tip[DNSSEC 管理]
DNSSEC 签名与 DS 记录由绑定的 DNS 服务商（如 Cloudflare 或 DNSPod）或域名注册商直接管理，需在对应服务商控制台中配置。
:::

## 检查 DNS 记录

1. 选择目标域名，点击 **校验 DNS**。
2. Octarq 会向公网解析器发起查询，检验关键记录：
   - **SPF**：检查域名根节点下是否存在有效的 `v=spf1` TXT 记录。
   - **DMARC**：检查 `_dmarc.{domain}` 下是否存在策略记录（例如 `v=DMARC1; p=none;`）。
   - **DKIM**：探测常用选择器（`default`、`octarq`、`google`、`mail`、`k1`、`sig1`）下的公钥发布状态。
   - **短链 CNAME**：检查分配给短链的自定义子域是否正确解析到当前 Octarq 实例。

:::note[Cloudflare 代理与 CNAME 校验]
当短链 CNAME 开启 Cloudflare 代理（橙色云朵）后，Cloudflare 会在响应中将别名展平为边缘节点 IP 地址。状态徽标可能显示为未验证，但这属于正常边缘行为，短链跳转不受影响。
:::

## 一键应用邮件蓝图

1. 选择目标域名，点击 **邮件蓝图**。
2. 查看推荐的邮件解析记录与当前比对状态：
   - **MX 记录**：`route1.mx.cloudflare.net`（优先级 `10`）与 `route2.mx.cloudflare.net`（优先级 `53`）
   - **SPF 记录**：TXT 记录 `v=spf1 include:_spf.mx.cloudflare.net ~all`
   - **DMARC 记录**：`_dmarc` 的 TXT 记录 `v=DMARC1; p=none; sp=none;`
3. 检查每项记录的状态标示：**正常**（已正确配置）、**缺失**（尚未添加）或 **配置冲突**（内容不一致）。
4. 点击 **应用蓝图**。Octarq 会自动在服务商处补全缺失记录并纠正冲突，不会更改现有的正常记录。

## 保留域名保护

为了保护共享路由并防止工作区之间的域名冲突：

- 与平台基础域名或其子域匹配的主机名专用于系统分配的工作区路由。
- 租户工作区不能登记或抢注系统保留的主机名。
- 如果添加的域名属于保留范围，Octarq 会拒绝操作并提示使用其他子域。

## 相关指南

- [动态 DNS (DDNS)](/help/ddns)
- [短链](/help/short-links)
- [邮箱](/help/mailboxes)
- [快速开始](/help/quickstart)
