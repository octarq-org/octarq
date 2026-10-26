import { Alert, Badge, Empty, ScreenWrap, PageHeader, GlassCard, Button, Input, Table, THead, TBody, TR, TH, TableError, confirmDialog, toast } from "@octarq/plugin-sdk";
import { useEffect, useMemo, useState } from "react";
import { useSearchParams, NavLink } from "react-router-dom";
import { api, Domain, effectiveLinkHosts } from "../../../api";
import { linksApi, Link } from "../api";
import { AlertCircle, Link2, Search, Settings, Plus, Activity, ShieldCheck, AlertTriangle, Blocks } from "lucide-react";
import { useTranslation } from "../../../i18n";
import { roleSatisfies, useCurrentRole } from "../../../shell/role";

import { usePluginGate } from "../../PluginGate";
import { parseLinksFilter, buildLinksFilterQuery } from "../filters";
import { ListSkeleton } from "../../../components/ListSkeleton";
import { LinkTableRow } from "./LinkTableRow";
import { LinkModals } from "./LinkModals";

export const SYSTEM_RESERVED_SLUGS = [
  "admin",
  "api",
  "app",
  "assets",
  "auth",
  "dashboard",
  "docs",
  "healthz",
  "help",
  "instance",
  "login",
  "logout",
  "portal",
  "settings",
  "static",
  "status",
  "webhook",
];

export default function LinksPage() {
  const { role, isInstanceAdmin } = useCurrentRole();
  const canDeleteLink = roleSatisfies("admin", role, isInstanceAdmin);
  const [links, setLinks] = useState<Link[]>([]);
  const [domains, setDomains] = useState<Domain[]>([]);
  const [tab, setTab] = useState<"links" | "anomalies" | "reserved">("links");

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
  const [error, setError] = useState<{ detail?: string } | null>(null);
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
    if (reset) setError(null);
    try {
      const limit = 50;
      const offset = reset ? 0 : page * limit;
      const res = await linksApi.links({ q, archived, limit, offset });
      if (res.length < limit) setHasMore(false);
      else setHasMore(true);

      setLinks((prev) => (reset ? res : [...prev, ...res]));
      setPage(reset ? 1 : page + 1);
      setError(null);
    } catch (e: unknown) {
      const err = e as { status?: number; message?: string };
      if (err.status === 404 || err.status === 402) {
        pluginGate.degrade(err.status);
        return;
      }
      // Everything else used to be swallowed: the first page rendered the
      // "no links yet" empty state over a 500, and a failed next page looked
      // like the end of the list.
      setError({ detail: err.status ? `HTTP ${err.status}` : err.message });
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
    try {
      await linksApi.updateLink(l.id, { archived: !l.archived });
      loadMore(true);
    } catch {
      // A rejected mutation used to bubble out of the click handler as an
      // unhandled rejection, leaving the row unchanged with no feedback.
      toast.error(t("links.actionFailed"));
    }
  };

  const handleDelete = async (l: Link) => {
    if (!(await confirmDialog(t("links.confirmDelete", { slug: l.slug })))) return;
    try {
      await linksApi.deleteLink(l.id);
      loadMore(true);
    } catch {
      toast.error(t("links.actionFailed"));
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

      {/* 3 Horizontal Projection Tabs: [Links | Anomaly Monitoring | Reserved Words] */}
      <div className="flex gap-0 border-b border-foreground/[0.06] mb-6 overflow-x-auto [scrollbar-width:none] [-ms-overflow-style:none] [&::-webkit-scrollbar]:hidden" role="tablist">
        <button
          role="tab"
          aria-selected={tab === "links"}
          onClick={() => setTab("links")}
          data-testid="tab-links"
          className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors shrink-0 whitespace-nowrap cursor-pointer ${
            tab === "links"
              ? "border-primary text-foreground"
              : "border-transparent text-foreground/45 hover:text-foreground/70"
          }`}
        >
          {t("links.tabLinks", "Links")}
        </button>
        <button
          role="tab"
          aria-selected={tab === "anomalies"}
          onClick={() => setTab("anomalies")}
          data-testid="tab-anomalies"
          className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors shrink-0 whitespace-nowrap cursor-pointer ${
            tab === "anomalies"
              ? "border-primary text-foreground"
              : "border-transparent text-foreground/45 hover:text-foreground/70"
          }`}
        >
          {t("links.tabAnomalies", "Anomaly Monitoring")}
        </button>
        <button
          role="tab"
          aria-selected={tab === "reserved"}
          onClick={() => setTab("reserved")}
          data-testid="tab-reserved"
          className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors shrink-0 whitespace-nowrap cursor-pointer ${
            tab === "reserved"
              ? "border-primary text-foreground"
              : "border-transparent text-foreground/45 hover:text-foreground/70"
          }`}
        >
          {t("links.tabReserved", "Reserved Words")}
        </button>
      </div>

      {tab === "links" && (
        <div className="space-y-4" data-testid="links-projection-view">
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
          {error && links.length === 0 ? (
            <TableError errorText={error.detail} onRetry={() => loadMore(true)} />
          ) : loading && links.length === 0 ? (
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
            <>
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
            {error && (
              <Alert
                variant="danger"
                icon={<AlertCircle className="h-4 w-4" />}
                actions={
                  <Button variant="outline" size="sm" onClick={() => loadMore()}>
                    {t("proTable.retry")}
                  </Button>
                }
              >
                {t("proTable.errorTitle")}
                {error.detail ? ` (${error.detail})` : ""}
              </Alert>
            )}
            </>
          )}
        </div>
      )}

      {tab === "anomalies" && (
        <div className="space-y-6" data-testid="anomalies-projection-view">
          {/* Overview Stat Cards */}
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
            <GlassCard className="p-4 space-y-1.5">
              <div className="flex items-center justify-between">
                <span className="text-xs text-foreground/60">{t("links.anomalyTraffic", "Traffic Anomalies")}</span>
                <Activity className="h-4 w-4 text-success-fg" />
              </div>
              <div className="text-xl font-semibold text-foreground">0</div>
              <div className="text-[11px] text-success-fg font-medium">{t("links.anomalyHealthySpikes", "Healthy · No spikes")}</div>
            </GlassCard>

            <GlassCard className="p-4 space-y-1.5">
              <div className="flex items-center justify-between">
                <span className="text-xs text-foreground/60">{t("links.anomalyBot", "Bot Filtering")}</span>
                <ShieldCheck className="h-4 w-4 text-primary" />
              </div>
              <div className="text-xl font-semibold text-foreground">{t("links.anomalyActive", "Active")}</div>
              <div className="text-[11px] text-foreground/60">{t("links.anomalyRateLimitActive", "Rate limiting active")}</div>
            </GlassCard>

            <GlassCard className="p-4 space-y-1.5">
              <div className="flex items-center justify-between">
                <span className="text-xs text-foreground/60">{t("links.anomalyLatency", "Redirect Latency")}</span>
                <Activity className="h-4 w-4 text-info-fg" />
              </div>
              <div className="text-xl font-semibold text-foreground">{t("links.anomalyLatencyValue", "< 35ms")}</div>
              <div className="text-[11px] text-success-fg font-medium">{t("links.anomalyP99Latency", "p99 edge redirect")}</div>
            </GlassCard>

            <GlassCard className="p-4 space-y-1.5">
              <div className="flex items-center justify-between">
                <span className="text-xs text-foreground/60">{t("links.anomalyLoops", "Redirect Loops")}</span>
                <AlertTriangle className="h-4 w-4 text-warning-fg" />
              </div>
              <div className="text-xl font-semibold text-foreground">0</div>
              <div className="text-[11px] text-foreground/60">{t("links.anomalyLoopClear", "Loop detector clear")}</div>
            </GlassCard>
          </div>

          {/* Anomaly Real-Time Monitor Card */}
          <GlassCard className="p-6 space-y-4">
            <div className="flex items-center justify-between border-b border-border/40 pb-3">
              <div>
                <h3 className="font-semibold text-sm text-foreground">{t("links.tabAnomalies", "Anomaly Monitoring")}</h3>
                <p className="text-xs text-foreground/60 mt-0.5">{t("links.anomalyDesc", "Real-time monitors track 404 floods, redirect loops, and abnormal traffic spikes.")}</p>
              </div>
              <Badge variant="success" className="text-xs">
                {t("links.anomalyOperational", "Operational")}
              </Badge>
            </div>

            <div className="py-8 text-center space-y-2">
              <ShieldCheck className="h-10 w-10 text-success-fg mx-auto opacity-80" />
              <h4 className="text-sm font-medium text-foreground">{t("links.anomalyClean", "No anomalies detected across short links in the last 24 hours.")}</h4>
              <p className="text-xs text-foreground/50 max-w-md mx-auto">
                {t("links.anomalyDesc", "Real-time monitors track 404 floods, redirect loops, and abnormal traffic spikes.")}
              </p>
            </div>
          </GlassCard>
        </div>
      )}

      {tab === "reserved" && (
        <div className="space-y-6" data-testid="reserved-projection-view">
          {/* Guidance Banner */}
          <div className="flex items-center justify-between p-3 rounded-lg border border-primary/20 bg-primary/5 text-xs text-foreground/80">
            <div className="flex items-center gap-2">
              <Blocks className="h-4 w-4 text-primary shrink-0" />
              <span>{t("links.reservedInstanceNotice", "Configure custom instance-level reserved slugs in Instance Settings.")}</span>
            </div>
            <NavLink
              to="/instance/settings"
              className="text-primary hover:underline font-medium text-xs whitespace-nowrap ml-3"
            >
              {t("links.reservedInstanceAction", "Configure Instance Settings →")}
            </NavLink>
          </div>

          {/* Reserved Words GlassCard */}
          <GlassCard className="p-6 space-y-4">
            <div>
              <h3 className="font-semibold text-sm text-foreground">{t("links.reservedTitle", "System Reserved Words")}</h3>
              <p className="text-xs text-foreground/60 mt-0.5">{t("links.reservedDesc", "Reserved words prevent short links from colliding with system routes and administrative endpoints.")}</p>
            </div>

            <div className="flex flex-wrap gap-2 pt-2">
              {SYSTEM_RESERVED_SLUGS.map((slug) => (
                <span
                  key={slug}
                  className="inline-flex items-center px-2.5 py-1 rounded-md text-xs font-mono bg-foreground/[0.04] border border-border text-foreground/80"
                >
                  /{slug}
                </span>
              ))}
            </div>
          </GlassCard>
        </div>
      )}

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
