// DTO interfaces and types for the octarq REST API.
import type {
  NotificationItem,
  NotificationPreferenceItem,
  RegisteredChannel,
} from "../pages/notifications/schemas";

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
  config: Record<string, unknown>;
  hasCredentials: boolean; // credentials are set (encrypted at rest, never returned)
  createdAt: string;
  updatedAt: string;
}

export interface ProviderAccountInput {
  name: string;
  type: string;
  config?: Record<string, unknown>;
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

export interface SMTPSenderInput {
  name: string;
  host: string;
  port: number;
  user: string;
  fromEmail: string;
  password?: string;
}

export interface NotificationChannel {
  id: number;
  name: string;
  type: string;
  config: string;
  enabled: boolean;
  createdAt: string;
}

export interface NotificationChannelInput {
  name: string;
  type: string;
  config?: string;
  enabled?: boolean;
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

export type { NotificationItem, NotificationPreferenceItem, RegisteredChannel };

export interface SessionRecord {
  id: number;
  userId: number;
  ip: string;
  userAgent: string;
  lastSeenAt: string;
  createdAt: string;
  isCurrent?: boolean;
}

/** A workspace's URL address. */
export interface OrgSlug {
  slug: string;
}

/** One external identity that can sign in as this account. */
export interface LinkedIdentity {
  id: number;
  provider: string;
  issuer: string;
  email: string;
  createdAt: string;
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

export interface ListAuditLogsParams {
  action?: string;
  targetType?: string;
  limit?: number;
  offset?: number;
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
  topLinks?: { id: number; slug: string; host: string; title: string; tags: string; clicks: number }[] | null;
  devices?: StatKV[] | null;
  countries?: StatKV[] | null;
  cities?: StatKV[] | null;
  recentEmails?: { id: number; from: string; subject: string; read: boolean; receivedAt: string }[] | null;
}

export interface Settings {
  reservedMailboxes: string;
  orgSlug: string;
  tenantSubdomain?: string;
  inboundTokenSet?: boolean;
  catchAll: boolean;
  autoWrapLinks: boolean;
  isInstanceAdmin: boolean;
}

export interface ReadinessCheck {
  id: string;
  status: "ok" | "degraded" | "blocked";
  title: string;
  detail: string;
  fixPath: string;
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
  baseDomain: string;
  sharedHosts?: string;
  metricsTokenSet: boolean;
  ratelimitAuthRpm: number;
  ratelimitApiRpm: number;
  ratelimitRedirectRpm: number;
  publicCorsOrigins?: string;
  systemSenderId?: number;
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
  area?: string;
  order?: number;
  requiredRole?: string;
}

export interface Action {
  id: string;
  label: string;
  path: string;
  icon: string;
  category: string;
  order?: number;
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
  core?: boolean;
  menus: MenuItem[];
  requires?: string[];
  requiredBy?: string[];
}

export interface Webhook {
  id: number;
  name: string;
  url: string;
  secret?: string;
  secretSet?: boolean;
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
