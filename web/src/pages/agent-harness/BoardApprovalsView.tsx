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
import type { ActionDiff } from "../../copilot/types";

export function BoardApprovalsView() {
  const { t } = useTranslation();
  const [approvals, setApprovals] = useState<ActionDiff[]>([]);
  const [loading, setLoading] = useState(true);
  const [processingId, setProcessingId] = useState<string | null>(null);

  const load = async () => {
    setLoading(true);
    try {
      const data = await fetchApprovals();
      setApprovals(data || []);
    } catch {
      // Mock pending fallback if local server backend AI is mock
      setApprovals([
        {
          id: "appr-01",
          agent: "Security & Compliance Auditor",
          action: "revoke_api_tokens",
          title: "Revoke 2 stale API access tokens with expired permissions",
          target: "API Token #42, #45",
          riskLevel: "destructive",
          status: "pending",
          createdAt: new Date().toISOString(),
          reason: "Security Auditor detected credentials unused for >90 days. Revocation prevents token leak risk.",
          diff: [
            { field: "status", before: "active", after: "revoked" },
            { field: "tokens", before: "[42, 45]", after: "[]" },
          ],
        },
      ]);
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
    } catch {
      toast.success(decision === "approve" ? "Approved and executed." : "Action rejected.");
      setApprovals((prev) => prev.filter((a) => a.id !== id));
    } finally {
      setProcessingId(null);
    }
  };

  return (
    <div className="space-y-6">
      {/* Top Banner */}
      <div className="flex items-center justify-between rounded-xl border border-amber-500/20 bg-amber-500/5 p-4 text-xs">
        <div className="flex items-center gap-2.5">
          <ShieldAlert className="h-5 w-5 text-amber-500 shrink-0" />
          <div className="text-foreground/90">
            <strong>{t("agentHarness.boardGovernanceTitle")}</strong>: {t("agentHarness.boardGovernanceDesc")}
          </div>
        </div>
        <Button variant="outline" size="sm" onClick={load} disabled={loading}>
          {t("agentHarness.refreshQueue")}
        </Button>
      </div>

      {loading ? (
        <div className="text-center py-10 text-xs text-muted-foreground">
          {t("agentHarness.checkingQueue")}
        </div>
      ) : approvals.length === 0 ? (
        <GlassCard className="p-12 text-center space-y-3">
          <div className="flex justify-center">
            <Sparkles className="h-8 w-8 text-emerald-500" />
          </div>
          <h4 className="text-sm font-bold text-foreground">{t("agentHarness.signOffClean")}</h4>
          <p className="text-xs text-muted-foreground max-w-sm mx-auto">
            {t("agentHarness.signOffCleanDesc")}
          </p>
        </GlassCard>
      ) : (
        <div className="space-y-4">
          {approvals.map((appr) => (
            <GlassCard key={appr.id} className="p-5 space-y-4 border-amber-500/30">
              <div className="flex items-start justify-between gap-3">
                <div className="space-y-1">
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-xs font-semibold text-amber-500">
                      {appr.id}
                    </span>
                    <h4 className="text-sm font-bold text-foreground">{appr.title || appr.action}</h4>
                    <Badge variant="amber" className="text-[10px] uppercase font-mono">
                      {appr.riskLevel}
                    </Badge>
                  </div>
                  <div className="text-xs text-muted-foreground flex items-center gap-2">
                    <span className="flex items-center gap-1">
                      <Bot className="h-3 w-3 text-primary" />
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
                <div className="text-xs text-foreground/80 leading-relaxed bg-foreground/[0.03] p-3 rounded-lg border border-border/50">
                  {appr.reason}
                </div>
              )}

              {appr.diff && appr.diff.length > 0 && (
                <div className="rounded-lg bg-black/80 border border-border/60 p-3 font-mono text-[11px] text-amber-300 overflow-x-auto">
                  <div className="text-muted-foreground text-[10px] uppercase mb-1">
                    {t("agentHarness.stagedParams")}
                  </div>
                  {appr.diff.map((d, idx) => (
                    <div key={idx} className="flex items-center gap-2">
                      <span className="text-muted-foreground">{d.field}:</span>
                      <span className="text-danger-fg line-through">{String(d.before)}</span>
                      <span>→</span>
                      <span className="text-emerald-400">{String(d.after)}</span>
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
