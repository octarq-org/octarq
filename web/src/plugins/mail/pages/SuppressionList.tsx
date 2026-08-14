import { useEffect, useState } from "react";
import { mailApi, MailSuppression } from "../api";
import { Button, Modal, Field, Badge, confirmDialog, timeAgo, Table, THead, TBody, TR, TH, TD, FormError } from "../../../ui";
import { useTranslation } from "../../../i18n";
import { Plus, Trash2, ShieldAlert } from "lucide-react";

export function SuppressionList() {
  const { t } = useTranslation();
  const [items, setItems] = useState<MailSuppression[]>([]);
  const [loading, setLoading] = useState(false);
  const [showAdd, setShowAdd] = useState(false);
  const [address, setAddress] = useState("");
  const [err, setErr] = useState<string | { message?: string; status?: number; requestId?: string }>("");
  const [saving, setSaving] = useState(false);

  async function load() {
    setLoading(true);
    try {
      setItems(await mailApi.suppressions());
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load();
  }, []);

  async function handleAdd() {
    setErr("");
    if (!address.trim() || !address.includes("@")) {
      setErr(t("mail.prefixDomainRequired"));
      return;
    }
    setSaving(true);
    try {
      await mailApi.createSuppression(address.trim());
      setShowAdd(false);
      setAddress("");
      load();
    } catch (e: any) {
      setErr(e);
    } finally {
      setSaving(false);
    }
  }

  async function handleRemove(item: MailSuppression) {
    if (await confirmDialog(t("mail.deleteSuppressionConfirm", { address: item.address }))) {
      await mailApi.deleteSuppression(item.id);
      load();
    }
  }

  function renderReasonBadge(reason: string) {
    switch (reason) {
      case "hard_bounce":
        return <Badge variant="danger">{t("mail.reasonHardBounce")}</Badge>;
      case "complaint":
        return <Badge variant="warning">{t("mail.reasonComplaint")}</Badge>;
      case "manual":
      default:
        return <Badge variant="info">{t("mail.reasonManual")}</Badge>;
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-sm font-semibold text-foreground/90">{t("mail.suppressionList")}</h2>
          <p className="text-xs text-foreground/40 mt-0.5">{t("mail.suppressionListDesc")}</p>
        </div>
        <Button variant="primary" onClick={() => setShowAdd(true)} className="gap-1.5 py-1 text-xs">
          <Plus className="h-3.5 w-3.5" />
          {t("mail.addSuppression")}
        </Button>
      </div>

      {loading && items.length === 0 ? (
        <div className="p-8 text-center text-xs text-foreground/40">{t("mail.loading")}</div>
      ) : items.length === 0 ? (
        <div className="p-8 text-center border border-dashed border-foreground/10 rounded-lg">
          <ShieldAlert className="h-8 w-8 mx-auto text-foreground/20 mb-2" />
          <p className="text-xs text-foreground/40">{t("mail.noSuppressions")}</p>
        </div>
      ) : (
        <div className="overflow-x-auto border border-foreground/[0.06] rounded-lg">
          <Table>
            <THead>
              <TR>
                <TH>{t("mail.addressHeader")}</TH>
                <TH>{t("mail.reasonHeader")}</TH>
                <TH>{t("mail.countHeader")}</TH>
                <TH>{t("mail.dateHeader")}</TH>
                <TH className="text-right">{t("mail.actionsHeader")}</TH>
              </TR>
            </THead>
            <TBody>
              {items.map((item) => (
                <TR key={item.id}>
                  <TD className="font-mono font-medium text-foreground">{item.address}</TD>
                  <TD>{renderReasonBadge(item.reason)}</TD>
                  <TD className="font-mono tnum text-foreground/60">{item.count}</TD>
                  <TD className="font-mono tnum text-foreground/40">{timeAgo(item.createdAt)}</TD>
                  <TD className="text-right">
                    <Button
                      variant="ghost"
                      onClick={() => handleRemove(item)}
                      title={t("mail.removeSuppression")}
                      className="p-1 text-foreground/40 hover:text-danger-fg"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </Button>
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
        </div>
      )}

      {showAdd && (
        <Modal title={t("mail.addSuppressionTitle")} onClose={() => setShowAdd(false)}>
          <div className="space-y-4">
            <Field label={t("mail.enterAddress")}>
              <input
                className="input w-full font-mono text-sm"
                value={address}
                onChange={(e) => setAddress(e.target.value)}
                placeholder={t("mail.addressPlaceholder")}
                autoFocus
              />
            </Field>
            {err && <FormError err={err} />}
            <div className="flex justify-end gap-2.5 pt-4 border-t border-foreground/[0.06]">
              <Button variant="ghost" onClick={() => setShowAdd(false)}>
                {t("mail.cancel")}
              </Button>
              <Button variant="primary" onClick={handleAdd} disabled={saving}>
                {t("mail.saveConfiguration")}
              </Button>
            </div>
          </div>
        </Modal>
      )}
    </div>
  );
}
