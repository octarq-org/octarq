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
      className={`flex w-full flex-col border-l-2 p-3.5 text-left transition-colors duration-[var(--dur-fast)] cursor-pointer ${
        isSelected ? "border-primary bg-accent-soft" : "border-transparent hover:bg-well"
      }`}
      onClick={onSelect}
    >
      <div className="mb-1 flex w-full items-center justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          {!email.read && <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-primary" />}
          <span className={`truncate text-[13px] ${email.read ? "text-muted-foreground" : "font-semibold text-foreground"}`}>
            {displayAddress}
          </span>
          {catMeta && (
            <Badge tone={catMeta.tone} className="shrink-0 px-1.5 py-0 text-[10px] font-medium uppercase">
              {t(`mail.cat_${email.category}`, catMeta.label)}
            </Badge>
          )}
        </div>
        <div className="ml-2 flex shrink-0 items-center gap-1.5">
          {renderImportanceDots(email.importance)}
          {!isOutbound && <AuthBadges spf={email.authSpf} dkim={email.authDkim} dmarc={email.authDmarc} compact />}
          <span className="font-mono text-[10.5px] text-muted-foreground">{timeAgo(email.receivedAt)}</span>
        </div>
      </div>
      <div className={`truncate text-xs ${email.read ? "text-muted-foreground" : "font-medium text-foreground"}`}>
        {email.subject || t("mail.noSubject")}
      </div>
      {email.summary && email.summary !== email.subject && (
        <p className="mt-1 line-clamp-1 text-[11px] text-muted-foreground">
          {email.summary}
        </p>
      )}
    </button>
  );
}
