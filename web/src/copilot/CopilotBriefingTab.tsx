import React from "react";
import { useTranslation } from "../i18n";

export function CopilotBriefingTab() {
  const { t } = useTranslation();

  return (
    <div
      className="flex flex-1 flex-col overflow-y-auto p-4"
      data-testid="copilot-briefing-tab"
    >
      <div className="flex flex-1 flex-col items-center justify-center rounded-2xl border border-dashed border-border p-6 text-center">
        <p className="text-sm font-semibold text-foreground">
          {t("copilot.briefingEmptyTitle")}
        </p>
        <p className="mt-1 max-w-sm text-xs text-muted-foreground">
          {t("copilot.briefingEmptyDesc")}
        </p>
      </div>
    </div>
  );
}
