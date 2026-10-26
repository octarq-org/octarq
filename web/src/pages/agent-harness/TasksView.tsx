import React, { useState } from "react";
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
} from "lucide-react";

export interface WorkforceTask {
  id: string;
  title: string;
  assignee: string;
  assigneeRole: string;
  priority: "urgent" | "high" | "normal" | "low";
  status: "todo" | "in_progress" | "awaiting_approval" | "done";
  createdAt: string;
  lastHeartbeat: string;
  summary: string;
  executionSteps?: string[];
}

const DEFAULT_TASKS: WorkforceTask[] = [
  {
    id: "AGT-101",
    title: "Diagnose DNS resolution latency on managed nameservers",
    assignee: "DevOps Specialist",
    assigneeRole: "Infrastructure & Network",
    priority: "urgent",
    status: "in_progress",
    createdAt: "15 mins ago",
    lastHeartbeat: "Just now",
    summary: "Probed root latency across 4 geographic regions. Detected 120ms DNS spike on secondary NS.",
    executionSteps: [
      "query_dns_records(domain='api.octarq.com')",
      "measure_ns_latency(targets=['1.1.1.1', '8.8.8.8'])",
      "flag_record_drift(expected='192.0.2.1', actual='192.0.2.1')",
    ],
  },
  {
    id: "AGT-102",
    title: "Audit 2FA keys and purge expired API access tokens",
    assignee: "Security & Compliance Auditor",
    assigneeRole: "Trust & Safety",
    priority: "high",
    status: "awaiting_approval",
    createdAt: "1 hour ago",
    lastHeartbeat: "10 mins ago",
    summary: "Found 2 unused API tokens with expired grants. Awaiting Board approval before revocation.",
    executionSteps: [
      "scan_workspace_tokens(scope='all')",
      "filter_unused_credentials(threshold_days=90)",
      "propose_token_revocation(token_ids=[42, 45]) -> awaiting_approval",
    ],
  },
  {
    id: "AGT-103",
    title: "Inspect dead-letter queue and reclassify unhandled webhooks",
    assignee: "DevOps Specialist",
    assigneeRole: "Infrastructure & Network",
    priority: "normal",
    status: "done",
    createdAt: "3 hours ago",
    lastHeartbeat: "2 hours ago",
    summary: "Replayed 4 failed dead letters with updated HMAC signature. All delivered with HTTP 200.",
    executionSteps: [
      "list_dead_letters() -> 4 records",
      "verify_relay_endpoint(id=2)",
      "replay_dead_letter(id=101) -> 200 OK",
      "replay_dead_letter(id=102) -> 200 OK",
    ],
  },
  {
    id: "AGT-104",
    title: "Review 24-hour inbound email deliverability & bounce trends",
    assignee: "Mail Deliverability Agent",
    assigneeRole: "Communications",
    priority: "low",
    status: "done",
    createdAt: "5 hours ago",
    lastHeartbeat: "4 hours ago",
    summary: "Generated daily deliverability digest. Spam complaint rate is 0.01% (well below 0.1% threshold).",
    executionSteps: [
      "fetch_mail_metrics(window='24h')",
      "aggregate_bounce_reasons()",
      "publish_deliverability_summary()",
    ],
  },
];

export function TasksView() {
  const { t } = useTranslation();
  const [tasks, setTasks] = useState<WorkforceTask[]>(DEFAULT_TASKS);
  const [statusFilter, setStatusFilter] = useState<string>("all");
  const [selectedTask, setSelectedTask] = useState<WorkforceTask | null>(null);
  const [assignModalOpen, setAssignModalOpen] = useState(false);

  // New task form state
  const [newTitle, setNewTitle] = useState("");
  const [newAssignee, setNewAssignee] = useState("DevOps Specialist");
  const [newPriority, setNewPriority] = useState<WorkforceTask["priority"]>("normal");
  const [newGoal, setNewGoal] = useState("");

  const filteredTasks = tasks.filter((t) => {
    if (statusFilter === "all") return true;
    return t.status === statusFilter;
  });

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

  const handleCreateTask = () => {
    if (!newTitle.trim()) {
      toast.error("Task title is required.");
      return;
    }
    const created: WorkforceTask = {
      id: `AGT-${Math.floor(100 + Math.random() * 900)}`,
      title: newTitle.trim(),
      assignee: newAssignee,
      assigneeRole: "Assigned Agent",
      priority: newPriority,
      status: "in_progress",
      createdAt: "Just now",
      lastHeartbeat: "Just now",
      summary: newGoal.trim() || "Autonomous task dispatched to agent workforce.",
      executionSteps: [`dispatch_task(goal="${newTitle}")`, "agent_executing_plan()"],
    };
    setTasks((prev) => [created, ...prev]);
    toast.success(`Task ${created.id} assigned to ${newAssignee}.`);
    setAssignModalOpen(false);
    setNewTitle("");
    setNewGoal("");
  };

  return (
    <div className="space-y-6">
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
              className={`rounded-lg px-3 py-1 text-xs font-medium transition-colors ${
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
          {t("agentHarness.assignTask", "Assign Task")}
        </Button>
      </div>

      {/* Task tickets list */}
      <div className="space-y-3">
        {filteredTasks.length === 0 ? (
          <GlassCard className="p-8 text-center text-xs text-muted-foreground">
            {t("agentHarness.noTasksFilter")}
          </GlassCard>
        ) : (
          filteredTasks.map((task) => (
            <div
              key={task.id}
              role="button"
              tabIndex={0}
              onClick={() => setSelectedTask(task)}
              onKeyDown={(e) => {
                if (e.key === "Enter" || e.key === " ") setSelectedTask(task);
              }}
              className="cursor-pointer"
            >
              <GlassCard className="p-4 flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 hover:border-primary/40 transition-all">
                <div className="flex items-start gap-3">
                  <span className="font-mono text-xs font-semibold text-primary shrink-0 mt-0.5">
                    {task.id}
                  </span>
                  <div className="space-y-1">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-bold text-foreground hover:text-primary transition-colors">
                        {task.title}
                      </span>
                      {getPriorityBadge(task.priority)}
                    </div>
                    <div className="text-xs text-muted-foreground flex items-center gap-3">
                      <span className="inline-flex items-center gap-1 font-medium text-foreground/80">
                        <Bot className="h-3 w-3 text-primary" />
                        {task.assignee}
                      </span>
                      <span>•</span>
                      <span>{t("agentHarness.heartbeatPrefix", "Heartbeat:")} {task.lastHeartbeat}</span>
                    </div>
                    <div className="text-xs text-foreground/70 line-clamp-1">
                      {task.summary}
                    </div>
                  </div>
                </div>

                <div className="flex items-center justify-between sm:justify-end gap-3 shrink-0 pt-2 sm:pt-0 border-t sm:border-t-0 border-border/40">
                  {getStatusBadge(task.status)}
                  <ChevronRight className="h-4 w-4 text-muted-foreground" />
                </div>
              </GlassCard>
            </div>
          ))
        )}
      </div>

      {/* Assign Task Modal */}
      {assignModalOpen && (
        <Modal
          onClose={() => setAssignModalOpen(false)}
          title={t("agentHarness.assignTaskModalTitle", "Assign Autonomous Task")}
        >
          <div className="space-y-4 p-4 text-xs">
            <Field label={t("agentHarness.taskGoalLabel", "Task Goal / Title")}>
              <Input
                value={newTitle}
                onChange={(e: any) => setNewTitle(e.target.value)}
                placeholder="e.g. Audit SSL certificates and identify upcoming expirations"
                className="text-xs"
              />
            </Field>
            <Field label={t("agentHarness.assigneeAgentLabel", "Assignee Agent")}>
              <Select
                value={newAssignee}
                onValueChange={setNewAssignee}
                options={[
                  { value: "DevOps Specialist", label: "DevOps Specialist (Infrastructure)" },
                  { value: "Security & Compliance Auditor", label: "Security & Compliance Auditor (Security)" },
                  { value: "Lead Orchestrator", label: "Lead Orchestrator (Executive)" },
                  { value: "Mail Deliverability Agent", label: "Mail Deliverability Agent (Email)" },
                ]}
              />
            </Field>
            <Field label={t("agentHarness.priorityLevelLabel", "Priority Level")}>
              <Select
                value={newPriority}
                onValueChange={(val: any) => setNewPriority(val)}
                options={[
                  { value: "urgent", label: "Urgent (Run immediately)" },
                  { value: "high", label: "High (Next heartbeat)" },
                  { value: "normal", label: "Normal (Standard schedule)" },
                  { value: "low", label: "Low (Background backlog)" },
                ]}
              />
            </Field>
            <Field label={t("agentHarness.taskInstructionsLabel", "Detailed Instructions & Context")}>
              <textarea
                className="w-full h-24 rounded-lg border border-border bg-background p-2 font-mono text-xs text-foreground focus:outline-hidden focus:ring-1 focus:ring-primary"
                value={newGoal}
                onChange={(e) => setNewGoal(e.target.value)}
                placeholder={t("agentHarness.taskInstructionsPlaceholder", "Provide exact domain names, limits, or parameters for the agent…")}
              />
            </Field>
            <div className="flex items-center justify-end gap-2 pt-3 border-t border-border">
              <Button variant="ghost" onClick={() => setAssignModalOpen(false)}>
                {t("agentHarness.cancelBtn", "Cancel")}
              </Button>
              <Button variant="primary" onClick={handleCreateTask}>
                {t("agentHarness.assignKickOffBtn", "Assign & Kick Off")}
              </Button>
            </div>
          </div>
        </Modal>
      )}

      {/* Task Detail Modal */}
      {selectedTask && (
        <Modal
          onClose={() => setSelectedTask(null)}
          title={`${selectedTask.id} — ${t("agentHarness.taskDetailModalTitle", "Task Execution Detail")}`}
        >
          <div className="space-y-4 p-4 text-xs">
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
