import { Empty, Field, Input, Modal, Badge, Button, Select, toast, confirmDialog, FormError } from "@octarq/plugin-sdk";
import { useEffect, useState } from "react";
import { NavLink } from "react-router-dom";
import { api, ProviderAccount } from "../../../api";
import { Cloud, KeyRound, Blocks } from "lucide-react";
import { useTranslation } from "../../../i18n";
import { roleSatisfies, useCurrentRole } from "../../../shell/role";

// Which registry owns provider accounts. An instance with the Integrations Hub
// mounted uses hub credentials; standalone Core has no hub and uses the local
// provider_accounts table. Read exactly one, never both: the registries number
// their rows independently.
type AccountMode = "integration" | "local" | null;

export function ProviderAccounts({ embed }: { embed?: boolean }) {
  const { t } = useTranslation();
  const { role, isInstanceAdmin } = useCurrentRole();
  const canManageProviders = roleSatisfies("admin", role, isInstanceAdmin);
  const [mode, setMode] = useState<AccountMode>(null);
  const [accounts, setAccounts] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<any | null>(null);

  async function load() {
    setLoading(true);
    try {
      const res = await fetch("/api/integrations?provider_type=dns", {
        credentials: "same-origin",
      });
      if (res.status === 404) {
        setMode("local");
        setAccounts(await api.providerAccounts());
        return;
      }
      if (!res.ok) {
        setMode(null);
        setAccounts([]);
        return;
      }
      const data = await res.json();
      setMode("integration");
      setAccounts(Array.isArray(data) ? data : []);
    } catch {
      setMode(null);
      setAccounts([]);
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => { load(); }, []);

  async function remove(id: number) {
    if (!(await confirmDialog(t("settings.confirmRemoveProvider")))) return;
    try {
      await api.deleteProviderAccount(id);
      load();
    } catch (e: any) {
      toast.error(e.message || t("settings.failedRemove"));
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <div className="text-xs font-semibold text-foreground/70">
          {t("settings.dnsApiAccounts")}
          <div className="text-[10px] text-foreground/50 font-normal mt-0.5">{t("settings.dnsApiAccountsDesc")}</div>
        </div>
        {canManageProviders && mode === "local" && (
          <Button variant="primary" className="text-xs py-1 px-2.5" onClick={() => setCreating(true)}>
            {t("settings.addProvider")}
          </Button>
        )}
        {canManageProviders && mode === "integration" && (
          <NavLink to="/settings/integrations?category=dns">
            <Button variant="primary" className="text-xs py-1 px-2.5">
              {t("settings.openIntegrationsHub")}
            </Button>
          </NavLink>
        )}
      </div>

      {mode === "integration" && (
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
      )}

      {loading ? (
        <div className="text-foreground/40 text-sm py-4 text-center">{t("settings.loadingLower")}</div>
      ) : mode === null ? (
        <Empty>
          <Cloud className="h-8 w-8 text-foreground/50 mb-1" />
          <div className="text-xs text-foreground/50">{t("settings.dnsProvidersUnavailable")}</div>
        </Empty>
      ) : accounts.length === 0 ? (
        <Empty>
          <Cloud className="h-8 w-8 text-foreground/50 mb-1" />
          <div className="text-xs text-foreground/50">{t("settings.noDnsProviders")}</div>
          {canManageProviders && mode === "local" && (
            <Button variant="outline" size="sm" className="mt-3 text-xs" onClick={() => setCreating(true)}>
              {t("settings.addProvider")}
            </Button>
          )}
          {canManageProviders && mode === "integration" && (
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
                {canManageProviders && mode === "integration" && (
                  <NavLink to="/settings/integrations?category=dns">
                    <Button variant="subtle" className="text-xs py-1 px-2.5">
                      {t("settings.openIntegrationsHub")}
                    </Button>
                  </NavLink>
                )}
                {canManageProviders && mode === "local" && (
                  <div className="flex items-center gap-2">
                    <Button variant="subtle" onClick={() => setEditing(a)} className="text-xs py-1 px-2.5">
                      {t("settings.edit")}
                    </Button>
                    <Button
                      variant="danger"
                      onClick={() => remove(a.id)}
                      className="text-xs py-1 px-2.5 border-0"
                    >
                      {t("settings.remove")}
                    </Button>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
      {(creating || editing) && (
        <ProviderAccountModal
          account={editing}
          onClose={() => { setCreating(false); setEditing(null); }}
          onSaved={() => { setCreating(false); setEditing(null); load(); }}
        />
      )}
    </div>
  );
}

function ProviderAccountModal({ account, onClose, onSaved }: { account: any; onClose: () => void; onSaved: () => void }) {
  const { t } = useTranslation();
  const [name, setName] = useState(account?.name || "");
  const [type, setType] = useState(account?.type || "cloudflare");
  const [config, setConfig] = useState<string>("");
  const [types, setTypes] = useState<string[]>([]);
  const [err, setErr] = useState<string | { message?: string; status?: number; requestId?: string }>("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api.dnsProviders().then(setTypes).catch(() => setTypes(["cloudflare", "dnspod"]));
  }, []);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true); setErr("");
    try {
      let cfgObj: any = {};
      if (config.trim()) {
        try { cfgObj = JSON.parse(config.trim()); }
        catch { cfgObj = type === "cloudflare" ? { apiToken: config.trim() } : { token: config.trim() }; }
      }
      if (account) {
        await api.updateProviderAccount(account.id, { name, type, config: cfgObj });
      } else {
        await api.createProviderAccount({ name, type, config: cfgObj });
      }
      toast.success(t("settings.saved"));
      onSaved();
    } catch (e: any) {
      setErr(e);
      toast.error(e?.message || t("settings.saveFailed", "Failed to save settings"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal title={account ? t("settings.editProviderAccount") : t("settings.registerProviderAccount")} onClose={onClose}>
      <form onSubmit={submit} className="space-y-4">
        <Field label={t("settings.providerLabelName")}>
          <Input value={name} onChange={(e: any) => setName(e.target.value)} placeholder={t("settings.providerLabelPlaceholder")} required autoFocus />
        </Field>
        {!account && (
          <Field label={t("settings.dnsProviderType")}>
            <Select
              value={type}
              onValueChange={setType}
              className="text-sm"
              options={types.map((ty) => ({ value: ty, label: <span className="capitalize">{ty}</span> }))}
            />
          </Field>
        )}
        <Field label={t("settings.apiKeysCredentials")} hint={account ? t("settings.apiKeysHintExisting") : t("settings.apiKeysHintNew")}>
          <Input className="font-mono text-xs" type="password" value={config} onChange={(e: any) => setConfig(e.target.value)} placeholder={account ? "••••••••" : t("settings.apiKeysPlaceholderNew")} required={!account} />
        </Field>
        {err && <FormError err={err} />}
        <div className="flex justify-end gap-2.5 pt-4 border-t border-foreground/[0.06]">
          <Button type="button" variant="ghost" onClick={onClose}>{t("settings.cancel")}</Button>
          <Button type="submit" variant="primary" disabled={busy || !name.trim()}>
            {busy ? t("settings.savingDots") : t("settings.saveConnection")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
