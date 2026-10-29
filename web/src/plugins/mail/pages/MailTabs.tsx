import { TabBar } from "@octarq/plugin-sdk";
import { useTranslation } from "../../../i18n";

export type MailTab = "mail" | "inbound" | "smtp" | "suppressions";

interface MailTabsProps {
  tab: MailTab;
  onTabChange: (tab: MailTab) => void;
}

export function MailTabs({ tab, onTabChange }: MailTabsProps) {
  const { t } = useTranslation();

  return (
    <TabBar
      className="mb-6"
      value={tab}
      onValueChange={(v) => onTabChange(v as MailTab)}
      items={[
        { value: "mail", label: t("mail.tabMail") },
        { value: "inbound", label: t("mail.tabInbound") },
        { value: "smtp", label: t("mail.tabSmtp") },
        { value: "suppressions", label: t("mail.tabSuppressions") },
      ]}
    />
  );
}
