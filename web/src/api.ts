// Thin fetch wrapper around the led JSON API.

export interface Link {
  id: number;
  host: string;
  slug: string;
  target: string;
  note: string;
  title: string;
  tags: string;
  expiresAt: string | null;
  expiredUrl: string;
  clickLimit: number;
  archived: boolean;
  enabled: boolean;
  clicks: number;
  hasPassword: boolean;
  createdAt: string;
}

export interface Domain {
  id: number;
  name: string;
  providerAccountId: number;
  zoneId: string;
  note: string;
  forMail: boolean;
  forLink: boolean;
  linkHosts: HostEntry[] | null; // hostnames short links are served on (usually subdomains)
  mailHosts: HostEntry[] | null; // hostnames mailboxes live under
  createdAt: string;
}

export interface ProviderAccount {
  id: number;
  name: string;
  type: string;
  config: any;
  createdAt: string;
  updatedAt: string;
}

export interface SMTPSender {
  id: number;
  name: string;
  host: string;
  port: number;
  user: string;
  fromEmail: string;
  createdAt: string;
}

export interface NotificationChannel {
  id: number;
  name: string;
  type: string;
  config: string;
  enabled: boolean;
  createdAt: string;
}

export interface LicenseStatus {
  licensed: boolean;
  email?: string;
  tier?: string;
  expiresAt?: string;
  source: "env" | "file" | "none";
  envOverride: boolean;
}

export interface SessionRecord {
  id: number;
  userId: number;
  ip: string;
  userAgent: string;
  lastSeenAt: string;
  createdAt: string;
  isCurrent?: boolean;
}

export interface LicenseActivateResult {
  ok: boolean;
  tier: string;
  email: string;
  requiresRestart: boolean;
  envOverride: boolean;
}

export interface Product {
  id: number;
  slug: string;
  name: string;
  tagline: string;
  description: string;
  homepageUrl: string;
  status: "active" | "draft";
  createdAt: string;
  updatedAt: string;
}

export interface Plan {
  id: number;
  productId: number;
  name: string;
  tier: string;
  interval: "month" | "year" | "once";
  priceCents: number;
  currency: string;
  features: string[] | null;
  checkoutUrl: string;
  highlighted: boolean;
  sort: number;
}

export interface ReleaseAsset {
  label: string;
  url: string;
  os: string;
  arch: string;
  kind: "binary" | "image" | "checksum" | string;
}

export interface Release {
  id: number;
  productId: number;
  version: string;
  channel: "stable" | "beta";
  notes: string;
  assets: ReleaseAsset[] | null;
  createdAt: string;
}

export interface ProductKeyInfo {
  productId: number;
  hasKey: boolean;
  publicKey?: string;
  createdAt?: string;
}

export interface IssuedLicense {
  id: number;
  productId: number;
  email: string;
  tier: string;
  token: string;
  expiresAt: string | null;
  status: "active" | "revoked";
  provider: string;
  createdAt: string;
}

export interface BillingConfig {
  webhookSecretSet: boolean;
  providers: string[];
}

export interface PriceMap {
  id: number;
  stripeRef: string; // plink_… (or price_…)
  productSlug: string;
  tier: string;
  term: string; // monthly | yearly | lifetime | ""
  createdAt: string;
}

export type PriceMapInput = Pick<PriceMap, "stripeRef" | "productSlug" | "tier" | "term">;

export type ProductInput = Omit<Product, "id" | "createdAt" | "updatedAt">;
export type PlanInput = Omit<Plan, "id" | "productId">;
export type ReleaseInput = Pick<Release, "version" | "channel" | "notes"> & { assets: ReleaseAsset[] };

export interface HostEntry {
  host: string;
  enabled: boolean;
}

export interface SSHKey {
  id: number;
  name: string;
  type: string;
  pubKey: string;
  createdAt: string;
}

export interface VPS {
  id: number;
  name: string;
  ip: string;
  port: number;
  user: string;
  sshKeyId: number;
  status: string;
  failCount: number;
  lastChecked: string | null;
  createdAt: string;
}

export interface AuditLog {
  id: number;
  orgId: number;
  actorId: number;
  action: string;
  targetType: string;
  targetId: number;
  meta: string;
  ip: string;
  createdAt: string;
}

export interface AbuseReport {
  id: number;
  slug: string;
  target: string;
  reason: string;
  description: string;
  reporterIp: string;
  status: string;
  createdAt: string;
}

// effectiveLinkHosts / effectiveMailHosts return only the enabled hostnames —
// disabled hosts are kept in config but don't serve traffic.
export function effectiveLinkHosts(d: Domain): string[] {
  return (d.linkHosts ?? []).filter((h) => h.enabled).map((h) => h.host);
}
export function effectiveMailHosts(d: Domain): string[] {
  return (d.mailHosts ?? []).filter((h) => h.enabled).map((h) => h.host);
}

export interface DNSRecord {
  id: string;
  type: string;
  name: string;
  content: string;
  ttl: number;
  proxied: boolean;
  comment: string;
  priority?: number | null;
}

export interface Mailbox {
  id: number;
  address: string;
  note: string;
  enabled: boolean;
  unread: number;
}

export interface Attachment {
  filename: string;
  contentType: string;
  size: number;
}

export interface Email {
  id: number;
  mailboxId: number;
  from: string;
  to: string;
  subject: string;
  text: string;
  html: string;
  read: boolean;
  note: string;
  attachments: string; // JSON string of Attachment[]
  authSpf: string;   // pass|fail|softfail|neutral|none|""
  authDkim: string;  // pass|fail|none|""
  authDmarc: string; // pass|fail|none|""
  receivedAt: string;
}

export interface StatKV {
  key: string;
  count: number;
}
export interface LinkStats {
  total: number;
  windowed: number;
  days: number;
  series: StatKV[];
  referers: StatKV[] | null;
  countries: StatKV[] | null;
  regions: StatKV[] | null;
  devices: StatKV[] | null;
  browsers: StatKV[] | null;
}

export interface Subscription {
  id: number;
  name: string;
  vendor: string;
  cost: number;
  currency: string;
  cycle: "monthly" | "yearly";
  nextRenewal: string | null;
  note: string;
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface FinanceSummary {
  count: number;
  monthlyByCurrency: Record<string, number>;
  yearlyByCurrency: Record<string, number>;
  renewingSoon: Subscription[];
}

export interface Token {
  id: number;
  name: string;
  prefix: string;
  note: string;
  lastUsedAt: string | null;
  createdAt: string;
}

export interface Overview {
  links: number;
  activeLinks: number;
  domains: number;
  linkDomains: number;
  mailDomains: number;
  mailboxes: number;
  emails: number;
  unread: number;
  tokens: number;
  totalClicks: number;
  clicks7d: number;
  clicks30d: number;
  botClicks7d: number;
  botClicks30d: number;
  includeBot: boolean;
  series: StatKV[] | null;
  topLinks: { id: number; slug: string; host: string; clicks: number }[] | null;
  devices: StatKV[] | null;
  countries: StatKV[] | null;
  recentEmails: { id: number; from: string; subject: string; read: boolean; receivedAt: string }[] | null;
}

class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body ? { "Content-Type": "application/json" } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) {
    let msg = res.statusText;
    try {
      const j = await res.json();
      if (j.error) msg = j.error;
    } catch {
      /* ignore */
    }
    throw new ApiError(res.status, msg);
  }
  if (res.status === 204) return undefined as T;
  const ct = res.headers.get("content-type") || "";
  if (!ct.includes("application/json")) return undefined as T;
  return res.json();
}

export interface Settings {
  reservedSlugs: string;
  reservedMailboxes: string;
  builtinReserved: string[];
  cloudflareTokenSet: boolean;
  orgSlug: string;
  inboundToken: string;
  catchAll: boolean;
  telegramBotToken: string;
  telegramChatId: string;
  googleClientId: string;
  googleClientSecretSet: boolean;
  githubClientId: string;
  githubClientSecretSet: boolean;
  dataRetentionDays: number;
  autoWrapLinks: boolean;
}

// EmailAIAnnotation is the Inbox AI plugin's per-email analysis (Pro/elite).
export interface EmailAIAnnotation {
  emailId: number;
  from: string;
  subject: string;
  summary: string;
  category: string; // bill | otp | marketing | important | personal | other
  importance: number; // 1..5
  otp: string;
  model: string;
  createdAt: string;
}

// AIStatus reports whether Inbox AI is licensed and active.
export interface AIStatus {
  licensed: boolean;
  enabled: boolean;
  provider?: string;
  model?: string;
}

// AISettings is the DB-backed LLM configuration for Inbox AI (the API key is
// never returned — only whether one is set).
export interface AISettings {
  providerId: string; // selected LLMProvider id ("" = none)
  briefingHour: number;
  configured: boolean; // a usable LLM backend is resolvable
}

export interface LLMProvider {
  id: number;
  name: string;
  provider: string;
  baseUrl: string;
  model: string;
  cheapModel: string;
  apiKeySet: boolean;
}

export interface LLMProviderInput {
  name: string;
  provider: string;
  apiKey?: string; // omit = keep, "" = clear
  baseUrl?: string;
  model?: string;
  cheapModel?: string;
}

// AISettingsPatch updates AISettings; omitted fields are left unchanged, an
// empty apiKey clears the stored key.
export interface AISettingsPatch {
  providerId?: string;
  briefingHour?: number;
}

export const api = {
  // overview
  overview: (includeBot = false) =>
    req<Overview>("GET", `/api/overview${includeBot ? "?includeBot=true" : ""}`),

  // Inbox AI (Pro/elite) — email summaries, classification, OTP extraction.
  aiStatus: () => req<AIStatus>("GET", "/api/ai/status"),
  aiEmails: (category = "", limit = 50) => {
    const q = new URLSearchParams();
    if (category) q.set("category", category);
    q.set("limit", String(limit));
    return req<EmailAIAnnotation[]>("GET", `/api/ai/emails?${q.toString()}`);
  },
  aiReprocess: (emailId: number) =>
    req<EmailAIAnnotation>("POST", `/api/ai/emails/${emailId}/reprocess`),
  aiSettings: () => req<AISettings>("GET", "/api/ai/settings"),
  updateAiSettings: (s: AISettingsPatch) => req<AISettings>("PUT", "/api/ai/settings", s),

  // LLM provider registry (led-pro ai plugin; absent in OSS build → 404)
  llmProviders: () => req<LLMProvider[]>("GET", "/api/llm-providers"),
  createLlmProvider: (p: LLMProviderInput) => req<LLMProvider>("POST", "/api/llm-providers", p),
  updateLlmProvider: (id: number, p: LLMProviderInput) => req<LLMProvider>("PUT", `/api/llm-providers/${id}`, p),
  deleteLlmProvider: (id: number) => req<void>("DELETE", `/api/llm-providers/${id}`),

  // settings
  settings: () => req<Settings>("GET", "/api/settings"),
  updateSettings: (s: {
    reservedSlugs?: string;
    reservedMailboxes?: string;
    cloudflareToken?: string;
    inboundToken?: string;
    catchAll?: boolean;
    telegramBotToken?: string;
    telegramChatId?: string;
    googleClientId?: string;
    googleClientSecret?: string;
    githubClientId?: string;
    githubClientSecret?: string;
    dataRetentionDays?: number;
    autoWrapLinks?: boolean;
  }) => req<Settings>("PUT", "/api/settings", s),

  // license (led-pro licensing plugin; absent in the OSS build → 404)
  license: () => req<LicenseStatus>("GET", "/api/license"),
  activateLicense: (token: string) =>
    req<LicenseActivateResult>("POST", "/api/license", { token }),
  deactivateLicense: () => req<{ ok: boolean; requiresRestart: boolean }>("DELETE", "/api/license"),

  // storefront (led-pro product plugin; absent in OSS build → 404)
  products: () => req<Product[]>("GET", "/api/products"),
  createProduct: (p: ProductInput) => req<Product>("POST", "/api/products", p),
  updateProduct: (id: number, p: ProductInput) => req<Product>("PUT", `/api/products/${id}`, p),
  deleteProduct: (id: number) => req<void>("DELETE", `/api/products/${id}`),
  plans: (productId: number) => req<Plan[]>("GET", `/api/products/${productId}/plans`),
  createPlan: (productId: number, p: PlanInput) => req<Plan>("POST", `/api/products/${productId}/plans`, p),
  updatePlan: (id: number, p: PlanInput) => req<Plan>("PUT", `/api/plans/${id}`, p),
  deletePlan: (id: number) => req<void>("DELETE", `/api/plans/${id}`),
  releases: (productId: number) => req<Release[]>("GET", `/api/products/${productId}/releases`),
  createRelease: (productId: number, rel: ReleaseInput) =>
    req<Release>("POST", `/api/products/${productId}/releases`, rel),
  deleteRelease: (id: number) => req<void>("DELETE", `/api/releases/${id}`),

  // issuer: per-product signing keys + issuance records (led-pro issuer plugin)
  productKey: (productId: number) => req<ProductKeyInfo>("GET", `/api/products/${productId}/key`),
  createProductKey: (productId: number, privateKey?: string) =>
    req<{ productId: number; publicKey: string; note: string }>(
      "POST",
      `/api/products/${productId}/key`,
      privateKey ? { privateKey } : {},
    ),
  deleteProductKey: (productId: number) => req<void>("DELETE", `/api/products/${productId}/key`),
  issued: (productId?: number) =>
    req<IssuedLicense[]>("GET", `/api/issued${productId ? `?productId=${productId}` : ""}`),

  // billing config + price map (led-pro billing plugin)
  billingConfig: () => req<BillingConfig>("GET", "/api/billing/config"),
  updateBillingConfig: (p: { webhookSecret?: string }) =>
    req<BillingConfig>("PUT", "/api/billing/config", p),
  billingPrices: () => req<PriceMap[]>("GET", "/api/billing/prices"),
  createBillingPrice: (p: PriceMapInput) => req<PriceMap>("POST", "/api/billing/prices", p),
  updateBillingPrice: (id: number, p: PriceMapInput) => req<PriceMap>("PUT", `/api/billing/prices/${id}`, p),
  deleteBillingPrice: (id: number) => req<void>("DELETE", `/api/billing/prices/${id}`),

  // auth
  me: () => req<{ username: string; orgId: number }>("GET", "/api/auth/me"),
  login: (username: string, password: string) =>
    req<{ ok?: boolean; twoFactorRequired?: boolean; username: string }>(
      "POST",
      "/api/auth/login",
      { username, password },
    ),
  verify2FA: (username: string, password: string, code: string) =>
    req<{ ok: boolean }>("POST", "/api/auth/2fa/verify", { username, password, code }),
  logout: () => req<{ ok: boolean }>("POST", "/api/auth/logout"),
  logoutAll: () => req<{ ok: boolean }>("POST", "/api/auth/logout-all"),
  sessions: () => req<SessionRecord[]>("GET", "/api/auth/sessions"),
  revokeSession: (id: number) => req<{ ok: boolean; self: boolean }>("DELETE", `/api/auth/sessions/${id}`),
  acceptInvite: (token: string, password: string) =>
    req<{ ok: boolean }>("POST", "/api/auth/invite/accept", { token, password }),

  // 2FA (operator TOTP)
  twoFAStatus: () => req<{ enabled: boolean }>("GET", "/api/auth/2fa/status"),
  twoFASetup: () =>
    req<{ secret: string; otpauthUrl: string; qrDataUri?: string }>("POST", "/api/auth/2fa/setup"),
  twoFAEnable: (code: string) =>
    req<{ ok: boolean; recoveryCodes: string[] }>("POST", "/api/auth/2fa/enable", { code }),
  twoFADisable: (opts: { code?: string; password?: string }) =>
    req<{ ok: boolean }>("POST", "/api/auth/2fa/disable", opts),

  // links
  links: (params: { q?: string; tag?: string; host?: string; archived?: boolean; limit?: number; offset?: number } = {}) => {
    const sp = new URLSearchParams();
    if (params.q) sp.set("q", params.q);
    if (params.tag) sp.set("tag", params.tag);
    if (params.host) sp.set("host", params.host);
    if (params.archived) sp.set("archived", "1");
    if (params.limit) sp.set("limit", params.limit.toString());
    if (params.offset) sp.set("offset", params.offset.toString());
    const qs = sp.toString();
    return req<Link[]>("GET", `/api/links${qs ? `?${qs}` : ""}`);
  },
  createLink: (l: Partial<Link> & { password?: string }) => req<Link>("POST", "/api/links", l),
  updateLink: (id: number, l: Partial<Link> & { password?: string }) =>
    req<Link>("PUT", `/api/links/${id}`, l),
  deleteLink: (id: number) => req("DELETE", `/api/links/${id}`),
  linkStats: (id: number, days = 30) => req<LinkStats>("GET", `/api/links/${id}/stats?days=${days}`),
  linkMetadata: (url: string) =>
    req<{ title: string; description: string; favicon: string }>(
      "GET",
      `/api/links/metadata?url=${encodeURIComponent(url)}`,
    ),

  // domains
  dnsProviders: () => req<string[]>("GET", "/api/dns/providers"),

  providerAccounts: () => req<ProviderAccount[]>("GET", "/api/provider-accounts"),
  createProviderAccount: (p: any) => req<ProviderAccount>("POST", "/api/provider-accounts", p),
  updateProviderAccount: (id: number, p: any) => req<ProviderAccount>("PUT", `/api/provider-accounts/${id}`, p),
  deleteProviderAccount: (id: number) => req("DELETE", `/api/provider-accounts/${id}`) ,

  smtpSenders: () => req<SMTPSender[]>("GET", "/api/smtp-senders"),
  createSMTPSender: (s: any) => req<SMTPSender>("POST", "/api/smtp-senders", s),
  updateSMTPSender: (id: number, s: any) => req<SMTPSender>("PUT", `/api/smtp-senders/${id}`, s),
  deleteSMTPSender: (id: number) => req("DELETE", `/api/smtp-senders/${id}`),

  syncDomains: (providerAccountId: number) =>
    req<{ ok: boolean; total: number; created: number; updated: number }>(
      "POST",
      "/api/domains/sync",
      { providerAccountId },
    ),
  domains: (q?: { q?: string; limit?: number; offset?: number }) => {
    const params = new URLSearchParams();
    if (q?.q) params.set("q", q.q);
    if (q?.limit) params.set("limit", q.limit.toString());
    if (q?.offset) params.set("offset", q.offset.toString());
    const query = params.toString();
    return req<Domain[]>("GET", `/api/domains${query ? "?" + query : ""}`);
  },
  createDomain: (d: any) => req<Domain>("POST", "/api/domains", d),
  updateDomain: (id: number, d: any) => req<Domain>("PUT", `/api/domains/${id}`, d),
  deleteDomain: (id: number) => req("DELETE", `/api/domains/${id}`),
  verifyDNS: (id: number) => req<{ spf: boolean; dkim: boolean; dmarc: boolean }>("GET", `/api/domains/${id}/verify-dns`),
  records: (id: number) => req<DNSRecord[]>("GET", `/api/domains/${id}/records`),
  createRecord: (id: number, r: Partial<DNSRecord>) =>
    req<DNSRecord>("POST", `/api/domains/${id}/records`, r),
  updateRecord: (id: number, rid: string, r: Partial<DNSRecord>) =>
    req<DNSRecord>("PUT", `/api/domains/${id}/records/${rid}`, r),
  deleteRecord: (id: number, rid: string) => req("DELETE", `/api/domains/${id}/records/${rid}`),

  // mail
  mailboxes: () => req<Mailbox[]>("GET", "/api/mailboxes"),
  createMailbox: (m: Partial<Mailbox>) => req<Mailbox>("POST", "/api/mailboxes", m),
  updateMailbox: (id: number, m: Partial<Mailbox>) => req<Mailbox>("PUT", `/api/mailboxes/${id}`, m),
  deleteMailbox: (id: number) => req("DELETE", `/api/mailboxes/${id}`),
  emails: (mailboxId?: number, params?: { q?: string; limit?: number; offset?: number }) => {
    const sp = new URLSearchParams();
    if (mailboxId) sp.set("mailbox", mailboxId.toString());
    if (params?.q) sp.set("q", params.q);
    if (params?.limit) sp.set("limit", params.limit.toString());
    if (params?.offset) sp.set("offset", params.offset.toString());
    const qs = sp.toString();
    return req<Email[]>("GET", `/api/emails${qs ? "?" + qs : ""}`);
  },
  email: (id: number) => req<Email>("GET", `/api/emails/${id}`),
  updateEmail: (id: number, e: { read?: boolean; note?: string }) =>
    req<Email>("PUT", `/api/emails/${id}`, e),
  deleteEmail: (id: number) => req("DELETE", `/api/emails/${id}`),
  readAllEmails: (mailbox?: number) =>
    req<{ ok: boolean; updated: number }>(
      "POST",
      `/api/emails/read-all${mailbox ? `?mailbox=${mailbox}` : ""}`,
    ),
  rawEmailUrl: (id: number) => `/api/emails/${id}/raw`,
  sendEmail: (m: { from?: string; to: string[]; subject: string; text?: string; html?: string; smtpSenderId?: number; trackLinks?: boolean }) =>
    req("POST", "/api/emails/send", m),

  // tokens
  tokens: () => req<Token[]>("GET", "/api/tokens"),
  createToken: (d: { name: string; note: string }) => req<{ token: string }>("POST", "/api/tokens", d),
  deleteToken: (id: number) => req<void>("DELETE", `/api/tokens/${id}`),

  // notification channels
  notificationChannels: () => req<NotificationChannel[]>("GET", "/api/notification-channels"),
  createNotificationChannel: (d: any) => req<NotificationChannel>("POST", "/api/notification-channels", d),
  updateNotificationChannel: (id: number, d: any) => req<NotificationChannel>("PUT", `/api/notification-channels/${id}`, d),
  deleteNotificationChannel: (id: number) => req<void>("DELETE", `/api/notification-channels/${id}`),
  testNotificationChannel: (id: number) => req<void>("POST", `/api/notification-channels/${id}/test`),

  // ssh keys
  sshKeys: () => req<SSHKey[]>("GET", "/api/ssh-keys"),
  createSSHKey: (d: { name: string; type: string; key?: string }) => req<SSHKey & { rawPrivateKey?: string }>("POST", "/api/ssh-keys", d),
  deleteSSHKey: (id: number) => req<void>("DELETE", `/api/ssh-keys/${id}`),

  // vps
  vpsList: () => req<VPS[]>("GET", "/api/vps"),
  createVPS: (d: Partial<VPS>) => req<VPS>("POST", "/api/vps", d),
  updateVPS: (id: number, d: Partial<VPS>) => req<VPS>("PUT", `/api/vps/${id}`, d),
  deleteVPS: (id: number) => req<void>("DELETE", `/api/vps/${id}`),

  // finance
  subscriptions: () => req<Subscription[]>("GET", "/api/subscriptions"),
  createSubscription: (d: Partial<Subscription>) => req<Subscription>("POST", "/api/subscriptions", d),
  updateSubscription: (id: number, d: Partial<Subscription>) => req<Subscription>("PUT", `/api/subscriptions/${id}`, d),
  deleteSubscription: (id: number) => req<void>("DELETE", `/api/subscriptions/${id}`),
  financeSummary: () => req<FinanceSummary>("GET", "/api/finance/summary"),

  // transactions
  transactions: () => req<Transaction[]>("GET", "/api/transactions"),
  createTransaction: (d: Partial<Transaction>) => req<Transaction>("POST", "/api/transactions", d),
  updateTransaction: (id: number, d: Partial<Transaction>) => req<Transaction>("PUT", `/api/transactions/${id}`, d),
  deleteTransaction: (id: number) => req<void>("DELETE", `/api/transactions/${id}`),
  deleteTransactionSeries: (parentId: string) => req<void>("DELETE", `/api/transactions/series/${parentId}`),

  // webhooks
  webhooks: () => req<Webhook[]>("GET", "/api/webhooks"),
  createWebhook: (d: Partial<Webhook>) => req<Webhook>("POST", "/api/webhooks", d),
  updateWebhook: (id: number, d: Partial<Webhook>) => req<Webhook>("PUT", `/api/webhooks/${id}`, d),
  deleteWebhook: (id: number) => req<void>("DELETE", `/api/webhooks/${id}`),

  // audit
  auditLogs: () => req<AuditLog[]>("GET", "/api/audit"),

  // abuse
  abuseReports: (status?: string) => req<AbuseReport[]>("GET", `/api/abuse${status ? `?status=${status}` : ''}`),
  updateAbuseReport: (id: number, status: string) => req<AbuseReport>("PUT", `/api/abuse/${id}`, { status }),

  // orgs
  orgs: () => req<Org[]>("GET", "/api/orgs"),
  createOrg: (d: { name: string }) => req<Org>("POST", "/api/orgs", d),
  updateOrg: (d: { name: string }) => req<Org>("PUT", "/api/org", d),
  switchOrg: (orgId: number) => req<{ ok: boolean }>("POST", "/api/auth/switch-org", { orgId }),
  orgMembers: () => req<OrgMember[]>("GET", "/api/org/members"),
  addOrgMember: (d: { email: string; role: string }) => req<{ ok: boolean }>("POST", "/api/org/members", d),
  deleteOrgMember: (userId: number) => req<void>("DELETE", `/api/org/members/${userId}`),

  // menus and user settings
  menus: () => req<MenuItem[]>("GET", "/api/menus"),
  getUserSettings: () => req<Record<string, string>>("GET", "/api/user/settings"),
  updateUserSettings: (key: string, value: string) => req<{ ok: boolean }>("PUT", "/api/user/settings", { key, value }),

  // Customer & Portal APIs (led-pro customer / portal plugins)
  customerRegister: (email: string, password: string) => req<{ ok: boolean; email: string; emailVerified: boolean }>("POST", "/api/customer/register", { email, password }),
  customerLogin: (email: string, password: string) => req<{ ok: boolean; email: string }>("POST", "/api/customer/login", { email, password }),
  customerLogout: () => req<{ ok: boolean }>("POST", "/api/customer/logout"),
  customerMe: () => req<{ email: string; createdAt: string }>("GET", "/api/customer/me"),
  claimAccount: (sessionId: string, password: string) => req<{ ok: boolean; email: string; emailVerified: boolean }>("POST", "/api/customer/claim-account", { sessionId, password }),
  portalLicenses: () => req<{ licenses: IssuedLicense[] }>("GET", "/api/portal/licenses"),
  portalDevices: (id: number) => req<LicenseDevice[]>("GET", `/api/portal/licenses/${id}/devices`),
  portalUnbindDevice: (id: number, deviceId: number) => req<{ ok: boolean }>("DELETE", `/api/portal/licenses/${id}/devices/${deviceId}`),
  portalBillingPortal: () => req<{ url: string }>("POST", "/api/portal/billing-portal"),

  // GDPR
  exportAccountData: () => req<any>("GET", "/api/account/export"),
  purgeAccountData: () => req<void>("DELETE", "/api/account/data"),
};

export interface Org {
  id: number;
  name: string;
  slug: string;
  role?: string;
}

export interface OrgMember {
  userId: number;
  email: string;
  role: string;
}

export interface MenuItem {
  id: string;
  label: string;
  path: string;
  icon: string;
  category: string;
}

export interface Transaction {
  id: number;
  parentId?: string;
  date: string;
  type: "income" | "expense";
  title: string;
  category: string;
  amount: number;
  currency: string;
  cycle: "one-off" | "monthly" | "yearly";
  invoiceEmailId?: number;
}

export interface Webhook {
  id: number;
  name: string;
  url: string;
  secret: string;
  events: string;
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface Customer {
  id: number;
  email: string;
  emailVerified: boolean;
  createdAt: string;
}

export interface LicenseDevice {
  id: number;
  issuedLicenseId: number;
  deviceId: string;
  name: string;
  ip: string;
  lastSeenAt: string;
  createdAt: string;
}

export { ApiError };

