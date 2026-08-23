import { useEffect, useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { api, Domain, HostEntry, ProviderAccount } from "../../../api";
import { dnsApi, DNSRecord, DNSVerifyResult, HostDNSStatus, LinkHostStatus, DNSRecordStatus } from "../api";
import { Code, Empty, Field, Guide, HostList, Modal, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button, toast, confirmDialog } from "../../../ui";
import { Globe, RefreshCw, Plus, Trash2, ArrowLeft, ArrowRight, ShieldCheck, Mail, Link as LinkIcon, Cloud, Settings, Layers, ListChecks, Server } from "lucide-react";
import { ProviderAccounts } from "./ProviderAccounts";
import { useTranslation } from "../../../i18n";
import { DnsHostRow, LinkHostRow, LinkHostGuide } from "./dnsStatus";
import { DomainEditorForm } from "./DomainEditorForm";
import { DomainHostManager } from "./DomainHostManager";
import { SyncModal } from "./SyncModal";
import { RecordsView } from "./RecordsView";
import { DDNSView } from "./DDNSView";
import { usePluginGate } from "../../PluginGate";
import { roleSatisfies, useCurrentRole } from "../../../shell/role";
import { ListSkeleton } from "../../../components/ListSkeleton";

type DomainSubTab = "records" | "routing" | "verification" | "settings";

export default function DomainsPage() {
  const { role, isInstanceAdmin } = useCurrentRole();
  const canDeleteDomain = roleSatisfies("admin", role, isInstanceAdmin);
  const { t } = useTranslation();
  const [domains, setDomains] = useState<Domain[]>([]);
  const [accounts, setAccounts] = useState<ProviderAccount[]>([]);
  const [active, setActive] = useState<Domain | "new" | null>(null);
  const [activeSubTab, setActiveSubTab] = useState<DomainSubTab>("records");
  const [syncing, setSyncing] = useState(false);
  const [q, setQ] = useState("");
  const [tab, setTab] = useState<"domains" | "ddns" | "settings">("domains");
  const [searchParams, setSearchParams] = useSearchParams();

  useEffect(() => {
    if (searchParams.get("create") === "1") {
      setActive("new");
      setSearchParams(prev => {
        const next = new URLSearchParams(prev);
        next.delete("create");
        return next;
      }, { replace: true });
    }
  }, [searchParams, setSearchParams]);

  const [page, setPage] = useState(0);
  const [hasMore, setHasMore] = useState(true);
  const [loading, setLoading] = useState(false);

  const [dnsStatus, setDnsStatus] = useState<DNSVerifyResult | null>(null);
  const [verifying, setVerifying] = useState(false);

  useEffect(() => {
    setDnsStatus(null);
  }, [active]);

  async function verifyDns(targetDomain?: Domain) {
    const dom = targetDomain || (typeof active === "object" && active !== null ? active : null);
    if (!dom) return;
    setVerifying(true);
    try {
      const res = await dnsApi.verifyDNS(dom.id);
      setDnsStatus(res);
    } catch (e: any) {
      toast.error(e.message || t("domains.verifyFailed"));
    } finally {
      setVerifying(false);
    }
  }

  const pluginGate = usePluginGate();

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

      if (active && active !== "new") {
        const refreshed = res.find(d => d.id === active.id);
        if (refreshed) setActive(refreshed);
      }
    } catch (e: any) {
      if (e.status === 404 || e.status === 402) {
        pluginGate.degrade(e.status);
      }
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    const timer = setTimeout(() => {
      loadMore(true);
    }, 200);
    return () => clearTimeout(timer);
  }, [q]);

  useEffect(() => {
    api.providerAccounts().then(setAccounts).catch(() => setAccounts([]));
  }, []);

  async function toggleService(domain: Domain, field: "forLink" | "forMail") {
    const current = field === "forLink" ? domain.forLink : domain.forMail;
    try {
      const res = await dnsApi.updateDomain(domain.id, { [field]: !current });
      loadMore(true);
      if (active && active !== "new" && active.id === domain.id) {
        setActive(res || { ...active, [field]: !current });
      }
    } catch (e: any) {
      toast.error(e.message || t("domains.updateFailed"));
    }
  }

  const linkCount = useMemo(() => domains.filter(d => d.forLink).length, [domains]);
  const mailCount = useMemo(() => domains.filter(d => d.forMail).length, [domains]);

  function getProviderName(dom: Domain) {
    if (!dom.providerAccountId) return t("domains.providerManual");
    const acc = accounts.find(a => a.id === dom.providerAccountId);
    return acc ? `${acc.name} (${acc.type})` : t("domains.provider");
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
            <Button variant="primary" onClick={() => { setActive("new"); }} className="gap-1.5 py-1.5 text-xs">
              <Plus className="h-3.5 w-3.5" />
              {t("domains.addDomain")}
            </Button>
          </div>
        }
      />

      <div className="flex gap-0 border-b border-foreground/[0.06] mb-6 overflow-x-auto [scrollbar-width:none] [-ms-overflow-style:none] [&::-webkit-scrollbar]:hidden">
        <button
          onClick={() => { setTab("domains"); }}
          className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors shrink-0 whitespace-nowrap ${
            tab === "domains"
              ? "border-primary text-foreground"
              : "border-transparent text-foreground/45 hover:text-foreground/70"
          }`}
        >
          {t("domains.tabDns")}
        </button>
        <button
          onClick={() => { setTab("ddns"); }}
          className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors flex items-center gap-1.5 shrink-0 whitespace-nowrap ${
            tab === "ddns"
              ? "border-primary text-foreground"
              : "border-transparent text-foreground/45 hover:text-foreground/70"
          }`}
        >
          {t("domains.tabDdns")}
        </button>
        <button
          onClick={() => { setTab("settings"); }}
          className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors flex items-center gap-1.5 shrink-0 whitespace-nowrap ${
            tab === "settings"
              ? "border-primary text-foreground"
              : "border-transparent text-foreground/45 hover:text-foreground/70"
          }`}
        >
          {t("domains.tabSettings")}
        </button>
      </div>

      {tab === "domains" && (
        <>
          {active === "new" ? (
            <div className="space-y-4">
              <div className="flex items-center gap-2">
                <Button variant="ghost" className="gap-1.5 text-xs py-1.5" onClick={() => setActive(null)}>
                  <ArrowLeft className="h-4 w-4" />
                  {t("domains.backToDomains")}
                </Button>
              </div>
              <GlassCard className="p-6">
                <h2 className="mb-4 text-lg font-bold text-foreground flex items-center gap-2">
                  <Globe className="h-5 w-5 text-accent-fg" />
                  {t("domains.addDomainZone")}
                </h2>
                <DomainEditorForm
                  domain={null}
                  accounts={accounts}
                  onCancel={() => setActive(null)}
                  onSaved={(savedDomain) => {
                    loadMore(true);
                    setActive(savedDomain || null);
                    setActiveSubTab("records");
                  }}
                />
              </GlassCard>
            </div>
          ) : active ? (
            /* Dedicated Domain Detail Workspace */
            <div className="space-y-6">
              <div className="flex flex-wrap items-center justify-between gap-4 pb-2 border-b border-foreground/[0.06]">
                <div className="flex items-center gap-3">
                  <Button
                    variant="subtle"
                    className="gap-1.5 text-xs py-1.5 px-3"
                    onClick={() => {
                      setActive(null);
                      setDnsStatus(null);
                    }}
                  >
                    <ArrowLeft className="h-4 w-4" />
                    {t("domains.backToDomains")}
                  </Button>
                  <div className="flex items-center gap-2">
                    <Globe className="h-5 w-5 text-accent-fg" />
                    <h2 className="font-mono text-xl font-bold text-foreground">{active.name}</h2>
                    <Badge tone="neutral" className="text-xs font-mono ml-1">
                      {getProviderName(active)}
                    </Badge>
                  </div>
                </div>

                <div className="flex items-center gap-2">
                  {canDeleteDomain && (
                    <Button
                      variant="danger"
                      onClick={async () => {
                        if (await confirmDialog(t("domains.removeConfirm", { name: active.name }))) {
                          await dnsApi.deleteDomain(active.id);
                          setActive(null);
                          loadMore(true);
                        }
                      }}
                      className="py-1 px-3 text-xs"
                    >
                      <Trash2 className="h-3.5 w-3.5 mr-1" />
                      {t("domains.delete")}
                    </Button>
                  )}
                </div>
              </div>

              {/* Sub Navigation Tabs */}
              <div className="flex gap-2 border-b border-foreground/[0.06] overflow-x-auto [scrollbar-width:none]">
                <button
                  type="button"
                  onClick={() => setActiveSubTab("records")}
                  className={`flex items-center gap-2 px-4 py-2.5 text-sm font-medium border-b-2 -mb-px transition-colors whitespace-nowrap ${
                    activeSubTab === "records"
                      ? "border-primary text-foreground font-semibold"
                      : "border-transparent text-foreground/50 hover:text-foreground/80"
                  }`}
                >
                  <Layers className="h-4 w-4 text-accent-fg" />
                  {t("domains.domainRecords")}
                </button>
                <button
                  type="button"
                  onClick={() => setActiveSubTab("routing")}
                  className={`flex items-center gap-2 px-4 py-2.5 text-sm font-medium border-b-2 -mb-px transition-colors whitespace-nowrap ${
                    activeSubTab === "routing"
                      ? "border-primary text-foreground font-semibold"
                      : "border-transparent text-foreground/50 hover:text-foreground/80"
                  }`}
                >
                  <LinkIcon className="h-4 w-4 text-accent-fg" />
                  {t("domains.domainRouting")}
                </button>
                <button
                  type="button"
                  onClick={() => setActiveSubTab("verification")}
                  className={`flex items-center gap-2 px-4 py-2.5 text-sm font-medium border-b-2 -mb-px transition-colors whitespace-nowrap ${
                    activeSubTab === "verification"
                      ? "border-primary text-foreground font-semibold"
                      : "border-transparent text-foreground/50 hover:text-foreground/80"
                  }`}
                >
                  <ShieldCheck className="h-4 w-4 text-success-fg" />
                  {t("domains.domainHealth")}
                </button>
                <button
                  type="button"
                  onClick={() => setActiveSubTab("settings")}
                  className={`flex items-center gap-2 px-4 py-2.5 text-sm font-medium border-b-2 -mb-px transition-colors whitespace-nowrap ${
                    activeSubTab === "settings"
                      ? "border-primary text-foreground font-semibold"
                      : "border-transparent text-foreground/50 hover:text-foreground/80"
                  }`}
                >
                  <Settings className="h-4 w-4 text-foreground/60" />
                  {t("domains.domainSettings")}
                </button>
              </div>

              {/* Sub-tab 1: DNS Records */}
              {activeSubTab === "records" && (
                <GlassCard className="p-6">
                  <div className="mb-4 flex items-center justify-between">
                    <div>
                      <h3 className="text-base font-semibold text-foreground">{t("domains.dnsRecords")}</h3>
                      <p className="text-xs text-foreground/50">{t("domains.pageDescription")}</p>
                    </div>
                  </div>
                  <RecordsView domain={active} />
                </GlassCard>
              )}

              {/* Sub-tab 2: Subdomain Routing */}
              {activeSubTab === "routing" && (
                <GlassCard className="p-6 space-y-5">
                  <div>
                    <h3 className="text-base font-semibold text-foreground">{t("domains.managedHosts")}</h3>
                    <p className="text-xs text-foreground/50 mt-1">
                      {t("domains.syncToggleHint")}
                    </p>
                  </div>
                  <DomainHostManager
                    domain={active}
                    onReload={async (updatedDomain?: Domain) => {
                      loadMore(true);
                      if (updatedDomain) {
                        setActive(updatedDomain);
                      } else {
                        try {
                          const res = await api.domains({ q: active.name, limit: 50, offset: 0 });
                          const updated = res.find(d => d.id === active.id);
                          if (updated) setActive(updated);
                        } catch {
                          /* ignore reload error */
                        }
                      }
                    }}
                  />
                </GlassCard>
              )}

              {/* Sub-tab 3: Health & Verification */}
              {activeSubTab === "verification" && (
                <GlassCard className="p-6 space-y-6">
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <div>
                      <h3 className="text-base font-semibold text-foreground">{t("domains.dnsSetupVerification")}</h3>
                      <p className="text-xs text-foreground/50 mt-1 leading-relaxed">
                        {t("domains.verificationHint")}
                      </p>
                    </div>
                    <Button
                      variant="primary"
                      onClick={() => verifyDns(active)}
                      disabled={verifying}
                      className="text-xs py-1.5 px-3.5 gap-1.5"
                    >
                      <RefreshCw className={`h-3.5 w-3.5 ${verifying ? "animate-spin" : ""}`} />
                      {verifying ? t("domains.verifying") : t("domains.verifyDnsSetup")}
                    </Button>
                  </div>

                  {dnsStatus === null ? (
                    <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 pt-2">
                      {(["SPF", "DKIM", "DMARC"] as const).map((label) => (
                        <div key={label} className="flex flex-col items-center p-4 rounded-xl bg-well border border-foreground/[0.05]">
                          <span className="text-xs uppercase font-bold text-foreground/45 tracking-wider">{t("domains.statusLabel", { label })}</span>
                          <div className="mt-2"><Badge tone="neutral">{t("domains.unknown")}</Badge></div>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <div className="space-y-5 pt-2">
                      <div className="space-y-3">
                        <span className="text-xs uppercase font-bold text-foreground/50 tracking-wider">{t("domains.mailHosts")}</span>
                        {(dnsStatus.hosts?.length
                          ? dnsStatus.hosts
                          : [{ host: active.name, spf: dnsStatus.spf, dmarc: dnsStatus.dmarc, dkim: dnsStatus.dkim }]
                        ).map((host) => (
                          <DnsHostRow key={host.host} host={host} />
                        ))}
                      </div>
                      {!!dnsStatus.links?.length && (
                        <div className="space-y-3">
                          <span className="text-xs uppercase font-bold text-foreground/50 tracking-wider">{t("domains.shortLinkHosts")}</span>
                          {dnsStatus.links.map((lh) => (
                            <LinkHostRow key={lh.host} link={lh} />
                          ))}
                        </div>
                      )}
                    </div>
                  )}
                  <LinkHostGuide apex={active.name} />
                </GlassCard>
              )}

              {/* Sub-tab 4: Domain Settings */}
              {activeSubTab === "settings" && (
                <GlassCard className="p-6">
                  <h3 className="text-base font-semibold text-foreground mb-4">{t("domains.domainSettings")}</h3>
                  <DomainEditorForm
                    key={active.id}
                    domain={active}
                    accounts={accounts}
                    onCancel={() => setActive(null)}
                    onSaved={(d) => {
                      if (d) setActive(d);
                      loadMore(true);
                      toast.success(t("domains.saveBasicInfo"));
                    }}
                  />
                </GlassCard>
              )}
            </div>
          ) : domains.length === 0 && !loading ? (
            /* Empty state (shared Empty component) */
            <Empty
              reason={t("domains.addFirstDomain")}
              detail={
                <>
                  <p className="text-sm text-foreground/50 leading-relaxed">{t("domains.addFirstDomainHint")}</p>
                  {accounts.length > 0 ? (
                    <p className="mt-3 text-xs text-foreground/45">
                      {t("domains.connectedProvidersPre")} <span className="font-mono tnum">{accounts.length}</span>
                    </p>
                  ) : (
                    <p className="mt-3 text-xs leading-relaxed text-foreground/45">{t("domains.providerBlockerDetail")}</p>
                  )}
                </>
              }
              action={
                <div className="flex flex-col items-center gap-2">
                  {accounts.length > 0 ? (
                    <Button variant="primary" onClick={() => setSyncing(true)} className="gap-1.5">
                      <RefreshCw className="h-4 w-4" />
                      {t("domains.syncFrom", { name: accounts.length === 1 ? accounts[0].name : t("domains.provider") })}
                    </Button>
                  ) : (
                    <Button variant="primary" onClick={() => setTab("settings")} className="gap-1.5">
                      <Plus className="h-4 w-4" />
                      {t("domains.connectProvider")}
                    </Button>
                  )}
                  <button
                    onClick={() => setActive("new")}
                    className="text-xs text-foreground/45 hover:text-foreground/70 underline underline-offset-2 transition-colors cursor-pointer"
                  >
                    {t("domains.orAddManually")}
                  </button>
                </div>
              }
            >
              <div className="h-14 w-14 rounded-2xl bg-accent-soft flex items-center justify-center text-accent-fg">
                <Globe className="h-7 w-7" />
              </div>
            </Empty>
          ) : (
            /* Primary Domain Hub (List / Grid View) */
            <div className="space-y-4">
              {/* Filter bar and quick stats */}
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div className="flex items-center gap-2 flex-1 max-w-md">
                  <input
                    className="input w-full text-sm"
                    placeholder={t("domains.searchDomains")}
                    value={q}
                    onChange={(e) => setQ(e.target.value)}
                  />
                  {q && (
                    <Button variant="ghost" className="text-xs py-1.5" onClick={() => setQ("")}>
                      {t("domains.emptyFilteredAction")}
                    </Button>
                  )}
                </div>
                <div className="flex items-center gap-2 text-xs text-foreground/60 font-medium">
                  <span className="bg-well px-2.5 py-1 rounded-lg border border-foreground/[0.06]">
                    {t("domains.allDomainsCount", { count: domains.length })}
                  </span>
                  <span className="bg-well px-2.5 py-1 rounded-lg border border-foreground/[0.06] text-accent-fg flex items-center gap-1">
                    <LinkIcon className="h-3 w-3" />
                    <span>{t("domains.linkEnabledCount", { count: linkCount })}</span>
                  </span>
                  <span className="bg-well px-2.5 py-1 rounded-lg border border-foreground/[0.06] text-success-fg flex items-center gap-1">
                    <Mail className="h-3 w-3" />
                    <span>{t("domains.mailEnabledCount", { count: mailCount })}</span>
                  </span>
                </div>
              </div>

              {loading && domains.length === 0 ? (
                <ListSkeleton rows={6} ariaLabel={t("domains.loading")} />
              ) : domains.length === 0 && q ? (
                <GlassCard className="flex flex-col items-center gap-3 p-8 text-center">
                  <p className="text-sm text-foreground/60">
                    {t("domains.emptyFilteredReason")} <span className="font-mono text-foreground/80">{`“${q}”`}</span>
                  </p>
                  <Button variant="ghost" className="text-xs py-1.5" onClick={() => setQ("")}>
                    {t("domains.emptyFilteredAction")}
                  </Button>
                </GlassCard>
              ) : (
                <div className="grid grid-cols-1 gap-3">
                  {domains.map((d) => (
                    <div
                      key={d.id}
                      className="glass p-4 rounded-2xl border border-foreground/[0.06] hover:border-foreground/20 transition-all cursor-pointer group"
                      onClick={() => {
                        setActive(d);
                        setActiveSubTab("records");
                      }}
                    >
                      <div className="flex flex-wrap items-center justify-between gap-4">
                        <div className="flex items-center gap-3 min-w-0">
                          <div className="h-10 w-10 rounded-xl bg-accent-soft flex items-center justify-center text-accent-fg shrink-0 group-hover:scale-105 transition-transform">
                            <Globe className="h-5 w-5" />
                          </div>
                          <div className="min-w-0">
                            <div className="flex items-center gap-2">
                              <span className="font-mono font-bold text-base text-foreground truncate">{d.name}</span>
                              <Badge tone="neutral" className="text-[11px] font-mono shrink-0">
                                {getProviderName(d)}
                              </Badge>
                            </div>
                            {d.note && (
                              <div className="truncate text-xs text-foreground/50 mt-1">{d.note}</div>
                            )}
                          </div>
                        </div>

                        <div className="flex items-center gap-3 shrink-0">
                          {/* Service Toggles */}
                          <div className="flex items-center gap-2 bg-well px-3 py-1.5 rounded-xl border border-foreground/[0.05]">
                            <button
                              type="button"
                              className={`flex items-center gap-1 text-xs px-2 py-0.5 rounded-md transition-colors cursor-pointer ${
                                d.forLink
                                  ? "bg-accent text-accent-fg font-medium"
                                  : "text-foreground/40 hover:text-foreground/70"
                              }`}
                              title={t("domains.toggleLinkRouting")}
                              onClick={(e) => {
                                e.stopPropagation();
                                toggleService(d, "forLink");
                              }}
                            >
                              <LinkIcon className="h-3 w-3" />
                              <span>{t("domains.thLink")}</span>
                            </button>
                            <button
                              type="button"
                              className={`flex items-center gap-1 text-xs px-2 py-0.5 rounded-md transition-colors cursor-pointer ${
                                d.forMail
                                  ? "bg-success-bg text-success-fg border border-success-border font-medium"
                                  : "text-foreground/40 hover:text-foreground/70"
                              }`}
                              title={t("domains.toggleMailRouting")}
                              onClick={(e) => {
                                e.stopPropagation();
                                toggleService(d, "forMail");
                              }}
                            >
                              <Mail className="h-3 w-3" />
                              <span>{t("domains.thMail")}</span>
                            </button>
                          </div>

                          {/* Quick Actions */}
                          <Button
                            variant="primary"
                            className="text-xs py-1.5 px-3 gap-1.5"
                            onClick={(e) => {
                              e.stopPropagation();
                              setActive(d);
                              setActiveSubTab("records");
                            }}
                          >
                            <Layers className="h-3.5 w-3.5" />
                            {t("domains.manageZone")}
                          </Button>
                          <Button
                            variant="subtle"
                            className="text-xs py-1.5 px-2.5"
                            title={t("domains.domainHealth")}
                            onClick={(e) => {
                              e.stopPropagation();
                              setActive(d);
                              setActiveSubTab("verification");
                              verifyDns(d);
                            }}
                          >
                            <ShieldCheck className="h-3.5 w-3.5" />
                          </Button>
                          <Button
                            variant="subtle"
                            className="text-xs py-1.5 px-2.5"
                            title={t("domains.domainSettings")}
                            onClick={(e) => {
                              e.stopPropagation();
                              setActive(d);
                              setActiveSubTab("settings");
                            }}
                          >
                            <Settings className="h-3.5 w-3.5" />
                          </Button>
                        </div>
                      </div>
                    </div>
                  ))}
                  {loading && <div className="p-3 text-center text-xs text-foreground/40">{t("domains.loading")}</div>}
                </div>
              )}
            </div>
          )}
        </>
      )}

      {tab === "ddns" && <DDNSView domains={domains} />}

      {tab === "settings" && (
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
