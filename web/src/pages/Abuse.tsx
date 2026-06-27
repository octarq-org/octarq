import { useEffect, useState } from "react";
import { api, AbuseReport } from "../api";
import { timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button } from "../ui";

export default function AbusePage() {
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
      alert("Failed to update status: " + e.message);
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
        title="Abuse Reports"
        description="Manage reports of spam or malicious links"
      />

      <div className="mb-6 flex flex-wrap gap-2">
        {(["open", "reviewed", "dismissed", "all"] as const).map((filter) => (
          <Button
            key={filter}
            variant={statusFilter === filter ? "primary" : "subtle"}
            onClick={() => setStatusFilter(filter)}
            className="capitalize rounded-full px-4 py-1 text-xs"
          >
            {filter}
          </Button>
        ))}
      </div>

      {loading ? (
        <div className="text-white/40 py-12 text-center">loading…</div>
      ) : reports.length === 0 ? (
        <GlassCard className="p-10 text-center text-white/40">
          No abuse reports found.
        </GlassCard>
      ) : (
        <div className="space-y-4">
          {reports.map((r) => (
            <GlassCard key={r.id} className="p-5">
              <div className="flex flex-wrap items-start justify-between gap-4">
                <div>
                  <div className="flex flex-wrap items-center gap-2">
                    <h3 className="font-semibold text-lg text-rose-400">/{r.slug}</h3>
                    <Badge tone={getReasonTone(r.reason)} className="uppercase tracking-wider">
                      {r.reason}
                    </Badge>
                    <Badge tone={getStatusTone(r.status)} className="capitalize">
                      {r.status}
                    </Badge>
                  </div>
                  <div className="mt-2 text-sm text-white/55 break-all">
                    Target:{" "}
                    <a
                      href={r.target}
                      target="_blank"
                      rel="noreferrer"
                      className="text-indigo-400 hover:underline transition-colors"
                    >
                      {r.target}
                    </a>
                  </div>
                </div>
                <div className="text-left sm:text-right text-xs text-white/40">
                  <div title={r.createdAt}>{timeAgo(r.createdAt)}</div>
                  <div className="mt-1">IP: {r.reporterIp}</div>
                </div>
              </div>

              <div className="mt-4 text-sm text-white/75 bg-white/[0.03] p-4 rounded-xl border border-white/[0.06] font-normal leading-relaxed">
                {r.description || <span className="text-white/30 italic">No description provided.</span>}
              </div>

              {r.status === "open" && (
                <div className="mt-4 flex gap-2 justify-end border-t border-white/[0.06] pt-4">
                  <Button
                    variant="ghost"
                    onClick={() => updateStatus(r.id, "dismissed")}
                    className="text-xs py-1.5"
                  >
                    Dismiss (Safe)
                  </Button>
                  <Button
                    variant="danger"
                    onClick={() => updateStatus(r.id, "reviewed")}
                    className="text-xs py-1.5"
                  >
                    Mark Reviewed
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
