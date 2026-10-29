// formatUSD renders a US-dollar amount. A billed turn can cost less than one
// cent, so amounts below one dollar keep four decimals instead of reading as
// $0.00.
export function formatUSD(value: number): string {
  return `$${value >= 1 ? value.toFixed(2) : Number(value.toFixed(4))}`;
}
