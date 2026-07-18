import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { api, Domain, effectiveMailHosts } from "../../../api";
import { mailApi, Attachment, Email, Mailbox } from "../api";
import { Code, Field, Guide, Modal, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button, toast } from "../../../ui";
import { Inbox, Send, Plus, CheckCircle, Mail as MailIcon, Paperclip, Settings, Trash2, Reply, Download, X, AlertTriangle, Sparkles } from "lucide-react";
import { MailSettings, SMTPSenders } from "../../../pages/Settings";
import { useTranslation } from "../../../i18n";
import { ReplyDraft } from "./types";

function parseAttachments(json: string): Attachment[] {
  try {
    const a = JSON.parse(json || "[]");
    return Array.isArray(a) ? a : [];
  } catch {
    return [];
  }
}


export function EmailViewForm({
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
  const { t } = useTranslation();
  const attachments = parseAttachments(email.attachments);
  const [aiEnabled, setAiEnabled] = useState(false);
  const [aiBusy, setAiBusy] = useState(false);
  const [summary, setSummary] = useState("");

  useEffect(() => {
    api.aiAssistStatus().then((s) => setAiEnabled(s.configured)).catch(() => {});
  }, []);
  useEffect(() => setSummary(""), [email.id]);

  async function summarize() {
    setAiBusy(true);
    try {
      const r = await api.aiSummarizeEmail(email.id);
      setSummary(r.summary);
    } catch {
      setSummary(t("mail.aiSummaryFailed"));
    } finally {
      setAiBusy(false);
    }
  }
  return (
    <GlassCard className="flex flex-col h-full max-h-full min-h-0">
      <div className="p-5 border-b border-white/[0.06] flex justify-between items-start shrink-0 gap-4">
         <div>
           <h2 className="text-lg font-bold text-white mb-2 leading-snug">{email.subject || t("mail.noSubject")}</h2>
           <div className="text-xs text-white/55 space-y-1.5">
             <div><span className="text-white/45">{t("mail.fromLabel")}</span> <span className="font-semibold text-white/80">{email.from}</span></div>
             <div><span className="text-white/45">{t("mail.toLabel")}</span> <span className="text-white/70">{email.to}</span></div>
             <div className="text-[11px] text-white/50 pt-0.5">{new Date(email.receivedAt).toLocaleString()}</div>
             <div className="mt-2.5 flex flex-wrap gap-1.5 pt-1">
               <AuthBadges spf={email.authSpf} dkim={email.authDkim} dmarc={email.authDmarc} />
             </div>
           </div>
         </div>
         <Button variant="ghost" onClick={onClose} className="p-2 shrink-0">
           <X className="h-4 w-4" />
         </Button>
      </div>
      
      {summary && (
        <div className="mx-5 mt-4 rounded-xl bg-indigo-500/10 border border-indigo-400/20 p-3.5 shrink-0">
          <div className="flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-wider text-indigo-300 mb-1.5">
            <Sparkles className="h-3 w-3" />
            {t("mail.aiSummaryTitle")}
          </div>
          <p className="text-sm text-white/85 leading-relaxed whitespace-pre-wrap">{summary}</p>
        </div>
      )}
      <div className="flex-1 overflow-y-auto p-5 min-h-[400px] bg-black/10">
        {email.html ? (
          <iframe srcDoc={email.html} className="h-full min-h-[400px] w-full bg-white rounded-xl shadow-inner border-0" sandbox="" title={t("mail.iframeTitle")} />
        ) : (
          <pre className="whitespace-pre-wrap break-words font-sans text-sm text-white/85 leading-relaxed">{email.text}</pre>
        )}
      </div>
      
      <div className="p-5 border-t border-white/[0.06] shrink-0 bg-white/[0.01]">
        {attachments.length > 0 && (
          <div className="mb-4 flex flex-wrap gap-2">
            {attachments.map((a, i) => (
              <span key={i} className="inline-flex items-center gap-1.5 rounded-lg bg-white/[0.05] border border-white/[0.06] px-2.5 py-1 text-xs text-white/85" title={t("mail.attachmentTitle", { type: a.contentType, size: a.size })}>
                <Paperclip className="h-3 w-3 text-indigo-400" />
                {a.filename || t("mail.attachmentFallback")} ({Math.max(1, Math.round(a.size / 1024))} KB)
              </span>
            ))}
          </div>
        )}
        
        <div className="flex flex-col sm:flex-row items-end gap-3 pt-2">
          <div className="w-full sm:flex-1">
            <Field label={t("mail.noteMemo")}>
              <input className="input w-full" value={note} onChange={(e) => setNote(e.target.value)} placeholder={t("mail.notePlaceholder")} />
            </Field>
          </div>
          <div className="flex gap-2 w-full sm:w-auto shrink-0 pb-1">
            {aiEnabled && (
              <Button variant="subtle" onClick={summarize} disabled={aiBusy} className="text-xs py-1.5 px-3 gap-1">
                <Sparkles className="h-3.5 w-3.5" />
                {aiBusy ? t("mail.aiSummarizing") : t("mail.aiSummarize")}
              </Button>
            )}
            <Button
              variant="subtle"
              onClick={async () => {
                await mailApi.updateEmail(email.id, { note });
                onChanged();
                toast.success(t("mail.noteSaved"));
              }}
              className="text-xs py-1.5 px-3"
            >
              {t("mail.saveNote")}
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
              {t("mail.reply")}
            </Button>
            <Button 
              variant="ghost"
              onClick={() => window.open(mailApi.rawEmailUrl(email.id))}
              className="text-xs py-1.5 px-3 font-mono"
            >
              <Download className="h-3.5 w-3.5 mr-1" />
              .eml
            </Button>
            <Button
              variant="danger"
              onClick={async () => {
                if (confirm(t("mail.deleteEmailConfirm"))) {
                  await mailApi.deleteEmail(email.id);
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


export function AuthBadges({
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
