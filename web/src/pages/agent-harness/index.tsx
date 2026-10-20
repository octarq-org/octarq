import { useCallback, useEffect, useState } from "react";
import {
  Alert,
  Badge,
  Button,
  confirmDialog,
  Empty,
  GlassCard,
  PageHeader,
  ScreenWrap,
  Skeleton,
  Table,
  TBody,
  TD,
  TH,
  THead,
  timeAgo,
  toast,
  TR,
  useTranslation,
} from "@octarq/plugin-sdk";
import {
  api,
  type HarnessActiveVersion,
  type HarnessSpecContent,
  type HarnessSpecDetail,
  type HarnessVersionItem,
} from "../../api";
import { HarnessDetailModal } from "./detail";
import { HarnessDiffModal } from "./diff";
import { HarnessEditorModal } from "./editor";

export default function AgentHarnessPage() {
  const { t } = useTranslation();

  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [activeVersion, setActiveVersion] = useState<HarnessActiveVersion | null>(null);
  const [versions, setVersions] = useState<HarnessVersionItem[]>([]);

  // Modal states
  const [editorOpen, setEditorOpen] = useState(false);
  const [editorInitialSpec, setEditorInitialSpec] = useState<HarnessSpecContent | null>(null);

  const [inspectSpecId, setInspectSpecId] = useState<number | null>(null);
  const [detailCache, setDetailCache] = useState<Record<number, HarnessSpecDetail>>({});

  const [diffOpen, setDiffOpen] = useState(false);
  const [diffTargetDetail, setDiffTargetDetail] = useState<HarnessSpecDetail | null>(null);
  const [activeDetail, setActiveDetail] = useState<HarnessSpecDetail | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await api.agentHarnessSpecs();
      setActiveVersion(res.active ?? null);
      setVersions(res.versions ?? []);

      // If active version exists, eagerly or lazily cache its detail for diff
      if (res.active?.id) {
        api
          .agentHarnessSpec(res.active.id)
          .then((d) => {
            setActiveDetail(d);
            setDetailCache((prev) => ({ ...prev, [d.id]: d }));
          })
          .catch(() => {
            /* ignore background cache failure */
          });
      } else {
        setActiveDetail(null);
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : t("agentHarness.loadError");
      setError(msg);
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    void load();
  }, [load]);

  // Handle Rollback
  const handleRollback = async (versionNum: number, specId: number) => {
    const confirmed = await confirmDialog({
      title: t("agentHarness.rollbackConfirmTitle", { version: versionNum }),
      message: t("agentHarness.rollbackConfirmDesc", { version: versionNum }),
      confirmLabel: t("agentHarness.rollback"),
      cancelLabel: t("agentHarness.cancel"),
    });

    if (!confirmed) return;

    try {
      await api.rollbackAgentHarnessSpec(specId);
      toast.success(t("agentHarness.rollbackSuccess", { version: versionNum }));
      await load();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : t("agentHarness.rollbackFailed");
      toast.error(msg);
    }
  };

  // Open Diff Modal
  const handleOpenDiff = async (item: HarnessVersionItem) => {
    try {
      let target = detailCache[item.id];
      if (!target) {
        target = await api.agentHarnessSpec(item.id);
        setDetailCache((prev) => ({ ...prev, [target.id]: target }));
      }
      setDiffTargetDetail(target);

      // Ensure active detail is loaded if active version exists
      if (activeVersion && (!activeDetail || activeDetail.id !== activeVersion.id)) {
        const act = await api.agentHarnessSpec(activeVersion.id);
        setActiveDetail(act);
        setDetailCache((prev) => ({ ...prev, [act.id]: act }));
      }

      setDiffOpen(true);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : t("agentHarness.loadError");
      toast.error(msg);
    }
  };

  // Open Inspector
  const handleInspect = async (item: HarnessVersionItem) => {
    setInspectSpecId(item.id);
  };

  // Open Editor
  const handleOpenEditor = (initial?: HarnessSpecContent | null) => {
    setEditorInitialSpec(initial ?? null);
    setEditorOpen(true);
  };

  const handleEditorSuccess = (created: HarnessSpecDetail) => {
    setDetailCache((prev) => ({ ...prev, [created.id]: created }));
    setActiveDetail(created);
    void load();
  };

  return (
    <ScreenWrap>
      <PageHeader
        title={t("agentHarness.pageTitle")}
        description={t("agentHarness.pageDesc")}
        action={
          !loading && !error && versions.length > 0 ? (
            <Button
              variant="primary"
              size="sm"
              onClick={() => handleOpenEditor()}
            >
              + {t("agentHarness.publishNew")}
            </Button>
          ) : undefined
        }
      />

      {/* 1. Loading State */}
      {loading && (
        <div data-testid="agent-harness-loading" className="space-y-6 mt-6">
          <GlassCard className="p-5 space-y-3">
            <Skeleton className="h-5 w-40" />
            <Skeleton className="h-4 w-72" />
          </GlassCard>
          <GlassCard className="p-0 overflow-hidden">
            <div className="p-4 border-b border-border/50">
              <Skeleton className="h-4 w-32" />
            </div>
            <div className="p-4 space-y-3">
              <Skeleton className="h-8 w-full" />
              <Skeleton className="h-8 w-full" />
              <Skeleton className="h-8 w-full" />
            </div>
          </GlassCard>
        </div>
      )}

      {/* 2. Error State */}
      {!loading && error && (
        <div data-testid="agent-harness-error" className="mt-6">
          <GlassCard className="p-6 text-center space-y-4 max-w-lg mx-auto">
            <Alert variant="danger" title={t("agentHarness.loadError")}>
              {error}
            </Alert>
            <div>
              <Button variant="outline" size="sm" onClick={() => void load()}>
                {t("agentHarness.retry")}
              </Button>
            </div>
          </GlassCard>
        </div>
      )}

      {/* 3. Empty State */}
      {!loading && !error && versions.length === 0 && (
        <div data-testid="agent-harness-empty" className="mt-8">
          <GlassCard className="p-10 text-center max-w-xl mx-auto space-y-4">
            <Empty
              reason={t("agentHarness.emptyTitle")}
              detail={t("agentHarness.emptyDesc")}
              action={
                <Button
                  variant="primary"
                  onClick={() => handleOpenEditor()}
                >
                  + {t("agentHarness.publishFirst")}
                </Button>
              }
            >
              <div className="text-3xl mb-1">🤖</div>
            </Empty>
          </GlassCard>
        </div>
      )}

      {/* 4. Ready State */}
      {!loading && !error && versions.length > 0 && (
        <div data-testid="agent-harness-ready" className="space-y-6 mt-6">
          {/* Active Version Status Card */}
          <GlassCard className="p-5">
            <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
              <div>
                <div className="flex items-center gap-2">
                  <span className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                    {t("agentHarness.activeSpec")}
                  </span>
                  {activeVersion ? (
                    <Badge variant="green" className="text-xs">
                      #{activeVersion.version}
                    </Badge>
                  ) : (
                    <Badge variant="indigo" className="text-xs">
                      {t("agentHarness.noActiveSpec")}
                    </Badge>
                  )}
                </div>

                {activeVersion ? (
                  <div className="mt-1.5 flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-foreground/80">
                    <span className="font-semibold text-foreground">
                      {activeVersion.name}
                    </span>
                    <span className="font-mono text-xs text-muted-foreground" title={activeVersion.fingerprint}>
                      {activeVersion.fingerprint.slice(0, 16)}…
                    </span>
                    <span className="text-xs text-muted-foreground">
                      {t("agentHarness.createdBy")}: <strong className="font-medium text-foreground">{activeVersion.createdBy || "—"}</strong>
                    </span>
                    <span className="text-xs text-muted-foreground" title={activeVersion.publishedAt}>
                      {timeAgo(activeVersion.publishedAt)}
                    </span>
                  </div>
                ) : (
                  <p className="mt-1 text-xs text-muted-foreground">
                    {t("agentHarness.noActiveSpecDesc")}
                  </p>
                )}
              </div>

              {activeVersion && (
                <div className="flex items-center gap-2 shrink-0">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => handleInspect(activeVersion as HarnessVersionItem)}
                    className="text-xs"
                  >
                    {t("agentHarness.viewDetail")}
                  </Button>
                </div>
              )}
            </div>
          </GlassCard>

          {/* Versions Table */}
          <GlassCard className="overflow-hidden p-0">
            <div className="border-b border-border/60 px-5 py-3 flex items-center justify-between">
              <h3 className="font-bold text-sm text-foreground">
                {t("agentHarness.versionHistory")}
              </h3>
              <span className="text-xs text-muted-foreground">
                {versions.length} {t("agentHarness.versionHistory").toLowerCase()}
              </span>
            </div>

            <Table>
              <THead className="border-b border-border/60">
                <TR>
                  <TH className="w-24">{t("agentHarness.version")}</TH>
                  <TH>{t("agentHarness.name")}</TH>
                  <TH>{t("agentHarness.fingerprint")}</TH>
                  <TH>{t("agentHarness.status")}</TH>
                  <TH>{t("agentHarness.publishedAt")}</TH>
                  <TH>{t("agentHarness.createdBy")}</TH>
                  <TH className="text-right">{t("agentHarness.actions")}</TH>
                </TR>
              </THead>
              <TBody className="divide-y divide-border/40">
                {versions.map((v) => {
                  const isActive = activeVersion?.id === v.id;

                  return (
                    <TR
                      key={v.id}
                      className={isActive ? "bg-primary/5" : undefined}
                    >
                      <TD>
                        <div className="flex items-center gap-1.5 font-mono font-bold text-sm">
                          <span>#{v.version}</span>
                          {isActive && (
                            <Badge variant="indigo" className="text-[10px] py-0 px-1">
                              {t("agentHarness.statusActive")}
                            </Badge>
                          )}
                        </div>
                      </TD>

                      <TD>
                        <span className="font-medium text-foreground">{v.name}</span>
                      </TD>

                      <TD>
                        <span
                          className="font-mono text-xs text-muted-foreground"
                          title={v.fingerprint}
                        >
                          {v.fingerprint.slice(0, 12)}…
                        </span>
                      </TD>

                      <TD>
                        <Badge variant={v.status === "published" ? "green" : "neutral"}>
                          {v.status === "published"
                            ? t("agentHarness.statusPublished")
                            : t("agentHarness.statusArchived")}
                        </Badge>
                      </TD>

                      <TD className="text-xs text-muted-foreground" title={v.publishedAt}>
                        {timeAgo(v.publishedAt)}
                      </TD>

                      <TD className="text-xs text-foreground/80">
                        {v.createdBy || "—"}
                      </TD>

                      <TD className="text-right whitespace-nowrap">
                        <div className="inline-flex items-center gap-1.5">
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => handleInspect(v)}
                            className="text-xs h-7 px-2"
                          >
                            {t("agentHarness.viewDetail")}
                          </Button>

                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => handleOpenDiff(v)}
                            className="text-xs h-7 px-2"
                          >
                            {t("agentHarness.diffWithActive")}
                          </Button>

                          {!isActive && (
                            <Button
                              variant="outline"
                              size="sm"
                              onClick={() => handleRollback(v.version, v.id)}
                              className="text-xs h-7 px-2 text-warning-fg border-warning-fg/30 hover:bg-warning-fg/10"
                            >
                              {t("agentHarness.rollback")}
                            </Button>
                          )}
                        </div>
                      </TD>
                    </TR>
                  );
                })}
              </TBody>
            </Table>
          </GlassCard>
        </div>
      )}

      {/* Modals */}
      <HarnessDetailModal
        open={inspectSpecId !== null}
        onClose={() => setInspectSpecId(null)}
        specId={inspectSpecId}
        cachedDetail={inspectSpecId ? detailCache[inspectSpecId] : null}
        isActive={inspectSpecId !== null && inspectSpecId === activeVersion?.id}
        onUseAsTemplate={(spec) => handleOpenEditor(spec)}
        onRollback={(vNum, id) => handleRollback(vNum, id)}
      />

      <HarnessDiffModal
        open={diffOpen}
        onClose={() => setDiffOpen(false)}
        activeDetail={activeDetail}
        targetDetail={diffTargetDetail}
      />

      <HarnessEditorModal
        open={editorOpen}
        onClose={() => setEditorOpen(false)}
        onSuccess={handleEditorSuccess}
        initialSpec={editorInitialSpec}
      />
    </ScreenWrap>
  );
}
