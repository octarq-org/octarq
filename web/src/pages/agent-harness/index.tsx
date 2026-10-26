import { useCallback, useEffect, useState } from "react";
import { useSearchParams, useInRouterContext } from "react-router-dom";
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
  ApiError,
  type HarnessLesson,
  type HarnessSpecContent,
  type HarnessSpecDetail,
  type HarnessVersionItem,
} from "../../api";
import { HarnessDetailModal } from "./detail";
import { HarnessDiffModal } from "./diff";
import { HarnessEditorModal } from "./editor";
import { HarnessProposalModal } from "./proposal";
import { HarnessLessonModal } from "./lesson-modal";
import { EvidenceViewer } from "./evidence";
import { Bot, BookOpen, Lock, Plus, UploadCloud, Terminal, ShieldAlert, Coins, Sliders } from "lucide-react";
import { FleetView } from "./FleetView";
import { TasksView } from "./TasksView";
import { BoardApprovalsView } from "./BoardApprovalsView";
import { BudgetsView } from "./BudgetsView";

export type AgentHarnessTab = "fleet" | "tasks" | "approvals" | "budgets" | "specs" | "lessons";

const VALID_TABS: readonly AgentHarnessTab[] = [
  "fleet",
  "tasks",
  "approvals",
  "budgets",
  "specs",
  "lessons",
] as const;

export default function AgentHarnessPage() {
  const inRouter = useInRouterContext();
  if (inRouter) {
    return <AgentHarnessWithRouter />;
  }
  return <AgentHarnessContent />;
}

function AgentHarnessWithRouter() {
  const [searchParams, setSearchParams] = useSearchParams();
  const tabParam = searchParams.get("tab") as AgentHarnessTab;
  const activeTab: AgentHarnessTab = VALID_TABS.includes(tabParam) ? tabParam : "fleet";

  const setActiveTab = useCallback(
    (tab: AgentHarnessTab) => {
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          next.set("tab", tab);
          return next;
        },
        { replace: true }
      );
    },
    [setSearchParams]
  );

  return <AgentHarnessContent activeTab={activeTab} setActiveTab={setActiveTab} />;
}

interface AgentHarnessContentProps {
  activeTab?: AgentHarnessTab;
  setActiveTab?: (tab: AgentHarnessTab) => void;
}

function AgentHarnessContent({
  activeTab: controlledTab,
  setActiveTab: controlledSetActiveTab,
}: AgentHarnessContentProps = {}) {
  const { t } = useTranslation();

  const [uncontrolledTab, setUncontrolledTab] = useState<AgentHarnessTab>("fleet");
  const activeTab = controlledTab ?? uncontrolledTab;
  const setActiveTab = controlledSetActiveTab ?? setUncontrolledTab;

  // Specs state
  const [specsLoading, setSpecsLoading] = useState(true);
  const [specsError, setSpecsError] = useState<string | null>(null);
  // Set when the server refuses the page because the caller is not a
  // workspace admin. It is a state of its own, not a failed load.
  const [forbidden, setForbidden] = useState(false);
  const [activeVersion, setActiveVersion] = useState<HarnessActiveVersion | null>(null);
  const [versions, setVersions] = useState<HarnessVersionItem[]>([]);

  // Lessons state
  const [lessonsLoading, setLessonsLoading] = useState(false);
  const [lessonsError, setLessonsError] = useState<string | null>(null);
  const [lessons, setLessons] = useState<HarnessLesson[]>([]);
  const [lessonsLoadedOnce, setLessonsLoadedOnce] = useState(false);

  // Modals state
  const [editorOpen, setEditorOpen] = useState(false);
  const [editorInitialSpec, setEditorInitialSpec] = useState<HarnessSpecContent | null>(null);

  const [proposalOpen, setProposalOpen] = useState(false);
  const [lessonModalOpen, setLessonModalOpen] = useState(false);

  const [inspectSpecId, setInspectSpecId] = useState<number | null>(null);
  const [detailCache, setDetailCache] = useState<Record<number, HarnessSpecDetail>>({});

  const [diffOpen, setDiffOpen] = useState(false);
  const [diffTargetDetail, setDiffTargetDetail] = useState<HarnessSpecDetail | null>(null);
  const [activeDetail, setActiveDetail] = useState<HarnessSpecDetail | null>(null);

  // Load Specs
  const loadSpecs = useCallback(async () => {
    setSpecsLoading(true);
    setSpecsError(null);
    try {
      const res = await api.agentHarnessSpecs();
      setActiveVersion(res.active ?? null);
      setVersions(res.versions ?? []);

      // If active version exists, cache its detail for diff
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
      // A member who opens the page is not an error: the harness is a
      // workspace-admin page, and the page says so instead of showing a
      // failed load.
      if (err instanceof ApiError && err.status === 403) {
        setForbidden(true);
        setSpecsError(null);
        return;
      }
      const msg = err instanceof Error ? err.message : t("agentHarness.loadError");
      setSpecsError(msg);
    } finally {
      setSpecsLoading(false);
    }
  }, [t]);

  // Load Lessons
  const loadLessons = useCallback(async () => {
    setLessonsLoading(true);
    setLessonsError(null);
    try {
      const res = await api.agentHarnessLessons();
      setLessons(res.lessons ?? []);
      setLessonsLoadedOnce(true);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : t("agentHarness.loadLessonsError");
      setLessonsError(msg);
    } finally {
      setLessonsLoading(false);
    }
  }, [t]);

  useEffect(() => {
    void loadSpecs();
  }, [loadSpecs]);

  useEffect(() => {
    if (activeTab === "lessons" && !lessonsLoadedOnce) {
      void loadLessons();
    }
  }, [activeTab, lessonsLoadedOnce, loadLessons]);

  // Handle Activate
  const handleActivate = async (versionNum: number, specId: number) => {
    const confirmed = await confirmDialog({
      title: t("agentHarness.activateConfirmTitle", { version: versionNum }),
      message: t("agentHarness.activateConfirmDesc", { version: versionNum }),
      confirmLabel: t("agentHarness.activate"),
      cancelLabel: t("agentHarness.cancel"),
      danger: false,
    });

    if (!confirmed) return;

    try {
      await api.activateAgentHarnessSpec(specId);
      toast.success(t("agentHarness.activateSuccess", { version: versionNum }));
      await loadSpecs();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : t("agentHarness.activateFailed");
      toast.error(msg);
    }
  };

  // Handle Discard
  const handleDiscard = async (versionNum: number, specId: number) => {
    const confirmed = await confirmDialog({
      title: t("agentHarness.discardConfirmTitle", { version: versionNum }),
      message: t("agentHarness.discardConfirmDesc", { version: versionNum }),
      confirmLabel: t("agentHarness.discard"),
      cancelLabel: t("agentHarness.cancel"),
      danger: true,
    });

    if (!confirmed) return;

    try {
      await api.discardAgentHarnessSpec(specId);
      toast.success(t("agentHarness.discardSuccess", { version: versionNum }));
      await loadSpecs();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : t("agentHarness.discardFailed");
      toast.error(msg);
    }
  };

  // Handle Dismiss Lesson
  const handleDismissLesson = async (lessonId: number) => {
    const confirmed = await confirmDialog({
      title: t("agentHarness.dismissConfirmTitle"),
      message: t("agentHarness.dismissConfirmDesc"),
      confirmLabel: t("agentHarness.dismissLesson"),
      cancelLabel: t("agentHarness.cancel"),
      danger: true,
    });

    if (!confirmed) return;

    try {
      await api.dismissAgentHarnessLesson(lessonId);
      toast.success(t("agentHarness.dismissSuccess"));
      await loadLessons();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : t("agentHarness.dismissFailed");
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
    void loadSpecs();
  };

  const handleProposalSuccess = (created: HarnessSpecDetail) => {
    setDetailCache((prev) => ({ ...prev, [created.id]: created }));
    void loadSpecs();
  };

  const handleLessonSuccess = (created: HarnessLesson) => {
    setLessons((prev) => [created, ...prev]);
    void loadLessons();
  };

  const hasDrafts = versions.some((v) => v.status === "draft");

  return (
    <ScreenWrap>
      <PageHeader
        title={t("agentHarness.pageTitle")}
        description={t("agentHarness.pageDesc")}
        action={
          activeTab === "specs" ? (
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={() => setProposalOpen(true)}
              >
                <UploadCloud className="h-3.5 w-3.5 mr-1" />
                {t("agentHarness.importProposal")}
              </Button>
              <Button
                variant="primary"
                size="sm"
                onClick={() => handleOpenEditor()}
              >
                <Plus className="h-3.5 w-3.5 mr-1" />
                {t("agentHarness.stageNew")}
              </Button>
            </div>
          ) : (
            <Button
              variant="primary"
              size="sm"
              onClick={() => setLessonModalOpen(true)}
            >
              <Plus className="h-3.5 w-3.5 mr-1" />
              {t("agentHarness.addLesson")}
            </Button>
          )
        }
      />

      {/* Executive Summary Strip */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 my-4">
        <GlassCard className="p-3.5 space-y-1">
          <div className="text-[11px] font-medium text-muted-foreground flex items-center gap-1.5">
            <Bot className="h-3.5 w-3.5 text-primary" />
            {t("agentHarness.statActiveAgents", "Active AI Employees")}
          </div>
          <div className="text-xl font-bold font-mono text-foreground">4</div>
        </GlassCard>
        <GlassCard className="p-3.5 space-y-1">
          <div className="text-[11px] font-medium text-muted-foreground flex items-center gap-1.5">
            <Terminal className="h-3.5 w-3.5 text-primary" />
            {t("agentHarness.statRunningTasks", "Running Tasks")}
          </div>
          <div className="text-xl font-bold font-mono text-primary">1 <span className="text-xs text-muted-foreground font-normal">{t("agentHarness.tasksQueuedCount", "({{count}} queued)", { count: 3 })}</span></div>
        </GlassCard>
        <GlassCard className="p-3.5 space-y-1">
          <div className="text-[11px] font-medium text-muted-foreground flex items-center gap-1.5">
            <ShieldAlert className="h-3.5 w-3.5 text-warning-fg" />
            {t("agentHarness.statPendingApprovals", "Awaiting Board Review")}
          </div>
          <div className="text-xl font-bold font-mono text-warning-fg">1</div>
        </GlassCard>
        <GlassCard className="p-3.5 space-y-1">
          <div className="text-[11px] font-medium text-muted-foreground flex items-center gap-1.5">
            <Coins className="h-3.5 w-3.5 text-primary" />
            {t("agentHarness.statBudgetUsage", "Monthly Budget Used")}
          </div>
          <div className="text-xl font-bold font-mono text-foreground">$14.20 <span className="text-xs text-muted-foreground font-normal">/ $50</span></div>
        </GlassCard>
      </div>

      {/* Tabs */}
      <div className="flex gap-2 border-b border-border/60 mt-4 mb-6 overflow-x-auto">
        <button
          type="button"
          onClick={() => setActiveTab("fleet")}
          className={`px-4 py-2.5 text-sm font-medium border-b-2 -mb-px transition-colors shrink-0 whitespace-nowrap cursor-pointer flex items-center gap-2 ${
            activeTab === "fleet"
              ? "border-primary text-foreground font-semibold"
              : "border-transparent text-muted-foreground hover:text-foreground"
          }`}
        >
          <Bot className="h-4 w-4" />
          <span>{t("agentHarness.tabFleet", "AI Employees")}</span>
          <Badge variant="neutral" className="text-[10px] uppercase font-mono tracking-wider py-0 px-1.5 ml-1">
            PREVIEW
          </Badge>
        </button>
        <button
          type="button"
          onClick={() => setActiveTab("tasks")}
          className={`px-4 py-2.5 text-sm font-medium border-b-2 -mb-px transition-colors shrink-0 whitespace-nowrap cursor-pointer flex items-center gap-2 ${
            activeTab === "tasks"
              ? "border-primary text-foreground font-semibold"
              : "border-transparent text-muted-foreground hover:text-foreground"
          }`}
        >
          <Terminal className="h-4 w-4" />
          <span>{t("agentHarness.tabTasks", "Tasks & Issues")}</span>
          <Badge variant="neutral" className="text-[10px] uppercase font-mono tracking-wider py-0 px-1.5 ml-1">
            PREVIEW
          </Badge>
        </button>
        <button
          type="button"
          onClick={() => setActiveTab("approvals")}
          className={`px-4 py-2.5 text-sm font-medium border-b-2 -mb-px transition-colors shrink-0 whitespace-nowrap cursor-pointer flex items-center gap-2 ${
            activeTab === "approvals"
              ? "border-primary text-foreground font-semibold"
              : "border-transparent text-muted-foreground hover:text-foreground"
          }`}
        >
          <ShieldAlert className="h-4 w-4" />
          <span>{t("agentHarness.tabApprovals", "Board Approvals")}</span>
        </button>
        <button
          type="button"
          onClick={() => setActiveTab("budgets")}
          className={`px-4 py-2.5 text-sm font-medium border-b-2 -mb-px transition-colors shrink-0 whitespace-nowrap cursor-pointer flex items-center gap-2 ${
            activeTab === "budgets"
              ? "border-primary text-foreground font-semibold"
              : "border-transparent text-muted-foreground hover:text-foreground"
          }`}
        >
          <Coins className="h-4 w-4" />
          <span>{t("agentHarness.tabBudgets", "Budgets & Governance")}</span>
          <Badge variant="neutral" className="text-[10px] uppercase font-mono tracking-wider py-0 px-1.5 ml-1">
            PREVIEW
          </Badge>
        </button>
        <button
          type="button"
          onClick={() => setActiveTab("specs")}
          className={`px-4 py-2.5 text-sm font-medium border-b-2 -mb-px transition-colors shrink-0 whitespace-nowrap cursor-pointer flex items-center gap-2 ${
            activeTab === "specs"
              ? "border-primary text-foreground font-semibold"
              : "border-transparent text-muted-foreground hover:text-foreground"
          }`}
        >
          <Sliders className="h-4 w-4" />
          <span>{t("agentHarness.tabSpecs")}</span>
          {versions.length > 0 && (
            <Badge variant="neutral" className="text-[10px] py-0 px-1.5 ml-1">
              {versions.length}
            </Badge>
          )}
        </button>
        <button
          type="button"
          onClick={() => setActiveTab("lessons")}
          className={`px-4 py-2.5 text-sm font-medium border-b-2 -mb-px transition-colors shrink-0 whitespace-nowrap cursor-pointer flex items-center gap-2 ${
            activeTab === "lessons"
              ? "border-primary text-foreground font-semibold"
              : "border-transparent text-muted-foreground hover:text-foreground"
          }`}
        >
          <BookOpen className="h-4 w-4" />
          <span>{t("agentHarness.tabLessons")}</span>
          {lessons.length > 0 && (
            <Badge variant="neutral" className="text-[10px] py-0 px-1.5 ml-1">
              {lessons.length}
            </Badge>
          )}
        </button>
      </div>

      {/* Prototype Status Callout */}
      {(activeTab === "fleet" || activeTab === "tasks" || activeTab === "budgets") && (
        <div data-testid="workforce-prototype-callout" className="flex items-center justify-between rounded-xl border border-border/70 bg-well/40 p-3 text-xs mb-4">
          <div className="flex items-center gap-2 text-muted-foreground">
            <Badge variant="neutral" className="text-[10px] uppercase font-mono tracking-wider">
              PROTOTYPE
            </Badge>
            <span>{t("agentHarness.fleetDesc")}</span>
          </div>
        </div>
      )}

      {activeTab === "fleet" && (
        <FleetView
          onOpenEditor={() => {
            setActiveTab("specs");
            handleOpenEditor();
          }}
        />
      )}

      {activeTab === "tasks" && <TasksView />}

      {activeTab === "approvals" && <BoardApprovalsView />}

      {activeTab === "budgets" && <BudgetsView />}

      {/* TAB 1: Specs and Versions */}
      {activeTab === "specs" && (
        <>
          {/* 1. Loading State */}
          {specsLoading && (
            <div data-testid="agent-harness-loading" className="space-y-6">
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

          {/* 2. Forbidden State */}
          {!specsLoading && forbidden && (
            <div data-testid="agent-harness-forbidden">
              <GlassCard className="p-10 text-center max-w-xl mx-auto space-y-4">
                <Empty
                  reason={t("agentHarness.forbiddenTitle")}
                  detail={t("agentHarness.forbiddenDesc")}
                >
                  <Lock className="h-6 w-6" />
                </Empty>
              </GlassCard>
            </div>
          )}

          {/* 3. Error State */}
          {!specsLoading && !forbidden && specsError && (
            <div data-testid="agent-harness-error">
              <GlassCard className="p-6 text-center space-y-4 max-w-lg mx-auto">
                <Alert variant="danger" title={t("agentHarness.loadError")}>
                  {specsError}
                </Alert>
                <div>
                  <Button variant="outline" size="sm" onClick={() => void loadSpecs()}>
                    {t("agentHarness.retry")}
                  </Button>
                </div>
              </GlassCard>
            </div>
          )}

          {/* 3. Empty State */}
          {!specsLoading && !specsError && versions.length === 0 && (
            <div data-testid="agent-harness-empty">
              <GlassCard className="p-10 text-center max-w-xl mx-auto space-y-4">
                <Empty
                  reason={t("agentHarness.emptyTitle")}
                  detail={t("agentHarness.emptyDesc")}
                  action={
                    <div className="flex items-center justify-center gap-3">
                      <Button
                        variant="primary"
                        onClick={() => handleOpenEditor()}
                      >
                        <Plus className="h-4 w-4 mr-1" />
                        {t("agentHarness.stageFirst")}
                      </Button>
                      <Button
                        variant="outline"
                        onClick={() => setProposalOpen(true)}
                      >
                        <UploadCloud className="h-4 w-4 mr-1" />
                        {t("agentHarness.importProposal")}
                      </Button>
                    </div>
                  }
                >
                  <div className="mb-1 flex justify-center">
                    <Bot className="h-8 w-8 text-muted-foreground" />
                  </div>
                </Empty>
              </GlassCard>
            </div>
          )}

          {/* 4. Ready State */}
          {!specsLoading && !specsError && versions.length > 0 && (
            <div data-testid="agent-harness-ready" className="space-y-6">
              {/* Draft Notice Banner if drafts exist */}
              {hasDrafts && (
                <div className="flex items-start gap-3 p-4 rounded-xl border border-warning-border bg-warning-bg text-warning-fg text-xs">
                  <div className="font-semibold">{t("agentHarness.draftNotice")}:</div>
                  <div className="flex-1">{t("agentHarness.draftNoticeDesc")}</div>
                </div>
              )}

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
                        <span className="text-xs text-muted-foreground" title={activeVersion.publishedAt ?? undefined}>
                          {activeVersion.publishedAt ? timeAgo(activeVersion.publishedAt) : t("agentHarness.notPublished")}
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
                    {t("agentHarness.versionCount", { count: versions.length })}
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
                      const isDraft = v.status === "draft";
                      const isDiscarded = v.status === "discarded";
                      const isArchived = v.status === "archived";

                      const getStatusBadge = () => {
                        switch (v.status) {
                          case "published":
                            return <Badge variant="green">{t("agentHarness.statusPublished")}</Badge>;
                          case "draft":
                            return <Badge variant="amber">{t("agentHarness.statusDraft")}</Badge>;
                          case "discarded":
                            return <Badge variant="red">{t("agentHarness.statusDiscarded")}</Badge>;
                          case "archived":
                          default:
                            return <Badge variant="neutral">{t("agentHarness.statusArchived")}</Badge>;
                        }
                      };

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
                            <div className="flex items-center gap-2">
                              <span className="font-medium text-foreground">{v.name}</span>
                              {v.evidence !== null && v.evidence !== undefined && (
                                <Badge variant="neutral" className="text-[10px] py-0 px-1">
                                  {t("agentHarness.evidence")}
                                </Badge>
                              )}
                            </div>
                          </TD>

                          <TD>
                            <span
                              className="font-mono text-xs text-muted-foreground"
                              title={v.fingerprint}
                            >
                              {v.fingerprint.slice(0, 12)}…
                            </span>
                          </TD>

                          <TD>{getStatusBadge()}</TD>

                          <TD className="text-xs text-muted-foreground" title={v.publishedAt ?? undefined}>
                            {v.publishedAt ? timeAgo(v.publishedAt) : "—"}
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

                              {isDraft && (
                                <>
                                  <Button
                                    variant="outline"
                                    size="sm"
                                    onClick={() => handleActivate(v.version, v.id)}
                                    className="text-xs h-7 px-2 text-primary border-primary/30 hover:bg-primary/10"
                                  >
                                    {t("agentHarness.activate")}
                                  </Button>
                                  <Button
                                    variant="outline"
                                    size="sm"
                                    onClick={() => handleDiscard(v.version, v.id)}
                                    className="text-xs h-7 px-2 text-danger-fg border-danger-fg/30 hover:bg-danger-fg/10"
                                  >
                                    {t("agentHarness.discard")}
                                  </Button>
                                </>
                              )}

                              {isArchived && !isActive && (
                                <Button
                                  variant="outline"
                                  size="sm"
                                  onClick={() => handleActivate(v.version, v.id)}
                                  className="text-xs h-7 px-2 text-primary border-primary/30 hover:bg-primary/10"
                                >
                                  {t("agentHarness.activate")}
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
        </>
      )}

      {/* TAB 2: Lessons Ledger */}
      {activeTab === "lessons" && (
        <>
          {/* Lessons Loading */}
          {lessonsLoading && (
            <div data-testid="agent-harness-lessons-loading" className="space-y-4">
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

          {/* Lessons Error */}
          {!lessonsLoading && lessonsError && (
            <div data-testid="agent-harness-lessons-error">
              <GlassCard className="p-6 text-center space-y-4 max-w-lg mx-auto">
                <Alert variant="danger" title={t("agentHarness.loadLessonsError")}>
                  {lessonsError}
                </Alert>
                <div>
                  <Button variant="outline" size="sm" onClick={() => void loadLessons()}>
                    {t("agentHarness.retry")}
                  </Button>
                </div>
              </GlassCard>
            </div>
          )}

          {/* Lessons Empty */}
          {!lessonsLoading && !lessonsError && lessons.length === 0 && (
            <div data-testid="agent-harness-lessons-empty">
              <GlassCard className="p-10 text-center max-w-xl mx-auto space-y-4">
                <Empty
                  reason={t("agentHarness.lessonsEmptyTitle")}
                  detail={t("agentHarness.lessonsEmptyDesc")}
                  action={
                    <Button
                      variant="primary"
                      onClick={() => setLessonModalOpen(true)}
                    >
                      <Plus className="h-4 w-4 mr-1" />
                      {t("agentHarness.addLesson")}
                    </Button>
                  }
                >
                  <div className="mb-1 flex justify-center">
                    <BookOpen className="h-8 w-8 text-muted-foreground" />
                  </div>
                </Empty>
              </GlassCard>
            </div>
          )}

          {/* Lessons Ready */}
          {!lessonsLoading && !lessonsError && lessons.length > 0 && (
            <div data-testid="agent-harness-lessons-ready" className="space-y-4">
              <GlassCard className="overflow-hidden p-0">
                <div className="border-b border-border/60 px-5 py-3 flex items-center justify-between">
                  <h3 className="font-bold text-sm text-foreground">
                    {t("agentHarness.lessonsTitle")}
                  </h3>
                  <span className="text-xs text-muted-foreground">
                    {t("agentHarness.lessonCount", { count: lessons.length })}
                  </span>
                </div>

                <Table>
                  <THead className="border-b border-border/60">
                    <TR>
                      <TH className="w-24">{t("agentHarness.lessonScope")}</TH>
                      <TH>{t("agentHarness.lessonNote")}</TH>
                      <TH>{t("agentHarness.status")}</TH>
                      <TH>{t("agentHarness.evidence")}</TH>
                      <TH>{t("agentHarness.createdBy")}</TH>
                      <TH>{t("agentHarness.createdAt")}</TH>
                      <TH className="text-right">{t("agentHarness.actions")}</TH>
                    </TR>
                  </THead>
                  <TBody className="divide-y divide-border/40">
                    {lessons.map((lesson) => {
                      const getStatusBadge = () => {
                        switch (lesson.status) {
                          case "adopted":
                            return <Badge variant="green">{t("agentHarness.lessonStatusAdopted")}</Badge>;
                          case "suggested":
                            return <Badge variant="amber">{t("agentHarness.lessonStatusSuggested")}</Badge>;
                          case "dismissed":
                          default:
                            return <Badge variant="neutral">{t("agentHarness.lessonStatusDismissed")}</Badge>;
                        }
                      };

                      return (
                        <TR key={lesson.id}>
                          <TD>
                            <Badge variant="neutral" className="font-mono text-xs">
                              {lesson.scope}
                            </Badge>
                          </TD>
                          <TD>
                            <span className="text-xs font-medium text-foreground">
                              {lesson.note}
                            </span>
                          </TD>
                          <TD>{getStatusBadge()}</TD>
                          <TD className="max-w-xs">
                            <EvidenceViewer evidence={lesson.evidence} />
                          </TD>
                          <TD className="text-xs text-foreground/80">
                            {lesson.createdBy || "—"}
                          </TD>
                          <TD className="text-xs text-muted-foreground" title={lesson.createdAt}>
                            {timeAgo(lesson.createdAt)}
                          </TD>
                          <TD className="text-right whitespace-nowrap">
                            {lesson.status !== "dismissed" && (
                              <Button
                                variant="outline"
                                size="sm"
                                onClick={() => handleDismissLesson(lesson.id)}
                                className="text-xs h-7 px-2 text-danger-fg border-danger-fg/30 hover:bg-danger-fg/10"
                              >
                                {t("agentHarness.dismissLesson")}
                              </Button>
                            )}
                          </TD>
                        </TR>
                      );
                    })}
                  </TBody>
                </Table>
              </GlassCard>
            </div>
          )}
        </>
      )}

      {/* Modals */}
      <HarnessDetailModal
        open={inspectSpecId !== null}
        onClose={() => setInspectSpecId(null)}
        specId={inspectSpecId}
        cachedDetail={inspectSpecId ? detailCache[inspectSpecId] : null}
        isActive={inspectSpecId !== null && inspectSpecId === activeVersion?.id}
        onUseAsTemplate={(spec) => handleOpenEditor(spec)}
        onActivate={(vNum, id) => handleActivate(vNum, id)}
        onDiscard={(vNum, id) => handleDiscard(vNum, id)}
      />

      <HarnessDiffModal
        open={diffOpen}
        onClose={() => setDiffOpen(false)}
        activeDetail={activeDetail}
        targetDetail={diffTargetDetail}
        onActivate={(vNum, id) => handleActivate(vNum, id)}
      />

      <HarnessEditorModal
        open={editorOpen}
        onClose={() => setEditorOpen(false)}
        onSuccess={handleEditorSuccess}
        initialSpec={editorInitialSpec}
      />

      <HarnessProposalModal
        open={proposalOpen}
        onClose={() => setProposalOpen(false)}
        onSuccess={handleProposalSuccess}
      />

      <HarnessLessonModal
        open={lessonModalOpen}
        onClose={() => setLessonModalOpen(false)}
        onSuccess={handleLessonSuccess}
      />
    </ScreenWrap>
  );
}
