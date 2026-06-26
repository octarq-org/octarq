import { useEffect, useMemo, useState } from "react";
import { api, DNSRecord, Domain, HostEntry, ProviderAccount } from "../api";
import { Empty, Field, HostList, Modal, Toggle, timeAgo } from "../ui";
import { Header } from "./Links";

interface HostRow {
  host: string;
  linkEnabled: boolean | null;
  mailEnabled: boolean | null;
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

export default function DomainsPage() {
  const [domains, setDomains] = useState<Domain[]>([]);
  const [accounts, setAccounts] = useState<ProviderAccount[]>([]);
  const [active, setActive] = useState<Domain | "new" | null>(null);
  const [syncing, setSyncing] = useState(false);
  const [q, setQ] = useState("");

  const [page, setPage] = useState(0);
  const [hasMore, setHasMore] = useState(true);
  const [loading, setLoading] = useState(false);

  async function loadMore(reset = false) {
    if (loading || (!hasMore && !reset)) return;
    setLoading(true);
    try {
      const limit = 50;
      const offset = reset ? 0 : page * limit;
      const res = await api.domains({ q, limit, offset });
      if (res.length < limit) setHasMore(false);
      else setHasMore(true);

      setDomains(prev => reset ? res : [...prev, ...res]);
      setPage(reset ? 1 : page + 1);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    const t = setTimeout(() => {
      loadMore(true);
    }, 200);
    return () => clearTimeout(t);
  }, [q]);

  useEffect(() => {
    api.providerAccounts().then(setAccounts).catch(() => setAccounts([]));
  }, []);

  const handleScroll = (e: React.UIEvent<HTMLDivElement>) => {
    const bottom = e.currentTarget.scrollHeight - e.currentTarget.scrollTop <= e.currentTarget.clientHeight + 100;
    if (bottom) loadMore();
  };

  async function toggleService(domain: Domain, field: "forLink" | "forMail") {
    const current = field === "forLink" ? domain.forLink : domain.forMail;
    await api.updateDomain(domain.id, { [field]: !current });
    loadMore(true);
    if (active && active !== "new" && active.id === domain.id) {
      setActive({ ...active, [field]: !current });
    }
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <Header title="Domains" subtitle="Sync & manage DNS records across providers">
        <div className="flex gap-2">
          <button className="btn-ghost" onClick={() => setSyncing(true)}>
            ⟳ Sync from Cloudflare
          </button>
          <button className="btn-primary" onClick={() => setActive("new")}>
            + Add domain
          </button>
        </div>
      </Header>

      <div className="grid grid-cols-[300px_1fr] gap-4 min-h-0 flex-1">
        {/* left column */}
        <div className="flex flex-col min-h-0">
          <div className="mb-2">
            <input
              className="input w-full"
              placeholder="Search domains…"
              value={q}
              onChange={(e) => setQ(e.target.value)}
            />
          </div>
          <div className="card flex-1 overflow-y-auto" onScroll={handleScroll}>
            {domains.length === 0 && !loading ? (
              <div className="p-8 text-center text-zinc-500">No domains found.</div>
            ) : (
              <div className="divide-y divide-zinc-800">
                {domains.map((d) => (
                  <div
                    key={d.id}
                    className={`flex w-full flex-col p-3 text-left hover:bg-zinc-900 transition-colors cursor-pointer ${
                      active !== "new" && active?.id === d.id ? "bg-zinc-800" : ""
                    }`}
                    onClick={() => setActive(d)}
                  >
                    <div className="flex items-center justify-between w-full">
                      <span className="font-medium truncate flex-1">{d.name}</span>
                      <div className="flex gap-1.5 shrink-0">
                        <button
                          className="p-1 hover:bg-zinc-700 rounded transition-colors cursor-pointer"
                          title="Toggle Link routing"
                          onClick={(e) => { e.stopPropagation(); toggleService(d, "forLink"); }}
                        >
                          <span className={`text-xs ${d.forLink ? "text-indigo-300" : "text-zinc-600 grayscale opacity-50"}`}>🔗</span>
                        </button>
                        <button
                          className="p-1 hover:bg-zinc-700 rounded transition-colors cursor-pointer"
                          title="Toggle Mail routing"
                          onClick={(e) => { e.stopPropagation(); toggleService(d, "forMail"); }}
                        >
                          <span className={`text-xs ${d.forMail ? "text-emerald-300" : "text-zinc-600 grayscale opacity-50"}`}>✉️</span>
                        </button>
                      </div>
                    </div>
                    {d.note && <div className="truncate text-xs text-amber-300/70 mt-1">📝 {d.note}</div>}
                  </div>
                ))}
                {loading && <div className="p-3 text-center text-xs text-zinc-500">Loading…</div>}
              </div>
            )}
          </div>
        </div>

        {/* right column */}
        <div className="min-h-0 overflow-y-auto pr-2 pb-8">
          {active === "new" ? (
             <div className="card p-5">
               <h2 className="mb-4 text-xl font-semibold">Add Domain</h2>
               <DomainEditorForm
                 domain={null}
                 accounts={accounts}
                 onCancel={() => setActive(null)}
                 onSaved={(savedDomain) => {
                   loadMore(true);
                   setActive(savedDomain || null);
                 }}
               />
             </div>
          ) : active ? (
             <div className="space-y-4">
                <div className="card p-5">
                  <div className="flex justify-between mb-4 border-b border-zinc-800 pb-4">
                     <h2 className="text-xl font-semibold">{active.name}</h2>
                     <button
                       className="btn-danger text-sm px-2"
                       onClick={async () => {
                         if (confirm(`Remove ${active.name}?`)) {
                           await api.deleteDomain(active.id);
                           setActive(null);
                           loadMore(true);
                         }
                       }}
                     >
                       Delete Domain
                     </button>
                  </div>
                  
                  <DomainEditorForm
                    key={active.id}
                    domain={active}
                    accounts={accounts}
                    onCancel={() => setActive(null)}
                    onSaved={(d) => {
                      if (d) setActive(d);
                      loadMore(true);
                    }}
                  />
                </div>
                
                <div className="card p-5">
                  <h3 className="mb-4 text-lg font-semibold text-zinc-300">Managed Hosts</h3>
                  <DomainHostManager
                    domain={active}
                    onReload={async () => {
                      loadMore(true);
                      const res = await api.domains({ q: active.name, limit: 1, offset: 0 });
                      const updated = res.find(d => d.id === active.id);
                      if (updated) setActive(updated);
                    }}
                  />
                </div>

                <div className="card p-5">
                  <h3 className="mb-4 text-lg font-semibold text-zinc-300">DNS Records</h3>
                  <RecordsView domain={active} />
                </div>
             </div>
          ) : (
            <div className="flex h-full items-center justify-center text-zinc-500/50">
              Select a domain to view details
            </div>
          )}
        </div>
      </div>
      
      {syncing && (
        <SyncModal
          accounts={accounts}
          onClose={() => setSyncing(false)}
          onSynced={() => {
            setSyncing(false);
            loadMore(true);
          }}
        />
      )}
    </div>
  );
}

function DomainEditorForm({ domain, accounts, onClose, onCancel, onSaved }: { domain: Domain | null; accounts: ProviderAccount[]; onClose?: () => void; onCancel: () => void; onSaved: (d?: any) => void; }) {
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
      let res;
      if (domain) res = await api.updateDomain(domain.id, payload);
      else res = await api.createDomain(payload);
      onSaved(res);
    } catch (e: any) { setErr(e.message ?? "save failed"); }
    finally { setBusy(false); }
  }

  return (
    <div className="space-y-4">
      <Field label="Domain name">
        <input className="input w-full" value={name} onChange={(e) => setName(e.target.value)} placeholder="example.com" disabled={!!domain} />
      </Field>
      <div className="grid grid-cols-2 gap-3">
        <Field label="Provider Account">
          <select className="input w-full" value={providerAccountId} onChange={(e) => setProviderAccountId(Number(e.target.value))}>
            {accounts.map((a) => <option key={a.id} value={a.id}>{a.name} ({a.type})</option>)}
            {accounts.length === 0 && <option value={0}>No accounts available</option>}
          </select>
        </Field>
        <Field label="Zone ID">
          <input className="input w-full" value={zoneId} onChange={(e) => setZoneId(e.target.value)} />
        </Field>
      </div>
      <Field label="Note">
        <textarea className="input w-full" rows={2} value={note} onChange={(e) => setNote(e.target.value)} />
      </Field>
      {!domain && (
        <>
          <Field label="Short-link hosts">
            <HostList hosts={linkHosts} onChange={setLinkHosts} suggestions={linkSubs} placeholder="go.example.com" baseDomain={name || undefined} emptyText="No short-link hosts added yet." />
          </Field>
          <Field label="Mail hosts">
            <HostList hosts={mailHosts} onChange={setMailHosts} suggestions={mailSubs} placeholder="mail.example.com" baseDomain={name || undefined} emptyText="No mail hosts added yet." />
          </Field>
        </>
      )}
      {err && <p className="mb-3 text-sm text-red-400">{err}</p>}
      <div className="flex justify-end gap-2 pt-4 border-t border-zinc-800">
        {onCancel && (
          <button className="btn-ghost" onClick={onCancel}>Cancel</button>
        )}
        <button className="btn-primary" onClick={save} disabled={busy}>{busy ? "…" : "Save Basic Info"}</button>
      </div>
    </div>
  );
}

function DomainHostManager({ domain, onReload }: { domain: Domain; onReload: () => void }) {
  const [busy, setBusy] = useState<string | null>(null);
  const hosts = useMemo(() => mergeHosts(domain), [domain]);

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
    <div className="bg-zinc-950/30 rounded p-4 border border-zinc-800">
      {hosts.length === 0 ? (
        <p className="text-sm text-zinc-600 mb-4">No hosts — add one below.</p>
      ) : (
        <table className="w-full text-sm mb-5">
          <thead>
            <tr className="text-left text-xs text-zinc-500 border-b border-zinc-800">
              <th className="py-2 pr-4 font-normal">Host</th>
              <th className="py-2 pr-4 font-normal text-indigo-400">🔗 Link</th>
              <th className="py-2 pr-4 font-normal text-emerald-400">✉️ Mail</th>
              <th className="py-2 font-normal" />
            </tr>
          </thead>
          <tbody className="divide-y divide-zinc-800/40">
            {hosts.map((h) => (
              <tr key={h.host} className="group">
                <td className="py-2 pr-4 font-mono text-xs text-zinc-300">{h.host}</td>
                <td className="py-2 pr-4">
                  {h.linkEnabled !== null ? (
                    <button
                      disabled={!!busy}
                      onClick={() => toggleHost(h.host, "linkHosts", h.linkEnabled!)}
                      className={`inline-flex items-center gap-1 rounded px-2 py-0.5 text-xs transition-colors cursor-pointer disabled:opacity-50 ${
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
                      className="text-xs text-zinc-600 hover:text-indigo-400 transition-colors px-2 py-0.5 cursor-pointer disabled:opacity-50"
                    >
                      + add
                    </button>
                  )}
                </td>
                <td className="py-2 pr-4">
                  {h.mailEnabled !== null ? (
                    <button
                      disabled={!!busy}
                      onClick={() => toggleHost(h.host, "mailHosts", h.mailEnabled!)}
                      className={`inline-flex items-center gap-1 rounded px-2 py-0.5 text-xs transition-colors cursor-pointer disabled:opacity-50 ${
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
                      className="text-xs text-zinc-600 hover:text-emerald-400 transition-colors px-2 py-0.5 cursor-pointer disabled:opacity-50"
                    >
                      + add
                    </button>
                  )}
                </td>
                <td className="py-2 text-right">
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
      <AddHostRow domain={domain} busy={busy === "add"} onAdd={addHost} />
    </div>
  );
}

function AddHostRow({ domain, busy, onAdd }: { domain: Domain; busy: boolean; onAdd: (host: string, forLink: boolean, forMail: boolean) => void; }) {
  const [draft, setDraft] = useState("");
  const [forLink, setForLink] = useState(true);
  const [forMail, setForMail] = useState(false);

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
    if (v && !v.includes(".")) v = `${v}.${domain.name}`;
    if (v && (forLink || forMail)) {
      onAdd(v, forLink, forMail);
      setDraft("");
    }
  }

  return (
    <div className="flex items-center gap-2 flex-wrap bg-zinc-900/50 p-2 rounded">
      <div className="relative flex-1 min-w-32">
        <input
          className="input h-8 text-sm py-1 w-full"
          placeholder="add a host…"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && submit()}
        />
        {draft && !draft.includes(".") && (
          <span className="pointer-events-none absolute inset-y-0 right-3 flex items-center text-xs text-zinc-500">
            → {draft.trim().toLowerCase()}.{domain.name}
          </span>
        )}
      </div>
      <label className="flex items-center gap-1.5 text-sm text-zinc-400 cursor-pointer select-none">
        <input type="checkbox" checked={forLink} onChange={(e) => setForLink(e.target.checked)} className="accent-indigo-500" />
        🔗 Link
      </label>
      <label className="flex items-center gap-1.5 text-sm text-zinc-400 cursor-pointer select-none">
        <input type="checkbox" checked={forMail} onChange={(e) => setForMail(e.target.checked)} className="accent-emerald-500" />
        ✉️ Mail
      </label>
      <button className="btn-primary h-8 py-1 text-sm" disabled={busy || !draft.trim() || (!forLink && !forMail)} onClick={submit}>
        + Add
      </button>
      {suggestions.length > 0 && (
        <div className="flex flex-wrap gap-1.5 w-full mt-1.5 px-1">
          {suggestions.slice(0, 4).map((s) => (
            <button
              key={s}
              type="button"
              disabled={busy}
              onClick={() => { setDraft(s); }}
              className="text-xs text-zinc-500 hover:text-zinc-300 border border-zinc-800 hover:border-zinc-600 rounded px-2 py-0.5 transition-colors cursor-pointer"
            >
              {s}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

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
            <select className="input w-full" value={accountId} onChange={e => setAccountId(Number(e.target.value))}>
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

const RECORD_TYPES = ["A", "AAAA", "CNAME", "TXT", "MX", "NS", "CAA"];

function RecordsView({ domain }: { domain: Domain }) {
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
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <select className="input min-w-[120px]" value={typeFilter} onChange={(e) => setTypeFilter(e.target.value)}>
          <option value="">All types</option>
          {presentTypes.map((t) => <option key={t} value={t}>{t}</option>)}
        </select>
        <input className="input flex-1 min-w-[140px]" placeholder="Filter name / content / note…" value={search} onChange={(e) => setSearch(e.target.value)} />
        <button className="btn-ghost shrink-0" onClick={() => setEditing("subdomain")}>+ Sub</button>
        <button className="btn-primary shrink-0" onClick={() => setEditing("new")}>+ Record</button>
      </div>
      
      <p className="text-xs text-zinc-500">Notes map to the provider's native record comment · {filtered.length}/{records?.length ?? 0} shown</p>
      
      {err && <p className="rounded bg-red-500/10 p-3 text-sm text-red-400">{err}</p>}
      
      {records === null ? (<p className="text-zinc-500 p-4 text-center">loading…</p>) : filtered.length === 0 ? (<p className="text-zinc-500 p-4 text-center">No matching records.</p>) : (
        <div className="bg-zinc-950/30 rounded border border-zinc-800">
          <table className="w-full text-sm">
            <thead className="text-left text-xs uppercase text-zinc-500">
              <tr className="border-b border-zinc-800">
                <th className="py-2 pl-3">Type</th>
                <th className="py-2">Name</th>
                <th className="py-2">Content</th>
                <th className="py-2">Note</th>
                <th className="py-2 pr-3"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-800/40">
              {filtered.map((r) => (
                <tr key={r.id} className="hover:bg-zinc-900/50 transition-colors">
                  <td className="py-2 pl-3"><span className="badge">{r.type}</span>{r.proxied && <span className="ml-1 text-orange-400" title="proxied">☁</span>}</td>
                  <td className="max-w-[120px] truncate">{r.name}</td>
                  <td className="max-w-[160px] truncate text-zinc-400">{r.content}</td>
                  <td className="max-w-[120px] truncate text-amber-300/80">{r.comment}</td>
                  <td className="text-right pr-3">
                    <button className="btn-ghost px-2 text-xs" onClick={() => setEditing(r)}>Edit</button>
                    <button className="btn-danger px-2 text-xs" onClick={async () => { if (confirm(`Delete ${r.type} ${r.name}?`)) { await api.deleteRecord(domain.id, r.id); load(); } }}>Del</button>
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
    </div>
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
  const contentHint: Record<string, string> = { A: "IPv4 address", AAAA: "IPv6 address", CNAME: "target hostname", TXT: "text value", MX: "mail server hostname", NS: "nameserver hostname", CAA: '0 issue "letsencrypt.org"' };

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
        <Field label="Type"><select className="input w-full" value={type} onChange={(e) => setType(e.target.value)}>{RECORD_TYPES.map((t) => <option key={t}>{t}</option>)}</select></Field>
        <Field label="Name"><input className="input w-full" value={name} onChange={(e) => setName(e.target.value)} placeholder="@ or sub" /></Field>
      </div>
      <div className={needsPriority ? "grid grid-cols-[1fr_100px] gap-3" : ""}>
        <Field label="Content" hint={contentHint[type.toUpperCase()]}><input className="input w-full" value={content} onChange={(e) => setContent(e.target.value)} /></Field>
        {needsPriority && (<Field label="Priority"><input type="number" min={0} className="input w-full" value={priority} onChange={(e) => setPriority(Number(e.target.value))} /></Field>)}
      </div>
      <Field label="Note (comment)"><input className="input w-full" value={comment} onChange={(e) => setComment(e.target.value)} /></Field>
      {canProxy && (<label className="mb-4 flex items-center gap-2 text-sm text-zinc-400"><Toggle on={proxied} onChange={setProxied} /> Proxied (Cloudflare)</label>)}
      {err && <p className="mb-3 text-sm text-red-400">{err}</p>}
      <div className="flex justify-end gap-2">
        <button className="btn-ghost" onClick={onClose}>Cancel</button>
        <button className="btn-primary" onClick={save}>Save</button>
      </div>
    </Modal>
  );
}
