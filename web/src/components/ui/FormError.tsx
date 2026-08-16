import { useTranslation } from "../../i18n";

// The most common failure statuses get a localized, generic line so a zh/es/pt/ja
// operator doesn't read an English detail for a failure whose meaning doesn't
// depend on the backend message. Anything not listed here falls back to the
// server's original message verbatim — collapsing an unknown error into a
// generic "something went wrong" would make every failure indistinguishable.
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

// Shared form-failure surface for plugin edit forms. The backend answers with
// a human message (no structured field names), so the added value here is what
// only a self-hosted operator can act on: the HTTP status and the server's
// X-Request-Id — the correlation id for their own logs. Both are machine
// values, so they render in mono.
export function FormError({
  err,
}: {
  err: string | { message?: string; status?: number; requestId?: string } | null | undefined;
}) {
  const { t } = useTranslation();
  if (!err) return null;
  const message = formErrorMessage(err, t);
  const status = typeof err === "object" ? err.status : undefined;
  const requestId = typeof err === "object" ? err.requestId : undefined;
  return (
    <div className="space-y-1">
      <p className="text-sm font-medium text-danger-fg">{message}</p>
      {status !== undefined && (
        <p className="font-mono tnum text-[11px] text-danger-fg/70">
          {t("uiCommon.formErrorStatus", { status })}
          {requestId && <> · {t("uiCommon.formErrorRequestId", { requestId })}</>}
        </p>
      )}
    </div>
  );
}
