import { Badge, timeAgo } from "@octarq/plugin-sdk";
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

const CATEGORY_META: Record<string, { tone: "amber" | "cyan" | "red" | "neutral"; label: string }> = {
  bill: { tone: "amber", label: "Bill" },
  otp: { tone: "cyan", label: "OTP" },
  marketing: { tone: "neutral", label: "Marketing" },
  important: { tone: "red", label: "Important" },
};

function renderImportanceDots(n?: number) {
  if (!n || n <= 0) return null;
  const clamped = Math.max(1, Math.min(5, n));
  return (
    <span
      className="text-[10px] tracking-wider text-warning-fg/80"
      title={`Importance: ${clamped}/5`}
      data-testid="importance-dots"
    >
      {"●".repeat(clamped)}{"○".repeat(5 - clamped)}
    </span>
  );
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

  const catMeta = email.category ? CATEGORY_META[email.category] : undefined;

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
          {catMeta && (
            <Badge tone={catMeta.tone} className="text-[10px] py-0 px-1.5 uppercase font-medium shrink-0">
              {t(`mail.cat_${email.category}`, catMeta.label)}
            </Badge>
          )}
        </div>
        <div className="flex items-center gap-1.5 shrink-0 ml-2">
          {renderImportanceDots(email.importance)}
          {!isOutbound && <AuthBadges spf={email.authSpf} dkim={email.authDkim} dmarc={email.authDmarc} compact />}
          <span className="text-[11px] text-foreground/40">{timeAgo(email.receivedAt)}</span>
        </div>
      </div>
      <div className={`truncate text-xs ${email.read ? "text-foreground/45" : "text-foreground/80 font-medium"}`}>
        {email.subject || t("mail.noSubject")}
      </div>
      {email.summary && email.summary !== email.subject && (
        <p className="mt-1 line-clamp-1 text-[11px] text-foreground/50">
          {email.summary}
        </p>
      )}
    </button>
  );
}
