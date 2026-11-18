import { useCallback, useEffect, useState } from "react";
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
  Select,
  Separator,
  StatusBadge,
  confirmDialog,
  toast,
} from "@octarq/plugin-sdk";
import { Copy, KeyRound, Plus, ShieldAlert, Trash2 } from "lucide-react";
import { api } from "../../../api";
import type { VaultCollection, VaultServiceToken } from "../../../api/types";
import { ListSkeleton } from "../../../components/ListSkeleton";
import { useTranslation } from "../../../i18n";
import { VAULT_ENVIRONMENTS } from "./types";

interface VaultTokensPanelProps {
  collections: VaultCollection[];
  canManage: boolean;
}

export function VaultTokensPanel({ collections, canManage }: VaultTokensPanelProps) {
  const { t } = useTranslation();
  const [tokens, setTokens] = useState<VaultServiceToken[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [createdPlaintext, setCreatedPlaintext] = useState<string | null>(null);

  const [name, setName] = useState("");
  const [collectionSlug, setCollectionSlug] = useState("default");
  const [environment, setEnvironment] = useState("default");
  const [folderPrefix, setFolderPrefix] = useState("/");
  const [permissions, setPermissions] = useState("read");
  const [expiresDays, setExpiresDays] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    api.vaultTokens()
      .then(setTokens)
      .catch((err: unknown) => setError(err instanceof Error ? err.message : t("vault.loadFailed")))
      .finally(() => setLoading(false));
  }, [t]);

  useEffect(() => {
    load();
  }, [load]);

  const collectionNameMap = new Map(collections.map((c) => [c.id, `${c.name} (${c.slug})`]));

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    setBusy(true);
    try {
      let expiresAt: string | undefined;
      const days = Number(expiresDays.trim());
      if (expiresDays.trim() && !Number.isNaN(days) && days > 0) {
        expiresAt = new Date(Date.now() + days * 86400_000).toISOString();
      }
      const res = await api.createVaultToken({
        name: name.trim(),
        collectionSlug,
        environment,
        folderPrefix: folderPrefix.trim() || "/",
        permissions,
        expiresAt,
      });
      setTokens((prev) => [res.token, ...prev]);
      setShowCreate(false);
      setCreatedPlaintext(res.plaintext);
      setName("");
      setExpiresDays("");
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : t("vault.loadFailed"));
    } finally {
      setBusy(false);
    }
  };

  const handleRevoke = async (tok: VaultServiceToken) => {
    if (!(await confirmDialog(t("vault.confirmRevokeToken", { name: tok.name })))) return;
    try {
      await api.revokeVaultToken(tok.id);
      toast.success(t("vault.tokenRevoked"));
      load();
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : t("vault.loadFailed"));
    }
  };

  const copyToken = async (val: string) => {
    try {
      await navigator.clipboard.writeText(val);
      toast.success(t("vault.copied"));
    } catch {
      toast.error(t("vault.copyFailed"));
    }
  };

  const envOptions = [
    { value: "default", label: t("vault.envDefault") },
    { value: "prod", label: t("vault.envProd") },
    { value: "staging", label: t("vault.envStaging") },
    { value: "dev", label: t("vault.envDev") },
  ].filter((o) => (VAULT_ENVIRONMENTS as readonly string[]).includes(o.value));

  const collectionOptions = collections.map((c) => ({ value: c.slug, label: `${c.name} (${c.slug})` }));
  if (!collectionOptions.some((o) => o.value === collectionSlug)) {
    collectionOptions.unshift({ value: collectionSlug, label: collectionSlug });
  }

  return (
    <PageSection
      title={t("vault.tokensTitle")}
      description={t("vault.tokensDesc")}
      action={
        canManage && (
          <Button
            type="button"
            variant="primary"
            size="sm"
            onClick={() => setShowCreate(true)}
            className="flex items-center gap-1.5"
          >
            <Plus className="h-3.5 w-3.5" />
            <span>{t("vault.createToken")}</span>
          </Button>
        )
      }
    >
      {loading ? (
        <PageCard className="mt-0 p-5">
          <ListSkeleton rows={3} ariaLabel={t("vault.loadingTokens")} />
        </PageCard>
      ) : error ? (
        <Alert variant="danger">
          <span className="text-xs">{error}</span>
        </Alert>
      ) : tokens.length === 0 ? (
        <PageEmpty
          className="mt-0"
          icon={<KeyRound className="h-5 w-5" />}
          title={t("vault.noTokens")}
          detail={t("vault.noTokensDetail")}
        />
      ) : (
        <PageRows className="mt-0">
          {tokens.map((tok, idx) => {
            const isRevoked = Boolean(tok.revokedAt);
            const isExpired = Boolean(!isRevoked && tok.expiresAt && new Date(tok.expiresAt).getTime() < Date.now());
            const statusTone = isRevoked ? "error" : isExpired ? "warning" : "success";
            const statusLabel = isRevoked
              ? t("vault.tokenStatusRevoked")
              : isExpired
                ? t("vault.tokenStatusExpired")
                : t("vault.tokenStatusActive");
            return (
              <div key={tok.id}>
                {idx > 0 && <Separator />}
                <div className="flex flex-col gap-3 px-4 py-3.5 text-sm sm:flex-row sm:items-center sm:justify-between">
                  <div className="min-w-0 flex-1 space-y-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="font-medium text-foreground">{tok.name}</span>
                      <Badge className="font-mono">{tok.tokenPrefix}…</Badge>
                      <StatusBadge tone={statusTone}>{statusLabel}</StatusBadge>
                    </div>
                    <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                      <span>{collectionNameMap.get(tok.collectionId) ?? `#${tok.collectionId}`}</span>
                      <span className="font-mono">
                        {t("vault.tokenScope", {
                          env: tok.environment,
                          path: tok.folderPrefix,
                          perms: tok.permissions,
                        })}
                      </span>
                      <span>
                        {tok.lastUsedAt
                          ? t("vault.tokenLastUsed", { date: new Date(tok.lastUsedAt).toLocaleDateString() })
                          : t("vault.tokenNeverUsed")}
                      </span>
                    </div>
                  </div>
                  {canManage && !isRevoked && (
                    <Button
                      type="button"
                      variant="danger"
                      size="sm"
                      onClick={() => void handleRevoke(tok)}
                      className="flex shrink-0 items-center gap-1 text-xs"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                      <span>{t("vault.revokeToken")}</span>
                    </Button>
                  )}
                </div>
              </div>
            );
          })}
        </PageRows>
      )}

      {showCreate && (
        <Modal title={t("vault.createToken")} onClose={() => setShowCreate(false)}>
          <form onSubmit={handleCreate} className="space-y-4">
            <Field label={t("vault.tokenName")}>
              <Input
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder={t("vault.tokenNamePlaceholder")}
                required
                autoFocus
              />
            </Field>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
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
            </div>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field label={t("vault.tokenFolderPrefix")}>
                <Input
                  className="font-mono text-xs"
                  value={folderPrefix}
                  onChange={(e) => setFolderPrefix(e.target.value)}
                  placeholder={t("vault.folderPathPlaceholder")}
                />
              </Field>
              <Field label={t("vault.tokenPermissions")}>
                <Select
                  aria-label={t("vault.tokenPermissions")}
                  value={permissions}
                  onValueChange={setPermissions}
                  options={[
                    { value: "read", label: t("vault.permRead") },
                    { value: "read_write", label: t("vault.permReadWrite") },
                  ]}
                />
              </Field>
            </div>
            <Field label={t("vault.tokenExpiresDays")}>
              <Input
                type="number"
                min={1}
                value={expiresDays}
                onChange={(e) => setExpiresDays(e.target.value)}
                placeholder={t("vault.tokenExpiresPlaceholder")}
              />
            </Field>
            <div className="flex justify-end gap-2 border-t border-border pt-4">
              <Button type="button" variant="ghost" onClick={() => setShowCreate(false)}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" variant="primary" disabled={busy || !name.trim()}>
                {busy ? t("vault.creating") : t("vault.create")}
              </Button>
            </div>
          </form>
        </Modal>
      )}

      {createdPlaintext && (
        <Modal title={t("vault.tokenCreatedTitle")} onClose={() => setCreatedPlaintext(null)}>
          <div className="space-y-4">
            <Alert variant="warning" icon={<ShieldAlert className="h-4 w-4 shrink-0" />}>
              <span className="text-xs">{t("vault.tokenCreatedWarning")}</span>
            </Alert>
            <div className="flex items-center gap-2 rounded-lg border border-border bg-well p-3">
              <code data-testid="created-service-token" className="min-w-0 flex-1 break-all font-mono text-xs text-foreground">
                {createdPlaintext}
              </code>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => void copyToken(createdPlaintext)}
                className="flex shrink-0 items-center gap-1 text-xs"
              >
                <Copy className="h-3.5 w-3.5" />
                <span>{t("vault.copyValue")}</span>
              </Button>
            </div>
            <Field label={t("vault.tokenCurlExample")}>
              <code className="block break-all rounded-lg border border-border bg-well p-3 font-mono text-[11px] text-muted-foreground">
                {`curl -H "Authorization: Bearer ${createdPlaintext}" "${window.location.origin}/api/vault/raw?format=env"`}
              </code>
            </Field>
            <div className="flex justify-end border-t border-border pt-4">
              <Button type="button" variant="primary" onClick={() => setCreatedPlaintext(null)}>
                {t("common.done")}
              </Button>
            </div>
          </div>
        </Modal>
      )}
    </PageSection>
  );
}
