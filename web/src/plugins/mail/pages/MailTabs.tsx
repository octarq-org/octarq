import { useTranslation } from "../../../i18n";

export type MailTab = "mail" | "inbound" | "smtp" | "suppressions";

interface MailTabsProps {
  tab: MailTab;
  onTabChange: (tab: MailTab) => void;
}

export function MailTabs({ tab, onTabChange }: MailTabsProps) {
  const { t } = useTranslation();

  return (
    <div className="flex gap-0 border-b border-foreground/[0.06] mb-6 overflow-x-auto [scrollbar-width:none] [-ms-overflow-style:none] [&::-webkit-scrollbar]:hidden">
      <button
        type="button"
        onClick={() => onTabChange("mail")}
        className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors shrink-0 whitespace-nowrap cursor-pointer ${
          tab === "mail"
            ? "border-primary text-foreground font-semibold"
            : "border-transparent text-foreground/45 hover:text-foreground/70"
        }`}
      >
        {t("mail.tabMail")}
      </button>
      <button
        type="button"
        onClick={() => onTabChange("inbound")}
        className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors shrink-0 whitespace-nowrap cursor-pointer ${
          tab === "inbound"
            ? "border-primary text-foreground font-semibold"
            : "border-transparent text-foreground/45 hover:text-foreground/70"
        }`}
      >
        {t("mail.tabInbound")}
      </button>
      <button
        type="button"
        onClick={() => onTabChange("smtp")}
        className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors shrink-0 whitespace-nowrap cursor-pointer ${
          tab === "smtp"
            ? "border-primary text-foreground font-semibold"
            : "border-transparent text-foreground/45 hover:text-foreground/70"
        }`}
      >
        {t("mail.tabSmtp")}
      </button>
      <button
        type="button"
        onClick={() => onTabChange("suppressions")}
        className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors shrink-0 whitespace-nowrap cursor-pointer ${
          tab === "suppressions"
            ? "border-primary text-foreground font-semibold"
            : "border-transparent text-foreground/45 hover:text-foreground/70"
        }`}
      >
        {t("mail.tabSuppressions")}
      </button>
    </div>
  );
}
