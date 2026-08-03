import { useEffect, useState } from "react";
import { api, AuditLog } from "../api";
import { timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Table, THead, TBody, TR, TH, TD } from "@octarq/plugin-sdk";
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
        <GlassCard className="overflow-hidden p-0">
          <Table>
            <THead className="border-b border-foreground/[0.06] bg-foreground/[0.02]">
              <TR>
                <TH>{t("audit.colTime")}</TH>
                <TH>{t("audit.colActor")}</TH>
                <TH>{t("audit.colAction")}</TH>
                <TH>{t("audit.colTarget")}</TH>
                <TH>{t("audit.colIp")}</TH>
                <TH>{t("audit.colMeta")}</TH>
              </TR>
            </THead>
            <TBody>
              {logs.map((l) => (
                <TR key={l.id}>
                  <TD className="whitespace-nowrap text-foreground/60 text-xs" title={l.createdAt}>
                    {timeAgo(l.createdAt)}
                  </TD>
                  <TD className="whitespace-nowrap text-sm font-medium">
                    {l.actorId === 0 ? (
                      <span className="text-foreground/40 italic">{t("audit.systemActor")}</span>
                    ) : (
                      <span className="text-foreground/80">{t("audit.user", { id: l.actorId })}</span>
                    )}
                  </TD>
                  <TD className="whitespace-nowrap">
                    <Badge tone={getActionTone(l.action)} className="font-mono">
                      {l.action}
                    </Badge>
                  </TD>
                  <TD className="whitespace-nowrap text-foreground/70 text-sm">
                    <span className="capitalize">{l.targetType}</span>{" "}
                    <span className="text-foreground/40 font-mono text-xs">#{l.targetId}</span>
                  </TD>
                  <TD className="whitespace-nowrap text-foreground/55 font-mono text-xs">
                    {l.ip}
                  </TD>
                  <TD className="text-xs text-foreground/40 font-mono break-all max-w-xs">
                    {l.meta}
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
        </GlassCard>
      )}
    </ScreenWrap>
  );
}
