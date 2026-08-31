import { timeAgo } from "../../../ui";
import { Email } from "../api";
import { AuthBadges } from "./AuthBadges";
import { useTranslation } from "../../../i18n";
import { MailFolder } from "./types";

interface EmailListItemProps {
  email: Email;
  folder: MailFolder;
  isSelected: boolean;
  onSelect: () => void;
}

export function EmailListItem({
  email,
  folder,
  isSelected,
  onSelect,
}: EmailListItemProps) {
  const { t } = useTranslation();

  const isOutbound = folder === "sent" || folder === "drafts";
  const displayAddress = isOutbound
    ? (email.to ? `${t("mail.toLabel")} ${email.to}` : t("mail.noSubject"))
    : (email.from || t("mail.unknownSender"));

  return (
    <button
      type="button"
      className={`flex w-full flex-col p-4 text-left hover:bg-foreground/[0.03] transition-colors cursor-pointer ${
        isSelected ? "bg-foreground/[0.06] border-l-2 border-primary" : ""
      }`}
      onClick={onSelect}
    >
      <div className="flex items-center justify-between w-full mb-1 gap-2">
        <div className="flex items-center gap-2 min-w-0">
          {!email.read && <span className="h-2 w-2 shrink-0 rounded-full bg-primary" />}
          <span className={`truncate text-sm ${email.read ? "text-foreground/60" : "font-bold text-foreground"}`}>
            {displayAddress}
          </span>
        </div>
        <div className="flex items-center gap-1.5 shrink-0 ml-2">
          {!isOutbound && <AuthBadges spf={email.authSpf} dkim={email.authDkim} dmarc={email.authDmarc} compact />}
          <span className="text-[11px] text-foreground/40">{timeAgo(email.receivedAt)}</span>
        </div>
      </div>
      <div className={`truncate text-xs ${email.read ? "text-foreground/45" : "text-foreground/80 font-medium"}`}>
        {email.subject || t("mail.noSubject")}
      </div>
    </button>
  );
}
