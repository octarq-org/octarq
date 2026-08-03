export interface DnsFilterState {
  type: string;
  q: string;
}

export function parseDnsFilter(searchParams: URLSearchParams | string): DnsFilterState {
  const params = typeof searchParams === "string" ? new URLSearchParams(searchParams) : searchParams;
  const type = params.get("type") || "";
  const q = params.get("q") || "";
  return { type, q };
}

export function buildDnsFilterQuery(
  filters: DnsFilterState,
  prevParams?: URLSearchParams
): URLSearchParams {
  const next = new URLSearchParams(prevParams);
  if (filters.type) {
    next.set("type", filters.type);
  } else {
    next.delete("type");
  }
  if (filters.q) {
    next.set("q", filters.q);
  } else {
    next.delete("q");
  }
  return next;
}
