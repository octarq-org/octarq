import { useEffect, useState } from "react";
import { api, DNSRecord, Domain } from "../api";
import { Empty, Field, Modal, Toggle } from "../ui";
import { Header } from "./Links";

export default function DomainsPage() {
  const [domains, setDomains] = useState<Domain[]>([]);
  const [providers, setProviders] = useState<string[]>([]);
  const [editing, setEditing] = useState<Domain | "new" | null>(null);
  const [recordsFor, setRecordsFor] = useState<Domain | null>(null);

  async function load() {
    setDomains(await api.domains());
  }
  useEffect(() => {
    load();
    api.dnsProviders().then(setProviders).catch(() => setProviders(["cloudflare"]));
  }, []);

  return (
    <div>
      <Header title="Domains" subtitle="Manage DNS records across providers">
        <button className="btn-primary" onClick={() => setEditing("new")}>
          + Add domain
        </button>
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
                  {d.forLink && <span className="badge">links</span>}
                  {d.forMail && <span className="badge">mail</span>}
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
    </div>
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
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  async function save() {
    setErr("");
    setBusy(true);
    const payload: any = { name, provider, zoneId, note, forLink, forMail };
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
      <div className="mb-4 flex gap-6">
        <label className="flex items-center gap-2 text-sm text-zinc-400">
          <Toggle on={forLink} onChange={setForLink} /> Serve short links
        </label>
        <label className="flex items-center gap-2 text-sm text-zinc-400">
          <Toggle on={forMail} onChange={setForMail} /> Accept email
        </label>
      </div>
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
  const [editing, setEditing] = useState<DNSRecord | "new" | null>(null);

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

  return (
    <Modal title={`DNS · ${domain.name}`} onClose={onClose} wide>
      <div className="mb-3 flex justify-between">
        <p className="text-sm text-zinc-500">Notes map to the provider's native record comment.</p>
        <button className="btn-primary" onClick={() => setEditing("new")}>
          + Record
        </button>
      </div>
      {err && <p className="mb-3 rounded bg-red-500/10 p-2 text-sm text-red-400">{err}</p>}
      {records === null ? (
        <p className="text-zinc-500">loading…</p>
      ) : records.length === 0 ? (
        <p className="text-zinc-500">No records.</p>
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
              {records.map((r) => (
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
          record={editing === "new" ? null : editing}
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
  record,
  onClose,
  onSaved,
}: {
  domainId: number;
  record: DNSRecord | null;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [type, setType] = useState(record?.type ?? "A");
  const [name, setName] = useState(record?.name ?? "");
  const [content, setContent] = useState(record?.content ?? "");
  const [comment, setComment] = useState(record?.comment ?? "");
  const [proxied, setProxied] = useState(record?.proxied ?? false);
  const [err, setErr] = useState("");

  async function save() {
    setErr("");
    const payload: Partial<DNSRecord> = { type, name, content, comment, proxied, ttl: 1 };
    try {
      if (record) await api.updateRecord(domainId, record.id, payload);
      else await api.createRecord(domainId, payload);
      onSaved();
    } catch (e: any) {
      setErr(e.message ?? "save failed");
    }
  }

  return (
    <Modal title={record ? "Edit record" : "New record"} onClose={onClose}>
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
      <Field label="Content">
        <input className="input" value={content} onChange={(e) => setContent(e.target.value)} />
      </Field>
      <Field label="Note (comment)">
        <input className="input" value={comment} onChange={(e) => setComment(e.target.value)} />
      </Field>
      <label className="mb-4 flex items-center gap-2 text-sm text-zinc-400">
        <Toggle on={proxied} onChange={setProxied} /> Proxied (Cloudflare)
      </label>
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
