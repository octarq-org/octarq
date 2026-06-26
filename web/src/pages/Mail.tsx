import { useEffect, useState } from "react";
import { api, Attachment, Domain, effectiveMailHosts, Email, Mailbox } from "../api";
import { Code, Field, Guide, Modal, Toggle, timeAgo } from "../ui";
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

  const [q, setQ] = useState("");
  const [page, setPage] = useState(0);
  const [hasMore, setHasMore] = useState(true);
  const [loading, setLoading] = useState(false);

  // Every mail host across all mail-enabled domains (incl. subdomains).
  const mailHostOptions = Array.from(new Set(domains.flatMap(effectiveMailHosts)));

  async function loadBoxes() {
    setBoxes(await api.mailboxes());
  }

  async function loadEmails(reset = false) {
    if (loading || (!hasMore && !reset)) return;
    setLoading(true);
    try {
      const limit = 50;
      const offset = reset ? 0 : page * limit;
      const res = await api.emails(active, { q, limit, offset });
      if (res.length < limit) setHasMore(false);
      else setHasMore(true);

      setEmails(prev => reset ? res : [...prev, ...res]);
      setPage(reset ? 1 : page + 1);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    loadBoxes();
    api.domains().then(setDomains).catch(() => {});
  }, []);

  useEffect(() => {
    const t = setTimeout(() => {
      loadEmails(true);
    }, 200);
    return () => clearTimeout(t);
  }, [active, q]);

  const handleScroll = (e: React.UIEvent<HTMLDivElement>) => {
    const bottom = e.currentTarget.scrollHeight - e.currentTarget.scrollTop <= e.currentTarget.clientHeight + 100;
    if (bottom) loadEmails();
  };

  async function openEmail(e: Email) {
    const full = await api.email(e.id);
    setOpen(full);
    loadEmails(true);
    loadBoxes();
  }

  async function markAllRead() {
    await api.readAllEmails(active);
    loadEmails(true);
    loadBoxes();
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
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

      {boxes.length === 0 && (
        <Guide title="Set up mail receiving with Cloudflare Email Routing" open>
          <ol className="ml-4 list-decimal space-y-1">
            <li>Add a domain in <b>Domains</b>, toggle <b>Accept email</b>, and list its mail hosts.</li>
            <li>In Cloudflare → <b>Email → Email Routing</b>, enable routing.</li>
            <li>Deploy <Code>deploy/cloudflare-email-worker.js</Code> with <Code>LED_ENDPOINT</Code>=<Code>{`${location.origin}/api/email/inbound`}</Code> and <Code>LED_TOKEN</Code> = your <Code>LED_INBOUND_TOKEN</Code>, then point a catch-all route at it.</li>
            <li>To send replies, configure an SMTP relay via Settings.</li>
          </ol>
        </Guide>
      )}

      <div className="grid grid-cols-[300px_1fr] gap-4 min-h-0 flex-1">
        {/* left column */}
        <div className="flex flex-col min-h-0">
          <div className="mb-2 flex items-center gap-2">
            <select
              className="input flex-1 min-w-0"
              value={active === undefined ? "" : active}
              onChange={(e) => setActive(e.target.value ? Number(e.target.value) : undefined)}
            >
              <option value="">All mail</option>
              {boxes.map(b => (
                <option key={b.id} value={b.id}>{b.address} {b.unread > 0 ? `(${b.unread})` : ""}</option>
              ))}
            </select>
            {active !== undefined && (
              <button
                className="btn-ghost shrink-0"
                title="Edit Mailbox"
                onClick={() => setEditBox(boxes.find(b => b.id === active) || null)}
              >
                ⚙
              </button>
            )}
          </div>
          <div className="mb-2">
            <input
              className="input w-full"
              placeholder="Search emails…"
              value={q}
              onChange={(e) => setQ(e.target.value)}
            />
          </div>
          <div className="card flex-1 overflow-y-auto" onScroll={handleScroll}>
            {emails.length === 0 && !loading ? (
              <div className="p-8 text-center text-zinc-500">No messages.</div>
            ) : (
              <div className="divide-y divide-zinc-800">
                {emails.map((e) => (
                  <button
                    key={e.id}
                    className={`flex w-full flex-col p-3 text-left hover:bg-zinc-900 transition-colors ${open?.id === e.id ? "bg-zinc-800" : ""}`}
                    onClick={() => openEmail(e)}
                  >
                    <div className="flex items-center justify-between w-full mb-1">
                      <div className="flex items-center gap-2 overflow-hidden">
                        {!e.read && <span className="h-2 w-2 shrink-0 rounded-full bg-indigo-400" />}
                        <span className={`truncate ${e.read ? "text-zinc-400" : "font-semibold"}`}>{e.from || "(unknown)"}</span>
                      </div>
                      <span className="shrink-0 text-xs text-zinc-500 ml-2">{timeAgo(e.receivedAt)}</span>
                    </div>
                    <div className={`truncate text-xs ${e.read ? "text-zinc-500" : "text-zinc-300"}`}>
                      {e.subject || "(no subject)"}
                    </div>
                  </button>
                ))}
                {loading && <div className="p-3 text-center text-xs text-zinc-500">Loading…</div>}
              </div>
            )}
          </div>
        </div>

        {/* right column */}
        <div className="min-h-0 overflow-y-auto pr-2 pb-8">
          {open ? (
            <EmailViewForm
              email={open}
              onClose={() => setOpen(null)}
              onReply={(d) => {
                setOpen(null);
                setCompose(d);
              }}
              onChanged={() => {
                loadEmails(true);
                loadBoxes();
              }}
            />
          ) : (
            <div className="flex h-full items-center justify-center text-zinc-500/50">
              Select an email to view details
            </div>
          )}
        </div>
      </div>

      {newBox && (
        <MailboxEditor
          box={null}
          hosts={mailHostOptions}
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
          hosts={mailHostOptions}
          onClose={() => setEditBox(null)}
          onSaved={() => {
            setEditBox(null);
            loadBoxes();
            loadEmails(true);
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

function EmailViewForm({
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
    <div className="card flex flex-col h-full max-h-full min-h-0">
      <div className="p-5 border-b border-zinc-800 flex justify-between items-start shrink-0">
         <div>
           <h2 className="text-xl font-semibold mb-2">{email.subject || "(no subject)"}</h2>
           <div className="text-sm text-zinc-400">
             <div><span className="text-zinc-500">From:</span> {email.from}</div>
             <div><span className="text-zinc-500">To:</span> {email.to}</div>
             <div className="text-xs text-zinc-600 mt-1">{new Date(email.receivedAt).toLocaleString()}</div>
           </div>
         </div>
         <div className="flex gap-2">
           <button className="btn-ghost px-2 text-sm" onClick={onClose}>✕</button>
         </div>
      </div>
      
      <div className="flex-1 overflow-y-auto p-5 min-h-0">
        {email.html ? (
          <iframe srcDoc={email.html} className="min-h-[400px] h-full w-full bg-white rounded" sandbox="" title="email" />
        ) : (
          <pre className="whitespace-pre-wrap break-words font-sans text-sm text-zinc-300">{email.text}</pre>
        )}
      </div>
      
      <div className="p-5 border-t border-zinc-800 shrink-0 bg-zinc-900/30">
        {attachments.length > 0 && (
          <div className="mb-4 flex flex-wrap gap-2">
            {attachments.map((a, i) => (
              <span key={i} className="badge bg-zinc-800 text-zinc-300" title={`${a.contentType} · ${a.size} bytes`}>
                📎 {a.filename || "attachment"} ({Math.max(1, Math.round(a.size / 1024))} KB)
              </span>
            ))}
          </div>
        )}
        <div className="flex items-end gap-2">
          <div className="flex-1">
            <Field label="Note">
              <input className="input w-full" value={note} onChange={(e) => setNote(e.target.value)} placeholder="Add a private note..." />
            </Field>
          </div>
          <button
            className="btn-ghost mb-3"
            onClick={async () => {
              await api.updateEmail(email.id, { note });
              onChanged();
            }}
          >
            Save note
          </button>
          <button
            className="btn-primary mb-3"
            onClick={() =>
              onReply({
                to: email.from,
                subject: email.subject.startsWith("Re:") ? email.subject : `Re: ${email.subject}`,
              })
            }
          >
            Reply
          </button>
          <a className="btn-ghost mb-3" href={api.rawEmailUrl(email.id)} download={`email-${email.id}.eml`}>
            .eml
          </a>
          <button
            className="btn-danger mb-3"
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
      </div>
    </div>
  );
}

function MailboxEditor({
  box,
  hosts,
  onClose,
  onSaved,
}: {
  box: Mailbox | null;
  hosts: string[];
  onClose: () => void;
  onSaved: () => void;
}) {
  const [prefix, setPrefix] = useState("");
  const [domain, setDomain] = useState(hosts[0] ?? "");
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
          <input className="input w-full" value={box.address} disabled />
        </Field>
      ) : hosts.length === 0 ? (
        <p className="mb-3 rounded bg-amber-500/10 p-2 text-sm text-amber-300">
          No mail-enabled hosts. Add a domain, toggle <b>Accept email</b>, and add a mail host first.
        </p>
      ) : (
        <Field label="Address" hint="pick a mail host (domain or subdomain)">
          <div className="flex items-center gap-1">
            <input
              className="input w-full"
              value={prefix}
              onChange={(e) => setPrefix(e.target.value)}
              placeholder="hi"
              autoFocus
            />
            <span className="text-zinc-500">@</span>
            <select className="input w-full" value={domain} onChange={(e) => setDomain(e.target.value)}>
              {hosts.map((h) => (
                <option key={h} value={h}>
                  {h}
                </option>
              ))}
            </select>
          </div>
        </Field>
      )}
      <Field label="Note">
        <textarea className="input w-full" rows={2} value={note} onChange={(e) => setNote(e.target.value)} />
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
  const [smtpSenderId, setSmtpSenderId] = useState<number>(0);
  const [senders, setSenders] = useState<any[]>([]);
  const [err, setErr] = useState("");
  const [ok, setOk] = useState(false);

  useEffect(() => {
    api.smtpSenders().then((s) => {
      setSenders(s);
      if (s.length > 0) setSmtpSenderId(s[0].id);
    });
  }, []);

  async function send() {
    setErr("");
    try {
      await api.sendEmail({
        to: to.split(",").map((s) => s.trim()).filter(Boolean),
        from,
        subject,
        text,
        smtpSenderId: smtpSenderId || undefined,
      });
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
          <Field label="SMTP Sender" hint="Choose which account to send through.">
            <select
              className="input w-full"
              value={smtpSenderId}
              onChange={(e) => setSmtpSenderId(Number(e.target.value))}
            >
              <option value={0}>System Default (LED_SMTP_*)</option>
              {senders.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name} ({s.fromEmail})
                </option>
              ))}
            </select>
          </Field>
          <Field label="From (Optional)" hint="Override sender address if SMTP allows it.">
            <input className="input w-full" value={from} onChange={(e) => setFrom(e.target.value)} />
          </Field>
          <Field label="To" hint="comma-separated">
            <input className="input w-full" value={to} onChange={(e) => setTo(e.target.value)} />
          </Field>
          <Field label="Subject">
            <input className="input w-full" value={subject} onChange={(e) => setSubject(e.target.value)} />
          </Field>
          <Field label="Body">
            <textarea className="input w-full" rows={6} value={text} onChange={(e) => setText(e.target.value)} />
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
