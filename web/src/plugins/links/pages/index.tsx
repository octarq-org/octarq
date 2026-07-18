import { useEffect, useState } from "react";
import { api, Domain, effectiveLinkHosts } from "../../../api";
import { linksApi, Link, LinkStats } from "../api";
import { Empty, Field, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button, StatCard } from "../../../ui";
import { Link2, Copy, Archive, Trash2, QrCode, Download, Eye, ExternalLink, Calendar, Search, Tag, Globe, Settings } from "lucide-react";
import { LinkSettings } from "./LinkSettings";
import { useTranslation } from "../../../i18n";

import { LinkEditorForm } from "./LinkEditorForm";
import { StatsView } from "./StatsView";
import { usePluginGate } from "../../PluginGate";

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
  const [tab, setTab] = useState<'links' | 'settings'>('links');
  const { t } = useTranslation();
  const pluginGate = usePluginGate();

  const linkHostOptions = Array.from(new Set(domains.flatMap(effectiveLinkHosts)));

  async function loadMore(reset = false) {
    if (loading || (!hasMore && !reset)) return;
    setLoading(true);
    try {
      const limit = 50;
      const offset = reset ? 0 : page * limit;
      const res = await linksApi.links({ q, archived, limit, offset });
      if (res.length < limit) setHasMore(false);
      else setHasMore(true);

      setLinks(prev => reset ? res : [...prev, ...res]);
      setPage(reset ? 1 : page + 1);
    } catch (e: any) {
      if (e.status === 404 || e.status === 402) {
        pluginGate.degrade(e.status);
      }
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    const timer = setTimeout(() => {
      loadMore(true);
    }, 200);
    return () => clearTimeout(timer);
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
    await linksApi.updateLink(l.id, { archived: !l.archived } as any);
    loadMore(true);
    if (active && active !== "new" && active.id === l.id) {
      setActive({ ...active, archived: !l.archived });
    }
  }

  return (
    <ScreenWrap>
      <PageHeader
        title={t("links.pageTitle")}
        description={t("links.pageDescription")}
        action={
          <div className="flex items-center gap-2">
            <Button variant="primary" onClick={() => setActive("new")} className="gap-1.5 py-1.5 text-xs">
              {t("links.newLink")}
            </Button>
          </div>
        }
      />

      <div className="flex gap-0 border-b border-white/[0.06] mb-6">
        <button
          onClick={() => setTab('links')}
          className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors ${
            tab === 'links'
              ? 'border-indigo-500 text-white'
              : 'border-transparent text-white/45 hover:text-white/70'
          }`}
        >
          {t("links.tabLinks")}
        </button>
        <button
          onClick={() => setTab('settings')}
          className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors flex items-center gap-1.5 ${
            tab === 'settings'
              ? 'border-indigo-500 text-white'
              : 'border-transparent text-white/45 hover:text-white/70'
          }`}
        >
          {t("links.tabSettings")}
        </button>
      </div>

      {tab === 'links' && (
      <div className="grid grid-cols-1 lg:grid-cols-[300px_1fr] gap-6 min-h-0 items-start">
        {/* Left column - links list */}
        <div className="flex flex-col min-h-0 w-full">
          <div className="mb-3 flex items-center gap-2">
            <div className="relative flex-1">
              <input
                className="input w-full !pl-8"
                placeholder={t("links.searchPlaceholder")}
                value={q}
                onChange={(e) => setQ(e.target.value)}
              />
              <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-white/50" />
            </div>
            <Button
              variant={archived ? "primary" : "subtle"}
              onClick={() => setArchived((a) => !a)}
              className="shrink-0 py-2 px-3 text-xs"
              title={t("links.toggleArchivedTitle")}
            >
              {archived ? t("links.archived") : t("links.active")}
            </Button>
          </div>
          
          <GlassCard className="overflow-hidden">
            <div className="overflow-y-auto max-h-[600px] divide-y divide-white/[0.04]" onScroll={handleScroll}>
              {links.length === 0 && !loading ? (
                <div className="p-8 text-center text-white/40 text-sm">{t("links.noLinksFound")}</div>
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
                          {t("links.clicksCount", { count: l.clicks })}
                        </Badge>
                      </div>
                      <div className="truncate text-xs text-white/50 mt-1.5 font-mono">{l.target}</div>
                    </button>
                  ))}
                  {loading && <div className="p-3 text-center text-xs text-white/40">{t("links.loading")}</div>}
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
                {t("links.createNewLink")}
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
                      {copied === active.id ? t("links.copied") : t("links.copyLink")}
                    </Button>
                    <Button variant="outline" className="text-xs py-1.5 px-3 gap-1.5" onClick={() => toggleArchive(active)}>
                      <Archive className="h-3.5 w-3.5" />
                      {active.archived ? t("links.unarchive") : t("links.archive")}
                    </Button>
                    <Button
                      variant="danger"
                      onClick={async () => {
                        if (confirm(t("links.confirmDelete", { slug: active.slug }))) {
                          await linksApi.deleteLink(active.id);
                          setActive(null);
                          loadMore(true);
                        }
                      }}
                      className="text-xs py-1.5 px-3 gap-1.5 bg-rose-500/10 hover:bg-rose-500/20 text-rose-300 border-0"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                      {t("links.delete")}
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
                  {t("links.linkQrCode")}
                </h3>
                <div className="bg-white p-3 rounded-2xl shadow-glow">
                  <img
                    src={`/api/links/${active.id}/qr`}
                    alt={t("links.qrAlt")}
                    className="rounded-lg"
                    width={180}
                    height={180}
                  />
                </div>
                <Button variant="ghost" className="text-xs py-1.5 px-4 gap-1.5" onClick={() => window.open(`/api/links/${active.id}/qr`)}>
                  <Download className="h-3.5 w-3.5" />
                  {t("links.downloadQrCode")}
                </Button>
              </GlassCard>
            </div>
          ) : (
            <GlassCard className="flex flex-col items-center justify-center py-20 px-6 text-center text-white/40 border border-white/[0.04]/40">
              <Link2 className="h-10 w-10 mb-2 opacity-50 text-indigo-400" />
              <p className="text-sm">{t("links.emptyDetail")}</p>
            </GlassCard>
          )}
        </div>
      </div>
      )}
      {tab === 'settings' && (
        <GlassCard className="p-6">
          <LinkSettings />
        </GlassCard>
      )}
    </ScreenWrap>
  );
}

