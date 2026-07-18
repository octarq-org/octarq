// DNS feature API surface — domain lifecycle (create/update/delete/sync), DNS
// record CRUD, and DNS verification, owned by the dns plugin.
//
// The shared `Domain` model, provider-account and SMTP-sender management, and the
// `domains` list read stay in the core api module: they are consumed by Settings
// and Overview and by the links/mail plugins, so core is their single home. This
// module imports `Domain` from there and owns only the DNS-detail types.
import { req, Domain } from "../../api";

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

export interface DNSRecordStatus {
  set: boolean;
  healthy: boolean;
  value?: string;
}

export interface DKIMStatus extends DNSRecordStatus {
  selector?: string;
}

export interface HostDNSStatus {
  host: string;
  spf: DNSRecordStatus;
  dmarc: DNSRecordStatus;
  dkim: DKIMStatus;
}

export interface LinkHostStatus {
  host: string;
  set: boolean;      // resolves (has a CNAME record)
  healthy: boolean;  // CNAME points into the domain's zone
  cname?: string;    // observed CNAME target
  target: string;    // expected target (the apex domain)
}

// verify-dns response: top-level fields describe the apex (back-compat);
// `hosts` carries per-mail-host results (subdomains included);
// `links` carries per-short-link-host CNAME resolution.
export interface DNSVerifyResult {
  spf: DNSRecordStatus;
  dmarc: DNSRecordStatus;
  dkim: DKIMStatus;
  hosts: HostDNSStatus[];
  links: LinkHostStatus[];
}

export const dnsApi = {
  syncDomains: (providerAccountId: number) =>
    req<{ ok: boolean; total: number; created: number; updated: number }>(
      "POST",
      "/api/domains/sync",
      { providerAccountId },
    ),
  createDomain: (d: any) => req<Domain>("POST", "/api/domains", d),
  updateDomain: (id: number, d: any) => req<Domain>("PUT", `/api/domains/${id}`, d),
  deleteDomain: (id: number) => req("DELETE", `/api/domains/${id}`),
  verifyDNS: (id: number) => req<DNSVerifyResult>("GET", `/api/domains/${id}/verify-dns`),
  records: (id: number) => req<DNSRecord[]>("GET", `/api/domains/${id}/records`),
  createRecord: (id: number, r: Partial<DNSRecord>) =>
    req<DNSRecord>("POST", `/api/domains/${id}/records`, r),
  updateRecord: (id: number, rid: string, r: Partial<DNSRecord>) =>
    req<DNSRecord>("PUT", `/api/domains/${id}/records/${rid}`, r),
  deleteRecord: (id: number, rid: string) => req("DELETE", `/api/domains/${id}/records/${rid}`),
};
