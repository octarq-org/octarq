import { useEffect, useMemo, useState } from "react";
import { api, DNSRecord, DNSVerifyResult, HostDNSStatus, LinkHostStatus, DNSRecordStatus, Domain, HostEntry, ProviderAccount } from "../api";
import { Code, Empty, Field, Guide, HostList, Modal, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button } from "../ui";
import { Globe, RefreshCw, Plus, Trash2, ArrowRight, ShieldCheck, Mail, Link as LinkIcon, Cloud } from "lucide-react";
import { ProviderAccounts } from "./Settings";
import { useTranslation } from "../i18n";

interface HostRow {
  host: string;
  linkEnabled: boolean | null;
  mailEnabled: boolean | null;
}

function mergeHosts(domain: Domain): HostRow[] {
  const map = new Map<string, HostRow>();
  for (const h of domain.linkHosts ?? []) {
    map.set(h.host, { host: h.host, linkEnabled: h.enabled, mailEnabled: null });
  }
  for (const h of domain.mailHosts ?? []) {
    const v = map.get(h.host);
    if (v) v.mailEnabled = h.enabled;
    else map.set(h.host, { host: h.host, linkEnabled: null, mailEnabled: h.enabled });
  }
  return Array.from(map.values());
}

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
                      <span className="text-[10px] uppercase font-bold text-white/35 tracking-wider">{t("domains.mailHosts")}</span>
                      {(dnsStatus.hosts?.length
                        ? dnsStatus.hosts
                        : [{ host: active.name, spf: dnsStatus.spf, dmarc: dnsStatus.dmarc, dkim: dnsStatus.dkim }]
                      ).map((host) => (
                        <DnsHostRow key={host.host} host={host} />
                      ))}
                    </div>
                    {!!dnsStatus.links?.length && (
                      <div className="space-y-2">
                        <span className="text-[10px] uppercase font-bold text-white/35 tracking-wider">{t("domains.shortLinkHosts")}</span>
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
function DnsStatusBadge({ status, label }: { status: DNSRecordStatus; label: string }) {
  const { t } = useTranslation();
  const tone = status.healthy ? "green" : status.set ? "amber" : "red";
  const text = status.healthy ? t("domains.configured") : status.set ? t("domains.misconfigured") : t("domains.missing");
  return (
    <div className="flex flex-col items-center p-3 rounded-xl bg-white/[0.02] border border-white/[0.04]">
      <span className="text-[10px] uppercase font-bold text-white/40 tracking-wider">{label}</span>
      <div className="mt-2"><Badge tone={tone as any}>{text}</Badge></div>
    </div>
  );
}

function DnsHostRow({ host }: { host: HostDNSStatus }) {
  const dkimLabel = host.dkim.selector ? `DKIM (${host.dkim.selector})` : "DKIM";
  return (
    <div className="rounded-xl bg-white/[0.015] border border-white/[0.04] p-3">
      <div className="flex items-center gap-2 mb-2.5 px-1">
        <Mail className="h-3.5 w-3.5 text-indigo-400/70 shrink-0" />
        <span className="text-xs font-mono text-white/70 truncate">{host.host}</span>
      </div>
      <div className="grid grid-cols-3 gap-2.5">
        <DnsStatusBadge status={host.spf} label="SPF" />
        <DnsStatusBadge status={host.dkim} label={dkimLabel} />
        <DnsStatusBadge status={host.dmarc} label="DMARC" />
      </div>
    </div>
  );
}

// Short-link host: CNAME confirmed into zone (green), resolves but target
// unverified — e.g. proxied/A-record (amber), or not resolving (red).
function LinkHostRow({ link }: { link: LinkHostStatus }) {
  const { t } = useTranslation();
  const tone = link.healthy ? "green" : link.set ? "amber" : "red";
  const text = link.healthy ? t("domains.pointsToZone") : link.set ? t("domains.unverified") : t("domains.notResolving");
  const detail = link.healthy
    ? `CNAME → ${link.cname}`
    : link.set
      ? (link.cname ? `CNAME → ${link.cname}` : t("domains.resolvesNoCname", { target: link.target }))
      : t("domains.addCname", { target: link.target });
  return (
    <div className="flex items-center gap-3 rounded-xl bg-white/[0.015] border border-white/[0.04] p-3">
      <LinkIcon className="h-3.5 w-3.5 text-indigo-400/70 shrink-0" />
      <div className="min-w-0 flex-1">
        <div className="text-xs font-mono text-white/70 truncate">{link.host}</div>
        <div className="text-[10px] text-white/40 truncate font-mono">{detail}</div>
      </div>
      <Badge tone={tone as any}>{text}</Badge>
    </div>
  );
}

// LinkHostGuide explains how to point a short-link subdomain at this app.
function LinkHostGuide({ apex }: { apex: string }) {
  const { t } = useTranslation();
  return (
    <Guide title={t("domains.guideTitle")}>
      <p>{t("domains.guideEachHost")}<Code>{`go.${apex}`}</Code>) {t("domains.guideIntro")}</p>
      <ul className="list-disc pl-4 space-y-1">
        <li><b>CNAME</b> {t("domains.guideCnameTo")} <Code>{apex}</Code> {t("domains.guideCnameRec")}</li>
        <li>{t("domains.guideOrAdd")} <b>A / AAAA</b> {t("domains.guideAaaaRec")}</li>
        <li>{t("domains.guideProxiedIntro")} <b>{t("domains.guideProxiedWord")}</b> {t("domains.guideProxiedMid")} <b>{t("domains.guideUnverifiedWord")}</b> {t("domains.guideProxiedEnd")}</li>
      </ul>
      <p className="text-white/40">{t("domains.guideTipIntro")} <b>{t("domains.guideSubdomainPreset")}</b> {t("domains.guideTipEnd")}</p>
    </Guide>
  );
}

function DomainEditorForm({ domain, accounts, onCancel, onSaved }: { domain: Domain | null; accounts: ProviderAccount[]; onCancel: () => void; onSaved: (d?: any) => void; }) {
  const { t } = useTranslation();
  const [name, setName] = useState(domain?.name ?? "");
  const [providerAccountId, setProviderAccountId] = useState(domain?.providerAccountId || accounts[0]?.id || 0);
  const [zoneId, setZoneId] = useState(domain?.zoneId ?? "");
  const [note, setNote] = useState(domain?.note ?? "");
  const [linkHosts, setLinkHosts] = useState<HostEntry[]>(domain?.linkHosts ?? []);
  const [mailHosts, setMailHosts] = useState<HostEntry[]>(domain?.mailHosts ?? []);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  const linkSubs = name ? [`go.${name}`, `s.${name}`, `link.${name}`, name] : [];
  const mailSubs = name ? [name, `mail.${name}`] : [];

  async function save() {
    setErr(""); setBusy(true);
    const payload: any = { name, providerAccountId, zoneId, note, linkHosts, mailHosts };
    try {
      let res;
      if (domain) res = await api.updateDomain(domain.id, payload);
      else res = await api.createDomain(payload);
      onSaved(res);
    } catch (e: any) { setErr(e.message ?? t("domains.saveFailed")); }
    finally { setBusy(false); }
  }

  return (
    <div className="space-y-4">
      <Field label={t("domains.domainName")}>
        <input className="input w-full font-mono" value={name} onChange={(e) => setName(e.target.value)} placeholder="example.com" disabled={!!domain} required />
      </Field>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <Field label={t("domains.dnsProviderConnection")}>
          <select className="input w-full" value={providerAccountId} onChange={(e) => setProviderAccountId(Number(e.target.value))}>
            {accounts.map((a) => <option key={a.id} value={a.id}>{a.name} ({a.type})</option>)}
            {accounts.length === 0 && <option value={0}>{t("domains.noAccountsAvailable")}</option>}
          </select>
        </Field>
        <Field label={t("domains.zoneIdentifier")}>
          <input className="input w-full font-mono text-xs" value={zoneId} onChange={(e) => setZoneId(e.target.value)} placeholder={t("domains.zoneIdPlaceholder")} />
        </Field>
      </div>
      <Field label={t("domains.internalAdminNote")}>
        <textarea className="input w-full" rows={2} value={note} onChange={(e) => setNote(e.target.value)} placeholder={t("domains.notePlaceholder")} />
      </Field>
      {!domain && (
        <>
          <Field label={t("domains.shortLinkSubdomains")}>
            <HostList hosts={linkHosts} onChange={setLinkHosts} suggestions={linkSubs} placeholder="go.example.com" baseDomain={name || undefined} emptyText={t("domains.noShortlinkSubdomains")} />
          </Field>
          <Field label={t("domains.inboundMailSubdomains")}>
            <HostList hosts={mailHosts} onChange={setMailHosts} suggestions={mailSubs} placeholder="mail.example.com" baseDomain={name || undefined} emptyText={t("domains.noMailboxSubdomains")} />
          </Field>
        </>
      )}
      {err && <p className="text-sm text-rose-400 font-medium">{err}</p>}
      <div className="flex justify-end gap-2.5 pt-4 border-t border-white/[0.06]">
        {onCancel && (
          <Button type="button" variant="ghost" onClick={onCancel}>{t("domains.cancel")}</Button>
        )}
        <Button variant="primary" onClick={save} disabled={busy || !name}>{busy ? t("domains.saving") : t("domains.saveBasicInfo")}</Button>
      </div>
    </div>
  );
}

function DomainHostManager({ domain, onReload }: { domain: Domain; onReload: () => void }) {
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
      await api.updateDomain(domain.id, { [service]: updated });
      await onReload();
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
      await api.updateDomain(domain.id, payload);
      await onReload();
    } finally {
      setBusy(null);
    }
  }

  async function removeHost(hostName: string) {
    setBusy(`remove:${hostName}`);
    try {
      await api.updateDomain(domain.id, {
        linkHosts: (domain.linkHosts ?? []).filter((h) => h.host !== hostName),
        mailHosts: (domain.mailHosts ?? []).filter((h) => h.host !== hostName),
      });
      await onReload();
    } finally {
      setBusy(null);
    }
  }

  return (
    <div className="bg-black/25 rounded-2xl p-4 border border-white/[0.05] space-y-4">
      {hosts.length === 0 ? (
        <p className="text-sm text-white/30">{t("domains.noActiveHosts")}</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-xs text-white/40 border-b border-white/[0.06] font-semibold uppercase tracking-wider">
                <th className="py-2.5 pr-4">{t("domains.thHost")}</th>
                <th className="py-2.5 pr-4 text-indigo-400">{t("domains.thLink")}</th>
                <th className="py-2.5 pr-4 text-emerald-400">{t("domains.thMail")}</th>
                <th className="py-2.5 text-right" />
              </tr>
            </thead>
            <tbody className="divide-y divide-white/[0.04]">
              {hosts.map((h) => (
                <tr key={h.host} className="group">
                  <td className="py-3 pr-4 font-mono text-xs text-white/70">{h.host}</td>
                  <td className="py-3 pr-4">
                    {h.linkEnabled !== null ? (
                      <button
                        disabled={!!busy}
                        onClick={() => toggleHost(h.host, "linkHosts", h.linkEnabled!)}
                        className={`inline-flex items-center gap-1 rounded px-2 py-0.5 text-xs font-medium transition-colors disabled:opacity-50 ${
                          h.linkEnabled
                            ? "bg-indigo-500/25 text-indigo-300 hover:bg-indigo-500/40"
                            : "bg-white/[0.06] text-white/40 hover:bg-white/10 line-through"
                        }`}
                      >
                        <span className={`h-1.5 w-1.5 rounded-full ${h.linkEnabled ? "bg-indigo-400" : "bg-white/[0.06]"}`} />
                        {h.linkEnabled ? t("domains.on") : t("domains.off")}
                      </button>
                    ) : (
                      <button
                        disabled={!!busy}
                        onClick={() => addHost(h.host, true, false)}
                        className="text-xs text-white/30 hover:text-indigo-400 transition-colors px-2 py-0.5 disabled:opacity-50"
                      >
                        {t("domains.addShort")}
                      </button>
                    )}
                  </td>
                  <td className="py-3 pr-4">
                    {h.mailEnabled !== null ? (
                      <button
                        disabled={!!busy}
                        onClick={() => toggleHost(h.host, "mailHosts", h.mailEnabled!)}
                        className={`inline-flex items-center gap-1 rounded px-2 py-0.5 text-xs font-medium transition-colors disabled:opacity-50 ${
                          h.mailEnabled
                            ? "bg-emerald-500/25 text-emerald-300 hover:bg-emerald-500/40"
                            : "bg-white/[0.06] text-white/40 hover:bg-white/10 line-through"
                        }`}
                      >
                        <span className={`h-1.5 w-1.5 rounded-full ${h.mailEnabled ? "bg-emerald-400" : "bg-white/[0.06]"}`} />
                        {h.mailEnabled ? t("domains.on") : t("domains.off")}
                      </button>
                    ) : (
                      <button
                        disabled={!!busy}
                        onClick={() => addHost(h.host, false, true)}
                        className="text-xs text-white/30 hover:text-emerald-400 transition-colors px-2 py-0.5 disabled:opacity-50"
                      >
                        {t("domains.addShort")}
                      </button>
                    )}
                  </td>
                  <td className="py-3 text-right">
                    <button
                      disabled={!!busy}
                      onClick={() => removeHost(h.host)}
                      title={t("domains.removeHost")}
                      className="text-xs text-rose-400/70 hover:text-rose-300 opacity-0 group-hover:opacity-100 transition-all disabled:opacity-30 px-2.5 py-0.5"
                    >
                      {t("domains.remove")}
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
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
    <div className="flex items-center gap-2 flex-wrap bg-white/[0.02] p-2.5 rounded-xl border border-white/[0.04]">
      <div className="relative flex-1 min-w-[150px]">
        <input
          className="input h-9 text-xs py-1.5 w-full"
          placeholder={t("domains.hostDraftPlaceholder")}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && submit()}
        />
        {draft && !draft.includes(".") && (
          <span className="pointer-events-none absolute inset-y-0 right-3 flex items-center text-[10px] text-white/35">
            → {draft.trim().toLowerCase()}.{domain.name}
          </span>
        )}
      </div>
      <label className="flex items-center gap-1.5 text-xs text-white/60 cursor-pointer select-none">
        <input type="checkbox" checked={forLink} onChange={(e) => setForLink(e.target.checked)} className="accent-indigo-500" />
        {t("domains.thLink")}
      </label>
      <label className="flex items-center gap-1.5 text-xs text-white/60 cursor-pointer select-none">
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
              className="text-[10px] text-white/40 hover:text-white/70 border border-white/[0.05] hover:border-white/15 bg-white/[0.01] hover:bg-white/[0.03] rounded-lg px-2 py-0.5 transition-colors cursor-pointer"
            >
              {s}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

function SyncModal({ accounts, onClose, onSynced }: { accounts: ProviderAccount[]; onClose: () => void; onSynced: () => void }) {
  const { t } = useTranslation();
  const [accountId, setAccountId] = useState<number>(accounts[0]?.id || 0);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [result, setResult] = useState<{ total: number; created: number; updated: number } | null>(null);

  async function run() {
    if (!accountId) return setErr(t("domains.selectProviderAccount"));
    setBusy(true); setErr("");
    try { const r = await api.syncDomains(accountId); setResult(r); }
    catch (e: any) { setErr(e.message ?? t("domains.syncFailed")); }
    finally { setBusy(false); }
  }

  return (
    <Modal title={t("domains.syncDnsZones")} onClose={onClose}>
      {result ? (
        <div className="py-4 text-center space-y-4">
          <div className="h-12 w-12 rounded-full bg-emerald-500/10 flex items-center justify-center text-emerald-400 mx-auto">
            <ShieldCheck className="h-6 w-6" />
          </div>
          <div>
            <p className="text-white font-semibold">{t("domains.zonesDetected", { count: result.total })}</p>
            <p className="text-xs text-white/55 mt-1">
              {t("domains.createdPrefix")} <span className="text-emerald-400 font-bold">{result.created}</span> {t("domains.updatedMid")} <span className="text-indigo-400 font-bold">{result.updated}</span> {t("domains.recordsSuffix")}
            </p>
          </div>
          <p className="text-[11px] text-white/40 max-w-xs mx-auto">{t("domains.syncToggleHint")}</p>
          <Button variant="primary" onClick={onSynced} className="w-full">{t("domains.done")}</Button>
        </div>
      ) : accounts.length === 0 ? (
        <div className="py-4 text-center space-y-2 text-white/55">
          <p className="font-semibold">{t("domains.noProviderAccounts")}</p>
          <p className="text-xs text-white/40">{t("domains.noProviderAccountsHint")}</p>
        </div>
      ) : (
        <>
          <p className="mb-4 text-xs text-white/55 leading-relaxed">{t("domains.syncIntro")}</p>
          <Field label={t("domains.dnsProviderConnection")}>
            <select className="input w-full" value={accountId} onChange={e => setAccountId(Number(e.target.value))}>
              <option value={0}>{t("domains.selectAccount")}</option>
              {accounts.map(a => <option key={a.id} value={a.id}>{a.name} ({a.type})</option>)}
            </select>
          </Field>
          {err && <p className="mb-4 text-sm text-rose-400 font-medium">{err}</p>}
          <div className="flex justify-end gap-2.5 pt-4 border-t border-white/[0.06]">
            <Button variant="ghost" onClick={onClose}>{t("domains.cancel")}</Button>
            <Button variant="primary" onClick={run} disabled={busy || !accountId}>{busy ? t("domains.queryingApi") : t("domains.syncZones")}</Button>
          </div>
        </>
      )}
    </Modal>
  );
}

const RECORD_TYPES = ["A", "AAAA", "CNAME", "TXT", "MX", "NS", "CAA"];

function RecordsView({ domain }: { domain: Domain }) {
  const { t } = useTranslation();
  const [records, setRecords] = useState<DNSRecord[] | null>(null);
  const [err, setErr] = useState("");
  const [editing, setEditing] = useState<DNSRecord | "new" | "subdomain" | null>(null);
  const [typeFilter, setTypeFilter] = useState("");
  const [search, setSearch] = useState("");

  async function load() {
    setErr("");
    try { setRecords(await api.records(domain.id)); }
    catch (e: any) { setErr(e.message ?? t("domains.loadRecordsFailed")); setRecords([]); }
  }
  useEffect(() => { load(); }, [domain.id]);

  const filtered = (records ?? []).filter((r) => {
    if (typeFilter && r.type !== typeFilter) return false;
    if (search) {
      const s = search.toLowerCase();
      return r.name.toLowerCase().includes(s) || r.content.toLowerCase().includes(s) || (r.comment ?? "").toLowerCase().includes(s);
    }
    return true;
  });
  const presentTypes = Array.from(new Set((records ?? []).map((r) => r.type)));

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <select className="input min-w-[120px] text-xs py-1" value={typeFilter} onChange={(e) => setTypeFilter(e.target.value)}>
          <option value="">{t("domains.allTypes")}</option>
          {presentTypes.map((rt) => <option key={rt} value={rt}>{rt}</option>)}
        </select>
        <input className="input flex-1 min-w-[140px] text-xs py-1" placeholder={t("domains.filterPlaceholder")} value={search} onChange={(e) => setSearch(e.target.value)} />
        <Button variant="subtle" onClick={() => setEditing("subdomain")} className="py-1 px-3 text-xs">{t("domains.presetButton")}</Button>
        <Button variant="primary" onClick={() => setEditing("new")} className="py-1 px-3 text-xs">{t("domains.customButton")}</Button>
      </div>

      <p className="text-[11px] text-white/35">{t("domains.recordsNote", { shown: filtered.length, total: records?.length ?? 0 })}</p>
      
      {err && <p className="rounded bg-rose-500/10 p-3 text-xs text-rose-400 font-medium">{err}</p>}
      
      {records === null ? (
        <p className="text-white/40 p-6 text-center text-xs">{t("domains.loadingRecords")}</p>
      ) : filtered.length === 0 ? (
        <p className="text-white/40 p-6 text-center text-xs">{t("domains.noRecordsMatching")}</p>
      ) : (
        <div className="bg-black/20 rounded-2xl border border-white/[0.05] overflow-hidden">
          <table className="w-full text-xs">
            <thead>
              <tr className="text-left text-white/40 border-b border-white/[0.06] bg-white/[0.01]">
                <th className="py-2.5 pl-4 font-semibold uppercase tracking-wider">{t("domains.thType")}</th>
                <th className="py-2.5 font-semibold uppercase tracking-wider">{t("domains.thName")}</th>
                <th className="py-2.5 font-semibold uppercase tracking-wider">{t("domains.thContent")}</th>
                <th className="py-2.5 font-semibold uppercase tracking-wider">{t("domains.thNote")}</th>
                <th className="py-2.5 pr-4 text-right" />
              </tr>
            </thead>
            <tbody className="divide-y divide-white/[0.04]">
              {filtered.map((r) => (
                <tr key={r.id} className="hover:bg-white/[0.01] transition-colors">
                  <td className="py-3 pl-4 font-semibold text-white/80">
                    <span className="inline-flex items-center gap-1.5">
                      <span className="font-mono text-white/85 bg-white/5 px-2 py-0.5 rounded-lg border border-white/[0.04]">{r.type}</span>
                      {r.proxied && (
                        <span title={t("domains.cloudflareProxied")}>
                          <Cloud className="h-3 w-3 text-amber-500 fill-amber-500/10" />
                        </span>
                      )}
                    </span>
                  </td>
                  <td className="max-w-[120px] truncate font-mono text-white/80">{r.name}</td>
                  <td className="max-w-[180px] truncate text-white/55 font-mono">{r.content}</td>
                  <td className="max-w-[120px] truncate text-indigo-300/80">{r.comment}</td>
                  <td className="py-3 pr-4 text-right">
                    <div className="flex gap-1.5 justify-end">
                      <Button variant="ghost" className="px-2 py-0.5 text-[10px]" onClick={() => setEditing(r)}>{t("domains.edit")}</Button>
                      <Button variant="danger" className="px-2 py-0.5 text-[10px] text-rose-400 bg-rose-500/0 hover:bg-rose-500/10 border-0" onClick={async () => { if (confirm(t("domains.deleteRecordConfirm", { type: r.type, name: r.name }))) { await api.deleteRecord(domain.id, r.id); load(); } }}>{t("domains.del")}</Button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {editing && (
        <RecordEditor
          domainId={domain.id} domainName={domain.name}
          linkHost={domain.linkHosts?.[0]?.host ?? ""}
          record={typeof editing === "string" ? null : editing}
          subdomain={editing === "subdomain"}
          onClose={() => setEditing(null)}
          onSaved={() => { setEditing(null); load(); }}
        />
      )}
    </div>
  );
}

function RecordEditor({ domainId, domainName, linkHost, record, subdomain, onClose, onSaved }: { domainId: number; domainName: string; linkHost?: string; record: DNSRecord | null; subdomain?: boolean; onClose: () => void; onSaved: () => void; }) {
  const { t } = useTranslation();
  const [type, setType] = useState(record?.type ?? "A");
  const [name, setName] = useState(record?.name ?? "");
  const [content, setContent] = useState(record?.content ?? "");
  const [comment, setComment] = useState(record?.comment ?? "");
  const [proxied, setProxied] = useState(record?.proxied ?? false);
  const [priority, setPriority] = useState<number>(record?.priority ?? 10);
  const [err, setErr] = useState("");

  const needsPriority = ["MX", "SRV", "URI"].includes(type.toUpperCase());
  const canProxy = ["A", "AAAA", "CNAME"].includes(type.toUpperCase());
  const contentHint: Record<string, string> = { A: t("domains.hintIpv4"), AAAA: t("domains.hintIpv6"), CNAME: t("domains.hintCname"), TXT: t("domains.hintTxt"), MX: t("domains.hintMx"), NS: t("domains.hintNs"), CAA: '0 issue "letsencrypt.org"' };

  const linkSub = linkHost && linkHost.endsWith("." + domainName) ? linkHost.slice(0, -(domainName.length + 1)) : linkHost === domainName ? "@" : "go";

  function preset(kind: "link" | "mail") {
    if (kind === "link") { setType("CNAME"); setName(name || linkSub); setContent(domainName); setComment("led short-link host"); setProxied(true); }
    else { setType("MX"); setName(name || "mail"); setContent("route1.mx.cloudflare.net"); setComment("led mailbox (Cloudflare Email Routing)"); setProxied(false); setPriority(10); }
  }

  async function save() {
    setErr("");
    const payload: Partial<DNSRecord> = { type, name, content, comment, proxied: canProxy ? proxied : false, ttl: 1 };
    if (needsPriority) payload.priority = Number(priority);
    try {
      if (record) await api.updateRecord(domainId, record.id, payload);
      else await api.createRecord(domainId, payload);
      onSaved();
    } catch (e: any) { setErr(e.message ?? t("domains.saveFailed")); }
  }

  return (
    <Modal title={record ? t("domains.modifyRecord") : subdomain ? t("domains.presetConfigurator") : t("domains.createRecord")} onClose={onClose}>
      {subdomain && (
        <div className="mb-4 flex gap-2.5">
          <Button variant="subtle" className="flex-1 py-1.5 text-xs gap-1.5" onClick={() => preset("link")}>
            <LinkIcon className="h-3.5 w-3.5 text-indigo-400" />
            {t("domains.setLinkCname")}
          </Button>
          <Button variant="subtle" className="flex-1 py-1.5 text-xs gap-1.5" onClick={() => preset("mail")}>
            <Mail className="h-3.5 w-3.5 text-emerald-400" />
            {t("domains.setMxRecords")}
          </Button>
        </div>
      )}
      <div className="space-y-4">
        <div className="grid grid-cols-2 gap-4">
          <Field label={t("domains.recordType")}>
            <select className="input w-full" value={type} onChange={(e) => setType(e.target.value)}>
              {RECORD_TYPES.map((rt) => <option key={rt}>{rt}</option>)}
            </select>
          </Field>
          <Field label={t("domains.nameHost")}>
            <input className="input w-full font-mono" value={name} onChange={(e) => setName(e.target.value)} placeholder={t("domains.namePlaceholder")} />
          </Field>
        </div>
        
        <div className={needsPriority ? "grid grid-cols-[1fr_120px] gap-4" : ""}>
          <Field label={t("domains.targetValue")} hint={contentHint[type.toUpperCase()]}>
            <input className="input w-full font-mono text-xs" value={content} onChange={(e) => setContent(e.target.value)} required />
          </Field>
          {needsPriority && (
            <Field label={t("domains.priority")}>
              <input type="number" min={0} className="input w-full" value={priority} onChange={(e) => setPriority(Number(e.target.value))} />
            </Field>
          )}
        </div>

        <Field label={t("domains.metadataComment")}>
          <input className="input w-full text-xs" value={comment} onChange={(e) => setComment(e.target.value)} placeholder={t("domains.commentPlaceholder")} />
        </Field>

        {canProxy && (
          <div className="flex items-center gap-2 pt-1">
            <Toggle on={proxied} onChange={setProxied} />
            <span className="text-xs text-white/60 select-none">{t("domains.proxiedLabel")}</span>
          </div>
        )}

        {err && <p className="text-sm text-rose-400 font-medium">{err}</p>}
        
        <div className="flex justify-end gap-2.5 pt-4 border-t border-white/[0.06]">
          <Button variant="ghost" onClick={onClose}>{t("domains.cancel")}</Button>
          <Button variant="primary" onClick={save}>{t("domains.saveRecord")}</Button>
        </div>
      </div>
    </Modal>
  );
}
