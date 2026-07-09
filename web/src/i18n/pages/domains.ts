export const domains = {
  en: {
    // Page header
    pageTitle: "DNS",
    pageDescription: "Sync & manage DNS records across Cloudflare and DNSPod",
    syncCloudflare: "Sync Cloudflare",
    addDomain: "Add Domain",

    // Tabs
    tabDns: "DNS",
    tabSettings: "Settings",

    // Domain list
    searchDomains: "Search domains…",
    noDomainsFound: "No domains found.",
    toggleLinkRouting: "Toggle Link routing",
    toggleMailRouting: "Toggle Mail routing",
    loading: "Loading…",

    // Domain editor / detail
    addDomainZone: "Add Domain Zone",
    removeConfirm: "Remove {{name}}?",
    delete: "Delete",
    managedHosts: "Managed Hosts",

    // DNS verification
    dnsSetupVerification: "DNS Setup Verification",
    verifying: "Verifying...",
    verifyDnsSetup: "Verify DNS Setup",
    verifyFailed: "Failed to verify DNS setup",
    verificationHint: "Check that each mail host's SPF, DKIM and DMARC records are set for email delivery, and that each short-link host resolves to this app.",
    statusLabel: "{{label}} Status",
    unknown: "Unknown",
    mailHosts: "Mail hosts",
    shortLinkHosts: "Short-link hosts",

    dnsRecords: "DNS Records",

    // Empty states
    addFirstDomain: "Add your first domain",
    addFirstDomainHint: "Import every zone from a connected DNS provider in one step, or add a domain manually.",
    syncFrom: "Sync from {{name}}",
    provider: "provider",
    connectProvider: "Connect a DNS provider",
    orAddManually: "or add a domain manually",
    selectDomainHint: "Select a domain from the sidebar list to manage DNS & routing.",

    // DnsStatusBadge
    configured: "✓ Configured",
    misconfigured: "! Misconfigured",
    missing: "✗ Missing",

    // LinkHostRow
    pointsToZone: "✓ Points to zone",
    unverified: "! Unverified",
    notResolving: "✗ Not resolving",
    resolvesNoCname: "resolves, but no CNAME into {{target}}",
    addCname: "add a CNAME → {{target}}",

    // LinkHostGuide
    guideTitle: "How to point a short-link host at this app",
    guideIntro: "needs a DNS record so visitors reach this server:",
    guideEachHost: "Each short-link host (e.g. ",
    guideCnameTo: "the link host to",
    guideCnameRec: "(recommended). The apex itself must already resolve to this server's IP.",
    guideOrAdd: "Or add an",
    guideAaaaRec: "record pointing straight at this server's IP.",
    guideProxiedIntro: "On Cloudflare you can keep the record",
    guideProxiedWord: "proxied",
    guideProxiedMid: "(orange cloud) for TLS & caching — verification will then show",
    guideUnverifiedWord: "Unverified",
    guideProxiedEnd: "since the CNAME is flattened, which is expected.",
    guideTipIntro: "Tip: use the",
    guideSubdomainPreset: "+ Subdomain",
    guideTipEnd: "preset in DNS Records → “Set Link CNAME” to create this record automatically.",

    // DomainEditorForm
    domainName: "Domain Name",
    dnsProviderConnection: "DNS Provider Connection",
    noAccountsAvailable: "No accounts available",
    zoneIdentifier: "Zone ID",
    zoneIdPlaceholder: "Auto-discovered if using Cloudflare",
    internalAdminNote: "Internal Admin Note",
    notePlaceholder: "Optional note for team members",
    shortLinkSubdomains: "Short-link Routing Subdomains",
    noShortlinkSubdomains: "No shortlink subdomains.",
    inboundMailSubdomains: "Inbound Mail Routing Subdomains",
    noMailboxSubdomains: "No mailbox subdomains.",
    cancel: "Cancel",
    saving: "Saving...",
    saveBasicInfo: "Save Basic Info",
    saveFailed: "save failed",

    // DomainHostManager
    noActiveHosts: "No active hosts registered. Add one below.",
    thHost: "Host",
    thLink: "🔗 Link",
    thMail: "✉️ Mail",
    on: "on",
    off: "off",
    addShort: "+ add",
    removeHost: "Remove host",
    remove: "Remove",

    // AddHostRow
    hostDraftPlaceholder: "e.g. blog or cname",
    addHostButton: "+ Add Host",

    // SyncModal
    syncDnsZones: "Sync DNS Zones",
    zonesDetected: "{{count}} zones detected",
    createdPrefix: "Created",
    updatedMid: "new, updated",
    recordsSuffix: "records.",
    syncToggleHint: "Use the 🔗 Link / ✉️ Mail toggles on each domain row to route active services.",
    done: "Done",
    noProviderAccounts: "No Provider Accounts Found",
    noProviderAccountsHint: "Configure your Cloudflare/DNSPod keys in Settings before syncing.",
    syncIntro: "Select a Cloudflare or DNSPod credentials connection. OCTARQ will query active domains and auto-populate your zone details.",
    selectAccount: "Select account...",
    selectProviderAccount: "Please select a provider account",
    syncFailed: "sync failed",
    queryingApi: "Querying API...",
    syncZones: "Sync Zones",

    // RecordsView
    allTypes: "All types",
    filterPlaceholder: "Filter name / content / comment…",
    presetButton: "+ Preset",
    customButton: "+ Custom",
    recordsNote: "Notes map directly to Cloudflare/DNSPod TXT comments · Showing {{shown}} of {{total}} records",
    loadRecordsFailed: "failed to load records",
    loadingRecords: "loading records…",
    noRecordsMatching: "No records matching search query.",
    thType: "Type",
    thName: "Name",
    thContent: "Content",
    thNote: "Note",
    cloudflareProxied: "Cloudflare Proxied",
    edit: "Edit",
    del: "Del",
    deleteRecordConfirm: "Delete {{type}} {{name}}?",

    // RecordEditor
    modifyRecord: "Modify Record",
    presetConfigurator: "Preset Configurator",
    createRecord: "Create Record",
    setLinkCname: "Set Link CNAME",
    setMxRecords: "Set MX records",
    recordType: "Record Type",
    nameHost: "Name (Host)",
    namePlaceholder: "@ or subdomain",
    targetValue: "Target Value",
    priority: "Priority",
    metadataComment: "Metadata Description / Comment",
    commentPlaceholder: "e.g. DNS verified token or note",
    proxiedLabel: "Proxied (Cloudflare Caching and SSL Proxy)",
    saveRecord: "Save Record",

    // contentHint
    hintIpv4: "IPv4 address",
    hintIpv6: "IPv6 address",
    hintCname: "target hostname",
    hintTxt: "text value",
    hintMx: "mail server hostname",
    hintNs: "nameserver hostname",
  },
  zh: {
    // Page header
    pageTitle: "DNS",
    pageDescription: "跨 Cloudflare 与 DNSPod 同步并管理 DNS 记录",
    syncCloudflare: "同步 Cloudflare",
    addDomain: "添加域名",

    // Tabs
    tabDns: "DNS",
    tabSettings: "设置",

    // Domain list
    searchDomains: "搜索域名…",
    noDomainsFound: "未找到域名。",
    toggleLinkRouting: "切换短链路由",
    toggleMailRouting: "切换邮件路由",
    loading: "加载中…",

    // Domain editor / detail
    addDomainZone: "添加域名区域",
    removeConfirm: "确定移除 {{name}}？",
    delete: "删除",
    managedHosts: "已管理主机",

    // DNS verification
    dnsSetupVerification: "DNS 配置校验",
    verifying: "校验中...",
    verifyDnsSetup: "校验 DNS 配置",
    verifyFailed: "校验 DNS 配置失败",
    verificationHint: "检查每个邮件主机的 SPF、DKIM 和 DMARC 记录是否已为邮件投递配置，以及每个短链主机是否解析到本应用。",
    statusLabel: "{{label}} 状态",
    unknown: "未知",
    mailHosts: "邮件主机",
    shortLinkHosts: "短链主机",

    dnsRecords: "DNS 记录",

    // Empty states
    addFirstDomain: "添加你的第一个域名",
    addFirstDomainHint: "一步从已连接的 DNS 服务商导入所有区域，或手动添加域名。",
    syncFrom: "从 {{name}} 同步",
    provider: "服务商",
    connectProvider: "连接 DNS 服务商",
    orAddManually: "或手动添加域名",
    selectDomainHint: "从左侧列表选择一个域名以管理 DNS 与路由。",

    // DnsStatusBadge
    configured: "✓ 已配置",
    misconfigured: "! 配置错误",
    missing: "✗ 缺失",

    // LinkHostRow
    pointsToZone: "✓ 指向区域",
    unverified: "! 未验证",
    notResolving: "✗ 未解析",
    resolvesNoCname: "已解析，但没有指向 {{target}} 的 CNAME",
    addCname: "添加 CNAME → {{target}}",

    // LinkHostGuide
    guideTitle: "如何将短链主机指向本应用",
    guideIntro: "需要一条 DNS 记录，访问者才能到达本服务器：",
    guideEachHost: "每个短链主机（例如 ",
    guideCnameTo: "将短链主机指向",
    guideCnameRec: "（推荐）。顶级域名本身必须已解析到本服务器的 IP。",
    guideOrAdd: "或添加一条",
    guideAaaaRec: "记录，直接指向本服务器的 IP。",
    guideProxiedIntro: "在 Cloudflare 上你可以让该记录保持",
    guideProxiedWord: "代理",
    guideProxiedMid: "（橙色云朵）以获得 TLS 与缓存 —— 此时校验会显示",
    guideUnverifiedWord: "未验证",
    guideProxiedEnd: "，因为 CNAME 被展平，这是正常现象。",
    guideTipIntro: "提示：使用",
    guideSubdomainPreset: "+ 子域名",
    guideTipEnd: "预设，在 DNS 记录 →“设置短链 CNAME”中自动创建此记录。",

    // DomainEditorForm
    domainName: "域名",
    dnsProviderConnection: "DNS 服务商连接",
    noAccountsAvailable: "暂无可用账户",
    zoneIdentifier: "Zone ID",
    zoneIdPlaceholder: "使用 Cloudflare 时自动发现",
    internalAdminNote: "内部管理备注",
    notePlaceholder: "给团队成员的可选备注",
    shortLinkSubdomains: "短链路由子域名",
    noShortlinkSubdomains: "暂无短链子域名。",
    inboundMailSubdomains: "入站邮件路由子域名",
    noMailboxSubdomains: "暂无邮箱子域名。",
    cancel: "取消",
    saving: "保存中...",
    saveBasicInfo: "保存基本信息",
    saveFailed: "保存失败",

    // DomainHostManager
    noActiveHosts: "尚未注册活动主机。请在下方添加。",
    thHost: "主机",
    thLink: "🔗 短链",
    thMail: "✉️ 邮件",
    on: "开",
    off: "关",
    addShort: "+ 添加",
    removeHost: "移除主机",
    remove: "移除",

    // AddHostRow
    hostDraftPlaceholder: "例如 blog 或 cname",
    addHostButton: "+ 添加主机",

    // SyncModal
    syncDnsZones: "同步 DNS 区域",
    zonesDetected: "检测到 {{count}} 个区域",
    createdPrefix: "新建",
    updatedMid: "条，更新",
    recordsSuffix: "条记录。",
    syncToggleHint: "使用每个域名行上的 🔗 短链 / ✉️ 邮件 开关来路由已启用的服务。",
    done: "完成",
    noProviderAccounts: "未找到服务商账户",
    noProviderAccountsHint: "同步前请先在设置中配置你的 Cloudflare/DNSPod 密钥。",
    syncIntro: "选择一个 Cloudflare 或 DNSPod 凭据连接。OCTARQ 将查询活动域名并自动填充你的区域详情。",
    selectAccount: "选择账户...",
    selectProviderAccount: "请选择一个服务商账户",
    syncFailed: "同步失败",
    queryingApi: "查询 API 中...",
    syncZones: "同步区域",

    // RecordsView
    allTypes: "全部类型",
    filterPlaceholder: "按名称 / 内容 / 备注筛选…",
    presetButton: "+ 预设",
    customButton: "+ 自定义",
    recordsNote: "备注直接映射到 Cloudflare/DNSPod 的 TXT 注释 · 显示 {{shown}} / {{total}} 条记录",
    loadRecordsFailed: "加载记录失败",
    loadingRecords: "加载记录中…",
    noRecordsMatching: "没有匹配搜索条件的记录。",
    thType: "类型",
    thName: "名称",
    thContent: "内容",
    thNote: "备注",
    cloudflareProxied: "Cloudflare 已代理",
    edit: "编辑",
    del: "删除",
    deleteRecordConfirm: "删除 {{type}} {{name}}？",

    // RecordEditor
    modifyRecord: "修改记录",
    presetConfigurator: "预设配置器",
    createRecord: "创建记录",
    setLinkCname: "设置短链 CNAME",
    setMxRecords: "设置 MX 记录",
    recordType: "记录类型",
    nameHost: "名称（主机）",
    namePlaceholder: "@ 或子域名",
    targetValue: "目标值",
    priority: "优先级",
    metadataComment: "元数据描述 / 备注",
    commentPlaceholder: "例如 DNS 验证令牌或备注",
    proxiedLabel: "代理（Cloudflare 缓存与 SSL 代理）",
    saveRecord: "保存记录",

    // contentHint
    hintIpv4: "IPv4 地址",
    hintIpv6: "IPv6 地址",
    hintCname: "目标主机名",
    hintTxt: "文本值",
    hintMx: "邮件服务器主机名",
    hintNs: "名称服务器主机名",
  },
};
