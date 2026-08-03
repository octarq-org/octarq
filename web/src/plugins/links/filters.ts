export interface LinksFilterState {
  q: string;
  archived: boolean;
}

export function parseLinksFilter(searchParams: URLSearchParams | string): LinksFilterState {
  const params = typeof searchParams === "string" ? new URLSearchParams(searchParams) : searchParams;
  const q = params.get("q") || "";
  const archived = params.get("archived") === "1";
  return { q, archived };
}

export function buildLinksFilterQuery(
  filters: LinksFilterState,
  prevParams?: URLSearchParams
): URLSearchParams {
  const next = new URLSearchParams(prevParams);
  if (filters.q) {
    next.set("q", filters.q);
  } else {
    next.delete("q");
  }
  if (filters.archived) {
    next.set("archived", "1");
  } else {
    next.delete("archived");
  }
  return next;
}
