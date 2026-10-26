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
} from "lucide-react";

export interface AIEmployee {
  id: string;
  name: string;
  role: string;
  department: string;
  model: string;
  status: "active" | "executing" | "idle" | "paused";
  heartbeat: string;
  budgetUsed: number;
  budgetLimit: number;
  capabilities: string[];
  directives: string;
}

const DEFAULT_EMPLOYEES: AIEmployee[] = [
  {
    id: "lead-orchestrator",
    name: "Lead Orchestrator",
    role: "Chief Operational Assistant",
    department: "Executive & Routing",
    model: "GPT-4o (OpenAI)",
    status: "active",
    heartbeat: "Every 15m",
    budgetUsed: 5.8,
    budgetLimit: 20.0,
    capabilities: ["all-tools", "delegate", "briefing"],
    directives: "Act as executive chief-of-staff. Coordinate workspace tasks, summarize metrics, and route issues.",
  },
  {
    id: "devops-engineer",
    name: "DevOps Specialist",
    role: "Infrastructure & Network Engineer",
    department: "Operations",
    model: "Claude 3.5 Sonnet",
    status: "executing",
    heartbeat: "Every 10m",
    budgetUsed: 4.5,
    budgetLimit: 15.0,
    capabilities: ["dns", "infra", "domains", "metrics"],
    directives: "Continuously verify nameserver latency, detect DNS record drift, and check TLS certificate expiry.",
  },
  {
    id: "security-auditor",
    name: "Security & Compliance Auditor",
    role: "Trust & Safety Inspector",
    department: "Security",
    model: "GPT-4o",
    status: "idle",
    heartbeat: "Every 1h",
    budgetUsed: 2.1,
    budgetLimit: 10.0,
    capabilities: ["tokens", "security", "rbac", "audit"],
    directives: "Audit API token permissions, enforce 2FA compliance, and inspect dead-letter security anomalies.",
  },
  {
    id: "mail-deliverability",
    name: "Mail Deliverability Agent",
    role: "Inbound & Relay Specialist",
    department: "Communications",
    model: "DeepSeek-V3",
    status: "active",
    heartbeat: "Every 30m",
    budgetUsed: 1.8,
    budgetLimit: 5.0,
    capabilities: ["mail", "mailstorage", "webhooks"],
    directives: "Analyze bounce spikes, classify inbound spam, and trigger external S3/R2 storage offloading.",
  },
];

export function FleetView({ onOpenEditor }: { onOpenEditor?: () => void }) {
  const { t } = useTranslation();
  const [employees, setEmployees] = useState<AIEmployee[]>(DEFAULT_EMPLOYEES);
  const [deployModalOpen, setDeployModalOpen] = useState(false);
  const [selectedEmployee, setSelectedEmployee] = useState<AIEmployee | null>(null);
  const [directivesModalOpen, setDirectivesModalOpen] = useState(false);

  // Deploy modal form state
  const [newName, setNewName] = useState("");
  const [newRole, setNewRole] = useState("Operations Assistant");
  const [newModel, setNewModel] = useState("GPT-4o");
  const [newDirectives, setNewDirectives] = useState("");

  const handleTriggerHeartbeat = (emp: AIEmployee) => {
    toast.success(`Heartbeat triggered for ${emp.name}. Pulling active workspace tasks…`);
    setEmployees((prev) =>
      prev.map((e) => (e.id === emp.id ? { ...e, status: "executing" as const } : e))
    );
    setTimeout(() => {
      setEmployees((prev) =>
        prev.map((e) => (e.id === emp.id ? { ...e, status: "active" as const } : e))
      );
    }, 3000);
  };

  const handleTogglePause = (emp: AIEmployee) => {
    const nextStatus: AIEmployee["status"] = emp.status === "paused" ? "active" : "paused";
    setEmployees((prev) =>
      prev.map((e) => (e.id === emp.id ? { ...e, status: nextStatus } : e))
    );
    toast.success(`${emp.name} is now ${nextStatus}.`);
  };

  const handleDeployAgent = () => {
    if (!newName.trim()) {
      toast.error("Agent name is required.");
      return;
    }
    const newEmp: AIEmployee = {
      id: `agent-${Date.now()}`,
      name: newName.trim(),
      role: newRole,
      department: "Workspace Operations",
      model: newModel,
      status: "active",
      heartbeat: "Every 15m",
      budgetUsed: 0.0,
      budgetLimit: 10.0,
      capabilities: ["general-ops"],
      directives: newDirectives.trim() || "Follow workspace directives and standing orders.",
    };
    setEmployees((prev) => [...prev, newEmp]);
    toast.success(`Deployed ${newEmp.name} to the workforce.`);
    setDeployModalOpen(false);
    setNewName("");
    setNewDirectives("");
  };

  const getRoleIcon = (dept: string) => {
    switch (dept) {
      case "Executive & Routing":
        return <Bot className="h-5 w-5 text-purple-400" />;
      case "Operations":
        return <Terminal className="h-5 w-5 text-cyan-400" />;
      case "Security":
        return <ShieldAlert className="h-5 w-5 text-amber-400" />;
      case "Communications":
        return <Mail className="h-5 w-5 text-blue-400" />;
      default:
        return <Cpu className="h-5 w-5 text-primary" />;
    }
  };

  const getStatusBadge = (status: AIEmployee["status"]) => {
    switch (status) {
      case "active":
        return (
          <span className="inline-flex items-center gap-1.5 rounded-full bg-emerald-500/10 px-2.5 py-0.5 text-xs font-semibold text-emerald-500">
            <span className="h-1.5 w-1.5 rounded-full bg-emerald-500 animate-pulse" />
            {t("agentHarness.statusReady")}
          </span>
        );
      case "executing":
        return (
          <span className="inline-flex items-center gap-1.5 rounded-full bg-blue-500/10 px-2.5 py-0.5 text-xs font-semibold text-blue-400">
            <span className="h-1.5 w-1.5 rounded-full bg-blue-400 animate-ping" />
            {t("agentHarness.statusExecuting")}
          </span>
        );
      case "idle":
        return (
          <span className="inline-flex items-center gap-1.5 rounded-full bg-foreground/10 px-2.5 py-0.5 text-xs font-medium text-foreground/60">
            <span className="h-1.5 w-1.5 rounded-full bg-foreground/40" />
            {t("agentHarness.statusIdle")}
          </span>
        );
      case "paused":
        return (
          <span className="inline-flex items-center gap-1.5 rounded-full bg-amber-500/10 px-2.5 py-0.5 text-xs font-semibold text-amber-500">
            <span className="h-1.5 w-1.5 rounded-full bg-amber-500" />
            {t("agentHarness.statusPaused")}
          </span>
        );
    }
  };

  return (
    <div className="space-y-6">
      {/* Top action row */}
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-sm font-semibold text-foreground">
            {t("agentHarness.tabFleet", "AI Employees")}
          </h3>
          <p className="text-xs text-muted-foreground mt-0.5">
            {t("agentHarness.fleetDesc")}
          </p>
        </div>
        <Button variant="primary" size="sm" onClick={() => setDeployModalOpen(true)}>
          <Plus className="h-4 w-4 mr-1.5" />
          {t("agentHarness.hireAgent", "Deploy AI Employee")}
        </Button>
      </div>

      {/* Grid of employee cards */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        {employees.map((emp) => {
          const budgetPct = Math.min(100, Math.round((emp.budgetUsed / emp.budgetLimit) * 100));
          return (
            <GlassCard key={emp.id} className="p-5 flex flex-col justify-between space-y-4">
              {/* Header */}
              <div className="flex items-start justify-between gap-3">
                <div className="flex items-start gap-3">
                  <div className="h-10 w-10 rounded-xl bg-foreground/5 border border-foreground/10 flex items-center justify-center shrink-0">
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
              </div>

              {/* Directives snippet */}
              <div className="rounded-lg bg-foreground/[0.03] border border-foreground/[0.06] p-2.5 text-xs text-foreground/80 italic font-mono leading-relaxed line-clamp-2">
                "{emp.directives}"
              </div>

              {/* Capabilities chips */}
              <div className="flex flex-wrap items-center gap-1.5">
                {emp.capabilities.map((cap) => (
                  <span
                    key={cap}
                    className="inline-flex items-center rounded-md bg-foreground/5 px-2 py-0.5 text-[10px] font-mono font-medium text-foreground/70 border border-foreground/10"
                  >
                    #{cap}
                  </span>
                ))}
              </div>

              {/* Budget usage gauge */}
              <div className="space-y-1.5 pt-1 border-t border-border/40">
                <div className="flex items-center justify-between text-xs">
                  <span className="text-muted-foreground flex items-center gap-1">
                    <Coins className="h-3.5 w-3.5" />
                    {t("agentHarness.monthlyBudget")}
                  </span>
                  <span className="font-mono font-medium text-foreground">
                    ${emp.budgetUsed.toFixed(2)} / ${emp.budgetLimit.toFixed(2)} ({budgetPct}%)
                  </span>
                </div>
                <div className="h-1.5 w-full rounded-full bg-foreground/10 overflow-hidden">
                  <div
                    className={`h-full rounded-full transition-all duration-300 ${
                      budgetPct > 80 ? "bg-amber-500" : "bg-primary"
                    }`}
                    style={{ width: `${budgetPct}%` }}
                  />
                </div>
              </div>

              {/* Actions footer */}
              <div className="flex items-center justify-end gap-2 pt-1">
                <Button
                  variant="outline"
                  size="sm"
                  className="text-xs"
                  onClick={() => {
                    setSelectedEmployee(emp);
                    setDirectivesModalOpen(true);
                  }}
                >
                  <Sliders className="h-3.5 w-3.5 mr-1" />
                  {t("agentHarness.directivesBtn")}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  className="text-xs"
                  onClick={() => handleTogglePause(emp)}
                >
                  {emp.status === "paused" ? (
                    <>
                      <Play className="h-3.5 w-3.5 mr-1" />
                      {t("agentHarness.resumeBtn")}
                    </>
                  ) : (
                    <>
                      <Pause className="h-3.5 w-3.5 mr-1" />
                      {t("agentHarness.pauseBtn")}
                    </>
                  )}
                </Button>
                <Button
                  variant="subtle"
                  size="sm"
                  className="text-xs"
                  disabled={emp.status === "paused"}
                  onClick={() => handleTriggerHeartbeat(emp)}
                >
                  <Sparkles className="h-3.5 w-3.5 mr-1" />
                  {t("agentHarness.heartbeatBtn")}
                </Button>
              </div>
            </GlassCard>
          );
        })}
      </div>

      {/* Deploy Agent Modal */}
      {deployModalOpen && (
        <Modal
          onClose={() => setDeployModalOpen(false)}
          title={t("agentHarness.hireAgent", "Deploy AI Employee")}
        >
          <div className="space-y-4 p-4 text-xs">
            <Field label={t("agentHarness.agentNameLabel")}>
              <Input
                value={newName}
                onChange={(e: any) => setNewName(e.target.value)}
                placeholder="e.g. Domain Reliability Specialist"
                className="text-xs"
              />
            </Field>
            <Field label={t("agentHarness.roleDeptLabel")}>
              <Select
                value={newRole}
                onValueChange={setNewRole}
                options={[
                  { value: "Operations Assistant", label: "Operations Assistant (General)" },
                  { value: "DNS & Traffic Specialist", label: "DNS & Traffic Specialist" },
                  { value: "Security Inspector", label: "Security & Secret Inspector" },
                  { value: "Mail Deliverability Specialist", label: "Mail Deliverability Specialist" },
                ]}
              />
            </Field>
            <Field label={t("agentHarness.modelProviderLabel")}>
              <Select
                value={newModel}
                onValueChange={setNewModel}
                options={[
                  { value: "GPT-4o", label: "OpenAI: GPT-4o (Default)" },
                  { value: "Claude 3.5 Sonnet", label: "Anthropic: Claude 3.5 Sonnet" },
                  { value: "DeepSeek-V3", label: "DeepSeek: DeepSeek-V3" },
                ]}
              />
            </Field>
            <Field label={t("agentHarness.standingDirectivesLabel")}>
              <textarea
                className="w-full h-24 rounded-lg border border-border bg-background p-2 font-mono text-xs text-foreground focus:outline-hidden focus:ring-1 focus:ring-primary"
                value={newDirectives}
                onChange={(e) => setNewDirectives(e.target.value)}
                placeholder={t("agentHarness.directivesPlaceholder")}
              />
            </Field>
            <div className="flex items-center justify-end gap-2 pt-3 border-t border-border">
              <Button variant="ghost" onClick={() => setDeployModalOpen(false)}>
                {t("agentHarness.cancelBtn")}
              </Button>
              <Button variant="primary" onClick={handleDeployAgent}>
                {t("agentHarness.deployAgentBtn")}
              </Button>
            </div>
          </div>
        </Modal>
      )}

      {/* Directives Modal */}
      {directivesModalOpen && selectedEmployee && (
        <Modal
          onClose={() => setDirectivesModalOpen(false)}
          title={`${selectedEmployee.name} — Directives`}
        >
          <div className="space-y-4 p-4 text-xs">
            <div>
              <span className="text-muted-foreground">{t("agentHarness.assignedRoleLabel")} </span>
              <strong className="text-foreground">{selectedEmployee.role}</strong>
            </div>
            <div>
              <span className="text-muted-foreground">{t("agentHarness.modelProviderText")} </span>
              <strong className="text-foreground">{selectedEmployee.model}</strong>
            </div>
            <Field label={t("agentHarness.standingDirectivesLabel")}>
              <textarea
                className="w-full h-28 rounded-lg border border-border bg-background p-2.5 font-mono text-xs text-foreground focus:outline-hidden focus:ring-1 focus:ring-primary"
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
                {t("agentHarness.openVersionEditor")}
              </Button>
              <Button
                variant="primary"
                size="sm"
                onClick={() => {
                  setEmployees((prev) =>
                    prev.map((e) => (e.id === selectedEmployee.id ? selectedEmployee : e))
                  );
                  toast.success("Directives updated.");
                  setDirectivesModalOpen(false);
                }}
              >
                {t("agentHarness.saveDirectives")}
              </Button>
            </div>
          </div>
        </Modal>
      )}
    </div>
  );
}
