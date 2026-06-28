import { useEffect, useState } from "react";
import { api, FinanceSummary, Subscription } from "../api";
import { Empty, Field, Modal, Toggle, ScreenWrap, PageHeader, GlassCard, Badge, Button, StatCard } from "../ui";
import { ShieldAlert, CreditCard, Calendar, TrendingUp, Sparkles, Trash2, Pencil, Landmark, Plus, ArrowUpRight, ArrowDownRight, Wallet } from "lucide-react";

const CURRENCIES = ["USD", "CNY", "EUR", "GBP", "JPY", "HKD", "SGD"];

interface Transaction {
  id: string;
  date: string;
  type: "income" | "expense";
  title: string;
  category: string;
  amount: number;
  currency: string;
  isSubscription?: boolean;
}

const DEFAULT_TRANSACTIONS: Transaction[] = [
  { id: "tx-1", date: "2026-06-25", type: "income", title: "Domain Sale: webdev.io", category: "Domain Trading", amount: 1850.00, currency: "USD" },
  { id: "tx-2", date: "2026-06-22", type: "expense", title: "Hetzner Cloud VPS rental", category: "Infrastructure", amount: 48.50, currency: "USD" },
  { id: "tx-3", date: "2026-06-18", type: "expense", title: "AWS Route53 renew: mycorp.com", category: "Domain Registration", amount: 12.00, currency: "USD" },
  { id: "tx-4", date: "2026-06-15", type: "income", title: "Consulting: DNS Cluster Setup", category: "Services", amount: 650.00, currency: "USD" },
  { id: "tx-5", date: "2026-06-10", type: "expense", title: "Google Workspace renewal", category: "SaaS Tools", amount: 36.00, currency: "USD" },
];

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
  const [activeTab, setActiveTab] = useState<"subscriptions" | "ledger">("subscriptions");
  const [subs, setSubs] = useState<Subscription[]>([]);
  const [summary, setSummary] = useState<FinanceSummary | null>(null);
  const [editItem, setEditItem] = useState<Subscription | null>(null);
  const [showAddSub, setShowAddSub] = useState(false);
  const [error, setError] = useState<{ status: number; message: string } | null>(null);

  // Transactions ledger states (Closed-loop flow)
  const [transactions, setTransactions] = useState<Transaction[]>([]);
  const [showAddTx, setShowAddTx] = useState(false);

  function load() {
    api.subscriptions()
      .then(setSubs)
      .catch((err) => setError({ status: err.status, message: err.message }));
    api.financeSummary()
      .then(setSummary)
      .catch(() => {});

    // Load ledger from localStorage
    const saved = localStorage.getItem("led_finance_ledger");
    if (saved) {
      try {
        setTransactions(JSON.parse(saved));
      } catch {
        setTransactions(DEFAULT_TRANSACTIONS);
      }
    } else {
      setTransactions(DEFAULT_TRANSACTIONS);
      localStorage.setItem("led_finance_ledger", JSON.stringify(DEFAULT_TRANSACTIONS));
    }
  }

  useEffect(() => { load(); }, []);

  // Save transactions to localStorage
  const saveTransactionsList = (list: Transaction[]) => {
    setTransactions(list);
    localStorage.setItem("led_finance_ledger", JSON.stringify(list));
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

  async function toggleEnabled(sub: Subscription) {
    await api.updateSubscription(sub.id, { enabled: !sub.enabled });
    load();
  }

  async function deleteSub(sub: Subscription) {
    if (!confirm(`Remove "${sub.name}"?`)) return;
    await api.deleteSubscription(sub.id);
    load();
  }

  // Add ledger transaction
  function handleAddTransaction(tx: Omit<Transaction, "id">) {
    const newTx: Transaction = {
      ...tx,
      id: "tx-" + Date.now(),
    };
    const updated = [newTx, ...transactions];
    saveTransactionsList(updated);
  }

  // Delete ledger transaction
  function handleDeleteTransaction(id: string) {
    if (!confirm("Are you sure you want to delete this ledger entry?")) return;
    const updated = transactions.filter(t => t.id !== id);
    saveTransactionsList(updated);
  }

  const currencies = summary ? Object.keys(summary.monthlyByCurrency) : [];

  // Calculate dynamic transaction entries from active subscriptions
  const virtualSubs: Transaction[] = subs.filter(s => s.enabled).map(s => {
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
      title: `Subscription: ${s.name} (${s.cycle === "monthly" ? "Monthly" : "Yearly"})`,
      category: "SaaS Tools",
      amount: s.cost,
      currency: s.currency,
      isSubscription: true,
    };
  });

  const combinedTransactions = [...virtualSubs, ...transactions];
  combinedTransactions.sort((a, b) => b.date.localeCompare(a.date));

  // Calculate ledger stats (Closed-loop ledger calculations)
  const totalIncome = combinedTransactions.filter(t => t.type === "income").reduce((acc, t) => acc + t.amount, 0);
  const totalExpense = combinedTransactions.filter(t => t.type === "expense").reduce((acc, t) => acc + t.amount, 0);
  const netBalance = totalIncome - totalExpense;

  return (
    <ScreenWrap>
      <PageHeader
        title="Finance Workspace"
        description="Monitor recurring cloud subscriptions and audit organization cash ledger records"
        action={
          activeTab === "subscriptions" ? (
            <Button variant="primary" onClick={() => setShowAddSub(true)} className="gap-1">
              <Plus className="h-4 w-4" /> Add Subscription
            </Button>
          ) : (
            <Button variant="primary" onClick={() => setShowAddTx(true)} className="gap-1">
              <Plus className="h-4 w-4" /> Add Transaction
            </Button>
          )
        }
      />

      {/* Tabs Switcher */}
      <div className="flex gap-1.5 p-1 rounded-xl bg-black/25 border border-white/[0.05] max-w-sm mb-6">
        <button
          onClick={() => setActiveTab("subscriptions")}
          className={`flex-1 rounded-lg py-2 text-xs font-semibold transition-all ${
            activeTab === "subscriptions" ? "bg-white/[0.08] text-white shadow-glow" : "text-white/50 hover:text-white/80"
          }`}
        >
          SaaS Subscriptions
        </button>
        <button
          onClick={() => setActiveTab("ledger")}
          className={`flex-1 rounded-lg py-2 text-xs font-semibold transition-all ${
            activeTab === "ledger" ? "bg-white/[0.08] text-white shadow-glow" : "text-white/50 hover:text-white/80"
          }`}
        >
          Income & Expense Ledger
        </button>
      </div>

      {activeTab === "subscriptions" ? (
        <>
          {/* Subscriptions Summary cards */}
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

          {/* Subscriptions list */}
          {subs.length === 0 ? (
            <Empty>
              <CreditCard className="h-10 w-10 text-white/30 mb-2" />
              <p className="text-sm text-white/50">No active subscriptions configured.</p>
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
        </>
      ) : (
        <>
          {/* Cash Ledger Summary Cards (Closed-Loop stats) */}
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
            <StatCard
              label="Total Revenue Income"
              value={fmtCost(totalIncome, "USD")}
              delta="Total received cash flow"
              icon={<ArrowUpRight className="h-4 w-4 text-emerald-400" />}
              positive={true}
              index={0}
            />
            <StatCard
              label="Total Expenditures"
              value={fmtCost(totalExpense, "USD")}
              delta="Total operating costs"
              icon={<ArrowDownRight className="h-4 w-4 text-rose-400" />}
              positive={false}
              index={1}
            />
            <StatCard
              label="Net Cash Profit"
              value={fmtCost(netBalance, "USD")}
              delta={netBalance >= 0 ? "Workspace Surplus" : "Workspace Deficit"}
              icon={<Wallet className="h-4 w-4" />}
              positive={netBalance >= 0}
              index={2}
            />
          </div>

          {/* Ledger Table */}
          {transactions.length === 0 ? (
            <Empty>
              <Landmark className="h-10 w-10 text-white/30 mb-2" />
              <p className="text-sm text-white/50">No transaction logs in cash ledger.</p>
            </Empty>
          ) : (
            <GlassCard className="overflow-hidden">
              <div className="overflow-x-auto">
                <table className="w-full text-sm border-collapse">
                  <thead className="border-b border-white/[0.06] bg-white/[0.02] text-white/55">
                    <tr className="text-left text-xs font-semibold uppercase tracking-wider">
                      <th className="px-5 py-3.5">Date</th>
                      <th className="px-5 py-3.5">Flow Type</th>
                      <th className="px-5 py-3.5">Transaction Title</th>
                      <th className="px-5 py-3.5">Category</th>
                      <th className="px-5 py-3.5 text-right">Amount</th>
                      <th className="px-5 py-3.5"></th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-white/[0.04]">
                    {combinedTransactions.map((tx) => (
                      <tr key={tx.id} className={`hover:bg-white/[0.02] transition-all ${tx.isSubscription ? "bg-white/[0.01]" : ""}`}>
                        <td className="px-5 py-4 font-mono text-xs text-white/60">{tx.date}</td>
                        <td className="px-5 py-4">
                          <Badge tone={tx.type === "income" ? "green" : "red"} className="uppercase font-bold tracking-wider text-[9px]">
                            {tx.type}
                          </Badge>
                        </td>
                        <td className="px-5 py-4 text-white font-medium">
                          <div className="flex items-center gap-2">
                            <span>{tx.title}</span>
                            {tx.isSubscription && (
                              <Badge tone="indigo" className="text-[9px] font-semibold uppercase tracking-wider px-1.5 py-0">Recurring</Badge>
                            )}
                          </div>
                        </td>
                        <td className="px-5 py-4">
                          <Badge tone="neutral" className="text-white/60 bg-white/5 border border-white/[0.05]">
                            {tx.category}
                          </Badge>
                        </td>
                        <td className={`px-5 py-4 text-right font-mono font-semibold ${tx.type === "income" ? "text-emerald-400" : "text-rose-400"}`}>
                          {tx.type === "income" ? "+" : "-"} {fmtCost(tx.amount, tx.currency)}
                        </td>
                        <td className="px-5 py-4">
                          <div className="flex gap-2 justify-end">
                            {tx.isSubscription ? (
                              <Button
                                variant="ghost"
                                onClick={() => setActiveTab("subscriptions")}
                                className="text-xs py-1 px-2.5 text-indigo-400 hover:text-indigo-300 font-semibold"
                              >
                                Manage
                              </Button>
                            ) : (
                              <Button
                                variant="danger"
                                onClick={() => handleDeleteTransaction(tx.id)}
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
        </>
      )}

      {/* Subscription Edit/Add Modal */}
      {(showAddSub || editItem) && (
        <SubModal
          sub={editItem}
          onClose={() => { setShowAddSub(false); setEditItem(null); }}
          onSaved={load}
        />
      )}

      {/* Transaction Add Modal */}
      {showAddTx && (
        <TransactionModal
          onClose={() => setShowAddTx(false)}
          onAdd={handleAddTransaction}
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

function TransactionModal({
  onClose,
  onAdd,
}: {
  onClose: () => void;
  onAdd: (tx: Omit<Transaction, "id">) => void;
}) {
  const [title, setTitle] = useState("");
  const [category, setCategory] = useState("Services");
  const [amount, setAmount] = useState("");
  const [type, setType] = useState<"income" | "expense">("expense");
  const [date, setDate] = useState(new Date().toISOString().slice(0, 10));

  const categories = [
    "Domain Trading",
    "Domain Registration",
    "Infrastructure",
    "SaaS Tools",
    "Services",
    "Advertising",
    "Salary",
    "Other",
  ];

  function submit(e: React.FormEvent) {
    e.preventDefault();
    onAdd({
      date,
      type,
      title,
      category,
      amount: parseFloat(amount) || 0,
      currency: "USD",
    });
    onClose();
  }

  return (
    <Modal title="Log Financial Transaction" onClose={onClose}>
      <form onSubmit={submit} className="space-y-4">
        <Field label="Transaction Title">
          <input className="input w-full" value={title} onChange={(e) => setTitle(e.target.value)} required placeholder="e.g. Client Server Setup Invoice" autoFocus />
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
            <Field label="Category">
              <select className="input w-full text-sm" value={category} onChange={(e) => setCategory(e.target.value)}>
                {categories.map((c) => <option key={c}>{c}</option>)}
              </select>
            </Field>
          </div>
        </div>

        <div className="flex gap-4">
          <div className="flex-1">
            <Field label="Amount ($)">
              <input className="input w-full font-mono text-sm" type="number" min="0" step="0.01" value={amount} onChange={(e) => setAmount(e.target.value)} required placeholder="0.00" />
            </Field>
          </div>
          <div className="flex-1">
            <Field label="Transaction Date">
              <input className="input w-full text-sm" type="date" value={date} onChange={(e) => setDate(e.target.value)} required />
            </Field>
          </div>
        </div>

        <div className="flex justify-end gap-2.5 pt-4 border-t border-white/[0.06]">
          <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" disabled={!title.trim() || !amount}>
            Save Ledger Entry
          </Button>
        </div>
      </form>
    </Modal>
  );
}
