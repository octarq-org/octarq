import { forwardRef } from "react";
import { cn } from "../cn";
import { useTranslation } from "../../i18n";

export const formErrorStatusKeys: Record<number, string> = {
  401: "uiCommon.errStatus401",
  403: "uiCommon.errStatus403",
  404: "uiCommon.errStatus404",
  429: "uiCommon.errStatus429",
  500: "uiCommon.errStatus500",
};

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

export const FormError = forwardRef<HTMLDivElement, FormErrorProps>(({ err, className }, ref) => {
  const { t } = useTranslation();
  if (!err) return null;
  const message = formErrorMessage(err, t);
  const status = typeof err === "object" ? err.status : undefined;
  const requestId = typeof err === "object" ? err.requestId : undefined;
  return (
    <div ref={ref} role="alert" aria-live="polite" className={cn("space-y-1", className)}>
      <p className="text-sm font-medium text-danger-fg">{message}</p>
      {status !== undefined && (
        <p className="font-mono tabular-nums tnum text-[11px] text-danger-fg/70">
          {t("uiCommon.formErrorStatus", { status })}
          {requestId && <> · {t("uiCommon.formErrorRequestId", { requestId })}</>}
        </p>
      )}
    </div>
  );
});
FormError.displayName = "FormError";
