import { useEffect, useState } from "react";
import { api, Domain, effectiveLinkHosts, Link, LinkStats } from "../api";
import { Empty, Field, Toggle, timeAgo } from "../ui";

export default function LinksPage() {
  const [links, setLinks] = useState<Link[]>([]);
  const [domains, setDomains] = useState<Domain[]>([]);
  const [q, setQ] = useState("");
  const [archived, setArchived] = useState(false);
  const [active, setActive] = useState<Link | "new" | null>(null);

  const [page, setPage] = useState(0);
  const [hasMore, setHasMore] = useState(true);
  const [loading, setLoading] = useState(false);
  const [copied, setCopied] = useState<number | null>(null);

  const linkHostOptions = Array.from(new Set(domains.flatMap(effectiveLinkHosts)));

  async function loadMore(reset = false) {
    if (loading || (!hasMore && !reset)) return;
    setLoading(true);
    try {
      const limit = 50;
      const offset = reset ? 0 : page * limit;
      const res = await api.links({ q, archived, limit, offset });
      if (res.length < limit) setHasMore(false);
      else setHasMore(true);

      setLinks(prev => reset ? res : [...prev, ...res]);
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
  }, [q, archived]);

  useEffect(() => {
    api.domains().then(setDomains).catch(() => {});
  }, []);

  const handleScroll = (e: React.UIEvent<HTMLDivElement>) => {
    const bottom = e.currentTarget.scrollHeight - e.currentTarget.scrollTop <= e.currentTarget.clientHeight + 100;
    if (bottom) loadMore();
  };

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
    loadMore(true);
    if (active && active !== "new" && active.id === l.id) {
      setActive({ ...active, archived: !l.archived });
    }
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <Header title="Links" subtitle="Short links with click analytics, tags & notes">
        <button className="btn-primary" onClick={() => setActive("new")}>
          + New link
        </button>
      </Header>

      <div className="grid grid-cols-[300px_1fr] gap-4 min-h-0 flex-1">
        {/* left column */}
        <div className="flex flex-col min-h-0">
          <div className="mb-2 flex items-center gap-2">
            <input
              className="input flex-1 min-w-0"
              placeholder="Search…"
              value={q}
              onChange={(e) => setQ(e.target.value)}
            />
            <button
              className={archived ? "btn-primary shrink-0" : "btn-ghost shrink-0"}
              onClick={() => setArchived((a) => !a)}
              title="Toggle archived view"
            >
              {archived ? "Arc" : "Act"}
            </button>
          </div>
          <div className="card flex-1 overflow-y-auto" onScroll={handleScroll}>
            {links.length === 0 && !loading ? (
              <div className="p-8 text-center text-white/40">No links found.</div>
            ) : (
              <div className="divide-y divide-white/[0.04]">
                {links.map((l) => (
                  <button
                    key={l.id}
                    className={`flex w-full flex-col p-3 text-left hover:bg-white/[0.04] transition-colors ${
                      active !== "new" && active?.id === l.id ? "bg-white/[0.06]" : ""
                    }`}
                    onClick={() => setActive(l)}
                  >
                    <div className="flex items-center gap-2 w-full">
                      <span className="font-medium text-indigo-300 truncate flex-1">
                        /{l.slug}
                      </span>
                      <span className="text-xs text-white/40 shrink-0">{l.clicks} clicks</span>
                    </div>
                    <div className="truncate text-xs text-white/55 mt-1">{l.target}</div>
                  </button>
                ))}
                {loading && <div className="p-3 text-center text-xs text-white/40">Loading…</div>}
              </div>
            )}
          </div>
        </div>

        {/* right column */}
        <div className="min-h-0 overflow-y-auto pr-2 pb-8">
          {active === "new" ? (
             <div className="card p-5">
               <h2 className="mb-4 text-xl font-semibold">New Link</h2>
               <LinkEditorForm
                 link={null}
                 hosts={linkHostOptions}
                 onCancel={() => setActive(null)}
                 onSaved={(savedLink) => {
                   loadMore(true);
                   setActive(savedLink || null);
                 }}
               />
             </div>
          ) : active ? (
             <div className="space-y-4">
                <div className="card p-5">
                  <div className="flex justify-between mb-4">
                     <h2 className="text-xl font-semibold">Edit Link</h2>
                     <div className="flex gap-2">
                       <button className="btn-ghost px-2 text-sm" onClick={() => copy(active)}>
                         {copied === active.id ? "Copied!" : "Copy URL"}
                       </button>
                       <button className="btn-ghost px-2 text-sm" onClick={() => toggleArchive(active)}>
                         {active.archived ? "Unarchive" : "Archive"}
                       </button>
                       <button
                         className="btn-danger px-2 text-sm"
                         onClick={async () => {
                           if (confirm(`Delete /${active.slug}?`)) {
                             await api.deleteLink(active.id);
                             setActive(null);
                             loadMore(true);
                           }
                         }}
                       >
                         Delete
                       </button>
                     </div>
                  </div>
                  <LinkEditorForm
                    key={active.id}
                    link={active}
                    hosts={linkHostOptions}
                    onCancel={() => setActive(null)}
                    onSaved={(l) => {
                      if (l) setActive(l);
                      loadMore(true);
                    }}
                  />
                </div>
                
                <StatsView link={active} />
                
                <div className="card p-5 flex flex-col items-center gap-3">
                  <h3 className="text-sm font-semibold text-white/55 self-start">QR Code</h3>
                  <img
                    src={`/api/links/${active.id}/qr`}
                    alt="QR"
                    className="rounded-lg bg-white p-2"
                    width={200}
                    height={200}
                  />
                  <a className="btn-ghost text-sm" href={`/api/links/${active.id}/qr`} download={`${active.slug}.png`}>
                    Download PNG
                  </a>
                </div>
             </div>
          ) : (
            <div className="flex h-full items-center justify-center text-white/40/50">
              Select a link to view details
            </div>
          )}
        </div>
      </div>
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
    <div className="mb-6 flex flex-wrap items-start justify-between gap-4 shrink-0">
      <div>
        <h1 className="font-display text-2xl font-bold tracking-tight text-white">{title}</h1>
        {subtitle && <p className="mt-1 text-sm text-white/50">{subtitle}</p>}
      </div>
      {children}
    </div>
  );
}

function LinkEditorForm({
  link,
  hosts,
  onCancel,
  onSaved,
}: {
  link: Link | null;
  hosts: string[];
  onCancel: () => void;
  onSaved: (l?: any) => void;
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
      let res;
      if (link) res = await api.updateLink(link.id, payload);
      else res = await api.createLink(payload);
      onSaved(res);
    } catch (e: any) {
      setErr(e.message ?? "save failed");
    }
  }

  return (
    <div className="space-y-4">
      <Field label="Destination URL">
        <div className="flex gap-2">
          <input
            className="input w-full"
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
          <input className="input w-full" value={slug} onChange={(e) => setSlug(e.target.value)} />
        </Field>
        <Field label="Host" hint={hosts.length ? "short-link host" : "enable short links first"}>
          <select className="input w-full" value={host} onChange={(e) => setHost(e.target.value)}>
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
          <input className="input w-full" value={title} onChange={(e) => setTitle(e.target.value)} />
          <button className="btn-ghost shrink-0" onClick={fetchTitle} disabled={fetching} title="Fetch from page">
            {fetching ? "…" : "Auto"}
          </button>
        </div>
      </Field>

      <Field label="Tags" hint="comma-separated">
        <input className="input w-full" value={tags} onChange={(e) => setTags(e.target.value)} placeholder="marketing, launch" />
      </Field>

      <Field label="Note" hint="Private remark">
        <textarea className="input w-full" rows={2} value={note} onChange={(e) => setNote(e.target.value)} />
      </Field>

      <div className="grid grid-cols-2 gap-3">
        <Field label="Password" hint={link?.hasPassword ? "set, leave blank to keep" : "optional"}>
          <input className="input w-full" value={password} onChange={(e) => setPassword(e.target.value)} />
        </Field>
        <Field label="Click limit" hint="0 = unlimited">
          <input
            type="number"
            min={0}
            className="input w-full"
            value={clickLimit}
            onChange={(e) => setClickLimit(Number(e.target.value))}
          />
        </Field>
      </div>

      <div className="grid grid-cols-2 gap-3">
        <Field label="Expires at">
          <input type="datetime-local" className="input w-full" value={expiresAt} onChange={(e) => setExpiresAt(e.target.value)} />
        </Field>
        <Field label="Expired URL">
          <input className="input w-full" value={expiredUrl} onChange={(e) => setExpiredUrl(e.target.value)} placeholder="optional" />
        </Field>
      </div>

      <div className="flex items-center gap-2 pt-2">
        <Toggle on={enabled} onChange={setEnabled} />
        <span className="text-sm text-white/55">Enabled</span>
      </div>

      {err && <p className="text-sm text-red-400">{err}</p>}

      <div className="flex justify-end gap-2 pt-4 border-t border-white/[0.06]">
        <button className="btn-ghost" onClick={onCancel}>
          Cancel
        </button>
        <button className="btn-primary" onClick={save}>
          Save
        </button>
      </div>
    </div>
  );
}

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
    <div className="card grid grid-cols-2 gap-2 p-3 bg-[#07070b]/30">
      {fields.map(([k, label]) => (
        <input
          key={k}
          className="input w-full"
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

function StatsView({ link }: { link: Link }) {
  const [stats, setStats] = useState<LinkStats | null>(null);
  useEffect(() => {
    setStats(null);
    api.linkStats(link.id).then(setStats);
  }, [link.id]);
  
  if (!stats) return <div className="card p-5 text-white/40">Loading stats…</div>;
  
  return (
    <div className="card p-5 space-y-4">
      <h3 className="text-sm font-semibold text-white/55">Analytics</h3>
      <div className="grid grid-cols-3 gap-3">
        <Stat label="Total clicks" value={stats.total} />
        <Stat label={`Last ${stats.days}d`} value={stats.windowed} />
        <Stat label="Days tracked" value={stats.series?.length || 0} />
      </div>
      <Spark series={stats.series} />
      <div className="grid grid-cols-2 md:grid-cols-3 gap-4">
        <TopList title="Countries" rows={stats.countries} />
        <TopList title="Regions" rows={stats.regions} />
        <TopList title="Devices" rows={stats.devices} />
        <TopList title="Browsers" rows={stats.browsers} />
        <TopList title="Referers" rows={stats.referers} />
      </div>
    </div>
  );
}

function Stat({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded bg-white/[0.04] border border-white/[0.06] p-3">
      <div className="text-xl font-bold text-white/80">{value}</div>
      <div className="text-xs text-white/40">{label}</div>
    </div>
  );
}

function Spark({ series }: { series: { key: string; count: number }[] }) {
  if (!series || !series.length) return <p className="text-sm text-white/40">No clicks in window.</p>;
  const max = Math.max(...series.map((s) => s.count), 1);
  return (
    <div className="rounded bg-white/[0.04] border border-white/[0.06] flex h-28 items-end gap-1 p-3">
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
      <div className="mb-1 text-xs font-semibold uppercase tracking-wide text-white/40">{title}</div>
      {!rows || rows.length === 0 ? (
        <p className="text-sm text-white/30">—</p>
      ) : (
        <div className="space-y-1">
          {rows.map((r) => (
            <div key={r.key} className="flex justify-between text-sm">
              <span className="truncate text-white/75 mr-2">{r.key || "(direct)"}</span>
              <span className="text-white/40">{r.count}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
