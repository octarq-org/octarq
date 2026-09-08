import { useMemo } from "react";
import { z } from "zod";
import { api, AuditLog } from "../api";
import { timeAgo, ScreenWrap, PageHeader, Badge } from "@octarq/plugin-sdk";
import { useTranslation } from "@octarq/plugin-sdk";
import { ProTable, ProColumn } from "../components/pro-table";

// Runtime type boundary safety using Zod schema
export const AuditLogSchema = z.object({
  id: z.number(),
  orgId: z.number(),
  actorId: z.number(),
  action: z.string(),
  targetType: z.string(),
  targetId: z.number(),
  meta: z.string().default(""),
  ip: z.string().default(""),
  createdAt: z.string(),
});

export default function AuditLogPage() {
  const { t } = useTranslation();

  const getActionTone = (action: string) => {
    if (action.includes(".delete")) return "red";
    if (action.includes(".create")) return "green";
    if (action.includes(".update")) return "amber";
    return "indigo";
  };

  const columns: ProColumn<AuditLog>[] = useMemo(
    () => [
      {
        title: t("audit.colTime"),
        dataIndex: "createdAt",
        key: "createdAt",
        hideInSearch: true,
        sorter: true,
        render: (_, record) => (
          <span
            className="whitespace-nowrap font-mono tnum text-xs text-foreground/60"
            title={record.createdAt}
          >
            {timeAgo(record.createdAt)}
          </span>
        ),
      },
      {
        title: t("audit.colActor"),
        dataIndex: "actorId",
        key: "actorId",
        hideInSearch: true,
        render: (_, record) => (
          <span className="whitespace-nowrap text-sm font-medium">
            {record.actorId === 0 ? (
              <span className="text-foreground/40 italic">
                {t("audit.systemActor")}
              </span>
            ) : (
              <span className="text-foreground/80">
                {t("audit.user", { id: record.actorId })}
              </span>
            )}
          </span>
        ),
      },
      {
        title: t("audit.colAction"),
        dataIndex: "action",
        key: "action",
        valueType: "text",
        render: (_, record) => (
          <Badge tone={getActionTone(record.action)} className="font-mono">
            {record.action}
          </Badge>
        ),
      },
      {
        title: t("audit.colTarget"),
        dataIndex: "targetType",
        key: "targetType",
        valueType: "text",
        render: (_, record) => (
          <span className="whitespace-nowrap text-foreground/70 text-sm">
            <span className="capitalize">{record.targetType}</span>{" "}
            <span className="text-foreground/40 font-mono text-xs">
              #{record.targetId}
            </span>
          </span>
        ),
      },
      {
        title: t("audit.colIp"),
        dataIndex: "ip",
        key: "ip",
        hideInSearch: true,
        render: (_, record) => (
          <span className="whitespace-nowrap text-foreground/55 font-mono text-xs">
            {record.ip}
          </span>
        ),
      },
      {
        title: t("audit.colMeta"),
        dataIndex: "meta",
        key: "meta",
        hideInSearch: true,
        render: (_, record) => (
          <span className="text-xs text-foreground/40 font-mono break-all max-w-xs block">
            {record.meta}
          </span>
        ),
      },
    ],
    [t]
  );

  const request = async (params: {
    page: number;
    pageSize: number;
    action?: string;
    targetType?: string;
  }) => {
    const limit = params.pageSize;
    const offset = (params.page - 1) * params.pageSize;
    const logs = await api.auditLogs({
      action: params.action,
      targetType: params.targetType,
      limit,
      offset,
    });

    const total = logs.length === limit ? offset + limit + 1 : offset + logs.length;

    return {
      data: logs,
      total,
      success: true,
    };
  };

  return (
    <ScreenWrap>
      <PageHeader
        title={t("audit.pageTitle")}
        description={t("audit.pageDesc")}
      />

      <ProTable<AuditLog>
        columns={columns}
        request={request}
        schema={AuditLogSchema}
        rowKey="id"
        queryKeyPrefix="auditLogs"
        pagination={{
          defaultPageSize: 10,
          pageSizeOptions: [10, 20, 50],
        }}
        emptyText={t("audit.emptyState")}
        ariaLabel={t("audit.loading")}
      />
    </ScreenWrap>
  );
}
