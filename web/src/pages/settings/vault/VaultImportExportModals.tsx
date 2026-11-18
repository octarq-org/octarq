import { useState } from "react";
import {
  Alert,
  Button,
  Field,
  Input,
  Modal,
  Select,
  Textarea,
  toast,
} from "@octarq/plugin-sdk";
import { Copy, Download, ShieldAlert, Upload } from "lucide-react";
import { api } from "../../../api";
import type { VaultCollection, VaultExportResult } from "../../../api/types";
import { useTranslation } from "../../../i18n";
import { VAULT_ENVIRONMENTS } from "./types";

interface VaultImportModalProps {
  collections: VaultCollection[];
  defaultCollectionSlug?: string;
  defaultEnvironment?: string;
  onClose: () => void;
  onImported: () => void;
}

export function VaultImportModal({
  collections,
  defaultCollectionSlug,
  defaultEnvironment,
  onClose,
  onImported,
}: VaultImportModalProps) {
  const { t } = useTranslation();
  const [collectionSlug, setCollectionSlug] = useState(defaultCollectionSlug || "default");
  const [environment, setEnvironment] = useState(defaultEnvironment || "default");
  const [folderPath, setFolderPath] = useState("/");
  const [format, setFormat] = useState<"env" | "json">("env");
  const [content, setContent] = useState("");
  const [busy, setBusy] = useState(false);

  const handleFile = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    const text = await file.text();
    setContent(text);
    if (file.name.endsWith(".json")) {
      setFormat("json");
    } else if (file.name.includes(".env")) {
      setFormat("env");
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!content.trim()) return;

    let jsonItems: Record<string, unknown> | undefined;
    if (format === "json") {
      try {
        const parsed = JSON.parse(content);
        if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
          toast.error(t("vault.invalidJson"));
          return;
        }
        jsonItems = parsed as Record<string, unknown>;
      } catch {
        toast.error(t("vault.invalidJson"));
        return;
      }
    }

    setBusy(true);
    try {
      const res = await api.importVaultSecrets({
        collectionSlug,
        environment,
        folderPath: folderPath.trim() || "/",
        format,
        ...(format === "env" ? { dotEnv: content } : { jsonItems }),
      });
      toast.success(t("vault.importedCount", { count: res.imported }));
      onImported();
      onClose();
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : t("vault.loadFailed"));
    } finally {
      setBusy(false);
    }
  };

  const envOptions = [
    { value: "default", label: t("vault.envDefault") },
    { value: "prod", label: t("vault.envProd") },
    { value: "staging", label: t("vault.envStaging") },
    { value: "dev", label: t("vault.envDev") },
  ].filter((o) => (VAULT_ENVIRONMENTS as readonly string[]).includes(o.value));

  const collectionOptions = collections.map((c) => ({ value: c.slug, label: `${c.name} (${c.slug})` }));
  if (!collectionOptions.some((c) => c.value === collectionSlug)) {
    collectionOptions.unshift({ value: collectionSlug, label: collectionSlug });
  }

  return (
    <Modal wide title={t("vault.importTitle")} onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-4">
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

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 sm:items-end">
          <Field label={t("vault.importFormat")}>
            <Select
              aria-label={t("vault.importFormat")}
              value={format}
              onValueChange={(v) => setFormat(v as "env" | "json")}
              options={[
                { value: "env", label: t("vault.formatEnv") },
                { value: "json", label: t("vault.formatJson") },
              ]}
            />
          </Field>
          <div className="mb-3">
            <label className="inline-flex cursor-pointer items-center gap-1.5 rounded-lg border border-border bg-card px-3 py-2 text-xs font-medium text-foreground hover:bg-surface-hover">
              <Upload className="h-3.5 w-3.5" />
              <span>{t("vault.uploadFile")}</span>
              <input
                type="file"
                accept=".env,.txt,.json,text/plain,application/json"
                className="sr-only"
                onChange={(e) => void handleFile(e)}
              />
            </label>
          </div>
        </div>

        <Field label={t("vault.importContent")}>
          <Textarea
            rows={8}
            className="font-mono text-xs"
            value={content}
            onChange={(e) => setContent(e.target.value)}
            placeholder={
              format === "env"
                ? t("vault.importPlaceholderEnv")
                : t("vault.importPlaceholderJson")
            }
            required
          />
        </Field>

        <div className="flex justify-end gap-2 border-t border-border pt-4">
          <Button type="button" variant="ghost" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button type="submit" variant="primary" disabled={busy || !content.trim()}>
            {busy ? t("vault.importing") : t("vault.importSubmit")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

interface VaultExportModalProps {
  collections: VaultCollection[];
  defaultCollectionSlug?: string;
  defaultEnvironment?: string;
  onClose: () => void;
}

export function VaultExportModal({
  collections,
  defaultCollectionSlug,
  defaultEnvironment,
  onClose,
}: VaultExportModalProps) {
  const { t } = useTranslation();
  const [collection, setCollection] = useState(defaultCollectionSlug || "");
  const [environment, setEnvironment] = useState(defaultEnvironment || "");
  const [format, setFormat] = useState<"env" | "json">("env");
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<VaultExportResult | null>(null);

  const handleExport = async () => {
    setBusy(true);
    try {
      const res = await api.exportVaultSecrets({
        collection: collection || undefined,
        environment: environment || undefined,
        format,
      });
      setResult(res);
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : t("vault.loadFailed"));
    } finally {
      setBusy(false);
    }
  };

  const formattedText = result
    ? result.format === "json"
      ? JSON.stringify(result.secrets ?? {}, null, 2)
      : result.dotEnv ?? ""
    : "";

  const copyOutput = async () => {
    try {
      await navigator.clipboard.writeText(formattedText);
      toast.success(t("vault.copied"));
    } catch {
      toast.error(t("vault.copyFailed"));
    }
  };

  const downloadOutput = () => {
    const ext = result?.format === "json" ? "json" : "env";
    const blob = new Blob([formattedText], { type: "text/plain;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `vault-${collection || "all"}-${environment || "all"}.${ext}`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  };

  const envOptions = [
    { value: "", label: t("vault.allEnvironments") },
    { value: "default", label: t("vault.envDefault") },
    { value: "prod", label: t("vault.envProd") },
    { value: "staging", label: t("vault.envStaging") },
    { value: "dev", label: t("vault.envDev") },
  ];

  const collectionOptions = [
    { value: "", label: t("vault.allCollections") },
    ...collections.map((c) => ({ value: c.slug, label: `${c.name} (${c.slug})` })),
  ];

  return (
    <Modal wide title={t("vault.exportTitle")} onClose={onClose}>
      <div className="space-y-4">
        <Alert variant="warning" icon={<ShieldAlert className="h-4 w-4 shrink-0" />}>
          <span className="text-xs">{t("vault.exportWarning")}</span>
        </Alert>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <Field label={t("vault.collectionLabel")}>
            <Select
              aria-label={t("vault.collectionLabel")}
              value={collection}
              onValueChange={setCollection}
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
          <Field label={t("vault.importFormat")}>
            <Select
              aria-label={t("vault.importFormat")}
              value={format}
              onValueChange={(v) => setFormat(v as "env" | "json")}
              options={[
                { value: "env", label: t("vault.formatEnv") },
                { value: "json", label: t("vault.formatJson") },
              ]}
            />
          </Field>
        </div>

        <div className="flex justify-end">
          <Button type="button" variant="primary" disabled={busy} onClick={() => void handleExport()}>
            {busy ? t("vault.exporting") : t("vault.exportFetch")}
          </Button>
        </div>

        {result && (
          <div className="space-y-3">
            <Textarea
              readOnly
              rows={8}
              className="font-mono text-xs"
              value={formattedText}
            />
            <div className="flex flex-wrap justify-end gap-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => void copyOutput()}
                className="flex items-center gap-1.5 text-xs"
              >
                <Copy className="h-3.5 w-3.5" />
                <span>{t("vault.copyOutput")}</span>
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={downloadOutput}
                className="flex items-center gap-1.5 text-xs"
              >
                <Download className="h-3.5 w-3.5" />
                <span>{t("vault.downloadFile")}</span>
              </Button>
            </div>
          </div>
        )}

        <div className="flex justify-end border-t border-border pt-4">
          <Button type="button" variant="ghost" onClick={onClose}>
            {t("common.close")}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
