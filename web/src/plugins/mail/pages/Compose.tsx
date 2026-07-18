import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { api, Domain, effectiveMailHosts } from "../../../api";
import { mailApi, Attachment, Email, Mailbox } from "../api";
import { Code, Field, Guide, Modal, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button, Select } from "../../../ui";
import { Inbox, Send, Plus, CheckCircle, Mail as MailIcon, Paperclip, Settings, Trash2, Reply, Download, X, AlertTriangle } from "lucide-react";
import { MailSettings } from "./MailSettings";
import { SMTPSenders } from "./SMTPSenders";
import { useTranslation } from "../../../i18n";
import { ReplyDraft } from "./types";

export function Compose({ draft, onClose }: { draft?: ReplyDraft; onClose: () => void }) {
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
  const { t } = useTranslation();

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
      await mailApi.sendEmail({
        to: to.split(",").map((s) => s.trim()).filter(Boolean),
        from,
        subject,
        text,
        smtpSenderId: smtpSenderId || undefined,
        trackLinks: autoWrapLinksEnabled ? trackLinks : false,
      });
      setOk(true);
    } catch (e: any) {
      setErr(e.message ?? t("mail.sendFailed"));
    }
  }

  return (
    <Modal title={t("mail.composeMail")} onClose={onClose}>
      {ok ? (
        <div className="py-6 text-center space-y-4">
          <div className="h-12 w-12 rounded-full bg-emerald-500/10 flex items-center justify-center text-emerald-400 mx-auto">
            <CheckCircle className="h-6 w-6" />
          </div>
          <p className="text-white font-semibold">{t("mail.messageSent")}</p>
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
          <Field label={t("mail.toRecipients")} hint={t("mail.toHint")}>
            <input className="input w-full font-mono text-sm" value={to} onChange={(e) => setTo(e.target.value)} placeholder={t("mail.toPlaceholder")} required />
          </Field>
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
                className="rounded border-zinc-700 bg-zinc-900/50 text-purple-600 focus:ring-purple-500"
              />
              <span>{t("mail.trackLinks")}</span>
            </label>
          )}
          {err && <p className="text-sm text-rose-400 font-medium">{err}</p>}
          <div className="flex justify-end gap-2.5 pt-4 border-t border-white/[0.06]">
            <Button variant="ghost" onClick={onClose}>
              {t("mail.cancel")}
            </Button>
            <Button variant="primary" onClick={send} disabled={!to || !subject}>
              {t("mail.sendMail")}
            </Button>
          </div>
        </div>
      )}
    </Modal>
  );
}

// AuthBadges renders compact SPF/DKIM/DMARC result pills.
// In compact mode (list row) only failing/suspicious results are shown to save space.
