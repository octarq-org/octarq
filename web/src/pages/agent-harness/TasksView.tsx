import React, { useState, useEffect } from "react";
import {
  GlassCard,
  Button,
  Badge,
  Modal,
  Field,
  Input,
  Select,
  toast,
  useTranslation,
} from "@octarq/plugin-sdk";
import {
  CheckCircle2,
  Clock,
  AlertCircle,
  PlayCircle,
  Plus,
  Bot,
  Filter,
  ArrowRight,
  Terminal,
  ShieldCheck,
  ChevronRight,
  Sparkles,
  AlertTriangle,
} from "lucide-react";
import { api, type WorkforceAgentDTO, type WorkforceTaskDTO } from "../../api";

export type WorkforceTask = WorkforceTaskDTO;

export function TasksView() {
  const { t } = useTranslation();
  const [tasks, setTasks] = useState<WorkforceTask[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [statusFilter, setStatusFilter] = useState<string>("all");
  const [selectedTask, setSelectedTask] = useState<WorkforceTask | null>(null);
  const [assignModalOpen, setAssignModalOpen] = useState(false);

  // New task form state
  const [newTitle, setNewTitle] = useState("");
  const [newAssigneeId, setNewAssigneeId] = useState("");
  const [newPriority, setNewPriority] = useState<WorkforceTask["priority"]>("normal");
  const [newGoal, setNewGoal] = useState("");

  // Assignees come from the workspace fleet. A ticket cannot be created while
  // the fleet is unknown or empty: there is no agent to own the work.
  const [agents, setAgents] = useState<WorkforceAgentDTO[]>([]);
  const [agentsLoading, setAgentsLoading] = useState(true);
  const [agentsError, setAgentsError] = useState<string | null>(null);

  const loadAgents = async () => {
    setAgentsLoading(true);
    setAgentsError(null);
    try {
      const list = await api.workforceAgents();
      setAgents(list || []);
      setNewAssigneeId((prev) => (prev && list?.some((a) => a.id === prev) ? prev : list?.[0]?.id ?? ""));
    } catch (err: unknown) {
      setAgentsError(err instanceof Error ? err.message : "Failed to load the workforce.");
      setAgents([]);
      setNewAssigneeId("");
    } finally {
      setAgentsLoading(false);
    }
  };

  const loadTasks = async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await api.workforceTasks({
        status: statusFilter === "all" ? undefined : statusFilter,
      });
      setTasks(data || []);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Failed to load workforce tasks.";
      setError(msg);
      setTasks([]); // Fail-closed: zero mock fallback
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadTasks();
  }, [statusFilter]);

  useEffect(() => {
    loadAgents();
  }, []);

  const handleOpenTask = async (task: WorkforceTask) => {
    setSelectedTask(task);
    try {
      const detailed = await api.workforceTask(task.id);
      setSelectedTask(detailed);
    } catch {
      // Keep existing task summary if step fetch fails
    }
  };

  const getPriorityBadge = (p: WorkforceTask["priority"]) => {
    switch (p) {
      case "urgent":
        return <Badge variant="red" className="text-[10px] uppercase font-mono">{t("agentHarness.priorityUrgent")}</Badge>;
      case "high":
        return <Badge variant="amber" className="text-[10px] uppercase font-mono">{t("agentHarness.priorityHigh")}</Badge>;
      case "normal":
        return <Badge variant="indigo" className="text-[10px] uppercase font-mono">{t("agentHarness.priorityNormal")}</Badge>;
      case "low":
        return <Badge variant="neutral" className="text-[10px] uppercase font-mono">{t("agentHarness.priorityLow")}</Badge>;
    }
  };

  const getStatusBadge = (s: WorkforceTask["status"]) => {
    switch (s) {
      case "in_progress":
        return (
          <span className="inline-flex items-center gap-1 text-xs text-primary font-medium">
            <PlayCircle className="h-3.5 w-3.5 animate-spin" />
            {t("agentHarness.statusInProgress")}
          </span>
        );
      case "awaiting_approval":
        return (
          <span className="inline-flex items-center gap-1 text-xs text-warning-fg font-medium">
            <AlertCircle className="h-3.5 w-3.5" />
            {t("agentHarness.statusAwaitingSignOff")}
          </span>
        );
      case "done":
        return (
          <span className="inline-flex items-center gap-1 text-xs text-success-fg font-medium">
            <CheckCircle2 className="h-3.5 w-3.5" />
            {t("agentHarness.statusCompleted")}
          </span>
        );
      case "todo":
        return (
          <span className="inline-flex items-center gap-1 text-xs text-muted-foreground font-medium">
            <Clock className="h-3.5 w-3.5" />
            {t("agentHarness.statusQueued")}
          </span>
        );
    }
  };

  const handleCreateTask = async () => {
    if (!newTitle.trim()) {
      toast.error("Task title is required.");
      return;
    }
    if (!newAssigneeId) {
      toast.error("Select an agent for the task.");
      return;
    }
    if (!newGoal.trim()) {
      toast.error(t("agentHarness.taskObjectiveRequired", "Describe what the task must achieve."));
      return;
    }

    try {
      const created = await api.createWorkforceTask({
        title: newTitle.trim(),
        assigneeAgentId: newAssigneeId,
        priority: newPriority,
        summary: newGoal.trim(),
      });
      setTasks((prev) => [created, ...prev]);
      toast.success(`Task ${created.id} assigned to ${created.assignee}.`);
      setAssignModalOpen(false);
      setNewTitle("");
      setNewGoal("");
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Failed to create task.";
      toast.error(msg);
    }
  };

  if (error) {
    return (
      <div className="space-y-6" data-testid="tasks-view-error">
        <GlassCard className="p-8 text-center flex flex-col items-center justify-center space-y-3">
          <AlertTriangle className="h-8 w-8 text-danger-fg" />
          <div>
            <h4 className="text-sm font-bold text-foreground">{t("agentHarness.tasksLoadError")}</h4>
            <p className="text-xs text-muted-foreground mt-1 max-w-md">{error}</p>
          </div>
          <Button variant="outline" size="sm" onClick={loadTasks}>
            {t("agentHarness.retryBtn")}
          </Button>
        </GlassCard>
      </div>
    );
  }

  if (loading) {
    return (
      <div className="space-y-6" data-testid="tasks-view">
        <div className="space-y-3">
          {[1, 2, 3, 4].map((i) => (
            <GlassCard key={i} className="p-4 h-24 animate-pulse bg-muted/20">
              <div />
            </GlassCard>
          ))}
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6" data-testid="tasks-view">
      {/* Header and filter bar */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
        <div className="flex items-center gap-1 rounded-xl bg-foreground/5 p-1 border border-border/50">
          {[
            { id: "all", label: t("agentHarness.tabAllTasks") },
            { id: "in_progress", label: t("agentHarness.statusInProgress") },
            { id: "awaiting_approval", label: t("agentHarness.statusAwaitingSignOff") },
            { id: "done", label: t("agentHarness.statusCompleted") },
          ].map((tab) => (
            <button
              key={tab.id}
              type="button"
              onClick={() => setStatusFilter(tab.id)}
              className={`rounded-lg px-3 py-1 text-xs font-medium transition-colors cursor-pointer ${
                statusFilter === tab.id
                  ? "bg-background text-foreground shadow-xs font-semibold"
                  : "text-muted-foreground hover:text-foreground"
              }`}
            >
              {tab.label}
            </button>
          ))}
        </div>

        <Button variant="primary" size="sm" onClick={() => setAssignModalOpen(true)}>
          <Plus className="h-4 w-4 mr-1.5" />
          {t("agentHarness.assignTask", "Assign Task Ticket")}
        </Button>
      </div>

      {tasks.length === 0 && (
        <GlassCard className="p-8 text-center text-muted-foreground text-xs" data-testid="tasks-view-empty">
          {t("agentHarness.tasksEmpty")}
        </GlassCard>
      )}

      {/* Task tickets list */}
      <div className="space-y-3">
        {tasks.map((task) => (
          <div
            key={task.id}
            onClick={() => handleOpenTask(task)}
            className="cursor-pointer"
          >
            <GlassCard className="p-4 transition-all hover:border-primary/40">
              <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
                {/* Left meta & title */}
                <div className="space-y-1.5 flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-xs font-bold text-foreground">
                      {task.id}
                    </span>
                    {getPriorityBadge(task.priority)}
                    {getStatusBadge(task.status)}
                  </div>
                  <h4 className="text-sm font-semibold text-foreground truncate">
                    {task.title}
                  </h4>
                  <p className="text-xs text-muted-foreground line-clamp-1">
                    {task.summary}
                  </p>
                </div>

                {/* Right assignee & heartbeat */}
                <div className="flex items-center gap-4 shrink-0 border-t sm:border-t-0 pt-2 sm:pt-0 border-border/40 justify-between sm:justify-end">
                  <div className="flex items-center gap-2 text-right">
                    <div className="h-8 w-8 rounded-lg bg-foreground/5 border border-border/60 flex items-center justify-center shrink-0">
                      <Bot className="h-4 w-4 text-primary" />
                    </div>
                    <div>
                      <div className="text-xs font-medium text-foreground">{task.assignee}</div>
                      <div className="text-[10px] text-muted-foreground flex items-center gap-1">
                        <Clock className="h-3 w-3" />
                        <span>{task.lastHeartbeat}</span>
                      </div>
                    </div>
                  </div>

                  <ChevronRight className="h-4 w-4 text-muted-foreground/60" />
                </div>
              </div>
            </GlassCard>
          </div>
        ))}
      </div>

      {/* Assign Task Modal */}
      {assignModalOpen && (
        <Modal
          onClose={() => setAssignModalOpen(false)}
          title={t("agentHarness.assignTask", "Assign Autonomous Task")}
        >
          <div className="space-y-4 pt-2">
            <Field label={t("agentHarness.taskTitle")}>
              <Input
                value={newTitle}
                onChange={(e) => setNewTitle(e.target.value)}
                placeholder="e.g. Audit TLS certificates and renew expiring nameserver SANs"
              />
            </Field>

            <Field label={t("agentHarness.assignedAgent")}>
              {agentsLoading ? (
                <div className="text-xs text-muted-foreground py-2">{t("agentHarness.loadingAgents", "Loading agents…")}</div>
              ) : agentsError ? (
                <div className="text-xs text-danger-fg py-2">
                  {agentsError}
                  <Button variant="ghost" size="sm" className="ml-2" onClick={loadAgents}>
                    {t("agentHarness.retryBtn")}
                  </Button>
                </div>
              ) : agents.length === 0 ? (
                <div className="text-xs text-muted-foreground py-2" data-testid="tasks-no-agents">
                  {t("agentHarness.noAgentsForTask", "Deploy an agent first. A task must belong to an agent in this workspace.")}
                </div>
              ) : (
                <Select
                  value={newAssigneeId}
                  onValueChange={(val) => setNewAssigneeId(val)}
                  options={agents.map((a) => ({
                    value: a.id,
                    label: `${a.name} (${a.department})`,
                  }))}
                />
              )}
            </Field>

            <Field label={t("agentHarness.priority")}>
              <Select
                value={newPriority}
                onValueChange={(val) => setNewPriority(val as WorkforceTask["priority"])}
                options={[
                  { value: "urgent", label: "Urgent" },
                  { value: "high", label: "High" },
                  { value: "normal", label: "Normal" },
                  { value: "low", label: "Low" },
                ]}
              />
            </Field>

            <Field label={t("agentHarness.objectiveContext")}>
              <textarea
                className="w-full h-24 rounded-lg border border-border/80 bg-well px-3 py-2 text-xs font-mono focus:outline-none focus:ring-1 focus:ring-primary"
                value={newGoal}
                onChange={(e) => setNewGoal(e.target.value)}
                placeholder={t("agentHarness.objectiveContextPlaceholder")}
              />
            </Field>

            <div className="flex justify-end gap-2 pt-2">
              <Button variant="ghost" size="sm" onClick={() => setAssignModalOpen(false)}>
                {t("common.cancel")}
              </Button>
              <Button
                variant="primary"
                size="sm"
                onClick={handleCreateTask}
                disabled={agentsLoading || agents.length === 0}
              >
                {t("agentHarness.dispatchTask")}
              </Button>
            </div>
          </div>
        </Modal>
      )}

      {/* Task Details & Tool Log Modal */}
      {selectedTask && (
        <Modal
          onClose={() => setSelectedTask(null)}
          title={`${t("agentHarness.taskDetails")}: ${selectedTask.id}`}
        >
          <div className="space-y-4 pt-1 text-xs">
            <div>
              <h4 className="text-sm font-bold text-foreground">{selectedTask.title}</h4>
              <div className="flex items-center gap-3 mt-1 text-muted-foreground">
                <span>{t("agentHarness.taskModalAssignee", "Assignee:")} <strong className="text-foreground">{selectedTask.assignee}</strong></span>
                <span>•</span>
                <span>{t("agentHarness.taskModalPriority", "Priority:")} {selectedTask.priority.toUpperCase()}</span>
                <span>•</span>
                <span>{t("agentHarness.taskModalStatus", "Status:")} {getStatusBadge(selectedTask.status)}</span>
              </div>
            </div>

            <div className="rounded-lg bg-foreground/[0.03] border border-border/50 p-3 space-y-1">
              <div className="text-[11px] font-semibold text-muted-foreground uppercase tracking-wider">
                {t("agentHarness.execSummaryLabel", "Execution Summary")}
              </div>
              <div className="text-foreground/90 leading-relaxed">{selectedTask.summary}</div>
            </div>

            {selectedTask.executionSteps && selectedTask.executionSteps.length > 0 && (
              <div className="space-y-1.5">
                <div className="text-[11px] font-semibold text-muted-foreground uppercase tracking-wider flex items-center gap-1.5">
                  <Terminal className="h-3.5 w-3.5" />
                  {t("agentHarness.toolExecLogLabel", "Tool Execution Log")}
                </div>
                <div className="rounded-lg bg-black/80 border border-border/60 p-3 font-mono text-[11px] text-success-fg space-y-1">
                  {selectedTask.executionSteps.map((step, idx) => (
                    <div key={idx} className="flex items-center gap-2">
                      <span className="text-muted-foreground">{idx + 1}.</span>
                      <span>{step}</span>
                    </div>
                  ))}
                </div>
              </div>
            )}

            <div className="flex items-center justify-end pt-2">
              <Button variant="primary" size="sm" onClick={() => setSelectedTask(null)}>
                {t("agentHarness.closeBtn", "Close")}
              </Button>
            </div>
          </div>
        </Modal>
      )}
    </div>
  );
}
