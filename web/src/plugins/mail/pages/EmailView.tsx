import { useEffect, useState } from "react";
import { api } from "../../../api";
import { mailApi, Attachment, Email } from "../api";
import { Field, GlassCard, Badge, Button, toast, confirmDialog } from "../../../ui";
import { Paperclip, Trash2, Reply, Download, X, Sparkles, ExternalLink, ShieldAlert, ArchiveRestore, Edit3 } from "lucide-react";
import { useTranslation } from "../../../i18n";
import { ReplyDraft } from "./types";
import { roleSatisfies, useCurrentRole } from "../../../shell/role";
import { AuthBadges } from "./AuthBadges";

export { AuthBadges };

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
  onEditDraft,
  onChanged,
}: {
  email: Email;
  onClose: () => void;
  onReply: (draft: ReplyDraft) => void;
  onEditDraft?: (draft: ReplyDraft) => void;
  onChanged: () => void;
}) {
  const [note, setNote] = useState(email.note ?? "");
  const { t } = useTranslation();
  const { role, isInstanceAdmin } = useCurrentRole();
  const canDeleteEmail = roleSatisfies("admin", role, isInstanceAdmin);
  const attachments = parseAttachments(email.attachments);
  const realAttachments = attachments.filter((a) => !a.inline);
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

  async function moveToFolder(folder: string) {
    try {
      await mailApi.updateEmailFolder(email.id, folder);
      toast.success(t("mail.movedToFolder", { folder }));
      onChanged();
      onClose();
    } catch (e: any) {
      toast.error(e.message || "Failed to move email");
    }
  }

  return (
    <GlassCard className="flex flex-col h-full max-h-full min-h-0">
      <div className="p-5 border-b border-foreground/[0.06] flex justify-between items-start shrink-0 gap-4">
        <div className="flex-1 min-w-0">
          <h2 className="text-lg font-bold text-foreground mb-2 leading-snug">{email.subject || t("mail.noSubject")}</h2>
          <div className="text-xs text-foreground/55 space-y-1.5">
            <div><span className="text-foreground/45">{t("mail.fromLabel")}</span> <span className="font-semibold text-foreground/80">{email.from}</span></div>
            <div><span className="text-foreground/45">{t("mail.toLabel")}</span> <span className="text-foreground/70">{email.to}</span></div>
            <div className="text-[11px] text-foreground/50 pt-0.5">{new Date(email.receivedAt).toLocaleString()}</div>
            <div className="mt-2.5 flex flex-wrap gap-1.5 pt-1">
              <AuthBadges spf={email.authSpf} dkim={email.authDkim} dmarc={email.authDmarc} />
            </div>
          </div>
        </div>
        <Button variant="ghost" onClick={onClose} className="p-2 shrink-0">
          <X className="h-4 w-4" />
        </Button>
      </div>

      {email.unsubscribeUrl && (
        <div className="mx-5 mt-3 flex items-center justify-between gap-3 px-3.5 py-2 bg-warning-bg border border-warning-border rounded-xl text-xs shrink-0">
          <span className="text-warning-fg font-medium truncate">{t("mail.unsubscribe")}</span>
          <Button
            variant="outline"
            className="text-xs py-1 px-2.5 gap-1 shrink-0 text-warning-fg border-warning-border hover:bg-warning-bg cursor-pointer"
            onClick={() => {
              window.open(email.unsubscribeUrl, "_blank", "noopener,noreferrer");
              toast.success(t("mail.unsubscribeOpened"));
            }}
          >
            <ExternalLink className="h-3 w-3" />
            <span>{t("mail.unsubscribe")}</span>
          </Button>
        </div>
      )}

      {summary && (
        <div className="mx-5 mt-4 rounded-xl bg-info-bg border border-info-border p-3.5 shrink-0">
          <div className="flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-wider text-accent-fg mb-1.5">
            <Sparkles className="h-3 w-3" />
            {t("mail.aiSummaryTitle")}
          </div>
          <p className="text-sm text-foreground/85 leading-relaxed whitespace-pre-wrap">{summary}</p>
        </div>
      )}

      <div className="flex-1 overflow-y-auto p-5 min-h-[400px] bg-well">
        {email.html ? (
          <iframe srcDoc={email.html} className="h-full min-h-[400px] w-full bg-white rounded-xl shadow-inner border-0" sandbox="" title={t("mail.iframeTitle")} />
        ) : (
          <pre className="whitespace-pre-wrap break-words font-sans text-sm text-foreground/85 leading-relaxed">{email.text}</pre>
        )}
      </div>

      <div className="p-5 border-t border-foreground/[0.06] shrink-0 bg-foreground/[0.01]">
        {realAttachments.length > 0 && (
          <div className="mb-4 flex flex-wrap gap-2">
            {realAttachments.map((a, i) => (
              <span key={i} className="inline-flex items-center gap-1.5 rounded-lg bg-foreground/[0.05] border border-foreground/[0.06] px-2.5 py-1 text-xs text-foreground/85" title={t("mail.attachmentTitle", { type: a.contentType, size: a.size })}>
                <Paperclip className="h-3 w-3 text-accent-fg" />
                {a.filename || t("mail.attachmentFallback")} <span className="font-mono tnum">({Math.max(1, Math.round(a.size / 1024))} KB)</span>
                {a.truncated && (
                  <Badge tone="amber" className="font-mono text-[9px] uppercase tracking-wider ml-1">
                    {t("mail.truncated")}
                  </Badge>
                )}
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
          <div className="flex flex-wrap gap-2 w-full sm:w-auto shrink-0 pb-1">
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
            {email.folder === "drafts" ? (
              <Button
                variant="primary"
                onClick={() => (onEditDraft || onReply)({ id: email.id, to: email.to, subject: email.subject, text: email.text })}
                className="text-xs py-1.5 px-3 gap-1"
              >
                <Edit3 className="h-3.5 w-3.5" />
                {t("mail.editDraft")}
              </Button>
            ) : (
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
            )}
            {email.folder !== "trash" ? (
              <Button variant="subtle" onClick={() => moveToFolder("trash")} title={t("mail.moveToTrash")} className="text-xs py-1.5 px-2">
                <Trash2 className="h-3.5 w-3.5" />
              </Button>
            ) : (
              <Button variant="subtle" onClick={() => moveToFolder("inbox")} title={t("mail.moveToInbox")} className="text-xs py-1.5 px-2">
                <ArchiveRestore className="h-3.5 w-3.5" />
              </Button>
            )}
            {email.folder !== "spam" && email.folder !== "trash" && (
              <Button variant="subtle" onClick={() => moveToFolder("spam")} title={t("mail.moveToSpam")} className="text-xs py-1.5 px-2">
                <ShieldAlert className="h-3.5 w-3.5" />
              </Button>
            )}
            <Button variant="ghost" onClick={() => window.open(mailApi.rawEmailUrl(email.id))} className="text-xs py-1.5 px-3 font-mono">
              <Download className="h-3.5 w-3.5 mr-1" />
              .eml
            </Button>
            {canDeleteEmail && (
              <Button
                variant="danger"
                onClick={async () => {
                  if (await confirmDialog(t("mail.deleteEmailConfirm"))) {
                    await mailApi.deleteEmail(email.id);
                    onChanged();
                    onClose();
                  }
                }}
                className="text-xs py-1.5 px-3 border-0"
              >
                <Trash2 className="h-3.5 w-3.5" />
              </Button>
            )}
          </div>
        </div>
      </div>
    </GlassCard>
  );
}
