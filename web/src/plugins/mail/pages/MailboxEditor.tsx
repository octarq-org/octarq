import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { api, Domain, effectiveMailHosts } from "../../../api";
import { mailApi, Attachment, Email, Mailbox } from "../api";
import { Code, Field, Guide, Modal, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button, Select, Alert } from "../../../ui";
import { Inbox, Send, Plus, CheckCircle, Mail as MailIcon, Paperclip, Settings, Trash2, Reply, Download, X, AlertTriangle } from "lucide-react";
import { MailSettings } from "./MailSettings";
import { SMTPSenders } from "./SMTPSenders";
import { useTranslation } from "../../../i18n";
import { roleSatisfies, useCurrentRole } from "../../../shell/role";

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
  const { t } = useTranslation();
  const { role, isInstanceAdmin } = useCurrentRole();
  const canDeleteBox = roleSatisfies("admin", role, isInstanceAdmin);
  const [prefix, setPrefix] = useState("");
  const [domain, setDomain] = useState(hosts[0] ?? "");
  const [note, setNote] = useState(box?.note ?? "");
  const [enabled, setEnabled] = useState(box?.enabled ?? true);
  const [err, setErr] = useState("");

  useEffect(() => {
    if (hosts.length > 0 && !domain) setDomain(hosts[0]);
  }, [hosts]);

  async function save() {
    setErr("");
    try {
      if (box) {
        await mailApi.updateMailbox(box.id, { note, enabled });
      } else {
        if (!prefix.trim() || !domain) return setErr(t("mail.prefixRequired"));
        await mailApi.createMailbox({ address: `${prefix.trim()}@${domain}`, note, enabled });
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
          <Alert variant="warning" className="p-3 text-xs flex items-center gap-1.5 font-normal">
            <AlertTriangle className="h-4 w-4" />
            {t("mail.noHosts")}
          </Alert>
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
              <span className="text-foreground/40">@</span>
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
          <span className="text-sm text-foreground/60 select-none">{t("mail.mailReceivingEnabled")}</span>
        </div>
        {canDeleteBox && box && (
          <Button
            variant="danger"
            onClick={async () => {
              if (confirm(t("mail.deleteMailboxConfirm", { address: box.address }))) {
                await mailApi.deleteMailbox(box.id);
                onSaved();
              }
            }}
            className="w-full text-xs py-1.5 border-0 mt-2"
          >
            {t("mail.deleteMailboxCompletely")}
          </Button>
        )}
        {err && <p className="text-sm text-danger-fg font-medium">{err}</p>}
        <div className="flex justify-end gap-2.5 pt-4 border-t border-foreground/[0.06]">
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

