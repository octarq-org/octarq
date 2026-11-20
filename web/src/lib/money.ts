export function formatUSD(value: number): string {
  if (value >= 1) return `$${value.toFixed(2)}`;
  const [whole, fraction = ""] = value.toFixed(6).replace(/0+$/, "").split(".");
  return `$${whole}.${fraction.padEnd(2, "0")}`;
}
