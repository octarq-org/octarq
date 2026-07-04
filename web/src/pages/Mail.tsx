import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { api, Attachment, Domain, effectiveMailHosts, Email, Mailbox } from "../api";
import { Code, Field, Guide, Modal, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button } from "../ui";
import { Inbox, Send, Plus, CheckCircle, Mail as MailIcon, Paperclip, Settings, Trash2, Reply, Download, X, AlertTriangle } from "lucide-react";
import { MailSettings, SMTPSenders } from "./Settings";

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

  const [searchParams, setSearchParams] = useSearchParams();
  const tabParam = searchParams.get("tab");
  const tab = (tabParam === "settings") ? "settings" : "mail";
  const setTab = (t: "mail" | "settings") => {
    setSearchParams(prev => {
      if (t === "mail") {
        prev.delete("tab");
      } else {
        prev.set("tab", t);
      }
      return prev;
    }, { replace: true });
  };

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
    <ScreenWrap>
      <PageHeader
        title="Mail"
        description="Custom domain email reading, replying & SMTP relays"
        action={
          <div className="flex gap-2">
            <Button variant="ghost" onClick={markAllRead} className="py-1.5 text-xs">
              Mark Read
            </Button>
            <Button variant="outline" onClick={() => setCompose(true)} className="gap-1.5 py-1.5 text-xs">
              <Send className="h-3.5 w-3.5" />
              Compose
            </Button>
            <Button variant="primary" onClick={() => setNewBox(true)} className="gap-1.5 py-1.5 text-xs">
              <Plus className="h-3.5 w-3.5" />
              New Mailbox
            </Button>
          </div>
        }
      />

      <div className="flex gap-0 border-b border-white/[0.06] mb-6">
        <button
          onClick={() => setTab('mail')}
          className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors ${
            tab === 'mail'
              ? 'border-indigo-500 text-white'
              : 'border-transparent text-white/45 hover:text-white/70'
          }`}
        >
          Mail
        </button>
        <button
          onClick={() => setTab('settings')}
          className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors flex items-center gap-1.5 ${
            tab === 'settings'
              ? 'border-indigo-500 text-white'
              : 'border-transparent text-white/45 hover:text-white/70'
          }`}
        >
          Settings
        </button>
      </div>

      {tab === 'mail' && (<>

      {boxes.length === 0 && (
        <Guide title="Set up mail receiving with Cloudflare Email Routing" open>
          <ol className="ml-4 list-decimal space-y-1.5 text-sm leading-relaxed text-white/70">
            <li>Add a domain in <b>Domains</b>, toggle <b>Accept email</b>, and list its mail hosts.</li>
            <li>In Cloudflare → <b>Email → Email Routing</b>, enable routing.</li>
            <li>Deploy <Code>deploy/cloudflare-email-worker.js</Code> with <Code>LED_ENDPOINT</Code> set to your <b>Inbound Webhook URL</b> (copy it from <b>Settings → Mailboxes</b> — the token is in the path, no header needed), then point a catch-all route at it.</li>
            <li>To send replies, configure an SMTP relay via Settings.</li>
          </ol>
        </Guide>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-[300px_1fr] gap-6 min-h-0 items-start">
        {/* Left column - list */}
        <div className="flex flex-col min-h-0 w-full">
          <div className="mb-3 flex items-center gap-2">
            <select
              className="input flex-1 min-w-0"
              value={active === undefined ? "" : active}
              onChange={(e) => setActive(e.target.value ? Number(e.target.value) : undefined)}
            >
              <option value="">All Mailboxes</option>
              {boxes.map(b => (
                <option key={b.id} value={b.id}>{b.address} {b.unread > 0 ? `(${b.unread})` : ""}</option>
              ))}
            </select>
            {active !== undefined && (
              <Button
                variant="ghost"
                title="Edit Mailbox"
                onClick={() => setEditBox(boxes.find(b => b.id === active) || null)}
                className="shrink-0 p-2"
              >
                <Settings className="h-4 w-4" />
              </Button>
            )}
          </div>
          
          <div className="mb-3">
            <input
              className="input w-full"
              placeholder="Search emails…"
              value={q}
              onChange={(e) => setQ(e.target.value)}
            />
          </div>
          
          <GlassCard className="overflow-hidden">
            <div className="overflow-y-auto max-h-[600px] divide-y divide-white/[0.04]" onScroll={handleScroll}>
              {emails.length === 0 && !loading ? (
                <div className="p-8 text-center text-white/40 text-sm">No messages.</div>
              ) : (
                <>
                  {emails.map((e) => (
                    <button
                      key={e.id}
                      className={`flex w-full flex-col p-4 text-left hover:bg-white/[0.03] transition-colors ${open?.id === e.id ? "bg-white/[0.05]" : ""}`}
                      onClick={() => openEmail(e)}
                    >
                      <div className="flex items-center justify-between w-full mb-1 gap-2">
                        <div className="flex items-center gap-2 overflow-hidden">
                          {!e.read && <span className="h-2 w-2 shrink-0 rounded-full bg-indigo-400" />}
                          <span className={`truncate text-sm ${e.read ? "text-white/55" : "font-semibold text-white"}`}>{e.from || "(unknown)"}</span>
                        </div>
                        <div className="flex items-center gap-1.5 shrink-0 ml-2">
                          <AuthBadges spf={e.authSpf} dkim={e.authDkim} dmarc={e.authDmarc} compact />
                          <span className="text-[11px] text-white/40">{timeAgo(e.receivedAt)}</span>
                        </div>
                      </div>
                      <div className={`truncate text-xs ${e.read ? "text-white/40" : "text-white/70"}`}>
                        {e.subject || "(no subject)"}
                      </div>
                    </button>
                  ))}
                  {loading && <div className="p-3 text-center text-xs text-white/40">Loading…</div>}
                </>
              )}
            </div>
          </GlassCard>
        </div>

        {/* Right column - email view pane */}
        <div className="w-full">
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
            <GlassCard className="flex flex-col items-center justify-center py-20 px-6 text-center text-white/40 border border-white/[0.04]/40">
              <Inbox className="h-10 w-10 mb-2 opacity-50 text-indigo-400" />
              <p className="text-sm">Select an email message from the inbox list to read.</p>
            </GlassCard>
          )}
        </div>
      </div>

      </>
      )}
      {tab === 'settings' && (
        <div className="space-y-8">
          <div>
            <h3 className="text-sm font-semibold text-white/70 mb-3">Inbound Configuration</h3>
            <GlassCard className="p-6"><MailSettings /></GlassCard>
          </div>
          <div>
            <h3 className="text-sm font-semibold text-white/70 mb-3">SMTP Outgoing Gateways</h3>
            <GlassCard className="p-6"><SMTPSenders /></GlassCard>
          </div>
        </div>
      )}

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
    </ScreenWrap>
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
    <GlassCard className="flex flex-col h-full max-h-full min-h-0">
      <div className="p-5 border-b border-white/[0.06] flex justify-between items-start shrink-0 gap-4">
         <div>
           <h2 className="text-lg font-bold text-white mb-2 leading-snug">{email.subject || "(no subject)"}</h2>
           <div className="text-xs text-white/55 space-y-1.5">
             <div><span className="text-white/45">From:</span> <span className="font-semibold text-white/80">{email.from}</span></div>
             <div><span className="text-white/45">To:</span> <span className="text-white/70">{email.to}</span></div>
             <div className="text-[11px] text-white/35 pt-0.5">{new Date(email.receivedAt).toLocaleString()}</div>
             <div className="mt-2.5 flex flex-wrap gap-1.5 pt-1">
               <AuthBadges spf={email.authSpf} dkim={email.authDkim} dmarc={email.authDmarc} />
             </div>
           </div>
         </div>
         <Button variant="ghost" onClick={onClose} className="p-2 shrink-0">
           <X className="h-4 w-4" />
         </Button>
      </div>
      
      <div className="flex-1 overflow-y-auto p-5 min-h-[400px] bg-black/10">
        {email.html ? (
          <iframe srcDoc={email.html} className="h-full min-h-[400px] w-full bg-white rounded-xl shadow-inner border-0" sandbox="" title="email" />
        ) : (
          <pre className="whitespace-pre-wrap break-words font-sans text-sm text-white/85 leading-relaxed">{email.text}</pre>
        )}
      </div>
      
      <div className="p-5 border-t border-white/[0.06] shrink-0 bg-white/[0.01]">
        {attachments.length > 0 && (
          <div className="mb-4 flex flex-wrap gap-2">
            {attachments.map((a, i) => (
              <span key={i} className="inline-flex items-center gap-1.5 rounded-lg bg-white/[0.05] border border-white/[0.06] px-2.5 py-1 text-xs text-white/85" title={`${a.contentType} · ${a.size} bytes`}>
                <Paperclip className="h-3 w-3 text-indigo-400" />
                {a.filename || "attachment"} ({Math.max(1, Math.round(a.size / 1024))} KB)
              </span>
            ))}
          </div>
        )}
        
        <div className="flex flex-col sm:flex-row items-end gap-3 pt-2">
          <div className="w-full sm:flex-1">
            <Field label="Note Memo">
              <input className="input w-full" value={note} onChange={(e) => setNote(e.target.value)} placeholder="Add a private note..." />
            </Field>
          </div>
          <div className="flex gap-2 w-full sm:w-auto shrink-0 pb-1">
            <Button
              variant="subtle"
              onClick={async () => {
                await api.updateEmail(email.id, { note });
                onChanged();
                alert("Note saved");
              }}
              className="text-xs py-1.5 px-3"
            >
              Save Note
            </Button>
            <Button
              variant="primary"
              onClick={() =>
                onReply({
                  to: email.from,
                  subject: email.subject.startsWith("Re:") ? email.subject : `Re: ${email.subject}`,
                })
              }
              className="text-xs py-1.5 px-3 gap-1"
            >
              <Reply className="h-3.5 w-3.5" />
              Reply
            </Button>
            <Button 
              variant="ghost"
              onClick={() => window.open(api.rawEmailUrl(email.id))}
              className="text-xs py-1.5 px-3 font-mono"
            >
              <Download className="h-3.5 w-3.5 mr-1" />
              .eml
            </Button>
            <Button
              variant="danger"
              onClick={async () => {
                if (confirm("Delete this email?")) {
                  await api.deleteEmail(email.id);
                  onChanged();
                  onClose();
                }
              }}
              className="text-xs py-1.5 px-3 bg-rose-500/10 hover:bg-rose-500/20 border-0"
            >
              <Trash2 className="h-3.5 w-3.5" />
            </Button>
          </div>
        </div>
      </div>
    </GlassCard>
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
    <Modal title={box ? "Edit Mailbox" : "Create Mailbox"} onClose={onClose}>
      <div className="space-y-4">
        {box ? (
          <Field label="Mailbox Address">
            <input className="input w-full font-mono text-sm" value={box.address} disabled />
          </Field>
        ) : hosts.length === 0 ? (
          <p className="rounded bg-amber-500/10 p-3 text-xs text-amber-300 flex items-center gap-1.5">
            <AlertTriangle className="h-4 w-4" />
            No mail-enabled hosts. Configure your custom domain first.
          </p>
        ) : (
          <Field label="Mailbox Prefix" hint="Choose username part of address">
            <div className="flex items-center gap-2">
              <input
                className="input w-full font-mono text-sm"
                value={prefix}
                onChange={(e) => setPrefix(e.target.value)}
                placeholder="e.g. sales"
                autoFocus
              />
              <span className="text-white/40">@</span>
              <select className="input w-full text-sm" value={domain} onChange={(e) => setDomain(e.target.value)}>
                {hosts.map((h) => (
                  <option key={h} value={h}>
                    {h}
                  </option>
                ))}
              </select>
            </div>
          </Field>
        )}
        <Field label="Note Memo">
          <textarea className="input w-full text-sm" rows={2} value={note} onChange={(e) => setNote(e.target.value)} placeholder="e.g. support operations" />
        </Field>
        <div className="flex items-center gap-3 py-1">
          <Toggle on={enabled} onChange={setEnabled} />
          <span className="text-sm text-white/60 select-none">Mail Receiving Enabled</span>
        </div>
        {box && (
          <Button
            variant="danger"
            onClick={async () => {
              if (confirm(`Delete mailbox ${box.address} and all its email messages?`)) {
                await api.deleteMailbox(box.id);
                onSaved();
              }
            }}
            className="w-full text-xs py-1.5 bg-rose-500/10 hover:bg-rose-500/25 border-0 mt-2"
          >
            Delete Mailbox Completely
          </Button>
        )}
        {err && <p className="text-sm text-rose-400 font-medium">{err}</p>}
        <div className="flex justify-end gap-2.5 pt-4 border-t border-white/[0.06]">
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" onClick={save}>
            Save Configuration
          </Button>
        </div>
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
  const [autoWrapLinksEnabled, setAutoWrapLinksEnabled] = useState(false);
  const [trackLinks, setTrackLinks] = useState(false);

  useEffect(() => {
    api.smtpSenders().then((s) => {
      setSenders(s);
      if (s.length > 0) setSmtpSenderId(s[0].id);
    });
    api.settings().then((s) => {
      if (s.autoWrapLinks) {
        setAutoWrapLinksEnabled(true);
        setTrackLinks(true);
      }
    }).catch(() => {});
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
        trackLinks: autoWrapLinksEnabled ? trackLinks : false,
      });
      setOk(true);
    } catch (e: any) {
      setErr(e.message ?? "send failed");
    }
  }

  return (
    <Modal title="Compose Mail" onClose={onClose}>
      {ok ? (
        <div className="py-6 text-center space-y-4">
          <div className="h-12 w-12 rounded-full bg-emerald-500/10 flex items-center justify-center text-emerald-400 mx-auto">
            <CheckCircle className="h-6 w-6" />
          </div>
          <p className="text-white font-semibold">Message Sent Successfully</p>
          <Button variant="primary" onClick={onClose} className="w-full">
            Done
          </Button>
        </div>
      ) : (
        <div className="space-y-4">
          <Field label="SMTP Connection" hint="Pick credentials used to send this mail">
            <select
              className="input w-full text-sm"
              value={smtpSenderId}
              onChange={(e) => setSmtpSenderId(Number(e.target.value))}
            >
              <option value={0}>System Default SMTP settings</option>
              {senders.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name} ({s.fromEmail})
                </option>
              ))}
            </select>
          </Field>
          <Field label="From (Override)" hint="SMTP servers may reject mismatched send addresses">
            <input className="input w-full font-mono text-sm" value={from} onChange={(e) => setFrom(e.target.value)} placeholder="e.g. custom@domain.com" />
          </Field>
          <Field label="To (Recipients)" hint="Comma-separated email addresses">
            <input className="input w-full font-mono text-sm" value={to} onChange={(e) => setTo(e.target.value)} placeholder="hello@world.com" required />
          </Field>
          <Field label="Subject Title">
            <input className="input w-full text-sm" value={subject} onChange={(e) => setSubject(e.target.value)} placeholder="Subject line" required />
          </Field>
          <Field label="Plaintext Message Body">
            <textarea className="input w-full text-sm font-sans" rows={6} value={text} onChange={(e) => setText(e.target.value)} placeholder="Type mail content here..." required />
          </Field>
          {autoWrapLinksEnabled && (
            <label className="flex items-center gap-2 cursor-pointer text-sm text-zinc-300 select-none">
              <input
                type="checkbox"
                checked={trackLinks}
                onChange={(e) => setTrackLinks(e.target.checked)}
                className="rounded border-zinc-700 bg-zinc-900/50 text-purple-600 focus:ring-purple-500"
              />
              <span>Track outbound links in email (Wrap with short links)</span>
            </label>
          )}
          {err && <p className="text-sm text-rose-400 font-medium">{err}</p>}
          <div className="flex justify-end gap-2.5 pt-4 border-t border-white/[0.06]">
            <Button variant="ghost" onClick={onClose}>
              Cancel
            </Button>
            <Button variant="primary" onClick={send} disabled={!to || !subject}>
              Send Mail
            </Button>
          </div>
        </div>
      )}
    </Modal>
  );
}

// AuthBadges renders compact SPF/DKIM/DMARC result pills.
// In compact mode (list row) only failing/suspicious results are shown to save space.
function AuthBadges({
  spf, dkim, dmarc, compact = false,
}: { spf?: string; dkim?: string; dmarc?: string; compact?: boolean }) {
  const badge = (label: string, result: string | undefined) => {
    if (!result || result === "none" || result === "") return null;
    const pass = result === "pass";
    const warn = result === "softfail" || result === "neutral";
    if (compact && pass) return null; // only show problems in list view
    
    let tone: "green" | "amber" | "red" = "red";
    if (pass) tone = "green";
    else if (warn) tone = "amber";

    return (
      <Badge key={label} tone={tone} className="font-mono text-[9px] uppercase tracking-wider">
        {label}:{result}
      </Badge>
    );
  };
  const badges = [badge("SPF", spf), badge("DKIM", dkim), badge("DMARC", dmarc)].filter(Boolean);
  if (badges.length === 0) return null;
  return <div className="flex gap-1 items-center">{badges}</div>;
}
