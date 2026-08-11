import { useNavigate } from "react-router-dom";
import { useTranslation } from "../../../i18n";
import { Panel, timeAgo } from "../../../ui";
import { useOverviewData } from "../../../api";

export default function RecentMailPanelWidget() {
  const o = useOverviewData();
  const nav = useNavigate();
  const { t } = useTranslation();

  if (!o || o.recentEmails === undefined) return null;

  const recentEmails = o.recentEmails as { id: number; from: string; subject: string; read: boolean; receivedAt: string }[] | null;

  return (
    <Panel title={t("mail.recentMail")}>
      {!recentEmails || recentEmails.length === 0 ? (
        <p className="text-sm text-foreground/50">{t("mail.noMail")}</p>
      ) : (
        <div className="divide-y divide-foreground/[0.04]">
          {recentEmails.map((e) => (
            <button
              key={e.id}
              onClick={() => nav("/mail")}
              className="flex w-full items-center gap-3 px-3 py-2.5 text-left hover:bg-foreground/[0.06] transition-colors"
            >
              {!e.read && <span className="h-2 w-2 shrink-0 rounded-full bg-indigo-400" />} /* ui-color-ok */
              <span className={`w-40 shrink-0 truncate text-sm ${e.read ? "text-foreground/55" : "font-semibold"}`}>
                {e.from || t("mail.unknownSender")}
              </span>
              <span className="flex-1 truncate text-sm text-foreground/55">{e.subject || t("mail.noSubject")}</span>
              <span className="shrink-0 text-xs text-foreground/40">{timeAgo(e.receivedAt)}</span>
            </button>
          ))}
        </div>
      )}
    </Panel>
  );
}
