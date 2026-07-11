export const CURRENCIES = ["USD", "CNY", "EUR", "GBP", "JPY", "HKD", "SGD"];

export function fmtCost(cost: number, currency: string) {
  return new Intl.NumberFormat("en-US", { style: "currency", currency, minimumFractionDigits: 2 }).format(cost);
}

