import { Table, THead, TBody, TR, TH, TD } from "./table";
import { Skeleton } from "./skeleton";

// Loading placeholder shaped like the table it replaces, so the layout does not
// jump when rows arrive. Moved here from the host app's pro-table; it depends on
// nothing tanstack.
export interface TableSkeletonProps {
  columnsCount?: number;
  rowsCount?: number;
  ariaLabel?: string;
}

export function TableSkeleton({ columnsCount = 5, rowsCount = 6, ariaLabel }: TableSkeletonProps) {
  const cols = Math.max(1, columnsCount);
  const rows = Math.max(1, rowsCount);

  return (
    <div aria-busy="true" role="status" aria-label={ariaLabel} className="w-full">
      <Table>
        <THead className="border-b border-foreground/[0.06] bg-foreground/[0.02]">
          <TR>
            {Array.from({ length: cols }).map((_, i) => (
              <TH key={i}>
                <Skeleton className="h-4 w-20" />
              </TH>
            ))}
          </TR>
        </THead>
        <TBody>
          {Array.from({ length: rows }).map((_, r) => (
            <TR key={r}>
              {Array.from({ length: cols }).map((_, c) => (
                <TD key={c}>
                  <Skeleton className={`h-4 ${c === 0 ? "w-24" : c === 1 ? "w-32" : "w-16"}`} />
                </TD>
              ))}
            </TR>
          ))}
        </TBody>
      </Table>
    </div>
  );
}
