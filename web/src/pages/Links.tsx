import { useEffect, useState } from "react";
import { api, Domain, effectiveLinkHosts, Link, LinkStats } from "../api";
import { Empty, Field, Modal, Toggle, timeAgo } from "../ui";

export default function LinksPage() {
  const [links, setLinks] = useState<Link[]>([]);
  const [domains, setDomains] = useState<Domain[]>([]);
  const [q, setQ] = useState("");
  const [archived, setArchived] = useState(false);
  const [editing, setEditing] = useState<Link | "new" | null>(null);
  const [statsFor, setStatsFor] = useState<Link | null>(null);
  const [qrFor, setQrFor] = useState<Link | null>(null);
  const [copied, setCopied] = useState<number | null>(null);

  // Every short-link host across all link-enabled domains (incl. subdomains).
  const linkHostOptions = Array.from(new Set(domains.flatMap(effectiveLinkHosts)));

  async function load() {
    setLinks(await api.links({ q, archived }));
  }
  useEffect(() => {
    load();
  }, [q, archived]);
  useEffect(() => {
    api.domains().then(setDomains).catch(() => {});
  }, []);

  function linkURL(l: Link) {
    return l.host ? `https://${l.host}/${l.slug}` : `${location.origin}/${l.slug}`;
  }
  async function copy(l: Link) {
    await navigator.clipboard.writeText(linkURL(l));
    setCopied(l.id);
    setTimeout(() => setCopied(null), 1200);
  }

  async function toggleArchive(l: Link) {
    await api.updateLink(l.id, { archived: !l.archived } as any);
    load();
  }

  return (
    <div>
      <Header title="Links" subtitle="Short links with click analytics, tags & notes">
        <button className="btn-primary" onClick={() => setEditing("new")}>
          + New link
        </button>
      </Header>

      <div className="mb-4 flex items-center gap-2">
        <input
          className="input"
          placeholder="Search slug, target, note, tag…"
          value={q}
          onChange={(e) => setQ(e.target.value)}
        />
        <button
          className={archived ? "btn-primary" : "btn-ghost"}
          onClick={() => setArchived((a) => !a)}
          title="Toggle archived view"
        >
          {archived ? "Archived" : "Active"}
        </button>
      </div>

      {links.length === 0 ? (
        <Empty>
          <span className="text-3xl">🔗</span>
          <p>No links yet</p>
        </Empty>
      ) : (
        <div className="card divide-y divide-zinc-800">
          {links.map((l) => (
            <div key={l.id} className="flex items-center gap-3 p-3">
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <span className="font-medium text-indigo-300">
                    /{l.slug}
                    {l.host && <span className="text-zinc-500"> @{l.host}</span>}
                  </span>
                  {!l.enabled && <span className="badge">disabled</span>}
                  {l.hasPassword && <span className="badge">🔒</span>}
                  {l.expiresAt && <span className="badge">⏳</span>}
                  {l.clickLimit > 0 && <span className="badge">≤{l.clickLimit}</span>}
                  {l.tags
                    ?.split(",")
                    .map((t) => t.trim())
                    .filter(Boolean)
                    .map((t) => (
                      <span key={t} className="badge bg-indigo-500/15 text-indigo-300">
                        #{t}
                      </span>
                    ))}
                </div>
                <div className="truncate text-sm text-zinc-400">{l.target}</div>
                {l.note && <div className="mt-0.5 truncate text-xs text-amber-300/80">📝 {l.note}</div>}
              </div>
              <div className="hidden text-right text-sm sm:block">
                <div className="font-semibold">{l.clicks}</div>
                <div className="text-xs text-zinc-500">clicks</div>
              </div>
              <div className="flex items-center gap-1">
                <button className="btn-ghost px-2" title="Copy link" onClick={() => copy(l)}>
                  {copied === l.id ? "✓" : "⧉"}
                </button>
                <button className="btn-ghost px-2" title="Stats" onClick={() => setStatsFor(l)}>
                  📊
                </button>
                <button className="btn-ghost px-2" title="QR" onClick={() => setQrFor(l)}>
                  ▦
                </button>
                <button className="btn-ghost px-2" onClick={() => setEditing(l)}>
                  Edit
                </button>
                <button
                  className="btn-ghost px-2"
                  title={l.archived ? "Unarchive" : "Archive"}
                  onClick={() => toggleArchive(l)}
                >
                  {l.archived ? "↩" : "🗄"}
                </button>
                <button
                  className="btn-danger px-2"
                  onClick={async () => {
                    if (confirm(`Delete /${l.slug}?`)) {
                      await api.deleteLink(l.id);
                      load();
                    }
                  }}
                >
                  Del
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      {editing && (
        <LinkEditor
          link={editing === "new" ? null : editing}
          hosts={linkHostOptions}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            load();
          }}
        />
      )}
      {statsFor && <StatsModal link={statsFor} onClose={() => setStatsFor(null)} />}
      {qrFor && <QRModal link={qrFor} onClose={() => setQrFor(null)} />}
    </div>
  );
}

export function Header({
  title,
  subtitle,
  children,
}: {
  title: string;
  subtitle?: string;
  children?: React.ReactNode;
}) {
  return (
    <div className="mb-5 flex items-end justify-between">
      <div>
        <h1 className="text-2xl font-bold">{title}</h1>
        {subtitle && <p className="text-sm text-zinc-500">{subtitle}</p>}
      </div>
      {children}
    </div>
  );
}

function LinkEditor({
  link,
  hosts,
  onClose,
  onSaved,
}: {
  link: Link | null;
  hosts: string[];
  onClose: () => void;
  onSaved: () => void;
}) {
  const [slug, setSlug] = useState(link?.slug ?? "");
  const [host, setHost] = useState(link?.host ?? "");
  const [target, setTarget] = useState(link?.target ?? "");
  const [title, setTitle] = useState(link?.title ?? "");
  const [note, setNote] = useState(link?.note ?? "");
  const [tags, setTags] = useState(link?.tags ?? "");
  const [password, setPassword] = useState("");
  const [expiresAt, setExpiresAt] = useState(link?.expiresAt?.slice(0, 16) ?? "");
  const [expiredUrl, setExpiredUrl] = useState(link?.expiredUrl ?? "");
  const [clickLimit, setClickLimit] = useState(link?.clickLimit ?? 0);
  const [enabled, setEnabled] = useState(link?.enabled ?? true);
  const [err, setErr] = useState("");
  const [fetching, setFetching] = useState(false);
  const [showUtm, setShowUtm] = useState(false);

  async function fetchTitle() {
    if (!target) return;
    setFetching(true);
    try {
      const m = await api.linkMetadata(target);
      if (m.title) setTitle(m.title);
    } catch {
      /* ignore */
    } finally {
      setFetching(false);
    }
  }

  async function save() {
    setErr("");
    const payload: any = {
      slug,
      host,
      target,
      title,
      note,
      tags,
      password,
      enabled,
      expiredUrl,
      clickLimit: Number(clickLimit) || 0,
      expiresAt: expiresAt ? new Date(expiresAt).toISOString() : null,
    };
    try {
      if (link) await api.updateLink(link.id, payload);
      else await api.createLink(payload);
      onSaved();
    } catch (e: any) {
      setErr(e.message ?? "save failed");
    }
  }

  return (
    <Modal title={link ? "Edit link" : "New link"} onClose={onClose}>
      <Field label="Destination URL">
        <div className="flex gap-2">
          <input
            className="input"
            value={target}
            onChange={(e) => setTarget(e.target.value)}
            placeholder="example.com/page"
          />
          <button className="btn-ghost shrink-0" onClick={() => setShowUtm((v) => !v)} title="UTM builder">
            UTM
          </button>
        </div>
      </Field>
      {showUtm && <UtmBuilder target={target} onApply={setTarget} />}
      <div className="grid grid-cols-2 gap-3">
        <Field label="Slug" hint="blank = random">
          <input className="input" value={slug} onChange={(e) => setSlug(e.target.value)} />
        </Field>
        <Field label="Host" hint={hosts.length ? "short-link host (subdomain)" : "enable short links on a domain first"}>
          <select className="input" value={host} onChange={(e) => setHost(e.target.value)}>
            <option value="">Any / default</option>
            {hosts.map((h) => (
              <option key={h} value={h}>
                {h}
              </option>
            ))}
            {host && !hosts.includes(host) && <option value={host}>{host}</option>}
          </select>
        </Field>
      </div>
      <Field label="Title">
        <div className="flex gap-2">
          <input className="input" value={title} onChange={(e) => setTitle(e.target.value)} />
          <button className="btn-ghost shrink-0" onClick={fetchTitle} disabled={fetching} title="Fetch from page">
            {fetching ? "…" : "Auto"}
          </button>
        </div>
      </Field>
      <Field label="Tags" hint="comma-separated">
        <input className="input" value={tags} onChange={(e) => setTags(e.target.value)} placeholder="marketing, launch" />
      </Field>
      <Field label="Note" hint="Private remark (not shown to visitors)">
        <textarea className="input" rows={2} value={note} onChange={(e) => setNote(e.target.value)} />
      </Field>
      <div className="grid grid-cols-2 gap-3">
        <Field label="Password" hint={link?.hasPassword ? "set, leave blank to keep" : "optional"}>
          <input className="input" value={password} onChange={(e) => setPassword(e.target.value)} />
        </Field>
        <Field label="Click limit" hint="0 = unlimited">
          <input
            type="number"
            min={0}
            className="input"
            value={clickLimit}
            onChange={(e) => setClickLimit(Number(e.target.value))}
          />
        </Field>
      </div>
      <div className="grid grid-cols-2 gap-3">
        <Field label="Expires at">
          <input type="datetime-local" className="input" value={expiresAt} onChange={(e) => setExpiresAt(e.target.value)} />
        </Field>
        <Field label="Expired URL" hint="redirect here once expired / over limit">
          <input className="input" value={expiredUrl} onChange={(e) => setExpiredUrl(e.target.value)} placeholder="optional" />
        </Field>
      </div>
      <div className="mb-4 flex items-center gap-2">
        <Toggle on={enabled} onChange={setEnabled} />
        <span className="text-sm text-zinc-400">Enabled</span>
      </div>
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

// UtmBuilder appends UTM query params to the destination URL (dub-style).
function UtmBuilder({ target, onApply }: { target: string; onApply: (url: string) => void }) {
  const [utm, setUtm] = useState({ source: "", medium: "", campaign: "", term: "", content: "" });
  function apply() {
    if (!target) return;
    let base = target;
    if (!base.includes("://")) base = "https://" + base;
    try {
      const u = new URL(base);
      const map: Record<string, string> = {
        utm_source: utm.source,
        utm_medium: utm.medium,
        utm_campaign: utm.campaign,
        utm_term: utm.term,
        utm_content: utm.content,
      };
      for (const [k, v] of Object.entries(map)) {
        if (v) u.searchParams.set(k, v);
        else u.searchParams.delete(k);
      }
      onApply(u.toString());
    } catch {
      /* ignore */
    }
  }
  const fields: [keyof typeof utm, string][] = [
    ["source", "source"],
    ["medium", "medium"],
    ["campaign", "campaign"],
    ["term", "term"],
    ["content", "content"],
  ];
  return (
    <div className="card mb-3 grid grid-cols-2 gap-2 p-3">
      {fields.map(([k, label]) => (
        <input
          key={k}
          className="input"
          placeholder={`utm_${label}`}
          value={utm[k]}
          onChange={(e) => setUtm({ ...utm, [k]: e.target.value })}
        />
      ))}
      <button className="btn-primary col-span-2" onClick={apply}>
        Apply UTM to URL
      </button>
    </div>
  );
}

function StatsModal({ link, onClose }: { link: Link; onClose: () => void }) {
  const [stats, setStats] = useState<LinkStats | null>(null);
  useEffect(() => {
    api.linkStats(link.id).then(setStats);
  }, [link.id]);
  return (
    <Modal title={`Stats · /${link.slug}`} onClose={onClose} wide>
      {!stats ? (
        <p className="text-zinc-500">loading…</p>
      ) : (
        <div className="space-y-4">
          <div className="grid grid-cols-3 gap-3">
            <Stat label="Total clicks" value={stats.total} />
            <Stat label={`Last ${stats.days}d`} value={stats.windowed} />
            <Stat label="Days tracked" value={stats.series.length} />
          </div>
          <Spark series={stats.series} />
          <div className="grid grid-cols-2 gap-4">
            <TopList title="Countries" rows={stats.countries} />
            <TopList title="Devices" rows={stats.devices} />
            <TopList title="Browsers" rows={stats.browsers} />
            <TopList title="Referers" rows={stats.referers} />
          </div>
        </div>
      )}
    </Modal>
  );
}

function Stat({ label, value }: { label: string; value: number }) {
  return (
    <div className="card p-3">
      <div className="text-2xl font-bold">{value}</div>
      <div className="text-xs text-zinc-500">{label}</div>
    </div>
  );
}

function Spark({ series }: { series: { key: string; count: number }[] }) {
  if (!series.length) return <p className="text-sm text-zinc-500">No clicks in window.</p>;
  const max = Math.max(...series.map((s) => s.count), 1);
  return (
    <div className="card flex h-28 items-end gap-1 p-3">
      {series.map((s) => (
        <div
          key={s.key}
          title={`${s.key}: ${s.count}`}
          className="flex-1 rounded-t bg-indigo-500/70"
          style={{ height: `${(s.count / max) * 100}%`, minHeight: 2 }}
        />
      ))}
    </div>
  );
}

function TopList({ title, rows }: { title: string; rows: { key: string; count: number }[] | null }) {
  return (
    <div>
      <div className="mb-1 text-xs font-semibold uppercase tracking-wide text-zinc-500">{title}</div>
      {!rows || rows.length === 0 ? (
        <p className="text-sm text-zinc-600">—</p>
      ) : (
        <div className="space-y-1">
          {rows.map((r) => (
            <div key={r.key} className="flex justify-between text-sm">
              <span className="truncate text-zinc-300">{r.key || "(direct)"}</span>
              <span className="text-zinc-500">{r.count}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function QRModal({ link, onClose }: { link: Link; onClose: () => void }) {
  return (
    <Modal title={`QR · /${link.slug}`} onClose={onClose}>
      <div className="flex flex-col items-center gap-3">
        <img
          src={`/api/links/${link.id}/qr`}
          alt="QR"
          className="rounded-lg bg-white p-2"
          width={240}
          height={240}
        />
        <a className="btn-ghost" href={`/api/links/${link.id}/qr`} download={`${link.slug}.png`}>
          Download PNG
        </a>
      </div>
    </Modal>
  );
}
