import { useEffect, useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { api, Domain, effectiveLinkHosts } from "../../../api";
import { linksApi, Link } from "../api";
import { Empty, ScreenWrap, PageHeader, GlassCard, Badge, Button, Input, Tabs, confirmDialog } from "../../../ui";
import { Link2, Copy, Archive, Trash2, QrCode, Download, Eye, Search, Settings } from "lucide-react";
import { useTranslation } from "../../../i18n";
import { roleSatisfies, useCurrentRole } from "../../../shell/role";

import { LinkEditorForm } from "./LinkEditorForm";
import { StatsView } from "./StatsView";
import { usePluginGate } from "../../PluginGate";
import { parseLinksFilter, buildLinksFilterQuery } from "../filters";
import { ListSkeleton } from "../../../components/ListSkeleton";

export default function LinksPage() {
  const { role, isInstanceAdmin } = useCurrentRole();
  const canDeleteLink = roleSatisfies("admin", role, isInstanceAdmin);
  const [links, setLinks] = useState<Link[]>([]);
  const [domains, setDomains] = useState<Domain[]>([]);

  const [searchParams, setSearchParams] = useSearchParams();
  const { q, archived } = useMemo(() => parseLinksFilter(searchParams), [searchParams]);
  const [searchInput, setSearchInput] = useState(q);

  useEffect(() => {
    setSearchInput(q);
  }, [q]);

  useEffect(() => {
    const timer = setTimeout(() => {
      if (searchInput !== q) {
        setSearchParams(prev => buildLinksFilterQuery({ q: searchInput, archived }, prev), { replace: true });
      }
    }, 250);
    return () => clearTimeout(timer);
  }, [searchInput, archived, q, setSearchParams]);

  const [active, setActive] = useState<Link | "new" | null>(null);

  const [page, setPage] = useState(0);
  const [hasMore, setHasMore] = useState(true);
  const [loading, setLoading] = useState(false);
  const [copied, setCopied] = useState<number | null>(null);
  const { t } = useTranslation();
  const pluginGate = usePluginGate();

  useEffect(() => {
    if (searchParams.get("create") === "1") {
      setActive("new");
      setSearchParams(prev => {
        const next = new URLSearchParams(prev);
        next.delete("create");
        return next;
      }, { replace: true });
    }
  }, [searchParams, setSearchParams]);

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
    loadMore(true);
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

      <div className="grid grid-cols-1 lg:grid-cols-[300px_1fr] gap-6 min-h-0 items-start">
        {/* Left column - links list */}
        <div className="flex flex-col min-h-0 w-full min-w-0">
          <div className="mb-3 flex items-center gap-2">
            <div className="relative flex-1 min-w-0">
              <Input
                className="w-full !pl-8 text-sm min-w-0"
                placeholder={t("links.searchPlaceholder")}
                value={searchInput}
                onChange={(e) => setSearchInput(e.target.value)}
              />
              <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-foreground/50 pointer-events-none" />
            </div>
            <Button
              variant={archived ? "primary" : "subtle"}
              onClick={() => {
                setSearchParams(prev => buildLinksFilterQuery({ q: searchInput, archived: !archived }, prev), { replace: true });
              }}
              className="shrink-0 py-2 px-3 text-xs"
              title={t("links.toggleArchivedTitle")}
            >
              {archived ? t("links.archived") : t("links.active")}
            </Button>
          </div>
          
          <GlassCard className="overflow-hidden">
            {links.length === 0 && loading ? (
              <ListSkeleton rows={7} ariaLabel={t("links.loading")} />
            ) : (
            <div className="overflow-y-auto max-h-[600px] divide-y divide-foreground/[0.04]" onScroll={handleScroll}>
              {links.length === 0 ? (
                q ? (
                  <div className="flex flex-col items-center gap-3 px-4 py-8 text-center">
                    <p className="text-sm text-foreground/60">
                      {t("links.emptyFilteredReason")} <span className="font-mono text-foreground/80">{`“${q}”`}</span>
                    </p>
                    <Button
                      variant="ghost"
                      className="text-xs py-1.5"
                      onClick={() => setSearchParams(prev => buildLinksFilterQuery({ q: "", archived: false }, prev), { replace: true })}
                    >
                      {t("links.emptyFilteredAction")}
                    </Button>
                  </div>
                ) : (
                  <Empty
                    reason={t("links.emptyNoLinksReason")}
                    detail={linkHostOptions.length > 0 ? (
                      <>{t("links.emptyNoLinksDetailPre")} <span className="font-mono">{`“${linkHostOptions.join(", ")}”`}</span></>
                    ) : (
                      <>{t("links.emptyNoHostDetailPre")} <span className="font-mono">{`“${window.location.origin}”`}</span></>
                    )}
                    action={<Button variant="primary" className="mt-1 text-xs py-1.5" onClick={() => setActive("new")}>{t("links.newLink")}</Button>}
                  >
                    <Link2 className="h-8 w-8 text-foreground/50 mb-1" />
                  </Empty>
                )
              ) : (
                <>
                  {links.map((l) => (
                    <button
                      key={l.id}
                      className={`flex w-full flex-col p-4 text-left hover:bg-foreground/[0.03] transition-colors min-w-0 ${
                        active !== "new" && active?.id === l.id ? "bg-foreground/[0.05]" : ""
                      }`}
                      onClick={() => setActive(l)}
                    >
                      <div className="flex items-center gap-2 w-full justify-between min-w-0">
                        <span className="font-mono font-semibold text-sm text-accent-fg truncate flex-1 min-w-0">
                          /{l.slug}
                        </span>
                        <Badge tone="neutral" className="text-[10px] shrink-0">
                          {t("links.clicksCount", { count: l.clicks })}
                        </Badge>
                      </div>
                      <div className="truncate text-xs text-foreground/50 mt-1.5 font-mono w-full">{l.target}</div>
                    </button>
                  ))}
                  {loading && <div className="p-3 text-center text-xs text-foreground/40">{t("links.loading")}</div>}
                </>
              )}
            </div>
            )}
          </GlassCard>
        </div>

        {/* Right column - detail editor / viewer */}
        <div className="w-full min-w-0">
          {active === "new" ? (
            <GlassCard className="p-5 min-w-0">
              <h2 className="mb-4 text-lg font-bold text-foreground flex items-center gap-2 min-w-0">
                <Link2 className="h-5 w-5 text-accent-fg shrink-0" />
                <span className="truncate">{t("links.createNewLink")}</span>
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
            <GlassCard className="p-5 min-w-0">
              <div className="flex flex-wrap justify-between items-center mb-5 border-b border-foreground/[0.06] pb-4 gap-3 min-w-0">
                <h2 className="font-mono text-lg font-bold text-foreground flex items-center gap-2 min-w-0 truncate">
                  <Link2 className="h-5 w-5 text-accent-fg shrink-0" />
                  <span className="truncate">/{active.slug}</span>
                </h2>
                <div className="flex flex-wrap items-center gap-2 shrink-0">
                  <Button variant="subtle" className="text-xs py-1.5 px-3 gap-1.5" onClick={() => copy(active)}>
                    <Copy className="h-3.5 w-3.5" />
                    {copied === active.id ? t("links.copied") : t("links.copyLink")}
                  </Button>
                  <Button variant="outline" className="text-xs py-1.5 px-3 gap-1.5" onClick={() => toggleArchive(active)}>
                    <Archive className="h-3.5 w-3.5" />
                    {active.archived ? t("links.unarchive") : t("links.archive")}
                  </Button>
                  {canDeleteLink && (
                    <Button
                      variant="danger"
                      onClick={async () => {
                        if (await confirmDialog(t("links.confirmDelete", { slug: active.slug }))) {
                          await linksApi.deleteLink(active.id);
                          setActive(null);
                          loadMore(true);
                        }
                      }}
                      className="text-xs py-1.5 px-3 gap-1.5 border-0"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                      {t("links.delete")}
                    </Button>
                  )}
                </div>
              </div>
              
              <Tabs
                defaultValue="details"
                items={[
                  {
                    value: "details",
                    label: (
                      <span className="flex items-center justify-center gap-1.5">
                        <Settings className="h-3.5 w-3.5" />
                        {t("links.tabDetails")}
                      </span>
                    ),
                    content: (
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
                    ),
                  },
                  {
                    value: "analytics",
                    label: (
                      <span className="flex items-center justify-center gap-1.5">
                        <Eye className="h-3.5 w-3.5" />
                        {t("links.tabAnalytics")}
                      </span>
                    ),
                    content: <StatsView link={active} />,
                  },
                  {
                    value: "qr",
                    label: (
                      <span className="flex items-center justify-center gap-1.5">
                        <QrCode className="h-3.5 w-3.5" />
                        {t("links.tabQr")}
                      </span>
                    ),
                    content: (
                      <div className="flex flex-col items-center gap-4 py-6 min-w-0">
                        <div className="bg-white p-4 rounded-xl shadow-sm border border-foreground/[0.08]">
                          <img
                            src={`/api/links/${active.id}/qr`}
                            alt={t("links.qrAlt")}
                            className="rounded-lg"
                            width={180}
                            height={180}
                          />
                        </div>
                        <Button
                          variant="subtle"
                          className="text-xs py-1.5 px-4 gap-1.5"
                          onClick={() => window.open(`/api/links/${active.id}/qr`)}
                        >
                          <Download className="h-3.5 w-3.5" />
                          {t("links.downloadQrCode")}
                        </Button>
                      </div>
                    ),
                  },
                ]}
              />
            </GlassCard>
          ) : (
            <GlassCard className="flex flex-col items-center justify-center py-10 px-6 text-center text-foreground/40 border border-foreground/[0.04]/40 min-w-0">
              <Link2 className="h-10 w-10 mb-2 opacity-50 text-accent-fg" />
              <p className="text-sm">{t("links.emptyDetail")}</p>
            </GlassCard>
          )}
        </div>
      </div>
    </ScreenWrap>
  );
}

