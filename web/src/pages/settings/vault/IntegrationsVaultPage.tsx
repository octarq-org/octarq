import { useCallback, useEffect, useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";
import {
  Alert,
  Badge,
  Button,
  Field,
  Input,
  Modal,
  PageCard,
  PageEmpty,
  PageRows,
  PageSection,
  PageShell,
  Select,
  Separator,
  StatusBadge,
  Tabs,
  Textarea,
  confirmDialog,
  toast,
} from "@octarq/plugin-sdk";
import {
  AlertCircle,
  Copy,
  Download,
  Eye,
  FolderPlus,
  History,
  KeyRound,
  Pencil,
  Plus,
  RefreshCw,
  Search,
  Send,
  ShieldCheck,
  Star,
  Trash2,
  Upload,
} from "lucide-react";
import { api } from "../../../api";
import type {
  VaultCategorySchema,
  VaultCollection,
  VaultItemMeta,
} from "../../../api/types";
import { ListSkeleton } from "../../../components/ListSkeleton";
import { useTranslation } from "../../../i18n";
import { roleSatisfies, useCurrentRole } from "../../../shell/role";
import {
  INTEGRATION_CATEGORIES,
  findSchemaForItem,
  itemMatchesTab,
  resolveTabFromCategory,
  type VaultTab,
} from "./types";
import { VaultItemModal } from "./VaultItemModal";
import { VaultRevealModal } from "./VaultRevealModal";
import { VaultVersionsModal } from "./VaultVersionsModal";
import { VaultTotpModal } from "./VaultTotpModal";
import { VaultExportModal, VaultImportModal } from "./VaultImportExportModals";
import { VaultTokensPanel } from "./VaultTokensPanel";

export function IntegrationsVaultPage() {
  const { t } = useTranslation();
  const [searchParams] = useSearchParams();
  const { role, isInstanceAdmin } = useCurrentRole();
  const canManage = roleSatisfies("admin", role, isInstanceAdmin);

  const initialCategory = searchParams.get("category");
  const initialSelection = useMemo(
    () => resolveTabFromCategory(initialCategory),
    [initialCategory],
  );

  const [activeTab, setActiveTab] = useState<VaultTab>(initialSelection.tab);
  const [providerTypeFilter, setProviderTypeFilter] = useState<string>(
    initialSelection.providerType,
  );
  const [collectionFilter, setCollectionFilter] = useState<string>("");
  const [environmentFilter, setEnvironmentFilter] = useState<string>("");
  const [searchQuery, setSearchQuery] = useState<string>("");

  const [schemas, setSchemas] = useState<VaultCategorySchema[]>([]);
  const [collections, setCollections] = useState<VaultCollection[]>([]);
  const [items, setItems] = useState<VaultItemMeta[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [showCreateModal, setShowCreateModal] = useState(false);
  const [editingItem, setEditingItem] = useState<VaultItemMeta | null>(null);
  const [revealingItem, setRevealingItem] = useState<VaultItemMeta | null>(null);
  const [versionsItem, setVersionsItem] = useState<VaultItemMeta | null>(null);
  const [totpItem, setTotpItem] = useState<VaultItemMeta | null>(null);
  const [showImport, setShowImport] = useState(false);
  const [showExport, setShowExport] = useState(false);
  const [showNewCollection, setShowNewCollection] = useState(false);
  const [testingId, setTestingId] = useState<number | null>(null);

  useEffect(() => {
    const resolved = resolveTabFromCategory(initialCategory);
    setActiveTab(resolved.tab);
    setProviderTypeFilter(resolved.providerType);
  }, [initialCategory]);

  const loadMeta = useCallback(async () => {
    try {
      const [schemaList, colList] = await Promise.all([
        api.vaultSchemas(),
        api.vaultCollections(),
      ]);
      setSchemas(Array.isArray(schemaList) ? schemaList : []);
      setCollections(Array.isArray(colList) ? colList : []);
    } catch {
      // Errors on items fetch surface in the main error alert.
    }
  }, []);

  const loadItems = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const list = await api.vaultItems({
        collection: collectionFilter || undefined,
        environment: environmentFilter || undefined,
        q: searchQuery.trim() || undefined,
      });
      setItems(Array.isArray(list) ? list : []);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("vault.loadFailed"));
    } finally {
      setLoading(false);
    }
  }, [collectionFilter, environmentFilter, searchQuery, t]);

  useEffect(() => {
    void loadMeta();
  }, [loadMeta]);

  useEffect(() => {
    void loadItems();
  }, [loadItems]);

  const filteredItems = useMemo(
    () => items.filter((it) => itemMatchesTab(it, activeTab, providerTypeFilter)),
    [items, activeTab, providerTypeFilter],
  );

  const copyText = async (text: string, message: string) => {
    try {
      await navigator.clipboard.writeText(text);
      toast.success(message);
    } catch {
      toast.error(t("vault.copyFailed"));
    }
  };

  const handleTest = async (item: VaultItemMeta) => {
    setTestingId(item.id);
    try {
      const res = await api.testVaultItem(item.id);
      toast.success(res.message);
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : t("vault.loadFailed"));
    } finally {
      setTestingId(null);
    }
  };

  const handleSetDefault = async (item: VaultItemMeta) => {
    try {
      await api.setDefaultVaultItem(item.id);
      toast.success(t("vault.defaultUpdated"));
      await loadItems();
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : t("vault.loadFailed"));
    }
  };

  const handleDelete = async (item: VaultItemMeta) => {
    if (!(await confirmDialog(t("vault.confirmDeleteItem", { name: item.name })))) return;
    try {
      await api.deleteVaultItem(item.id);
      toast.success(t("vault.itemDeleted"));
      setItems((prev) => prev.filter((x) => x.id !== item.id));
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : t("vault.loadFailed"));
    }
  };

  const categoryLabels: Record<string, string> = {
    llm: t("vault.categoryLlm"),
    payment: t("vault.categoryPayment"),
    storage: t("vault.categoryStorage"),
    communication: t("vault.categoryCommunication"),
    dns: t("vault.categoryDns"),
  };

  const collectionOptions = [
    { value: "", label: t("vault.allCollections") },
    ...collections.map((c) => ({ value: c.slug, label: `${c.name} (${c.slug})` })),
  ];

  const envOptions = [
    { value: "", label: t("vault.allEnvironments") },
    { value: "default", label: t("vault.envDefault") },
    { value: "prod", label: t("vault.envProd") },
    { value: "staging", label: t("vault.envStaging") },
    { value: "dev", label: t("vault.envDev") },
  ];

  return (
    <PageShell
      title={t("vault.title")}
      description={t("vault.description")}
      action={
        canManage && (
          <div className="flex flex-wrap items-center gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => setShowImport(true)}
              className="flex items-center gap-1.5"
            >
              <Upload className="h-3.5 w-3.5" />
              <span>{t("vault.importBtn")}</span>
            </Button>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => setShowExport(true)}
              className="flex items-center gap-1.5"
            >
              <Download className="h-3.5 w-3.5" />
              <span>{t("vault.exportBtn")}</span>
            </Button>
            <Button
              type="button"
              variant="primary"
              size="sm"
              onClick={() => setShowCreateModal(true)}
              className="flex items-center gap-1.5"
            >
              <Plus className="h-3.5 w-3.5" />
              <span>{t("vault.newItem")}</span>
            </Button>
          </div>
        )
      }
    >
      <div className="mt-4 space-y-4">
        <Tabs
          value={activeTab}
          onValueChange={(val) => {
            setActiveTab(val as VaultTab);
            if (val !== "integrations") {
              setProviderTypeFilter("");
            }
          }}
          items={[
            { value: "all", label: t("vault.tabAll"), content: null },
            { value: "integrations", label: t("vault.tabIntegrations"), content: null },
            { value: "secrets", label: t("vault.tabSecrets"), content: null },
            { value: "logins", label: t("vault.tabLogins"), content: null },
            { value: "tokens", label: t("vault.tabTokens"), content: null },
          ]}
        />

        {activeTab !== "tokens" && (
          <>
            <PageCard className="mt-0 p-4">
              <div className="grid grid-cols-1 gap-3 md:grid-cols-[1fr_auto_1fr_auto]">
                <div className="flex items-center gap-2">
                  <div className="min-w-0 flex-1">
                    <Select
                      aria-label={t("vault.collectionLabel")}
                      value={collectionFilter}
                      onValueChange={setCollectionFilter}
                      options={collectionOptions}
                    />
                  </div>
                  {canManage && (
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={() => setShowNewCollection(true)}
                      className="flex shrink-0 items-center gap-1.5"
                    >
                      <FolderPlus className="h-3.5 w-3.5" />
                      <span>{t("vault.newCollection")}</span>
                    </Button>
                  )}
                </div>

                <div className="w-full md:w-44">
                  <Select
                    aria-label={t("vault.environmentLabel")}
                    value={environmentFilter}
                    onValueChange={setEnvironmentFilter}
                    options={envOptions}
                  />
                </div>

                <div className="relative md:col-span-2">
                  <Search className="pointer-events-none absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
                  <Input
                    className="pl-8 text-sm"
                    value={searchQuery}
                    onChange={(e) => setSearchQuery(e.target.value)}
                    placeholder={t("vault.searchPlaceholder")}
                    aria-label={t("vault.searchPlaceholder")}
                  />
                </div>
              </div>

              {activeTab === "integrations" && (
                <div className="mt-3 flex flex-wrap items-center gap-1.5 border-t border-border pt-3">
                  <Button
                    type="button"
                    size="sm"
                    variant={providerTypeFilter === "" ? "primary" : "outline"}
                    onClick={() => setProviderTypeFilter("")}
                  >
                    {t("vault.categoryAll")}
                  </Button>
                  {INTEGRATION_CATEGORIES.map((cat) => (
                    <Button
                      key={cat}
                      type="button"
                      size="sm"
                      variant={providerTypeFilter === cat ? "primary" : "outline"}
                      onClick={() => setProviderTypeFilter(cat)}
                    >
                      {categoryLabels[cat]}
                    </Button>
                  ))}
                </div>
              )}
            </PageCard>

            <PageSection className="mt-4">
              {loading ? (
                <PageCard className="mt-0 p-5">
                  <ListSkeleton rows={4} ariaLabel={t("vault.loadingItems")} />
                </PageCard>
              ) : error ? (
                <Alert
                  variant="danger"
                  icon={<AlertCircle className="h-5 w-5 shrink-0" />}
                  actions={
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={() => void loadItems()}
                      className="flex items-center gap-1.5 text-xs"
                    >
                      <RefreshCw className="h-3.5 w-3.5" />
                      <span>{t("common.retry")}</span>
                    </Button>
                  }
                >
                  <span className="text-xs">{error}</span>
                </Alert>
              ) : filteredItems.length === 0 ? (
                <PageEmpty
                  className="mt-0"
                  icon={<ShieldCheck className="h-5 w-5" />}
                  title={t("vault.emptyTitle")}
                  detail={t("vault.emptyDetail")}
                  action={
                    canManage ? (
                      <Button
                        type="button"
                        variant="primary"
                        size="sm"
                        onClick={() => setShowCreateModal(true)}
                        className="flex items-center gap-1.5"
                      >
                        <Plus className="h-3.5 w-3.5" />
                        <span>{t("vault.newItem")}</span>
                      </Button>
                    ) : undefined
                  }
                />
              ) : (
                <PageRows className="mt-0">
                  {filteredItems.map((item, idx) => {
                    const schema = findSchemaForItem(schemas, item);
                    const metaEntries = Object.entries(item.metadata ?? {}).slice(0, 4);
                    return (
                      <div key={item.id} data-testid={`vault-item-${item.id}`}>
                        {idx > 0 && <Separator />}
                        <div className="flex flex-col gap-3 px-4 py-3.5 text-sm lg:flex-row lg:items-center lg:justify-between">
                          <div className="min-w-0 flex-1 space-y-1.5">
                            <div className="flex flex-wrap items-center gap-2">
                              <span className="font-medium text-foreground">{item.name}</span>
                              <Badge tone="info">
                                {schema?.label ?? item.provider ?? item.itemType}
                              </Badge>
                              <Badge>
                                {t("vault.versionBadge", { version: item.currentVersion })}
                              </Badge>
                              <Badge tone="outline" className="font-mono">
                                {`${item.collectionSlug}/${item.environment}`}
                              </Badge>
                              {item.isDefault && (
                                <Badge tone="success">{t("vault.defaultBadge")}</Badge>
                              )}
                              <StatusBadge tone={item.configured ? "success" : "idle"}>
                                {item.configured ? t("vault.configured") : t("vault.unconfigured")}
                              </StatusBadge>
                            </div>

                            <div className="flex flex-wrap items-center gap-2">
                              <code className="rounded border border-border bg-well px-1.5 py-0.5 font-mono text-[11px] text-muted-foreground select-all">
                                {item.ref}
                              </code>
                              <Button
                                type="button"
                                variant="ghost"
                                size="sm"
                                className="h-6 px-1.5 text-[11px] text-muted-foreground"
                                aria-label={t("vault.copyRef")}
                                onClick={() => void copyText(item.ref, t("vault.refCopied"))}
                              >
                                <Copy className="h-3 w-3" />
                              </Button>
                              {item.fieldKeys && item.fieldKeys.length > 0 && (
                                <span className="font-mono text-[11px] text-muted-foreground">
                                  {`${t("vault.fieldsCount", { count: item.fieldKeys.length })} · ••••••••`}
                                </span>
                              )}
                            </div>

                            {metaEntries.length > 0 && (
                              <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                                {metaEntries.map(([k, v]) => (
                                  <span
                                    key={k}
                                    className="rounded border border-border bg-well px-1.5 py-0.5 font-mono text-[11px]"
                                  >
                                    {`${k}: ${String(v)}`}
                                  </span>
                                ))}
                              </div>
                            )}
                          </div>

                          <div className="flex flex-wrap shrink-0 items-center gap-1.5">
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

                            {item.itemType === "twofa_totp" && (
                              <Button
                                type="button"
                                variant="outline"
                                size="sm"
                                onClick={() => setTotpItem(item)}
                                className="flex items-center gap-1 text-xs"
                              >
                                <KeyRound className="h-3.5 w-3.5" />
                                <span>{t("vault.totp")}</span>
                              </Button>
                            )}

                            {schema?.testable && (
                              <Button
                                type="button"
                                variant="outline"
                                size="sm"
                                disabled={testingId === item.id}
                                onClick={() => void handleTest(item)}
                                className="flex items-center gap-1 text-xs"
                              >
                                <Send className="h-3.5 w-3.5" />
                                <span>{testingId === item.id ? t("vault.testing") : t("vault.test")}</span>
                              </Button>
                            )}

                            {canManage && item.providerType && !item.isDefault && (
                              <Button
                                type="button"
                                variant="outline"
                                size="sm"
                                onClick={() => void handleSetDefault(item)}
                                className="flex items-center gap-1 text-xs"
                              >
                                <Star className="h-3.5 w-3.5" />
                                <span>{t("vault.setDefault")}</span>
                              </Button>
                            )}

                            {canManage && (
                              <Button
                                type="button"
                                variant="outline"
                                size="sm"
                                onClick={() => setEditingItem(item)}
                                className="flex items-center gap-1 text-xs"
                              >
                                <Pencil className="h-3.5 w-3.5" />
                                <span>{t("vault.edit")}</span>
                              </Button>
                            )}

                            <Button
                              type="button"
                              variant="outline"
                              size="sm"
                              onClick={() => setVersionsItem(item)}
                              className="flex items-center gap-1 text-xs"
                            >
                              <History className="h-3.5 w-3.5" />
                              <span>{t("vault.versions")}</span>
                            </Button>

                            {canManage && (
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
                            )}
                          </div>
                        </div>
                      </div>
                    );
                  })}
                </PageRows>
              )}
            </PageSection>
          </>
        )}

        {activeTab === "tokens" && (
          <VaultTokensPanel collections={collections} canManage={canManage} />
        )}
      </div>

      {(showCreateModal || editingItem) && (
        <VaultItemModal
          schemas={schemas}
          collections={collections}
          editingItem={editingItem}
          defaultTab={activeTab}
          defaultProviderType={providerTypeFilter}
          defaultCollectionSlug={collectionFilter}
          defaultEnvironment={environmentFilter}
          onClose={() => {
            setShowCreateModal(false);
            setEditingItem(null);
          }}
          onSaved={() => {
            setShowCreateModal(false);
            setEditingItem(null);
            void loadItems();
            void loadMeta();
          }}
        />
      )}

      {revealingItem && (
        <VaultRevealModal
          item={revealingItem}
          onClose={() => setRevealingItem(null)}
        />
      )}

      {versionsItem && (
        <VaultVersionsModal
          item={versionsItem}
          onClose={() => setVersionsItem(null)}
          onRolledBack={() => void loadItems()}
        />
      )}

      {totpItem && (
        <VaultTotpModal
          item={totpItem}
          onClose={() => setTotpItem(null)}
        />
      )}

      {showImport && (
        <VaultImportModal
          collections={collections}
          defaultCollectionSlug={collectionFilter || "default"}
          defaultEnvironment={environmentFilter || "default"}
          onClose={() => setShowImport(false)}
          onImported={() => {
            void loadItems();
            void loadMeta();
          }}
        />
      )}

      {showExport && (
        <VaultExportModal
          collections={collections}
          defaultCollectionSlug={collectionFilter}
          defaultEnvironment={environmentFilter}
          onClose={() => setShowExport(false)}
        />
      )}

      {showNewCollection && (
        <NewCollectionModal
          onClose={() => setShowNewCollection(false)}
          onCreated={(col) => {
            setCollections((prev) => [...prev, col]);
            setCollectionFilter(col.slug);
            setShowNewCollection(false);
          }}
        />
      )}
    </PageShell>
  );
}

function NewCollectionModal({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated: (col: VaultCollection) => void;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [kind, setKind] = useState("project");
  const [description, setDescription] = useState("");
  const [busy, setBusy] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    setBusy(true);
    try {
      const col = await api.createVaultCollection({
        name: name.trim(),
        slug: slug.trim() || undefined,
        kind,
        description: description.trim() || undefined,
      });
      toast.success(t("vault.collectionCreated"));
      onCreated(col);
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : t("vault.loadFailed"));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal title={t("vault.createCollectionTitle")} onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-4">
        <Field label={t("vault.collectionName")}>
          <Input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={t("vault.collectionNamePlaceholder")}
            required
            autoFocus
          />
        </Field>
        <Field label={t("vault.collectionSlug")} hint={t("vault.collectionSlugHint")}>
          <Input
            className="font-mono text-xs"
            value={slug}
            onChange={(e) => setSlug(e.target.value)}
            placeholder={t("vault.collectionSlugPlaceholder")}
          />
        </Field>
        <Field label={t("vault.collectionKind")}>
          <Select
            aria-label={t("vault.collectionKind")}
            value={kind}
            onValueChange={setKind}
            options={[
              { value: "project", label: t("vault.kindProject") },
              { value: "vault", label: t("vault.kindVault") },
              { value: "integration", label: t("vault.kindIntegration") },
            ]}
          />
        </Field>
        <Field label={t("vault.collectionDescription")}>
          <Textarea
            rows={2}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />
        </Field>
        <div className="flex justify-end gap-2 border-t border-border pt-4">
          <Button type="button" variant="ghost" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button type="submit" variant="primary" disabled={busy || !name.trim()}>
            {busy ? t("vault.creating") : t("vault.create")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
