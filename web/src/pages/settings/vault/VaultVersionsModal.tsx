import { useCallback, useEffect, useState } from "react";
import { Alert, Badge, Button, Modal, confirmDialog, toast } from "@octarq/plugin-sdk";
import { RotateCcw } from "lucide-react";
import { api } from "../../../api";
import type { VaultItemMeta, VaultVersionDTO } from "../../../api/types";
import { useTranslation } from "../../../i18n";

interface VaultVersionsModalProps {
  item: VaultItemMeta;
  canRollback?: boolean;
  onClose: () => void;
  onRolledBack: (updated: VaultItemMeta) => void;
}

export function VaultVersionsModal({
  item,
  canRollback = true,
  onClose,
  onRolledBack,
}: VaultVersionsModalProps) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [versions, setVersions] = useState<VaultVersionDTO[]>([]);
  const [busyVersion, setBusyVersion] = useState<number | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    api.vaultItemVersions(item.id)
      .then(setVersions)
      .catch((err: unknown) => setError(err instanceof Error ? err.message : t("vault.loadFailed")))
      .finally(() => setLoading(false));
  }, [item.id, t]);

  useEffect(() => {
    load();
  }, [load]);

  const handleRollback = async (version: number) => {
    if (!(await confirmDialog(t("vault.confirmRollback", { version })))) return;
    setBusyVersion(version);
    try {
      const updated = await api.rollbackVaultItem(item.id, version);
      toast.success(t("vault.rolledBack", { version }));
      onRolledBack(updated);
      onClose();
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : t("vault.loadFailed"));
    } finally {
      setBusyVersion(null);
    }
  };

  return (
    <Modal
      wide
      title={t("vault.versionsTitle", { name: item.name })}
      onClose={onClose}
    >
      <div className="space-y-4">
        {loading ? (
          <div className="py-6 text-center text-xs text-muted-foreground">
            {t("vault.loadingVersions")}
          </div>
        ) : error ? (
          <Alert variant="danger">
            <span className="text-xs">{error}</span>
          </Alert>
        ) : versions.length === 0 ? (
          <div className="py-6 text-center text-xs text-muted-foreground">
            {t("vault.noVersions")}
          </div>
        ) : (
          <div className="divide-y divide-border rounded-lg border border-border bg-well">
            {versions.map((v) => {
              const isCurrent = v.version === item.currentVersion;
              return (
                <div key={v.version} className="flex flex-col gap-2 p-3 sm:flex-row sm:items-center sm:justify-between">
                  <div className="min-w-0 flex-1 space-y-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <Badge tone={isCurrent ? "success" : "default"}>
                        {t("vault.versionBadge", { version: v.version })}
                      </Badge>
                      {isCurrent && (
                        <Badge tone="info">{t("vault.currentVersion")}</Badge>
                      )}
                      <span className="font-mono text-[11px] text-muted-foreground">
                        {new Date(v.createdAt).toLocaleString()}
                      </span>
                    </div>
                    {v.changeNote && (
                      <p className="text-xs text-foreground">{v.changeNote}</p>
                    )}
                  </div>
                  {canRollback && !isCurrent && (
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      disabled={busyVersion === v.version}
                      onClick={() => void handleRollback(v.version)}
                      className="flex shrink-0 items-center gap-1.5 text-xs"
                    >
                      <RotateCcw className="h-3.5 w-3.5" />
                      <span>{t("vault.rollback")}</span>
                    </Button>
                  )}
                </div>
              );
            })}
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
