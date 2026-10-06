import { useMemo } from "react";
import { Button, Empty, GlassCard, Input, Badge } from "@octarq/plugin-sdk";
import { Domain, ProviderAccount } from "../../../api";
import { Globe, RefreshCw, Plus, Mail, Link as LinkIcon, Settings, Layers, ShieldCheck } from "lucide-react";
import { useTranslation } from "../../../i18n";
import { ListSkeleton } from "../../../components/ListSkeleton";
import type { DomainSubTab } from "./DomainDetailWorkspace";

export interface DomainListHubProps {
  domains: Domain[];
  accounts: ProviderAccount[];
  loading: boolean;
  q: string;
  onSearchChange: (q: string) => void;
  onSelectDomain: (d: Domain, subTab?: DomainSubTab) => void;
  onAddNewDomain: () => void;
  onSyncCloudflare: () => void;
  onConnectProvider: () => void;
  onToggleService: (domain: Domain, field: "forLink" | "forMail") => void;
  getProviderName: (dom: Domain) => string;
}

export function DomainListHub({
  domains,
  accounts,
  loading,
  q,
  onSearchChange,
  onSelectDomain,
  onAddNewDomain,
  onSyncCloudflare,
  onConnectProvider,
  onToggleService,
  getProviderName,
}: DomainListHubProps) {
  const { t } = useTranslation();

  const linkCount = useMemo(() => domains.filter((d) => d.forLink).length, [domains]);
  const mailCount = useMemo(() => domains.filter((d) => d.forMail).length, [domains]);

  if (domains.length === 0 && !loading) {
    return (
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
              <Button variant="primary" onClick={onSyncCloudflare} className="gap-1.5">
                <RefreshCw className="h-4 w-4" />
                {t("domains.syncFrom", { name: accounts.length === 1 ? accounts[0].name : t("domains.provider") })}
              </Button>
            ) : (
              <Button variant="primary" onClick={onConnectProvider} className="gap-1.5">
                <Plus className="h-4 w-4" />
                {t("domains.connectProvider")}
              </Button>
            )}
            <button
              onClick={onAddNewDomain}
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
    );
  }

  return (
    <div className="space-y-4">
      {/* Filter bar and quick stats */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2 flex-1 max-w-md">
          <Input
            className="text-sm"
            placeholder={t("domains.searchDomains")}
            value={q}
            onChange={(e) => onSearchChange(e.target.value)}
          />
          {q && (
            <Button variant="ghost" className="text-xs py-1.5" onClick={() => onSearchChange("")}>
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
          <Button variant="ghost" className="text-xs py-1.5" onClick={() => onSearchChange("")}>
            {t("domains.emptyFilteredAction")}
          </Button>
        </GlassCard>
      ) : (
        <div className="grid grid-cols-1 gap-3">
          {domains.map((d) => (
            <div
              key={d.id}
              role="button"
              tabIndex={0}
              className="glass p-4 rounded-2xl border border-foreground/[0.06] hover:border-foreground/20 transition-colors cursor-pointer group focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
              onClick={() => {
                onSelectDomain(d, "records");
              }}
              onKeyDown={(e) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  onSelectDomain(d, "records");
                }
              }}
            >
              <div className="flex flex-wrap items-center justify-between gap-4">
                <div className="flex items-center gap-3 min-w-0">
                  <div className="h-10 w-10 rounded-xl bg-accent-soft flex items-center justify-center text-accent-fg shrink-0 group-hover:scale-105 transition-transform">
                    <Globe className="h-5 w-5" aria-hidden="true" />
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
                        onToggleService(d, "forLink");
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
                        onToggleService(d, "forMail");
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
                      onSelectDomain(d, "records");
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
                      onSelectDomain(d, "verification");
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
                      onSelectDomain(d, "settings");
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
  );
}
