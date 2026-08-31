import { Search, X, Mail as MailIcon, Inbox } from "lucide-react";
import { GlassCard, Empty, Button } from "../../../ui";
import { ListSkeleton } from "../../../components/ListSkeleton";
import { Email, Mailbox } from "../api";
import { MailFolder } from "./types";
import { EmailListItem } from "./EmailListItem";
import { useTranslation } from "../../../i18n";

interface EmailListPaneProps {
  emails: Email[];
  loading: boolean;
  folder: MailFolder;
  q: string;
  onSearchChange: (q: string) => void;
  openEmailId?: number;
  onOpenEmail: (e: Email) => void;
  onScroll: (e: React.UIEvent<HTMLDivElement>) => void;
  boxes: Mailbox[];
  activeMailboxId?: number;
  onNewBox: () => void;
}

export function EmailListPane({
  emails,
  loading,
  folder,
  q,
  onSearchChange,
  openEmailId,
  onOpenEmail,
  onScroll,
  boxes,
  activeMailboxId,
  onNewBox,
}: EmailListPaneProps) {
  const { t } = useTranslation();

  return (
    <div className="flex flex-col min-h-0 w-full">
      <div className="mb-3 relative">
        <input
          className="input w-full !pl-8 text-sm"
          placeholder={t("mail.searchPlaceholder")}
          value={q}
          onChange={(e) => onSearchChange(e.target.value)}
        />
        <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-foreground/45 pointer-events-none" />
        {q && (
          <button
            type="button"
            onClick={() => onSearchChange("")}
            className="absolute right-2.5 top-2.5 text-foreground/40 hover:text-foreground p-0.5 cursor-pointer"
          >
            <X className="h-3.5 w-3.5" />
          </button>
        )}
      </div>

      <GlassCard className="overflow-hidden">
        {emails.length === 0 && loading ? (
          <ListSkeleton rows={7} ariaLabel={t("mail.loading")} />
        ) : (
          <div className="overflow-y-auto max-h-[650px] divide-y divide-foreground/[0.04]" onScroll={onScroll}>
            {emails.length === 0 ? (
              q ? (
                <div className="flex flex-col items-center gap-3 px-4 py-8 text-center">
                  <p className="text-sm text-foreground/60">
                    {t("mail.emptyFilteredReason")} <span className="font-mono text-foreground/80">{`“${q}”`}</span>
                  </p>
                  <Button variant="ghost" className="text-xs py-1.5" onClick={() => onSearchChange("")}>
                    {t("mail.clearSearch")}
                  </Button>
                </div>
              ) : boxes.length === 0 ? (
                <Empty
                  reason={t("mail.emptyNoBoxesReason")}
                  detail={t("mail.emptyNoBoxesDetail")}
                  action={
                    <Button variant="primary" className="mt-1 text-xs py-1.5" onClick={onNewBox}>
                      {t("mail.newMailbox")}
                    </Button>
                  }
                >
                  <MailIcon className="h-8 w-8 text-foreground/50 mb-1" />
                </Empty>
              ) : (
                <Empty
                  reason={
                    <>
                      {t("mail.emptyInboxReasonPre")}{" "}
                      <span className="font-mono">
                        {`“${boxes.find((b) => b.id === activeMailboxId)?.address ?? boxes[0]?.address}”`}
                      </span>
                    </>
                  }
                  detail={t("mail.emptyInboxDetail")}
                >
                  <Inbox className="h-8 w-8 text-foreground/50 mb-1" />
                </Empty>
              )
            ) : (
              <>
                {emails.map((e) => (
                  <EmailListItem
                    key={e.id}
                    email={e}
                    folder={folder}
                    isSelected={openEmailId === e.id}
                    onSelect={() => onOpenEmail(e)}
                  />
                ))}
                {loading && <div className="p-3 text-center text-xs text-foreground/40">{t("mail.loading")}</div>}
              </>
            )}
          </div>
        )}
      </GlassCard>
    </div>
  );
}
