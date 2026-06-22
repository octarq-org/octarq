// Thin fetch wrapper around the led JSON API.

export interface Link {
  id: number;
  host: string;
  slug: string;
  target: string;
  note: string;
  title: string;
  expiresAt: string | null;
  enabled: boolean;
  clicks: number;
  hasPassword: boolean;
  createdAt: string;
}

export interface Domain {
  id: number;
  name: string;
  provider: string;
  zoneId: string;
  note: string;
  forMail: boolean;
  forLink: boolean;
  createdAt: string;
}

export interface DNSRecord {
  id: string;
  type: string;
  name: string;
  content: string;
  ttl: number;
  proxied: boolean;
  comment: string;
}

export interface Mailbox {
  id: number;
  address: string;
  note: string;
  enabled: boolean;
  unread: number;
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

export const api = {
  // auth
  me: () => req<{ username: string }>("GET", "/api/auth/me"),
  login: (username: string, password: string) =>
    req<{ ok: boolean }>("POST", "/api/auth/login", { username, password }),
  logout: () => req<{ ok: boolean }>("POST", "/api/auth/logout"),

  // links
  links: (q = "") => req<Link[]>("GET", `/api/links${q ? `?q=${encodeURIComponent(q)}` : ""}`),
  createLink: (l: Partial<Link> & { password?: string }) => req<Link>("POST", "/api/links", l),
  updateLink: (id: number, l: Partial<Link> & { password?: string }) =>
    req<Link>("PUT", `/api/links/${id}`, l),
  deleteLink: (id: number) => req("DELETE", `/api/links/${id}`),
  linkStats: (id: number, days = 30) => req<LinkStats>("GET", `/api/links/${id}/stats?days=${days}`),

  // domains
  dnsProviders: () => req<string[]>("GET", "/api/dns/providers"),
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
  sendEmail: (m: { from?: string; to: string[]; subject: string; text?: string; html?: string }) =>
    req("POST", "/api/emails/send", m),

  // api tokens
  tokens: () => req<Token[]>("GET", "/api/tokens"),
  createToken: (t: { name: string; note?: string }) =>
    req<Token & { token: string }>("POST", "/api/tokens", t),
  deleteToken: (id: number) => req("DELETE", `/api/tokens/${id}`),
};

export { ApiError };
