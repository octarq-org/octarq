import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api, useOverviewData } from "../../../api";
import { useTranslation } from "../../../i18n";
import { SetupStep } from "../../../components/SetupStep";

export default function MailSetupStepWidget() {
  const o = useOverviewData();
  const [smtpCount, setSmtpCount] = useState<number | null>(null);
  const nav = useNavigate();
  const { t } = useTranslation();

  useEffect(() => {
    if (!o || o.mailboxes === undefined) return;
    api.smtpSenders().then(s => setSmtpCount(s.length)).catch(() => {});
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
