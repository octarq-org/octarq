import { Inbox, Settings } from "lucide-react";
import { Button } from "../../../ui";
import { Mailbox } from "../api";
import { useTranslation } from "../../../i18n";

interface MailboxBarProps {
  boxes: Mailbox[];
  active?: number;
  totalUnread: number;
  activeBox?: Mailbox;
  onSelect: (id?: number) => void;
  onEdit: (box: Mailbox) => void;
}

export function MailboxBar({
  boxes,
  active,
  totalUnread,
  activeBox,
  onSelect,
  onEdit,
}: MailboxBarProps) {
  const { t } = useTranslation();

  if (boxes.length === 0) return null;

  return (
    <div className="flex flex-wrap items-center justify-between gap-3 bg-well p-2 rounded-2xl border border-foreground/[0.05]">
      <div className="flex items-center gap-1.5 flex-wrap flex-1">
        <button
          type="button"
          onClick={() => onSelect(undefined)}
          className={`flex items-center gap-1.5 px-3 py-1.5 rounded-xl text-xs font-medium transition-colors cursor-pointer ${
            active === undefined
              ? "bg-accent text-accent-fg shadow-sm font-semibold"
              : "text-foreground/60 hover:text-foreground hover:bg-foreground/[0.03]"
          }`}
        >
          <Inbox className="h-3.5 w-3.5" />
          <span>{t("mail.allMailboxes")}</span>
          {totalUnread > 0 && (
            <span className={`text-[10px] px-1.5 py-0.2 rounded-full ${active === undefined ? "bg-accent-fg/20 text-accent-fg" : "bg-primary text-primary-fg"}`}>
              {totalUnread}
            </span>
          )}
        </button>
        {boxes.map((b) => (
          <button
            key={b.id}
            type="button"
            onClick={() => onSelect(b.id)}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-xl text-xs font-mono transition-colors cursor-pointer ${
              active === b.id
                ? "bg-accent text-accent-fg shadow-sm font-semibold"
                : "text-foreground/70 hover:text-foreground hover:bg-foreground/[0.03]"
            }`}
          >
            <span>{b.address}</span>
            {b.unread > 0 && (
              <span className={`text-[10px] px-1.5 py-0.2 rounded-full ${active === b.id ? "bg-accent-fg/20 text-accent-fg" : "bg-primary text-primary-fg font-sans font-bold"}`}>
                {b.unread}
              </span>
            )}
          </button>
        ))}
      </div>
      {activeBox && (
        <Button
          variant="subtle"
          className="text-xs py-1 px-2.5 gap-1 shrink-0"
          onClick={() => onEdit(activeBox)}
          title={t("mail.editMailbox")}
        >
          <Settings className="h-3.5 w-3.5" />
          <span>{t("mail.editMailbox")}</span>
        </Button>
      )}
    </div>
  );
}
