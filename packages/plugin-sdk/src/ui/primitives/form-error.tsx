import { forwardRef } from "react";
import { cn } from "../cn";
import { useTranslation } from "../../i18n";

// The most common failure statuses get a localized, generic line so a zh/es/pt/ja
// operator doesn't read an English detail for a failure whose meaning doesn't
// depend on the backend message. Anything not listed falls back to the server's
// original message verbatim — collapsing an unknown error into a generic
// "something went wrong" would make every failure indistinguishable.
//
// Moved here from web/src/components/ui/FormError.tsx so a plugin package can
// render the same failure surface; it reads the SDK's own i18n context, which
// the host app feeds (the uiCommon.* keys below live in the host dictionary).
export const formErrorStatusKeys: Record<number, string> = {
  401: "uiCommon.errStatus401",
  403: "uiCommon.errStatus403",
  404: "uiCommon.errStatus404",
  429: "uiCommon.errStatus429",
  500: "uiCommon.errStatus500",
};

// Localizes a form-failure error: mapped statuses render their generic copy,
// everything else (unknown status, or the message a caller passed directly)
// keeps the backend's own text — the most traceable fallback there is.
export function formErrorMessage(
  err: string | { message?: string; status?: number; requestId?: string } | null | undefined,
  t: (key: string, vars?: Record<string, string | number>) => string,
): string {
  if (typeof err === "string") return err;
  const status = err?.status;
  const key = status !== undefined ? formErrorStatusKeys[status] : undefined;
  if (key) return t(key);
  return err?.message ?? "";
}

export interface FormErrorProps {
  err: string | { message?: string; status?: number; requestId?: string } | null | undefined;
  className?: string;
}

// Shared form-failure surface for plugin edit forms. The backend answers with a
// human message (no structured field names), so the added value here is what
// only a self-hosted operator can act on: the HTTP status and the server's
// X-Request-Id — the correlation id for their own logs. Both are machine values,
// so they render in mono.
export const FormError = forwardRef<HTMLDivElement, FormErrorProps>(({ err, className }, ref) => {
  const { t } = useTranslation();
  if (!err) return null;
  const message = formErrorMessage(err, t);
  const status = typeof err === "object" ? err.status : undefined;
  const requestId = typeof err === "object" ? err.requestId : undefined;
  return (
    <div ref={ref} className={cn("space-y-1", className)}>
      <p className="text-sm font-medium text-danger-fg">{message}</p>
      {status !== undefined && (
        <p className="font-mono tnum text-[11px] text-danger-fg/70">
          {t("uiCommon.formErrorStatus", { status })}
          {requestId && <> · {t("uiCommon.formErrorRequestId", { requestId })}</>}
        </p>
      )}
    </div>
  );
});
FormError.displayName = "FormError";
