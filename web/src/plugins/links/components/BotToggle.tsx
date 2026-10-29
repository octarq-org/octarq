import { useTranslation } from "../../../i18n";

export function BotToggle({ value, onChange }: { value: boolean; onChange: (v: boolean) => void }) {
  const { t } = useTranslation();
  return (
    <button
      onClick={() => onChange(!value)}
      title={value ? t("links.hideBotTraffic") : t("links.showBotTraffic")}
      className={`flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium transition ${
        value
          ? "bg-warning-bg text-warning-fg border border-warning-border hover:brightness-95"
          : "bg-surface-hover text-muted-foreground hover:bg-surface-hover"
      }`}
    >
      <span>{value ? t("links.botsOn") : t("links.botsOff")}</span>
    </button>
  );
}
