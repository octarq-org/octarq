import { useEffect, useState } from "react";
import { Empty, Field, Modal, ScreenWrap, PageHeader, GlassCard, Badge, Button, StatCard, LockedFeature } from "../ui";
import { ShieldAlert, CreditCard, Calendar, TrendingUp, Trash2, Pencil, Landmark, Plus, ArrowUpRight, ArrowDownRight, Wallet, RefreshCw } from "lucide-react";
import { api, Transaction } from "../api";

const CURRENCIES = ["USD", "CNY", "EUR", "GBP", "JPY", "HKD", "SGD"];

// Generate pre-populated historical occurrences for default recurring series
const generateDefaultSeries = (
  parentId: string,
  title: string,
  type: "income" | "expense",
  category: string,
  amount: number,
  currency: string,
  cycle: "monthly" | "yearly",
  startMonth: number // 0-indexed
): Omit<Transaction, "id">[] => {
  const list: Omit<Transaction, "id">[] = [];
  const today = new Date();
  
  // Generate occurrences from startMonth of 2026 up to today
  for (let m = startMonth; m <= today.getMonth(); m++) {
    const dayStr = String(15).padStart(2, "0");
    const monthStr = String(m + 1).padStart(2, "0");
    list.push({
      parentId,
      date: `2026-${monthStr}-${dayStr}`,
      type,
      title,
      category,
      amount,
      currency,
      cycle,
    });
  }
  return list;
};

const getDefaultTransactions = (): Omit<Transaction, "id">[] => {
  const list: Omit<Transaction, "id">[] = [
    { date: "2026-06-25", type: "income", title: "Domain Sale: webdev.io", category: "Domain Trading", amount: 1850.00, currency: "USD", cycle: "one-off" },
    { date: "2026-06-22", type: "expense", title: "Hetzner Cloud VPS rental", category: "Infrastructure", amount: 48.50, currency: "USD", cycle: "one-off" },
    { date: "2026-06-18", type: "expense", title: "AWS Route53 renew: mycorp.com", category: "Domain Registration", amount: 12.00, currency: "USD", cycle: "one-off" },
  ];

  // Recurring expense series: GitHub Copilot (Monthly, from February 2026)
  list.push(...generateDefaultSeries("series-github", "GitHub Copilot Subscription", "expense", "SaaS Tools", 10.00, "USD", "monthly", 1));

  // Recurring income series: VPS Tenant leasing (Monthly, from March 2026)
  list.push(...generateDefaultSeries("series-vps-rent", "VPS Leasing: Client A", "income", "Services", 120.00, "USD", "monthly", 2));

  return list;
};

function fmtCost(cost: number, currency: string) {
  return new Intl.NumberFormat("en-US", { style: "currency", currency, minimumFractionDigits: 2 }).format(cost);
}

export default function FinancePage() {
  const [filterType, setFilterType] = useState<"all" | "recurring" | "one-off">("all");
  const [flowFilter, setFlowFilter] = useState<"all" | "income" | "expense">("all");
  const [transactions, setTransactions] = useState<Transaction[]>([]);
  
  // Modals state
  const [showAddModal, setShowAddModal] = useState(false);
  const [editingTx, setEditingTx] = useState<Transaction | null>(null);
  // Pro-gate: 402 (unlicensed) → upsell, 404 (plugin absent in OSS build) → neutral note.
  const [error, setError] = useState<{ status: number; message: string } | null>(null);

  function load() {
    api.transactions().then((res) => {
      if (res.length > 0) {
        setTransactions(res);
      } else {
        // Seed mock data if database is empty
        const defaults = getDefaultTransactions();
        Promise.all(defaults.map(tx => {
          return api.createTransaction(tx);
        })).then(() => {
          api.transactions().then(setTransactions);
        });
      }
    }).catch(err => {
      setError({ status: err.status, message: err.message });
    });
  }

  useEffect(() => { load(); }, []);

  // Add new transaction (or recurring series)
  function handleAddTransaction(payload: {
    title: string;
    type: "income" | "expense";
    category: string;
    amount: number;
    currency: string;
    cycle: "one-off" | "monthly" | "yearly";
    date: string;
  }) {
    if (payload.cycle === "one-off") {
      api.createTransaction({
        date: payload.date,
        type: payload.type,
        title: payload.title,
        category: payload.category,
        amount: payload.amount,
        currency: payload.currency,
        cycle: "one-off",
      }).then(() => load());
    } else {
      // Generate recurring occurrences from the chosen start date up to today
      const parentId = "series-" + Date.now();
      const occurrences: any[] = [];
      const startDate = new Date(payload.date);
      const today = new Date();
      
      let cursor = new Date(startDate);
      while (cursor <= today || occurrences.length === 0) {
        const dateStr = cursor.toISOString().slice(0, 10);
        occurrences.push({
          parentId,
          date: dateStr,
          type: payload.type,
          title: payload.title,
          category: payload.category,
          amount: payload.amount,
          currency: payload.currency,
          cycle: payload.cycle,
        });

        if (payload.cycle === "monthly") {
          cursor.setMonth(cursor.getMonth() + 1);
        } else {
          cursor.setFullYear(cursor.getFullYear() + 1);
        }
      }
      Promise.all(occurrences.map(tx => api.createTransaction(tx))).then(() => load());
    }
  }

  // Edit/Adjust a transaction
  function handleSaveEdit(
    targetTx: Transaction,
    fields: { title: string; category: string; amount: number; currency: string; date: string },
    scope: "one" | "all"
  ) {
    if (scope === "one" || !targetTx.parentId) {
      api.updateTransaction(targetTx.id, fields).then(() => load());
    } else {
      // Update all future occurrences in this recurring series
      const toUpdate = transactions.filter(t => t.parentId === targetTx.parentId && t.date >= targetTx.date);
      Promise.all(toUpdate.map(t => api.updateTransaction(t.id, {
        ...fields,
        date: t.date, // keep original date for each occurrence
      }))).then(() => load());
    }
  }

  // Delete transaction or series
  function handleDeleteTransaction(targetTx: Transaction) {
    if (!targetTx.parentId) {
      if (!confirm("Are you sure you want to delete this transaction record?")) return;
      api.deleteTransaction(targetTx.id).then(() => load());
    } else {
      const choice = confirm(
        "This transaction belongs to a recurring series.\n\n" +
        "Click OK to delete THIS SPECIFIC OCCURRENCE only.\n" +
        "Click Cancel to keep it."
      );
      if (choice) {
        api.deleteTransaction(targetTx.id).then(() => load());
      }
    }
  }

  // Delete entire recurring series
  function handleDeleteSeries(parentId: string) {
    if (!confirm("Are you sure you want to delete the ENTIRE recurring series? This will erase all history for this contract.")) return;
    api.deleteTransactionSeries(parentId).then(() => load());
  }

  // Filter list
  const filteredTransactions = transactions.filter((tx) => {
    // 1. Cycle filter
    if (filterType === "recurring" && tx.cycle === "one-off") return false;
    if (filterType === "one-off" && tx.cycle !== "one-off") return false;
    // 2. Flow filter
    if (flowFilter === "income" && tx.type !== "income") return false;
    if (flowFilter === "expense" && tx.type !== "expense") return false;
    return true;
  });

  // Dynamic calculations of ledger indicators
  const totalIncome = transactions.filter(t => t.type === "income").reduce((acc, t) => acc + t.amount, 0);
  const totalExpense = transactions.filter(t => t.type === "expense").reduce((acc, t) => acc + t.amount, 0);
  const netBalance = totalIncome - totalExpense;

  if (error) {
    return (
      <ScreenWrap>
        <LockedFeature
          status={error.status}
          tier="pro"
          feature="FinOps Spend Optimization"
          description="Gain deep visibility into your operational cash flow and subscription expenditures with proactive lifecycle alerts."
          perks={[
            "Consolidated subscription tracking with annualized run-rate analysis",
            "Automated renewal alerts dispatched to your preferred notification channels",
            "Unified income and expense ledger for cash flow optimization",
          ]}
          icon={<Landmark className="h-7 w-7" />}
          pricingHref="https://octarq.com/pricing/"
        />
      </ScreenWrap>
    );
  }

  return (
    <ScreenWrap>
      <PageHeader
        title="Bookkeeping"
        description="Consolidated subscription expense ledger & cash flow tracking"
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
        {/* Cycle Filter */}
        <div className="flex gap-1.5 p-1 rounded-xl bg-black/25 border border-white/[0.05] shrink-0">
          <button
            onClick={() => setFilterType("all")}
            className={`rounded-lg px-4 py-1.5 text-xs font-semibold transition-all ${
              filterType === "all" ? "bg-white/[0.08] text-white shadow-glow" : "text-white/50 hover:text-white/80"
            }`}
          >
            All Logs ({transactions.length})
          </button>
          <button
            onClick={() => setFilterType("recurring")}
            className={`rounded-lg px-4 py-1.5 text-xs font-semibold transition-all flex items-center gap-1 ${
              filterType === "recurring" ? "bg-white/[0.08] text-white shadow-glow" : "text-white/50 hover:text-white/80"
            }`}
          >
            <RefreshCw className="h-3 w-3" /> Recurring ({transactions.filter(t => t.cycle !== "one-off").length})
          </button>
          <button
            onClick={() => setFilterType("one-off")}
            className={`rounded-lg px-4 py-1.5 text-xs font-semibold transition-all ${
              filterType === "one-off" ? "bg-white/[0.08] text-white shadow-glow" : "text-white/50 hover:text-white/80"
            }`}
          >
            One-off Ledger ({transactions.filter(t => t.cycle === "one-off").length})
          </button>
        </div>

        {/* Flow Filter */}
        <div className="flex gap-1.5 p-1 rounded-xl bg-black/25 border border-white/[0.05] shrink-0">
          <button
            onClick={() => setFlowFilter("all")}
            className={`rounded-lg px-4 py-1.5 text-xs font-semibold transition-all ${
              flowFilter === "all" ? "bg-white/[0.08] text-white shadow-glow" : "text-white/50 hover:text-white/80"
            }`}
          >
            All Flows
          </button>
          <button
            onClick={() => setFlowFilter("income")}
            className={`rounded-lg px-4 py-1.5 text-xs font-semibold transition-all ${
              flowFilter === "income" ? "bg-emerald-500/15 text-emerald-400 font-bold border border-emerald-500/10" : "text-white/50 hover:text-white/80"
            }`}
          >
            Income Only ({transactions.filter(t => t.type === "income").length})
          </button>
          <button
            onClick={() => setFlowFilter("expense")}
            className={`rounded-lg px-4 py-1.5 text-xs font-semibold transition-all ${
              flowFilter === "expense" ? "bg-rose-500/15 text-rose-400 font-bold border border-rose-500/10" : "text-white/50 hover:text-white/80"
            }`}
          >
            Expenses Only ({transactions.filter(t => t.type === "expense").length})
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
                  <th className="px-5 py-3.5">Category</th>
                  <th className="px-5 py-3.5">Cycle</th>
                  <th className="px-5 py-3.5 text-right">Amount</th>
                  <th className="px-5 py-3.5"></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/[0.04]">
                {filteredTransactions.map((tx) => (
                  <tr key={tx.id} className={`hover:bg-white/[0.02] transition-all ${tx.parentId ? "bg-white/[0.01]" : ""}`}>
                    <td className="px-5 py-4 font-mono text-xs text-white/60">{tx.date}</td>
                    <td className="px-5 py-4">
                      <Badge tone={tx.type === "income" ? "green" : "red"} className="uppercase font-bold tracking-wider text-[9px]">
                        {tx.type}
                      </Badge>
                    </td>
                    <td className="px-5 py-4 text-white font-medium">
                      <div className="flex items-center gap-2">
                        <span>{tx.title}</span>
                        {tx.parentId && (
                          <Badge tone="indigo" className="text-[9px] font-semibold uppercase tracking-wider px-1.5 py-0">Recurring Series</Badge>
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
                          <RefreshCw className="h-3 w-3" /> {tx.cycle}
                        </span>
                      )}
                    </td>
                    <td className={`px-5 py-4 text-right font-mono font-semibold ${tx.type === "income" ? "text-emerald-400" : "text-rose-400"}`}>
                      {tx.type === "income" ? "+" : "-"} {fmtCost(tx.amount, tx.currency)}
                    </td>
                    <td className="px-5 py-4">
                      <div className="flex gap-2 justify-end">
                        <Button
                          variant="ghost"
                          onClick={() => setEditingTx(tx)}
                          className="text-xs py-1 px-2.5"
                        >
                          <Pencil className="h-3.5 w-3.5 mr-1" /> Edit
                        </Button>
                        <Button
                          variant="danger"
                          onClick={() => handleDeleteTransaction(tx)}
                          className="text-xs py-1 px-2.5 bg-rose-500/0 hover:bg-rose-500/10 border-0"
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </Button>
                        {tx.parentId && (
                          <Button
                            variant="danger"
                            onClick={() => handleDeleteSeries(tx.parentId!)}
                            className="text-[10px] py-1 px-2 bg-rose-950/20 hover:bg-rose-900/40 text-rose-300 border-rose-900/30"
                            title="Delete entire recurring series history"
                          >
                            Delete Contract
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

      {/* Edit / Adjust Occurrence Modal */}
      {editingTx && (
        <EditTransactionModal
          tx={editingTx}
          onClose={() => setEditingTx(null)}
          onSave={handleSaveEdit}
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
    date: string;
  }) => void;
}) {
  const [title, setTitle] = useState("");
  const [type, setType] = useState<"income" | "expense">("expense");
  const [category, setCategory] = useState("SaaS Tools");
  const [amount, setAmount] = useState("");
  const [currency, setCurrency] = useState("USD");
  const [cycle, setCycle] = useState<"one-off" | "monthly" | "yearly">("one-off");
  const [date, setDate] = useState(new Date().toISOString().slice(0, 10));

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

  function submit(e: React.FormEvent) {
    e.preventDefault();
    onAdd({
      title,
      type,
      category,
      amount: parseFloat(amount) || 0,
      currency,
      cycle,
      date,
    });
    onClose();
  }

  return (
    <Modal title="Log Financial Transaction" onClose={onClose}>
      <form onSubmit={submit} className="space-y-4">
        <Field label="Transaction Title">
          <input className="input w-full font-sans" value={title} onChange={(e) => setTitle(e.target.value)} required placeholder="e.g. Vercel Hosting / Consulting Retainer" autoFocus />
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
          <Field label="Payment Cycle / Term">
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
                    onChange={() => setCycle(item.value)}
                    className="accent-indigo-500"
                  />
                  <span className="text-xs text-white/70">{item.label}</span>
                </label>
              ))}
            </div>
          </Field>

          <Field label={cycle === "one-off" ? "Transaction Date" : "Billing Cycle Start Date"}>
            <input className="input w-full text-sm font-sans" type="date" value={date} onChange={(e) => setDate(e.target.value)} required />
          </Field>
        </div>

        <div className="flex justify-end gap-2.5 pt-4 border-t border-white/[0.06]">
          <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" disabled={!title.trim() || !amount}>
            Save Record
          </Button>
        </div>
      </form>
    </Modal>
  );
}

function EditTransactionModal({
  tx,
  onClose,
  onSave,
}: {
  tx: Transaction;
  onClose: () => void;
  onSave: (
    targetTx: Transaction,
    fields: { title: string; category: string; amount: number; currency: string; date: string },
    scope: "one" | "all"
  ) => void;
}) {
  const [title, setTitle] = useState(tx.title);
  const [category, setCategory] = useState(tx.category);
  const [amount, setAmount] = useState(tx.amount.toString());
  const [currency, setCurrency] = useState(tx.currency);
  const [date, setDate] = useState(tx.date);

  // Edit scope choice: 'one' (this occurrence) or 'all' (all future occurrences)
  const [editScope, setEditScope] = useState<"one" | "all">("one");

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

  function submit(e: React.FormEvent) {
    e.preventDefault();
    onSave(
      tx,
      {
        title,
        category,
        amount: parseFloat(amount) || 0,
        currency,
        date,
      },
      editScope
    );
    onClose();
  }

  return (
    <Modal title={tx.parentId ? "Adjust Occurrence" : "Edit Transaction"} onClose={onClose}>
      <form onSubmit={submit} className="space-y-4">
        {tx.parentId && (
          <div className="bg-indigo-500/10 border border-indigo-500/20 rounded-xl p-3 text-xs text-indigo-200">
            💡 This transaction belongs to a recurring contract. You can edit this single instance or the entire future series.
          </div>
        )}

        <Field label="Title">
          <input className="input w-full font-sans" value={title} onChange={(e) => setTitle(e.target.value)} required />
        </Field>

        <div className="flex gap-4">
          <div className="flex-1">
            <Field label="Category">
              <select className="input w-full text-sm" value={category} onChange={(e) => setCategory(e.target.value)}>
                {categories.map((c) => <option key={c}>{c}</option>)}
              </select>
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

        <div className="flex gap-4">
          <div className="flex-1">
            <Field label="Amount">
              <input className="input w-full font-mono text-sm" type="number" min="0" step="0.01" value={amount} onChange={(e) => setAmount(e.target.value)} required />
            </Field>
          </div>
          <div className="flex-1">
            <Field label="Occurrence Date">
              <input className="input w-full text-sm font-sans" type="date" value={date} onChange={(e) => setDate(e.target.value)} required />
            </Field>
          </div>
        </div>

        {tx.parentId && (
          <Field label="Update Scope">
            <div className="flex gap-4 mt-1.5">
              <label className="flex items-center gap-2 cursor-pointer select-none">
                <input
                  type="radio"
                  name="editScope"
                  value="one"
                  checked={editScope === "one"}
                  onChange={() => setEditScope("one")}
                  className="accent-indigo-500"
                />
                <span className="text-xs text-white/80 font-medium">Apply to THIS occurrence only</span>
              </label>
              <label className="flex items-center gap-2 cursor-pointer select-none">
                <input
                  type="radio"
                  name="editScope"
                  value="all"
                  checked={editScope === "all"}
                  onChange={() => setEditScope("all")}
                  className="accent-indigo-500"
                />
                <span className="text-xs text-white/80 font-medium">Apply to FUTURE occurrences in series</span>
              </label>
            </div>
          </Field>
        )}

        <div className="flex justify-end gap-2.5 pt-4 border-t border-white/[0.06]">
          <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" disabled={!title.trim() || !amount}>
            Save Changes
          </Button>
        </div>
      </form>
    </Modal>
  );
}
