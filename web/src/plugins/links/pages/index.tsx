import { Empty, ScreenWrap, PageHeader, GlassCard, Button, Input, Table, THead, TBody, TR, TH, confirmDialog } from "@octarq/plugin-sdk";
import { useEffect, useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { api, Domain, effectiveLinkHosts } from "../../../api";
import { linksApi, Link } from "../api";
import { Link2, Search, Settings, Plus } from "lucide-react";
import { useTranslation } from "../../../i18n";
import { roleSatisfies, useCurrentRole } from "../../../shell/role";

import { usePluginGate } from "../../PluginGate";
import { parseLinksFilter, buildLinksFilterQuery } from "../filters";
import { ListSkeleton } from "../../../components/ListSkeleton";
import { LinkTableRow } from "./LinkTableRow";
import { LinkModals } from "./LinkModals";

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
        setSearchParams(
          (prev) => buildLinksFilterQuery({ q: searchInput, archived }, prev),
          { replace: true },
        );
      }
    }, 250);
    return () => clearTimeout(timer);
  }, [searchInput, archived, q, setSearchParams]);

  const [createOpen, setCreateOpen] = useState(false);
  const [editingLink, setEditingLink] = useState<Link | null>(null);
  const [analyticsLink, setAnalyticsLink] = useState<Link | null>(null);
  const [qrLink, setQrLink] = useState<Link | null>(null);

  const [page, setPage] = useState(0);
  const [hasMore, setHasMore] = useState(true);
  const [loading, setLoading] = useState(false);
  const [copiedId, setCopiedId] = useState<number | null>(null);
  const { t } = useTranslation();
  const pluginGate = usePluginGate();

  useEffect(() => {
    if (searchParams.get("create") === "1") {
      setCreateOpen(true);
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          next.delete("create");
          return next;
        },
        { replace: true },
      );
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

      setLinks((prev) => (reset ? res : [...prev, ...res]));
      setPage(reset ? 1 : page + 1);
    } catch (e: unknown) {
      const err = e as { status?: number };
      if (err.status === 404 || err.status === 402) {
        pluginGate.degrade(err.status);
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
    const bottom =
      e.currentTarget.scrollHeight - e.currentTarget.scrollTop <=
      e.currentTarget.clientHeight + 100;
    if (bottom) loadMore();
  };

  function linkURL(l: Link) {
    return l.host ? `https://${l.host}/${l.slug}` : `${location.origin}/${l.slug}`;
  }

  const copy = (l: Link) => {
    navigator.clipboard.writeText(linkURL(l));
    setCopiedId(l.id);
    setTimeout(() => setCopiedId(null), 2000);
  };

  const toggleArchive = async (l: Link) => {
    await linksApi.updateLink(l.id, { archived: !l.archived });
    loadMore(true);
  };

  const handleDelete = async (l: Link) => {
    if (await confirmDialog(t("links.confirmDelete", { slug: l.slug }))) {
      await linksApi.deleteLink(l.id);
      loadMore(true);
    }
  };

  return (
    <ScreenWrap>
      <PageHeader
        title={t("links.pageTitle")}
        description={t("links.pageDescription")}
        action={
          <Button variant="primary" onClick={() => setCreateOpen(true)} className="gap-1.5 py-1.5 text-xs">
            <Plus className="h-3.5 w-3.5" />
            {t("links.newLink")}
          </Button>
        }
      />

      <div className="space-y-4">
        {/* Search, Filter Tabs & Batch Toolbar */}
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-2 flex-1 max-w-sm">
            <div className="relative w-full">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-foreground/40" />
              <Input
                className="pl-8 text-xs py-1.5"
                placeholder={t("links.searchPlaceholder")}
                value={searchInput}
                onChange={(e) => setSearchInput(e.target.value)}
              />
            </div>
            {searchInput && (
              <Button variant="ghost" className="text-xs py-1.5 px-2" onClick={() => setSearchInput("")}>
                {t("links.emptyFilteredAction")}
              </Button>
            )}
          </div>

          <div className="flex items-center gap-2">
            <div className="flex items-center bg-foreground/[0.04] p-0.5 rounded-lg border border-foreground/[0.06] text-xs">
              <button
                type="button"
                onClick={() => setSearchParams((prev) => buildLinksFilterQuery({ q, archived: false }, prev), { replace: true })}
                className={`px-2.5 py-1 rounded-md transition-colors font-medium cursor-pointer ${
                  !archived ? "bg-card text-foreground shadow-2xs" : "text-foreground/50 hover:text-foreground"
                }`}
              >
                {t("links.active")}
              </button>
              <button
                type="button"
                onClick={() => setSearchParams((prev) => buildLinksFilterQuery({ q, archived: true }, prev), { replace: true })}
                className={`px-2.5 py-1 rounded-md transition-colors font-medium cursor-pointer ${
                  archived ? "bg-card text-foreground shadow-2xs" : "text-foreground/50 hover:text-foreground"
                }`}
              >
                {t("links.archived")}
              </button>
            </div>
          </div>
        </div>

        {/* Links Data Table / Empty Views */}
        {loading && links.length === 0 ? (
          <ListSkeleton rows={8} ariaLabel={t("links.loading")} />
        ) : links.length === 0 ? (
          q ? (
            <GlassCard className="flex flex-col items-center gap-3 px-4 py-12 text-center">
              <p className="text-sm text-foreground/60">
                {t("links.emptyFilteredReason")} <span className="font-mono text-foreground/80">{`“${q}”`}</span>
              </p>
              <Button
                variant="ghost"
                className="text-xs py-1.5"
                onClick={() => setSearchParams((prev) => buildLinksFilterQuery({ q: "", archived: false }, prev), { replace: true })}
              >
                {t("links.emptyFilteredAction")}
              </Button>
            </GlassCard>
          ) : (
            <Empty
              reason={t("links.emptyNoLinksReason")}
              detail={
                linkHostOptions.length > 0 ? (
                  <>
                    {t("links.emptyNoLinksDetailPre")} <span className="font-mono">{`“${linkHostOptions.join(", ")}”`}</span>
                  </>
                ) : (
                  <>
                    {t("links.emptyNoHostDetailPre")} <span className="font-mono">{`“${window.location.origin}”`}</span>
                  </>
                )
              }
              action={
                <Button variant="primary" className="mt-1 text-xs py-1.5" onClick={() => setCreateOpen(true)}>
                  {t("links.newLink")}
                </Button>
              }
            >
              <Link2 className="h-8 w-8 text-foreground/50 mb-1" />
            </Empty>
          )
        ) : (
          <GlassCard className="overflow-hidden border border-foreground/[0.06]">
            <div className="overflow-x-auto max-h-[700px] overflow-y-auto" onScroll={handleScroll}>
              <Table>
                <THead className="border-b border-foreground/[0.06] bg-foreground/[0.02]">
                  <TR>
                    <TH className="min-w-[200px]">{t("links.pageTitle")}</TH>
                    <TH className="min-w-[240px]">{t("links.targetDestination")}</TH>
                    <TH className="w-24 text-center">{t("links.totalClicks")}</TH>
                    <TH className="w-24 text-center">{t("links.active")}</TH>
                    <TH className="text-right w-44" />
                  </TR>
                </THead>
                <TBody className="divide-y divide-foreground/[0.04]">
                  {links.map((l) => (
                    <LinkTableRow
                      key={l.id}
                      link={l}
                      isCopied={copiedId === l.id}
                      canDeleteLink={canDeleteLink}
                      onCopy={copy}
                      onAnalytics={setAnalyticsLink}
                      onQr={setQrLink}
                      onEdit={setEditingLink}
                      onToggleArchive={toggleArchive}
                      onDelete={handleDelete}
                      linkURL={linkURL}
                    />
                  ))}
                </TBody>
              </Table>
              {loading && <div className="p-3 text-center text-xs text-foreground/40">{t("links.loading")}</div>}
            </div>
          </GlassCard>
        )}
      </div>

      <LinkModals
        createOpen={createOpen}
        editingLink={editingLink}
        analyticsLink={analyticsLink}
        qrLink={qrLink}
        linkHostOptions={linkHostOptions}
        copiedId={copiedId}
        onCloseCreate={() => setCreateOpen(false)}
        onCloseEdit={() => setEditingLink(null)}
        onCloseAnalytics={() => setAnalyticsLink(null)}
        onCloseQr={() => setQrLink(null)}
        onRefreshLinks={() => loadMore(true)}
        onCopyQrLink={copy}
        linkURL={linkURL}
      />
    </ScreenWrap>
  );
}
