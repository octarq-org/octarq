import { Inbox, Send, FileText, Trash2, ShieldAlert } from "lucide-react";
import { useTranslation } from "../../../i18n";
import { MailFolder } from "./types";

interface FolderNavProps {
  currentFolder: MailFolder;
  onSelectFolder: (folder: MailFolder) => void;
}

export function FolderNav({ currentFolder, onSelectFolder }: FolderNavProps) {
  const { t } = useTranslation();

  const folders: { key: MailFolder; label: string; icon: any }[] = [
    { key: "inbox", label: t("mail.folderInbox"), icon: Inbox },
    { key: "sent", label: t("mail.folderSent"), icon: Send },
    { key: "drafts", label: t("mail.folderDrafts"), icon: FileText },
    { key: "trash", label: t("mail.folderTrash"), icon: Trash2 },
    { key: "spam", label: t("mail.folderSpam"), icon: ShieldAlert },
  ];

  return (
    <div className="flex items-center gap-1 overflow-x-auto pb-1 [scrollbar-width:none] [-ms-overflow-style:none] [&::-webkit-scrollbar]:hidden">
      {folders.map((f) => {
        const Icon = f.icon;
        const active = currentFolder === f.key;
        return (
          <button
            key={f.key}
            type="button"
            onClick={() => onSelectFolder(f.key)}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-xl text-xs font-medium transition-colors cursor-pointer shrink-0 ${
              active
                ? "bg-primary text-primary-fg shadow-sm font-semibold"
                : "text-foreground/70 hover:text-foreground hover:bg-foreground/[0.05]"
            }`}
          >
            <Icon className="h-3.5 w-3.5" />
            <span>{f.label}</span>
          </button>
        );
      })}
    </div>
  );
}
