import { useEffect, useState, type ReactNode } from "react";
import { Badge, GlassCard, Table, TBody, TD, TH, THead, TR, useTranslation } from "@octarq/plugin-sdk";
import { Receipt } from "lucide-react";
import { api, type WorkforceAgentDTO, type WorkforceSpendEventDTO } from "../../api";
import { formatUSD } from "../../lib/money";

// SpendLedger shows the append-only ledger behind an agent's budget: one row
// per billed turn, with the model, the tokens, and the cost that was charged.
export function SpendLedger() {
  const { t } = useTranslation();
  const [agents, setAgents] = useState<WorkforceAgentDTO[]>([]);
  const [agentId, setAgentId] = useState("");
  const [events, setEvents] = useState<WorkforceSpendEventDTO[]>([]);
  const [agentsLoading, setAgentsLoading] = useState(true);
  const [eventsLoading, setEventsLoading] = useState(false);
  const [loadError, setLoadError] = useState(false);

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      try {
        const list = await api.workforceAgents();
        if (cancelled) return;
        setAgents(list);
        setAgentId((current) => current || list[0]?.id || "");
      } catch {
        if (cancelled) return;
        setLoadError(true);
      } finally {
        if (!cancelled) setAgentsLoading(false);
      }
    };
    void load();
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    if (!agentId) return;
    let cancelled = false;
    const load = async () => {
      setEventsLoading(true);
      try {
        const list = await api.workforceAgentSpend(agentId);
        if (cancelled) return;
        setEvents(list);
        setLoadError(false);
      } catch {
        if (cancelled) return;
        setLoadError(true);
      } finally {
        if (!cancelled) setEventsLoading(false);
      }
    };
    void load();
    return () => {
      cancelled = true;
    };
  }, [agentId]);

  let body: ReactNode;
  if (agentsLoading || eventsLoading) {
    body = (
      <p className="py-4 text-center text-xs text-muted-foreground">
        {t("agentHarness.spendLoading", "Loading spend…")}
      </p>
    );
  } else if (loadError) {
    body = (
      <p className="py-4 text-center text-xs text-danger-fg">
        {t("agentHarness.spendLoadError", "Could not load spend data.")}
      </p>
    );
  } else if (agents.length === 0) {
    body = (
      <p className="py-4 text-center text-xs text-muted-foreground">
        {t("agentHarness.noAgentsForSpend", "Deploy an AI employee to record spend.")}
      </p>
    );
  } else if (events.length === 0) {
    body = (
      <p className="py-4 text-center text-xs text-muted-foreground">
        {t("agentHarness.noSpendEvents", "This agent has no billed turns yet.")}
      </p>
    );
  } else {
    body = (
      <Table>
        <THead>
          <TR>
            <TH>{t("agentHarness.spendColTime", "Time")}</TH>
            <TH>{t("agentHarness.spendColModel", "Model")}</TH>
            <TH>{t("agentHarness.spendColTokens", "Tokens")}</TH>
            <TH>{t("agentHarness.spendColSource", "Source")}</TH>
            <TH className="text-right">{t("agentHarness.spendColCost", "Cost")}</TH>
          </TR>
        </THead>
        <TBody>
          {events.map((event) => (
            <TR key={event.id} className="align-top">
              <TD className="whitespace-nowrap text-muted-foreground">
                {new Date(event.createdAt).toLocaleString()}
              </TD>
              <TD className="font-mono text-foreground">{event.model}</TD>
              <TD className="text-muted-foreground">
                {t("agentHarness.spendTokens", "{{input}} in / {{output}} out", {
                  input: event.inputTokens.toLocaleString(),
                  output: event.outputTokens.toLocaleString(),
                })}
              </TD>
              <TD>
                <Badge variant={event.source === "chat" ? "success" : "info"} className="text-[10px]">
                  {event.source === "chat"
                    ? t("agentHarness.spendSourceChat", "Chat")
                    : t("agentHarness.spendSourceRuntime", "Runtime")}
                </Badge>
              </TD>
              <TD className="text-right font-mono text-foreground">{formatUSD(event.costUsd)}</TD>
            </TR>
          ))}
        </TBody>
      </Table>
    );
  }

  return (
    <GlassCard className="p-5 space-y-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h4 className="text-sm font-bold text-foreground flex items-center gap-2">
            <Receipt className="h-4 w-4 text-accent-fg" />
            {t("agentHarness.recentSpend", "Recent Spend")}
          </h4>
          <p className="text-xs text-muted-foreground mt-0.5">
            {t("agentHarness.recentSpendDesc", "Each billed turn, newest first.")}
          </p>
        </div>
        {agents.length > 0 && (
          <label className="flex items-center gap-2 text-xs text-muted-foreground">
            {t("agentHarness.spendAgent", "Agent")}
            <select
              aria-label={t("agentHarness.spendAgent", "Agent")}
              value={agentId}
              onChange={(e) => setAgentId(e.target.value)}
              className="h-9 rounded-lg border border-border bg-card px-2.5 text-xs text-foreground outline-none transition-colors focus:border-accent-border focus:ring-1 focus:ring-accent-border"
            >
              {agents.map((agent) => (
                <option key={agent.id} value={agent.id}>
                  {agent.name}
                </option>
              ))}
            </select>
          </label>
        )}
      </div>
      {body}
    </GlassCard>
  );
}
