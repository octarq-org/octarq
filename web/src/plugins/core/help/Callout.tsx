import type { ReactNode } from "react";
import { AlertCircle, AlertTriangle, Info, Lightbulb } from "lucide-react";
import { useTranslation } from "../../../i18n";

export type CalloutType = "tip" | "warning" | "important" | "note";

const CALLOUTS = {
  tip: { key: "help.callout_tip", fallback: "TIP", Icon: Lightbulb },
  warning: { key: "help.callout_warning", fallback: "WARNING", Icon: AlertTriangle },
  important: { key: "help.callout_important", fallback: "IMPORTANT", Icon: AlertCircle },
  note: { key: "help.callout_note", fallback: "NOTE", Icon: Info },
} as const;

export function Callout({ type, children }: { type: CalloutType; children: ReactNode }) {
  const { t } = useTranslation();
  const { key, fallback, Icon } = CALLOUTS[type];

  return (
    <div className={`callout callout-${type}`}>
      <div className="callout-title">
        <Icon className="h-4 w-4 shrink-0" aria-hidden="true" />
        <span>{t(key, fallback)}</span>
      </div>
      <div className="callout-content">{children}</div>
    </div>
  );
}
