import { useEffect, useMemo, useState } from "react";
import { api, Domain, HostEntry, ProviderAccount } from "../../../api";
import { dnsApi, DNSRecord, DNSVerifyResult, HostDNSStatus, LinkHostStatus, DNSRecordStatus } from "../api";
import { Code, Empty, Field, Guide, HostList, Modal, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button, Select } from "../../../ui";
import { Globe, RefreshCw, Plus, Trash2, ArrowRight, ShieldCheck, Mail, Link as LinkIcon, Cloud } from "lucide-react";
import { ProviderAccounts } from "./ProviderAccounts";
import { useTranslation } from "../../../i18n";

export function SyncModal({ accounts, onClose, onSynced }: { accounts: ProviderAccount[]; onClose: () => void; onSynced: () => void }) {
  const { t } = useTranslation();
  const [accountId, setAccountId] = useState<number>(accounts[0]?.id || 0);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [result, setResult] = useState<{ total: number; created: number; updated: number } | null>(null);

  async function run() {
    if (!accountId) return setErr(t("domains.selectProviderAccount"));
    setBusy(true); setErr("");
    try { const r = await dnsApi.syncDomains(accountId); setResult(r); }
    catch (e: any) { setErr(e.message ?? t("domains.syncFailed")); }
    finally { setBusy(false); }
  }

  return (
    <Modal title={t("domains.syncDnsZones")} onClose={onClose}>
      {result ? (
        <div className="py-4 text-center space-y-4">
          <div className="h-12 w-12 rounded-full bg-emerald-500/10 flex items-center justify-center text-success-fg mx-auto">
            <ShieldCheck className="h-6 w-6" />
          </div>
          <div>
            <p className="text-foreground font-semibold">{t("domains.zonesDetected", { count: result.total })}</p>
            <p className="text-xs text-foreground/55 mt-1">
              {t("domains.createdPrefix")} <span className="text-success-fg font-bold">{result.created}</span> {t("domains.updatedMid")} <span className="text-accent-fg font-bold">{result.updated}</span> {t("domains.recordsSuffix")}
            </p>
          </div>
          <p className="text-[11px] text-foreground/40 max-w-xs mx-auto">{t("domains.syncToggleHint")}</p>
          <Button variant="primary" onClick={onSynced} className="w-full">{t("domains.done")}</Button>
        </div>
      ) : accounts.length === 0 ? (
        <div className="py-4 text-center space-y-2 text-foreground/55">
          <p className="font-semibold">{t("domains.noProviderAccounts")}</p>
          <p className="text-xs text-foreground/40">{t("domains.noProviderAccountsHint")}</p>
        </div>
      ) : (
        <>
          <p className="mb-4 text-xs text-foreground/55 leading-relaxed">{t("domains.syncIntro")}</p>
          <Field label={t("domains.dnsProviderConnection")}>
            <Select
              value={String(accountId)}
              onValueChange={(v) => setAccountId(Number(v))}
              options={[
                { value: "0", label: t("domains.selectAccount") },
                ...accounts.map((a) => ({ value: String(a.id), label: `${a.name} (${a.type})` })),
              ]}
            />
          </Field>
          {err && <p className="mb-4 text-sm text-danger-fg font-medium">{err}</p>}
          <div className="flex justify-end gap-2.5 pt-4 border-t border-foreground/[0.06]">
            <Button variant="ghost" onClick={onClose}>{t("domains.cancel")}</Button>
            <Button variant="primary" onClick={run} disabled={busy || !accountId}>{busy ? t("domains.queryingApi") : t("domains.syncZones")}</Button>
          </div>
        </>
      )}
    </Modal>
  );
}

