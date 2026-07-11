import { useEffect, useState } from "react";
import { Empty, Field, Modal, ScreenWrap, PageHeader, GlassCard, Badge, Button, StatCard, LockedFeature } from "@octarq-org/plugin-sdk";
import { ShieldAlert, CreditCard, Calendar, TrendingUp, Trash2, Pencil, Landmark, Plus, ArrowUpRight, ArrowDownRight, Wallet, RefreshCw, Check } from "lucide-react";
import { api, Transaction } from "../../api";
import { useTranslation } from "@octarq-org/plugin-sdk";

import { CURRENCIES, fmtCost } from "./shared";
import { AddTransactionModal, EditTransactionModal } from "./modals";

export default function FinancePage() {
  const { t } = useTranslation();
  const [filterType, setFilterType] = useState<"all" | "recurring" | "one-off">("all");
  const [flowFilter, setFlowFilter] = useState<"all" | "income" | "expense">("all");
  const [transactions, setTransactions] = useState<Transaction[]>([]);
  
  // Modals state
  const [showAddModal, setShowAddModal] = useState(false);
  const [editingTx, setEditingTx] = useState<Transaction | null>(null);
  // Pro-gate: 402 (unlicensed) → upsell, 404 (plugin absent in OSS build) → neutral note.
  const [error, setError] = useState<{ status: number } | null>(null);

  function load() {
    api.transactions().then(setTransactions).catch(err => {
      setError({ status: err.status });
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
      if (!confirm(t("finance.confirmDeleteTx"))) return;
      api.deleteTransaction(targetTx.id).then(() => load());
    } else {
      const choice = confirm(t("finance.confirmDeleteOccurrence"));
      if (choice) {
        api.deleteTransaction(targetTx.id).then(() => load());
      }
    }
  }

  // Delete entire recurring series
  function handleDeleteSeries(parentId: string) {
    if (!confirm(t("finance.confirmDeleteSeries"))) return;
    api.deleteTransactionSeries(parentId).then(() => load());
  }

  // Confirm a pending (e.g. AI OCR-extracted) transaction into the ledger.
  function handleConfirmTransaction(targetTx: Transaction) {
    api.confirmTransaction(targetTx.id).then(() => load());
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
          feature={t("finance.lockedFeature")}
          description={t("finance.lockedDesc")}
          perks={[
            t("finance.perk1"),
            t("finance.perk2"),
            t("finance.perk3"),
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
        title={t("finance.pageTitle")}
        description={t("finance.pageDesc")}
        action={
          <Button variant="primary" onClick={() => setShowAddModal(true)} className="gap-1">
            <Plus className="h-4 w-4" /> {t("finance.addTransaction")}
          </Button>
        }
      />

      {/* Cash Ledger Stats Card Group */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
        <StatCard
          label={t("finance.statRevenueLabel")}
          value={fmtCost(totalIncome, "USD")}
          delta={t("finance.statRevenueDelta")}
          icon={<ArrowUpRight className="h-4 w-4 text-emerald-400" />}
          positive={true}
          index={0}
        />
        <StatCard
          label={t("finance.statExpenseLabel")}
          value={fmtCost(totalExpense, "USD")}
          delta={t("finance.statExpenseDelta")}
          icon={<ArrowDownRight className="h-4 w-4 text-rose-400" />}
          positive={false}
          index={1}
        />
        <StatCard
          label={t("finance.statNetLabel")}
          value={fmtCost(netBalance, "USD")}
          delta={netBalance >= 0 ? t("finance.surplus") : t("finance.deficit")}
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
            {t("finance.filterAllLogs", { count: transactions.length })}
          </button>
          <button
            onClick={() => setFilterType("recurring")}
            className={`rounded-lg px-4 py-1.5 text-xs font-semibold transition-all flex items-center gap-1 ${
              filterType === "recurring" ? "bg-white/[0.08] text-white shadow-glow" : "text-white/50 hover:text-white/80"
            }`}
          >
            <RefreshCw className="h-3 w-3" /> {t("finance.filterRecurring", { count: transactions.filter(tx => tx.cycle !== "one-off").length })}
          </button>
          <button
            onClick={() => setFilterType("one-off")}
            className={`rounded-lg px-4 py-1.5 text-xs font-semibold transition-all ${
              filterType === "one-off" ? "bg-white/[0.08] text-white shadow-glow" : "text-white/50 hover:text-white/80"
            }`}
          >
            {t("finance.filterOneOff", { count: transactions.filter(tx => tx.cycle === "one-off").length })}
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
            {t("finance.filterAllFlows")}
          </button>
          <button
            onClick={() => setFlowFilter("income")}
            className={`rounded-lg px-4 py-1.5 text-xs font-semibold transition-all ${
              flowFilter === "income" ? "bg-emerald-500/15 text-emerald-400 font-bold border border-emerald-500/10" : "text-white/50 hover:text-white/80"
            }`}
          >
            {t("finance.filterIncomeOnly", { count: transactions.filter(tx => tx.type === "income").length })}
          </button>
          <button
            onClick={() => setFlowFilter("expense")}
            className={`rounded-lg px-4 py-1.5 text-xs font-semibold transition-all ${
              flowFilter === "expense" ? "bg-rose-500/15 text-rose-400 font-bold border border-rose-500/10" : "text-white/50 hover:text-white/80"
            }`}
          >
            {t("finance.filterExpensesOnly", { count: transactions.filter(tx => tx.type === "expense").length })}
          </button>
        </div>
      </div>

      {/* Unified Table */}
      {filteredTransactions.length === 0 ? (
        <Empty>
          <Landmark className="h-10 w-10 text-white/50 mb-2" />
          <p className="text-sm text-white/50">{t("finance.emptyState")}</p>
        </Empty>
      ) : (
        <GlassCard className="overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-sm border-collapse">
              <thead className="border-b border-white/[0.06] bg-white/[0.02] text-white/55">
                <tr className="text-left text-xs font-semibold uppercase tracking-wider">
                  <th className="px-5 py-3.5">{t("finance.thDate")}</th>
                  <th className="px-5 py-3.5">{t("finance.thFlow")}</th>
                  <th className="px-5 py-3.5">{t("finance.thTitle")}</th>
                  <th className="px-5 py-3.5">{t("finance.thCategory")}</th>
                  <th className="px-5 py-3.5">{t("finance.thCycle")}</th>
                  <th className="px-5 py-3.5 text-right">{t("finance.thAmount")}</th>
                  <th className="px-5 py-3.5"></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/[0.04]">
                {filteredTransactions.map((tx) => (
                  <tr key={tx.id} className={`hover:bg-white/[0.02] transition-all ${tx.status === "pending" ? "bg-amber-500/[0.06]" : tx.parentId ? "bg-white/[0.01]" : ""}`}>
                    <td className="px-5 py-4 font-mono text-xs text-white/60">{tx.date}</td>
                    <td className="px-5 py-4">
                      <Badge tone={tx.type === "income" ? "green" : "red"} className="uppercase font-bold tracking-wider text-[9px]">
                        {tx.type === "income" ? t("finance.flowIncome") : t("finance.flowExpense")}
                      </Badge>
                    </td>
                    <td className="px-5 py-4 text-white font-medium">
                      <div className="flex items-center gap-2">
                        <span>{tx.title}</span>
                        {tx.parentId && (
                          <Badge tone="indigo" className="text-[9px] font-semibold uppercase tracking-wider px-1.5 py-0">{t("finance.badgeRecurringSeries")}</Badge>
                        )}
                        {tx.status === "pending" && (
                          <Badge tone="amber" className="text-[9px] font-semibold uppercase tracking-wider px-1.5 py-0">{t("finance.badgePendingReview")}</Badge>
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
                        <span className="text-white/40">{t("finance.cycleOneOff")}</span>
                      ) : (
                        <span className="text-indigo-400 font-semibold flex items-center gap-1">
                          <RefreshCw className="h-3 w-3" /> {tx.cycle === "monthly" ? t("finance.cycleMonthly") : t("finance.cycleYearly")}
                        </span>
                      )}
                    </td>
                    <td className={`px-5 py-4 text-right font-mono font-semibold ${tx.type === "income" ? "text-emerald-400" : "text-rose-400"}`}>
                      {tx.type === "income" ? "+" : "-"} {fmtCost(tx.amount, tx.currency)}
                    </td>
                    <td className="px-5 py-4">
                      <div className="flex gap-2 justify-end">
                        {tx.status === "pending" && (
                          <Button
                            variant="ghost"
                            onClick={() => handleConfirmTransaction(tx)}
                            className="text-xs py-1 px-2.5 text-emerald-300 hover:bg-emerald-500/10"
                            title={t("finance.confirmTitle")}
                          >
                            <Check className="h-3.5 w-3.5 mr-1" /> {t("finance.confirmBtn")}
                          </Button>
                        )}
                        <Button
                          variant="ghost"
                          onClick={() => setEditingTx(tx)}
                          className="text-xs py-1 px-2.5"
                        >
                          <Pencil className="h-3.5 w-3.5 mr-1" /> {t("finance.editBtn")}
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
                            title={t("finance.deleteContractTitle")}
                          >
                            {t("finance.deleteContract")}
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

