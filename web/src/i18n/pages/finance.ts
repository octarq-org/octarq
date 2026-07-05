export const finance = {
  en: {
    lockedFeature: "FinOps Spend Optimization",
    lockedDesc:
      "Gain deep visibility into your operational cash flow and subscription expenditures with proactive lifecycle alerts.",
    perk1: "Consolidated subscription tracking with annualized run-rate analysis",
    perk2: "Automated renewal alerts dispatched to your preferred notification channels",
    perk3: "Unified income and expense ledger for cash flow optimization",

    pageTitle: "Bookkeeping",
    pageDesc: "Consolidated subscription expense ledger & cash flow tracking",
    addTransaction: "Add Transaction",

    statRevenueLabel: "Total Revenue Income",
    statRevenueDelta: "Total cash inflows received",
    statExpenseLabel: "Total Expenditures",
    statExpenseDelta: "Total subscription & one-off fees",
    statNetLabel: "Net Cash Profit",
    surplus: "Surplus",
    deficit: "Deficit",

    filterAllLogs: "All Logs ({{count}})",
    filterRecurring: "Recurring ({{count}})",
    filterOneOff: "One-off Ledger ({{count}})",
    filterAllFlows: "All Flows",
    filterIncomeOnly: "Income Only ({{count}})",
    filterExpensesOnly: "Expenses Only ({{count}})",

    emptyState: "No transaction logs match current filters.",

    thDate: "Date",
    thFlow: "Flow",
    thTitle: "Title",
    thCategory: "Category",
    thCycle: "Cycle",
    thAmount: "Amount",

    flowIncome: "income",
    flowExpense: "expense",

    badgeRecurringSeries: "Recurring Series",
    badgePendingReview: "Pending Review",

    cycleOneOff: "One-off",
    cycleMonthly: "monthly",
    cycleYearly: "yearly",

    confirmBtn: "Confirm",
    confirmTitle: "Confirm this auto-extracted transaction into the ledger",
    editBtn: "Edit",
    deleteContract: "Delete Contract",
    deleteContractTitle: "Delete entire recurring series history",

    confirmDeleteTx: "Are you sure you want to delete this transaction record?",
    confirmDeleteOccurrence:
      "This transaction belongs to a recurring series.\n\nClick OK to delete THIS SPECIFIC OCCURRENCE only.\nClick Cancel to keep it.",
    confirmDeleteSeries:
      "Are you sure you want to delete the ENTIRE recurring series? This will erase all history for this contract.",

    addModalTitle: "Log Financial Transaction",
    fieldTransactionTitle: "Transaction Title",
    titlePlaceholder: "e.g. Vercel Hosting / Consulting Retainer",
    fieldFlowType: "Flow Type",
    optExpenditure: "Expenditure (Out)",
    optRevenue: "Revenue Income (In)",
    fieldCategoryVendor: "Category / Vendor",
    fieldAmount: "Amount",
    fieldCurrency: "Currency",
    fieldPaymentCycle: "Payment Cycle / Term",
    cycleOptOneOff: "One-off",
    cycleOptMonthly: "Monthly",
    cycleOptYearly: "Yearly",
    fieldTransactionDate: "Transaction Date",
    fieldBillingStartDate: "Billing Cycle Start Date",
    cancel: "Cancel",
    saveRecord: "Save Record",

    editModalTitleAdjust: "Adjust Occurrence",
    editModalTitleEdit: "Edit Transaction",
    recurringInfo:
      "💡 This transaction belongs to a recurring contract. You can edit this single instance or the entire future series.",
    fieldTitle: "Title",
    fieldCategory: "Category",
    fieldOccurrenceDate: "Occurrence Date",
    fieldUpdateScope: "Update Scope",
    scopeThisOnly: "Apply to THIS occurrence only",
    scopeFuture: "Apply to FUTURE occurrences in series",
    saveChanges: "Save Changes",
  },
  zh: {
    lockedFeature: "FinOps 支出优化",
    lockedDesc: "深入洞察运营现金流与订阅支出，并获得主动的生命周期提醒。",
    perk1: "整合订阅跟踪，提供年化运行成本分析",
    perk2: "自动续订提醒，推送至你偏好的通知渠道",
    perk3: "统一的收支账本，助力现金流优化",

    pageTitle: "记账",
    pageDesc: "整合订阅支出账本与现金流跟踪",
    addTransaction: "添加交易",

    statRevenueLabel: "总收入",
    statRevenueDelta: "已收到的现金流入总额",
    statExpenseLabel: "总支出",
    statExpenseDelta: "订阅与一次性费用总额",
    statNetLabel: "净现金利润",
    surplus: "盈余",
    deficit: "亏损",

    filterAllLogs: "全部记录 ({{count}})",
    filterRecurring: "周期性 ({{count}})",
    filterOneOff: "一次性账本 ({{count}})",
    filterAllFlows: "全部流水",
    filterIncomeOnly: "仅收入 ({{count}})",
    filterExpensesOnly: "仅支出 ({{count}})",

    emptyState: "没有符合当前筛选条件的交易记录。",

    thDate: "日期",
    thFlow: "流向",
    thTitle: "标题",
    thCategory: "分类",
    thCycle: "周期",
    thAmount: "金额",

    flowIncome: "收入",
    flowExpense: "支出",

    badgeRecurringSeries: "周期性系列",
    badgePendingReview: "待审核",

    cycleOneOff: "一次性",
    cycleMonthly: "每月",
    cycleYearly: "每年",

    confirmBtn: "确认",
    confirmTitle: "将此自动提取的交易确认入账",
    editBtn: "编辑",
    deleteContract: "删除合约",
    deleteContractTitle: "删除整个周期性系列的历史记录",

    confirmDeleteTx: "确定要删除这条交易记录吗？",
    confirmDeleteOccurrence:
      "此交易属于一个周期性系列。\n\n点击“确定”仅删除此特定发生记录。\n点击“取消”保留它。",
    confirmDeleteSeries:
      "确定要删除整个周期性系列吗？这将清除该合约的所有历史记录。",

    addModalTitle: "记录财务交易",
    fieldTransactionTitle: "交易标题",
    titlePlaceholder: "例如：Vercel 托管 / 咨询顾问费",
    fieldFlowType: "流向类型",
    optExpenditure: "支出（流出）",
    optRevenue: "收入（流入）",
    fieldCategoryVendor: "分类 / 供应商",
    fieldAmount: "金额",
    fieldCurrency: "币种",
    fieldPaymentCycle: "付款周期 / 期限",
    cycleOptOneOff: "一次性",
    cycleOptMonthly: "每月",
    cycleOptYearly: "每年",
    fieldTransactionDate: "交易日期",
    fieldBillingStartDate: "账单周期起始日期",
    cancel: "取消",
    saveRecord: "保存记录",

    editModalTitleAdjust: "调整发生记录",
    editModalTitleEdit: "编辑交易",
    recurringInfo:
      "💡 此交易属于一个周期性合约。你可以编辑此单条记录或整个未来系列。",
    fieldTitle: "标题",
    fieldCategory: "分类",
    fieldOccurrenceDate: "发生日期",
    fieldUpdateScope: "更新范围",
    scopeThisOnly: "仅应用于此发生记录",
    scopeFuture: "应用于系列中未来的发生记录",
    saveChanges: "保存更改",
  },
};
