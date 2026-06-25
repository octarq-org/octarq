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
  createdAt: string;
  updatedAt: string;
}

export interface HostEntry {
  host: string;
  enabled: boolean;
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
  devices: StatKV[] | null;
  browsers: StatKV[] | null;
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
}

export const api = {
  // overview
  overview: () => req<Overview>("GET", "/api/overview"),

  // settings
  settings: () => req<Settings>("GET", "/api/settings"),
  updateSettings: (s: {
    reservedSlugs?: string;
    reservedMailboxes?: string;
    cloudflareToken?: string;
  }) => req<Settings>("PUT", "/api/settings", s),

  // auth
  me: () => req<{ username: string }>("GET", "/api/auth/me"),
  login: (username: string, password: string) =>
    req<{ ok: boolean }>("POST", "/api/auth/login", { username, password }),
  logout: () => req<{ ok: boolean }>("POST", "/api/auth/logout"),

  // links
  links: (params: { q?: string; tag?: string; host?: string; archived?: boolean } = {}) => {
    const sp = new URLSearchParams();
    if (params.q) sp.set("q", params.q);
    if (params.tag) sp.set("tag", params.tag);
    if (params.host) sp.set("host", params.host);
    if (params.archived) sp.set("archived", "1");
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
  deleteProviderAccount: (id: number) => req("DELETE", `/api/provider-accounts/${id}`),

  syncDomains: (providerAccountId: number) =>
    req<{ ok: boolean; total: number; created: number; updated: number }>(
      "POST",
      "/api/domains/sync",
      { providerAccountId },
    ),
  domains: () => req<Domain[]>("GET", "/api/domains"),
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
  emails: (mailbox?: number, q = "") =>
    req<Email[]>(
      "GET",
      `/api/emails?${mailbox ? `mailbox=${mailbox}&` : ""}${q ? `q=${encodeURIComponent(q)}` : ""}`,
    ),
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
  sendEmail: (m: { from?: string; to: string[]; subject: string; text?: string; html?: string }) =>
    req("POST", "/api/emails/send", m),

  // api tokens
  tokens: () => req<Token[]>("GET", "/api/tokens"),
  createToken: (t: { name: string; note?: string }) =>
    req<Token & { token: string }>("POST", "/api/tokens", t),
  deleteToken: (id: number) => req("DELETE", `/api/tokens/${id}`),
};

export { ApiError };
