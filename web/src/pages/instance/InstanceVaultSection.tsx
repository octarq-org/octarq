import { useCallback, useEffect, useState } from "react";
import {
  Badge,
  Button,
  Field,
  Input,
  Modal,
  PageCard,
  PageEmpty,
  PageRows,
  PageSection,
  Separator,
  StatusBadge,
  confirmDialog,
  toast,
} from "@octarq/plugin-sdk";
import { Eye, EyeOff, KeyRound, Pencil, Plus, ShieldCheck, Trash2 } from "lucide-react";
import { api } from "../../api";
import type { VaultItemMeta } from "../../api/types";
import { ListSkeleton } from "../../components/ListSkeleton";
import { useTranslation } from "../../i18n";
import { VaultRevealModal } from "../settings/vault/VaultRevealModal";

export function InstanceVaultSection() {
  const { t } = useTranslation();
  const [items, setItems] = useState<VaultItemMeta[]>([]);
  const [loading, setLoading] = useState(true);
  const [showPutModal, setShowPutModal] = useState(false);
  const [editingItem, setEditingItem] = useState<VaultItemMeta | null>(null);
  const [revealingItem, setRevealingItem] = useState<VaultItemMeta | null>(null);

  const load = useCallback(() => {
    if (!api.instanceVaultItems) {
      setItems([]);
      setLoading(false);
      return;
    }
    setLoading(true);
    api.instanceVaultItems()
      .then((rows) => setItems(Array.isArray(rows) ? rows : []))
      .catch(() => setItems([]))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const handleDelete = async (item: VaultItemMeta) => {
    if (!(await confirmDialog(t("vault.confirmDeleteInstanceItem", { name: item.keyName || item.name })))) {
      return;
    }
    try {
      await api.deleteInstanceVaultItem(item.id);
      toast.success(t("vault.itemDeleted"));
      setItems((prev) => prev.filter((x) => x.id !== item.id));
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : t("vault.loadFailed"));
    }
  };

  return (
    <PageSection
      title={t("vault.instanceTitle")}
      description={t("vault.instanceDesc")}
      action={
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => {
            setEditingItem(null);
            setShowPutModal(true);
          }}
          className="flex items-center gap-1.5"
        >
          <Plus className="h-3.5 w-3.5" />
          <span>{t("vault.instanceAdd")}</span>
        </Button>
      }
    >
      {loading ? (
        <PageCard className="mt-0 p-5">
          <ListSkeleton rows={2} ariaLabel={t("vault.loadingItems")} />
        </PageCard>
      ) : items.length === 0 ? (
        <PageEmpty
          className="mt-0"
          icon={<ShieldCheck className="h-5 w-5" />}
          title={t("vault.instanceEmpty")}
        />
      ) : (
        <PageRows className="mt-0">
          {items.map((item, idx) => (
            <div key={item.id} data-testid={`instance-vault-item-${item.id}`}>
              {idx > 0 && <Separator />}
              <div className="flex flex-col gap-3 px-4 py-3.5 text-sm sm:flex-row sm:items-center sm:justify-between">
                <div className="min-w-0 flex-1 space-y-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <KeyRound className="h-3.5 w-3.5 text-muted-foreground" />
                    <span className="font-medium text-foreground">{item.name}</span>
                    {item.keyName && (
                      <Badge className="font-mono">{item.keyName}</Badge>
                    )}
                    <Badge>{t("vault.versionBadge", { version: item.currentVersion })}</Badge>
                    <StatusBadge tone={item.configured ? "success" : "idle"}>
                      {item.configured ? t("vault.configured") : t("vault.unconfigured")}
                    </StatusBadge>
                  </div>
                  <div className="font-mono text-xs text-muted-foreground">
                    {`${item.collectionSlug}/${item.environment} · ••••••••`}
                  </div>
                </div>
                <div className="flex shrink-0 flex-wrap items-center gap-1.5">
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => setRevealingItem(item)}
                    className="flex items-center gap-1 text-xs"
                  >
                    <Eye className="h-3.5 w-3.5" />
                    <span>{t("vault.reveal")}</span>
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => {
                      setEditingItem(item);
                      setShowPutModal(true);
                    }}
                    className="flex items-center gap-1 text-xs"
                  >
                    <Pencil className="h-3.5 w-3.5" />
                    <span>{t("vault.edit")}</span>
                  </Button>
                  <Button
                    type="button"
                    variant="danger"
                    size="sm"
                    onClick={() => void handleDelete(item)}
                    className="flex items-center gap-1 text-xs"
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                    <span>{t("vault.delete")}</span>
                  </Button>
                </div>
              </div>
            </div>
          ))}
        </PageRows>
      )}

      {showPutModal && (
        <PutInstanceSecretModal
          editingItem={editingItem}
          onClose={() => {
            setShowPutModal(false);
            setEditingItem(null);
          }}
          onSaved={() => {
            setShowPutModal(false);
            setEditingItem(null);
            load();
          }}
        />
      )}

      {revealingItem && (
        <VaultRevealModal
          item={revealingItem}
          revealFn={api.revealInstanceVaultItem}
          warningText={t("vault.instanceRevealWarning")}
          onClose={() => setRevealingItem(null)}
        />
      )}
    </PageSection>
  );
}

function PutInstanceSecretModal({
  editingItem,
  onClose,
  onSaved,
}: {
  editingItem: VaultItemMeta | null;
  onClose: () => void;
  onSaved: () => void;
}) {
  const { t } = useTranslation();
  const [keyName, setKeyName] = useState(editingItem?.keyName ?? "");
  const [name, setName] = useState(editingItem?.name ?? "");
  const [secretValue, setSecretValue] = useState("");
  const [changeNote, setChangeNote] = useState("");
  const [revealed, setRevealed] = useState(false);
  const [busy, setBusy] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!keyName.trim() || !secretValue.trim()) return;
    setBusy(true);
    try {
      await api.putInstanceVaultItem({
        keyName: keyName.trim(),
        name: name.trim() || keyName.trim(),
        itemType: "kv_secret",
        payload: { value: secretValue.trim() },
        changeNote: changeNote.trim() || undefined,
      });
      toast.success(t("vault.itemSaved"));
      onSaved();
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : t("vault.loadFailed"));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal
      title={editingItem ? t("vault.instanceEdit") : t("vault.instanceAdd")}
      onClose={onClose}
    >
      <form onSubmit={handleSubmit} className="space-y-4">
        <Field label={t("vault.instanceKeyName")}>
          <Input
            className="font-mono text-xs"
            value={keyName}
            onChange={(e) => setKeyName(e.target.value)}
            placeholder={t("vault.instanceKeyPlaceholder")}
            readOnly={Boolean(editingItem)}
            required
            autoFocus={!editingItem}
          />
        </Field>
        <Field label={t("vault.itemName")}>
          <Input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={t("vault.instanceKeyPlaceholder")}
          />
        </Field>
        <Field label={t("vault.instanceSecretValue")}>
          <div className="flex items-center gap-1.5">
            <Input
              type={revealed ? "text" : "password"}
              className="font-mono text-xs"
              value={secretValue}
              onChange={(e) => setSecretValue(e.target.value)}
              placeholder={editingItem ? "••••••••" : ""}
              required
              autoFocus={Boolean(editingItem)}
            />
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="shrink-0 px-2"
              aria-label={revealed ? t("vault.hide") : t("vault.show")}
              onClick={() => setRevealed((prev) => !prev)}
            >
              {revealed ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
            </Button>
          </div>
        </Field>
        <Field label={t("vault.changeNote")}>
          <Input
            value={changeNote}
            onChange={(e) => setChangeNote(e.target.value)}
            placeholder={t("vault.changeNotePlaceholder")}
          />
        </Field>
        <div className="flex justify-end gap-2 border-t border-border pt-4">
          <Button type="button" variant="ghost" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button type="submit" variant="primary" disabled={busy || !keyName.trim() || !secretValue.trim()}>
            {busy ? t("vault.saving") : t("vault.save")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
