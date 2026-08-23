import { useEffect, useMemo, useState } from "react";
import { api, Domain, HostEntry, ProviderAccount } from "../../../api";
import { dnsApi, DNSRecord, DNSVerifyResult, HostDNSStatus, LinkHostStatus, DNSRecordStatus } from "../api";
import { Code, Empty, Field, Guide, HostList, Modal, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button, Select, Table, THead, TBody, TR, TH, TD, toast } from "../../../ui";
import { Globe, RefreshCw, Plus, Trash2, ArrowRight, ShieldCheck, Mail, Link as LinkIcon, Cloud } from "lucide-react";
import { ProviderAccounts } from "./ProviderAccounts";
import { useTranslation } from "../../../i18n";
import { mergeHosts } from "./shared";

export function DomainHostManager({ domain, onReload }: { domain: Domain; onReload: (updated?: Domain) => void }) {
  const { t } = useTranslation();
  const [busy, setBusy] = useState<string | null>(null);
  const hosts = useMemo(() => mergeHosts(domain), [domain]);

  async function toggleHost(hostName: string, service: "linkHosts" | "mailHosts", currentEnabled: boolean) {
    const key = `${service}:${hostName}`;
    if (busy === key) return;
    setBusy(key);
    const list = (service === "linkHosts" ? domain.linkHosts : domain.mailHosts) ?? [];
    const updated = list.map((h) =>
      h.host === hostName ? { ...h, enabled: !currentEnabled } : h,
    );
    try {
      const res = await dnsApi.updateDomain(domain.id, { [service]: updated });
      await onReload(res);
    } catch (e: any) {
      toast.error(e.message || t("domains.updateFailed"));
    } finally {
      setBusy(null);
    }
  }

  async function addHost(hostName: string, forLink: boolean, forMail: boolean) {
    if (!hostName || (!forLink && !forMail)) return;
    setBusy("add");
    const linkHosts = domain.linkHosts ?? [];
    const mailHosts = domain.mailHosts ?? [];
    const payload: Record<string, unknown> = {};
    if (forLink && !linkHosts.some((h) => h.host === hostName)) {
      payload.linkHosts = [...linkHosts, { host: hostName, enabled: true }];
    }
    if (forMail && !mailHosts.some((h) => h.host === hostName)) {
      payload.mailHosts = [...mailHosts, { host: hostName, enabled: true }];
    }
    try {
      const res = await dnsApi.updateDomain(domain.id, payload);
      await onReload(res);
    } catch (e: any) {
      toast.error(e.message || t("domains.addHostFailed"));
    } finally {
      setBusy(null);
    }
  }

  async function removeHost(hostName: string) {
    setBusy(`remove:${hostName}`);
    try {
      const res = await dnsApi.updateDomain(domain.id, {
        linkHosts: (domain.linkHosts ?? []).filter((h) => h.host !== hostName),
        mailHosts: (domain.mailHosts ?? []).filter((h) => h.host !== hostName),
      });
      await onReload(res);
    } catch (e: any) {
      toast.error(e.message || t("domains.removeHostFailed"));
    } finally {
      setBusy(null);
    }
  }

  return (
    <div className="bg-well rounded-2xl p-4 border border-foreground/[0.05] space-y-4">
      {hosts.length === 0 ? (
        <p className="text-sm text-foreground/50">{t("domains.noActiveHosts")}</p>
      ) : (
        <Table>
          <THead className="border-b border-foreground/[0.06]">
            <TR>
              <TH>{t("domains.thHost")}</TH>
              <TH className="text-accent-fg">{t("domains.thLink")}</TH>
              <TH className="text-success-fg">{t("domains.thMail")}</TH>
              <TH className="text-right" />
            </TR>
          </THead>
          <TBody className="divide-y divide-foreground/[0.04]">
            {hosts.map((h) => (
              <TR key={h.host} className="group">
                <TD className="font-mono text-xs text-foreground/70">{h.host}</TD>
                <TD>
                  {h.linkEnabled !== null ? (
                    <button
                      disabled={!!busy}
                      onClick={() => toggleHost(h.host, "linkHosts", h.linkEnabled!)}
                      className={`inline-flex items-center gap-1 rounded px-2 py-0.5 text-xs font-medium transition-colors disabled:opacity-50 ${
                        h.linkEnabled
                          ? "bg-primary/25 text-accent-fg hover:bg-primary/40"
                          : "bg-foreground/[0.06] text-foreground/40 hover:bg-foreground/10 line-through"
                      }`}
                    >
                      <span className={`h-1.5 w-1.5 rounded-full ${h.linkEnabled ? "bg-primary" : "bg-foreground/[0.06]"}`} />
                      {h.linkEnabled ? t("domains.on") : t("domains.off")}
                    </button>
                  ) : (
                    <button
                      disabled={!!busy}
                      onClick={() => addHost(h.host, true, false)}
                      className="text-xs text-foreground/50 hover:text-accent-fg transition-colors px-2 py-0.5 disabled:opacity-50"
                    >
                      {t("domains.addShort")}
                    </button>
                  )}
                </TD>
                <TD>
                  {h.mailEnabled !== null ? (
                    <button
                      disabled={!!busy}
                      onClick={() => toggleHost(h.host, "mailHosts", h.mailEnabled!)}
                      className={`inline-flex items-center gap-1 rounded px-2 py-0.5 text-xs font-medium transition-colors disabled:opacity-50 ${
                        h.mailEnabled
                          ? "bg-success-bg text-success-fg border border-success-border hover:brightness-95"
                          : "bg-foreground/[0.06] text-foreground/40 hover:bg-foreground/10 line-through"
                      }`}
                    >
                      <span className={`h-1.5 w-1.5 rounded-full ${h.mailEnabled ? "bg-success-fg" : "bg-foreground/[0.06]"}`} />
                      {h.mailEnabled ? t("domains.on") : t("domains.off")}
                    </button>
                  ) : (
                    <button
                      disabled={!!busy}
                      onClick={() => addHost(h.host, false, true)}
                      className="text-xs text-foreground/50 hover:text-success-fg transition-colors px-2 py-0.5 disabled:opacity-50"
                    >
                      {t("domains.addShort")}
                    </button>
                  )}
                </TD>
                <TD className="text-right">
                  <button
                    disabled={!!busy}
                    onClick={() => removeHost(h.host)}
                    title={t("domains.removeHost")}
                    className="text-xs text-danger-fg/70 hover:text-danger-fg opacity-0 group-hover:opacity-100 transition-all disabled:opacity-30 px-2.5 py-0.5"
                  >
                    {t("domains.remove")}
                  </button>
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
      )}
      <AddHostRow domain={domain} busy={busy === "add"} onAdd={addHost} />
    </div>
  );
}

function AddHostRow({ domain, busy, onAdd }: { domain: Domain; busy: boolean; onAdd: (host: string, forLink: boolean, forMail: boolean) => void; }) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState("");
  const [forLink, setForLink] = useState(true);
  const [forMail, setForMail] = useState(false);

  const existing = useMemo(() => {
    const s = new Set<string>();
    for (const h of domain.linkHosts ?? []) s.add(h.host);
    for (const h of domain.mailHosts ?? []) s.add(h.host);
    return s;
  }, [domain]);

  const suggestions = useMemo(() => {
    const candidates = [
      `go.${domain.name}`,
      `s.${domain.name}`,
      `link.${domain.name}`,
      domain.name,
      `mail.${domain.name}`,
    ];
    return candidates.filter((c) => !existing.has(c));
  }, [domain, existing]);

  function submit() {
    let v = draft.trim().toLowerCase();
    if (v && !v.includes(".")) v = `${v}.${domain.name}`;
    if (v && (forLink || forMail)) {
      onAdd(v, forLink, forMail);
      setDraft("");
    }
  }

  return (
    <div className="flex items-center gap-2 flex-wrap bg-foreground/[0.02] p-2.5 rounded-xl border border-foreground/[0.04]">
      <div className="relative flex-1 min-w-[150px]">
        <input
          className="input h-9 text-xs py-1.5 w-full"
          placeholder={t("domains.hostDraftPlaceholder")}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && submit()}
        />
        {draft && !draft.includes(".") && (
          <span className="pointer-events-none absolute inset-y-0 right-3 flex items-center text-[10px] text-foreground/50">
            → {draft.trim().toLowerCase()}.{domain.name}
          </span>
        )}
      </div>
      <label className="flex items-center gap-1.5 text-xs text-foreground/60 cursor-pointer select-none">
        <input type="checkbox" checked={forLink} onChange={(e) => setForLink(e.target.checked)} className="accent-[var(--primary)]" />
        {t("domains.thLink")}
      </label>
      <label className="flex items-center gap-1.5 text-xs text-foreground/60 cursor-pointer select-none">
        <input type="checkbox" checked={forMail} onChange={(e) => setForMail(e.target.checked)} className="accent-emerald-500" />
        {t("domains.thMail")}
      </label>
      <Button variant="primary" className="h-9 py-1 px-3 text-xs" disabled={busy || !draft.trim() || (!forLink && !forMail)} onClick={submit}>
        {t("domains.addHostButton")}
      </Button>
      {suggestions.length > 0 && (
        <div className="flex flex-wrap gap-1.5 w-full mt-2 px-1">
          {suggestions.slice(0, 4).map((s) => (
            <button
              key={s}
              type="button"
              disabled={busy}
              onClick={() => { setDraft(s); }}
              className="text-[10px] text-foreground/40 hover:text-foreground/70 border border-foreground/[0.05] hover:border-foreground/15 bg-foreground/[0.01] hover:bg-foreground/[0.03] rounded-lg px-2 py-0.5 transition-colors cursor-pointer"
            >
              {s}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

