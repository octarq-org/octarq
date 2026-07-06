import { Domain } from "../../api";

export interface HostRow {
  host: string;
  linkEnabled: boolean | null;
  mailEnabled: boolean | null;
}

export function mergeHosts(domain: Domain): HostRow[] {
  const map = new Map<string, HostRow>();
  for (const h of domain.linkHosts ?? []) {
    map.set(h.host, { host: h.host, linkEnabled: h.enabled, mailEnabled: null });
  }
  for (const h of domain.mailHosts ?? []) {
    const v = map.get(h.host);
    if (v) v.mailEnabled = h.enabled;
    else map.set(h.host, { host: h.host, linkEnabled: null, mailEnabled: h.enabled });
  }
  return Array.from(map.values());
}
