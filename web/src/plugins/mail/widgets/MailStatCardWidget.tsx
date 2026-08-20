import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api, Overview } from "../../../api";
import { useTranslation } from "../../../i18n";
import { StatCard } from "../../../ui";
import { Mail } from "lucide-react";

export default function MailStatCardWidget() {
  const [o, setO] = useState<Overview | null>(null);
  const nav = useNavigate();
  const { t } = useTranslation();

  useEffect(() => {
    api.overview().then(setO).catch(() => {});
  }, []);

  if (!o || o.mailboxes === undefined) return null;

  return (
    <StatCard
      label={t("overview.mailboxes")}
      value={(o.mailboxes ?? 0).toLocaleString()}
      delta={t("overview.unread", { count: o.unread ?? 0 })}
      icon={<Mail className="h-4 w-4" />}
      onClick={() => nav("/mail")}
      index={2}
    />
  );
}
