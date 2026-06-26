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
  inboundToken: string;
  catchAll: boolean;
  telegramBotToken: string;
  telegramChatId: string;
  googleClientId: string;
  googleClientSecretSet: boolean;
  githubClientId: string;
  githubClientSecretSet: boolean;
  dataRetentionDays: number;
}

export const api = {
  // overview
  overview: (includeBot = false) =>
    req<Overview>("GET", `/api/overview${includeBot ? "?includeBot=true" : ""}`),

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
  }) => req<Settings>("PUT", "/api/settings", s),

  // auth
  me: () => req<{ username: string }>("GET", "/api/auth/me"),
  login: (username: string, password: string) =>
    req<{ ok: boolean }>("POST", "/api/auth/login", { username, password }),
  logout: () => req<{ ok: boolean }>("POST", "/api/auth/logout"),

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
  sendEmail: (m: { from?: string; to: string[]; subject: string; text?: string; html?: string; smtpSenderId?: number }) =>
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

  // audit
  auditLogs: () => req<AuditLog[]>("GET", "/api/audit"),

  // abuse
  abuseReports: (status?: string) => req<AbuseReport[]>("GET", `/api/abuse${status ? `?status=${status}` : ''}`),
  updateAbuseReport: (id: number, status: string) => req<AbuseReport>("PUT", `/api/abuse/${id}`, { status }),
};

export { ApiError };
