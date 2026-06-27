import { useEffect, useState } from "react";
import { api, FinanceSummary, Subscription } from "../api";
import { Empty, Field, Modal, Toggle } from "../ui";

const CURRENCIES = ["USD", "CNY", "EUR", "GBP", "JPY", "HKD", "SGD"];

function fmtCost(cost: number, currency: string) {
  return new Intl.NumberFormat("en-US", { style: "currency", currency, minimumFractionDigits: 2 }).format(cost);
}

function daysUntil(iso: string | null): number | null {
  if (!iso) return null;
  return Math.ceil((new Date(iso).getTime() - Date.now()) / 86400000);
}

function RenewalBadge({ nextRenewal }: { nextRenewal: string | null }) {
  const days = daysUntil(nextRenewal);
  if (days === null) return <span className="text-white/30 text-xs">—</span>;
  const date = new Date(nextRenewal!).toLocaleDateString();
  const urgent = days <= 7;
  const soon = days <= 14;
  return (
    <span className={`text-xs px-2 py-0.5 rounded-full ${
      urgent ? "bg-red-500/20 text-red-400" :
      soon   ? "bg-yellow-500/20 text-yellow-400" :
               "bg-white/[0.06] text-white/55"
    }`}>
      {date} ({days}d)
    </span>
  );
}

export default function FinancePage() {
  const [subs, setSubs] = useState<Subscription[]>([]);
  const [summary, setSummary] = useState<FinanceSummary | null>(null);
  const [editItem, setEditItem] = useState<Subscription | null>(null);
  const [showAdd, setShowAdd] = useState(false);
  const [error, setError] = useState<{ status: number; message: string } | null>(null);

  function load() {
    api.subscriptions()
      .then(setSubs)
      .catch((err) => setError({ status: err.status, message: err.message }));
    api.financeSummary()
      .then(setSummary)
      .catch(() => {});
  }

  useEffect(() => { load(); }, []);

  if (error) {
    return (
      <div className="card flex flex-col items-center justify-center gap-4 py-20 px-6 text-center">
        <div className="text-5xl">{error.status === 402 ? "🔒" : "🔌"}</div>
        <div>
          <h2 className="text-xl font-bold mb-1">
            {error.status === 402 ? "Pro Feature Locked" : "Feature Unavailable"}
          </h2>
          <p className="text-sm text-white/55 max-w-md mx-auto">
            {error.status === 402
              ? "A valid led-pro license is required to use Finance features."
              : "The Finance tracking feature is not available or disabled in this installation."}
          </p>
        </div>
        {error.status === 402 && (
          <a
            href="/settings/license"
            className="btn-primary mt-2"
          >
            Manage License
          </a>
        )}
      </div>
    );
  }

  async function toggleEnabled(sub: Subscription) {
    await api.updateSubscription(sub.id, { enabled: !sub.enabled });
    load();
  }

  async function deleteSub(sub: Subscription) {
    if (!confirm(`Remove "${sub.name}"?`)) return;
    await api.deleteSubscription(sub.id);
    load();
  }

  const currencies = summary ? Object.keys(summary.monthlyByCurrency) : [];

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="font-display text-2xl font-bold tracking-tight text-white">Finance</h1>
          <p className="text-sm text-white/55">Track recurring SaaS spend and renewal dates.</p>
        </div>
        <button className="btn-primary" onClick={() => setShowAdd(true)}>+ Add Subscription</button>
      </div>

      {/* Summary cards */}
      {summary && currencies.length > 0 && (
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
          <div className="card p-4">
            <div className="text-xs text-white/40 uppercase tracking-wider mb-1">Active</div>
            <div className="text-2xl font-bold">{summary.count}</div>
            <div className="text-xs text-white/40">subscriptions</div>
          </div>
          {currencies.map((cur) => (
            <div key={cur} className="card p-4">
              <div className="text-xs text-white/40 uppercase tracking-wider mb-1">Monthly · {cur}</div>
              <div className="text-2xl font-bold">{fmtCost(summary.monthlyByCurrency[cur], cur)}</div>
              <div className="text-xs text-white/40">{fmtCost(summary.yearlyByCurrency[cur], cur)} / yr</div>
            </div>
          ))}
          <div className="card p-4">
            <div className="text-xs text-white/40 uppercase tracking-wider mb-1">Renewing Soon</div>
            <div className={`text-2xl font-bold ${summary.renewingSoon.length > 0 ? "text-yellow-400" : ""}`}>
              {summary.renewingSoon.length}
            </div>
            <div className="text-xs text-white/40">within 14 days</div>
          </div>
        </div>
      )}

      {/* Subscription list */}
      {subs.length === 0 ? (
        <Empty>
          <div className="text-4xl mb-2">💳</div>
          <p>No subscriptions yet.</p>
          <button className="btn-primary mt-4" onClick={() => setShowAdd(true)}>Add Subscription</button>
        </Empty>
      ) : (
        <div className="card overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-white/[0.06] text-left text-xs text-white/40 uppercase tracking-wider">
                <th className="px-4 py-3">Name</th>
                <th className="px-4 py-3">Vendor</th>
                <th className="px-4 py-3 text-right">Cost</th>
                <th className="px-4 py-3">Cycle</th>
                <th className="px-4 py-3">Next Renewal</th>
                <th className="px-4 py-3 text-center">Active</th>
                <th className="px-4 py-3"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-white/[0.04]/60">
              {subs.map((sub) => (
                <tr key={sub.id} className={`hover:bg-white/[0.04] transition-colors ${!sub.enabled ? "opacity-50" : ""}`}>
                  <td className="px-4 py-3 font-medium">
                    {sub.name}
                    {sub.note && <div className="text-xs text-white/40 truncate max-w-[12rem]">{sub.note}</div>}
                  </td>
                  <td className="px-4 py-3 text-white/55">{sub.vendor || "—"}</td>
                  <td className="px-4 py-3 text-right font-mono">{fmtCost(sub.cost, sub.currency)}</td>
                  <td className="px-4 py-3 text-white/55 capitalize">{sub.cycle}</td>
                  <td className="px-4 py-3"><RenewalBadge nextRenewal={sub.nextRenewal} /></td>
                  <td className="px-4 py-3 text-center">
                    <Toggle on={sub.enabled} onChange={() => toggleEnabled(sub)} />
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex gap-2 justify-end">
                      <button className="btn-ghost text-xs px-2" onClick={() => setEditItem(sub)}>Edit</button>
                      <button
                        className="btn-ghost text-xs px-2 text-red-400 hover:text-red-300 hover:bg-red-950"
                        onClick={() => deleteSub(sub)}
                      >
                        Remove
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {(showAdd || editItem) && (
        <SubModal
          sub={editItem}
          onClose={() => { setShowAdd(false); setEditItem(null); }}
          onSaved={load}
        />
      )}
    </div>
  );
}

function SubModal({ sub, onClose, onSaved }: { sub: Subscription | null; onClose: () => void; onSaved: () => void }) {
  const [name, setName] = useState(sub?.name ?? "");
  const [vendor, setVendor] = useState(sub?.vendor ?? "");
  const [cost, setCost] = useState(sub?.cost?.toString() ?? "");
  const [currency, setCurrency] = useState(sub?.currency ?? "USD");
  const [cycle, setCycle] = useState<"monthly" | "yearly">(sub?.cycle ?? "monthly");
  const [nextRenewal, setNextRenewal] = useState(
    sub?.nextRenewal ? sub.nextRenewal.slice(0, 10) : ""
  );
  const [note, setNote] = useState(sub?.note ?? "");
  const [enabled, setEnabled] = useState(sub?.enabled ?? true);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr("");
    try {
      const payload = {
        name,
        vendor,
        cost: parseFloat(cost) || 0,
        currency,
        cycle,
        nextRenewal: nextRenewal ? new Date(nextRenewal).toISOString() : null,
        note,
        enabled,
      };
      if (sub) {
        await api.updateSubscription(sub.id, payload);
      } else {
        await api.createSubscription(payload);
      }
      onSaved();
      onClose();
    } catch (e: any) {
      setErr(e.message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal title={sub ? "Edit Subscription" : "Add Subscription"} onClose={onClose}>
      <form onSubmit={submit}>
        <Field label="Name">
          <input className="input w-full" value={name} onChange={(e) => setName(e.target.value)} required autoFocus />
        </Field>
        <Field label="Vendor">
          <input className="input w-full" value={vendor} onChange={(e) => setVendor(e.target.value)} placeholder="e.g. GitHub, Vercel" />
        </Field>
        <div className="flex gap-3">
          <div className="flex-1">
            <Field label="Cost">
              <input className="input w-full font-mono" type="number" min="0" step="0.01" value={cost} onChange={(e) => setCost(e.target.value)} required />
            </Field>
          </div>
          <div className="w-28">
            <Field label="Currency">
              <select className="input w-full" value={currency} onChange={(e) => setCurrency(e.target.value)}>
                {CURRENCIES.map((c) => <option key={c}>{c}</option>)}
              </select>
            </Field>
          </div>
        </div>
        <Field label="Billing Cycle">
          <div className="flex gap-3">
            {(["monthly", "yearly"] as const).map((c) => (
              <label key={c} className="flex items-center gap-2 cursor-pointer">
                <input type="radio" name="cycle" value={c} checked={cycle === c} onChange={() => setCycle(c)} />
                <span className="capitalize text-sm">{c}</span>
              </label>
            ))}
          </div>
        </Field>
        <Field label="Next Renewal">
          <input className="input w-full" type="date" value={nextRenewal} onChange={(e) => setNextRenewal(e.target.value)} />
        </Field>
        <Field label="Note">
          <input className="input w-full" value={note} onChange={(e) => setNote(e.target.value)} placeholder="Optional note" />
        </Field>
        <div className="flex items-center gap-3 mb-4">
          <Toggle on={enabled} onChange={setEnabled} />
          <span className="text-sm text-white/55">Active</span>
        </div>
        {err && <p className="mb-3 text-sm text-red-400">{err}</p>}
        <div className="flex justify-end gap-2 mt-4">
          <button type="button" className="btn-ghost" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn-primary" disabled={busy || !name}>
            {busy ? "..." : "Save"}
          </button>
        </div>
      </form>
    </Modal>
  );
}
