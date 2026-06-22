import { useEffect, useState } from "react";
import { api, Attachment, Domain, Email, Mailbox } from "../api";
import { Field, Modal, Toggle, timeAgo } from "../ui";
import { Header } from "./Links";

interface ReplyDraft {
  to: string;
  subject: string;
}

export default function MailPage() {
  const [boxes, setBoxes] = useState<Mailbox[]>([]);
  const [domains, setDomains] = useState<Domain[]>([]);
  const [active, setActive] = useState<number | undefined>(undefined);
  const [emails, setEmails] = useState<Email[]>([]);
  const [open, setOpen] = useState<Email | null>(null);
  const [newBox, setNewBox] = useState(false);
  const [editBox, setEditBox] = useState<Mailbox | null>(null);
  const [compose, setCompose] = useState<ReplyDraft | true | null>(null);

  const mailDomains = domains.filter((d) => d.forMail);

  async function loadBoxes() {
    setBoxes(await api.mailboxes());
  }
  async function loadEmails() {
    setEmails(await api.emails(active));
  }
  useEffect(() => {
    loadBoxes();
    api.domains().then(setDomains).catch(() => {});
  }, []);
  useEffect(() => {
    loadEmails();
  }, [active]);

  async function openEmail(e: Email) {
    const full = await api.email(e.id);
    setOpen(full);
    loadEmails();
    loadBoxes();
  }

  async function markAllRead() {
    await api.readAllEmails(active);
    loadEmails();
    loadBoxes();
  }

  return (
    <div>
      <Header title="Mail" subtitle="Domain mailboxes & inbox">
        <div className="flex gap-2">
          <button className="btn-ghost" onClick={markAllRead}>
            Mark all read
          </button>
          <button className="btn-ghost" onClick={() => setCompose(true)}>
            Compose
          </button>
          <button className="btn-primary" onClick={() => setNewBox(true)}>
            + Mailbox
          </button>
        </div>
      </Header>

      <div className="grid grid-cols-[200px_1fr] gap-4">
        {/* mailbox list */}
        <div className="card h-fit">
          <button
            className={`w-full px-3 py-2 text-left text-sm ${active === undefined ? "bg-zinc-800" : "hover:bg-zinc-900"}`}
            onClick={() => setActive(undefined)}
          >
            All mail
          </button>
          {boxes.map((b) => (
            <div
              key={b.id}
              className={`group flex items-center justify-between border-t border-zinc-800 px-3 py-2 text-sm ${
                active === b.id ? "bg-zinc-800" : "hover:bg-zinc-900"
              }`}
            >
              <button className="min-w-0 flex-1 text-left" onClick={() => setActive(b.id)}>
                <div className="flex items-center gap-1">
                  <span className={`truncate ${b.enabled ? "" : "text-zinc-600 line-through"}`}>{b.address}</span>
                  {b.unread > 0 && (
                    <span className="rounded-full bg-indigo-500 px-1.5 text-[10px] font-bold">{b.unread}</span>
                  )}
                </div>
                {b.note && <div className="truncate text-[10px] text-amber-300/70">📝 {b.note}</div>}
              </button>
              <button className="btn-ghost px-1 opacity-0 group-hover:opacity-100" onClick={() => setEditBox(b)}>
                ⚙
              </button>
            </div>
          ))}
        </div>

        {/* email list */}
        <div className="card divide-y divide-zinc-800">
          {emails.length === 0 ? (
            <div className="p-8 text-center text-zinc-500">No messages.</div>
          ) : (
            emails.map((e) => (
              <button
                key={e.id}
                className="flex w-full items-center gap-3 p-3 text-left hover:bg-zinc-900"
                onClick={() => openEmail(e)}
              >
                {!e.read && <span className="h-2 w-2 shrink-0 rounded-full bg-indigo-400" />}
                <div className="min-w-0 flex-1">
                  <div className="flex justify-between">
                    <span className={`truncate ${e.read ? "text-zinc-400" : "font-semibold"}`}>{e.from || "(unknown)"}</span>
                    <span className="shrink-0 text-xs text-zinc-500">{timeAgo(e.receivedAt)}</span>
                  </div>
                  <div className={`truncate text-sm ${e.read ? "text-zinc-500" : "text-zinc-300"}`}>
                    {e.subject || "(no subject)"}
                  </div>
                  {e.note && <div className="truncate text-xs text-amber-300/70">📝 {e.note}</div>}
                </div>
              </button>
            ))
          )}
        </div>
      </div>

      {open && (
        <EmailView
          email={open}
          onClose={() => setOpen(null)}
          onReply={(d) => {
            setOpen(null);
            setCompose(d);
          }}
          onChanged={() => {
            loadEmails();
            loadBoxes();
          }}
        />
      )}
      {newBox && (
        <MailboxEditor
          box={null}
          domains={mailDomains}
          onClose={() => setNewBox(false)}
          onSaved={() => {
            setNewBox(false);
            loadBoxes();
          }}
        />
      )}
      {editBox && (
        <MailboxEditor
          box={editBox}
          domains={mailDomains}
          onClose={() => setEditBox(null)}
          onSaved={() => {
            setEditBox(null);
            loadBoxes();
            loadEmails();
          }}
        />
      )}
      {compose && (
        <Compose draft={compose === true ? undefined : compose} onClose={() => setCompose(null)} />
      )}
    </div>
  );
}

function parseAttachments(json: string): Attachment[] {
  try {
    const a = JSON.parse(json || "[]");
    return Array.isArray(a) ? a : [];
  } catch {
    return [];
  }
}

function EmailView({
  email,
  onClose,
  onReply,
  onChanged,
}: {
  email: Email;
  onClose: () => void;
  onReply: (draft: ReplyDraft) => void;
  onChanged: () => void;
}) {
  const [note, setNote] = useState(email.note ?? "");
  const attachments = parseAttachments(email.attachments);
  return (
    <Modal title={email.subject || "(no subject)"} onClose={onClose} wide>
      <div className="mb-3 text-sm text-zinc-400">
        <div>
          <span className="text-zinc-500">From:</span> {email.from}
        </div>
        <div>
          <span className="text-zinc-500">To:</span> {email.to}
        </div>
        <div className="text-xs text-zinc-600">{new Date(email.receivedAt).toLocaleString()}</div>
      </div>
      <div className="card max-h-[50vh] overflow-y-auto p-4">
        {email.html ? (
          <iframe srcDoc={email.html} className="h-[45vh] w-full bg-white" sandbox="" title="email" />
        ) : (
          <pre className="whitespace-pre-wrap break-words font-sans text-sm text-zinc-300">{email.text}</pre>
        )}
      </div>
      {attachments.length > 0 && (
        <div className="mt-2 flex flex-wrap gap-2">
          {attachments.map((a, i) => (
            <span key={i} className="badge" title={`${a.contentType} · ${a.size} bytes`}>
              📎 {a.filename || "attachment"} ({Math.max(1, Math.round(a.size / 1024))} KB)
            </span>
          ))}
        </div>
      )}
      <div className="mt-3 flex items-end gap-2">
        <div className="flex-1">
          <Field label="Note">
            <input className="input" value={note} onChange={(e) => setNote(e.target.value)} />
          </Field>
        </div>
        <button
          className="btn-ghost"
          onClick={async () => {
            await api.updateEmail(email.id, { note });
            onChanged();
          }}
        >
          Save note
        </button>
        <button
          className="btn-primary"
          onClick={() =>
            onReply({
              to: email.from,
              subject: email.subject.startsWith("Re:") ? email.subject : `Re: ${email.subject}`,
            })
          }
        >
          Reply
        </button>
        <a className="btn-ghost" href={api.rawEmailUrl(email.id)} download={`email-${email.id}.eml`}>
          .eml
        </a>
        <button
          className="btn-danger"
          onClick={async () => {
            if (confirm("Delete this email?")) {
              await api.deleteEmail(email.id);
              onChanged();
              onClose();
            }
          }}
        >
          Delete
        </button>
      </div>
    </Modal>
  );
}

function MailboxEditor({
  box,
  domains,
  onClose,
  onSaved,
}: {
  box: Mailbox | null;
  domains: Domain[];
  onClose: () => void;
  onSaved: () => void;
}) {
  const [prefix, setPrefix] = useState("");
  const [domain, setDomain] = useState(domains[0]?.name ?? "");
  const [note, setNote] = useState(box?.note ?? "");
  const [enabled, setEnabled] = useState(box?.enabled ?? true);
  const [err, setErr] = useState("");

  async function save() {
    setErr("");
    try {
      if (box) {
        await api.updateMailbox(box.id, { note, enabled });
      } else {
        if (!prefix.trim() || !domain) {
          setErr("prefix and domain are required");
          return;
        }
        await api.createMailbox({ address: `${prefix.trim()}@${domain}`, note, enabled });
      }
      onSaved();
    } catch (e: any) {
      setErr(e.message ?? "save failed");
    }
  }

  return (
    <Modal title={box ? "Edit mailbox" : "New mailbox"} onClose={onClose}>
      {box ? (
        <Field label="Address">
          <input className="input" value={box.address} disabled />
        </Field>
      ) : domains.length === 0 ? (
        <p className="mb-3 rounded bg-amber-500/10 p-2 text-sm text-amber-300">
          No mail-enabled domains. Add a domain and toggle <b>Accept email</b> first.
        </p>
      ) : (
        <Field label="Address" hint="pick a mail-enabled domain">
          <div className="flex items-center gap-1">
            <input
              className="input"
              value={prefix}
              onChange={(e) => setPrefix(e.target.value)}
              placeholder="hi"
              autoFocus
            />
            <span className="text-zinc-500">@</span>
            <select className="input" value={domain} onChange={(e) => setDomain(e.target.value)}>
              {domains.map((d) => (
                <option key={d.id} value={d.name}>
                  {d.name}
                </option>
              ))}
            </select>
          </div>
        </Field>
      )}
      <Field label="Note">
        <textarea className="input" rows={2} value={note} onChange={(e) => setNote(e.target.value)} />
      </Field>
      <label className="mb-4 flex items-center gap-2 text-sm text-zinc-400">
        <Toggle on={enabled} onChange={setEnabled} /> Enabled
      </label>
      {box && (
        <button
          className="btn-danger mb-3"
          onClick={async () => {
            if (confirm(`Delete mailbox ${box.address} and all its mail?`)) {
              await api.deleteMailbox(box.id);
              onSaved();
            }
          }}
        >
          Delete mailbox
        </button>
      )}
      {err && <p className="mb-3 text-sm text-red-400">{err}</p>}
      <div className="flex justify-end gap-2">
        <button className="btn-ghost" onClick={onClose}>
          Cancel
        </button>
        <button className="btn-primary" onClick={save}>
          Save
        </button>
      </div>
    </Modal>
  );
}

function Compose({ draft, onClose }: { draft?: ReplyDraft; onClose: () => void }) {
  const [to, setTo] = useState(draft?.to ?? "");
  const [from, setFrom] = useState("");
  const [subject, setSubject] = useState(draft?.subject ?? "");
  const [text, setText] = useState("");
  const [err, setErr] = useState("");
  const [ok, setOk] = useState(false);

  async function send() {
    setErr("");
    try {
      await api.sendEmail({ to: to.split(",").map((s) => s.trim()).filter(Boolean), from, subject, text });
      setOk(true);
    } catch (e: any) {
      setErr(e.message ?? "send failed");
    }
  }

  return (
    <Modal title="Compose" onClose={onClose}>
      {ok ? (
        <div className="py-6 text-center">
          <p className="mb-3 text-green-400">✓ Sent</p>
          <button className="btn-primary" onClick={onClose}>
            Done
          </button>
        </div>
      ) : (
        <>
          <Field label="From" hint="blank = configured SMTP sender">
            <input className="input" value={from} onChange={(e) => setFrom(e.target.value)} />
          </Field>
          <Field label="To" hint="comma-separated">
            <input className="input" value={to} onChange={(e) => setTo(e.target.value)} />
          </Field>
          <Field label="Subject">
            <input className="input" value={subject} onChange={(e) => setSubject(e.target.value)} />
          </Field>
          <Field label="Body">
            <textarea className="input" rows={6} value={text} onChange={(e) => setText(e.target.value)} />
          </Field>
          {err && <p className="mb-3 text-sm text-red-400">{err}</p>}
          <div className="flex justify-end gap-2">
            <button className="btn-ghost" onClick={onClose}>
              Cancel
            </button>
            <button className="btn-primary" onClick={send}>
              Send
            </button>
          </div>
        </>
      )}
    </Modal>
  );
}
