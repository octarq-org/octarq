import { Inbox, Send, FileText, Trash2, ShieldAlert } from "lucide-react";
import { TabBar } from "@octarq/plugin-sdk";
import { useTranslation } from "../../../i18n";
import { MailFolder } from "./types";

interface FolderNavProps {
  currentFolder: MailFolder;
  onSelectFolder: (folder: MailFolder) => void;
}

export function FolderNav({ currentFolder, onSelectFolder }: FolderNavProps) {
  const { t } = useTranslation();

  const folders = [
    { value: "inbox", label: t("mail.folderInbox"), icon: <Inbox className="h-3.5 w-3.5" /> },
    { value: "sent", label: t("mail.folderSent"), icon: <Send className="h-3.5 w-3.5" /> },
    { value: "drafts", label: t("mail.folderDrafts"), icon: <FileText className="h-3.5 w-3.5" /> },
    { value: "trash", label: t("mail.folderTrash"), icon: <Trash2 className="h-3.5 w-3.5" /> },
    { value: "spam", label: t("mail.folderSpam"), icon: <ShieldAlert className="h-3.5 w-3.5" /> },
  ];

  return (
    <TabBar
      size="sm"
      value={currentFolder}
      onValueChange={(v) => onSelectFolder(v as MailFolder)}
      items={folders}
    />
  );
}
