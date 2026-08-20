import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api, Overview } from "../../../api";
import { useTranslation } from "../../../i18n";
import { StatCard } from "../../../ui";
import { Link2, MousePointerClick } from "lucide-react";

export default function LinksStatCardWidget() {
  const [o, setO] = useState<Overview | null>(null);
  const nav = useNavigate();
  const { t } = useTranslation();

  useEffect(() => {
    api.overview().then(setO).catch(() => {});
  }, []);

  if (!o || (o.links === undefined && o.totalClicks === undefined)) return null;

  return (
    <>
      {o.totalClicks !== undefined && (
        <StatCard
          label={t("overview.totalClicks")}
          value={(o.totalClicks ?? 0).toLocaleString()}
          delta={t("overview.clicks7d", { count: o.clicks7d ?? 0 })}
          icon={<MousePointerClick className="h-4 w-4" />}
          onClick={() => nav("/links")}
          index={0}
        />
      )}
      {o.links !== undefined && (
        <StatCard
          label={t("overview.shortLinks")}
          value={(o.links ?? 0).toLocaleString()}
          delta={t("overview.activeLinks", { count: o.activeLinks ?? 0 })}
          icon={<Link2 className="h-4 w-4" />}
          onClick={() => nav("/links")}
          index={1}
        />
      )}
    </>
  );
}
