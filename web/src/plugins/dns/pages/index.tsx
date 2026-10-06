import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { Button, GlassCard, PageHeader, ScreenWrap, toast } from "@octarq/plugin-sdk";
import { api, Domain, ProviderAccount } from "../../../api";
import { dnsApi } from "../api";
import { Globe, RefreshCw, Plus, ArrowLeft } from "lucide-react";
import { ProviderAccounts } from "./ProviderAccounts";
import { useTranslation } from "../../../i18n";
import { DomainEditorForm } from "./DomainEditorForm";
import { SyncModal } from "./SyncModal";
import { DDNSView } from "./DDNSView";
import { usePluginGate } from "../../PluginGate";
import { roleSatisfies, useCurrentRole } from "../../../shell/role";
import { DomainDetailWorkspace, type DomainSubTab } from "./DomainDetailWorkspace";
import { DomainListHub } from "./DomainListHub";

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
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          next.delete("create");
          return next;
        },
        { replace: true },
      );
    }
  }, [searchParams, setSearchParams]);

  const [page, setPage] = useState(0);
  const [hasMore, setHasMore] = useState(true);
  const [loading, setLoading] = useState(false);

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

      setDomains((prev) => (reset ? res : [...prev, ...res]));
      setPage(reset ? 1 : page + 1);

      if (active && active !== "new") {
        const refreshed = res.find((d) => d.id === active.id);
        if (refreshed) setActive(refreshed);
      }
    } catch (e: unknown) {
      const err = e as { status?: number };
      if (err.status === 404 || err.status === 402) {
        pluginGate.degrade(err.status);
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
    } catch (e: unknown) {
      const err = e as { message?: string };
      toast.error(err.message || t("domains.updateFailed"));
    }
  }

  function getProviderName(dom: Domain) {
    if (!dom.providerAccountId) return t("domains.providerManual");
    const acc = accounts.find((a) => a.id === dom.providerAccountId);
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
            <Button variant="primary" onClick={() => setActive("new")} className="gap-1.5 py-1.5 text-xs">
              <Plus className="h-3.5 w-3.5" />
              {t("domains.addDomain")}
            </Button>
          </div>
        }
      />

      <div className="flex gap-0 border-b border-foreground/[0.06] mb-6 overflow-x-auto [scrollbar-width:none] [-ms-overflow-style:none] [&::-webkit-scrollbar]:hidden">
        <button
          onClick={() => setTab("domains")}
          className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors shrink-0 whitespace-nowrap ${
            tab === "domains"
              ? "border-primary text-foreground"
              : "border-transparent text-foreground/45 hover:text-foreground/70"
          }`}
        >
          {t("domains.tabDns")}
        </button>
        <button
          onClick={() => setTab("ddns")}
          className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors flex items-center gap-1.5 shrink-0 whitespace-nowrap ${
            tab === "ddns"
              ? "border-primary text-foreground"
              : "border-transparent text-foreground/45 hover:text-foreground/70"
          }`}
        >
          {t("domains.tabDdns")}
        </button>
        <button
          onClick={() => setTab("settings")}
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
            <DomainDetailWorkspace
              domain={active}
              accounts={accounts}
              canDeleteDomain={canDeleteDomain}
              initialSubTab={activeSubTab}
              onBack={() => setActive(null)}
              onDeleted={() => {
                setActive(null);
                loadMore(true);
              }}
              onUpdated={(updatedDomain) => {
                setActive(updatedDomain);
                loadMore(true);
              }}
              onReloadDomains={loadMore}
              getProviderName={getProviderName}
            />
          ) : (
            <DomainListHub
              domains={domains}
              accounts={accounts}
              loading={loading}
              q={q}
              onSearchChange={setQ}
              onSelectDomain={(dom, subTab) => {
                setActive(dom);
                if (subTab) setActiveSubTab(subTab);
              }}
              onAddNewDomain={() => setActive("new")}
              onSyncCloudflare={() => setSyncing(true)}
              onConnectProvider={() => setTab("settings")}
              onToggleService={toggleService}
              getProviderName={getProviderName}
            />
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
