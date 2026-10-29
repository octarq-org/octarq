import { timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button, Empty, TabBar, toast } from "@octarq/plugin-sdk";
import { ShieldAlert } from "lucide-react";
import { useEffect, useState } from "react";
import { api, AbuseReport } from "../api";
import { useTranslation } from "../i18n";
import { ListSkeleton } from "../components/ListSkeleton";

export default function AbusePage() {
  const { t } = useTranslation();
  const [reports, setReports] = useState<AbuseReport[]>([]);
  const [loading, setLoading] = useState(true);
  const [statusFilter, setStatusFilter] = useState("open");

  const load = () => {
    setLoading(true);
    api.abuseReports(statusFilter === "all" ? "" : statusFilter)
      .then(setReports)
      .finally(() => setLoading(false));
  };

  useEffect(() => {
    load();
  }, [statusFilter]);

  const updateStatus = async (id: number, status: string) => {
    try {
      await api.updateAbuseReport(id, status);
      load();
    } catch (e: any) {
      toast.error(t("abuse.updateFailed", { error: e.message }));
    }
  };

  const getStatusTone = (status: string) => {
    if (status === "open") return "amber";
    if (status === "reviewed") return "green";
    return "neutral";
  };

  const getReasonTone = (reason: string) => {
    if (reason === "phishing") return "red";
    if (reason === "malware") return "red";
    return "indigo";
  };

  return (
    <ScreenWrap>
      <PageHeader
        title={t("abuse.pageTitle")}
        description={t("abuse.pageDesc")}
      />

      {/* Same segmented strip as every other list page, so the status filter
          does not read as a separate pill control on one page only. */}
      <TabBar
        className="mb-6"
        size="sm"
        value={statusFilter}
        onValueChange={setStatusFilter}
        items={(["open", "reviewed", "dismissed", "all"] as const).map((filter) => ({
          value: filter,
          label: t("abuse.filter" + filter.charAt(0).toUpperCase() + filter.slice(1)),
        }))}
      />

      {loading ? (
        <ListSkeleton rows={5} ariaLabel={t("abuse.loading")} />
      ) : reports.length === 0 ? (
        <Empty reason={t("abuse.emptyState")}>
          <ShieldAlert />
        </Empty>
      ) : (
        <div className="space-y-4">
          {reports.map((r) => (
            <GlassCard key={r.id} className="p-5">
              <div className="flex flex-wrap items-start justify-between gap-4">
                <div>
                  <div className="flex flex-wrap items-center gap-2">
                    {/* AbuseReport model has no title/tags in backend; retain slug display */}
                    <h3 className="font-display text-[17px] font-semibold tracking-[-0.01em] text-danger-fg">/{r.slug}</h3>
                    <Badge tone={getReasonTone(r.reason)} className="uppercase tracking-wider">
                      {t("abuse.reason" + r.reason.charAt(0).toUpperCase() + r.reason.slice(1)) || r.reason}
                    </Badge>
                    <Badge tone={getStatusTone(r.status)} className="capitalize">
                      {t("abuse.filter" + r.status.charAt(0).toUpperCase() + r.status.slice(1)) || r.status}
                    </Badge>
                  </div>
                  <div className="mt-2 text-sm text-muted-foreground break-all">
                    {t("abuse.target")}{" "}
                    <a
                      href={r.target}
                      target="_blank"
                      rel="noreferrer"
                      className="text-accent-fg hover:underline transition-colors"
                    >
                      {r.target}
                    </a>
                  </div>
                </div>
                <div className="text-left sm:text-right text-xs text-muted-foreground">
                  <div className="font-mono tnum" title={r.createdAt}>{timeAgo(r.createdAt)}</div>
                  <div className="mt-1">{t("abuse.ip", { ip: r.reporterIp })}</div>
                </div>
              </div>

              <div className="mt-4 text-sm text-foreground bg-well p-4 rounded-xl border border-border font-normal leading-relaxed">
                {r.description || <span className="text-muted-foreground italic">{t("abuse.noDescription")}</span>}
              </div>

              {r.status === "open" && (
                <div className="mt-4 flex gap-2 justify-end border-t border-border pt-4">
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => updateStatus(r.id, "dismissed")}
                  >
                    {t("abuse.dismissSafe")}
                  </Button>
                  <Button
                    variant="danger"
                    size="sm"
                    onClick={() => updateStatus(r.id, "reviewed")}
                  >
                    {t("abuse.markReviewed")}
                  </Button>
                </div>
              )}
            </GlassCard>
          ))}
        </div>
      )}
    </ScreenWrap>
  );
}
