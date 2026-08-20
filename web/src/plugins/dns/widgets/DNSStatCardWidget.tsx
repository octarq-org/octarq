import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api, Overview } from "../../../api";
import { useTranslation } from "../../../i18n";
import { StatCard } from "../../../ui";
import { Globe } from "lucide-react";

export default function DNSStatCardWidget() {
  const [o, setO] = useState<Overview | null>(null);
  const nav = useNavigate();
  const { t } = useTranslation();

  useEffect(() => {
    api.overview().then(setO).catch(() => {});
  }, []);

  if (!o || o.domains === undefined) return null;

  return (
    <StatCard
      label={t("overview.domains")}
      value={(o.domains ?? 0).toLocaleString()}
      delta={t("overview.domainsDelta", { link: o.linkDomains ?? 0, mail: o.mailDomains ?? 0 })}
      icon={<Globe className="h-4 w-4" />}
      onClick={() => nav("/domains")}
      index={3}
    />
  );
}
