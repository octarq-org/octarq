import { Empty, Badge, Button, toast } from "@octarq/plugin-sdk";
import { useEffect, useState } from "react";
import { NavLink } from "react-router-dom";
import { KeyRound, Send, Blocks } from "lucide-react";
import { useTranslation } from "../../../i18n";
import { roleSatisfies, useCurrentRole } from "../../../shell/role";

export function SMTPSenders() {
  const { t } = useTranslation();
  const { role, isInstanceAdmin } = useCurrentRole();
  const canManageSMTP = roleSatisfies("admin", role, isInstanceAdmin);
  const [senders, setSenders] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [testingId, setTestingId] = useState<number | null>(null);

  async function load() {
    setLoading(true);
    try {
      const res = await fetch("/api/integrations?provider_type=communication", {
        credentials: "same-origin",
      });
      if (res.ok) {
        const data = await res.json();
        const commSenders = (Array.isArray(data) ? data : []).filter(
          (item: any) => item.provider === "smtp" || item.provider === "resend"
        );
        setSenders(commSenders);
      } else {
        setSenders([]);
      }
    } catch {
      setSenders([]);
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => { load(); }, []);

  async function test(id: number, name: string) {
    setTestingId(id);
    try {
      const res = await fetch(`/api/integrations/${id}/test`, {
        method: "POST",
        credentials: "same-origin",
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok || data.ok === false) {
        throw new Error(data.detail || data.message || "Connection test failed");
      }
      toast.success(data.message || t("settings.smtpTestSent", { to: name }));
    } catch (e: any) {
      toast.error(t("settings.testFailed", { msg: e?.message || "unknown error" }));
    } finally {
      setTestingId(null);
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <div className="text-xs font-semibold text-foreground/70">
          {t("settings.smtpOutgoingGateways")}
          <div className="text-[10px] text-foreground/50 font-normal mt-0.5">{t("settings.smtpOutgoingGatewaysDesc")}</div>
        </div>
        {canManageSMTP && (
          <NavLink to="/settings/integrations?category=communication">
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
          to="/settings/integrations?category=communication"
          className="text-primary hover:underline font-medium text-xs whitespace-nowrap ml-3"
        >
          {t("settings.openIntegrationsHub")} →
        </NavLink>
      </div>

      {loading ? (
        <div className="text-foreground/40 text-sm py-6 text-center">{t("settings.loadingLower")}</div>
      ) : senders.length === 0 ? (
        <Empty>
          <Send className="h-8 w-8 text-foreground/50 mb-1" />
          <div className="text-xs text-foreground/50">{t("settings.noSmtpSenders")}</div>
          {canManageSMTP && (
            <NavLink to="/settings/integrations?category=communication">
              <Button variant="outline" size="sm" className="mt-3 text-xs">
                {t("settings.openIntegrationsHub")}
              </Button>
            </NavLink>
          )}
        </Empty>
      ) : (
        <div className="divide-y divide-foreground/[0.04] border border-foreground/[0.05] rounded-xl bg-well overflow-hidden">
          {senders.map(s => (
            <div key={s.id} className="flex items-center justify-between p-4 group">
              <div>
                <div className="font-semibold text-sm text-foreground flex items-center gap-1.5">
                  {s.name}
                  <Badge tone={s.provider === "resend" ? "indigo" : "cyan"} className="uppercase tracking-wider text-[9px]">
                    {s.provider}
                  </Badge>
                  {s.configured ?? true
                    ? <Badge tone="green" className="text-[9px]"><KeyRound className="h-2.5 w-2.5 mr-0.5 inline" />{t("settings.smtpPasswordSet")}</Badge>
                    : <Badge tone="amber" className="text-[9px]">{t("settings.smtpNoPassword")}</Badge>}
                </div>
                {(s.summary || s.fromEmail) && (
                  <div className="text-xs text-foreground/40 mt-1 font-mono">
                    {s.summary || `${s.fromEmail} via ${s.host}:${s.port || 587}`}
                  </div>
                )}
              </div>
              {canManageSMTP && (
                <div className="flex items-center gap-2">
                  <Button
                    variant="subtle"
                    onClick={() => test(s.id, s.name)}
                    disabled={testingId === s.id}
                    className="text-xs py-1 px-2.5"
                  >
                    {testingId === s.id ? t("settings.testing") : t("settings.test")}
                  </Button>
                  <NavLink to="/settings/integrations?category=communication">
                    <Button variant="subtle" className="text-xs py-1 px-2.5">
                      {t("settings.openIntegrationsHub")}
                    </Button>
                  </NavLink>
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
