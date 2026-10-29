import { forwardRef, type ReactNode } from "react";
import { cn } from "./cn";
import { useTranslation } from "../i18n";
import { GlassCard } from "./primitives/glass-card";
import { TableError, errorStatus } from "./primitives/table-error";
import { LockedFeature } from "./locked";

// The four-state surface every data-backed page needs, in one component.
//
// Before this existed each plugin page re-derived the same three branches and
// drifted: 402/404 (absent feature) rendered the upsell, 403 rendered an access
// card, and everything else had to become a retryable failure. Pages that
// collapsed the last two told a licensed operator the feature "is not in this
// build" for a 500, or showed an empty table for a rejected fetch.
//
//   <LoadFailure error={error} feature={...} header={<PageHeader … />} onRetry={load} />
//
// `header` is rendered for the access-denied and failure branches, never above
// the upsell mask (which speaks for the whole feature, not page content).
export interface AccessDeniedProps {
  title?: string;
  body?: string;
  icon?: ReactNode;
  className?: string;
}

export const AccessDenied = forwardRef<HTMLDivElement, AccessDeniedProps>(
  ({ title, body, icon, className }, ref) => {
    const { t } = useTranslation();

    return (
      <GlassCard ref={ref} className={cn("mx-auto mt-8 max-w-md rounded-2xl p-10 text-center", className)}>
        {icon ?? <ShieldGlyph className="mx-auto mb-4 h-8 w-8 text-danger-fg" />}
        <h3 className="mb-1.5 font-display text-[1.125rem] font-semibold tracking-[-0.01em]">{title ?? t("uiCommon.accessDeniedTitle")}</h3>
        <p className="text-[13px] leading-relaxed text-muted-foreground">{body ?? t("uiCommon.accessDeniedBody")}</p>
      </GlassCard>
    );
  },
);
AccessDenied.displayName = "AccessDenied";

export interface LoadFailureProps {
  error?: unknown;
  status?: number; // explicit override for callers that only kept the number
  feature: string; // upsell title for 402 / 404
  description?: string;
  perks?: string[];
  icon?: ReactNode;
  tier?: "pro" | "elite";
  pricingHref?: string;
  header?: ReactNode; // rendered on the 403 and failure branches
  onRetry?: () => void;
  errorText?: ReactNode; // overrides the detail line derived from `error`
  deniedTitle?: string;
  deniedBody?: string;
  className?: string;
}

// Pulls the human-readable detail out of the two shapes callers keep: a plain
// Error / string, or the page-local `{ status, detail }` record.
function errorDetail(error: unknown): string | undefined {
  if (typeof error === "string") return error || undefined;
  if (error instanceof Error) return error.message || undefined;
  if (error && typeof error === "object") {
    const record = error as Record<string, unknown>;
    for (const key of ["detail", "message"] as const) {
      const value = record[key];
      if (typeof value === "string" && value) return value;
    }
  }
  return undefined;
}

export function LoadFailure({
  error,
  status,
  feature,
  description,
  perks,
  icon,
  tier = "pro",
  pricingHref,
  header,
  onRetry,
  errorText,
  deniedTitle,
  deniedBody,
  className,
}: LoadFailureProps) {
  const resolved = status ?? errorStatus(error);

  if (resolved === 402 || resolved === 404) {
    return (
      <LockedFeature
        status={resolved}
        tier={tier}
        feature={feature}
        description={description}
        perks={perks}
        icon={icon}
        pricingHref={pricingHref}
        className={className}
      />
    );
  }

  return (
    <>
      {header}
      {resolved === 403 ? (
        <AccessDenied title={deniedTitle} body={deniedBody} className={className} />
      ) : (
        <TableError
          error={error}
          errorText={errorText ?? errorDetail(error) ?? (resolved ? `HTTP ${resolved}` : undefined)}
          onRetry={onRetry}
          className={className}
        />
      )}
    </>
  );
}

function ShieldGlyph({ className }: { className?: string }) {
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
      <path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z" />
      <path d="M12 8v4" />
      <path d="M12 16h.01" />
    </svg>
  );
}
