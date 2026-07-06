import { useEffect, useState } from "react";
import { Empty, Field, Modal, ScreenWrap, PageHeader, GlassCard, Badge, Button, StatCard, LockedFeature } from "../../ui";
import { ShieldAlert, CreditCard, Calendar, TrendingUp, Trash2, Pencil, Landmark, Plus, ArrowUpRight, ArrowDownRight, Wallet, RefreshCw, Check } from "lucide-react";
import { api, Transaction } from "../../api";
import { useTranslation } from "../../i18n";
import { CURRENCIES, fmtCost } from "./shared";

export function AddTransactionModal({
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
  const { t } = useTranslation();
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
    <Modal title={t("finance.addModalTitle")} onClose={onClose}>
      <form onSubmit={submit} className="space-y-4">
        <Field label={t("finance.fieldTransactionTitle")}>
          <input className="input w-full font-sans" value={title} onChange={(e) => setTitle(e.target.value)} required placeholder={t("finance.titlePlaceholder")} autoFocus />
        </Field>

        <div className="flex gap-4">
          <div className="flex-1">
            <Field label={t("finance.fieldFlowType")}>
              <select className="input w-full text-sm" value={type} onChange={(e) => setType(e.target.value as "income" | "expense")}>
                <option value="expense">{t("finance.optExpenditure")}</option>
                <option value="income">{t("finance.optRevenue")}</option>
              </select>
            </Field>
          </div>
          <div className="flex-1">
            <Field label={t("finance.fieldCategoryVendor")}>
              <select className="input w-full text-sm" value={category} onChange={(e) => setCategory(e.target.value)}>
                {categories.map((c) => <option key={c}>{c}</option>)}
              </select>
            </Field>
          </div>
        </div>

        <div className="flex gap-4">
          <div className="flex-1">
            <Field label={t("finance.fieldAmount")}>
              <input className="input w-full font-mono text-sm" type="number" min="0" step="0.01" value={amount} onChange={(e) => setAmount(e.target.value)} required placeholder="0.00" />
            </Field>
          </div>
          <div className="w-28">
            <Field label={t("finance.fieldCurrency")}>
              <select className="input w-full text-sm" value={currency} onChange={(e) => setCurrency(e.target.value)}>
                {CURRENCIES.map((c) => <option key={c}>{c}</option>)}
              </select>
            </Field>
          </div>
        </div>

        <div className="border-t border-white/[0.05] pt-4 space-y-4">
          <Field label={t("finance.fieldPaymentCycle")}>
            <div className="flex gap-4 mt-1">
              {([
                { value: "one-off", label: t("finance.cycleOptOneOff") },
                { value: "monthly", label: t("finance.cycleOptMonthly") },
                { value: "yearly", label: t("finance.cycleOptYearly") },
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

          <Field label={cycle === "one-off" ? t("finance.fieldTransactionDate") : t("finance.fieldBillingStartDate")}>
            <input className="input w-full text-sm font-sans" type="date" value={date} onChange={(e) => setDate(e.target.value)} required />
          </Field>
        </div>

        <div className="flex justify-end gap-2.5 pt-4 border-t border-white/[0.06]">
          <Button type="button" variant="ghost" onClick={onClose}>{t("finance.cancel")}</Button>
          <Button type="submit" variant="primary" disabled={!title.trim() || !amount}>
            {t("finance.saveRecord")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}


export function EditTransactionModal({
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
  const { t } = useTranslation();
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
    <Modal title={tx.parentId ? t("finance.editModalTitleAdjust") : t("finance.editModalTitleEdit")} onClose={onClose}>
      <form onSubmit={submit} className="space-y-4">
        {tx.parentId && (
          <div className="bg-indigo-500/10 border border-indigo-500/20 rounded-xl p-3 text-xs text-indigo-200">
            {t("finance.recurringInfo")}
          </div>
        )}

        <Field label={t("finance.fieldTitle")}>
          <input className="input w-full font-sans" value={title} onChange={(e) => setTitle(e.target.value)} required />
        </Field>

        <div className="flex gap-4">
          <div className="flex-1">
            <Field label={t("finance.fieldCategory")}>
              <select className="input w-full text-sm" value={category} onChange={(e) => setCategory(e.target.value)}>
                {categories.map((c) => <option key={c}>{c}</option>)}
              </select>
            </Field>
          </div>
          <div className="w-28">
            <Field label={t("finance.fieldCurrency")}>
              <select className="input w-full text-sm" value={currency} onChange={(e) => setCurrency(e.target.value)}>
                {CURRENCIES.map((c) => <option key={c}>{c}</option>)}
              </select>
            </Field>
          </div>
        </div>

        <div className="flex gap-4">
          <div className="flex-1">
            <Field label={t("finance.fieldAmount")}>
              <input className="input w-full font-mono text-sm" type="number" min="0" step="0.01" value={amount} onChange={(e) => setAmount(e.target.value)} required />
            </Field>
          </div>
          <div className="flex-1">
            <Field label={t("finance.fieldOccurrenceDate")}>
              <input className="input w-full text-sm font-sans" type="date" value={date} onChange={(e) => setDate(e.target.value)} required />
            </Field>
          </div>
        </div>

        {tx.parentId && (
          <Field label={t("finance.fieldUpdateScope")}>
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
                <span className="text-xs text-white/80 font-medium">{t("finance.scopeThisOnly")}</span>
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
                <span className="text-xs text-white/80 font-medium">{t("finance.scopeFuture")}</span>
              </label>
            </div>
          </Field>
        )}

        <div className="flex justify-end gap-2.5 pt-4 border-t border-white/[0.06]">
          <Button type="button" variant="ghost" onClick={onClose}>{t("finance.cancel")}</Button>
          <Button type="submit" variant="primary" disabled={!title.trim() || !amount}>
            {t("finance.saveChanges")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
