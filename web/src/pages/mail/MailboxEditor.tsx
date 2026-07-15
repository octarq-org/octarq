import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { api, Attachment, Domain, effectiveMailHosts, Email, Mailbox } from "../../api";
import { Code, Field, Guide, Modal, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button, Select } from "../../ui";
import { Inbox, Send, Plus, CheckCircle, Mail as MailIcon, Paperclip, Settings, Trash2, Reply, Download, X, AlertTriangle } from "lucide-react";
import { MailSettings, SMTPSenders } from "../Settings";
import { useTranslation } from "../../i18n";

export function MailboxEditor({
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
  const { t } = useTranslation();

  async function save() {
    setErr("");
    try {
      if (box) {
        await api.updateMailbox(box.id, { note, enabled });
      } else {
        if (!prefix.trim() || !domain) {
          setErr(t("mail.prefixDomainRequired"));
          return;
        }
        await api.createMailbox({ address: `${prefix.trim()}@${domain}`, note, enabled });
      }
      onSaved();
    } catch (e: any) {
      setErr(e.message ?? t("mail.saveFailed"));
    }
  }

  return (
    <Modal title={box ? t("mail.editMailbox") : t("mail.createMailbox")} onClose={onClose}>
      <div className="space-y-4">
        {box ? (
          <Field label={t("mail.mailboxAddress")}>
            <input className="input w-full font-mono text-sm" value={box.address} disabled />
          </Field>
        ) : hosts.length === 0 ? (
          <p className="rounded bg-amber-500/10 p-3 text-xs text-amber-300 flex items-center gap-1.5">
            <AlertTriangle className="h-4 w-4" />
            {t("mail.noHosts")}
          </p>
        ) : (
          <Field label={t("mail.mailboxPrefix")} hint={t("mail.prefixHint")}>
            <div className="flex items-center gap-2">
              <input
                className="input w-full font-mono text-sm"
                value={prefix}
                onChange={(e) => setPrefix(e.target.value)}
                placeholder={t("mail.prefixPlaceholder")}
                autoFocus
              />
              <span className="text-white/40">@</span>
              <div className="min-w-0 flex-1">
                <Select
                  className="text-sm"
                  value={domain}
                  onValueChange={setDomain}
                  options={hosts.map((h) => ({ value: h, label: h }))}
                />
              </div>
            </div>
          </Field>
        )}
        <Field label={t("mail.noteMemo")}>
          <textarea className="input w-full text-sm" rows={2} value={note} onChange={(e) => setNote(e.target.value)} placeholder={t("mail.noteMemoPlaceholder")} />
        </Field>
        <div className="flex items-center gap-3 py-1">
          <Toggle on={enabled} onChange={setEnabled} />
          <span className="text-sm text-white/60 select-none">{t("mail.mailReceivingEnabled")}</span>
        </div>
        {box && (
          <Button
            variant="danger"
            onClick={async () => {
              if (confirm(t("mail.deleteMailboxConfirm", { address: box.address }))) {
                await api.deleteMailbox(box.id);
                onSaved();
              }
            }}
            className="w-full text-xs py-1.5 bg-rose-500/10 hover:bg-rose-500/25 border-0 mt-2"
          >
            {t("mail.deleteMailboxCompletely")}
          </Button>
        )}
        {err && <p className="text-sm text-rose-400 font-medium">{err}</p>}
        <div className="flex justify-end gap-2.5 pt-4 border-t border-white/[0.06]">
          <Button variant="ghost" onClick={onClose}>
            {t("mail.cancel")}
          </Button>
          <Button variant="primary" onClick={save}>
            {t("mail.saveConfiguration")}
          </Button>
        </div>
      </div>
    </Modal>
  );
}

