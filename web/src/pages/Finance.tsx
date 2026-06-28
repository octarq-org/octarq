import { useEffect, useState } from "react";
import { api, FinanceSummary, Subscription } from "../api";
import { Empty, Field, Modal, Toggle, ScreenWrap, PageHeader, GlassCard, Badge, Button, StatCard } from "../ui";
import { ShieldAlert, CreditCard, Calendar, TrendingUp, Trash2, Pencil, Landmark, Plus, ArrowUpRight, ArrowDownRight, Wallet, RefreshCw } from "lucide-react";

const CURRENCIES = ["USD", "CNY", "EUR", "GBP", "JPY", "HKD", "SGD"];

interface Transaction {
  id: string;
  date: string; // payment date or last renewal date
  type: "income" | "expense";
  title: string;
  category: string;
  amount: number;
  currency: string;
  cycle: "one-off" | "monthly" | "yearly";
  nextRenewal?: string | null;
  isSubscription?: boolean;
  subRef?: Subscription; // reference to backend subscription object
}

const DEFAULT_TRANSACTIONS: Transaction[] = [
  { id: "tx-1", date: "2026-06-25", type: "income", title: "Domain Sale: webdev.io", category: "Domain Trading", amount: 1850.00, currency: "USD", cycle: "one-off" },
  { id: "tx-2", date: "2026-06-22", type: "expense", title: "Hetzner Cloud VPS rental", category: "Infrastructure", amount: 48.50, currency: "USD", cycle: "one-off" },
  { id: "tx-3", date: "2026-06-18", type: "expense", title: "AWS Route53 renew: mycorp.com", category: "Domain Registration", amount: 12.00, currency: "USD", cycle: "one-off" },
  { id: "tx-4", date: "2026-06-15", type: "income", title: "Consulting: DNS Cluster Setup", category: "Services", amount: 650.00, currency: "USD", cycle: "one-off" },
  { id: "tx-5", date: "2026-06-10", type: "expense", title: "Google Workspace renewal", category: "SaaS Tools", amount: 36.00, currency: "USD", cycle: "one-off" },
];

function fmtCost(cost: number, currency: string) {
  return new Intl.NumberFormat("en-US", { style: "currency", currency, minimumFractionDigits: 2 }).format(cost);
}

export default function FinancePage() {
  const [filterType, setFilterType] = useState<"all" | "recurring" | "one-off">("all");
  const [subs, setSubs] = useState<Subscription[]>([]);
  const [summary, setSummary] = useState<FinanceSummary | null>(null);
  
  // Modal toggle states
  const [showAddModal, setShowAddModal] = useState(false);
  const [editingSub, setEditingSub] = useState<Subscription | null>(null);

  const [oneOffTx, setOneOffTx] = useState<Transaction[]>([]);
  const [error, setError] = useState<{ status: number; message: string } | null>(null);

  function load() {
    api.subscriptions()
      .then(setSubs)
      .catch((err) => setError({ status: err.status, message: err.message }));
    api.financeSummary()
      .then(setSummary)
      .catch(() => {});

    // Load one-off ledger from localStorage
    const saved = localStorage.getItem("led_finance_oneoff");
    if (saved) {
      try {
        setOneOffTx(JSON.parse(saved));
      } catch {
        setOneOffTx(DEFAULT_TRANSACTIONS);
      }
    } else {
      setOneOffTx(DEFAULT_TRANSACTIONS);
      localStorage.setItem("led_finance_oneoff", JSON.stringify(DEFAULT_TRANSACTIONS));
    }
  }

  useEffect(() => { load(); }, []);

  // Save one-off ledger list to localStorage
  const saveOneOffList = (list: Transaction[]) => {
    setOneOffTx(list);
    localStorage.setItem("led_finance_oneoff", JSON.stringify(list));
  };

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

  // Delete subscription (backend)
  async function handleDeleteSubscription(subId: number) {
    if (!confirm("Are you sure you want to remove this recurring subscription?")) return;
    try {
      await api.deleteSubscription(subId);
      load();
    } catch (e: any) {
      alert(e.message || "Failed to remove subscription");
    }
  }

  // Delete one-off transaction (localStorage)
  function handleDeleteOneOff(txId: string) {
    if (!confirm("Delete this transaction record?")) return;
    const updated = oneOffTx.filter(t => t.id !== txId);
    saveOneOffList(updated);
  }

  // Unified Save / Add function (Handles either Subscriptions or One-off transactions)
  async function handleAddTransaction(payload: {
    title: string;
    type: "income" | "expense";
    category: string;
    amount: number;
    currency: string;
    cycle: "one-off" | "monthly" | "yearly";
    nextRenewalDate?: string;
    date: string;
  }) {
    if (payload.cycle === "one-off") {
      const newTx: Transaction = {
        id: "tx-" + Date.now(),
        date: payload.date,
        type: payload.type,
        title: payload.title,
        category: payload.category,
        amount: payload.amount,
        currency: payload.currency,
        cycle: "one-off",
      };
      const updated = [newTx, ...oneOffTx];
      saveOneOffList(updated);
    } else {
      // Save recurring to backend as Subscription
      const subPayload = {
        name: payload.title,
        vendor: payload.category, // map category to vendor or keep it as vendor
        cost: payload.amount,
        currency: payload.currency,
        cycle: payload.cycle,
        nextRenewal: payload.nextRenewalDate ? new Date(payload.nextRenewalDate).toISOString() : null,
        note: `Auto-generated via ledger`,
        enabled: true,
      };
      await api.createSubscription(subPayload);
      load();
    }
  }

  // Merge subscriptions (Server) + One-offs (Local)
  const virtualSubs: Transaction[] = subs.map(s => {
    let paymentDate = new Date().toISOString().slice(0, 10);
    if (s.nextRenewal) {
      const next = new Date(s.nextRenewal);
      if (s.cycle === "monthly") {
        next.setMonth(next.getMonth() - 1);
      } else {
        next.setFullYear(next.getFullYear() - 1);
      }
      paymentDate = next.toISOString().slice(0, 10);
    }
    return {
      id: `sub-tx-${s.id}`,
      date: paymentDate,
      type: "expense" as const,
      title: s.name,
      category: s.vendor || "SaaS Tools",
      amount: s.cost,
      currency: s.currency,
      cycle: s.cycle,
      nextRenewal: s.nextRenewal,
      isSubscription: true,
      subRef: s,
    };
  });

  const allTransactions = [...virtualSubs, ...oneOffTx];
  allTransactions.sort((a, b) => b.date.localeCompare(a.date));

  // Apply filters
  const filteredTransactions = allTransactions.filter(tx => {
    if (filterType === "recurring") return tx.cycle === "monthly" || tx.cycle === "yearly";
    if (filterType === "one-off") return tx.cycle === "one-off";
    return true;
  });

  // Calculate ledger stats (Closed-loop ledger calculations over ALL transactions)
  const totalIncome = allTransactions.filter(t => t.type === "income").reduce((acc, t) => acc + t.amount, 0);
  const totalExpense = allTransactions.filter(t => t.type === "expense").reduce((acc, t) => acc + t.amount, 0);
  const netBalance = totalIncome - totalExpense;

  return (
    <ScreenWrap>
      <PageHeader
        title="Finance Workspace"
        description="Unified ledger tracking SaaS recurring cost streams and one-off workspace cash flow"
        action={
          <Button variant="primary" onClick={() => setShowAddModal(true)} className="gap-1">
            <Plus className="h-4 w-4" /> Add Transaction
          </Button>
        }
      />

      {/* Cash Ledger Stats Card Group */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
        <StatCard
          label="Total Revenue Income"
          value={fmtCost(totalIncome, "USD")}
          delta="Total cash inflows received"
          icon={<ArrowUpRight className="h-4 w-4 text-emerald-400" />}
          positive={true}
          index={0}
        />
        <StatCard
          label="Total Expenditures"
          value={fmtCost(totalExpense, "USD")}
          delta="Total subscription & one-off fees"
          icon={<ArrowDownRight className="h-4 w-4 text-rose-400" />}
          positive={false}
          index={1}
        />
        <StatCard
          label="Net Cash Profit"
          value={fmtCost(netBalance, "USD")}
          delta={netBalance >= 0 ? "Surplus" : "Deficit"}
          icon={<Wallet className="h-4 w-4" />}
          positive={netBalance >= 0}
          index={2}
        />
      </div>

      {/* Filter Options */}
      <div className="flex justify-between items-center mb-4 gap-4 flex-wrap">
        <div className="flex gap-1.5 p-1 rounded-xl bg-black/25 border border-white/[0.05] max-w-md shrink-0">
          <button
            onClick={() => setFilterType("all")}
            className={`rounded-lg px-4 py-1.5 text-xs font-semibold transition-all ${
              filterType === "all" ? "bg-white/[0.08] text-white shadow-glow" : "text-white/50 hover:text-white/80"
            }`}
          >
            All Logs ({allTransactions.length})
          </button>
          <button
            onClick={() => setFilterType("recurring")}
            className={`rounded-lg px-4 py-1.5 text-xs font-semibold transition-all flex items-center gap-1 ${
              filterType === "recurring" ? "bg-white/[0.08] text-white shadow-glow" : "text-white/50 hover:text-white/80"
            }`}
          >
            <RefreshCw className="h-3 w-3" /> Recurring Subscriptions ({virtualSubs.length})
          </button>
          <button
            onClick={() => setFilterType("one-off")}
            className={`rounded-lg px-4 py-1.5 text-xs font-semibold transition-all ${
              filterType === "one-off" ? "bg-white/[0.08] text-white shadow-glow" : "text-white/50 hover:text-white/80"
            }`}
          >
            One-off Ledger ({oneOffTx.length})
          </button>
        </div>
      </div>

      {/* Unified Table */}
      {filteredTransactions.length === 0 ? (
        <Empty>
          <Landmark className="h-10 w-10 text-white/30 mb-2" />
          <p className="text-sm text-white/50">No transaction logs match current filters.</p>
        </Empty>
      ) : (
        <GlassCard className="overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-sm border-collapse">
              <thead className="border-b border-white/[0.06] bg-white/[0.02] text-white/55">
                <tr className="text-left text-xs font-semibold uppercase tracking-wider">
                  <th className="px-5 py-3.5">Date</th>
                  <th className="px-5 py-3.5">Flow</th>
                  <th className="px-5 py-3.5">Title</th>
                  <th className="px-5 py-3.5">Category / Vendor</th>
                  <th className="px-5 py-3.5">Payment Cycle</th>
                  <th className="px-5 py-3.5 text-right">Amount</th>
                  <th className="px-5 py-3.5"></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/[0.04]">
                {filteredTransactions.map((tx) => (
                  <tr key={tx.id} className={`hover:bg-white/[0.02] transition-all ${tx.isSubscription ? "bg-white/[0.01]" : ""}`}>
                    <td className="px-5 py-4 font-mono text-xs text-white/60">
                      {tx.cycle === "one-off" ? tx.date : (tx.nextRenewal ? `Renews ${new Date(tx.nextRenewal).toLocaleDateString()}` : "Recurring")}
                    </td>
                    <td className="px-5 py-4">
                      <Badge tone={tx.type === "income" ? "green" : "red"} className="uppercase font-bold tracking-wider text-[9px]">
                        {tx.type}
                      </Badge>
                    </td>
                    <td className="px-5 py-4 text-white font-medium">
                      <div className="flex items-center gap-2">
                        <span>{tx.title}</span>
                        {tx.isSubscription && (
                          <Badge tone="indigo" className="text-[9px] font-semibold uppercase tracking-wider px-1.5 py-0">SaaS Sub</Badge>
                        )}
                      </div>
                    </td>
                    <td className="px-5 py-4">
                      <Badge tone="neutral" className="text-white/60 bg-white/5 border border-white/[0.05]">
                        {tx.category}
                      </Badge>
                    </td>
                    <td className="px-5 py-4 capitalize text-xs text-white/65">
                      {tx.cycle === "one-off" ? (
                        <span className="text-white/40">One-off</span>
                      ) : (
                        <span className="text-indigo-400 font-semibold flex items-center gap-1">
                          <RefreshCw className="h-3 w-3 animate-spin-slow" /> {tx.cycle}
                        </span>
                      )}
                    </td>
                    <td className={`px-5 py-4 text-right font-mono font-semibold ${tx.type === "income" ? "text-emerald-400" : "text-rose-400"}`}>
                      {tx.type === "income" ? "+" : "-"} {fmtCost(tx.amount, tx.currency)}
                    </td>
                    <td className="px-5 py-4">
                      <div className="flex gap-2 justify-end">
                        {tx.isSubscription ? (
                          <>
                            <Button
                              variant="ghost"
                              onClick={() => setEditingSub(tx.subRef || null)}
                              className="text-xs py-1 px-2.5"
                            >
                              <Pencil className="h-3.5 w-3.5 mr-1" /> Edit
                            </Button>
                            <Button
                              variant="danger"
                              onClick={() => tx.subRef && handleDeleteSubscription(tx.subRef.id)}
                              className="text-xs py-1 px-2 bg-rose-500/0 hover:bg-rose-500/10 border-0"
                            >
                              <Trash2 className="h-3.5 w-3.5" />
                            </Button>
                          </>
                        ) : (
                          <Button
                            variant="danger"
                            onClick={() => handleDeleteOneOff(tx.id)}
                            className="text-xs py-1 px-2 bg-rose-500/0 hover:bg-rose-500/10 border-0"
                          >
                            <Trash2 className="h-3.5 w-3.5" />
                          </Button>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </GlassCard>
      )}

      {/* Unified Add Transaction Modal */}
      {showAddModal && (
        <AddTransactionModal
          onClose={() => setShowAddModal(false)}
          onAdd={handleAddTransaction}
        />
      )}

      {/* Edit Subscription Modal */}
      {editingSub && (
        <EditSubscriptionModal
          sub={editingSub}
          onClose={() => setEditingSub(null)}
          onSaved={() => { setEditingSub(null); load(); }}
        />
      )}
    </ScreenWrap>
  );
}

function AddTransactionModal({
  onClose,
  onAdd,
}: {
  onClose: () => void;
  onAdd: (payload: {
    title: string;
    type: "income" | "expense";
    category: string;
    amount: number;
    currency: string;
    cycle: "one-off" | "monthly" | "yearly";
    nextRenewalDate?: string;
    date: string;
  }) => Promise<void>;
}) {
  const [title, setTitle] = useState("");
  const [type, setType] = useState<"income" | "expense">("expense");
  const [category, setCategory] = useState("SaaS Tools");
  const [amount, setAmount] = useState("");
  const [currency, setCurrency] = useState("USD");
  const [cycle, setCycle] = useState<"one-off" | "monthly" | "yearly">("one-off");
  
  // Specific fields
  const [date, setDate] = useState(new Date().toISOString().slice(0, 10));
  const [nextRenewalDate, setNextRenewalDate] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  const categories = [
    "SaaS Tools",
    "Infrastructure",
    "Domain Registration",
    "Domain Trading",
    "Services",
    "Advertising",
    "Consulting",
    "Other",
  ];

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr("");
    try {
      await onAdd({
        title,
        type,
        category,
        amount: parseFloat(amount) || 0,
        currency,
        cycle,
        date,
        nextRenewalDate,
      });
      onClose();
    } catch (e: any) {
      setErr(e.message || "Failed to save transaction");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal title="Log Financial Transaction" onClose={onClose}>
      <form onSubmit={submit} className="space-y-4">
        <Field label="Transaction Title">
          <input className="input w-full font-sans" value={title} onChange={(e) => setTitle(e.target.value)} required placeholder="e.g. Vercel Hosting / Client Invoice" autoFocus />
        </Field>

        <div className="flex gap-4">
          <div className="flex-1">
            <Field label="Flow Type">
              <select className="input w-full text-sm" value={type} onChange={(e) => setType(e.target.value as "income" | "expense")}>
                <option value="expense">Expenditure (Out)</option>
                <option value="income">Revenue Income (In)</option>
              </select>
            </Field>
          </div>
          <div className="flex-1">
            <Field label="Category / Vendor">
              <select className="input w-full text-sm" value={category} onChange={(e) => setCategory(e.target.value)}>
                {categories.map((c) => <option key={c}>{c}</option>)}
              </select>
            </Field>
          </div>
        </div>

        <div className="flex gap-4">
          <div className="flex-1">
            <Field label="Amount">
              <input className="input w-full font-mono text-sm" type="number" min="0" step="0.01" value={amount} onChange={(e) => setAmount(e.target.value)} required placeholder="0.00" />
            </Field>
          </div>
          <div className="w-28">
            <Field label="Currency">
              <select className="input w-full text-sm" value={currency} onChange={(e) => setCurrency(e.target.value)}>
                {CURRENCIES.map((c) => <option key={c}>{c}</option>)}
              </select>
            </Field>
          </div>
        </div>

        <div className="border-t border-white/[0.05] pt-4 space-y-4">
          <Field label="Payment Term / Cycle">
            <div className="flex gap-4 mt-1">
              {([
                { value: "one-off", label: "One-off (一次性)" },
                { value: "monthly", label: "Monthly (每月)" },
                { value: "yearly", label: "Yearly (每年)" },
              ] as const).map((item) => (
                <label key={item.value} className="flex items-center gap-2 cursor-pointer select-none">
                  <input
                    type="radio"
                    name="cycle"
                    value={item.value}
                    checked={cycle === item.value}
                    onChange={() => {
                      setCycle(item.value);
                      if (item.value !== "one-off") setType("expense"); // SaaS subscriptions are typically expenses
                    }}
                    className="accent-indigo-500"
                  />
                  <span className="text-xs text-white/70">{item.label}</span>
                </label>
              ))}
            </div>
          </Field>

          {cycle === "one-off" ? (
            <Field label="Transaction Date">
              <input className="input w-full text-sm font-sans" type="date" value={date} onChange={(e) => setDate(e.target.value)} required />
            </Field>
          ) : (
            <Field label="Next Renewal Date">
              <input className="input w-full text-sm font-sans" type="date" value={nextRenewalDate} onChange={(e) => setNextRenewalDate(e.target.value)} required />
            </Field>
          )}
        </div>

        {err && <p className="text-sm text-rose-400 font-medium">{err}</p>}

        <div className="flex justify-end gap-2.5 pt-4 border-t border-white/[0.06]">
          <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" disabled={busy || !title.trim() || !amount}>
            {busy ? "Saving..." : "Log Transaction"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

function EditSubscriptionModal({
  sub,
  onClose,
  onSaved,
}: {
  sub: Subscription;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [name, setName] = useState(sub.name);
  const [vendor, setVendor] = useState(sub.vendor || "");
  const [cost, setCost] = useState(sub.cost.toString());
  const [currency, setCurrency] = useState(sub.currency);
  const [cycle, setCycle] = useState<"monthly" | "yearly">(sub.cycle as "monthly" | "yearly");
  const [nextRenewal, setNextRenewal] = useState(
    sub.nextRenewal ? sub.nextRenewal.slice(0, 10) : ""
  );
  const [note, setNote] = useState(sub.note || "");
  const [enabled, setEnabled] = useState(sub.enabled);
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
      await api.updateSubscription(sub.id, payload);
      onSaved();
    } catch (e: any) {
      setErr(e.message || "Failed to save subscription");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal title="Edit Subscription" onClose={onClose}>
      <form onSubmit={submit} className="space-y-4">
        <Field label="Subscription Name">
          <input className="input w-full" value={name} onChange={(e) => setName(e.target.value)} required placeholder="e.g. GitHub Copilot" />
        </Field>
        
        <Field label="Category / Vendor">
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
            {busy ? "Saving..." : "Save Changes"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
