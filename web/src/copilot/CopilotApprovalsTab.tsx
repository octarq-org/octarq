import { useState } from "react";
import { Button, cn, LockedFeature } from "@octarq/plugin-sdk";
import { ShieldAlert } from "lucide-react";
import { useTranslation } from "../i18n";
import { useCopilotStore } from "./store";
import { ActionDiffCard } from "./ActionDiffCard";

export interface CopilotApprovalsTabProps {
  onRefreshApprovals: () => Promise<void>;
  approvalsLoading: boolean;
}

export function CopilotApprovalsTab({
  onRefreshApprovals,
  approvalsLoading,
}: CopilotApprovalsTabProps) {
  const { t } = useTranslation();
  const { pendingApprovals, approvalsError } = useCopilotStore();
  const [approvalsFilter, setApprovalsFilter] = useState<"all" | "pending" | "resolved">("all");

  const pendingCount = pendingApprovals.filter((a) => a.status === "pending").length;

  const filteredApprovals = pendingApprovals.filter((a) => {
    if (approvalsFilter === "pending") return a.status === "pending";
    if (approvalsFilter === "resolved") return a.status !== "pending";
    return true;
  });

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      {/* Filter Pills */}
      <div className="flex items-center justify-between border-b border-border bg-well/20 px-4 py-2.5">
        <div className="flex items-center gap-1 text-xs">
          <button
            type="button"
            onClick={() => setApprovalsFilter("all")}
            className={cn(
              "rounded-lg px-2.5 py-1 font-medium transition-colors",
              approvalsFilter === "all"
                ? "bg-surface-hover text-foreground font-semibold"
                : "text-muted-foreground hover:text-foreground",
            )}
          >
            {t("copilot.filterAll", "全部")} ({pendingApprovals.length})
          </button>
          <button
            type="button"
            onClick={() => setApprovalsFilter("pending")}
            className={cn(
              "rounded-lg px-2.5 py-1 font-medium transition-colors",
              approvalsFilter === "pending"
                ? "bg-warning-fg/15 text-warning-fg font-semibold"
                : "text-muted-foreground hover:text-foreground",
            )}
          >
            {t("copilot.filterPending", "待审批")} ({pendingCount})
          </button>
          <button
            type="button"
            onClick={() => setApprovalsFilter("resolved")}
            className={cn(
              "rounded-lg px-2.5 py-1 font-medium transition-colors",
              approvalsFilter === "resolved"
                ? "bg-surface-hover text-foreground font-semibold"
                : "text-muted-foreground hover:text-foreground",
            )}
          >
            {t("copilot.filterResolved", "已处理")} ({pendingApprovals.length - pendingCount})
          </button>
        </div>
      </div>

      {/* Approvals List */}
      <div className="flex-1 overflow-y-auto p-4 space-y-3.5">
        {approvalsError === "locked" && filteredApprovals.length === 0 ? (
          <div data-testid="copilot-approvals-locked">
            <LockedFeature status={402} feature="Agent Approvals" />
          </div>
        ) : approvalsError && filteredApprovals.length === 0 ? (
          <div role="alert" className="py-6 text-center space-y-2">
            <p className="text-sm font-medium text-danger-fg">
              {t("copilot.approvalsLoadFailed", "待审队列加载失败")}
            </p>
            <p className="text-xs text-muted-foreground max-w-xs mx-auto break-words">
              {approvalsError}
            </p>
            <Button variant="subtle" size="sm" onClick={() => void onRefreshApprovals()}>
              {t("copilot.retry", "重试")}
            </Button>
          </div>
        ) : approvalsLoading && filteredApprovals.length === 0 ? (
          <div className="py-12 text-center">
            <p className="text-xs text-muted-foreground">
              {t("copilot.approvalsLoading", "正在加载待审队列…")}
            </p>
          </div>
        ) : filteredApprovals.length === 0 ? (
          <div className="py-12 text-center space-y-2">
            <ShieldAlert className="mx-auto h-8 w-8 text-muted-foreground/60" />
            <p className="text-sm font-medium text-foreground">
              {t("copilot.noApprovals", "暂无待审批的高危操作")}
            </p>
            <p className="text-xs text-muted-foreground max-w-xs mx-auto">
              {t(
                "copilot.noApprovalsDesc",
                "当外部 Agent 或自动化规则提议破坏性变更时，待审 Action Diff 卡片将在此显示。",
              )}
            </p>
          </div>
        ) : (
          filteredApprovals.map((action) => (
            <ActionDiffCard key={action.id} action={action} />
          ))
        )}
      </div>
    </div>
  );
}
