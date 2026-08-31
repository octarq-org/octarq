import { useEffect, useState, useRef } from "react";
import { api } from "../../../api";
import { mailApi, MailContact } from "../api";
import { Field, Modal, Button, Select, FormError, toast } from "../../../ui";
import { CheckCircle, Save } from "lucide-react";
import { useTranslation } from "../../../i18n";
import { ReplyDraft } from "./types";

export function Compose({
  draft,
  onClose,
  onDraftSaved,
}: {
  draft?: ReplyDraft;
  onClose: () => void;
  onDraftSaved?: () => void;
}) {
  const [draftId, setDraftId] = useState<number | undefined>(draft?.id);
  const [to, setTo] = useState(draft?.to ?? "");
  const [from, setFrom] = useState("");
  const [subject, setSubject] = useState(draft?.subject ?? "");
  const [text, setText] = useState(draft?.text ?? "");
  const [smtpSenderId, setSmtpSenderId] = useState<number>(0);
  const [senders, setSenders] = useState<any[]>([]);
  const [err, setErr] = useState<string | { message?: string; status?: number; requestId?: string }>("");
  const [ok, setOk] = useState(false);
  const [autoWrapLinksEnabled, setAutoWrapLinksEnabled] = useState(false);
  const [trackLinks, setTrackLinks] = useState(false);

  // Contacts autocomplete
  const [contacts, setContacts] = useState<MailContact[]>([]);
  const [showSuggestions, setShowSuggestions] = useState(false);
  const toContainerRef = useRef<HTMLDivElement>(null);
  const blurTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const { t } = useTranslation();

  useEffect(() => {
    return () => {
      if (blurTimerRef.current) clearTimeout(blurTimerRef.current);
    };
  }, []);

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

  useEffect(() => {
    if (!to.trim()) {
      mailApi.contacts({ limit: 5 }).then(setContacts).catch(() => {});
      return;
    }
    const timer = setTimeout(() => {
      mailApi.contacts({ query: to.trim(), limit: 5 }).then(setContacts).catch(() => {});
    }, 150);
    return () => clearTimeout(timer);
  }, [to]);

  async function handleSaveDraft() {
    try {
      const res = await mailApi.saveDraft({
        id: draftId,
        to,
        subject,
        text,
      });
      setDraftId(res.id);
      toast.success(t("mail.draftSaved"));
      onDraftSaved?.();
    } catch (e: any) {
      toast.error(e.message || "Failed to save draft");
    }
  }

  async function send() {
    setErr("");
    try {
      await mailApi.sendEmail({
        to: to.split(",").map((s) => s.trim()).filter(Boolean),
        from,
        subject,
        text,
        smtpSenderId: smtpSenderId || undefined,
        trackLinks: autoWrapLinksEnabled ? trackLinks : false,
      });
      if (draftId) {
        mailApi.deleteDraft(draftId).catch(() => {});
      }
      setOk(true);
    } catch (e: any) {
      setErr(e);
    }
  }

  function selectContact(c: MailContact) {
    setTo(c.address);
    setShowSuggestions(false);
  }

  return (
    <Modal title={t("mail.composeMail")} onClose={onClose}>
      {ok ? (
        <div className="py-6 text-center space-y-4">
          <div className="h-12 w-12 rounded-full bg-success-bg border border-success-border flex items-center justify-center text-success-fg mx-auto">
            <CheckCircle className="h-6 w-6" />
          </div>
          <p className="text-foreground font-semibold">{t("mail.messageSent")}</p>
          <Button variant="primary" onClick={onClose} className="w-full">
            {t("mail.done")}
          </Button>
        </div>
      ) : (
        <div className="space-y-4">
          <Field label={t("mail.smtpConnection")} hint={t("mail.smtpConnectionHint")}>
            <Select
              className="text-sm"
              value={String(smtpSenderId)}
              onValueChange={(v) => setSmtpSenderId(Number(v))}
              options={[
                { value: "0", label: t("mail.systemDefaultSmtp") },
                ...senders.map((s) => ({ value: String(s.id), label: `${s.name} (${s.fromEmail})` })),
              ]}
            />
          </Field>
          <Field label={t("mail.fromOverride")} hint={t("mail.fromOverrideHint")}>
            <input className="input w-full font-mono text-sm" value={from} onChange={(e) => setFrom(e.target.value)} placeholder={t("mail.fromPlaceholder")} />
          </Field>
          <div ref={toContainerRef} className="relative">
            <Field label={t("mail.toRecipients")} hint={t("mail.toHint")}>
              <input
                className="input w-full font-mono text-sm"
                value={to}
                onChange={(e) => {
                  setTo(e.target.value);
                  setShowSuggestions(true);
                }}
                onFocus={() => setShowSuggestions(true)}
                onBlur={() => {
                  blurTimerRef.current = setTimeout(() => setShowSuggestions(false), 200);
                }}
                placeholder={t("mail.toPlaceholder")}
                required
              />
            </Field>
            {showSuggestions && contacts.length > 0 && (
              <div className="absolute left-0 right-0 top-full mt-1 z-30 bg-card border border-foreground/[0.1] rounded-xl shadow-lg overflow-hidden divide-y divide-foreground/[0.05]">
                {contacts.map((c) => (
                  <button
                    key={c.id}
                    type="button"
                    className="w-full px-3.5 py-2 text-left text-xs hover:bg-foreground/[0.05] transition-colors flex items-center justify-between cursor-pointer"
                    onMouseDown={(e) => {
                      e.preventDefault();
                      selectContact(c);
                    }}
                  >
                    <span className="font-mono text-foreground/90">{c.name ? `${c.name} <${c.address}>` : c.address}</span>
                    <span className="text-[10px] text-foreground/40 font-mono">{c.interactionCount}</span>
                  </button>
                ))}
              </div>
            )}
          </div>
          <Field label={t("mail.subjectTitle")}>
            <input className="input w-full text-sm" value={subject} onChange={(e) => setSubject(e.target.value)} placeholder={t("mail.subjectPlaceholder")} required />
          </Field>
          <Field label={t("mail.bodyLabel")}>
            <textarea className="input w-full text-sm font-sans" rows={6} value={text} onChange={(e) => setText(e.target.value)} placeholder={t("mail.bodyPlaceholder")} required />
          </Field>
          {autoWrapLinksEnabled && (
            <label className="flex items-center gap-2 cursor-pointer text-sm text-zinc-300 select-none">
              <input
                type="checkbox"
                checked={trackLinks}
                onChange={(e) => setTrackLinks(e.target.checked)}
                className="rounded border-zinc-700 bg-zinc-900/50 text-accent-fg focus:ring-ring"
              />
              <span>{t("mail.trackLinks")}</span>
            </label>
          )}
          {err && <FormError err={err} />}
          <div className="flex items-center justify-between gap-2.5 pt-4 border-t border-foreground/[0.06]">
            <Button
              variant="outline"
              type="button"
              onClick={handleSaveDraft}
              className="text-xs py-1.5 px-3 gap-1.5"
            >
              <Save className="h-3.5 w-3.5" />
              {t("mail.saveDraft")}
            </Button>
            <div className="flex gap-2">
              <Button variant="ghost" onClick={onClose}>
                {t("mail.cancel")}
              </Button>
              <Button variant="primary" onClick={send} disabled={!to || !subject}>
                {t("mail.sendMail")}
              </Button>
            </div>
          </div>
        </div>
      )}
    </Modal>
  );
}
