import { useEffect, useMemo, useState } from "react";
import { api, DNSRecord, Domain, HostEntry, ProviderAccount } from "../api";
import { Code, Empty, Field, Guide, HostList, Modal, Toggle } from "../ui";
import { Header } from "./Links";

// ─── unified host view ────────────────────────────────────────────────────────
// Merges linkHosts + mailHosts into one list so each hostname appears once.
interface HostRow {
  host: string;
  linkEnabled: boolean | null; // null = not in linkHosts at all
  mailEnabled: boolean | null; // null = not in mailHosts at all
}

function mergeHosts(domain: Domain): HostRow[] {
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

// ─── page ─────────────────────────────────────────────────────────────────────
export default function DomainsPage() {
  const [domains, setDomains] = useState<Domain[]>([]);
  const [accounts, setAccounts] = useState<ProviderAccount[]>([]);
  const [editing, setEditing] = useState<Domain | "new" | null>(null);
  const [recordsFor, setRecordsFor] = useState<Domain | null>(null);
  const [syncing, setSyncing] = useState(false);

  async function load() {
    setDomains(await api.domains());
  }
  useEffect(() => {
    load();
    api.providerAccounts().then(setAccounts).catch(() => setAccounts([]));
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
            <DomainRow
              key={d.id}
              domain={d}
              accounts={accounts}
              onEdit={() => setEditing(d)}
              onRecords={() => setRecordsFor(d)}
              onReload={load}
              onDelete={async () => {
                if (confirm(`Remove ${d.name}?`)) {
                  await api.deleteDomain(d.id);
                  load();
                }
              }}
            />
          ))}
        </div>
      )}

      {editing && (
        <DomainEditor
          domain={editing === "new" ? null : editing}
          accounts={accounts}
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
          accounts={accounts}
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

// ─── DomainRow ────────────────────────────────────────────────────────────────
function DomainRow({
  domain,
  accounts,
  onEdit,
  onRecords,
  onReload,
  onDelete,
}: {
  domain: Domain;
  accounts: ProviderAccount[];
  onEdit: () => void;
  onRecords: () => void;
  onReload: () => void;
  onDelete: () => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const hosts = useMemo(() => mergeHosts(domain), [domain]);
  const providerName = accounts.find(a => a.id === domain.providerAccountId)?.name || `ID: ${domain.providerAccountId}`;

  // Toggle domain-level master switch (forLink / forMail).
  async function toggleService(field: "forLink" | "forMail") {
    if (busy) return;
    setBusy(field);
    const current = field === "forLink" ? domain.forLink : domain.forMail;
    try {
      await api.updateDomain(domain.id, { [field]: !current });
      await onReload();
    } finally {
      setBusy(null);
    }
  }

  // Toggle a single host's enabled flag for a specific service.
  async function toggleHost(hostName: string, service: "linkHosts" | "mailHosts", currentEnabled: boolean) {
    const key = `${service}:${hostName}`;
    if (busy === key) return;
    setBusy(key);
    const list = (service === "linkHosts" ? domain.linkHosts : domain.mailHosts) ?? [];
    const updated = list.map((h) =>
      h.host === hostName ? { ...h, enabled: !currentEnabled } : h,
    );
    try {
      await api.updateDomain(domain.id, { [service]: updated });
      await onReload();
    } finally {
      setBusy(null);
    }
  }

  // Add a host to linkHosts, mailHosts, or both.
  async function addHost(hostName: string, forLink: boolean, forMail: boolean) {
    if (!hostName || (!forLink && !forMail)) return;
    setBusy("add");
    const linkHosts = domain.linkHosts ?? [];
    const mailHosts = domain.mailHosts ?? [];
    const payload: Record<string, unknown> = {};
    if (forLink && !linkHosts.some((h) => h.host === hostName)) {
      payload.linkHosts = [...linkHosts, { host: hostName, enabled: true }];
    }
    if (forMail && !mailHosts.some((h) => h.host === hostName)) {
      payload.mailHosts = [...mailHosts, { host: hostName, enabled: true }];
    }
    try {
      await api.updateDomain(domain.id, payload);
      await onReload();
    } finally {
      setBusy(null);
    }
  }

  // Remove a host from both lists.
  async function removeHost(hostName: string) {
    setBusy(`remove:${hostName}`);
    try {
      await api.updateDomain(domain.id, {
        linkHosts: (domain.linkHosts ?? []).filter((h) => h.host !== hostName),
        mailHosts: (domain.mailHosts ?? []).filter((h) => h.host !== hostName),
      });
      await onReload();
    } finally {
      setBusy(null);
    }
  }

  return (
    <div>
      {/* ── main row ── */}
      <div className="flex items-center gap-3 px-4 py-3">
        {/* expand toggle */}
        <button
          className="text-zinc-500 hover:text-zinc-200 w-4 text-xs transition-colors"
          onClick={() => setExpanded((v) => !v)}
          title={expanded ? "Collapse hosts" : "Expand hosts"}
        >
          {expanded ? "▾" : "▸"}
        </button>

        {/* domain name */}
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2 flex-wrap">
            <span className="font-medium">{domain.name}</span>
            <span className="badge">{providerName}</span>
            {domain.note && (
              <span className="text-xs text-amber-300/70 truncate max-w-xs" title={domain.note}>
                📝 {domain.note}
              </span>
            )}
          </div>
          {!expanded && hosts.length > 0 && (
            <div className="mt-0.5 text-xs text-zinc-500">
              {hosts.length} host{hosts.length > 1 ? "s" : ""}
              {hosts.some((h) => h.linkEnabled === false || h.mailEnabled === false) && (
                <span className="ml-1 text-amber-400/70">(some disabled)</span>
              )}
            </div>
          )}
        </div>

        {/* master switches */}
        <div className="flex items-center gap-3 shrink-0">
          <label className="flex items-center gap-1.5 cursor-pointer select-none">
            <Toggle
              on={domain.forLink}
              onChange={() => toggleService("forLink")}
            />
            <span className={`text-xs ${domain.forLink ? "text-indigo-300" : "text-zinc-600"}`}>
              🔗 Link
            </span>
          </label>
          <label className="flex items-center gap-1.5 cursor-pointer select-none">
            <Toggle
              on={domain.forMail}
              onChange={() => toggleService("forMail")}
            />
            <span className={`text-xs ${domain.forMail ? "text-emerald-300" : "text-zinc-600"}`}>
              ✉️ Mail
            </span>
          </label>
        </div>

        {/* actions */}
        <button className="btn-ghost text-xs" onClick={onRecords}>DNS</button>
        <button className="btn-ghost text-xs" onClick={onEdit}>Edit</button>
        <button className="btn-danger text-xs" onClick={onDelete}>Del</button>
      </div>

      {/* ── subtable ── */}
      {expanded && (
        <div className="border-t border-zinc-800/60 bg-zinc-950/30 px-4 pb-3 pt-2 animate-expand">
          {hosts.length === 0 ? (
            <p className="text-xs text-zinc-600 py-2">No hosts — add one below.</p>
          ) : (
            <table className="w-full text-sm mb-3">
              <thead>
                <tr className="text-left text-xs text-zinc-500 border-b border-zinc-800">
                  <th className="py-1.5 pr-4 font-normal">Host</th>
                  <th className="py-1.5 pr-4 font-normal text-indigo-400">🔗 Link</th>
                  <th className="py-1.5 pr-4 font-normal text-emerald-400">✉️ Mail</th>
                  <th className="py-1.5 font-normal" />
                </tr>
              </thead>
              <tbody className="divide-y divide-zinc-800/40">
                {hosts.map((h) => (
                  <tr key={h.host} className="group">
                    <td className="py-1.5 pr-4 font-mono text-xs text-zinc-300">{h.host}</td>

                    {/* Link toggle */}
                    <td className="py-1.5 pr-4">
                      {h.linkEnabled !== null ? (
                        <button
                          disabled={!!busy}
                          onClick={() => toggleHost(h.host, "linkHosts", h.linkEnabled!)}
                          title={h.linkEnabled ? "Disable link on this host" : "Enable link on this host"}
                          className={`inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-xs transition-colors cursor-pointer disabled:opacity-50 ${
                            h.linkEnabled
                              ? "bg-indigo-500/20 text-indigo-300 hover:bg-indigo-500/30"
                              : "bg-zinc-800 text-zinc-500 hover:bg-zinc-700 line-through"
                          }`}
                        >
                          <span className={`h-1.5 w-1.5 rounded-full ${h.linkEnabled ? "bg-indigo-400" : "bg-zinc-600"}`} />
                          {h.linkEnabled ? "on" : "off"}
                        </button>
                      ) : (
                        <button
                          disabled={!!busy}
                          onClick={() => addHost(h.host, true, false)}
                          title="Add this host to link hosts"
                          className="text-xs text-zinc-600 hover:text-indigo-400 transition-colors px-1.5 py-0.5 cursor-pointer disabled:opacity-50"
                        >
                          + add
                        </button>
                      )}
                    </td>

                    {/* Mail toggle */}
                    <td className="py-1.5 pr-4">
                      {h.mailEnabled !== null ? (
                        <button
                          disabled={!!busy}
                          onClick={() => toggleHost(h.host, "mailHosts", h.mailEnabled!)}
                          title={h.mailEnabled ? "Disable mail on this host" : "Enable mail on this host"}
                          className={`inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-xs transition-colors cursor-pointer disabled:opacity-50 ${
                            h.mailEnabled
                              ? "bg-emerald-500/20 text-emerald-300 hover:bg-emerald-500/30"
                              : "bg-zinc-800 text-zinc-500 hover:bg-zinc-700 line-through"
                          }`}
                        >
                          <span className={`h-1.5 w-1.5 rounded-full ${h.mailEnabled ? "bg-emerald-400" : "bg-zinc-600"}`} />
                          {h.mailEnabled ? "on" : "off"}
                        </button>
                      ) : (
                        <button
                          disabled={!!busy}
                          onClick={() => addHost(h.host, false, true)}
                          title="Add this host to mail hosts"
                          className="text-xs text-zinc-600 hover:text-emerald-400 transition-colors px-1.5 py-0.5 cursor-pointer disabled:opacity-50"
                        >
                          + add
                        </button>
                      )}
                    </td>

                    {/* remove */}
                    <td className="py-1.5 text-right">
                      <button
                        disabled={!!busy}
                        onClick={() => removeHost(h.host)}
                        title="Remove host"
                        className="text-xs text-zinc-600 hover:text-red-400 opacity-0 group-hover:opacity-100 transition-all cursor-pointer disabled:opacity-30"
                      >
                        ✕
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}

          {/* add host row */}
          <AddHostRow
            domain={domain}
            busy={busy === "add"}
            onAdd={addHost}
          />
        </div>
      )}
    </div>
  );
}

// ─── AddHostRow ───────────────────────────────────────────────────────────────
function AddHostRow({
  domain,
  busy,
  onAdd,
}: {
  domain: Domain;
  busy: boolean;
  onAdd: (host: string, forLink: boolean, forMail: boolean) => void;
}) {
  const [draft, setDraft] = useState("");
  const [forLink, setForLink] = useState(true);
  const [forMail, setForMail] = useState(false);

  // Quick-add suggestions: common subdomains not yet in any list.
  const existing = useMemo(() => {
    const s = new Set<string>();
    for (const h of domain.linkHosts ?? []) s.add(h.host);
    for (const h of domain.mailHosts ?? []) s.add(h.host);
    return s;
  }, [domain]);

  const suggestions = useMemo(() => {
    const candidates = [
      `go.${domain.name}`,
      `s.${domain.name}`,
      `link.${domain.name}`,
      domain.name,
      `mail.${domain.name}`,
    ];
    return candidates.filter((c) => !existing.has(c));
  }, [domain, existing]);

  function submit() {
    let v = draft.trim().toLowerCase();
    // Auto-append base domain when user types a bare label (no dot)
    if (v && !v.includes(".")) v = `${v}.${domain.name}`;
    if (v && (forLink || forMail)) {
      onAdd(v, forLink, forMail);
      setDraft("");
    }
  }

  return (
    <div className="flex items-center gap-2 flex-wrap">
      <div className="relative flex-1 min-w-32">
        <input
          className="input h-7 text-xs py-0 w-full"
          placeholder="add a host…"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && submit()}
        />
        {draft && !draft.includes(".") && (
          <span className="pointer-events-none absolute inset-y-0 right-2 flex items-center text-[10px] text-zinc-500">
            → {draft.trim().toLowerCase()}.{domain.name}
          </span>
        )}
      </div>
      <label className="flex items-center gap-1 text-xs text-zinc-400 cursor-pointer select-none">
        <input
          type="checkbox"
          checked={forLink}
          onChange={(e) => setForLink(e.target.checked)}
          className="accent-indigo-500"
        />
        🔗 Link
      </label>
      <label className="flex items-center gap-1 text-xs text-zinc-400 cursor-pointer select-none">
        <input
          type="checkbox"
          checked={forMail}
          onChange={(e) => setForMail(e.target.checked)}
          className="accent-emerald-500"
        />
        ✉️ Mail
      </label>
      <button
        className="btn-primary h-7 py-0 text-xs"
        disabled={busy || !draft.trim() || (!forLink && !forMail)}
        onClick={submit}
      >
        + Add
      </button>
      {suggestions.length > 0 && (
        <div className="flex flex-wrap gap-1 w-full mt-0.5">
          {suggestions.slice(0, 4).map((s) => (
            <button
              key={s}
              type="button"
              disabled={busy}
              onClick={() => { setDraft(s); }}
              className="text-xs text-zinc-500 hover:text-zinc-300 border border-zinc-800 hover:border-zinc-600 rounded px-1.5 py-0.5 transition-colors cursor-pointer"
            >
              {s}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}


// ─── SyncModal ────────────────────────────────────────────────────────────────
function SyncModal({ accounts, onClose, onSynced }: { accounts: ProviderAccount[]; onClose: () => void; onSynced: () => void }) {
  const [accountId, setAccountId] = useState<number>(accounts[0]?.id || 0);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [result, setResult] = useState<{ total: number; created: number; updated: number } | null>(null);

  async function run() {
    if (!accountId) return setErr("Please select a provider account");
    setBusy(true); setErr("");
    try { const r = await api.syncDomains(accountId); setResult(r); }
    catch (e: any) { setErr(e.message ?? "sync failed"); }
    finally { setBusy(false); }
  }

  return (
    <Modal title="Sync Domains" onClose={onClose}>
      {result ? (
        <div className="py-4 text-center">
          <p className="mb-2 text-2xl">✅</p>
          <p className="text-zinc-300">{result.total} zones — <span className="text-green-400">{result.created} new</span>, <span className="text-indigo-300">{result.updated} updated</span>.</p>
          <p className="mt-2 text-xs text-zinc-500">Use the 🔗 Link / ✉️ Mail toggles on each domain row to enable services.</p>
          <button className="btn-primary mt-4" onClick={onSynced}>Done</button>
        </div>
      ) : accounts.length === 0 ? (
        <div className="py-4 text-center text-zinc-400">
          <p>You need to create a Provider Account first.</p>
          <p className="mt-2 text-xs">Go to Settings to configure your DNS provider.</p>
        </div>
      ) : (
        <>
          <p className="mb-3 text-sm text-zinc-400">Sync all domains from a provider account.</p>
          <Field label="Provider Account">
            <select className="input" value={accountId} onChange={e => setAccountId(Number(e.target.value))}>
              <option value={0}>Select account...</option>
              {accounts.map(a => <option key={a.id} value={a.id}>{a.name} ({a.type})</option>)}
            </select>
          </Field>
          {err && <p className="mb-3 text-sm text-red-400">{err}</p>}
          <div className="flex justify-end gap-2">
            <button className="btn-ghost" onClick={onClose}>Cancel</button>
            <button className="btn-primary" onClick={run} disabled={busy || !accountId}>{busy ? "Syncing…" : "Sync"}</button>
          </div>
        </>
      )}
    </Modal>
  );
}

// ─── DomainEditor ─────────────────────────────────────────────────────────────
function DomainEditor({ domain, accounts, onClose, onSaved }: { domain: Domain | null; accounts: ProviderAccount[]; onClose: () => void; onSaved: () => void; }) {
  const [name, setName] = useState(domain?.name ?? "");
  const [providerAccountId, setProviderAccountId] = useState(domain?.providerAccountId || accounts[0]?.id || 0);
  const [zoneId, setZoneId] = useState(domain?.zoneId ?? "");
  const [note, setNote] = useState(domain?.note ?? "");
  const [linkHosts, setLinkHosts] = useState<HostEntry[]>(domain?.linkHosts ?? []);
  const [mailHosts, setMailHosts] = useState<HostEntry[]>(domain?.mailHosts ?? []);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  const linkSubs = name ? [`go.${name}`, `s.${name}`, `link.${name}`, name] : [];
  const mailSubs = name ? [name, `mail.${name}`] : [];

  async function save() {
    setErr(""); setBusy(true);
    const payload: any = { name, providerAccountId, zoneId, note, linkHosts, mailHosts };
    try {
      if (domain) await api.updateDomain(domain.id, payload);
      else await api.createDomain(payload);
      onSaved();
    } catch (e: any) { setErr(e.message ?? "save failed"); }
    finally { setBusy(false); }
  }

  return (
    <Modal title={domain ? "Edit domain" : "Add domain"} onClose={onClose}>
      <Field label="Domain name">
        <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="example.com" disabled={!!domain} />
      </Field>
      <div className="grid grid-cols-2 gap-3">
        <Field label="Provider Account">
          <select className="input" value={providerAccountId} onChange={(e) => setProviderAccountId(Number(e.target.value))}>
            {accounts.map((a) => <option key={a.id} value={a.id}>{a.name} ({a.type})</option>)}
            {accounts.length === 0 && <option value={0}>No accounts available</option>}
          </select>
        </Field>
        <Field label="Zone ID">
          <input className="input" value={zoneId} onChange={(e) => setZoneId(e.target.value)} />
        </Field>
      </div>
      <Field label="Note">
        <textarea className="input" rows={2} value={note} onChange={(e) => setNote(e.target.value)} />
      </Field>
      <Field label="Short-link hosts" hint="Hosts serving short links. Also manageable from the domain list subtable.">
        <HostList hosts={linkHosts} onChange={setLinkHosts} suggestions={linkSubs} placeholder="go.example.com" baseDomain={name || undefined} emptyText="No short-link hosts added yet." />
      </Field>
      <Field label="Mail hosts" hint="Hosts accepting mailboxes. Also manageable from the domain list subtable.">
        <HostList hosts={mailHosts} onChange={setMailHosts} suggestions={mailSubs} placeholder="mail.example.com" baseDomain={name || undefined} emptyText="No mail hosts added yet." />
      </Field>
      {err && <p className="mb-3 text-sm text-red-400">{err}</p>}
      <div className="flex justify-end gap-2">
        <button className="btn-ghost" onClick={onClose}>Cancel</button>
        <button className="btn-primary" onClick={save} disabled={busy}>{busy ? "…" : "Save"}</button>
      </div>
    </Modal>
  );
}

// ─── DNS Records ──────────────────────────────────────────────────────────────
const RECORD_TYPES = ["A", "AAAA", "CNAME", "TXT", "MX", "NS", "CAA"];

function RecordsModal({ domain, onClose }: { domain: Domain; onClose: () => void }) {
  const [records, setRecords] = useState<DNSRecord[] | null>(null);
  const [err, setErr] = useState("");
  const [editing, setEditing] = useState<DNSRecord | "new" | "subdomain" | null>(null);
  const [typeFilter, setTypeFilter] = useState("");
  const [search, setSearch] = useState("");

  async function load() {
    setErr("");
    try { setRecords(await api.records(domain.id)); }
    catch (e: any) { setErr(e.message ?? "failed to load records"); setRecords([]); }
  }
  useEffect(() => { load(); }, [domain.id]);

  const filtered = (records ?? []).filter((r) => {
    if (typeFilter && r.type !== typeFilter) return false;
    if (search) {
      const s = search.toLowerCase();
      return r.name.toLowerCase().includes(s) || r.content.toLowerCase().includes(s) || (r.comment ?? "").toLowerCase().includes(s);
    }
    return true;
  });
  const presentTypes = Array.from(new Set((records ?? []).map((r) => r.type)));

  return (
    <Modal title={`DNS · ${domain.name}`} onClose={onClose} wide>
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <select className="input max-w-[110px]" value={typeFilter} onChange={(e) => setTypeFilter(e.target.value)}>
          <option value="">All types</option>
          {presentTypes.map((t) => <option key={t} value={t}>{t}</option>)}
        </select>
        <input className="input flex-1" placeholder="Filter name / content / note…" value={search} onChange={(e) => setSearch(e.target.value)} />
        <button className="btn-ghost shrink-0" onClick={() => setEditing("subdomain")}>+ Subdomain</button>
        <button className="btn-primary shrink-0" onClick={() => setEditing("new")}>+ Record</button>
      </div>
      <p className="mb-2 text-xs text-zinc-500">Notes map to the provider's native record comment · {filtered.length}/{records?.length ?? 0} shown</p>
      {err && <p className="mb-3 rounded bg-red-500/10 p-2 text-sm text-red-400">{err}</p>}
      {records === null ? (<p className="text-zinc-500">loading…</p>) : filtered.length === 0 ? (<p className="text-zinc-500">No matching records.</p>) : (
        <div className="max-h-96 overflow-y-auto">
          <table className="w-full text-sm">
            <thead className="text-left text-xs uppercase text-zinc-500">
              <tr><th className="py-1">Type</th><th>Name</th><th>Content</th><th>Note</th><th></th></tr>
            </thead>
            <tbody>
              {filtered.map((r) => (
                <tr key={r.id} className="border-t border-zinc-800">
                  <td className="py-1.5"><span className="badge">{r.type}</span>{r.proxied && <span className="ml-1 text-orange-400" title="proxied">☁</span>}</td>
                  <td className="max-w-[120px] truncate">{r.name}</td>
                  <td className="max-w-[160px] truncate text-zinc-400">{r.content}</td>
                  <td className="max-w-[120px] truncate text-amber-300/80">{r.comment}</td>
                  <td className="text-right">
                    <button className="btn-ghost px-2" onClick={() => setEditing(r)}>Edit</button>
                    <button className="btn-danger px-2" onClick={async () => { if (confirm(`Delete ${r.type} ${r.name}?`)) { await api.deleteRecord(domain.id, r.id); load(); } }}>Del</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {editing && (
        <RecordEditor
          domainId={domain.id} domainName={domain.name}
          linkHost={domain.linkHosts?.[0]?.host ?? ""}
          record={typeof editing === "string" ? null : editing}
          subdomain={editing === "subdomain"}
          onClose={() => setEditing(null)}
          onSaved={() => { setEditing(null); load(); }}
        />
      )}
    </Modal>
  );
}

function RecordEditor({ domainId, domainName, linkHost, record, subdomain, onClose, onSaved }: { domainId: number; domainName: string; linkHost?: string; record: DNSRecord | null; subdomain?: boolean; onClose: () => void; onSaved: () => void; }) {
  const [type, setType] = useState(record?.type ?? "A");
  const [name, setName] = useState(record?.name ?? "");
  const [content, setContent] = useState(record?.content ?? "");
  const [comment, setComment] = useState(record?.comment ?? "");
  const [proxied, setProxied] = useState(record?.proxied ?? false);
  const [priority, setPriority] = useState<number>(record?.priority ?? 10);
  const [err, setErr] = useState("");

  const needsPriority = ["MX", "SRV", "URI"].includes(type.toUpperCase());
  const canProxy = ["A", "AAAA", "CNAME"].includes(type.toUpperCase());
  const contentHint: Record<string, string> = { A: "IPv4 address, e.g. 203.0.113.10", AAAA: "IPv6 address", CNAME: "target hostname", TXT: "text value", MX: "mail server hostname", NS: "nameserver hostname", CAA: '0 issue "letsencrypt.org"' };

  const linkSub = linkHost && linkHost.endsWith("." + domainName) ? linkHost.slice(0, -(domainName.length + 1)) : linkHost === domainName ? "@" : "go";

  function preset(kind: "link" | "mail") {
    if (kind === "link") { setType("CNAME"); setName(name || linkSub); setContent(domainName); setComment("led short-link host"); setProxied(true); }
    else { setType("MX"); setName(name || "mail"); setContent("route1.mx.cloudflare.net"); setComment("led mailbox (Cloudflare Email Routing)"); setProxied(false); setPriority(10); }
  }

  async function save() {
    setErr("");
    const payload: Partial<DNSRecord> = { type, name, content, comment, proxied: canProxy ? proxied : false, ttl: 1 };
    if (needsPriority) payload.priority = Number(priority);
    try {
      if (record) await api.updateRecord(domainId, record.id, payload);
      else await api.createRecord(domainId, payload);
      onSaved();
    } catch (e: any) { setErr(e.message ?? "save failed"); }
  }

  return (
    <Modal title={record ? "Edit record" : subdomain ? "New subdomain" : "New record"} onClose={onClose}>
      {subdomain && (<div className="mb-3 flex gap-2"><button className="btn-ghost flex-1" onClick={() => preset("link")}>🔗 Short-link subdomain</button><button className="btn-ghost flex-1" onClick={() => preset("mail")}>✉️ Email subdomain</button></div>)}
      <div className="grid grid-cols-2 gap-3">
        <Field label="Type"><select className="input" value={type} onChange={(e) => setType(e.target.value)}>{RECORD_TYPES.map((t) => <option key={t}>{t}</option>)}</select></Field>
        <Field label="Name"><input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="@ or sub" /></Field>
      </div>
      <div className={needsPriority ? "grid grid-cols-[1fr_100px] gap-3" : ""}>
        <Field label="Content" hint={contentHint[type.toUpperCase()]}><input className="input" value={content} onChange={(e) => setContent(e.target.value)} /></Field>
        {needsPriority && (<Field label="Priority"><input type="number" min={0} className="input" value={priority} onChange={(e) => setPriority(Number(e.target.value))} /></Field>)}
      </div>
      <Field label="Note (comment)"><input className="input" value={comment} onChange={(e) => setComment(e.target.value)} /></Field>
      {canProxy && (<label className="mb-4 flex items-center gap-2 text-sm text-zinc-400"><Toggle on={proxied} onChange={setProxied} /> Proxied (Cloudflare)</label>)}
      {err && <p className="mb-3 text-sm text-red-400">{err}</p>}
      <div className="flex justify-end gap-2">
        <button className="btn-ghost" onClick={onClose}>Cancel</button>
        <button className="btn-primary" onClick={save}>Save</button>
      </div>
    </Modal>
  );
}
