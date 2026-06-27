import { useEffect, useState } from "react";
import { api, FinanceSummary, Subscription } from "../api";
import { Empty, Field, Modal, Toggle, ScreenWrap, PageHeader, GlassCard, Badge, Button, StatCard } from "../ui";
import { ShieldAlert, CreditCard, Calendar, TrendingUp, Sparkles, Trash2, Pencil } from "lucide-react";

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
  
  const tone = urgent ? "red" : soon ? "amber" : "neutral";
  return (
    <Badge tone={tone}>
      {date} ({days}d)
    </Badge>
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
      <ScreenWrap>
        <GlassCard className="flex flex-col items-center justify-center gap-5 py-16 px-6 text-center max-w-md mx-auto mt-12">
          <div className="h-14 w-14 rounded-2xl bg-rose-500/10 flex items-center justify-center text-rose-400">
            <ShieldAlert className="h-8 w-8" />
          </div>
          <div>
            <h2 className="text-xl font-bold mb-2">
              {error.status === 402 ? "Pro Feature Locked" : "Feature Unavailable"}
            </h2>
            <p className="text-sm text-white/50 leading-relaxed">
              {error.status === 402
                ? "A valid led-pro license is required to use Finance features."
                : "The Finance tracking feature is not available or disabled in this installation."}
            </p>
          </div>
          {error.status === 402 && (
            <Button
              variant="primary"
              onClick={() => window.location.href = "/settings/license"}
              className="mt-2"
            >
              Manage License
            </Button>
          )}
        </GlassCard>
      </ScreenWrap>
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
    <ScreenWrap>
      <PageHeader
        title="Finance"
        description="Track recurring SaaS spend and renewal dates"
        action={
          <Button variant="primary" onClick={() => setShowAdd(true)}>
            + Add Subscription
          </Button>
        }
      />

      {/* Summary cards */}
      {summary && currencies.length > 0 && (
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
          <StatCard
            label="Active Subscriptions"
            value={summary.count}
            icon={<CreditCard className="h-4 w-4" />}
            index={0}
          />
          {currencies.map((cur, i) => (
            <StatCard
              key={cur}
              label={`Monthly Spend (${cur})`}
              value={fmtCost(summary.monthlyByCurrency[cur], cur)}
              delta={`${fmtCost(summary.yearlyByCurrency[cur], cur)} / yr`}
              positive={true}
              icon={<TrendingUp className="h-4 w-4" />}
              index={i + 1}
            />
          ))}
          <StatCard
            label="Renewing Soon"
            value={summary.renewingSoon.length}
            delta="within 14 days"
            positive={summary.renewingSoon.length === 0}
            icon={<Calendar className="h-4 w-4" />}
            index={currencies.length + 1}
          />
        </div>
      )}

      {/* Subscription list */}
      {subs.length === 0 ? (
        <Empty>
          <CreditCard className="h-10 w-10 text-white/30 mb-2" />
          <p className="text-sm text-white/50">No subscriptions yet.</p>
          <Button variant="primary" className="mt-4" onClick={() => setShowAdd(true)}>
            Add Subscription
          </Button>
        </Empty>
      ) : (
        <GlassCard className="overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-sm border-collapse">
              <thead className="border-b border-white/[0.06] bg-white/[0.02] text-white/55">
                <tr className="text-left text-xs font-semibold uppercase tracking-wider">
                  <th className="px-5 py-3.5">Name</th>
                  <th className="px-5 py-3.5">Vendor</th>
                  <th className="px-5 py-3.5 text-right">Cost</th>
                  <th className="px-5 py-3.5">Cycle</th>
                  <th className="px-5 py-3.5">Next Renewal</th>
                  <th className="px-5 py-3.5 text-center">Active</th>
                  <th className="px-5 py-3.5"></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/[0.04]">
                {subs.map((sub) => (
                  <tr
                    key={sub.id}
                    className={`hover:bg-white/[0.02] transition-all ${
                      !sub.enabled ? "opacity-45 bg-black/10" : ""
                    }`}
                  >
                    <td className="px-5 py-4 font-medium text-white">
                      {sub.name}
                      {sub.note && (
                        <div className="text-xs text-white/40 truncate max-w-[12rem] mt-0.5" title={sub.note}>
                          {sub.note}
                        </div>
                      )}
                    </td>
                    <td className="px-5 py-4 text-white/60">{sub.vendor || "—"}</td>
                    <td className="px-5 py-4 text-right font-mono text-white/90">
                      {fmtCost(sub.cost, sub.currency)}
                    </td>
                    <td className="px-5 py-4 text-white/55 capitalize text-xs">{sub.cycle}</td>
                    <td className="px-5 py-4">
                      <RenewalBadge nextRenewal={sub.nextRenewal} />
                    </td>
                    <td className="px-5 py-4 text-center">
                      <div className="inline-flex items-center justify-center">
                        <Toggle on={sub.enabled} onChange={() => toggleEnabled(sub)} />
                      </div>
                    </td>
                    <td className="px-5 py-4">
                      <div className="flex gap-2 justify-end">
                        <Button
                          variant="ghost"
                          onClick={() => setEditItem(sub)}
                          className="text-xs py-1 px-2.5"
                        >
                          <Pencil className="h-3 w-3 mr-1" />
                          Edit
                        </Button>
                        <Button
                          variant="danger"
                          onClick={() => deleteSub(sub)}
                          className="text-xs py-1 px-2.5 bg-rose-500/0 hover:bg-rose-500/10 border-0"
                        >
                          <Trash2 className="h-3.5 w-3.5 mr-1" />
                          Remove
                        </Button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </GlassCard>
      )}

      {(showAdd || editItem) && (
        <SubModal
          sub={editItem}
          onClose={() => { setShowAdd(false); setEditItem(null); }}
          onSaved={load}
        />
      )}
    </ScreenWrap>
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
    <Modal title={sub ? "Edit Subscription" : "Create Subscription"} onClose={onClose}>
      <form onSubmit={submit} className="space-y-4">
        <Field label="Subscription Name">
          <input className="input w-full" value={name} onChange={(e) => setName(e.target.value)} required autoFocus placeholder="e.g. GitHub Copilot" />
        </Field>
        
        <Field label="Vendor Name">
          <input className="input w-full" value={vendor} onChange={(e) => setVendor(e.target.value)} placeholder="e.g. GitHub, Vercel" />
        </Field>
        
        <div className="flex gap-4">
          <div className="flex-1">
            <Field label="Cost">
              <input className="input w-full font-mono" type="number" min="0" step="0.01" value={cost} onChange={(e) => setCost(e.target.value)} required />
            </Field>
          </div>
          <div className="w-32">
            <Field label="Currency">
              <select className="input w-full" value={currency} onChange={(e) => setCurrency(e.target.value)}>
                {CURRENCIES.map((c) => <option key={c}>{c}</option>)}
              </select>
            </Field>
          </div>
        </div>

        <Field label="Billing Cycle">
          <div className="flex gap-4 mt-1">
            {(["monthly", "yearly"] as const).map((c) => (
              <label key={c} className="flex items-center gap-2 cursor-pointer select-none">
                <input
                  type="radio"
                  name="cycle"
                  value={c}
                  checked={cycle === c}
                  onChange={() => setCycle(c)}
                  className="accent-indigo-500"
                />
                <span className="capitalize text-sm text-white/70">{c}</span>
              </label>
            ))}
          </div>
        </Field>

        <Field label="Next Renewal Date">
          <input className="input w-full" type="date" value={nextRenewal} onChange={(e) => setNextRenewal(e.target.value)} />
        </Field>

        <Field label="Private Note">
          <input className="input w-full" value={note} onChange={(e) => setNote(e.target.value)} placeholder="e.g. charged to corporate card" />
        </Field>

        <div className="flex items-center gap-3 pt-2">
          <Toggle on={enabled} onChange={setEnabled} />
          <span className="text-sm text-white/60 select-none">Active Subscription</span>
        </div>

        {err && <p className="text-sm text-rose-400 font-medium">{err}</p>}
        
        <div className="flex justify-end gap-2.5 pt-4 border-t border-white/[0.06]">
          <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" disabled={busy || !name}>
            {busy ? "Saving..." : "Save Subscription"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
