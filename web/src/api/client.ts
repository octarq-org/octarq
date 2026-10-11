// Thin fetch wrapper and endpoint methods for the octarq JSON API.
import { parseWithFallback } from "../lib/parseWithFallback";
import {
  NotificationsListSchema,
  NotificationPreferencesListSchema,
  RegisteredChannelsListSchema,
  SimpleOKSchema,
} from "../pages/notifications/schemas";
import type {
  AbuseReport,
  Action,
  AuditLog,
  Domain,
  HelpCategory,
  HelpDocContent,
  HelpDocMeta,
  InstancePluginInfo,
  InstanceSettings,
  LinkedIdentity,
  ListAuditLogsParams,
  MenuItem,
  NotificationChannel,
  NotificationChannelInput,
  NotificationChannelType,
  NotificationItem,
  NotificationPreferenceItem,
  Org,
  OrgMember,
  OrgSlug,
  Overview,
  PluginInfo,
  ProviderAccount,
  ProviderAccountInput,
  ReadinessCheck,
  RegisteredChannel,
  SessionRecord,
  Settings,
  SMTPSender,
  SMTPSenderInput,
  SubsystemStatusResponse,
  Token,
  Webhook,
  WebhookEventGroup,
} from "./types";

export class ApiError extends Error {
  status: number;
  body?: any;
  /** Server-side correlation id (X-Request-Id), when the response carried one. */
  requestId?: string;
  constructor(status: number, message: string, body?: any, requestId?: string) {
    super(message);
    this.status = status;
    this.body = body;
    this.requestId = requestId;
  }
}

export function getAppLang(): string {
  try {
    const saved = localStorage.getItem("lang");
    if (saved) return saved;
  } catch {
    /* ignore */
  }
  return navigator.language || "en";
}

interface ErrorDetail {
  location?: string;
  message?: string;
}

interface ErrorEnvelope {
  message?: string;
  error?: string;
  detail?: string;
  details?: Array<string | ErrorDetail>;
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
    let parsed: ErrorEnvelope | null = null;
    try {
      parsed = (await res.json()) as ErrorEnvelope;
      if (parsed?.message) {
        msg = parsed.message;
        if (parsed.details && Array.isArray(parsed.details) && parsed.details.length > 0) {
          const detailMsgs = parsed.details
            .map((d) => {
              if (typeof d === "string") return d;
              if (d?.message) {
                const loc = d.location ? `${d.location.replace(/^body\./, "")}: ` : "";
                return `${loc}${d.message}`;
              }
              return "";
            })
            .filter(Boolean);
          if (detailMsgs.length > 0) {
            msg = `${parsed.message}: ${detailMsgs.join(", ")}`;
          }
        }
      } else if (parsed?.error) {
        msg = parsed.error;
      } else if (parsed?.detail) {
        msg = parsed.detail;
      }
    } catch {
      /* not JSON — keep statusText */
    }
    throw new ApiError(res.status, msg, parsed, res.headers.get("x-request-id") ?? undefined);
  }
  if (res.status === 204) return undefined as T;
  const ct = res.headers.get("content-type") || "";
  if (!ct.includes("application/json")) return undefined as T;
  return res.json();
}

export const api = {
  // subsystem status (public)
  subsystemStatus: () => req<SubsystemStatusResponse>("GET", "/api/status"),

  // instance readiness (instance admins only)
  instanceReadiness: () => req<ReadinessCheck[]>("GET", "/api/instance/readiness"),

  // plugins: instance-level registry (read-only, instance admins only)
  instancePlugins: () => req<InstancePluginInfo[]>("GET", "/api/instance/plugins"),

  // instance-level plugin menus (instance admins only)
  instanceMenus: () => req<MenuItem[]>("GET", "/api/instance/menus"),

  // build metadata (authenticated — never surfaced pre-login)
  instanceBuild: () =>
    req<{ version: string; commit: string; builtAt: string }>("GET", "/api/instance/build"),

  // overview
  overview: (includeBot = false) =>
    req<Overview>("GET", `/api/overview${includeBot ? "?includeBot=true" : ""}`),

  // settings
  settings: () => req<Settings>("GET", "/api/settings"),
  inboundToken: () => req<{ inboundToken: string }>("GET", "/api/settings/inbound-token"),
  updateSettings: (s: {
    reservedMailboxes?: string;
    inboundToken?: string;
    catchAll?: boolean;
    autoWrapLinks?: boolean;
  }) => req<Settings>("PUT", "/api/settings", s),

  instanceSettings: () => req<InstanceSettings>("GET", "/api/instance-settings"),
  instanceLinkSettings: () => req<{ reservedSlugs: string; builtinReserved: string[] }>("GET", "/api/instance/link-settings"),
  updateInstanceLinkSettings: (s: { reservedSlugs?: string }) =>
    req<{ reservedSlugs: string; builtinReserved: string[] }>("PUT", "/api/instance/link-settings", s),
  updateInstanceSettings: (s: Partial<InstanceSettings> & {
    googleClientSecret?: string;
    githubClientSecret?: string;
    metricsToken?: string;
  }) => req<InstanceSettings>("PUT", "/api/instance-settings", s),
  testInstanceMail: (to: string) =>
    req<{ ok: boolean }>("POST", "/api/instance/mail/test", { to }),

  // auth
  authConfig: () => req<{ googleEnabled: boolean; githubEnabled: boolean; registrationEnabled: boolean; appName: string; logoUrl: string; brandColor: string; brandColor2: string }>("GET", "/api/auth/config"),
  me: () => req<{ email: string; username?: string; orgId: number; role?: string; emailVerified?: boolean; isInstanceAdmin?: boolean }>("GET", "/api/auth/me"),
  register: (email: string, password: string, orgName?: string) =>
    req<{ ok: boolean; email: string; username?: string; verificationRequired?: boolean }>(
      "POST",
      "/api/auth/register",
      orgName ? { email, password, orgName } : { email, password },
    ),
  login: (email: string, password: string) =>
    req<{ ok?: boolean; twoFactorRequired?: boolean; email: string; username?: string }>(
      "POST",
      "/api/auth/login",
      { email, password },
    ),
  verify2FA: (email: string, password: string, code: string) =>
    req<{ ok: boolean }>("POST", "/api/auth/2fa/verify", { email, password, code }),
  verify2FAChallenge: (code: string) =>
    req<{ ok: boolean }>("POST", "/api/auth/2fa/verify", { code }),
  forgotPassword: (email: string) => req<{ ok: boolean }>("POST", "/api/auth/forgot", { email }),
  resetPassword: (token: string, password: string) => req<{ ok: boolean }>("POST", "/api/auth/reset", { token, password }),
  changePassword: (currentPassword: string, newPassword: string) =>
    req<{ ok: boolean }>("POST", "/api/auth/password", { currentPassword, newPassword }),
  changeEmail: (newEmail: string, currentPassword?: string) =>
    req<{ ok: boolean; email: string; verificationSent?: boolean }>("PUT", "/api/auth/email", { newEmail, currentPassword }),
  resendVerification: (email: string) =>
    req<{ ok: boolean; mailConfigured: boolean }>("POST", "/api/auth/resend-verification", { email }),
  logout: () => req<{ ok: boolean }>("POST", "/api/auth/logout"),
  logoutAll: () => req<{ ok: boolean }>("POST", "/api/auth/logout-all"),
  sessions: () => req<SessionRecord[]>("GET", "/api/auth/sessions"),
  revokeSession: (id: number) => req<{ ok: boolean; self: boolean }>("DELETE", `/api/auth/sessions/${id}`),
  identities: () => req<LinkedIdentity[]>("GET", "/api/account/identities"),
  unlinkIdentity: (id: number) => req<{ ok: boolean }>("DELETE", `/api/account/identities/${id}`),
  deleteUserAccount: (confirm: string) =>
    req<{ ok: boolean }>("DELETE", "/api/account/user", { confirm }),
  acceptInvite: (token: string, password: string) =>
    req<{ ok: boolean }>("POST", "/api/auth/invite/accept", { token, password }),

  // 2FA
  twoFAStatus: () => req<{ enabled: boolean }>("GET", "/api/auth/2fa/status"),
  twoFASetup: (password: string) =>
    req<{ secret: string; otpauthUrl: string; qrDataUri?: string }>("POST", "/api/auth/2fa/setup", { password }),
  twoFAEnable: (code: string) =>
    req<{ ok: boolean; recoveryCodes: string[] }>("POST", "/api/auth/2fa/enable", { code }),
  twoFADisable: (opts: { code: string; password: string }) =>
    req<{ ok: boolean }>("POST", "/api/auth/2fa/disable", opts),

  // single-step AI assists
  aiAssistStatus: () => req<{ configured: boolean; provider: string }>("GET", "/api/ai/assist/status"),
  aiSuggestSlug: (target: string, title?: string) =>
    req<{ slugs: string[] }>("POST", "/api/ai/assist/suggest-slug", { target, title }),
  aiSummarizeEmail: (id: number) => req<{ summary: string }>("POST", `/api/ai/assist/summarize-email/${id}`),

  // domains
  dnsProviders: () => req<string[]>("GET", "/api/dns/providers"),
  providerAccounts: () => req<ProviderAccount[]>("GET", "/api/provider-accounts"),
  createProviderAccount: (p: ProviderAccountInput) => req<ProviderAccount>("POST", "/api/provider-accounts", p),
  updateProviderAccount: (id: number, p: Partial<ProviderAccountInput>) => req<ProviderAccount>("PUT", `/api/provider-accounts/${id}`, p),
  deleteProviderAccount: (id: number) => req<void>("DELETE", `/api/provider-accounts/${id}`),

  smtpSenders: () => req<SMTPSender[]>("GET", "/api/smtp-senders"),
  createSMTPSender: (s: SMTPSenderInput) => req<SMTPSender>("POST", "/api/smtp-senders", s),
  updateSMTPSender: (id: number, s: Partial<SMTPSenderInput>) => req<SMTPSender>("PUT", `/api/smtp-senders/${id}`, s),
  deleteSMTPSender: (id: number) => req<void>("DELETE", `/api/smtp-senders/${id}`),
  testSMTPSender: (id: number) => req<{ ok: boolean }>("POST", `/api/smtp-senders/${id}/test`),

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
  notificationChannelTypes: () => req<NotificationChannelType[]>("GET", "/api/notification-channel-types"),
  notificationChannels: () => req<NotificationChannel[]>("GET", "/api/notification-channels"),
  createNotificationChannel: (d: NotificationChannelInput) => req<NotificationChannel>("POST", "/api/notification-channels", d),
  updateNotificationChannel: (id: number, d: Partial<NotificationChannelInput>) => req<NotificationChannel>("PUT", `/api/notification-channels/${id}`, d),
  deleteNotificationChannel: (id: number) => req<void>("DELETE", `/api/notification-channels/${id}`),
  testNotificationChannel: (id: number) => req<void>("POST", `/api/notification-channels/${id}/test`),

  // In-App Notifications
  notifications: async (opts?: { unreadOnly?: boolean; limit?: number; offset?: number }): Promise<NotificationItem[]> => {
    const params = new URLSearchParams();
    if (opts?.unreadOnly) params.set("unread_only", "true");
    if (opts?.limit) params.set("limit", String(opts.limit));
    if (opts?.offset) params.set("offset", String(opts.offset));
    const q = params.toString();
    const raw = await req<unknown>("GET", `/api/notifications${q ? `?${q}` : ""}`);
    return parseWithFallback(NotificationsListSchema, raw, []);
  },
  markNotificationRead: async (id: number): Promise<{ ok: boolean }> => {
    const raw = await req<unknown>("PATCH", `/api/notifications/${id}/read`);
    return parseWithFallback(SimpleOKSchema, raw, { ok: true });
  },
  markAllNotificationsRead: async (): Promise<{ ok: boolean }> => {
    const raw = await req<unknown>("POST", "/api/notifications/read-all");
    return parseWithFallback(SimpleOKSchema, raw, { ok: true });
  },
  deleteNotification: async (id: number): Promise<{ ok: boolean }> => {
    const raw = await req<unknown>("DELETE", `/api/notifications/${id}`);
    return parseWithFallback(SimpleOKSchema, raw, { ok: true });
  },

  // Notification Preferences
  notificationPreferences: async (): Promise<NotificationPreferenceItem[]> => {
    const raw = await req<unknown>("GET", "/api/notifications/routes");
    return parseWithFallback(NotificationPreferencesListSchema, raw, []);
  },
  updateNotificationPreferences: async (
    preferences: { eventPattern: string; channels: string[] }[],
  ): Promise<NotificationPreferenceItem[]> => {
    const raw = await req<unknown>("PUT", "/api/notifications/routes", { preferences });
    return parseWithFallback(NotificationPreferencesListSchema, raw, []);
  },
  resetNotificationPreferences: async (): Promise<{ ok: boolean }> => {
    const raw = await req<unknown>("POST", "/api/notifications/routes/reset");
    return parseWithFallback(SimpleOKSchema, raw, { ok: true });
  },
  registeredNotificationChannels: async (): Promise<RegisteredChannel[]> => {
    const raw = await req<unknown>("GET", "/api/notification-preferences/channels");
    return parseWithFallback(RegisteredChannelsListSchema, raw, []);
  },

  // webhooks
  webhooks: () => req<Webhook[]>("GET", "/api/webhooks"),
  webhookEvents: () => req<WebhookEventGroup[]>("GET", "/api/webhooks/events"),
  createWebhook: (d: Partial<Webhook>) => req<Webhook>("POST", "/api/webhooks", d),
  updateWebhook: (id: number, d: Partial<Webhook>) => req<Webhook>("PUT", `/api/webhooks/${id}`, d),
  deleteWebhook: (id: number) => req<void>("DELETE", `/api/webhooks/${id}`),
  testWebhook: (id: number) => req<{ ok: boolean }>("POST", `/api/webhooks/${id}/test`),

  // audit
  auditLogs: (q?: ListAuditLogsParams) => {
    const params = new URLSearchParams();
    if (q?.action) params.set("action", q.action);
    if (q?.targetType) params.set("targetType", q.targetType);
    if (q?.limit !== undefined) params.set("limit", q.limit.toString());
    if (q?.offset !== undefined) params.set("offset", q.offset.toString());
    const query = params.toString();
    return req<AuditLog[]>("GET", `/api/audit${query ? "?" + query : ""}`);
  },

  // abuse
  abuseReports: (status?: string) => req<AbuseReport[]>("GET", `/api/abuse${status ? `?status=${status}` : ""}`),
  updateAbuseReport: (id: number, status: string) => req<AbuseReport>("PUT", `/api/abuse/${id}`, { status }),

  // orgs
  orgs: () => req<Org[]>("GET", "/api/orgs"),
  createOrg: (d: { name: string }) => req<Org>("POST", "/api/orgs", d),
  updateOrg: (d: { name: string }) => req<Org>("PUT", "/api/org", d),
  orgSlug: () => req<OrgSlug>("GET", "/api/org/slug"),
  updateOrgSlug: (slug: string) => req<OrgSlug>("PUT", "/api/org/slug", { slug }),
  switchOrg: (orgId: number) => req<{ ok: boolean }>("POST", "/api/auth/switch-org", { orgId }),
  orgMembers: () => req<OrgMember[]>("GET", "/api/org/members"),
  addOrgMember: (d: { email: string; role: string }) =>
    req<{ ok: boolean; emailSent?: boolean }>("POST", "/api/org/members", d),
  updateOrgMemberRole: (userId: number, role: string) =>
    req<{ ok: boolean }>("PATCH", `/api/org/members/${userId}`, { role }),
  deleteOrgMember: (userId: number) => req<void>("DELETE", `/api/org/members/${userId}`),
  resendOrgMemberInvite: (userId: number) =>
    req<{ ok: boolean; emailSent?: boolean }>("POST", `/api/org/members/${userId}/resend`),

  // menus and user settings
  menus: () => req<MenuItem[]>("GET", "/api/menus"),
  actions: () => req<Action[]>("GET", "/api/actions"),
  plugins: () => req<PluginInfo[]>("GET", "/api/plugins"),
  updatePlugin: (key: string, enabled: boolean) =>
    req<{ ok: boolean }>("PUT", `/api/plugins/${key}`, { enabled }),
  getUserSettings: () => req<Record<string, string>>("GET", "/api/user/settings"),
  updateUserSettings: (key: string, value: string) => req<{ ok: boolean }>("PUT", "/api/user/settings", { key, value }),

  // Instance backup
  downloadBackup: async (): Promise<{ blob: Blob; filename: string }> => {
    const res = await fetch("/api/admin/backup");
    if (!res.ok) {
      let msg = res.statusText;
      try {
        const parsed = await res.json();
        if (parsed?.message) msg = parsed.message;
        else if (parsed?.error) msg = parsed.error;
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
  exportWorkspaceData: () => req<Record<string, unknown>>("GET", "/api/account/export"),
  purgeWorkspaceData: () => req<void>("DELETE", "/api/account/data"),

  // Help
  helpCategories: () => req<HelpCategory[]>("GET", "/api/help/categories"),
  helpIndex: (lang?: string) => req<HelpDocMeta[]>("GET", lang ? `/api/help/docs?lang=${encodeURIComponent(lang)}` : "/api/help/docs", undefined, lang),
  helpPage: (slug: string, lang?: string) => req<HelpDocContent>("GET", lang ? `/api/help/docs/${encodeURIComponent(slug)}?lang=${encodeURIComponent(lang)}` : `/api/help/docs/${encodeURIComponent(slug)}`, undefined, lang),
};
