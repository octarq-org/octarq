import { Button, TabBar } from "@octarq/plugin-sdk";
import { Inbox, Settings } from "lucide-react";
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

  const items = [
    {
      value: "all",
      label: t("mail.allMailboxes"),
      icon: <Inbox className="h-3.5 w-3.5" />,
      count: totalUnread > 0 ? totalUnread : undefined,
    },
    ...boxes.map((b) => ({
      value: String(b.id),
      label: <span className="font-mono">{b.address}</span>,
      count: b.unread > 0 ? b.unread : undefined,
    })),
  ];

  return (
    <div className="flex flex-wrap items-center justify-between gap-3">
      <div className="min-w-0 flex-1">
        <TabBar
          size="sm"
          value={active === undefined ? "all" : String(active)}
          onValueChange={(v) => onSelect(v === "all" ? undefined : Number(v))}
          items={items}
        />
      </div>
      {activeBox && (
        <Button variant="ghost" size="sm" onClick={() => onEdit(activeBox)} title={t("mail.editMailbox")}>
          <Settings className="h-3.5 w-3.5" />
          <span>{t("mail.editMailbox")}</span>
        </Button>
      )}
    </div>
  );
}
