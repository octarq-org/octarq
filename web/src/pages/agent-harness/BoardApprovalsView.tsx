import React, { useEffect, useState } from "react";
import {
  GlassCard,
  Button,
  Badge,
  toast,
  useTranslation,
} from "@octarq/plugin-sdk";
import {
  ShieldAlert,
  Check,
  X,
  Bot,
  AlertTriangle,
  Clock,
  Sparkles,
  ArrowRight,
} from "lucide-react";
import { fetchApprovals, decideApproval } from "../../copilot/api";
import { api, type WorkforceTaskDTO } from "../../api";
import type { ActionDiff } from "../../copilot/types";

export function BoardApprovalsView() {
  const { t } = useTranslation();
  const [approvals, setApprovals] = useState<ActionDiff[]>([]);
  const [loading, setLoading] = useState(true);
  const [processingId, setProcessingId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  // Task approvals are the workforce domain's awaiting_approval gate. They are
  // a different object from a staged tool action, so they get their own list
  // and their own failure state.
  const [taskApprovals, setTaskApprovals] = useState<WorkforceTaskDTO[]>([]);
  const [taskLoading, setTaskLoading] = useState(true);
  const [taskError, setTaskError] = useState<string | null>(null);
  const [processingTaskId, setProcessingTaskId] = useState<string | null>(null);

  const loadTasks = async () => {
    setTaskLoading(true);
    setTaskError(null);
    try {
      const data = await api.workforceApprovals();
      setTaskApprovals(Array.isArray(data) ? data : []);
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : t("agentHarness.loadError");
      setTaskError(message);
      setTaskApprovals([]);
    } finally {
      setTaskLoading(false);
    }
  };

  useEffect(() => {
    void loadTasks();
  }, []);

  const handleTaskDecision = async (id: string, decision: "approve" | "reject") => {
    setProcessingTaskId(id);
    try {
      await api.decideWorkforceApproval(id, decision);
      toast.success(decision === "approve" ? t("agentHarness.taskApproved", "Task approved and resumed.") : t("agentHarness.taskRejected", "Task rejected and returned to the queue."));
      setTaskApprovals((prev) => prev.filter((a) => a.id !== id));
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : "Action failed.";
      toast.error(message);
    } finally {
      setProcessingTaskId(null);
    }
  };

  const load = async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await fetchApprovals();
      setApprovals(data || []);
    } catch (err: unknown) {
      // Pre-v1.0 Clean-Cut: Strictly eliminate mock fallbacks and fail-closed.
      const message = err instanceof Error ? err.message : t("agentHarness.loadError");
      setError(message);
      setApprovals([]);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    load();
  }, []);

  const handleDecision = async (id: string, decision: "approve" | "reject") => {
    setProcessingId(id);
    try {
      await decideApproval(id, decision);
      toast.success(decision === "approve" ? "Approved and executed." : "Action rejected.");
      setApprovals((prev) => prev.filter((a) => a.id !== id));
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : "Action failed.";
      toast.error(message);
    } finally {
      setProcessingId(null);
    }
  };

  return (
    <div className="space-y-6">
      {/* Top Banner */}
      <div className="flex items-center justify-between rounded-xl border border-warning-border bg-warning-bg p-4 text-xs">
        <div className="flex items-center gap-2.5">
          <ShieldAlert className="h-5 w-5 text-warning-fg shrink-0" />
          <div className="text-foreground">
            <strong>{t("agentHarness.boardGovernanceTitle")}</strong>: {t("agentHarness.boardGovernanceDesc")}
          </div>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={() => {
            void load();
            void loadTasks();
          }}
          disabled={loading || taskLoading}
        >
          {t("agentHarness.refreshQueue")}
        </Button>
      </div>

      {/* Task approvals: the workforce awaiting_approval gate */}
      <div data-testid="board-task-approvals" className="space-y-3">
        <h3 className="text-sm font-semibold text-foreground">
          {t("agentHarness.taskApprovalsTitle", "Task Approvals")}
        </h3>
        {taskLoading ? (
          <div className="text-center py-4 text-xs text-muted-foreground">
            {t("agentHarness.checkingQueue")}
          </div>
        ) : taskError ? (
          <div data-testid="board-task-approvals-error" className="text-xs text-danger-fg">
            {taskError}{" "}
            <Button variant="ghost" size="sm" onClick={() => void loadTasks()}>
              {t("agentHarness.retry")}
            </Button>
          </div>
        ) : taskApprovals.length === 0 ? (
          <div className="text-xs text-muted-foreground">
            {t("agentHarness.taskApprovalsEmpty", "No task is awaiting approval.")}
          </div>
        ) : (
          <div className="space-y-2">
            {taskApprovals.map((taskItem) => (
              <GlassCard key={taskItem.id} className="p-4 flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
                <div className="space-y-1">
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-xs font-semibold text-warning-fg">{taskItem.id}</span>
                    <h4 className="text-sm font-bold text-foreground">{taskItem.title}</h4>
                    <Badge variant="warning" className="text-[10px] uppercase font-mono">
                      {taskItem.priority}
                    </Badge>
                  </div>
                  <div className="text-xs text-muted-foreground flex items-center gap-2">
                    <span className="flex items-center gap-1">
                      <Bot className="h-3 w-3 text-accent-fg" />
                      {taskItem.assignee || taskItem.assigneeAgentId}
                    </span>
                  </div>
                  {taskItem.summary && (
                    <p className="text-xs text-foreground max-w-xl">{taskItem.summary}</p>
                  )}
                </div>
                <div className="flex items-center gap-2 shrink-0">
                  <Button
                    variant="danger"
                    size="sm"
                    disabled={processingTaskId === taskItem.id}
                    onClick={() => handleTaskDecision(taskItem.id, "reject")}
                  >
                    <X className="h-3.5 w-3.5 mr-1" />
                    {t("agentHarness.rejectAction")}
                  </Button>
                  <Button
                    variant="primary"
                    size="sm"
                    disabled={processingTaskId === taskItem.id}
                    onClick={() => handleTaskDecision(taskItem.id, "approve")}
                  >
                    <Check className="h-3.5 w-3.5 mr-1" />
                    {t("agentHarness.approveExecute")}
                  </Button>
                </div>
              </GlassCard>
            ))}
          </div>
        )}
      </div>

      {loading ? (
        <div className="text-center py-10 text-xs text-muted-foreground">
          {t("agentHarness.checkingQueue")}
        </div>
      ) : error ? (
        <div data-testid="board-approvals-error">
          <GlassCard className="p-10 text-center space-y-4 max-w-lg mx-auto">
            <div className="flex justify-center">
              <AlertTriangle className="h-8 w-8 text-danger-fg" />
            </div>
            <div className="space-y-1">
              <h4 className="text-sm font-bold text-foreground">
                {t("agentHarness.loadError")}
              </h4>
              <p className="text-xs text-muted-foreground">{error}</p>
            </div>
            <div>
              <Button variant="outline" size="sm" onClick={load}>
                {t("agentHarness.retry")}
              </Button>
            </div>
          </GlassCard>
        </div>
      ) : approvals.length === 0 ? (
        <div data-testid="board-approvals-empty">
          <GlassCard className="p-12 text-center space-y-3">
            <div className="flex justify-center">
              <Sparkles className="h-8 w-8 text-success-fg" />
            </div>
            <h4 className="text-sm font-bold text-foreground">{t("agentHarness.signOffClean")}</h4>
            <p className="text-xs text-muted-foreground max-w-sm mx-auto">
              {t("agentHarness.signOffCleanDesc")}
            </p>
          </GlassCard>
        </div>
      ) : (
        <div className="space-y-4">
          {approvals.map((appr) => (
            <GlassCard key={appr.id} className="p-5 space-y-4 border-warning-border">
              <div className="flex items-start justify-between gap-3">
                <div className="space-y-1">
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-xs font-semibold text-warning-fg">
                      {appr.id}
                    </span>
                    <h4 className="text-sm font-bold text-foreground">{appr.title || appr.action}</h4>
                    <Badge variant="warning" className="text-[10px] uppercase font-mono">
                      {appr.riskLevel}
                    </Badge>
                  </div>
                  <div className="text-xs text-muted-foreground flex items-center gap-2">
                    <span className="flex items-center gap-1">
                      <Bot className="h-3 w-3 text-accent-fg" />
                      {t("agentHarness.requesterLabel")} {appr.agent || "AI Agent"}
                    </span>
                    <span>•</span>
                    <span className="flex items-center gap-1">
                      <Clock className="h-3 w-3" />
                      {t("agentHarness.targetLabel")} {appr.target}
                    </span>
                  </div>
                </div>

                <div className="flex items-center gap-2 shrink-0">
                  <Button
                    variant="danger"
                    size="sm"
                    disabled={processingId === appr.id}
                    onClick={() => handleDecision(appr.id, "reject")}
                  >
                    <X className="h-3.5 w-3.5 mr-1" />
                    {t("agentHarness.rejectAction")}
                  </Button>
                  <Button
                    variant="primary"
                    size="sm"
                    disabled={processingId === appr.id}
                    onClick={() => handleDecision(appr.id, "approve")}
                  >
                    <Check className="h-3.5 w-3.5 mr-1" />
                    {t("agentHarness.approveExecute")}
                  </Button>
                </div>
              </div>

              {appr.reason && (
                <div className="text-xs text-foreground leading-relaxed bg-well p-3 rounded-lg border border-border">
                  {appr.reason}
                </div>
              )}

              {appr.diff && appr.diff.length > 0 && (
                <div className="rounded-lg bg-black/80 border border-border p-3 font-mono text-[11px] text-warning-fg overflow-x-auto">
                  <div className="text-muted-foreground text-[10px] uppercase mb-1">
                    {t("agentHarness.stagedParams")}
                  </div>
                  {appr.diff.map((d, idx) => (
                    <div key={idx} className="flex items-center gap-2">
                      <span className="text-muted-foreground">{d.field}:</span>
                      <span className="text-danger-fg line-through">{String(d.before)}</span>
                      <span>→</span>
                      <span className="text-success-fg">{String(d.after)}</span>
                    </div>
                  ))}
                </div>
              )}
            </GlassCard>
          ))}
        </div>
      )}
    </div>
  );
}
