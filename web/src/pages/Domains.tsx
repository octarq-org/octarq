import { useEffect, useState } from "react";
import { api, DNSRecord, Domain, effectiveLinkHosts, effectiveMailHosts } from "../api";
import { Code, Empty, Field, Guide, HostList, Modal, Toggle } from "../ui";
import { Header } from "./Links";

export default function DomainsPage() {
  const [domains, setDomains] = useState<Domain[]>([]);
  const [providers, setProviders] = useState<string[]>([]);
  const [editing, setEditing] = useState<Domain | "new" | null>(null);
  const [recordsFor, setRecordsFor] = useState<Domain | null>(null);
  const [syncing, setSyncing] = useState(false);

  async function load() {
    setDomains(await api.domains());
  }
  useEffect(() => {
    load();
    api.dnsProviders().then(setProviders).catch(() => setProviders(["cloudflare"]));
  }, []);

  return (
    <div>
      <Header title="Domains" subtitle="Sync & manage DNS records across providers">
        <div className="flex gap-2">
          <button className="btn-ghost" onClick={() => setSyncing(true)}>
            ⟳ Sync from Cloudflare
          </button>
          <button className="btn-primary" onClick={() => setEditing("new")}>
            + Add domain
          </button>
        </div>
      </Header>

      {domains.length === 0 ? (
        <Empty>
          <span className="text-3xl">🌐</span>
          <p>No domains yet</p>
        </Empty>
      ) : (
        <div className="card divide-y divide-zinc-800">
          {domains.map((d) => (
            <div key={d.id} className="flex items-center gap-3 p-3">
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <span className="font-medium">{d.name}</span>
                  <span className="badge">{d.provider}</span>
                  {effectiveLinkHosts(d).map((h) => (
                    <span key={h} className="badge bg-indigo-500/15 text-indigo-300" title="short-link host">
                      🔗 {h}
                    </span>
                  ))}
                  {effectiveMailHosts(d).map((h) => (
                    <span key={h} className="badge bg-emerald-500/15 text-emerald-300" title="mail host">
                      ✉️ {h}
                    </span>
                  ))}
                </div>
                {d.note && <div className="mt-0.5 truncate text-xs text-amber-300/80">📝 {d.note}</div>}
              </div>
              <button className="btn-ghost" onClick={() => setRecordsFor(d)}>
                DNS records
              </button>
              <button className="btn-ghost" onClick={() => setEditing(d)}>
                Edit
              </button>
              <button
                className="btn-danger"
                onClick={async () => {
                  if (confirm(`Remove ${d.name}?`)) {
                    await api.deleteDomain(d.id);
                    load();
                  }
                }}
              >
                Del
              </button>
            </div>
          ))}
        </div>
      )}

      {editing && (
        <DomainEditor
          domain={editing === "new" ? null : editing}
          providers={providers}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            load();
          }}
        />
      )}
      {recordsFor && <RecordsModal domain={recordsFor} onClose={() => setRecordsFor(null)} />}
      {syncing && (
        <SyncModal
          onClose={() => setSyncing(false)}
          onSynced={() => {
            setSyncing(false);
            load();
          }}
        />
      )}
    </div>
  );
}

// SyncModal imports every Cloudflare zone the token can access in one click.
function SyncModal({ onClose, onSynced }: { onClose: () => void; onSynced: () => void }) {
  const [apiToken, setApiToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [result, setResult] = useState<{ total: number; created: number; updated: number } | null>(null);

  async function run() {
    setBusy(true);
    setErr("");
    try {
      const r = await api.syncDomains("cloudflare", { apiToken });
      setResult(r);
    } catch (e: any) {
      setErr(e.message ?? "sync failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal title="Sync from Cloudflare" onClose={onClose}>
      {result ? (
        <div className="py-4 text-center">
          <p className="mb-2 text-2xl">✅</p>
          <p className="text-zinc-300">
            {result.total} zones — <span className="text-green-400">{result.created} new</span>,{" "}
            <span className="text-indigo-300">{result.updated} updated</span>.
          </p>
          <p className="mt-2 text-xs text-zinc-500">
            Toggle <b>links</b> / <b>mail</b> on each domain to enable those services.
          </p>
          <button className="btn-primary mt-4" onClick={onSynced}>
            Done
          </button>
        </div>
      ) : (
        <>
          <p className="mb-3 text-sm text-zinc-400">
            Paste a Cloudflare API token with <b>Zone:Read</b> + <b>DNS:Edit</b>. led pulls every
            zone and stores the token (encrypted) for managing their records.
          </p>
          <Guide title="How to create the Cloudflare API token">
            <ol className="ml-4 list-decimal space-y-1">
              <li>
                Cloudflare dashboard → <b>My Profile → API Tokens → Create Token</b>.
              </li>
              <li>
                Use the <b>Edit zone DNS</b> template, or a custom token with permissions{" "}
                <Code>Zone · DNS · Edit</Code> and <Code>Zone · Zone · Read</Code>.
              </li>
              <li>
                Under <b>Zone Resources</b> choose <i>All zones</i> (so sync sees every domain), then
                create and copy the token.
              </li>
            </ol>
          </Guide>
          <Field label="Cloudflare API Token">
            <input
              className="input"
              value={apiToken}
              onChange={(e) => setApiToken(e.target.value)}
              placeholder="••••••••••••"
              autoFocus
            />
          </Field>
          {err && <p className="mb-3 text-sm text-red-400">{err}</p>}
          <div className="flex justify-end gap-2">
            <button className="btn-ghost" onClick={onClose}>
              Cancel
            </button>
            <button className="btn-primary" onClick={run} disabled={busy || !apiToken}>
              {busy ? "Syncing…" : "Sync"}
            </button>
          </div>
        </>
      )}
    </Modal>
  );
}

function DomainEditor({
  domain,
  providers,
  onClose,
  onSaved,
}: {
  domain: Domain | null;
  providers: string[];
  onClose: () => void;
  onSaved: () => void;
}) {
  const [name, setName] = useState(domain?.name ?? "");
  const [provider, setProvider] = useState(domain?.provider ?? providers[0] ?? "cloudflare");
  const [zoneId, setZoneId] = useState(domain?.zoneId ?? "");
  const [apiToken, setApiToken] = useState("");
  const [note, setNote] = useState(domain?.note ?? "");
  const [forLink, setForLink] = useState(domain?.forLink ?? false);
  const [forMail, setForMail] = useState(domain?.forMail ?? false);
  const [linkHosts, setLinkHosts] = useState<string[]>(domain?.linkHosts ?? []);
  const [mailHosts, setMailHosts] = useState<string[]>(domain?.mailHosts ?? []);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  // When enabling short links, seed a "go." subdomain rather than the apex.
  function enableLinks(on: boolean) {
    setForLink(on);
    if (on && linkHosts.length === 0 && name) setLinkHosts([`go.${name}`]);
  }
  function enableMail(on: boolean) {
    setForMail(on);
    if (on && mailHosts.length === 0 && name) setMailHosts([name]);
  }
  const linkSubs = name ? [`go.${name}`, `s.${name}`, `link.${name}`, name] : [];
  const mailSubs = name ? [name, `mail.${name}`] : [];

  async function save() {
    setErr("");
    setBusy(true);
    const payload: any = { name, provider, zoneId, note, forLink, forMail, linkHosts, mailHosts };
    if (apiToken) payload.config = { apiToken };
    try {
      if (domain) await api.updateDomain(domain.id, payload);
      else await api.createDomain(payload);
      onSaved();
    } catch (e: any) {
      setErr(e.message ?? "save failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal title={domain ? "Edit domain" : "Add domain"} onClose={onClose}>
      <Field label="Domain name">
        <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="example.com" disabled={!!domain} />
      </Field>
      <div className="grid grid-cols-2 gap-3">
        <Field label="Provider">
          <select className="input" value={provider} onChange={(e) => setProvider(e.target.value)}>
            {providers.map((p) => (
              <option key={p} value={p}>
                {p}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Zone ID">
          <input className="input" value={zoneId} onChange={(e) => setZoneId(e.target.value)} />
        </Field>
      </div>
      <Field label="API Token" hint={domain ? "leave blank to keep existing" : "Cloudflare API token with DNS edit"}>
        <input className="input" value={apiToken} onChange={(e) => setApiToken(e.target.value)} placeholder="••••••••" />
      </Field>
      <Field label="Note">
        <textarea className="input" rows={2} value={note} onChange={(e) => setNote(e.target.value)} />
      </Field>
      <div className="mb-3 flex gap-6">
        <label className="flex items-center gap-2 text-sm text-zinc-400">
          <Toggle on={forLink} onChange={enableLinks} /> Serve short links
        </label>
        <label className="flex items-center gap-2 text-sm text-zinc-400">
          <Toggle on={forMail} onChange={enableMail} /> Accept email
        </label>
      </div>
      {forLink && (
        <>
          <Field label="Short-link hosts" hint="one or more — each must resolve to this server">
            <HostList hosts={linkHosts} onChange={setLinkHosts} suggestions={linkSubs} placeholder="go.example.com" />
          </Field>
          <Guide title="How to point a short-link host at led">
            <p>Each host above must resolve to the machine running led. In your DNS provider create:</p>
            <ul className="ml-4 list-disc space-y-1">
              <li>
                a <b>CNAME</b> from the subdomain (e.g. <Code>go</Code>) to this dashboard's host{" "}
                <Code>{location.hostname}</Code> (Cloudflare-proxied is fine), <i>or</i>
              </li>
              <li>
                an <b>A</b> record from the subdomain to your server's public IP.
              </li>
            </ul>
            <p>
              On Cloudflare zones you can do this in one click: open <b>DNS records → + Subdomain → 🔗
              Short-link subdomain</b>, then make sure led terminates TLS for the host (or sits behind a
              reverse proxy / Cloudflare that does).
            </p>
          </Guide>
        </>
      )}
      {forMail && (
        <>
          <Field label="Mail hosts" hint="domains/subdomains mailboxes live under, e.g. mail.example.com">
            <HostList hosts={mailHosts} onChange={setMailHosts} suggestions={mailSubs} placeholder="mail.example.com" />
          </Field>
          <Guide title="How to receive mail on these hosts (Cloudflare)">
            <ol className="ml-4 list-decimal space-y-1">
              <li>
                In Cloudflare → <b>Email → Email Routing</b>, enable routing for the zone (this adds the
                required MX + SPF records automatically).
              </li>
              <li>
                Deploy the worker in <Code>deploy/cloudflare-email-worker.js</Code> and set its vars{" "}
                <Code>LED_ENDPOINT</Code>=<Code>{`${location.origin}/api/email/inbound`}</Code> and{" "}
                <Code>LED_TOKEN</Code> = your <Code>LED_INBOUND_TOKEN</Code>.
              </li>
              <li>
                Point a <b>catch-all</b> route at that worker. Mail to any address on these hosts then
                lands in led (auto-creating a mailbox when catch-all is on).
              </li>
            </ol>
          </Guide>
        </>
      )}
      {err && <p className="mb-3 text-sm text-red-400">{err}</p>}
      <div className="flex justify-end gap-2">
        <button className="btn-ghost" onClick={onClose}>
          Cancel
        </button>
        <button className="btn-primary" onClick={save} disabled={busy}>
          {busy ? "…" : "Save"}
        </button>
      </div>
    </Modal>
  );
}

const RECORD_TYPES = ["A", "AAAA", "CNAME", "TXT", "MX", "NS", "CAA"];

function RecordsModal({ domain, onClose }: { domain: Domain; onClose: () => void }) {
  const [records, setRecords] = useState<DNSRecord[] | null>(null);
  const [err, setErr] = useState("");
  const [editing, setEditing] = useState<DNSRecord | "new" | "subdomain" | null>(null);
  const [typeFilter, setTypeFilter] = useState("");
  const [search, setSearch] = useState("");

  async function load() {
    setErr("");
    try {
      setRecords(await api.records(domain.id));
    } catch (e: any) {
      setErr(e.message ?? "failed to load records");
      setRecords([]);
    }
  }
  useEffect(() => {
    load();
  }, [domain.id]);

  const filtered = (records ?? []).filter((r) => {
    if (typeFilter && r.type !== typeFilter) return false;
    if (search) {
      const s = search.toLowerCase();
      return (
        r.name.toLowerCase().includes(s) ||
        r.content.toLowerCase().includes(s) ||
        (r.comment ?? "").toLowerCase().includes(s)
      );
    }
    return true;
  });
  const presentTypes = Array.from(new Set((records ?? []).map((r) => r.type)));

  return (
    <Modal title={`DNS · ${domain.name}`} onClose={onClose} wide>
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <select className="input max-w-[110px]" value={typeFilter} onChange={(e) => setTypeFilter(e.target.value)}>
          <option value="">All types</option>
          {presentTypes.map((t) => (
            <option key={t} value={t}>
              {t}
            </option>
          ))}
        </select>
        <input
          className="input flex-1"
          placeholder="Filter name / content / note…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <button className="btn-ghost shrink-0" onClick={() => setEditing("subdomain")} title="Quick subdomain for link/mail">
          + Subdomain
        </button>
        <button className="btn-primary shrink-0" onClick={() => setEditing("new")}>
          + Record
        </button>
      </div>
      <p className="mb-2 text-xs text-zinc-500">
        Notes map to the provider's native record comment · {filtered.length}/{records?.length ?? 0} shown
      </p>
      {err && <p className="mb-3 rounded bg-red-500/10 p-2 text-sm text-red-400">{err}</p>}
      {records === null ? (
        <p className="text-zinc-500">loading…</p>
      ) : filtered.length === 0 ? (
        <p className="text-zinc-500">No matching records.</p>
      ) : (
        <div className="max-h-96 overflow-y-auto">
          <table className="w-full text-sm">
            <thead className="text-left text-xs uppercase text-zinc-500">
              <tr>
                <th className="py-1">Type</th>
                <th>Name</th>
                <th>Content</th>
                <th>Note</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((r) => (
                <tr key={r.id} className="border-t border-zinc-800">
                  <td className="py-1.5">
                    <span className="badge">{r.type}</span>
                    {r.proxied && <span className="ml-1 text-orange-400" title="proxied">☁</span>}
                  </td>
                  <td className="max-w-[120px] truncate">{r.name}</td>
                  <td className="max-w-[160px] truncate text-zinc-400">{r.content}</td>
                  <td className="max-w-[120px] truncate text-amber-300/80">{r.comment}</td>
                  <td className="text-right">
                    <button className="btn-ghost px-2" onClick={() => setEditing(r)}>
                      Edit
                    </button>
                    <button
                      className="btn-danger px-2"
                      onClick={async () => {
                        if (confirm(`Delete ${r.type} ${r.name}?`)) {
                          await api.deleteRecord(domain.id, r.id);
                          load();
                        }
                      }}
                    >
                      Del
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {editing && (
        <RecordEditor
          domainId={domain.id}
          domainName={domain.name}
          linkHost={domain.linkHosts?.[0] ?? ""}
          record={typeof editing === "string" ? null : editing}
          subdomain={editing === "subdomain"}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            load();
          }}
        />
      )}
    </Modal>
  );
}

function RecordEditor({
  domainId,
  domainName,
  linkHost,
  record,
  subdomain,
  onClose,
  onSaved,
}: {
  domainId: number;
  domainName: string;
  linkHost?: string;
  record: DNSRecord | null;
  subdomain?: boolean;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [type, setType] = useState(record?.type ?? "A");
  const [name, setName] = useState(record?.name ?? "");
  const [content, setContent] = useState(record?.content ?? "");
  const [comment, setComment] = useState(record?.comment ?? "");
  const [proxied, setProxied] = useState(record?.proxied ?? false);
  const [priority, setPriority] = useState<number>(record?.priority ?? 10);
  const [err, setErr] = useState("");

  const needsPriority = ["MX", "SRV", "URI"].includes(type.toUpperCase());
  const canProxy = ["A", "AAAA", "CNAME"].includes(type.toUpperCase());
  const contentHint: Record<string, string> = {
    A: "IPv4 address, e.g. 203.0.113.10",
    AAAA: "IPv6 address",
    CNAME: "target hostname, e.g. example.com",
    TXT: "text value",
    MX: "mail server hostname",
    NS: "nameserver hostname",
    CAA: "e.g. 0 issue \"letsencrypt.org\"",
  };

  // The record name for the configured short-link host (e.g. linkHost
  // "go.example.com" on zone "example.com" -> "go"; apex -> "@").
  const linkSub =
    linkHost && linkHost.endsWith("." + domainName)
      ? linkHost.slice(0, -(domainName.length + 1))
      : linkHost === domainName
        ? "@"
        : "go";

  // Subdomain presets: one click to set up a host for short links or email.
  function preset(kind: "link" | "mail") {
    if (kind === "link") {
      setType("CNAME");
      setName(name || linkSub);
      setContent(domainName); // CNAME to apex; point apex at your led host
      setComment("led short-link host");
      setProxied(true);
    } else {
      setType("MX");
      setName(name || "mail");
      setContent("route1.mx.cloudflare.net");
      setComment("led mailbox (Cloudflare Email Routing)");
      setProxied(false);
      setPriority(10);
    }
  }

  async function save() {
    setErr("");
    const payload: Partial<DNSRecord> = {
      type,
      name,
      content,
      comment,
      proxied: canProxy ? proxied : false,
      ttl: 1,
    };
    if (needsPriority) payload.priority = Number(priority);
    try {
      if (record) await api.updateRecord(domainId, record.id, payload);
      else await api.createRecord(domainId, payload);
      onSaved();
    } catch (e: any) {
      setErr(e.message ?? "save failed");
    }
  }

  return (
    <Modal title={record ? "Edit record" : subdomain ? "New subdomain" : "New record"} onClose={onClose}>
      {subdomain && (
        <div className="mb-3 flex gap-2">
          <button className="btn-ghost flex-1" onClick={() => preset("link")}>
            🔗 Short-link subdomain
          </button>
          <button className="btn-ghost flex-1" onClick={() => preset("mail")}>
            ✉️ Email subdomain
          </button>
        </div>
      )}
      <div className="grid grid-cols-2 gap-3">
        <Field label="Type">
          <select className="input" value={type} onChange={(e) => setType(e.target.value)}>
            {RECORD_TYPES.map((t) => (
              <option key={t}>{t}</option>
            ))}
          </select>
        </Field>
        <Field label="Name">
          <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="@ or sub" />
        </Field>
      </div>
      <div className={needsPriority ? "grid grid-cols-[1fr_100px] gap-3" : ""}>
        <Field label="Content" hint={contentHint[type.toUpperCase()]}>
          <input className="input" value={content} onChange={(e) => setContent(e.target.value)} />
        </Field>
        {needsPriority && (
          <Field label="Priority">
            <input
              type="number"
              min={0}
              className="input"
              value={priority}
              onChange={(e) => setPriority(Number(e.target.value))}
            />
          </Field>
        )}
      </div>
      <Field label="Note (comment)">
        <input className="input" value={comment} onChange={(e) => setComment(e.target.value)} />
      </Field>
      {canProxy && (
        <label className="mb-4 flex items-center gap-2 text-sm text-zinc-400">
          <Toggle on={proxied} onChange={setProxied} /> Proxied (Cloudflare)
        </label>
      )}
      {err && <p className="mb-3 text-sm text-red-400">{err}</p>}
      <div className="flex justify-end gap-2">
        <button className="btn-ghost" onClick={onClose}>
          Cancel
        </button>
        <button className="btn-primary" onClick={save}>
          Save
        </button>
      </div>
    </Modal>
  );
}
