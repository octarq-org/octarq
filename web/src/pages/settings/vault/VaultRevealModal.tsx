import { useEffect, useState } from "react";
import { Alert, Badge, Button, Modal, toast } from "@octarq/plugin-sdk";
import { Copy, Eye, EyeOff, ShieldAlert } from "lucide-react";
import { api } from "../../../api";
import type { VaultItemMeta, VaultSecretReveal } from "../../../api/types";
import { useTranslation } from "../../../i18n";

interface VaultRevealModalProps {
  item: VaultItemMeta;
  revealFn?: (id: number) => Promise<VaultSecretReveal>;
  warningText?: string;
  onClose: () => void;
}

export function VaultRevealModal({ item, revealFn, warningText, onClose }: VaultRevealModalProps) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [data, setData] = useState<VaultSecretReveal | null>(null);
  const [visibleKeys, setVisibleKeys] = useState<Record<string, boolean>>({});

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    const call = revealFn ?? api.revealVaultItem;
    call(item.id)
      .then((res) => {
        if (cancelled) return;
        setData(res);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        setError(err instanceof Error ? err.message : t("vault.loadFailed"));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [item.id, revealFn, t]);

  const copyValue = async (val: string) => {
    try {
      await navigator.clipboard.writeText(val);
      toast.success(t("vault.copied"));
    } catch {
      toast.error(t("vault.copyFailed"));
    }
  };

  return (
    <Modal
      wide
      title={t("vault.revealTitle", { name: item.name })}
      onClose={onClose}
    >
      <div className="space-y-4">
        <Alert variant="warning" icon={<ShieldAlert className="h-4 w-4 shrink-0" />}>
          <span className="text-xs">{warningText ?? t("vault.revealWarning")}</span>
        </Alert>

        {loading ? (
          <div className="py-6 text-center text-xs text-muted-foreground">
            {t("vault.loadingSecret")}
          </div>
        ) : error ? (
          <Alert variant="danger">
            <span className="text-xs">{error}</span>
          </Alert>
        ) : data ? (
          <div className="space-y-3">
            <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
              <Badge>{t("vault.versionBadge", { version: data.version })}</Badge>
              {data.meta.ref && <code className="font-mono text-xs text-foreground">{data.meta.ref}</code>}
            </div>
            <div className="divide-y divide-border rounded-lg border border-border bg-well">
              {Object.entries(data.payload).map(([k, v]) => {
                const strVal = typeof v === "string" ? v : JSON.stringify(v);
                const shown = Boolean(visibleKeys[k]);
                return (
                  <div key={k} className="flex flex-col gap-2 p-3 sm:flex-row sm:items-center sm:justify-between">
                    <div className="min-w-0 flex-1 space-y-1">
                      <div className="font-mono text-xs font-medium text-muted-foreground">{k}</div>
                      <div className="break-all font-mono text-xs text-foreground">
                        {shown ? strVal : "••••••••••••"}
                      </div>
                    </div>
                    <div className="flex shrink-0 items-center gap-1.5">
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        onClick={() => setVisibleKeys((prev) => ({ ...prev, [k]: !prev[k] }))}
                        className="flex items-center gap-1 text-xs"
                      >
                        {shown ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
                        <span>{shown ? t("vault.hide") : t("vault.show")}</span>
                      </Button>
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        onClick={() => void copyValue(strVal)}
                        className="flex items-center gap-1 text-xs"
                      >
                        <Copy className="h-3.5 w-3.5" />
                        <span>{t("vault.copyValue")}</span>
                      </Button>
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        ) : null}

        <div className="flex justify-end border-t border-border pt-4">
          <Button type="button" variant="outline" onClick={onClose}>
            {t("common.close")}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
