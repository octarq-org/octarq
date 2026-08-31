import { useState } from "react";
import { dnsApi, EmailBlueprintRecord, BlueprintStatus } from "../api";
import { Modal, Button, Alert, Badge, FormError } from "../../../ui";
import { useTranslation } from "../../../i18n";
import { roleSatisfies, useCurrentRole } from "../../../shell/role";
import { CheckCircle2, XCircle, AlertCircle, Copy, Check, Zap } from "lucide-react";

function statusIcon(status: BlueprintStatus) {
  switch (status) {
    case "ok":
      return <CheckCircle2 className="h-4 w-4 text-success-fg flex-shrink-0" />;
    case "mismatch":
      return <AlertCircle className="h-4 w-4 text-warning-fg flex-shrink-0" />;
    default:
      return <XCircle className="h-4 w-4 text-danger-fg flex-shrink-0" />;
  }
}

function CopyButton({ text }: { text: string }) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);
  function copy() {
    navigator.clipboard.writeText(text).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  }
  return (
    <button
      type="button"
      onClick={copy}
      className="p-1 rounded hover:bg-foreground/10 text-foreground/50 hover:text-foreground transition-colors"
      title={t("domains.copyRecord")}
    >
      {copied ? <Check className="h-3 w-3 text-success-fg" /> : <Copy className="h-3 w-3" />}
    </button>
  );
}

export function EmailBlueprintPanel({
  domainId,
  hasProvider,
  onClose,
  onApplied,
}: {
  domainId: number;
  hasProvider: boolean;
  onClose: () => void;
  onApplied: () => void;
}) {
  const { t } = useTranslation();
  const { role, isInstanceAdmin } = useCurrentRole();
  const isAdmin = roleSatisfies("admin", role, isInstanceAdmin);

  const [records, setRecords] = useState<EmailBlueprintRecord[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [applying, setApplying] = useState(false);
  const [err, setErr] = useState<string | { message?: string; status?: number; requestId?: string }>("");
  const [applyResult, setApplyResult] = useState<{ applied: number; skipped: number } | null>(null);

  async function loadBlueprint() {
    setLoading(true);
    setErr("");
    setApplyResult(null);
    try {
      const recs = await dnsApi.emailBlueprint(domainId);
      setRecords(recs);
    } catch (e: any) {
      setErr(e);
    } finally {
      setLoading(false);
    }
  }

  async function apply() {
    setApplying(true);
    setErr("");
    try {
      const result = await dnsApi.applyEmailBlueprint(domainId);
      setApplyResult({ applied: result.applied, skipped: result.skipped });
      const recs = await dnsApi.emailBlueprint(domainId);
      setRecords(recs);
      onApplied();
    } catch (e: any) {
      setErr(e);
    } finally {
      setApplying(false);
    }
  }

  // Load on first render
  if (records === null && !loading && !err) {
    loadBlueprint();
  }

  const allOk = records !== null && records.every((r) => r.status === "ok");
  const hasMissing = records !== null && records.some((r) => r.status !== "ok");

  return (
    <Modal title={t("domains.emailBlueprintTitle")} onClose={onClose}>
      <div className="space-y-4">
        <p className="text-xs text-foreground/60">{t("domains.emailBlueprintDesc")}</p>

        {loading && (
          <p className="text-foreground/40 text-xs text-center py-4">{t("domains.emailBlueprintLoading")}</p>
        )}

        {err && <FormError err={err} />}

        {applyResult && (
          <Alert variant="success" className="text-xs p-3">
            {t("domains.emailBlueprintApplyResult", { applied: applyResult.applied, skipped: applyResult.skipped })}
          </Alert>
        )}

        {records !== null && (
          <div className="rounded-xl border border-foreground/[0.08] overflow-hidden">
            {records.map((rec, i) => (
              <div
                key={i}
                className={`flex items-start gap-3 px-3 py-2.5 text-xs ${
                  i > 0 ? "border-t border-foreground/[0.06]" : ""
                } ${rec.status === "ok" ? "bg-success-bg/20" : rec.status === "mismatch" ? "bg-warning-bg/20" : ""}`}
              >
                {statusIcon(rec.status)}
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2 flex-wrap">
                    <span className="font-mono bg-foreground/8 border border-foreground/[0.06] px-1.5 py-0.5 rounded text-[10px] font-semibold">
                      {rec.type}
                    </span>
                    <span className="font-mono text-foreground/70 truncate">{rec.name}</span>
                    <Badge
                      variant={rec.status === "ok" ? "success" : rec.status === "mismatch" ? "warning" : "danger"}
                      className="text-[10px]"
                    >
                      {t(`domains.blueprintStatus_${rec.status}`)}
                    </Badge>
                  </div>
                  <p className="font-mono text-foreground/50 truncate mt-0.5 text-[10px]">{rec.content}</p>
                </div>
                <CopyButton text={rec.content} />
              </div>
            ))}
          </div>
        )}

        {allOk && (
          <Alert variant="success" className="text-xs p-3">
            {t("domains.emailBlueprintAllOk")}
          </Alert>
        )}

        <div className="flex justify-between gap-2 pt-2 border-t border-foreground/[0.06]">
          <Button variant="ghost" onClick={onClose} className="text-xs">
            {t("domains.cancel")}
          </Button>
          <div className="flex gap-2">
            <Button variant="subtle" onClick={loadBlueprint} disabled={loading} className="text-xs">
              {t("domains.emailBlueprintRefresh")}
            </Button>
            {isAdmin && hasProvider && hasMissing && (
              <Button
                variant="primary"
                onClick={apply}
                disabled={applying || !hasMissing}
                className="text-xs gap-1.5"
              >
                <Zap className="h-3 w-3" />
                {applying ? t("domains.emailBlueprintApplying") : t("domains.emailBlueprintApply")}
              </Button>
            )}
          </div>
        </div>
      </div>
    </Modal>
  );
}
