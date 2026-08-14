import { createContext, useContext, ReactNode, HTMLAttributes, TableHTMLAttributes, ThHTMLAttributes, TdHTMLAttributes } from "react";
import { cn } from "../cn";

// A small set of themed table primitives — native table elements carrying octarq's
// glass styling. Compose them like plain <table>/<thead>/<tr>/<th>/<td>:
//
//   <Table>
//     <THead><TR><TH>Name</TH></TR></THead>
//     <TBody><TR><TD>…</TD></TR></TBody>
//   </Table>

export type TableDensity = "comfortable" | "compact";

const TableDensityContext = createContext<TableDensity>("comfortable");

// The setter is part of the same context so a preferences UI can change the
// density without the host wiring a second channel for it. Default is a no-op:
// a table rendered outside any provider still reads "comfortable" and renders.
const SetTableDensityContext = createContext<(d: TableDensity) => void>(() => {});

export interface TableDensityProviderProps {
  density?: TableDensity;
  onDensityChange?: (d: TableDensity) => void;
  children: ReactNode;
}

export function TableDensityProvider({ density, onDensityChange, children }: TableDensityProviderProps) {
  return (
    <TableDensityContext.Provider value={density ?? "comfortable"}>
      <SetTableDensityContext.Provider value={onDensityChange ?? NOOP}>
        {children}
      </SetTableDensityContext.Provider>
    </TableDensityContext.Provider>
  );
}

const NOOP = () => {};

export function useTableDensity(): TableDensity {
  return useContext(TableDensityContext);
}

export function useSetTableDensity(): (d: TableDensity) => void {
  return useContext(SetTableDensityContext);
}

export function Table({ className, ...props }: TableHTMLAttributes<HTMLTableElement>) {
  return (
    <div className="w-full overflow-x-auto">
      <table className={cn("w-full border-collapse text-left text-sm", className)} {...props} />
    </div>
  );
}

export function THead({ className, ...props }: HTMLAttributes<HTMLTableSectionElement>) {
  return <thead className={cn("text-foreground/50", className)} {...props} />;
}

export function TBody({ className, ...props }: HTMLAttributes<HTMLTableSectionElement>) {
  return <tbody className={cn("divide-y divide-foreground/[0.06]", className)} {...props} />;
}

export function TR({ className, ...props }: HTMLAttributes<HTMLTableRowElement>) {
  return <tr className={cn("transition-colors hover:bg-foreground/[0.03]", className)} {...props} />;
}

export function TH({ className, ...props }: ThHTMLAttributes<HTMLTableCellElement>) {
  const density = useTableDensity();
  return (
    <th
      className={cn(
        "whitespace-nowrap px-3 text-[12px] font-medium uppercase tracking-wide",
        density === "compact" ? "py-1" : "py-2",
        className,
      )}
      {...props}
    />
  );
}

export function TD({ className, ...props }: TdHTMLAttributes<HTMLTableCellElement>) {
  const density = useTableDensity();
  return (
    <td
      className={cn(
        "px-3 text-foreground/80",
        // Tightened from py-2.5 for the denser look, but NOT down to py-1.5:
        // that left comfortable a hair from compact and made the density
        // control pointless. Comfortable matches the header's py-2; compact
        // stays visibly tighter. Whatever these values become, the two must
        // stay distinguishable — tableDensity.test.tsx pins that.
        density === "compact" ? "py-1" : "py-2",
        className,
      )}
      {...props}
    />
  );
}
