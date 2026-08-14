import { useTranslation } from "../../i18n";

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
  const message = typeof err === "string" ? err : err.message;
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
