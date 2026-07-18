// Mail feature API surface — the slice of the JSON API owned by the mail plugin.
// SMTP-sender management stays in core (shared with Settings and Overview); the
// cross-feature `Domain` model and its host helpers live in the core api module.
import { req } from "../../api";

export interface Mailbox {
  id: number;
  address: string;
  note: string;
  enabled: boolean;
  unread: number;
}

export interface Attachment {
  filename: string;
  contentType: string;
  size: number;
}

export interface Email {
  id: number;
  mailboxId: number;
  from: string;
  to: string;
  subject: string;
  text: string;
  html: string;
  read: boolean;
  note: string;
  attachments: string; // JSON string of Attachment[]
  authSpf: string;   // pass|fail|softfail|neutral|none|""
  authDkim: string;  // pass|fail|none|""
  authDmarc: string; // pass|fail|none|""
  receivedAt: string;
}

export const mailApi = {
  mailboxes: () => req<Mailbox[]>("GET", "/api/mailboxes"),
  createMailbox: (m: Partial<Mailbox>) => req<Mailbox>("POST", "/api/mailboxes", m),
  updateMailbox: (id: number, m: Partial<Mailbox>) => req<Mailbox>("PUT", `/api/mailboxes/${id}`, m),
  deleteMailbox: (id: number) => req("DELETE", `/api/mailboxes/${id}`),
  emails: (mailboxId?: number, params?: { q?: string; limit?: number; offset?: number }) => {
    const sp = new URLSearchParams();
    if (mailboxId) sp.set("mailbox", mailboxId.toString());
    if (params?.q) sp.set("q", params.q);
    if (params?.limit) sp.set("limit", params.limit.toString());
    if (params?.offset) sp.set("offset", params.offset.toString());
    const qs = sp.toString();
    return req<Email[]>("GET", `/api/emails${qs ? "?" + qs : ""}`);
  },
  email: (id: number) => req<Email>("GET", `/api/emails/${id}`),
  updateEmail: (id: number, e: { read?: boolean; note?: string }) =>
    req<Email>("PUT", `/api/emails/${id}`, e),
  deleteEmail: (id: number) => req("DELETE", `/api/emails/${id}`),
  readAllEmails: (mailbox?: number) =>
    req<{ ok: boolean; updated: number }>(
      "POST",
      `/api/emails/read-all${mailbox ? `?mailbox=${mailbox}` : ""}`,
    ),
  rawEmailUrl: (id: number) => `/api/emails/${id}/raw`,
  sendEmail: (m: { from?: string; to: string[]; subject: string; text?: string; html?: string; smtpSenderId?: number; trackLinks?: boolean }) =>
    req("POST", "/api/emails/send", m),
};
