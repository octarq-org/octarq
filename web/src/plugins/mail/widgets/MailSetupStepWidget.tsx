import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api, Overview } from "../../../api";
import { useTranslation } from "../../../i18n";
import { SetupStep } from "../../../components/SetupStep";

export default function MailSetupStepWidget() {
  const [o, setO] = useState<Overview | null>(null);
  const [smtpCount, setSmtpCount] = useState<number | null>(null);
  const nav = useNavigate();
  const { t } = useTranslation();

  useEffect(() => {
    api.overview().then(setO).catch(() => {});
    api.smtpSenders().then(s => setSmtpCount(s.length)).catch(() => {});
  }, []);

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
