import { useEffect, useState } from "react";
import { api, AbuseReport } from "../api";
import { Header } from "./Links";
import { timeAgo } from "../ui";

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

  return (
    <div>
      <Header title="Abuse Reports" subtitle="Manage reports of spam or malicious links" />

      <div className="mb-4 flex gap-2">
        <button
          onClick={() => setStatusFilter("open")}
          className={`rounded-full px-3 py-1 text-sm ${statusFilter === "open" ? "bg-indigo-500 text-white" : "bg-white/[0.06] text-white/55 hover:text-white/80"}`}
        >
          Open
        </button>
        <button
          onClick={() => setStatusFilter("reviewed")}
          className={`rounded-full px-3 py-1 text-sm ${statusFilter === "reviewed" ? "bg-indigo-500 text-white" : "bg-white/[0.06] text-white/55 hover:text-white/80"}`}
        >
          Reviewed
        </button>
        <button
          onClick={() => setStatusFilter("dismissed")}
          className={`rounded-full px-3 py-1 text-sm ${statusFilter === "dismissed" ? "bg-indigo-500 text-white" : "bg-white/[0.06] text-white/55 hover:text-white/80"}`}
        >
          Dismissed
        </button>
        <button
          onClick={() => setStatusFilter("all")}
          className={`rounded-full px-3 py-1 text-sm ${statusFilter === "all" ? "bg-indigo-500 text-white" : "bg-white/[0.06] text-white/55 hover:text-white/80"}`}
        >
          All
        </button>
      </div>

      {loading ? (
        <div className="text-white/40 py-10 text-center">loading…</div>
      ) : reports.length === 0 ? (
        <div className="card p-8 text-center text-white/40">No abuse reports found.</div>
      ) : (
        <div className="space-y-4">
          {reports.map((r) => (
            <div key={r.id} className="card p-4">
              <div className="flex items-start justify-between">
                <div>
                  <div className="flex items-center gap-2">
                    <h3 className="font-semibold text-lg text-rose-400">/{r.slug}</h3>
                    <span className="rounded bg-white/[0.06] px-2 py-0.5 text-xs text-white/55 uppercase tracking-wide">
                      {r.reason}
                    </span>
                    <span className={`rounded px-2 py-0.5 text-xs tracking-wide ${r.status === 'open' ? 'bg-amber-500/20 text-amber-500' : 'bg-white/[0.06] text-white/40'}`}>
                      {r.status}
                    </span>
                  </div>
                  <div className="mt-1 text-sm text-white/55 break-all">
                    Target: <a href={r.target} target="_blank" rel="noreferrer" className="text-indigo-400 hover:underline">{r.target}</a>
                  </div>
                </div>
                <div className="text-right text-xs text-white/40">
                  <div title={r.createdAt}>{timeAgo(r.createdAt)}</div>
                  <div className="mt-1">IP: {r.reporterIp}</div>
                </div>
              </div>
              <div className="mt-3 text-sm text-white/75 bg-white/[0.03] p-3 rounded-lg border border-white/[0.06]/50">
                {r.description || <span className="text-white/30 italic">No description provided.</span>}
              </div>
              {r.status === "open" && (
                <div className="mt-4 flex gap-2 justify-end border-t border-white/[0.06] pt-3">
                  <button className="btn-ghost text-sm px-3 py-1.5" onClick={() => updateStatus(r.id, "dismissed")}>
                    Dismiss (Safe)
                  </button>
                  <button className="btn-primary text-sm px-3 py-1.5 bg-rose-600 hover:bg-rose-500" onClick={() => updateStatus(r.id, "reviewed")}>
                    Mark Reviewed
                  </button>
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
