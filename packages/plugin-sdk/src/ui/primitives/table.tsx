import {
  createContext,
  forwardRef,
  useContext,
  type HTMLAttributes,
  type ReactNode,
  type TableHTMLAttributes,
  type TdHTMLAttributes,
  type ThHTMLAttributes,
} from "react";
import { cn } from "../cn";


export type TableDensity = "comfortable" | "compact";

const TableDensityContext = createContext<TableDensity>("comfortable");

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

export type TableProps = TableHTMLAttributes<HTMLTableElement>;

export const Table = forwardRef<HTMLTableElement, TableProps>(({ className, ...props }, ref) => (
  <div className="w-full overflow-x-auto">
    <table ref={ref} className={cn("w-full border-collapse text-left text-[13px]", className)} {...props} />
  </div>
));
Table.displayName = "Table";

export type THeadProps = HTMLAttributes<HTMLTableSectionElement>;

export const THead = forwardRef<HTMLTableSectionElement, THeadProps>(({ className, ...props }, ref) => (
  <thead ref={ref} className={cn("text-muted-foreground", className)} {...props} />
));
THead.displayName = "THead";

export type TBodyProps = HTMLAttributes<HTMLTableSectionElement>;

export const TBody = forwardRef<HTMLTableSectionElement, TBodyProps>(({ className, ...props }, ref) => (
  <tbody ref={ref} className={cn("divide-y divide-border", className)} {...props} />
));
TBody.displayName = "TBody";

export type TRProps = HTMLAttributes<HTMLTableRowElement>;

export const TR = forwardRef<HTMLTableRowElement, TRProps>(({ className, ...props }, ref) => (
  <tr
    ref={ref}
    className={cn("transition-colors duration-[var(--dur-fast)] hover:bg-surface-hover", className)}
    {...props}
  />
));
TR.displayName = "TR";

export type THProps = ThHTMLAttributes<HTMLTableCellElement>;

export const TH = forwardRef<HTMLTableCellElement, THProps>(({ className, ...props }, ref) => {
  const density = useTableDensity();
  return (
    <th
      ref={ref}
      className={cn(
        "whitespace-nowrap px-4 font-mono text-[10.5px] font-medium uppercase tracking-[0.08em]",
        density === "compact" ? "py-1" : "py-2.5",
        className,
      )}
      {...props}
    />
  );
});
TH.displayName = "TH";

export type TDProps = TdHTMLAttributes<HTMLTableCellElement>;

export const TD = forwardRef<HTMLTableCellElement, TDProps>(({ className, ...props }, ref) => {
  const density = useTableDensity();
  return (
    <td
      ref={ref}
      className={cn(
        "px-4 text-foreground/80",
        density === "compact" ? "py-1" : "py-2.5",
        className,
      )}
      {...props}
    />
  );
});
TD.displayName = "TD";
