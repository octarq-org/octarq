import { useCallback, useEffect, useState } from "react";
import { Alert, Button, Modal, toast } from "@octarq/plugin-sdk";
import { Copy, RefreshCw } from "lucide-react";
import { api } from "../../../api";
import type { VaultItemMeta } from "../../../api/types";
import { useTranslation } from "../../../i18n";

interface VaultTotpModalProps {
  item: VaultItemMeta;
  onClose: () => void;
}

export function VaultTotpModal({ item, onClose }: VaultTotpModalProps) {
  const { t } = useTranslation();
  const [code, setCode] = useState("");
  const [remaining, setRemaining] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchCode = useCallback(async () => {
    setError(null);
    try {
      const res = await api.vaultItemTOTP(item.id);
      setCode(res.code);
      setRemaining(res.remainingSeconds);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("vault.loadFailed"));
    } finally {
      setLoading(false);
    }
  }, [item.id, t]);

  useEffect(() => {
    void fetchCode();
  }, [fetchCode]);

  useEffect(() => {
    if (loading || error) return;
    const timer = window.setInterval(() => {
      setRemaining((prev) => {
        if (prev <= 1) {
          void fetchCode();
          return 0;
        }
        return prev - 1;
      });
    }, 1000);
    return () => window.clearInterval(timer);
  }, [loading, error, fetchCode]);

  const copyCode = async () => {
    if (!code) return;
    try {
      await navigator.clipboard.writeText(code);
      toast.success(t("vault.copied"));
    } catch {
      toast.error(t("vault.copyFailed"));
    }
  };

  return (
    <Modal title={t("vault.totpTitle", { name: item.name })} onClose={onClose}>
      <div className="space-y-4">
        {loading ? (
          <div className="py-6 text-center text-xs text-muted-foreground">
            {t("vault.loadingSecret")}
          </div>
        ) : error ? (
          <Alert variant="danger">
            <span className="text-xs">{error}</span>
          </Alert>
        ) : (
          <div className="space-y-4 rounded-lg border border-border bg-well p-5 text-center">
            <div
              data-testid="totp-code-value"
              className="font-mono tnum text-3xl font-bold tracking-[0.25em] text-foreground"
            >
              {code}
            </div>
            <div className="text-xs font-mono tnum text-muted-foreground">
              {t("vault.totpRemaining", { seconds: remaining })}
            </div>
            <div className="flex justify-center gap-2">
              <Button
                type="button"
                variant="primary"
                size="sm"
                onClick={() => void copyCode()}
                className="flex items-center gap-1.5"
              >
                <Copy className="h-3.5 w-3.5" />
                <span>{t("vault.copyCode")}</span>
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => void fetchCode()}
                className="flex items-center gap-1.5"
              >
                <RefreshCw className="h-3.5 w-3.5" />
                <span>{t("common.refresh")}</span>
              </Button>
            </div>
          </div>
        )}

        <div className="flex justify-end border-t border-border pt-4">
          <Button type="button" variant="outline" onClick={onClose}>
            {t("common.close")}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
