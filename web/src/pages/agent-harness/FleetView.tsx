import React, { useState, useEffect } from "react";
import {
  GlassCard,
  Button,
  Modal,
  Field,
  Input,
  Select,
  Empty,
  toast,
  useTranslation,
} from "@octarq/plugin-sdk";
import {
  Bot,
  Terminal,
  ShieldAlert,
  Mail,
  Play,
  Pause,
  Plus,
  Sliders,
  Sparkles,
  Cpu,
  Clock,
  Coins,
  AlertTriangle,
} from "lucide-react";
import { api, type WorkforceAgentDTO } from "../../api";

export type AIEmployee = WorkforceAgentDTO;

export function FleetView({ onOpenEditor }: { onOpenEditor?: () => void }) {
  const { t } = useTranslation();
  const [employees, setEmployees] = useState<AIEmployee[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [deployModalOpen, setDeployModalOpen] = useState(false);
  const [selectedEmployee, setSelectedEmployee] = useState<AIEmployee | null>(null);
  const [directivesModalOpen, setDirectivesModalOpen] = useState(false);

  // Deploy modal form state
  const [newName, setNewName] = useState("");
  const [newRole, setNewRole] = useState("Operations Assistant");
  const [newModel, setNewModel] = useState("GPT-4o");
  const [newDirectives, setNewDirectives] = useState("");

  const loadEmployees = async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await api.workforceAgents();
      setEmployees(data || []);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Failed to load workforce agents.";
      setError(msg);
      setEmployees([]); // Fail-closed: zero mock fallback
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadEmployees();
  }, []);

  const handleTriggerHeartbeat = async (emp: AIEmployee) => {
    const prevEmployees = employees;
    // Optimistic mutation
    setEmployees((prev) =>
      prev.map((e) => (e.id === emp.id ? { ...e, status: "executing" as const } : e))
    );

    try {
      await api.triggerWorkforceHeartbeat(emp.id);
      toast.success(`Heartbeat triggered for ${emp.name}.`);
      const updated = await api.workforceAgent(emp.id);
      setEmployees((prev) =>
        prev.map((e) => (e.id === emp.id ? { ...e, ...updated } : e))
      );
    } catch (err: unknown) {
      // Rollback on rejection
      setEmployees(prevEmployees);
      const msg = err instanceof Error ? err.message : "Failed to trigger heartbeat.";
      toast.error(msg);
    }
  };

  const handleTogglePause = async (emp: AIEmployee) => {
    const prevEmployees = employees;
    const nextStatus: AIEmployee["status"] = emp.status === "paused" ? "active" : "paused";

    // Optimistic mutation
    setEmployees((prev) =>
      prev.map((e) => (e.id === emp.id ? { ...e, status: nextStatus } : e))
    );

    try {
      if (nextStatus === "paused") {
        await api.pauseWorkforceAgent(emp.id);
      } else {
        await api.resumeWorkforceAgent(emp.id);
      }
      toast.success(`${emp.name} is now ${nextStatus}.`);
    } catch (err: unknown) {
      // Rollback on rejection
      setEmployees(prevEmployees);
      const msg = err instanceof Error ? err.message : "Failed to update agent status.";
      toast.error(msg);
    }
  };

  const handleDeployAgent = async () => {
    if (!newName.trim()) {
      toast.error("Agent name is required.");
      return;
    }

    try {
      const created = await api.createWorkforceAgent({
        name: newName.trim(),
        role: newRole,
        department: "Workspace Operations",
        model: newModel,
        directives: newDirectives.trim() || "Follow workspace directives and standing orders.",
        heartbeat: "Every 15m",
        budgetLimit: 10.0,
        capabilities: ["general-ops"],
      });
      setEmployees((prev) => [...prev, created]);
      toast.success(`Deployed ${created.name} to the workforce.`);
      setDeployModalOpen(false);
      setNewName("");
      setNewDirectives("");
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Failed to deploy agent.";
      toast.error(msg);
    }
  };

  const handleSaveDirectives = async () => {
    if (!selectedEmployee) return;
    const prevEmployees = employees;
    const targetId = selectedEmployee.id;
    const targetDirectives = selectedEmployee.directives;

    // Optimistic mutation
    setEmployees((prev) =>
      prev.map((e) => (e.id === targetId ? { ...e, directives: targetDirectives } : e))
    );
    setDirectivesModalOpen(false);

    try {
      await api.updateWorkforceAgent(targetId, { directives: targetDirectives });
      toast.success("Directives updated successfully.");
    } catch (err: unknown) {
      // Rollback on rejection
      setEmployees(prevEmployees);
      const msg = err instanceof Error ? err.message : "Failed to update directives.";
      toast.error(msg);
    }
  };

  const getRoleIcon = (dept: string) => {
    switch (dept) {
      case "Executive & Routing":
        return <Bot className="h-5 w-5 text-accent-fg" />;
      case "Operations":
        return <Terminal className="h-5 w-5 text-accent-fg" />;
      case "Security":
        return <ShieldAlert className="h-5 w-5 text-warning-fg" />;
      case "Communications":
        return <Mail className="h-5 w-5 text-accent-fg" />;
      default:
        return <Cpu className="h-5 w-5 text-accent-fg" />;
    }
  };

  const getStatusBadge = (status: AIEmployee["status"]) => {
    switch (status) {
      case "active":
        return (
          <span className="inline-flex items-center gap-1.5 rounded-full bg-success-bg border border-success-border px-2.5 py-0.5 text-xs font-semibold text-success-fg">
            <span className="h-1.5 w-1.5 rounded-full bg-success-fg animate-pulse" />
            {t("agentHarness.statusReady")}
          </span>
        );
      case "executing":
        return (
          <span className="inline-flex items-center gap-1.5 rounded-full bg-accent-soft border border-primary/20 px-2.5 py-0.5 text-xs font-semibold text-accent-fg">
            <span className="h-1.5 w-1.5 rounded-full bg-primary animate-ping" />
            {t("agentHarness.statusExecuting")}
          </span>
        );
      case "idle":
        return (
          <span className="inline-flex items-center gap-1.5 rounded-full bg-border px-2.5 py-0.5 text-xs font-medium text-muted-foreground">
            <span className="h-1.5 w-1.5 rounded-full bg-foreground/40" />
            {t("agentHarness.statusIdle")}
          </span>
        );
      case "paused":
        return (
          <span className="inline-flex items-center gap-1.5 rounded-full bg-warning-bg border border-warning-border px-2.5 py-0.5 text-xs font-semibold text-warning-fg">
            <span className="h-1.5 w-1.5 rounded-full bg-warning-fg" />
            {t("agentHarness.statusPaused")}
          </span>
        );
      default:
        return null;
    }
  };

  if (error) {
    return (
      <div className="space-y-6" data-testid="fleet-view-error">
        <GlassCard className="p-8 text-center flex flex-col items-center justify-center space-y-3">
          <AlertTriangle className="h-8 w-8 text-danger-fg" />
          <div>
            <h4 className="text-sm font-bold text-foreground">{t("agentHarness.fleetLoadError")}</h4>
            <p className="text-xs text-muted-foreground mt-1 max-w-md">{error}</p>
          </div>
          <Button variant="outline" size="sm" onClick={loadEmployees}>
            {t("agentHarness.retryBtn")}
          </Button>
        </GlassCard>
      </div>
    );
  }

  if (loading) {
    return (
      <div className="space-y-6" data-testid="fleet-view">
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {[1, 2, 3, 4].map((i) => (
            <GlassCard key={i} className="p-5 h-48 animate-pulse bg-muted">
              <div />
            </GlassCard>
          ))}
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6" data-testid="fleet-view">
      {/* Top action row */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h3 className="text-sm font-semibold text-foreground">
            {t("agentHarness.tabFleet", "AI Employees")}
          </h3>
          <p className="text-xs text-muted-foreground mt-0.5">
            {t("agentHarness.fleetDesc")}
          </p>
        </div>
        <Button variant="primary" size="sm" className="shrink-0 whitespace-nowrap" onClick={() => setDeployModalOpen(true)}>
          <Plus className="h-4 w-4 mr-1.5" />
          {t("agentHarness.hireAgent", "Deploy AI Employee")}
        </Button>
      </div>

      {employees.length === 0 && (
        <Empty reason={t("agentHarness.fleetEmpty")}>
          <Bot />
        </Empty>
      )}

      {/* Grid of employee cards */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        {employees.map((emp) => {
          const budgetPct = Math.min(100, Math.round((emp.budgetUsed / (emp.budgetLimit || 1)) * 100));
          return (
            <GlassCard key={emp.id} className="p-5 flex flex-col justify-between space-y-4">
              {/* Header */}
              <div className="flex items-start justify-between gap-3">
                <div className="flex items-start gap-3">
                  <div className="h-10 w-10 rounded-xl bg-well border border-border flex items-center justify-center shrink-0">
                    {getRoleIcon(emp.department)}
                  </div>
                  <div>
                    <div className="flex items-center gap-2">
                      <h4 className="text-sm font-bold text-foreground">{emp.name}</h4>
                      {getStatusBadge(emp.status)}
                    </div>
                    <div className="text-xs text-muted-foreground font-medium">{emp.role}</div>
                    <div className="text-[11px] text-muted-foreground/80 mt-0.5 flex items-center gap-2">
                      <span className="inline-flex items-center gap-1">
                        <Cpu className="h-3 w-3" />
                        {emp.model}
                      </span>
                      <span>•</span>
                      <span className="inline-flex items-center gap-1">
                        <Clock className="h-3 w-3" />
                        {emp.heartbeat}
                      </span>
                    </div>
                  </div>
                </div>

                <div className="flex items-center gap-1.5">
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-8 px-2 text-xs"
                    onClick={() => handleTriggerHeartbeat(emp)}
                    title={t("agentHarness.wakeAgentNow", "Trigger heartbeat now")}
                  >
                    <Play className="h-3.5 w-3.5 text-accent-fg mr-1" />
                    {t("agentHarness.heartbeatBtn", "Heartbeat")}
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    className={`h-8 w-8 p-0 ${emp.status === "paused" ? "text-success-fg hover:text-success-fg" : "text-muted-foreground hover:text-foreground"}`}
                    onClick={() => handleTogglePause(emp)}
                    title={emp.status === "paused" ? t("agentHarness.resumeBtn", "Resume") : t("agentHarness.pauseBtn", "Pause")}
                  >
                    {emp.status === "paused" ? <Play className="h-4 w-4" /> : <Pause className="h-4 w-4" />}
                  </Button>
                </div>
              </div>

              {/* Core Directives / Behavioral Rules snippet */}
              <div className="bg-well rounded-lg p-3 text-xs border border-border">
                <div className="flex items-center justify-between text-[11px] font-semibold text-muted-foreground mb-1">
                  <span className="flex items-center gap-1.5">
                    <Sparkles className="h-3 w-3 text-accent-fg" />
                    {t("agentHarness.coreDirectives", "Core Directives")}
                  </span>
                  <button
                    type="button"
                    onClick={() => {
                      setSelectedEmployee(emp);
                      setDirectivesModalOpen(true);
                    }}
                    className="text-accent-fg hover:underline flex items-center gap-1 font-normal cursor-pointer"
                  >
                    <Sliders className="h-3 w-3" />
                    {t("agentHarness.tuneDirectives", "Tune Directives")}
                  </button>
                </div>
                <p className="text-foreground font-mono text-[11.5px] leading-relaxed line-clamp-2">
                  {emp.directives}
                </p>
              </div>

              {/* Capabilities & Monthly Budget Footer */}
              <div className="pt-2 border-t border-border flex items-center justify-between gap-4 text-xs">
                {/* Capabilities pills */}
                <div className="flex flex-wrap gap-1">
                  {emp.capabilities.map((cap) => (
                    <span
                      key={cap}
                      className="px-2 py-0.5 rounded-md bg-well border border-border text-[10px] font-mono text-muted-foreground"
                    >
                      {cap}
                    </span>
                  ))}
                </div>

                {/* Budget usage progress */}
                <div className="flex items-center gap-2 shrink-0">
                  <div className="text-right">
                    <div className="text-[10px] text-muted-foreground flex items-center justify-end gap-1">
                      <Coins className="h-3 w-3" />
                      <span>{t("agentHarness.cycleSpend")}</span>
                    </div>
                    <div className="text-xs font-mono font-semibold text-foreground">
                      ${emp.budgetUsed.toFixed(2)}{" "}
                      <span className="text-[10px] text-muted-foreground font-normal">
                        / ${emp.budgetLimit.toFixed(2)}
                      </span>
                    </div>
                  </div>
                  <div className="w-12 h-1.5 bg-border rounded-full overflow-hidden">
                    <div
                      className={`h-full ${budgetPct > 90 ? "bg-danger-fg" : "bg-primary"}`}
                      style={{ width: `${budgetPct}%` }}
                    />
                  </div>
                </div>
              </div>
            </GlassCard>
          );
        })}
      </div>

      {/* Deploy AI Employee Modal */}
      {deployModalOpen && (
        <Modal
          onClose={() => setDeployModalOpen(false)}
          title={t("agentHarness.hireAgent", "Deploy AI Employee")}
        >
          <div className="space-y-4 pt-2">
            <Field label={t("agentHarness.agentName")}>
              <Input
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
                placeholder="e.g. SRE Incident Commander"
              />
            </Field>
            <Field label={t("agentHarness.agentRole")}>
              <Input
                value={newRole}
                onChange={(e) => setNewRole(e.target.value)}
                placeholder="e.g. Site Reliability Engineer"
              />
            </Field>
            <Field label={t("agentHarness.assignedModel")}>
              <Select
                value={newModel}
                onValueChange={(val) => setNewModel(val)}
                options={[
                  { value: "GPT-4o", label: "GPT-4o (OpenAI)" },
                  { value: "Claude 3.5 Sonnet", label: "Claude 3.5 Sonnet (Anthropic)" },
                  { value: "DeepSeek-V3", label: "DeepSeek-V3" },
                ]}
              />
            </Field>
            <Field label={t("agentHarness.initialDirectives")}>
              <textarea
                className="w-full h-24 rounded-lg border border-border bg-well px-3 py-2 text-xs font-mono focus:outline-none focus:ring-1 focus:ring-accent-border"
                value={newDirectives}
                onChange={(e) => setNewDirectives(e.target.value)}
                placeholder={t("agentHarness.initialDirectivesPlaceholder")}
              />
            </Field>
            <div className="flex justify-end gap-2 pt-2">
              <Button variant="ghost" size="sm" onClick={() => setDeployModalOpen(false)}>
                {t("common.cancel")}
              </Button>
              <Button variant="primary" size="sm" onClick={handleDeployAgent}>
                {t("agentHarness.confirmHire")}
              </Button>
            </div>
          </div>
        </Modal>
      )}

      {/* Directives Tuning Modal */}
      {directivesModalOpen && selectedEmployee && (
        <Modal
          onClose={() => setDirectivesModalOpen(false)}
          title={`${t("agentHarness.tuneDirectives", "Tune Directives")}: ${selectedEmployee.name}`}
        >
          <div className="space-y-4 pt-2">
            <div className="text-xs text-muted-foreground">
              {t("agentHarness.directivesModalHint", "Standing directives shape how this agent evaluates goals, executes tools, and interacts with teammates.")}
            </div>
            <Field label={t("agentHarness.standingDirectives", "Standing Directives")}>
              <textarea
                className="w-full h-36 rounded-lg border border-border bg-well px-3 py-2 text-xs font-mono leading-relaxed focus:outline-none focus:ring-1 focus:ring-accent-border"
                value={selectedEmployee.directives}
                onChange={(e) =>
                  setSelectedEmployee({ ...selectedEmployee, directives: e.target.value })
                }
              />
            </Field>
            <div className="flex items-center justify-between pt-2">
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  setDirectivesModalOpen(false);
                  onOpenEditor?.();
                }}
              >
                {t("agentHarness.openVersionEditor", "Open Version Editor")}
              </Button>
              <Button
                variant="primary"
                size="sm"
                onClick={handleSaveDirectives}
              >
                {t("agentHarness.saveDirectives", "Save Directives")}
              </Button>
            </div>
          </div>
        </Modal>
      )}
    </div>
  );
}
