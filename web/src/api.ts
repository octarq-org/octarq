// Thin fetch wrapper around the octarq JSON API.

export interface StatKV {
  key: string;
  count: number;
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
  hasCredentials: boolean; // credentials are set (encrypted at rest, never returned)
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
  passSet: boolean; // password is set (encrypted at rest, never returned)
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

export interface SessionRecord {
  id: number;
  userId: number;
  ip: string;
  userAgent: string;
  lastSeenAt: string;
  createdAt: string;
  isCurrent?: boolean;
}

export interface HostEntry {
  host: string;
  enabled: boolean;
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
  cities: StatKV[] | null;
  recentEmails: { id: number; from: string; subject: string; read: boolean; receivedAt: string }[] | null;
}

class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

export async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
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
  reservedMailboxes: string;
  orgSlug: string;
  inboundToken: string;
  catchAll: boolean;
  autoWrapLinks: boolean;
  isInstanceAdmin: boolean;
}

export interface InstanceSettings {
  reservedSlugs: string;
  builtinReserved: string[];
  googleClientId: string;
  googleClientSecretSet: boolean;
  githubClientId: string;
  githubClientSecretSet: boolean;
  dataRetentionDays: number;
  allowRegistration: boolean;
  appName: string;
  metricsTokenSet: boolean;
  ratelimitAuthRpm: number;
  ratelimitApiRpm: number;
  ratelimitRedirectRpm: number;
}

export const api = {
  // overview
  overview: (includeBot = false) =>
    req<Overview>("GET", `/api/overview${includeBot ? "?includeBot=true" : ""}`),

  // settings
  settings: () => req<Settings>("GET", "/api/settings"),
  updateSettings: (s: {
    reservedMailboxes?: string;
    inboundToken?: string;
    catchAll?: boolean;
    autoWrapLinks?: boolean;
  }) => req<Settings>("PUT", "/api/settings", s),

  instanceSettings: () => req<InstanceSettings>("GET", "/api/instance-settings"),
  updateInstanceSettings: (s: {
    reservedSlugs?: string;
    googleClientId?: string;
    googleClientSecret?: string;
    githubClientId?: string;
    githubClientSecret?: string;
    dataRetentionDays?: number;
    allowRegistration?: boolean;
    appName?: string;
    metricsToken?: string;
    ratelimitAuthRpm?: number;
    ratelimitApiRpm?: number;
    ratelimitRedirectRpm?: number;
  }) => req<InstanceSettings>("PUT", "/api/instance-settings", s),

  // auth
  authConfig: () => req<{ googleEnabled: boolean; githubEnabled: boolean; registrationEnabled: boolean; appName: string; logoUrl: string; brandColor: string; brandColor2: string }>("GET", "/api/auth/config"),
  me: () => req<{ username: string; orgId: number; role?: string }>("GET", "/api/auth/me"),
  register: (email: string, password: string) =>
    req<{ ok: boolean; username: string }>("POST", "/api/auth/register", { email, password }),
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

  // single-step AI assists (OSS, BYO key — buttons hide when unconfigured)
  aiAssistStatus: () => req<{ configured: boolean; provider: string }>("GET", "/api/ai/assist/status"),
  aiSuggestSlug: (target: string, title?: string) =>
    req<{ slugs: string[] }>("POST", "/api/ai/assist/suggest-slug", { target, title }),
  aiSummarizeEmail: (id: number) => req<{ summary: string }>("POST", `/api/ai/assist/summarize-email/${id}`),

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

  domains: (q?: { q?: string; limit?: number; offset?: number }) => {
    const params = new URLSearchParams();
    if (q?.q) params.set("q", q.q);
    if (q?.limit) params.set("limit", q.limit.toString());
    if (q?.offset) params.set("offset", q.offset.toString());
    const query = params.toString();
    return req<Domain[]>("GET", `/api/domains${query ? "?" + query : ""}`);
  },

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

  // webhooks
  webhooks: () => req<Webhook[]>("GET", "/api/webhooks"),
  webhookEvents: () => req<WebhookEventGroup[]>("GET", "/api/webhooks/events"),
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
  plugins: () => req<PluginInfo[]>("GET", "/api/plugins"),
  updatePlugin: (key: string, enabled: boolean) =>
    req<{ ok: boolean }>("PUT", `/api/plugins/${key}`, { enabled }),
  getUserSettings: () => req<Record<string, string>>("GET", "/api/user/settings"),
  updateUserSettings: (key: string, value: string) => req<{ ok: boolean }>("PUT", "/api/user/settings", { key, value }),

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
  joinedAt?: string;
  pending?: boolean;
}

export interface MenuItem {
  id: string;
  label: string;
  path: string;
  icon: string;
  category: string;
  order?: number;
  // Advisory minimum org role (member < admin < owner) — items the current
  // user doesn't meet are hidden from the sidebar/command palette. Mirrors
  // PluginMenuItem.requiredRole; enforcement stays server-side.
  requiredRole?: string;
}

export interface PluginInfo {
  key: string;
  title: string;
  description?: string;
  enabled: boolean;
  menus: MenuItem[];
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

export interface WebhookEventDef {
  key: string;
  group: string;
  title: string;
  description: string;
}

export interface WebhookEventGroup {
  group: string;
  events: WebhookEventDef[];
}

export { ApiError };

