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
  linkHosts: HostEntry[] | null;
  mailHosts: HostEntry[] | null;
  createdAt: string;
}

export interface ProviderAccount {
  id: number;
  name: string;
  type: string;
  config: Record<string, unknown>;
  hasCredentials: boolean;
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
  passSet: boolean;
  createdAt: string;
}

export interface SMTPSenderInput {
  name: string;
  host: string;
  port: number;
  user: string;
  fromEmail: string;
  pass?: string;
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

export interface OrgSlug {
  slug: string;
}

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
  description?: string;
  category: string;
  order: number;
  routes?: string[];
  actions?: string[];
}

export interface HelpDocContent {
  title: string;
  html: string;
}

export interface HelpSearchSnippet {
  text: string;
  offsets: [number, number][];
}

export interface HelpSearchHit {
  slug: string;
  title: string;
  category: string;
  anchor: string;
  anchorTitle: string;
  snippet: HelpSearchSnippet;
  score: number;
  url: string;
}

export interface HelpSearchResponse {
  query: string;
  lang: string;
  results: HelpSearchHit[];
}

export interface Token {
  id: number;
  name: string;
  prefix: string;
  note: string;
  userId: number;
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
  version?: string;
  revision?: string;
}

export interface Overview extends Record<string, unknown> {
  tokens?: number;
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
}

export interface Settings {
  orgSlug: string;
  tenantSubdomain?: string;
  inboundTokenSet?: boolean;
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
  publicCorsOrigins?: string;
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
  external?: boolean;
}

export interface Action {
  id: string;
  label: string;
  description?: string;
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

export interface LLMProviderDTO {
  id: number;
  name: string;
  backend: string;
  baseUrl: string;
  model: string;
  cheapModel: string;
  isDefault: boolean;
  hasKey: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface LLMProviderInput {
  name: string;
  backend: string;
  apiKey?: string;
  baseUrl?: string;
  model?: string;
  cheapModel?: string;
}

export interface LLMModel {
  id: string;
  object?: string;
  owned_by?: string;
}

export interface DiscoverModelsInput {
  providerId?: number;
  backend?: string;
  baseUrl?: string;
  apiKey?: string;
}

export interface ModelPriceDTO {
  model: string;
  displayName: string;
  providerId: string;
  input: number;
  output: number;
  reasoning: number;
  cacheRead: number;
  cacheWrite: number;
}

export interface VaultSchemaField {
  key: string;
  label: string;
  kind: "text" | "secret" | "textarea" | "boolean" | "number" | "url" | string;
  required?: boolean;
  secret?: boolean;
  default?: unknown;
  hint?: string;
  nonSecret?: boolean;
}

export interface VaultProviderSchema {
  id: string;
  label: string;
  itemType: string;
  providerType?: string;
  testable?: boolean;
  fields: VaultSchemaField[];
}

export interface VaultCategorySchema {
  key: string;
  label: string;
  itemType: string;
  providers: VaultProviderSchema[];
}

export interface VaultCollection {
  id: number;
  orgId: number;
  scope: "instance" | "org" | "user";
  slug: string;
  name: string;
  kind: string;
  description?: string;
  icon?: string;
  createdBy?: number;
  createdAt: string;
  updatedAt: string;
}

export interface VaultItemMeta {
  id: number;
  secretId: string;
  orgId: number;
  scope: "instance" | "org" | "user";
  ownerUserId?: number;
  collectionId: number;
  collectionSlug: string;
  environment: string;
  folderPath: string;
  itemType: string;
  providerType?: string;
  provider?: string;
  keyName?: string;
  name: string;
  tags?: string[];
  isDefault: boolean;
  currentVersion: number;
  configured: boolean;
  fieldKeys?: string[];
  metadata?: Record<string, unknown>;
  expiresAt?: string;
  rotationIntervalDays?: number;
  lastRotatedAt?: string;
  ref: string;
  createdAt: string;
  updatedAt: string;
}

export interface VaultSecretReveal {
  meta: VaultItemMeta;
  version: number;
  payload: Record<string, unknown>;
}

export interface VaultVersionDTO {
  version: number;
  createdBy?: number;
  changeNote?: string;
  createdAt: string;
}

export interface VaultServiceToken {
  id: number;
  orgId: number;
  name: string;
  tokenPrefix: string;
  collectionId: number;
  environment: string;
  folderPrefix: string;
  permissions: string;
  expiresAt?: string;
  lastUsedAt?: string;
  revokedAt?: string;
  createdBy?: number;
  createdAt: string;
}

export interface VaultServiceTokenCreateResult {
  token: VaultServiceToken;
  plaintext: string;
}

export interface VaultTOTPResult {
  code: string;
  remainingSeconds: number;
}

export interface VaultExportResult {
  format: string;
  dotEnv?: string;
  secrets?: Record<string, unknown>;
  refs?: Record<string, string>;
}

export interface VaultCreateItemInput {
  scope?: string;
  collectionSlug?: string;
  environment?: string;
  folderPath?: string;
  itemType?: string;
  providerType?: string;
  provider?: string;
  keyName?: string;
  name: string;
  tags?: string[];
  isDefault?: boolean;
  metadata?: Record<string, unknown>;
  payload: Record<string, unknown>;
  changeNote?: string;
  expiresAt?: string;
  rotationIntervalDays?: number;
}

export interface VaultUpdateItemInput {
  name?: string;
  isDefault?: boolean;
  metadata?: Record<string, unknown>;
  payload?: Record<string, unknown>;
  changeNote?: string;
}
