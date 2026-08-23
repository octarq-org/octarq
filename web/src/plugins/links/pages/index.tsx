import { useEffect, useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { api, Domain, effectiveLinkHosts } from "../../../api";
import { linksApi, Link } from "../api";
import { Empty, ScreenWrap, PageHeader, GlassCard, Badge, Button, Input, Modal, Table, THead, TBody, TR, TH, TD, confirmDialog } from "../../../ui";
import { Link2, Copy, Check, Archive, Trash2, QrCode, Download, Eye, Search, Settings, ExternalLink, Lock, Bot, Clock, Tag, Plus, BarChart3, Edit3 } from "lucide-react";
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
    setCopiedId(l.id);
    setTimeout(() => setCopiedId(null), 1200);
  }

  async function toggleArchive(l: Link) {
    await linksApi.updateLink(l.id, { archived: !l.archived } as any);
    loadMore(true);
    if (editingLink?.id === l.id) {
      setEditingLink({ ...editingLink, archived: !l.archived });
    }
  }

  const totalClicksSum = useMemo(() => links.reduce((sum, l) => sum + (l.clicks || 0), 0), [links]);

  return (
    <ScreenWrap>
      <PageHeader
        title={t("links.pageTitle")}
        description={t("links.pageDescription")}
        action={
          <div className="flex items-center gap-2">
            <Button variant="primary" onClick={() => setCreateOpen(true)} className="gap-1.5 py-1.5 text-xs">
              <Plus className="h-3.5 w-3.5" />
              {t("links.newLink")}
            </Button>
          </div>
        }
      />

      {/* Primary Link Hub (Full Width Workspace) */}
      <div className="space-y-4">
        {/* Toolbar: Search, Archive filter, Quick stats */}
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-2 flex-1 max-w-lg min-w-[240px]">
            <div className="relative flex-1 min-w-0">
              <Input
                className="w-full !pl-8 text-sm min-w-0"
                placeholder={t("links.searchPlaceholder")}
                value={searchInput}
                onChange={(e) => setSearchInput(e.target.value)}
              />
              <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-foreground/50 pointer-events-none" />
            </div>
            {searchInput && (
              <Button
                variant="ghost"
                className="text-xs py-1.5 px-2 shrink-0"
                onClick={() => setSearchParams(prev => buildLinksFilterQuery({ q: "", archived }, prev), { replace: true })}
              >
                {t("links.emptyFilteredAction")}
              </Button>
            )}
          </div>

          <div className="flex items-center gap-2 shrink-0">
            {/* Active vs Archived segmented toggle */}
            <div className="inline-flex rounded-xl bg-well p-0.5 border border-foreground/[0.08]">
              <button
                type="button"
                onClick={() => {
                  if (archived) setSearchParams(prev => buildLinksFilterQuery({ q: searchInput, archived: false }, prev), { replace: true });
                }}
                className={`px-3 py-1 text-xs font-medium rounded-lg transition-colors cursor-pointer ${
                  !archived
                    ? "bg-accent text-accent-fg shadow-sm font-semibold"
                    : "text-foreground/60 hover:text-foreground"
                }`}
              >
                {t("links.active")}
              </button>
              <button
                type="button"
                onClick={() => {
                  if (!archived) setSearchParams(prev => buildLinksFilterQuery({ q: searchInput, archived: true }, prev), { replace: true });
                }}
                className={`px-3 py-1 text-xs font-medium rounded-lg transition-colors cursor-pointer ${
                  archived
                    ? "bg-accent text-accent-fg shadow-sm font-semibold"
                    : "text-foreground/60 hover:text-foreground"
                }`}
              >
                {t("links.archived")}
              </button>
            </div>

            <span className="hidden sm:inline-flex items-center gap-1.5 text-xs text-foreground/60 bg-well px-2.5 py-1.5 rounded-xl border border-foreground/[0.06] font-mono">
              <Eye className="h-3 w-3 text-accent-fg" />
              <span>{t("links.clicksCount", { count: totalClicksSum })}</span>
            </span>
          </div>
        </div>

        {/* Links Content */}
        {links.length === 0 && loading ? (
          <ListSkeleton rows={7} ariaLabel={t("links.loading")} />
        ) : links.length === 0 ? (
          q ? (
            <GlassCard className="flex flex-col items-center gap-3 px-4 py-12 text-center">
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
            </GlassCard>
          ) : (
            <Empty
              reason={t("links.emptyNoLinksReason")}
              detail={linkHostOptions.length > 0 ? (
                <>{t("links.emptyNoLinksDetailPre")} <span className="font-mono">{`“${linkHostOptions.join(", ")}”`}</span></>
              ) : (
                <>{t("links.emptyNoHostDetailPre")} <span className="font-mono">{`“${window.location.origin}”`}</span></>
              )}
              action={<Button variant="primary" className="mt-1 text-xs py-1.5" onClick={() => setCreateOpen(true)}>{t("links.newLink")}</Button>}
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
                    <TH className="min-w-[200px]">{t("links.shortSlug")}</TH>
                    <TH className="min-w-[240px]">{t("links.targetDestination")}</TH>
                    <TH className="w-24 text-center">{t("links.totalClicks")}</TH>
                    <TH className="w-24 text-center">{t("links.active")}</TH>
                    <TH className="text-right w-44" />
                  </TR>
                </THead>
                <TBody className="divide-y divide-foreground/[0.04]">
                  {links.map((l) => (
                    <TR key={l.id} className="hover:bg-foreground/[0.02] transition-colors group">
                      {/* Short Slug & Host */}
                      <TD>
                        <div className="flex flex-col gap-1">
                          <div className="flex items-center gap-2">
                            <span className="font-mono font-bold text-sm text-accent-fg truncate max-w-[220px]">
                              {l.host ? `${l.host}/` : "/"}
                              <span className="text-foreground">{l.slug}</span>
                            </span>
                            <button
                              type="button"
                              onClick={() => copy(l)}
                              title={t("links.copyLink")}
                              className="p-1 text-foreground/40 hover:text-foreground rounded transition-colors cursor-pointer"
                            >
                              {copiedId === l.id ? (
                                <Check className="h-3.5 w-3.5 text-success-fg" />
                              ) : (
                                <Copy className="h-3.5 w-3.5" />
                              )}
                            </button>
                            <a
                              href={linkURL(l)}
                              target="_blank"
                              rel="noreferrer"
                              title={t("links.openLink")}
                              className="p-1 text-foreground/40 hover:text-accent-fg rounded transition-colors"
                            >
                              <ExternalLink className="h-3.5 w-3.5" />
                            </a>
                          </div>
                          {l.title && <div className="text-xs text-foreground/60 truncate max-w-[280px]">{l.title}</div>}
                        </div>
                      </TD>

                      {/* Destination Target URL & Tags */}
                      <TD>
                        <div className="flex flex-col gap-1">
                          <div className="font-mono text-xs text-foreground/75 truncate max-w-[320px]" title={l.target}>
                            {l.target}
                          </div>
                          <div className="flex items-center gap-1.5 flex-wrap">
                            {l.hasPassword && (
                              <span className="inline-flex items-center gap-0.5 text-[10px] text-warning-fg bg-warning-bg px-1.5 py-0.5 rounded" title={t("links.accessProtectionPassword")}>
                                <Lock className="h-2.5 w-2.5" />
                              </span>
                            )}
                            {l.routingRules?.length ? (
                              <span className="inline-flex items-center gap-0.5 text-[10px] text-accent-fg bg-accent-soft px-1.5 py-0.5 rounded" title={t("links.routingRules")}>
                                <Bot className="h-2.5 w-2.5" />
                                {l.routingRules.length}
                              </span>
                            ) : null}
                            {l.expiresAt && (
                              <span className="inline-flex items-center gap-0.5 text-[10px] text-foreground/50 bg-foreground/[0.05] px-1.5 py-0.5 rounded" title={new Date(l.expiresAt).toLocaleString()}>
                                <Clock className="h-2.5 w-2.5" />
                              </span>
                            )}
                            {l.tags && (
                              <span className="inline-flex items-center gap-0.5 text-[10px] text-foreground/50 bg-foreground/[0.05] px-1.5 py-0.5 rounded font-mono truncate max-w-[140px]">
                                <Tag className="h-2.5 w-2.5" />
                                {l.tags}
                              </span>
                            )}
                          </div>
                        </div>
                      </TD>

                      {/* Clicks */}
                      <TD className="text-center font-mono">
                        <Badge tone="neutral" className="text-xs font-semibold">
                          {l.clicks || 0}
                        </Badge>
                      </TD>

                      {/* Status */}
                      <TD className="text-center">
                        <Badge tone={l.archived ? "neutral" : "info"} className="text-[10px]">
                          {l.archived ? t("links.archived") : t("links.active")}
                        </Badge>
                      </TD>

                      {/* Actions */}
                      <TD className="text-right">
                        <div className="flex items-center justify-end gap-1.5">
                          <Button
                            variant="ghost"
                            className="p-1.5 text-xs text-foreground/60 hover:text-accent-fg"
                            title={t("links.tabAnalytics")}
                            onClick={() => setAnalyticsLink(l)}
                          >
                            <BarChart3 className="h-4 w-4" />
                          </Button>
                          <Button
                            variant="ghost"
                            className="p-1.5 text-xs text-foreground/60 hover:text-foreground"
                            title={t("links.tabQr")}
                            onClick={() => setQrLink(l)}
                          >
                            <QrCode className="h-4 w-4" />
                          </Button>
                          <Button
                            variant="ghost"
                            className="p-1.5 text-xs text-foreground/60 hover:text-foreground"
                            title={t("links.editLink")}
                            onClick={() => setEditingLink(l)}
                          >
                            <Edit3 className="h-4 w-4" />
                          </Button>
                          <Button
                            variant="ghost"
                            className="p-1.5 text-xs text-foreground/50 hover:text-foreground"
                            title={l.archived ? t("links.unarchive") : t("links.archive")}
                            onClick={() => toggleArchive(l)}
                          >
                            <Archive className="h-4 w-4" />
                          </Button>
                          {canDeleteLink && (
                            <Button
                              variant="ghost"
                              className="p-1.5 text-xs text-danger-fg/70 hover:text-danger-fg"
                              title={t("links.delete")}
                              onClick={async () => {
                                if (await confirmDialog(t("links.confirmDelete", { slug: l.slug }))) {
                                  await linksApi.deleteLink(l.id);
                                  loadMore(true);
                                }
                              }}
                            >
                              <Trash2 className="h-4 w-4" />
                            </Button>
                          )}
                        </div>
                      </TD>
                    </TR>
                  ))}
                </TBody>
              </Table>
              {loading && <div className="p-3 text-center text-xs text-foreground/40">{t("links.loading")}</div>}
            </div>
          </GlassCard>
        )}
      </div>

      {/* Create New Link Modal */}
      {createOpen && (
        <Modal title={t("links.createNewLink")} onClose={() => setCreateOpen(false)}>
          <div className="p-1">
            <LinkEditorForm
              link={null}
              hosts={linkHostOptions}
              onCancel={() => setCreateOpen(false)}
              onSaved={() => {
                setCreateOpen(false);
                loadMore(true);
              }}
            />
          </div>
        </Modal>
      )}

      {/* Edit Link Modal */}
      {editingLink && (
        <Modal title={`${t("links.editLink")} /${editingLink.slug}`} onClose={() => setEditingLink(null)}>
          <div className="p-1">
            <LinkEditorForm
              key={editingLink.id}
              link={editingLink}
              hosts={linkHostOptions}
              onCancel={() => setEditingLink(null)}
              onSaved={(l) => {
                setEditingLink(null);
                loadMore(true);
              }}
            />
          </div>
        </Modal>
      )}

      {/* Analytics Modal */}
      {analyticsLink && (
        <Modal title={`${t("links.clickPerformanceAnalytics")} — /${analyticsLink.slug}`} onClose={() => setAnalyticsLink(null)}>
          <div className="p-2 max-h-[80vh] overflow-y-auto">
            <StatsView link={analyticsLink} />
          </div>
        </Modal>
      )}

      {/* QR Code Modal */}
      {qrLink && (
        <Modal title={`${t("links.linkQrCode")} — /${qrLink.slug}`} onClose={() => setQrLink(null)}>
          <div className="flex flex-col items-center gap-5 py-4">
            <div className="bg-white p-5 rounded-2xl shadow-sm border border-foreground/[0.08]">
              <img
                src={`/api/links/${qrLink.id}/qr`}
                alt={t("links.qrAlt")}
                className="rounded-lg"
                width={220}
                height={220}
              />
            </div>
            <div className="font-mono text-xs text-foreground/70 bg-well px-3 py-1.5 rounded-lg border border-foreground/[0.05] truncate max-w-sm">
              {linkURL(qrLink)}
            </div>
            <div className="flex items-center gap-2">
              <Button
                variant="subtle"
                className="text-xs py-1.5 px-4 gap-1.5"
                onClick={() => copy(qrLink)}
              >
                {copiedId === qrLink.id ? <Check className="h-3.5 w-3.5 text-success-fg" /> : <Copy className="h-3.5 w-3.5" />}
                {copiedId === qrLink.id ? t("links.copied") : t("links.copyLink")}
              </Button>
              <Button
                variant="primary"
                className="text-xs py-1.5 px-4 gap-1.5"
                onClick={() => window.open(`/api/links/${qrLink.id}/qr`)}
              >
                <Download className="h-3.5 w-3.5" />
                {t("links.downloadQrCode")}
              </Button>
            </div>
          </div>
        </Modal>
      )}
    </ScreenWrap>
  );
}
