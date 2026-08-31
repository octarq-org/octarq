export type MailFolder = "inbox" | "sent" | "drafts" | "trash" | "spam";

export interface ReplyDraft {
  id?: number;
  mailboxId?: number;
  to: string;
  subject: string;
  text?: string;
  html?: string;
}
