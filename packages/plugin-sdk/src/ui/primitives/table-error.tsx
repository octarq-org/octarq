import { ReactNode } from "react";
import { useTranslation } from "../../i18n";
import { Alert } from "./alert";
import { Button } from "./button";
import { LockedFeature } from "../locked";

// Failure state for a table load. Moved here from the host app's pro-table; it
// depends on nothing tanstack, and plugins had no way to render it.
//
// 402 is the licensed-feature case, so it renders the same upsell mask the rest
// of the product uses rather than an error.
export interface TableErrorProps {
  error?: unknown;
  errorText?: ReactNode;
  onRetry?: () => void;
}

// Narrowed without `as any`: the error arrives from whatever the caller's data
// layer rejected with, so both the axios-shaped { response.status } and a plain
// { status } are accepted, and anything else is "no status".
function errorStatus(error: unknown): number {
  if (!error || typeof error !== "object") return 0;
  const direct = (error as { status?: unknown }).status;
  if (typeof direct === "number") return direct;
  const response = (error as { response?: unknown }).response;
  if (response && typeof response === "object") {
    const nested = (response as { status?: unknown }).status;
    if (typeof nested === "number") return nested;
  }
  return 0;
}

export function TableError({ error, errorText, onRetry }: TableErrorProps) {
  const { t } = useTranslation();

  if (errorStatus(error) === 402) {
    return (
      <div className="p-6">
        <LockedFeature status={402} feature="pro_table" />
      </div>
    );
  }

  const errorMessage =
    errorText ??
    (error instanceof Error ? error.message : typeof error === "string" ? error : t("proTable.errorTitle"));

  return (
    <div className="p-6">
      <Alert
        variant="danger"
        icon={<AlertCircleGlyph className="h-4 w-4" />}
        actions={
          onRetry && (
            <Button size="sm" variant="outline" onClick={onRetry} className="gap-1.5">
              <RotateGlyph className="h-3.5 w-3.5" />
              <span>{t("proTable.retry")}</span>
            </Button>
          )
        }
      >
        <div className="space-y-1">
          <p className="font-semibold text-danger-fg">{t("proTable.errorTitle")}</p>
          <p className="text-xs text-danger-fg/80">{String(errorMessage)}</p>
        </div>
      </Alert>
    </div>
  );
}

// Inline so the package stays free of an icon-library dependency.
function AlertCircleGlyph({ className }: { className?: string }) {
  return (
    <svg
      className={className}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <circle cx="12" cy="12" r="10" />
      <path d="M12 8v4" />
      <path d="M12 16h.01" />
    </svg>
  );
}

function RotateGlyph({ className }: { className?: string }) {
  return (
    <svg
      className={className}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8" />
      <path d="M3 3v5h5" />
    </svg>
  );
}
