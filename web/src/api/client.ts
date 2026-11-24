import { ApiError, getAppLang, req } from "@octarq/plugin-sdk";
import { parseWithFallback } from "../lib/parseWithFallback";
import {
  NotificationsListSchema,
  NotificationPreferencesListSchema,
  RegisteredChannelsListSchema,
  SimpleOKSchema,
} from "../pages/notifications/schemas";
import type {
  Action,
  AuditLog,
  Domain,
  HelpCategory,
  HelpDocContent,
  HelpDocMeta,
  HelpSearchResponse,
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
  DiscoverModelsInput,
  LLMModel,
  LLMProviderDTO,
  LLMProviderInput,
  ModelPriceDTO,
  RegisteredChannel,
  SessionRecord,
  Settings,
  SMTPSender,
  SMTPSenderInput,
  SubsystemStatusResponse,
  Token,
  VaultCategorySchema,
  VaultCollection,
  VaultCreateItemInput,
  VaultExportResult,
  VaultItemMeta,
  VaultSecretReveal,
  VaultServiceToken,
  VaultServiceTokenCreateResult,
  VaultTOTPResult,
  VaultUpdateItemInput,
  VaultVersionDTO,
  Webhook,
  WebhookEventGroup,
} from "./types";

export { ApiError, getAppLang, req } from "@octarq/plugin-sdk";

export const api = {
  subsystemStatus: () => req<SubsystemStatusResponse>("GET", "/api/status"),

  instanceReadiness: () => req<ReadinessCheck[]>("GET", "/api/instance/readiness"),

  instancePlugins: () => req<InstancePluginInfo[]>("GET", "/api/instance/plugins"),

  instanceMenus: () => req<MenuItem[]>("GET", "/api/instance/menus"),

  instanceBuild: () =>
    req<{ version: string; commit: string; builtAt: string }>("GET", "/api/instance/build"),

  overview: () => req<Overview>("GET", "/api/overview"),

  settings: () => req<Settings>("GET", "/api/settings"),
  inboundToken: () => req<{ inboundToken: string }>("GET", "/api/settings/inbound-token"),
  updateSettings: (s: { inboundToken?: string }) => req<Settings>("PUT", "/api/settings", s),

  instanceSettings: () => req<InstanceSettings>("GET", "/api/instance-settings"),
  updateInstanceSettings: (s: Partial<InstanceSettings> & {
    googleClientSecret?: string;
    githubClientSecret?: string;
    metricsToken?: string;
  }) => req<InstanceSettings>("PUT", "/api/instance-settings", s),
  testInstanceMail: (to: string) =>
    req<{ ok: boolean }>("POST", "/api/instance/mail/test", { to }),

  authConfig: () => req<{ googleEnabled: boolean; githubEnabled: boolean; registrationEnabled: boolean; appName: string; logoUrl: string; brandColor: string; brandColor2: string }>("GET", "/api/auth/config"),
  me: () => req<{ email: string; username?: string; orgId: number; role?: string; emailVerified?: boolean; isInstanceAdmin?: boolean }>("GET", "/api/auth/me"),
  register: (email: string, password: string, orgName?: string) =>
    req<{ ok: boolean; email: string; username?: string }>(
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

  twoFAStatus: () => req<{ enabled: boolean }>("GET", "/api/auth/2fa/status"),
  twoFASetup: (password: string) =>
    req<{ secret: string; otpauthUrl: string; qrDataUri?: string }>("POST", "/api/auth/2fa/setup", { password }),
  twoFAEnable: (code: string) =>
    req<{ ok: boolean; recoveryCodes: string[] }>("POST", "/api/auth/2fa/enable", { code }),
  twoFADisable: (opts: { code: string; password: string }) =>
    req<{ ok: boolean }>("POST", "/api/auth/2fa/disable", opts),

  aiAssistStatus: () => req<{ configured: boolean; provider: string }>("GET", "/api/ai/assist/status"),

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

  tokens: () => req<Token[]>("GET", "/api/tokens"),
  createToken: (d: { name: string; note?: string; role?: "member" | "admin" | "owner"; expiresInDays?: number }) =>
    req<{ token: string } & Token>("POST", "/api/tokens", d),
  updateToken: (id: number, d: { name?: string; note?: string; role?: "member" | "admin" | "owner"; expiresInDays?: number }) =>
    req<Token>("PUT", `/api/tokens/${id}`, d),
  deleteToken: (id: number) => req<void>("DELETE", `/api/tokens/${id}`),

  notificationChannelTypes: () => req<NotificationChannelType[]>("GET", "/api/notification-channel-types"),
  notificationChannels: () => req<NotificationChannel[]>("GET", "/api/notification-channels"),
  createNotificationChannel: (d: NotificationChannelInput) => req<NotificationChannel>("POST", "/api/notification-channels", d),
  updateNotificationChannel: (id: number, d: Partial<NotificationChannelInput>) => req<NotificationChannel>("PUT", `/api/notification-channels/${id}`, d),
  deleteNotificationChannel: (id: number) => req<void>("DELETE", `/api/notification-channels/${id}`),
  testNotificationChannel: (id: number) => req<void>("POST", `/api/notification-channels/${id}/test`),

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

  webhooks: () => req<Webhook[]>("GET", "/api/webhooks"),
  webhookEvents: () => req<WebhookEventGroup[]>("GET", "/api/webhooks/events"),
  createWebhook: (d: Partial<Webhook>) => req<Webhook>("POST", "/api/webhooks", d),
  updateWebhook: (id: number, d: Partial<Webhook>) => req<Webhook>("PUT", `/api/webhooks/${id}`, d),
  deleteWebhook: (id: number) => req<void>("DELETE", `/api/webhooks/${id}`),
  testWebhook: (id: number) => req<{ ok: boolean }>("POST", `/api/webhooks/${id}/test`),
  webhookRelayEndpoints: () => req<any[]>("GET", "/api/webhookrelay/endpoints"),
  webhookRelayDeadLetters: () => req<any[]>("GET", "/api/webhookrelay/dead-letters"),
  replayWebhookDeadLetter: (id: number) => req<any>("POST", `/api/webhookrelay/dead-letters/${id}/replay`),

  auditLogs: (q?: ListAuditLogsParams) => {
    const params = new URLSearchParams();
    if (q?.action) params.set("action", q.action);
    if (q?.targetType) params.set("targetType", q.targetType);
    if (q?.limit !== undefined) params.set("limit", q.limit.toString());
    if (q?.offset !== undefined) params.set("offset", q.offset.toString());
    const query = params.toString();
    return req<AuditLog[]>("GET", `/api/audit${query ? "?" + query : ""}`);
  },


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

  menus: () => req<MenuItem[]>("GET", "/api/menus"),
  actions: () => req<Action[]>("GET", "/api/actions"),
  plugins: () => req<PluginInfo[]>("GET", "/api/plugins"),
  updatePlugin: (key: string, enabled: boolean) =>
    req<{ ok: boolean }>("PUT", `/api/plugins/${key}`, { enabled }),
  getUserSettings: () => req<Record<string, string>>("GET", "/api/user/settings"),
  updateUserSettings: (key: string, value: string) => req<{ ok: boolean }>("PUT", "/api/user/settings", { key, value }),

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
        msg = res.statusText;
      }
      throw new ApiError(res.status, msg);
    }
    const cd = res.headers.get("content-disposition") || "";
    const m = /filename="?([^"]+)"?/.exec(cd);
    return { blob: await res.blob(), filename: m?.[1] || "octarq-backup" };
  },

  exportWorkspaceData: async (): Promise<{ blob: Blob; filename: string }> => {
    const res = await fetch("/api/account/export", {
      headers: {
        "Accept-Language": getAppLang(),
      },
    });
    if (!res.ok) {
      let msg = res.statusText;
      try {
        const parsed = await res.json();
        if (parsed?.message) msg = parsed.message;
        else if (parsed?.error) msg = parsed.error;
        else if (parsed?.detail) msg = parsed.detail;
      } catch {
        msg = res.statusText;
      }
      throw new ApiError(res.status, msg);
    }
    const cd = res.headers.get("content-disposition") || "";
    const m = /filename="?([^"]+)"?/.exec(cd);
    return { blob: await res.blob(), filename: m?.[1] || `workspace-export-${new Date().toISOString().split("T")[0]}.zip` };
  },
  purgeWorkspaceData: () => req<void>("DELETE", "/api/account/data"),

  helpCategories: () => req<HelpCategory[]>("GET", "/api/help/categories"),
  helpIndex: (lang?: string) => req<HelpDocMeta[]>("GET", lang ? `/api/help/docs?lang=${encodeURIComponent(lang)}` : "/api/help/docs", undefined, lang),
  helpPage: (slug: string, lang?: string) => req<HelpDocContent>("GET", lang ? `/api/help/docs/${encodeURIComponent(slug)}?lang=${encodeURIComponent(lang)}` : `/api/help/docs/${encodeURIComponent(slug)}`, undefined, lang),
  helpSearch: (q: string, lang?: string, limit = 20, category?: string, signal?: AbortSignal) => {
    const params = new URLSearchParams({ q });
    if (lang) params.set("lang", lang);
    if (limit) params.set("limit", String(limit));
    if (category) params.set("category", category);
    return req<HelpSearchResponse>("GET", `/api/help/search?${params.toString()}`, undefined, lang, signal);
  },

  llmBackends: async (): Promise<string[]> => {
    const res = await req<{ backends?: string[] }>("GET", "/api/llm-gateway/backends");
    return res.backends ?? [];
  },
  llmProviders: async (): Promise<LLMProviderDTO[]> => {
    const res = await req<{ providers?: LLMProviderDTO[] }>("GET", "/api/llm-gateway/providers");
    return res.providers ?? [];
  },
  createLLMProvider: (input: LLMProviderInput): Promise<LLMProviderDTO> =>
    req<{ provider: LLMProviderDTO }>("POST", "/api/llm-gateway/providers", input).then((r) => r.provider),
  updateLLMProvider: (id: number, input: LLMProviderInput): Promise<LLMProviderDTO> =>
    req<{ provider: LLMProviderDTO }>("PUT", `/api/llm-gateway/providers/${id}`, input).then((r) => r.provider),
  deleteLLMProvider: (id: number): Promise<{ ok: boolean }> =>
    req<{ ok: boolean }>("DELETE", `/api/llm-gateway/providers/${id}`),
  setDefaultLLMProvider: (id: number): Promise<{ ok: boolean }> =>
    req<{ ok: boolean }>("POST", `/api/llm-gateway/providers/${id}/default`),
  testLLMProvider: (id: number): Promise<{ ok: boolean; model: string }> =>
    req<{ ok: boolean; model: string }>("POST", `/api/llm-gateway/providers/${id}/test`),
  discoverModels: async (input: DiscoverModelsInput): Promise<LLMModel[]> => {
    const res = await req<{ models?: LLMModel[] }>("POST", "/api/llm-gateway/models", input);
    return res.models ?? [];
  },
  modelPrices: async (): Promise<ModelPriceDTO[]> => {
    const res = await req<{ prices?: ModelPriceDTO[] }>("GET", "/api/llm-gateway/prices");
    return res.prices ?? [];
  },

  vaultSchemas: () => req<VaultCategorySchema[]>("GET", "/api/vault/schemas"),
  vaultCollections: () => req<VaultCollection[]>("GET", "/api/vault/collections"),
  createVaultCollection: (d: { name: string; slug?: string; kind?: string; description?: string }) =>
    req<VaultCollection>("POST", "/api/vault/collections", d),
  deleteVaultCollection: (id: number) =>
    req<{ ok: boolean; message: string }>("DELETE", `/api/vault/collections/${id}`),
  vaultItems: (q?: {
    collection?: string;
    environment?: string;
    folder_path?: string;
    item_type?: string;
    provider_type?: string;
    provider?: string;
    scope?: string;
    q?: string;
  }) => {
    const params = new URLSearchParams();
    if (q?.collection) params.set("collection", q.collection);
    if (q?.environment) params.set("environment", q.environment);
    if (q?.folder_path) params.set("folder_path", q.folder_path);
    if (q?.item_type) params.set("item_type", q.item_type);
    if (q?.provider_type) params.set("provider_type", q.provider_type);
    if (q?.provider) params.set("provider", q.provider);
    if (q?.scope) params.set("scope", q.scope);
    if (q?.q) params.set("q", q.q);
    const qs = params.toString();
    return req<VaultItemMeta[]>("GET", `/api/vault/items${qs ? `?${qs}` : ""}`);
  },
  createVaultItem: (d: VaultCreateItemInput) => req<VaultItemMeta>("POST", "/api/vault/items", d),
  updateVaultItem: (id: number, d: VaultUpdateItemInput) =>
    req<VaultItemMeta>("PUT", `/api/vault/items/${id}`, d),
  deleteVaultItem: (id: number) =>
    req<{ ok: boolean; message: string }>("DELETE", `/api/vault/items/${id}`),
  revealVaultItem: (id: number) => req<VaultSecretReveal>("POST", `/api/vault/items/${id}/reveal`),
  vaultItemVersions: (id: number) => req<VaultVersionDTO[]>("GET", `/api/vault/items/${id}/versions`),
  rollbackVaultItem: (id: number, version: number) =>
    req<VaultItemMeta>("POST", `/api/vault/items/${id}/rollback/${version}`),
  setDefaultVaultItem: (id: number) => req<VaultItemMeta>("POST", `/api/vault/items/${id}/default`),
  testVaultItem: (id: number) => req<{ ok: boolean; message: string }>("POST", `/api/vault/items/${id}/test`),
  vaultItemTOTP: (id: number) => req<VaultTOTPResult>("GET", `/api/vault/items/${id}/totp`),
  importVaultSecrets: (d: {
    collectionSlug?: string;
    environment?: string;
    folderPath?: string;
    format: "env" | "json";
    dotEnv?: string;
    jsonItems?: Record<string, unknown>;
  }) => req<{ imported: number }>("POST", "/api/vault/import", d),
  exportVaultSecrets: (q: { collection?: string; environment?: string; format: "env" | "json" }) => {
    const params = new URLSearchParams();
    if (q.collection) params.set("collection", q.collection);
    if (q.environment) params.set("environment", q.environment);
    params.set("format", q.format);
    return req<VaultExportResult>("GET", `/api/vault/export?${params.toString()}`);
  },
  vaultTokens: () => req<VaultServiceToken[]>("GET", "/api/vault/tokens"),
  createVaultToken: (d: {
    name: string;
    collectionSlug?: string;
    environment?: string;
    folderPrefix?: string;
    permissions?: string;
    expiresAt?: string;
  }) => req<VaultServiceTokenCreateResult>("POST", "/api/vault/tokens", d),
  revokeVaultToken: (id: number) =>
    req<{ ok: boolean; message: string }>("DELETE", `/api/vault/tokens/${id}`),

  instanceVaultItems: () => req<VaultItemMeta[]>("GET", "/api/instance/vault/items"),
  putInstanceVaultItem: (d: VaultCreateItemInput) =>
    req<VaultItemMeta>("POST", "/api/instance/vault/items", d),
  revealInstanceVaultItem: (id: number) =>
    req<VaultSecretReveal>("POST", `/api/instance/vault/items/${id}/reveal`),
  deleteInstanceVaultItem: (id: number) =>
    req<{ ok: boolean; message: string }>("DELETE", `/api/instance/vault/items/${id}`),
};
