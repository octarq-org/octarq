import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api, useOverviewData } from "../../../api";
import { useTranslation } from "../../../i18n";
import { SetupStep } from "../../../app-ui/SetupStep";

export default function MailSetupStepWidget() {
  const o = useOverviewData();
  const [smtpCount, setSmtpCount] = useState<number | null>(null);
  const nav = useNavigate();
  const { t } = useTranslation();

  useEffect(() => {
    if (!o || o.mailboxes === undefined) return;
    let cancelled = false;
    (async () => {
      try {
        // The Integrations Hub owns mail senders when it is mounted. Count the
        // same registry the mail pages offer, not just the local table.
        const res = await fetch("/api/integrations?provider_type=communication", {
          credentials: "same-origin",
        });
        if (res.status === 404) {
          const list = await api.smtpSenders();
          if (!cancelled) setSmtpCount(list.length);
          return;
        }
        if (!res.ok) return;
        const data = await res.json();
        const list = (Array.isArray(data) ? data : []).filter(
          (item: any) => item.provider === "smtp" || item.provider === "resend"
        );
        if (!cancelled) setSmtpCount(list.length);
      } catch {
        // Leave the step in the unknown state instead of reporting zero.
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [o]);

  if (!o || o.mailboxes === undefined) return null;

  const completed = smtpCount !== null && smtpCount > 0;

  return (
    <SetupStep
      title={t("overview.stepSmtpTitle")}
      description={t("overview.stepSmtpDesc")}
      completed={completed}
      onClick={() => nav("/mail?tab=settings")}
    />
  );
}
