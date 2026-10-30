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
  /** Wire field is `pass`; the server encrypts it at rest. */
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

// Agent Harness
export type HarnessVersionStatus = "draft" | "published" | "archived" | "discarded";

export interface HarnessSpecContent {
  version: number;
  name: string;
  preamble: string;
  extraRules: string[];
  toolAllow: string[];
  maxSteps: number;
}

// A spec on the way in. The optional fields may be left out: an omitted
// field means the same thing as an empty one.
export interface HarnessSpecInput {
  version: number;
  name: string;
  preamble?: string;
  extraRules?: string[];
  toolAllow?: string[];
  maxSteps?: number;
}

export interface HarnessActiveVersion {
  id: number;
  name: string;
  version: number;
  fingerprint: string;
  status: HarnessVersionStatus;
  createdBy: string;
  createdAt: string;
  publishedAt: string | null;
  evidence: unknown | null;
}

export interface HarnessVersionItem {
  id: number;
  name: string;
  version: number;
  fingerprint: string;
  status: HarnessVersionStatus;
  createdBy: string;
  createdAt: string;
  publishedAt: string | null;
  evidence: unknown | null;
}

export interface HarnessSpecsListResponse {
  active: HarnessActiveVersion | null;
  versions: HarnessVersionItem[];
}

export interface HarnessSpecDetail {
  id: number;
  name: string;
  version: number;
  fingerprint: string;
  status: HarnessVersionStatus;
  createdBy: string;
  createdAt: string;
  publishedAt: string | null;
  evidence: unknown | null;
  spec: HarnessSpecContent;
}

export interface HarnessProposalInput {
  spec: HarnessSpecInput;
  evidence?: unknown;
  [key: string]: unknown;
}

export type HarnessLessonStatus = "suggested" | "adopted" | "dismissed";

export interface HarnessLesson {
  id: number;
  scope: string;
  note: string;
  status: HarnessLessonStatus;
  evidence: unknown | null;
  createdBy: string;
  createdAt: string;
  specId: number | null;
}

export interface HarnessLessonsResponse {
  lessons: HarnessLesson[];
}

export interface CreateHarnessLessonInput {
  scope: string;
  note: string;
  evidence?: unknown;
}

// AI Workforce
export type WorkforceAgentStatus = "active" | "executing" | "idle" | "paused";
export type WorkforceTaskPriority = "urgent" | "high" | "normal" | "low";
export type WorkforceTaskStatus = "todo" | "in_progress" | "awaiting_approval" | "done";

export interface WorkforceAgentDTO {
  id: string;
  orgId?: number;
  name: string;
  role: string;
  department: string;
  model: string;
  status: WorkforceAgentStatus;
  heartbeat: string;
  budgetUsed: number;
  budgetLimit: number;
  // budgetPeriodStart stamps the calendar month budgetUsed covers.
  budgetPeriodStart?: string;
  capabilities: string[];
  directives: string;
  lastHeartbeatAt?: string;
  createdAt?: string;
  updatedAt?: string;
}

export interface CreateWorkforceAgentInput {
  name: string;
  role: string;
  department: string;
  model: string;
  directives?: string;
  heartbeat?: string;
  budgetLimit: number;
  capabilities?: string[];
}

export interface UpdateWorkforceAgentInput {
  name?: string;
  role?: string;
  department?: string;
  model?: string;
  directives?: string;
  heartbeat?: string;
  budgetLimit?: number;
  status?: WorkforceAgentStatus;
  capabilities?: string[];
}

export interface WorkforceTaskStepDTO {
  id: number;
  taskId: string;
  stepNumber: number;
  stepText: string;
  status: string;
  createdAt: string;
}

export interface WorkforceTaskDTO {
  id: string;
  orgId?: number;
  title: string;
  assigneeAgentId?: string;
  assignee: string;
  assigneeRole: string;
  priority: WorkforceTaskPriority;
  status: WorkforceTaskStatus;
  createdAt: string;
  lastHeartbeat: string;
  lastHeartbeatAt?: string;
  summary: string;
  context?: string;
  executionSteps?: string[];
}

export interface CreateWorkforceTaskInput {
  title: string;
  // The assignee is a real agent from this workspace; the server copies its
  // name and role onto the ticket.
  assigneeAgentId: string;
  priority?: WorkforceTaskPriority;
  summary: string;
  context?: string;
}

export interface UpdateWorkforceTaskInput {
  title?: string;
  status?: WorkforceTaskStatus;
  priority?: WorkforceTaskPriority;
  summary?: string;
  context?: string;
}

export interface WorkforceSpendAgentItem {
  name: string;
  model: string;
  spend: number;
  pct: number;
}

export interface WorkforceSpendProviderItem {
  provider: string;
  spend: number;
  pct: number;
}

// Prices are per million tokens in US dollars. A model with no price cannot
// bill a turn.
export interface WorkforceModelPriceDTO {
  model: string;
  inputPerMTok: number;
  outputPerMTok: number;
  createdAt?: string;
  updatedAt?: string;
}

export interface SetWorkforceModelPriceInput {
  model: string;
  inputPerMTok: number;
  outputPerMTok: number;
}

export type WorkforceSpendSource = "chat" | "runtime";

export interface WorkforceSpendEventDTO {
  id: number;
  agentId: string;
  model: string;
  inputTokens: number;
  outputTokens: number;
  reasoningTokens: number;
  costUsd: number;
  source: WorkforceSpendSource;
  sessionId?: string;
  /** Set on a runtime turn: the producer's identifier for it. */
  turnId?: string;
  createdAt: string;
}

export interface RecordWorkforceSpendInput {
  model: string;
  inputTokens: number;
  outputTokens: number;
  reasoningTokens?: number;
  /** The runner's identifier for this turn. A retry with the same value bills once. */
  turnId: string;
}

/** The agent a chat turn is billed to when the request names none. */
export interface WorkforceDefaultAgentDTO {
  /** Empty when the workspace bills no default. */
  agentId: string;
  /** Display name of the agent. Empty when the setting outlives its agent. */
  name: string;
}

export interface WorkforceOverviewDTO {
  activeAgents: number;
  runningTasks: number;
  queuedTasks: number;
  pendingApprovals: number;
  monthlySpend: number;
  monthlyLimit: number;
  autoHalt: boolean;
  spendByAgent?: WorkforceSpendAgentItem[];
  spendByProvider?: WorkforceSpendProviderItem[];
}

