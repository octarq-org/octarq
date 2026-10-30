import { useTranslation } from "../i18n";
import type { WorkforceAgentDTO } from "../api/types";
import { formatUSD } from "../lib/money";

export interface CopilotAgentPickerProps {
  agents: WorkforceAgentDTO[];
  value: string;
  onChange: (agentId: string) => void;
  loadFailed: boolean;
  // autoHalt is the workspace switch that makes a budget limit blocking. When
  // it is unknown, the picker does not claim the limit stops the agent.
  autoHalt: boolean | null;
}

// Admin-only control in the chat footer: picks the workforce agent the next
// turns are charged to. An empty value leaves the turns unbilled.
export function CopilotAgentPicker({ agents, value, onChange, loadFailed, autoHalt }: CopilotAgentPickerProps) {
  const { t } = useTranslation();
  const selected = agents.find((a) => a.id === value);
  const paused = selected?.status === "paused";
  const atLimit =
    autoHalt === true && selected != null && paused !== true && selected.budgetUsed >= selected.budgetLimit;

  return (
    <div className="space-y-1">
      <div className="flex items-center gap-2">
        <label
          htmlFor="copilot-agent-attribution"
          className="shrink-0 font-mono text-[10px] uppercase tracking-[0.08em] text-muted-foreground"
        >
          {t("copilot.agentBillTo", "Bill to")}
        </label>
        <select
          id="copilot-agent-attribution"
          aria-label={t("copilot.agentSelectLabel", "Agent to bill")}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          className="h-8 min-w-0 flex-1 rounded-lg border border-border bg-card px-2 text-xs text-foreground outline-none transition-colors focus:border-accent-border focus:ring-1 focus:ring-accent-border"
        >
          <option value="">{t("copilot.agentNone", "No agent")}</option>
          {agents.map((agent) => (
            <option key={agent.id} value={agent.id}>
              {agent.name}
            </option>
          ))}
        </select>
        {selected && (
          <span className="shrink-0 font-mono text-[10px] text-muted-foreground">
            {t("copilot.agentSpend", "{{used}} of {{limit}}", {
              used: formatUSD(selected.budgetUsed),
              limit: formatUSD(selected.budgetLimit),
            })}
          </span>
        )}
      </div>
      {selected && (
        <p className="text-[10px] text-muted-foreground">
          {t("copilot.agentWorkspaceDefault", "Every chat turn in this workspace bills this agent.")}
        </p>
      )}
      {loadFailed && (
        <p className="text-[10px] text-warning-fg">{t("copilot.agentLoadError", "Could not load agents.")}</p>
      )}
      {paused && (
        <p className="text-[10px] text-warning-fg">
          {t("copilot.agentPaused", "This agent is paused. Resume it in AI Workforce.")}
        </p>
      )}
      {atLimit && (
        <p className="text-[10px] text-warning-fg">
          {t(
            "copilot.agentBudgetExceeded",
            "This agent reached its monthly budget limit. Raise the limit in AI Workforce.",
          )}
        </p>
      )}
    </div>
  );
}
