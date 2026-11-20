import { useEffect, useMemo, useState } from "react";
import {
  Button,
  Field,
  Input,
  Modal,
  Select,
  Switch,
  Textarea,
  toast,
} from "@octarq/plugin-sdk";
import { Eye, EyeOff, Plus, Trash2 } from "lucide-react";
import { api } from "../../../api";
import type {
  VaultCategorySchema,
  VaultCollection,
  VaultItemMeta,
  VaultProviderSchema,
  VaultSchemaField,
} from "../../../api/types";
import { useTranslation } from "../../../i18n";
import {
  findSchemaForItem,
  looksSecretFieldKey,
  VAULT_ENVIRONMENTS,
  type VaultTab,
} from "./types";

interface CustomFieldRow {
  id: string;
  key: string;
  value: string;
  secret: boolean;
  revealed: boolean;
}

interface VaultItemModalProps {
  schemas: VaultCategorySchema[];
  collections: VaultCollection[];
  editingItem?: VaultItemMeta | null;
  defaultTab: VaultTab;
  defaultProviderType?: string;
  defaultCollectionSlug?: string;
  defaultEnvironment?: string;
  onClose: () => void;
  onSaved: (item: VaultItemMeta) => void;
}

function initialCategoryKey(
  schemas: VaultCategorySchema[],
  tab: VaultTab,
  providerType?: string,
): string {
  if (providerType && schemas.some((c) => c.key === providerType)) {
    return providerType;
  }
  if (tab === "secrets" && schemas.some((c) => c.key === "secrets")) {
    return "secrets";
  }
  if (tab === "logins" && schemas.some((c) => c.key === "vault_items")) {
    return "vault_items";
  }
  return schemas[0]?.key ?? "";
}

function buildDefaultsForProvider(provider?: VaultProviderSchema): Record<string, unknown> {
  const values: Record<string, unknown> = {};
  if (!provider) return values;
  for (const field of provider.fields) {
    if (field.default !== undefined) {
      values[field.key] = field.default;
    } else if (field.kind === "boolean") {
      values[field.key] = false;
    } else {
      values[field.key] = "";
    }
  }
  return values;
}

export function VaultItemModal({
  schemas,
  collections,
  editingItem,
  defaultTab,
  defaultProviderType,
  defaultCollectionSlug,
  defaultEnvironment,
  onClose,
  onSaved,
}: VaultItemModalProps) {
  const { t } = useTranslation();
  const isEdit = Boolean(editingItem);
  const editingSchema = useMemo(
    () => (editingItem ? findSchemaForItem(schemas, editingItem) : undefined),
    [schemas, editingItem],
  );

  const [categoryKey, setCategoryKey] = useState<string>(() =>
    initialCategoryKey(schemas, defaultTab, defaultProviderType),
  );

  const selectedCategory = useMemo(
    () => schemas.find((c) => c.key === categoryKey) ?? schemas[0],
    [schemas, categoryKey],
  );

  const [providerId, setProviderId] = useState<string>(() => {
    if (editingSchema) return editingSchema.id;
    const cat = schemas.find((c) => c.key === initialCategoryKey(schemas, defaultTab, defaultProviderType)) ?? schemas[0];
    return cat?.providers[0]?.id ?? "";
  });

  const selectedProvider = useMemo<VaultProviderSchema | undefined>(() => {
    if (isEdit) return editingSchema;
    return selectedCategory?.providers.find((p) => p.id === providerId) ?? selectedCategory?.providers[0];
  }, [isEdit, editingSchema, selectedCategory, providerId]);

  const [name, setName] = useState(editingItem?.name ?? "");
  const [collectionSlug, setCollectionSlug] = useState(() => {
    if (editingItem) return editingItem.collectionSlug;
    if (defaultCollectionSlug) return defaultCollectionSlug;
    return selectedProvider?.itemType === "integration" ? "integrations" : "default";
  });
  const [environment, setEnvironment] = useState(() =>
    editingItem?.environment || (defaultEnvironment && defaultEnvironment !== "" ? defaultEnvironment : "default"),
  );
  const [folderPath, setFolderPath] = useState(editingItem?.folderPath ?? "/");
  const [keyName, setKeyName] = useState(editingItem?.keyName ?? "");
  const [isDefault, setIsDefault] = useState(editingItem?.isDefault ?? false);
  const [changeNote, setChangeNote] = useState("");
  const [values, setValues] = useState<Record<string, unknown>>(() => {
    if (editingItem && editingSchema) {
      const init: Record<string, unknown> = {};
      for (const f of editingSchema.fields) {
        if (f.secret || f.kind === "secret") {
          init[f.key] = "";
        } else if (editingItem.metadata && editingItem.metadata[f.key] !== undefined) {
          init[f.key] = editingItem.metadata[f.key];
        } else if (f.default !== undefined) {
          init[f.key] = f.default;
        } else if (f.kind === "boolean") {
          init[f.key] = false;
        } else {
          init[f.key] = "";
        }
      }
      return init;
    }
    return buildDefaultsForProvider(selectedProvider);
  });
  const [revealedKeys, setRevealedKeys] = useState<Record<string, boolean>>({});
  const [customFields, setCustomFields] = useState<CustomFieldRow[]>(() => {
    if (!editingItem) return [];
    const schemaKeys = new Set((editingSchema?.fields ?? []).map((f) => f.key));
    return (editingItem.fieldKeys ?? [])
      .filter((k) => !schemaKeys.has(k))
      .map((k, idx) => ({
        id: `existing-${idx}-${k}`,
        key: k,
        value: typeof editingItem.metadata?.[k] === "string" ? String(editingItem.metadata[k]) : "",
        secret: looksSecretFieldKey(k),
        revealed: false,
      }));
  });
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (isEdit || defaultCollectionSlug) return;
    if (selectedProvider?.itemType === "integration") {
      setCollectionSlug("integrations");
    } else if (selectedProvider) {
      setCollectionSlug("default");
    }
  }, [isEdit, defaultCollectionSlug, selectedProvider]);

  const handleCategoryChange = (nextKey: string) => {
    setCategoryKey(nextKey);
    const cat = schemas.find((c) => c.key === nextKey);
    const nextProv = cat?.providers[0];
    setProviderId(nextProv?.id ?? "");
    setValues(buildDefaultsForProvider(nextProv));
    setRevealedKeys({});
  };

  const handleProviderChange = (nextProvId: string) => {
    setProviderId(nextProvId);
    const nextProv = selectedCategory?.providers.find((p) => p.id === nextProvId);
    setValues(buildDefaultsForProvider(nextProv));
    setRevealedKeys({});
  };

  const setFieldValue = (key: string, val: unknown) => {
    setValues((prev) => ({ ...prev, [key]: val }));
  };

  const addCustomField = () => {
    setCustomFields((prev) => [
      ...prev,
      {
        id: `cf-${Date.now()}-${prev.length}`,
        key: "",
        value: "",
        secret: true,
        revealed: false,
      },
    ]);
  };

  const updateCustomField = (id: string, patch: Partial<CustomFieldRow>) => {
    setCustomFields((prev) => prev.map((row) => (row.id === id ? { ...row, ...patch } : row)));
  };

  const removeCustomField = (id: string) => {
    setCustomFields((prev) => prev.filter((row) => row.id !== id));
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const trimmedName = name.trim();
    if (!trimmedName) return;

    setBusy(true);
    try {
      const payload: Record<string, unknown> = {};
      let hasPayloadChanges = false;

      if (selectedProvider) {
        for (const f of selectedProvider.fields) {
          const raw = values[f.key];
          const isSec = Boolean(f.secret || f.kind === "secret");
          if (f.kind === "boolean") {
            payload[f.key] = Boolean(raw);
            if (!isEdit || Boolean(raw) !== Boolean(editingItem?.metadata?.[f.key] ?? f.default)) {
              hasPayloadChanges = true;
            }
          } else if (f.kind === "number") {
            const str = String(raw ?? "").trim();
            if (str !== "") {
              const num = Number(str);
              if (!Number.isNaN(num)) {
                payload[f.key] = num;
                if (!isEdit || num !== Number(editingItem?.metadata?.[f.key])) {
                  hasPayloadChanges = true;
                }
              }
            }
          } else {
            const str = typeof raw === "string" ? raw.trim() : String(raw ?? "").trim();
            if (isEdit && isSec) {
              if (str !== "") {
                payload[f.key] = str;
                hasPayloadChanges = true;
              }
            } else if (str !== "") {
              payload[f.key] = str;
              if (!isEdit || str !== String(editingItem?.metadata?.[f.key] ?? "")) {
                hasPayloadChanges = true;
              }
            }
          }
        }
      }

      for (const cf of customFields) {
        const k = cf.key.trim();
        const v = cf.value.trim();
        if (!k) continue;
        if (isEdit && cf.secret && v === "") continue;
        if (v !== "") {
          payload[k] = v;
          hasPayloadChanges = true;
        }
      }

      let saved: VaultItemMeta;
      if (isEdit && editingItem) {
        saved = await api.updateVaultItem(editingItem.id, {
          name: trimmedName,
          isDefault: editingItem.providerType ? isDefault : undefined,
          payload: hasPayloadChanges || changeNote.trim() ? payload : undefined,
          changeNote: changeNote.trim() || undefined,
        });
      } else {
        const itemType = selectedProvider?.itemType || "kv_secret";
        const providerType = selectedProvider?.providerType;
        const provider = itemType === "integration" || providerType ? selectedProvider?.id : undefined;
        saved = await api.createVaultItem({
          collectionSlug,
          environment,
          folderPath: folderPath.trim() || "/",
          itemType,
          providerType,
          provider,
          keyName: keyName.trim() || undefined,
          name: trimmedName,
          isDefault: providerType ? isDefault : false,
          payload,
          changeNote: changeNote.trim() || undefined,
        });
      }

      toast.success(t("vault.itemSaved"));
      onSaved(saved);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : t("vault.loadFailed");
      toast.error(msg);
    } finally {
      setBusy(false);
    }
  };

  const renderSchemaField = (field: VaultSchemaField) => {
    const isSecret = Boolean(field.secret || field.kind === "secret");
    const isRevealed = Boolean(revealedKeys[field.key]);
    const val = values[field.key];
    const req = !isEdit && Boolean(field.required);
    const hint = isEdit && isSecret ? t("vault.secretKeepHint") : field.hint;

    if (field.kind === "boolean") {
      return (
        <Field key={field.key} label={field.label} hint={hint}>
          <label className="flex cursor-pointer items-center gap-2.5 text-xs text-foreground">
            <Switch
              checked={Boolean(val)}
              onCheckedChange={(checked) => setFieldValue(field.key, checked)}
              aria-label={field.label}
            />
            <span>{Boolean(val) ? t("vault.switchEnabled") : t("vault.switchDisabled")}</span>
          </label>
        </Field>
      );
    }

    if (field.kind === "textarea") {
      return (
        <Field key={field.key} label={field.label} hint={hint}>
          <Textarea
            rows={3}
            className="font-mono text-xs"
            value={String(val ?? "")}
            onChange={(e) => setFieldValue(field.key, e.target.value)}
            placeholder={isEdit && isSecret ? "••••••••" : ""}
            required={req}
          />
        </Field>
      );
    }

    return (
      <Field key={field.key} label={field.label} hint={hint}>
        <div className="relative flex items-center gap-1.5">
          <Input
            type={isSecret && !isRevealed ? "password" : field.kind === "number" ? "number" : field.kind === "url" ? "url" : "text"}
            autoComplete="off"
            className={isSecret || field.kind === "url" ? "font-mono text-xs" : "text-sm"}
            value={String(val ?? "")}
            onChange={(e) => setFieldValue(field.key, e.target.value)}
            placeholder={isEdit && isSecret ? "••••••••" : ""}
            required={req}
          />
          {isSecret && (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="shrink-0 px-2"
              aria-label={isRevealed ? t("vault.hide") : t("vault.show")}
              onClick={() => setRevealedKeys((prev) => ({ ...prev, [field.key]: !prev[field.key] }))}
            >
              {isRevealed ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
            </Button>
          )}
        </div>
      </Field>
    );
  };

  const envOptions = [
    { value: "default", label: t("vault.envDefault") },
    { value: "prod", label: t("vault.envProd") },
    { value: "staging", label: t("vault.envStaging") },
    { value: "dev", label: t("vault.envDev") },
  ].filter((opt) => (VAULT_ENVIRONMENTS as readonly string[]).includes(opt.value));

  const collectionOptions = collections.map((c) => ({ value: c.slug, label: `${c.name} (${c.slug})` }));
  if (collectionSlug && !collectionOptions.some((o) => o.value === collectionSlug)) {
    collectionOptions.unshift({ value: collectionSlug, label: collectionSlug });
  }

  return (
    <Modal
      wide
      title={isEdit ? t("vault.editItemTitle") : t("vault.createItemTitle")}
      onClose={onClose}
    >
      <form onSubmit={handleSubmit} className="space-y-4">
        {!isEdit && (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <Field label={t("vault.categorySelect")}>
              <Select
                aria-label={t("vault.categorySelect")}
                value={selectedCategory?.key ?? ""}
                onValueChange={handleCategoryChange}
                options={schemas.map((cat) => ({ value: cat.key, label: cat.label }))}
              />
            </Field>
            <Field label={t("vault.providerSelect")}>
              <Select
                aria-label={t("vault.providerSelect")}
                value={selectedProvider?.id ?? ""}
                onValueChange={handleProviderChange}
                options={(selectedCategory?.providers ?? []).map((p) => ({
                  value: p.id,
                  label: p.label,
                }))}
              />
            </Field>
          </div>
        )}

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field label={t("vault.itemName")}>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t("vault.itemNamePlaceholder")}
              required
              autoFocus
            />
          </Field>
          {!isEdit && (
            <Field label={t("vault.keyName")} hint={t("vault.keyNameHint")}>
              <Input
                className="font-mono text-xs"
                value={keyName}
                onChange={(e) => setKeyName(e.target.value)}
                placeholder={t("vault.keyNamePlaceholder")}
              />
            </Field>
          )}
        </div>

        {!isEdit && (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            <Field label={t("vault.collectionLabel")}>
              <Select
                aria-label={t("vault.collectionLabel")}
                value={collectionSlug}
                onValueChange={setCollectionSlug}
                options={collectionOptions}
              />
            </Field>
            <Field label={t("vault.environmentLabel")}>
              <Select
                aria-label={t("vault.environmentLabel")}
                value={environment}
                onValueChange={setEnvironment}
                options={envOptions}
              />
            </Field>
            <Field label={t("vault.folderPath")}>
              <Input
                className="font-mono text-xs"
                value={folderPath}
                onChange={(e) => setFolderPath(e.target.value)}
                placeholder={t("vault.folderPathPlaceholder")}
              />
            </Field>
          </div>
        )}

        {selectedProvider && (
          <div className="space-y-3 rounded-lg border border-border bg-well p-3.5">
            {selectedProvider.fields.map(renderSchemaField)}
          </div>
        )}

        <div className="space-y-2.5">
          <div className="flex items-center justify-between gap-2">
            <span className="text-xs font-medium text-foreground">{t("vault.customFieldsTitle")}</span>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={addCustomField}
              className="flex items-center gap-1 text-xs"
            >
              <Plus className="h-3.5 w-3.5" />
              {t("vault.addCustomField")}
            </Button>
          </div>
          {customFields.map((cf) => (
            <div key={cf.id} className="grid grid-cols-1 gap-2 rounded-lg border border-border bg-well p-2.5 sm:grid-cols-[1fr_1fr_auto_auto] sm:items-center">
              <Input
                className="font-mono text-xs"
                value={cf.key}
                onChange={(e) => updateCustomField(cf.id, { key: e.target.value })}
                placeholder={t("vault.customFieldKey")}
                aria-label={t("vault.customFieldKey")}
              />
              <div className="flex items-center gap-1">
                <Input
                  type={cf.secret && !cf.revealed ? "password" : "text"}
                  className="font-mono text-xs"
                  value={cf.value}
                  onChange={(e) => updateCustomField(cf.id, { value: e.target.value })}
                  placeholder={isEdit && cf.secret ? "••••••••" : t("vault.customFieldValue")}
                  aria-label={t("vault.customFieldValue")}
                />
                {cf.secret && (
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="shrink-0 px-2"
                    aria-label={cf.revealed ? t("vault.hide") : t("vault.show")}
                    onClick={() => updateCustomField(cf.id, { revealed: !cf.revealed })}
                  >
                    {cf.revealed ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
                  </Button>
                )}
              </div>
              <label className="flex cursor-pointer items-center gap-1.5 text-xs text-muted-foreground">
                <Switch
                  checked={cf.secret}
                  onCheckedChange={(v) => updateCustomField(cf.id, { secret: v })}
                  aria-label={t("vault.customFieldSecret")}
                />
                <span>{t("vault.customFieldSecret")}</span>
              </label>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="text-danger-fg hover:text-danger-fg"
                aria-label={t("vault.removeField")}
                onClick={() => removeCustomField(cf.id)}
              >
                <Trash2 className="h-3.5 w-3.5" />
              </Button>
            </div>
          ))}
        </div>

        {(selectedProvider?.providerType || editingItem?.providerType) && (
          <label className="flex cursor-pointer items-center gap-2.5 text-xs text-foreground">
            <Switch
              checked={isDefault}
              onCheckedChange={setIsDefault}
              aria-label={t("vault.isDefaultLabel")}
            />
            <span>{t("vault.isDefaultLabel")}</span>
          </label>
        )}

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
          <Button type="submit" variant="primary" disabled={busy || !name.trim()}>
            {busy
              ? isEdit
                ? t("vault.saving")
                : t("vault.creating")
              : isEdit
                ? t("vault.save")
                : t("vault.create")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
