// formatUSD renders a US-dollar amount. A billed turn can cost less than one
// cent, so a sub-dollar amount keeps up to six decimals — the scale of the
// ledger cost — instead of reading as $0.00. Trailing zeros are trimmed, and
// two decimals stay so $0.50 does not read as $0.5.
export function formatUSD(value: number): string {
  if (value >= 1) return `$${value.toFixed(2)}`;
  const [whole, fraction = ""] = value.toFixed(6).replace(/0+$/, "").split(".");
  return `$${whole}.${fraction.padEnd(2, "0")}`;
}
