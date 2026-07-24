import { useEffect, useState } from "react";
import { api, AuditLog } from "../api";
import { timeAgo, ScreenWrap, PageHeader, GlassCard, Badge } from "@octarq/plugin-sdk";
import { useTranslation } from "@octarq/plugin-sdk";

export default function AuditLogPage() {
  const { t } = useTranslation();
  const [logs, setLogs] = useState<AuditLog[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    api.auditLogs()
      .then(setLogs)
      .finally(() => setLoading(false));
  }, []);

  const getActionTone = (action: string) => {
    if (action.includes(".delete")) return "red";
    if (action.includes(".create")) return "green";
    if (action.includes(".update")) return "amber";
    return "indigo";
  };

  return (
    <ScreenWrap>
      <PageHeader
        title={t("audit.pageTitle")}
        description={t("audit.pageDesc")}
      />

      {loading ? (
        <div className="text-foreground/40 py-12 text-center">{t("audit.loading")}</div>
      ) : logs.length === 0 ? (
        <GlassCard className="p-10 text-center text-foreground/40">
          {t("audit.emptyState")}
        </GlassCard>
      ) : (
        <GlassCard className="overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm border-collapse">
              <thead className="border-b border-foreground/[0.06] bg-foreground/[0.02] text-foreground/55">
                <tr>
                  <th className="px-5 py-3.5 font-semibold text-xs uppercase tracking-wider">{t("audit.colTime")}</th>
                  <th className="px-5 py-3.5 font-semibold text-xs uppercase tracking-wider">{t("audit.colActor")}</th>
                  <th className="px-5 py-3.5 font-semibold text-xs uppercase tracking-wider">{t("audit.colAction")}</th>
                  <th className="px-5 py-3.5 font-semibold text-xs uppercase tracking-wider">{t("audit.colTarget")}</th>
                  <th className="px-5 py-3.5 font-semibold text-xs uppercase tracking-wider">{t("audit.colIp")}</th>
                  <th className="px-5 py-3.5 font-semibold text-xs uppercase tracking-wider">{t("audit.colMeta")}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-foreground/[0.04]">
                {logs.map((l) => (
                  <tr key={l.id} className="hover:bg-foreground/[0.02] transition-colors">
                    <td className="whitespace-nowrap px-5 py-4 text-foreground/60 text-xs" title={l.createdAt}>
                      {timeAgo(l.createdAt)}
                    </td>
                    <td className="whitespace-nowrap px-5 py-4 text-sm font-medium">
                      {l.actorId === 0 ? (
                        <span className="text-foreground/40 italic">{t("audit.systemActor")}</span>
                      ) : (
                        <span className="text-foreground/80">{t("audit.user", { id: l.actorId })}</span>
                      )}
                    </td>
                    <td className="whitespace-nowrap px-5 py-4">
                      <Badge tone={getActionTone(l.action)} className="font-mono">
                        {l.action}
                      </Badge>
                    </td>
                    <td className="whitespace-nowrap px-5 py-4 text-foreground/70 text-sm">
                      <span className="capitalize">{l.targetType}</span>{" "}
                      <span className="text-foreground/40 font-mono text-xs">#{l.targetId}</span>
                    </td>
                    <td className="whitespace-nowrap px-5 py-4 text-foreground/55 font-mono text-xs">
                      {l.ip}
                    </td>
                    <td className="px-5 py-4 text-xs text-foreground/40 font-mono break-all max-w-xs">
                      {l.meta}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </GlassCard>
      )}
    </ScreenWrap>
  );
}
