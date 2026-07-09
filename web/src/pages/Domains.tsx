import { useEffect, useMemo, useState } from "react";
import { api, DNSRecord, DNSVerifyResult, HostDNSStatus, LinkHostStatus, DNSRecordStatus, Domain, HostEntry, ProviderAccount } from "../api";
import { Code, Empty, Field, Guide, HostList, Modal, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button } from "../ui";
import { Globe, RefreshCw, Plus, Trash2, ArrowRight, ShieldCheck, Mail, Link as LinkIcon, Cloud } from "lucide-react";
import { ProviderAccounts } from "./Settings";
import { useTranslation } from "../i18n";
import { DnsHostRow, LinkHostRow, LinkHostGuide } from "./domains/dnsStatus";
import { DomainEditorForm } from "./domains/DomainEditorForm";
import { DomainHostManager } from "./domains/DomainHostManager";
import { SyncModal } from "./domains/SyncModal";
import { RecordsView } from "./domains/RecordsView";

export default function DomainsPage() {
  const { t } = useTranslation();
  const [domains, setDomains] = useState<Domain[]>([]);
  const [accounts, setAccounts] = useState<ProviderAccount[]>([]);
  const [active, setActive] = useState<Domain | "new" | null>(null);
  const [syncing, setSyncing] = useState(false);
  const [q, setQ] = useState("");
  const [tab, setTab] = useState<'domains' | 'settings'>('domains');

  const [page, setPage] = useState(0);
  const [hasMore, setHasMore] = useState(true);
  const [loading, setLoading] = useState(false);

  const [dnsStatus, setDnsStatus] = useState<DNSVerifyResult | null>(null);
  const [verifying, setVerifying] = useState(false);

  useEffect(() => {
    setDnsStatus(null);
  }, [active]);

  async function verifyDns() {
    if (!active || active === "new") return;
    setVerifying(true);
    try {
      const res = await api.verifyDNS(active.id);
      setDnsStatus(res);
    } catch (e: any) {
      alert(e.message || t("domains.verifyFailed"));
    } finally {
      setVerifying(false);
    }
  }

  async function loadMore(reset = false) {
    if (loading || (!hasMore && !reset)) return;
    setLoading(true);
    try {
      const limit = 50;
      const offset = reset ? 0 : page * limit;
      const res = await api.domains({ q, limit, offset });
      if (res.length < limit) setHasMore(false);
      else setHasMore(true);

      setDomains(prev => reset ? res : [...prev, ...res]);
      setPage(reset ? 1 : page + 1);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    const t = setTimeout(() => {
      loadMore(true);
    }, 200);
    return () => clearTimeout(t);
  }, [q]);

  useEffect(() => {
    api.providerAccounts().then(setAccounts).catch(() => setAccounts([]));
  }, []);

  const handleScroll = (e: React.UIEvent<HTMLDivElement>) => {
    const bottom = e.currentTarget.scrollHeight - e.currentTarget.scrollTop <= e.currentTarget.clientHeight + 100;
    if (bottom) loadMore();
  };

  async function toggleService(domain: Domain, field: "forLink" | "forMail") {
    const current = field === "forLink" ? domain.forLink : domain.forMail;
    await api.updateDomain(domain.id, { [field]: !current });
    loadMore(true);
    if (active && active !== "new" && active.id === domain.id) {
      setActive({ ...active, [field]: !current });
    }
  }

  return (
    <ScreenWrap>
      <PageHeader
        title={t("domains.pageTitle")}
        description={t("domains.pageDescription")}
        action={
          <div className="flex gap-2">
            <Button variant="ghost" onClick={() => setSyncing(true)} className="gap-1.5 py-1.5 text-xs">
              <RefreshCw className="h-3.5 w-3.5" />
              {t("domains.syncCloudflare")}
            </Button>
            <Button variant="primary" onClick={() => setActive("new")} className="gap-1.5 py-1.5 text-xs">
              <Plus className="h-3.5 w-3.5" />
              {t("domains.addDomain")}
            </Button>
          </div>
        }
      />

      <div className="flex gap-0 border-b border-white/[0.06] mb-6">
        <button
          onClick={() => setTab('domains')}
          className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors ${
            tab === 'domains'
              ? 'border-indigo-500 text-white'
              : 'border-transparent text-white/45 hover:text-white/70'
          }`}
        >
          {t("domains.tabDns")}
        </button>
        <button
          onClick={() => setTab('settings')}
          className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors flex items-center gap-1.5 ${
            tab === 'settings'
              ? 'border-indigo-500 text-white'
              : 'border-transparent text-white/45 hover:text-white/70'
          }`}
        >
          {t("domains.tabSettings")}
        </button>
      </div>

      {tab === 'domains' && (

      <div className="grid grid-cols-1 lg:grid-cols-[300px_1fr] gap-6 min-h-0 items-start">
        {/* Left list column */}
        <div className="flex flex-col min-h-0 w-full">
          <div className="mb-3">
            <input
              className="input w-full"
              placeholder={t("domains.searchDomains")}
              value={q}
              onChange={(e) => setQ(e.target.value)}
            />
          </div>
          <GlassCard className="overflow-hidden">
            <div className="overflow-y-auto max-h-[600px] divide-y divide-white/[0.04]" onScroll={handleScroll}>
              {domains.length === 0 && !loading ? (
                <div className="p-8 text-center text-white/40 text-sm">{t("domains.noDomainsFound")}</div>
              ) : (
                <>
                  {domains.map((d) => (
                    <div
                      key={d.id}
                      className={`flex w-full flex-col p-4 text-left hover:bg-white/[0.03] transition-colors cursor-pointer ${
                        active !== "new" && active?.id === d.id ? "bg-white/[0.05]" : ""
                      }`}
                      onClick={() => setActive(d)}
                    >
                      <div className="flex items-center justify-between w-full gap-2">
                        <span className="font-semibold text-sm truncate flex-1 text-white">{d.name}</span>
                        <div className="flex gap-2 shrink-0">
                          <button
                            className="p-1 hover:bg-white/10 rounded transition-colors"
                            title={t("domains.toggleLinkRouting")}
                            onClick={(e) => { e.stopPropagation(); toggleService(d, "forLink"); }}
                          >
                            <LinkIcon className={`h-3.5 w-3.5 ${d.forLink ? "text-indigo-400" : "text-white/20"}`} />
                          </button>
                          <button
                            className="p-1 hover:bg-white/10 rounded transition-colors"
                            title={t("domains.toggleMailRouting")}
                            onClick={(e) => { e.stopPropagation(); toggleService(d, "forMail"); }}
                          >
                            <Mail className={`h-3.5 w-3.5 ${d.forMail ? "text-emerald-400" : "text-white/20"}`} />
                          </button>
                        </div>
                      </div>
                      {d.note && <div className="truncate text-[11px] text-amber-300/70 mt-1.5 font-medium">📝 {d.note}</div>}
                    </div>
                  ))}
                  {loading && <div className="p-3 text-center text-xs text-white/40">{t("domains.loading")}</div>}
                </>
              )}
            </div>
          </GlassCard>
        </div>

        {/* Right content column */}
        <div className="w-full space-y-5">
          {active === "new" ? (
            <GlassCard className="p-5">
              <h2 className="mb-4 text-lg font-bold text-white flex items-center gap-2">
                <Globe className="h-5 w-5 text-indigo-400" />
                {t("domains.addDomainZone")}
              </h2>
              <DomainEditorForm
                domain={null}
                accounts={accounts}
                onCancel={() => setActive(null)}
                onSaved={(savedDomain) => {
                  loadMore(true);
                  setActive(savedDomain || null);
                }}
              />
            </GlassCard>
          ) : active ? (
            <div className="space-y-6">
              <GlassCard className="p-5">
                <div className="flex justify-between items-center mb-5 border-b border-white/[0.06] pb-4">
                  <h2 className="text-xl font-bold text-white flex items-center gap-2">
                    <Globe className="h-5 w-5 text-indigo-400" />
                    {active.name}
                  </h2>
                  <Button
                    variant="danger"
                    onClick={async () => {
                      if (confirm(t("domains.removeConfirm", { name: active.name }))) {
                        await api.deleteDomain(active.id);
                        setActive(null);
                        loadMore(true);
                      }
                    }}
                    className="py-1 px-2.5 text-xs bg-rose-500/10 hover:bg-rose-500/20 text-rose-300 border-0"
                  >
                    <Trash2 className="h-3.5 w-3.5 mr-1" />
                    {t("domains.delete")}
                  </Button>
                </div>
                
                <DomainEditorForm
                  key={active.id}
                  domain={active}
                  accounts={accounts}
                  onCancel={() => setActive(null)}
                  onSaved={(d) => {
                    if (d) setActive(d);
                    loadMore(true);
                  }}
                />
              </GlassCard>
              
              <GlassCard className="p-5">
                <h3 className="mb-4 text-sm font-semibold text-white/80 uppercase tracking-wider">{t("domains.managedHosts")}</h3>
                <DomainHostManager
                  domain={active}
                  onReload={async () => {
                    loadMore(true);
                    const res = await api.domains({ q: active.name, limit: 1, offset: 0 });
                    const updated = res.find(d => d.id === active.id);
                    if (updated) setActive(updated);
                  }}
                />
              </GlassCard>

              <GlassCard className="p-5 space-y-4">
                <div className="flex items-center justify-between">
                  <h3 className="text-sm font-semibold text-white/80 uppercase tracking-wider">{t("domains.dnsSetupVerification")}</h3>
                  <Button 
                    variant="subtle" 
                    onClick={verifyDns}
                    disabled={verifying}
                    className="text-xs py-1 px-2.5"
                  >
                    {verifying ? t("domains.verifying") : t("domains.verifyDnsSetup")}
                  </Button>
                </div>
                <p className="text-xs text-white/50 leading-relaxed">
                  {t("domains.verificationHint")}
                </p>
                {dnsStatus === null ? (
                  <div className="grid grid-cols-3 gap-3 pt-2">
                    {(["SPF", "DKIM", "DMARC"] as const).map((label) => (
                      <div key={label} className="flex flex-col items-center p-3.5 rounded-xl bg-white/[0.02] border border-white/[0.04]">
                        <span className="text-[10px] uppercase font-bold text-white/40 tracking-wider">{t("domains.statusLabel", { label })}</span>
                        <div className="mt-2"><Badge tone="neutral">{t("domains.unknown")}</Badge></div>
                      </div>
                    ))}
                  </div>
                ) : (
                  <div className="space-y-4 pt-2">
                    <div className="space-y-3">
                      <span className="text-[10px] uppercase font-bold text-white/50 tracking-wider">{t("domains.mailHosts")}</span>
                      {(dnsStatus.hosts?.length
                        ? dnsStatus.hosts
                        : [{ host: active.name, spf: dnsStatus.spf, dmarc: dnsStatus.dmarc, dkim: dnsStatus.dkim }]
                      ).map((host) => (
                        <DnsHostRow key={host.host} host={host} />
                      ))}
                    </div>
                    {!!dnsStatus.links?.length && (
                      <div className="space-y-2">
                        <span className="text-[10px] uppercase font-bold text-white/50 tracking-wider">{t("domains.shortLinkHosts")}</span>
                        {dnsStatus.links.map((lh) => (
                          <LinkHostRow key={lh.host} link={lh} />
                        ))}
                      </div>
                    )}
                  </div>
                )}
                <LinkHostGuide apex={active.name} />
              </GlassCard>

              <GlassCard className="p-5">
                <h3 className="mb-4 text-sm font-semibold text-white/80 uppercase tracking-wider">{t("domains.dnsRecords")}</h3>
                <RecordsView domain={active} />
              </GlassCard>
            </div>
          ) : domains.length === 0 && !loading ? (
            <GlassCard className="flex flex-col items-center justify-center py-16 px-6 text-center border border-white/[0.04]/40">
              <div className="h-14 w-14 rounded-2xl bg-indigo-500/10 flex items-center justify-center text-indigo-400 mb-4">
                <Globe className="h-7 w-7" />
              </div>
              <h3 className="text-lg font-bold text-white mb-1.5">{t("domains.addFirstDomain")}</h3>
              <p className="text-sm text-white/50 max-w-sm leading-relaxed mb-6">
                {t("domains.addFirstDomainHint")}
              </p>
              {accounts.length > 0 ? (
                <Button variant="primary" onClick={() => setSyncing(true)} className="gap-1.5">
                  <RefreshCw className="h-4 w-4" />
                  {t("domains.syncFrom", { name: accounts.length === 1 ? accounts[0].name : t("domains.provider") })}
                </Button>
              ) : (
                <Button variant="primary" onClick={() => setTab('settings')} className="gap-1.5">
                  <Plus className="h-4 w-4" />
                  {t("domains.connectProvider")}
                </Button>
              )}
              <button
                onClick={() => setActive("new")}
                className="mt-3 text-xs text-white/45 hover:text-white/70 underline underline-offset-2 transition-colors"
              >
                {t("domains.orAddManually")}
              </button>
            </GlassCard>
          ) : (
            <GlassCard className="flex flex-col items-center justify-center py-20 px-6 text-center text-white/40 border border-white/[0.04]/40">
              <Globe className="h-10 w-10 mb-2 opacity-50 text-indigo-400" />
              <p className="text-sm">{t("domains.selectDomainHint")}</p>
            </GlassCard>
          )}
        </div>
      </div>
      
      )
      }
      {tab === 'settings' && (
        <GlassCard className="p-6">
          <ProviderAccounts />
        </GlassCard>
      )}

      {syncing && (
        <SyncModal
          accounts={accounts}
          onClose={() => setSyncing(false)}
          onSynced={() => {
            setSyncing(false);
            loadMore(true);
          }}
        />
      )}
    </ScreenWrap>
  );
}

// Three-state badge: healthy (green), present-but-misconfigured (amber), missing (red).
