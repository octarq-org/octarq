import { Empty, Badge, Button } from "@octarq/plugin-sdk";
import { useEffect, useState } from "react";
import { NavLink } from "react-router-dom";
import { Cloud, KeyRound, Blocks } from "lucide-react";
import { useTranslation } from "../../../i18n";
import { roleSatisfies, useCurrentRole } from "../../../shell/role";

export function ProviderAccounts({ embed }: { embed?: boolean }) {
  const { t } = useTranslation();
  const { role, isInstanceAdmin } = useCurrentRole();
  const canManageProviders = roleSatisfies("admin", role, isInstanceAdmin);
  const [accounts, setAccounts] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);

  async function load() {
    setLoading(true);
    try {
      const res = await fetch("/api/integrations?provider_type=dns", {
        credentials: "same-origin",
      });
      if (res.ok) {
        const data = await res.json();
        setAccounts(Array.isArray(data) ? data : []);
      } else {
        setAccounts([]);
      }
    } catch {
      setAccounts([]);
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => { load(); }, []);

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <div className="text-xs font-semibold text-foreground/70">
          {t("settings.dnsApiAccounts")}
          <div className="text-[10px] text-foreground/50 font-normal mt-0.5">{t("settings.dnsApiAccountsDesc")}</div>
        </div>
        {canManageProviders && (
          <NavLink to="/settings/integrations?category=dns">
            <Button variant="primary" className="text-xs py-1 px-2.5">
              {t("settings.openIntegrationsHub")}
            </Button>
          </NavLink>
        )}
      </div>

      <div className="flex items-center justify-between p-3 rounded-lg border border-primary/20 bg-primary/5 text-xs text-foreground/80">
        <div className="flex items-center gap-2">
          <Blocks className="h-4 w-4 text-primary shrink-0" />
          <span>{t("settings.integrationsHubBanner")}</span>
        </div>
        <NavLink
          to="/settings/integrations?category=dns"
          className="text-primary hover:underline font-medium text-xs whitespace-nowrap ml-3"
        >
          {t("settings.openIntegrationsHub")} →
        </NavLink>
      </div>

      {loading ? (
        <div className="text-foreground/40 text-sm py-4 text-center">{t("settings.loadingLower")}</div>
      ) : accounts.length === 0 ? (
        <Empty>
          <Cloud className="h-8 w-8 text-foreground/50 mb-1" />
          <div className="text-xs text-foreground/50">{t("settings.noDnsProviders")}</div>
          {canManageProviders && (
            <NavLink to="/settings/integrations?category=dns">
              <Button variant="outline" size="sm" className="mt-3 text-xs">
                {t("settings.openIntegrationsHub")}
              </Button>
            </NavLink>
          )}
        </Empty>
      ) : (
        <div className="divide-y divide-foreground/[0.04] border border-foreground/[0.05] rounded-xl bg-well overflow-hidden">
          {accounts.map(a => {
            const providerType = a.provider || a.type || "cloudflare";
            const isConfigured = a.configured ?? a.hasCredentials ?? true;
            return (
              <div key={a.id} className="flex items-center justify-between p-4">
                <div>
                  <div className="font-semibold text-sm text-foreground">{a.name}</div>
                  <div className="text-xs text-foreground/40 mt-1 flex items-center gap-1.5">
                    <Badge
                      tone={providerType === "cloudflare" ? "indigo" : providerType === "dnspod" ? "cyan" : "amber"}
                      className="uppercase tracking-wider text-[9px]"
                    >
                      {providerType}
                    </Badge>
                    {isConfigured
                      ? <Badge tone="green" className="text-[9px]"><KeyRound className="h-2.5 w-2.5 mr-0.5 inline" />{t("settings.credentialsSet")}</Badge>
                      : <Badge tone="amber" className="text-[9px]">{t("settings.noCredentials")}</Badge>}
                  </div>
                </div>
                {canManageProviders && (
                  <NavLink to="/settings/integrations?category=dns">
                    <Button variant="subtle" className="text-xs py-1 px-2.5">
                      {t("settings.openIntegrationsHub")}
                    </Button>
                  </NavLink>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
