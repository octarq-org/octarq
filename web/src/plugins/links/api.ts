// Links feature API surface — the slice of the JSON API owned by the links
// plugin. Shared primitives (the fetch client `req`, the cross-feature `Domain`
// model and its host helpers) stay in the core api module and are imported here.
import { req, StatKV } from "../../api";

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

export const linksApi = {
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
};
