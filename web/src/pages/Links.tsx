import { useEffect, useState } from "react";
import { api, Domain, effectiveLinkHosts, Link, LinkStats } from "../api";
import { Empty, Field, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button, StatCard } from "../ui";
import { Link2, Copy, Archive, Trash2, QrCode, Download, Eye, ExternalLink, Calendar, Search, Tag, Globe } from "lucide-react";

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
    <ScreenWrap>
      <PageHeader
        title="Links"
        description="Short links with click analytics, redirection & routing"
        action={
          <Button variant="primary" onClick={() => setActive("new")} className="gap-1.5 py-1.5 text-xs">
            + New Link
          </Button>
        }
      />

      <div className="grid grid-cols-1 lg:grid-cols-[300px_1fr] gap-6 min-h-0 items-start">
        {/* Left column - links list */}
        <div className="flex flex-col min-h-0 w-full">
          <div className="mb-3 flex items-center gap-2">
            <div className="relative flex-1">
              <input
                className="input w-full pl-8"
                placeholder="Search links…"
                value={q}
                onChange={(e) => setQ(e.target.value)}
              />
              <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-white/30" />
            </div>
            <Button
              variant={archived ? "primary" : "subtle"}
              onClick={() => setArchived((a) => !a)}
              className="shrink-0 py-2 px-3 text-xs"
              title="Toggle archived view"
            >
              {archived ? "Archived" : "Active"}
            </Button>
          </div>
          
          <GlassCard className="overflow-hidden">
            <div className="overflow-y-auto max-h-[600px] divide-y divide-white/[0.04]" onScroll={handleScroll}>
              {links.length === 0 && !loading ? (
                <div className="p-8 text-center text-white/40 text-sm">No links found.</div>
              ) : (
                <>
                  {links.map((l) => (
                    <button
                      key={l.id}
                      className={`flex w-full flex-col p-4 text-left hover:bg-white/[0.03] transition-colors ${
                        active !== "new" && active?.id === l.id ? "bg-white/[0.05]" : ""
                      }`}
                      onClick={() => setActive(l)}
                    >
                      <div className="flex items-center gap-2 w-full justify-between">
                        <span className="font-semibold text-sm text-indigo-300 truncate flex-1">
                          /{l.slug}
                        </span>
                        <Badge tone="neutral" className="text-[10px]">
                          {l.clicks} clicks
                        </Badge>
                      </div>
                      <div className="truncate text-xs text-white/50 mt-1.5 font-mono">{l.target}</div>
                    </button>
                  ))}
                  {loading && <div className="p-3 text-center text-xs text-white/40">Loading…</div>}
                </>
              )}
            </div>
          </GlassCard>
        </div>

        {/* Right column - detail editor / viewer */}
        <div className="w-full space-y-6">
          {active === "new" ? (
            <GlassCard className="p-5">
              <h2 className="mb-4 text-lg font-bold text-white flex items-center gap-2">
                <Link2 className="h-5 w-5 text-indigo-400" />
                Create New Link
              </h2>
              <LinkEditorForm
                link={null}
                hosts={linkHostOptions}
                onCancel={() => setActive(null)}
                onSaved={(savedLink) => {
                  loadMore(true);
                  setActive(savedLink || null);
                }}
              />
            </GlassCard>
          ) : active ? (
            <div className="space-y-6">
              <GlassCard className="p-5">
                <div className="flex flex-wrap justify-between items-center mb-5 border-b border-white/[0.06] pb-4 gap-4">
                  <h2 className="text-lg font-bold text-white flex items-center gap-2">
                    <Link2 className="h-5 w-5 text-indigo-400" />
                    /{active.slug}
                  </h2>
                  <div className="flex flex-wrap gap-2">
                    <Button variant="subtle" className="text-xs py-1.5 px-3 gap-1.5" onClick={() => copy(active)}>
                      <Copy className="h-3.5 w-3.5" />
                      {copied === active.id ? "Copied!" : "Copy Link"}
                    </Button>
                    <Button variant="outline" className="text-xs py-1.5 px-3 gap-1.5" onClick={() => toggleArchive(active)}>
                      <Archive className="h-3.5 w-3.5" />
                      {active.archived ? "Unarchive" : "Archive"}
                    </Button>
                    <Button
                      variant="danger"
                      onClick={async () => {
                        if (confirm(`Delete /${active.slug}?`)) {
                          await api.deleteLink(active.id);
                          setActive(null);
                          loadMore(true);
                        }
                      }}
                      className="text-xs py-1.5 px-3 gap-1.5 bg-rose-500/10 hover:bg-rose-500/20 text-rose-300 border-0"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                      Delete
                    </Button>
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
              </GlassCard>
              
              <StatsView link={active} />
              
              <GlassCard className="p-5 flex flex-col items-center gap-4">
                <h3 className="text-sm font-semibold text-white/80 uppercase tracking-wider self-start flex items-center gap-1.5">
                  <QrCode className="h-4 w-4 text-indigo-400" />
                  Link QR Code
                </h3>
                <div className="bg-white p-3 rounded-2xl shadow-glow">
                  <img
                    src={`/api/links/${active.id}/qr`}
                    alt="QR"
                    className="rounded-lg"
                    width={180}
                    height={180}
                  />
                </div>
                <Button variant="ghost" className="text-xs py-1.5 px-4 gap-1.5" onClick={() => window.open(`/api/links/${active.id}/qr`)}>
                  <Download className="h-3.5 w-3.5" />
                  Download QR Code
                </Button>
              </GlassCard>
            </div>
          ) : (
            <GlassCard className="flex flex-col items-center justify-center py-20 px-6 text-center text-white/40 border border-white/[0.04]/40">
              <Link2 className="h-10 w-10 mb-2 opacity-50 text-indigo-400" />
              <p className="text-sm">Select a short link from the sidebar list to inspect click history & metrics.</p>
            </GlassCard>
          )}
        </div>
      </div>
    </ScreenWrap>
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
      <Field label="Destination Target URL">
        <div className="flex gap-2">
          <input
            className="input w-full font-mono text-sm"
            value={target}
            onChange={(e) => setTarget(e.target.value)}
            placeholder="https://example.com/blog-post-xyz"
            required
          />
          <Button variant="subtle" className="shrink-0 text-xs py-1" type="button" onClick={() => setShowUtm((v) => !v)}>
            UTM
          </Button>
        </div>
      </Field>
      {showUtm && <UtmBuilder target={target} onApply={setTarget} />}
      
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <Field label="Short Slug" hint="Leave empty for auto-generated slug">
          <input className="input w-full font-mono" value={slug} onChange={(e) => setSlug(e.target.value)} placeholder="e.g. promo2026" />
        </Field>
        <Field label="Routing Host Domain" hint={hosts.length ? "Configured domains" : "Configure domains first"}>
          <select className="input w-full" value={host} onChange={(e) => setHost(e.target.value)}>
            <option value="">Default (Apex Domain)</option>
            {hosts.map((h) => (
              <option key={h} value={h}>
                {h}
              </option>
            ))}
            {host && !hosts.includes(host) && <option value={host}>{host}</option>}
          </select>
        </Field>
      </div>

      <Field label="Metadata Page Title">
        <div className="flex gap-2">
          <input className="input w-full text-sm" value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Auto-populated title for page previews" />
          <Button variant="subtle" className="shrink-0 text-xs py-1" type="button" onClick={fetchTitle} disabled={fetching}>
            {fetching ? "..." : "Fetch"}
          </Button>
        </div>
      </Field>

      <Field label="Tags" hint="Comma-separated tokens">
        <input className="input w-full text-sm" value={tags} onChange={(e) => setTags(e.target.value)} placeholder="e.g. q3-ads, product-hunt" />
      </Field>

      <Field label="Internal Admin Note">
        <textarea className="input w-full text-sm" rows={2} value={note} onChange={(e) => setNote(e.target.value)} placeholder="Notes visible only to team members" />
      </Field>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <Field label="Access Protection Password" hint={link?.hasPassword ? "Key set. Fill to overwrite, blank to keep" : "Optional password check"}>
          <input className="input w-full font-mono text-sm" type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder="••••••••" />
        </Field>
        <Field label="Total Click Limitation" hint="0 = Unlimited redirects">
          <input
            type="number"
            min={0}
            className="input w-full font-mono"
            value={clickLimit}
            onChange={(e) => setClickLimit(Number(e.target.value))}
          />
        </Field>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <Field label="Automatic Expiry Date">
          <input type="datetime-local" className="input w-full text-sm" value={expiresAt} onChange={(e) => setExpiresAt(e.target.value)} />
        </Field>
        <Field label="Redirect URL After Expiry" hint="Destination target after link has expired">
          <input className="input w-full text-sm font-mono" value={expiredUrl} onChange={(e) => setExpiredUrl(e.target.value)} placeholder="e.g. https://my-site.com/expired" />
        </Field>
      </div>

      <div className="flex items-center gap-3 pt-2">
        <Toggle on={enabled} onChange={setEnabled} />
        <span className="text-sm text-white/60 select-none">Link Routing Active</span>
      </div>

      {err && <p className="text-sm text-rose-400 font-medium">{err}</p>}

      <div className="flex justify-end gap-2.5 pt-4 border-t border-white/[0.06]">
        <Button variant="ghost" onClick={onCancel}>
          Cancel
        </Button>
        <Button variant="primary" onClick={save} disabled={!target}>
          Save Link
        </Button>
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
    <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-2.5 p-4 bg-black/30 border border-white/[0.05] rounded-xl">
      {fields.map(([k, label]) => (
        <input
          key={k}
          className="input w-full text-xs h-8"
          placeholder={`utm_${label}`}
          value={utm[k]}
          onChange={(e) => setUtm({ ...utm, [k]: e.target.value })}
        />
      ))}
      <Button variant="subtle" className="sm:col-span-2 md:col-span-3 h-8 text-xs py-1.5" onClick={apply}>
        Apply UTM Parameters
      </Button>
    </div>
  );
}

function StatsView({ link }: { link: Link }) {
  const [stats, setStats] = useState<LinkStats | null>(null);
  useEffect(() => {
    setStats(null);
    api.linkStats(link.id).then(setStats);
  }, [link.id]);
  
  if (!stats) return <div className="text-white/40 p-4 text-xs">Loading analytics…</div>;
  
  return (
    <GlassCard className="p-5 space-y-5">
      <h3 className="text-sm font-semibold text-white/80 uppercase tracking-wider flex items-center gap-1.5">
        <Eye className="h-4 w-4 text-indigo-400" />
        Click Performance Analytics
      </h3>
      <div className="grid grid-cols-3 gap-4">
        <StatCard label="Total Clicks" value={stats.total} index={0} />
        <StatCard label={`Last ${stats.days}d`} value={stats.windowed} index={1} />
        <StatCard label="Tracking Window" value={`${stats.series?.length || 0}d`} index={2} />
      </div>
      <Spark series={stats.series} />
      <div className="grid grid-cols-2 md:grid-cols-3 gap-6 pt-2 border-t border-white/[0.04]">
        <TopList title="Countries" icon={<Globe className="h-3 w-3 text-white/40 mr-1 inline" />} rows={stats.countries} />
        <TopList title="Regions" icon={<ExternalLink className="h-3 w-3 text-white/40 mr-1 inline" />} rows={stats.regions} />
        <TopList title="Devices" icon={<Eye className="h-3 w-3 text-white/40 mr-1 inline" />} rows={stats.devices} />
        <TopList title="Browsers" icon={<Link2 className="h-3 w-3 text-white/40 mr-1 inline" />} rows={stats.browsers} />
        <TopList title="Referers" icon={<Tag className="h-3 w-3 text-white/40 mr-1 inline" />} rows={stats.referers} />
      </div>
    </GlassCard>
  );
}

function Spark({ series }: { series: { key: string; count: number }[] }) {
  if (!series || !series.length) return <p className="text-xs text-white/40 italic">No click data in active window.</p>;
  const max = Math.max(...series.map((s) => s.count), 1);
  return (
    <div className="rounded-xl bg-black/30 border border-white/[0.05] flex h-24 items-end gap-1 p-3">
      {series.map((s) => (
        <div
          key={s.key}
          title={`${s.key}: ${s.count} clicks`}
          className="flex-1 rounded-t-md bg-indigo-500/70 hover:bg-indigo-400 transition-all cursor-pointer"
          style={{ height: `${(s.count / max) * 100}%`, minHeight: 3 }}
        />
      ))}
    </div>
  );
}

function TopList({ title, icon, rows }: { title: string; icon?: React.ReactNode; rows: { key: string; count: number }[] | null }) {
  return (
    <div className="space-y-2">
      <div className="text-[11px] font-semibold uppercase tracking-wider text-white/35 flex items-center">
        {icon}
        {title}
      </div>
      {!rows || rows.length === 0 ? (
        <p className="text-xs text-white/30 italic">—</p>
      ) : (
        <div className="space-y-1.5">
          {rows.map((r) => (
            <div key={r.key} className="flex justify-between text-xs font-normal">
              <span className="truncate text-white/70 mr-2 font-mono" title={r.key || "Direct/Unknown"}>{r.key || "(direct)"}</span>
              <span className="text-white/45 font-semibold font-mono">{r.count}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
