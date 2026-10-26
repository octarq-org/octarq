import { Field, Input, Textarea, Modal, Button, Select, FormError, toast } from "@octarq/plugin-sdk";
import { useEffect, useState, useRef } from "react";
import { Link } from "react-router-dom";
import { api } from "../../../api";
import { mailApi, MailContact } from "../api";
import { CheckCircle, Save, AlertTriangle } from "lucide-react";
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
  const [loadingSenders, setLoadingSenders] = useState(true);
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
    setLoadingSenders(true);
    fetch("/api/integrations?provider_type=communication", {
      credentials: "same-origin",
    })
      .then((res) => (res.ok ? res.json() : []))
      .then((data: any[]) => {
        const commSenders = (Array.isArray(data) ? data : []).filter(
          (item: any) => item.provider === "smtp" || item.provider === "resend"
        );
        setSenders(commSenders);
        if (commSenders.length > 0) {
          const defaultSender = commSenders.find((s: any) => s.isDefault) || commSenders[0];
          setSmtpSenderId(defaultSender.id);
        } else {
          setSmtpSenderId(0);
        }
      })
      .catch(() => {
        setSenders([]);
        setSmtpSenderId(0);
      })
      .finally(() => {
        setLoadingSenders(false);
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
          <Button variant="primary" onClick={onClose} >
            {t("mail.done")}
          </Button>
        </div>
      ) : (
        <div className="space-y-4">
          {!loadingSenders && senders.length === 0 ? (
            <div className="rounded-lg border border-warning/30 bg-warning/5 p-4 text-xs text-foreground/80 space-y-3">
              <div className="flex items-start gap-2.5">
                <AlertTriangle className="h-4 w-4 text-warning shrink-0 mt-0.5" />
                <div className="space-y-1">
                  <p className="font-semibold text-foreground">
                    {t("settings.noSmtpSenders")}
                  </p>
                  <p className="text-muted-foreground">
                    {t("settings.integrationsHubBanner")}
                  </p>
                </div>
              </div>
              <Link
                to="/settings/integrations?category=communication"
                className="inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
              >
                {t("settings.openIntegrationsHub")} →
              </Link>
            </div>
          ) : (
            <Field label={t("mail.smtpConnection")} hint={t("mail.smtpConnectionHint")}>
              <Select
                className="text-sm"
                value={String(smtpSenderId)}
                onValueChange={(v) => setSmtpSenderId(Number(v))}
                options={senders.map((s) => ({
                  value: String(s.id),
                  label: s.summary
                    ? `${s.name} (${s.summary})`
                    : s.fromEmail
                      ? `${s.name} (${s.fromEmail})`
                      : `${s.name} (${s.provider})`,
                }))}
              />
            </Field>
          )}
          <Field label={t("mail.fromOverride")} hint={t("mail.fromOverrideHint")}>
            <Input className="font-mono text-sm" value={from} onChange={(e) => setFrom(e.target.value)} placeholder={t("mail.fromPlaceholder")} />
          </Field>
          <div ref={toContainerRef} className="relative">
            <Field label={t("mail.toRecipients")} hint={t("mail.toHint")}>
              <Input
                className="font-mono text-sm"
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
                    className="px-3.5 py-2 text-left text-xs hover:bg-foreground/[0.05] transition-colors flex items-center justify-between cursor-pointer"
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
            <Input className="text-sm" value={subject} onChange={(e) => setSubject(e.target.value)} placeholder={t("mail.subjectPlaceholder")} required />
          </Field>
          <Field label={t("mail.bodyLabel")}>
            <Textarea className="text-sm font-sans" rows={6} value={text} onChange={(e) => setText(e.target.value)} placeholder={t("mail.bodyPlaceholder")} required />
          </Field>
          {autoWrapLinksEnabled && (
            <label className="flex items-center gap-2 cursor-pointer text-sm text-zinc-300 select-none">
              <Input
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
              <Button variant="primary" onClick={send} disabled={!to || !subject || senders.length === 0}>
                {t("mail.sendMail")}
              </Button>
            </div>
          </div>
        </div>
      )}
    </Modal>
  );
}
