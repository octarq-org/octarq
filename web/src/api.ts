// Thin fetch wrapper around the octarq JSON API.
import { useEffect, useState } from "react";

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

// One selectable channel type in the Alerts provider list. Built-ins and
// plugin-contributed types are the same shape — the point of the registry is
// that the UI cannot tell them apart. Mirrors api.NotificationChannelType.
export interface NotificationChannelType {
  type: string;
  title: string;
  description: string;
  icon: string;
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


export interface ApiToken {
  id: number;
  name: string;
  token?: string;
  lastUsedAt?: string;
  createdAt: string;
}

export interface HelpCategory {
  key: string;
  order: number;
  icon: string;
  labels: Record<string, string>;
}

export interface HelpDocMeta {
  slug: string;
  title: string;
  category: string;
  order: number;
}

export interface HelpDocContent {
  title: string;
  html: string;
}

export interface Token {
  id: number;
  name: string;
  prefix: string;
  note: string;
  /** The user the token acts as. Its authority is theirs, read live, so
   *  removing them from the workspace revokes the token too. */
  userId: number;
  /** Narrows the token below its holder, never above: the effective role is
   *  min(holder's role, this). "" reads as "member" server-side. */
  role: "" | "member" | "admin" | "owner";
  lastUsedAt: string | null;
  expiresAt: string | null;
  createdAt: string;
}

export interface SubsystemHealth {
  name: string;
  status: "ok" | "degraded" | "down" | "na";
  detail?: string;
}

export interface SubsystemStatusResponse {
  overall: "ok" | "degraded" | "down";
  subsystems: SubsystemHealth[];
  time: string;
}

export interface Overview extends Record<string, unknown> {
  tokens?: number;
  includeBot?: boolean;
  links?: number;
  activeLinks?: number;
  domains?: number;
  linkDomains?: number;
  mailDomains?: number;
  mailboxes?: number;
  emails?: number;
  unread?: number;
  totalClicks?: number;
  clicks7d?: number;
  clicks30d?: number;
  botClicks7d?: number;
  botClicks30d?: number;
  series?: StatKV[] | null;
  topLinks?: { id: number; slug: string; host: string; clicks: number }[] | null;
  devices?: StatKV[] | null;
  countries?: StatKV[] | null;
  cities?: StatKV[] | null;
  recentEmails?: { id: number; from: string; subject: string; read: boolean; receivedAt: string }[] | null;
}

export class ApiError extends Error {
  status: number;
  body?: any;
  constructor(status: number, message: string, body?: any) {
    super(message);
    this.status = status;
    this.body = body;
  }
}

function getAppLang(): string {
  try {
    const saved = localStorage.getItem("lang");
    if (saved) return saved;
  } catch {
    /* ignore */
  }
  return navigator.language || "en";
}

export async function req<T>(method: string, path: string, body?: unknown, lang?: string): Promise<T> {
  const currentLang = lang || getAppLang();
  const headers: Record<string, string> = {
    "Accept-Language": currentLang,
  };
  if (body) {
    headers["Content-Type"] = "application/json";
  }
  const res = await fetch(path, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) {
    let msg = res.statusText;
    let parsed: any;
    try {
      parsed = await res.json();
      // `error` is our own shape; `detail` is huma's RFC7807 field. Prefer
      // whichever the endpoint used rather than falling back to statusText.
      if (parsed?.error) msg = parsed.error;
      else if (parsed?.detail) msg = parsed.detail;
    } catch {
      /* not JSON — keep statusText */
    }
    // Carry the decoded body: some errors are structured (e.g. a 409 from the
    // plugin toggle names the dependents that block the change) and the caller
    // needs the fields, not just the message.
    throw new ApiError(res.status, msg, parsed);
  }
  if (res.status === 204) return undefined as T;
  const ct = res.headers.get("content-type") || "";
  if (!ct.includes("application/json")) return undefined as T;
  return res.json();
}

export interface Settings {
  reservedMailboxes: string;
  orgSlug: string;
  inboundToken?: string;
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
  requireEmailVerification?: boolean;
  appName: string;
  metricsTokenSet: boolean;
  ratelimitAuthRpm: number;
  ratelimitApiRpm: number;
  ratelimitRedirectRpm: number;
}

export const api = {
  // subsystem status (public)
  subsystemStatus: () => req<SubsystemStatusResponse>("GET", "/api/status"),

  // plugins: instance-level registry (read-only, instance admins only)
  instancePlugins: () => req<InstancePluginInfo[]>("GET", "/api/instance/plugins"),

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
    requireEmailVerification?: boolean;
    appName?: string;
    metricsToken?: string;
    ratelimitAuthRpm?: number;
    ratelimitApiRpm?: number;
    ratelimitRedirectRpm?: number;
  }) => req<InstanceSettings>("PUT", "/api/instance-settings", s),

  // auth
  authConfig: () => req<{ googleEnabled: boolean; githubEnabled: boolean; registrationEnabled: boolean; appName: string; logoUrl: string; brandColor: string; brandColor2: string }>("GET", "/api/auth/config"),
  me: () => req<{ email: string; username?: string; orgId: number; role?: string; emailVerified?: boolean }>("GET", "/api/auth/me"),
  register: (email: string, password: string) =>
    req<{ ok: boolean; email: string; username?: string }>("POST", "/api/auth/register", { email, password }),
  login: (email: string, password: string) =>
    req<{ ok?: boolean; twoFactorRequired?: boolean; email: string; username?: string }>(
      "POST",
      "/api/auth/login",
      { email, password },
    ),
  verify2FA: (email: string, password: string, code: string) =>
    req<{ ok: boolean }>("POST", "/api/auth/2fa/verify", { email, password, code }),
  forgotPassword: (email: string) => req<{ ok: boolean }>("POST", "/api/auth/forgot", { email }),
  resetPassword: (token: string, password: string) => req<{ ok: boolean }>("POST", "/api/auth/reset", { token, password }),
  // Authenticated change, as opposed to the emailed reset above. Succeeding
  // here revokes every other session; this one survives.
  changePassword: (currentPassword: string, newPassword: string) =>
    req<{ ok: boolean }>("POST", "/api/auth/password", { currentPassword, newPassword }),
  changeEmail: (newEmail: string, currentPassword?: string) =>
    req<{ ok: boolean; email: string; verificationSent?: boolean }>("PUT", "/api/auth/email", { newEmail, currentPassword }),
  resendVerification: (email: string) => req<{ ok: boolean }>("POST", "/api/auth/resend-verification", { email }),
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
  createToken: (d: { name: string; note?: string; role?: "member" | "admin" | "owner"; expiresInDays?: number }) =>
    req<{ token: string } & Token>("POST", "/api/tokens", d),
  updateToken: (id: number, d: { name?: string; note?: string; role?: "member" | "admin" | "owner"; expiresInDays?: number }) =>
    req<Token>("PUT", `/api/tokens/${id}`, d),
  deleteToken: (id: number) => req<void>("DELETE", `/api/tokens/${id}`),

  // notification channels
  // The available channel types, org-scoped: a type contributed by a plugin the
  // workspace has disabled is not listed. Drives the provider list in Alerts.
  notificationChannelTypes: () => req<NotificationChannelType[]>("GET", "/api/notification-channel-types"),
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
  // inviteUrl/inviteToken come back only when the address had no account yet.
  // Delivery of that link by email is best-effort on the server (it needs the
  // mail plugin mounted and an SMTP sender configured, and failures are logged,
  // not returned), so the caller has to surface the link — otherwise on an
  // instance without mail the invite exists and nobody can reach it.
  addOrgMember: (d: { email: string; role: string }) =>
    req<{ ok: boolean; inviteToken?: string; inviteUrl?: string }>("POST", "/api/org/members", d),
  updateOrgMemberRole: (userId: number, role: string) =>
    req<{ ok: boolean }>("PATCH", `/api/org/members/${userId}`, { role }),
  deleteOrgMember: (userId: number) => req<void>("DELETE", `/api/org/members/${userId}`),

  // menus and user settings
  menus: () => req<MenuItem[]>("GET", "/api/menus"),
  plugins: () => req<PluginInfo[]>("GET", "/api/plugins"),
  updatePlugin: (key: string, enabled: boolean) =>
    req<{ ok: boolean }>("PUT", `/api/plugins/${key}`, { enabled }),
  getUserSettings: () => req<Record<string, string>>("GET", "/api/user/settings"),
  updateUserSettings: (key: string, value: string) => req<{ ok: boolean }>("PUT", "/api/user/settings", { key, value }),

  // Instance backup (instance admins only). Not a `req` call: the response is a
  // binary database dump, and `req` returns undefined for anything that isn't
  // JSON. The server names the file in Content-Disposition — it knows the
  // driver, so it knows whether this is a .db or a .sql — and the caller only
  // falls back to a generic name if the header is missing.
  downloadBackup: async (): Promise<{ blob: Blob; filename: string }> => {
    const res = await fetch("/api/admin/backup");
    if (!res.ok) {
      let msg = res.statusText;
      try {
        const parsed = await res.json();
        if (parsed?.error) msg = parsed.error;
        else if (parsed?.detail) msg = parsed.detail;
      } catch {
        /* not JSON — keep statusText */
      }
      throw new ApiError(res.status, msg);
    }
    const cd = res.headers.get("content-disposition") || "";
    const m = /filename="?([^"]+)"?/.exec(cd);
    return { blob: await res.blob(), filename: m?.[1] || "octarq-backup" };
  },

  // GDPR
  exportWorkspaceData: () => req<any>("GET", "/api/account/export"),
  purgeWorkspaceData: () => req<void>("DELETE", "/api/account/data"),

  // Help
  helpCategories: () => req<HelpCategory[]>("GET", "/api/help/categories"),
  helpIndex: (lang?: string) => req<HelpDocMeta[]>("GET", lang ? `/api/help/docs?lang=${encodeURIComponent(lang)}` : "/api/help/docs", undefined, lang),
  helpPage: (slug: string, lang?: string) => req<HelpDocContent>("GET", lang ? `/api/help/docs/${encodeURIComponent(slug)}?lang=${encodeURIComponent(lang)}` : `/api/help/docs/${encodeURIComponent(slug)}`, undefined, lang),
};

const overviewInflight = new Map<boolean, { promise: Promise<Overview>; time: number }>();

export function fetchOverview(includeBot = false): Promise<Overview> {
  const key = !!includeBot;
  const now = Date.now();
  const cached = overviewInflight.get(key);
  if (cached && now - cached.time < 2000) {
    return cached.promise;
  }
  const promise = api.overview(key).catch((err) => {
    overviewInflight.delete(key);
    throw err;
  });
  overviewInflight.set(key, { promise, time: now });
  return promise;
}

export function useOverviewData(includeBot = false): Overview | null {
  const [data, setData] = useState<Overview | null>(null);
  useEffect(() => {
    let active = true;
    fetchOverview(includeBot).then((res) => {
      if (active) setData(res);
    }).catch(() => {});
    return () => {
      active = false;
    };
  }, [includeBot]);
  return data;
}

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

export interface InstancePluginInfo {
  name: string;
  featureKey: string;
  title: string;
  category: string;
  core: boolean;
  enabledByDefault: boolean;
  requires: string[];
  hasUI: boolean;
}

export interface PluginInfo {
  key: string;
  title: string;
  description?: string;
  icon?: string;
  category?: string;
  tags?: string[];
  enabled: boolean;
  /** Always-on plumbing: listed so it's visible, but its switch is dead. */
  core?: boolean;
  menus: MenuItem[];
  requires?: string[];
  requiredBy?: string[];
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

// No AuthMethod type or api.authMethods() here. /api/auth/methods has exactly
// one consumer — plugin-sso's login button — and a Pro package cannot import
// the host's api module, so it calls fetch() directly. The typed wrapper had no
// reachable caller and drifting it from the live untyped call was the only
// thing it could ever do. Expose it through @octarq/plugin-sdk if a plugin
// needs it typed.



