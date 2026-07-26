import { useNavigate } from "react-router-dom";
import { useTranslation } from "../../../i18n";
import { Panel } from "../../../ui";
import { useOverviewData } from "../../../api";

export default function TopLinksPanelWidget() {
  const o = useOverviewData();
  const nav = useNavigate();
  const { t } = useTranslation();

  if (!o || o.topLinks === undefined) return null;

  const topLinks = o.topLinks as { id: number; slug: string; host: string; clicks: number }[] | null;

  return (
    <Panel title={t("links.topLinks")}>
      {!topLinks || topLinks.length === 0 ? (
        <p className="text-sm text-foreground/50">{t("links.noLinks")}</p>
      ) : (
        <div className="space-y-1">
          {topLinks.map((l) => (
            <button
              key={l.id}
              onClick={() => nav("/links")}
              className="flex w-full items-center justify-between rounded-xl px-3 py-2 text-left text-sm hover:bg-foreground/[0.06] transition-colors"
            >
              <span className="truncate text-accent-fg">
                /{l.slug}
                {l.host && <span className="text-foreground/40"> @{l.host}</span>}
              </span>
              <span className="shrink-0 font-semibold">{l.clicks}</span>
            </button>
          ))}
        </div>
      )}
    </Panel>
  );
}
