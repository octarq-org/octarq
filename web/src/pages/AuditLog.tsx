import { useEffect, useState } from "react";
import { api, AuditLog } from "../api";
import { Header } from "./Links";
import { timeAgo } from "../ui";

export default function AuditLogPage() {
  const [logs, setLogs] = useState<AuditLog[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    api.auditLogs()
      .then(setLogs)
      .finally(() => setLoading(false));
  }, []);

  return (
    <div>
      <Header title="Audit Log" subtitle="History of administrative actions" />

      {loading ? (
        <div className="text-zinc-500 py-10 text-center">loading…</div>
      ) : logs.length === 0 ? (
        <div className="card p-8 text-center text-zinc-500">No audit logs found.</div>
      ) : (
        <div className="card overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-zinc-800 text-zinc-400">
              <tr>
                <th className="px-4 py-3 font-medium">Time</th>
                <th className="px-4 py-3 font-medium">Actor</th>
                <th className="px-4 py-3 font-medium">Action</th>
                <th className="px-4 py-3 font-medium">Target</th>
                <th className="px-4 py-3 font-medium">IP</th>
                <th className="px-4 py-3 font-medium">Meta</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-800/50">
              {logs.map((l) => (
                <tr key={l.id} className="hover:bg-zinc-800/20">
                  <td className="whitespace-nowrap px-4 py-3 text-zinc-400" title={l.createdAt}>
                    {timeAgo(l.createdAt)}
                  </td>
                  <td className="whitespace-nowrap px-4 py-3">
                    {l.actorId === 0 ? <span className="text-zinc-500">system/token</span> : `User ${l.actorId}`}
                  </td>
                  <td className="whitespace-nowrap px-4 py-3 font-medium text-indigo-300">
                    {l.action}
                  </td>
                  <td className="whitespace-nowrap px-4 py-3">
                    {l.targetType} <span className="text-zinc-500">#{l.targetId}</span>
                  </td>
                  <td className="whitespace-nowrap px-4 py-3 text-zinc-400">
                    {l.ip}
                  </td>
                  <td className="px-4 py-3 text-xs text-zinc-500 font-mono break-all max-w-xs">
                    {l.meta}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
