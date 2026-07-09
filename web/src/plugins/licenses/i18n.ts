// Translation namespace for the licenses plugin, injected via UIPlugin.i18n
// (see ./index.ts) and merged under the `licenses.*` key. Previously lived in
// web/src/i18n/pages/licenses.ts and was baked into the central bundle; now the
// plugin owns it, so a build that doesn't compose the plugin never ships it.
export const licenses = {
  en: {
    lockedFeature: "License Management",
    lockedDescription: "Track and manage the software licenses sold through your storefront.",
    lockedPerk1: "Each product signs licenses with its own key (Ed25519)",
    lockedPerk2: "See every license\u2019s buyer, tier, and status",
    lockedPerk3: "Licenses revoke automatically when a subscription ends",
    pageTitle: "Licenses",
    pageDesc: "All issued licenses: active, expired, and revoked",
    emptyState: "No licenses issued yet.",
    colEmail: "Email",
    colTier: "Tier",
    colProduct: "Product",
    colVia: "Via",
    colStatus: "Status",
    colExpires: "Expires",
    colIssued: "Issued",
    never: "never",
  },
  zh: {
    lockedFeature: "许可证管理",
    lockedDescription: "跟踪和管理通过店面售出的软件许可证。",
    lockedPerk1: "每个产品使用独立密钥签发许可证（Ed25519）",
    lockedPerk2: "查看每个许可证的买家、等级和状态",
    lockedPerk3: "订阅结束时自动吊销许可证",
    pageTitle: "许可证",
    pageDesc: "全部已签发的许可证：有效、过期与已吊销",
    emptyState: "尚未签发任何许可证。",
    colEmail: "邮箱",
    colTier: "等级",
    colProduct: "产品",
    colVia: "渠道",
    colStatus: "状态",
    colExpires: "到期",
    colIssued: "签发",
    never: "永不过期",
  },
};
