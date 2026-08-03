import { useEffect, useState } from "react";
import { Domain } from "../../../api";
import { dnsApi, DDNSToken, CreateDDNSTokenResult } from "../api";
import { GlassCard, Button, Modal, Field, Badge, Empty, timeAgo, toast, Alert, confirmDialog } from "../../../ui";
import { KeyRound, Plus, Trash2, Copy, Check, AlertTriangle } from "lucide-react";
import { useTranslation } from "../../../i18n";
import { roleSatisfies, useCurrentRole } from "../../../shell/role";

export function DDNSView({ domains }: { domains: Domain[] }) {
  const { t } = useTranslation();
  const { role, isInstanceAdmin } = useCurrentRole();
  const canManageDDNS = roleSatisfies("admin", role, isInstanceAdmin);
  const [tokens, setTokens] = useState<DDNSToken[]>([]);
  const [loading, setLoading] = useState(true);

  // New Token Modal State
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [domainId, setDomainId] = useState<number>(domains[0]?.id || 0);
  const [recordName, setRecordName] = useState("");
  const [recordType, setRecordType] = useState<"A" | "AAAA">("A");
  const [label, setLabel] = useState("");
  const [creating, setCreating] = useState(false);

  // Secret Modal State
  const [createdResult, setCreatedResult] = useState<CreateDDNSTokenResult | null>(null);
  const [copiedSecret, setCopiedSecret] = useState(false);
  const [copiedUrl, setCopiedUrl] = useState(false);

  function loadTokens() {
    setLoading(true);
    dnsApi
      .ddnsTokens()
      .then(setTokens)
      .catch((e: any) => toast.error(e.message || "Failed to load DDNS tokens"))
      .finally(() => setLoading(false));
  }

  useEffect(() => {
    loadTokens();
  }, []);

  useEffect(() => {
    if (domains.length > 0 && !domainId) {
      setDomainId(domains[0].id);
    }
  }, [domains]);

  // Auto-complete FQDN prefix when domain selection changes
  function handleDomainChange(newDomainId: number) {
    setDomainId(newDomainId);
    const dom = domains.find((d) => d.id === newDomainId);
    if (dom && (!recordName || recordName.endsWith(dom.name))) {
      setRecordName(`home.${dom.name}`);
    }
  }

  async function handleCreateToken(e: React.FormEvent) {
    e.preventDefault();
    if (!domainId || !recordName) return;

    setCreating(true);
    try {
      const res = await dnsApi.createDDNSToken({
        domainId,
        recordName: recordName.trim(),
        recordType,
        label: label.trim(),
      });
      setShowCreateModal(false);
      setCreatedResult(res);
      setRecordName("");
      setLabel("");
      loadTokens();
    } catch (err: any) {
      toast.error(err.message || "Failed to create DDNS token");
    } finally {
      setCreating(false);
    }
  }

  async function handleDeleteToken(id: number) {
    if (!(await confirmDialog(t("domains.revokeConfirm")))) {
      return;
    }
    try {
      await dnsApi.deleteDDNSToken(id);
      toast.success("DDNS token revoked");
      loadTokens();
    } catch (err: any) {
      toast.error(err.message || "Failed to revoke token");
    }
  }

  function copyText(text: string, setCopiedState: (v: boolean) => void) {
    navigator.clipboard.writeText(text);
    setCopiedState(true);
    setTimeout(() => setCopiedState(false), 2000);
  }

  const fullUpdateUrl = createdResult
    ? `${window.location.origin}${createdResult.updateUrl}`
    : "";

  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h2 className="text-lg font-bold text-foreground flex items-center gap-2">
            <KeyRound className="h-5 w-5 text-accent-fg" />
            {t("domains.ddnsTitle")}
          </h2>
          <p className="text-xs text-muted-foreground mt-1">
            {t("domains.ddnsDescription")}
          </p>
        </div>

        {canManageDDNS && (
          <Button
            variant="primary"
            onClick={() => {
              if (domains.length > 0) {
                const dom = domains[0];
                setDomainId(dom.id);
                if (!recordName) setRecordName(`home.${dom.name}`);
              }
              setCreatedResult(null);
              setShowCreateModal(true);
            }}
            className="gap-1.5 py-1.5 text-xs shrink-0 self-start sm:self-auto"
          >
            <Plus className="h-4 w-4" />
            {t("domains.newDdnsToken")}
          </Button>
        )}
      </div>

      {loading ? (
        <div className="py-8 text-center text-xs text-muted-foreground">
          {t("domains.loadingTokens")}
        </div>
      ) : tokens.length === 0 ? (
        <Empty>
          <KeyRound className="h-8 w-8 text-muted-foreground/60 mb-2" />
          <p className="text-sm text-muted-foreground">
            {t("domains.noDdnsTokens")}
          </p>
        </Empty>
      ) : (
        <div className="space-y-3">
          {tokens.map((tok) => {
            const dom = domains.find((d) => d.id === tok.domainId);
            return (
              <GlassCard
                key={tok.id}
                className="p-4 flex flex-col sm:flex-row sm:items-center justify-between gap-4"
              >
                <div className="space-y-1 min-w-0">
                  <div className="flex items-center gap-2 flex-wrap">
                    <span className="font-semibold text-sm text-foreground">
                      {tok.label || tok.recordName}
                    </span>
                    <Badge tone="cyan" className="font-mono text-[10px]">
                      {tok.recordType}
                    </Badge>
                    <span className="font-mono text-xs text-muted-foreground">
                      {tok.recordName}
                    </span>
                  </div>

                  <div className="flex items-center gap-4 text-xs text-muted-foreground flex-wrap">
                    <span>
                      {t("domains.lastIp")}:{" "}
                      <span className="font-mono text-foreground font-medium">
                        {tok.lastIp || "—"}
                      </span>
                    </span>
                    <span>
                      {t("domains.lastSeen")}:{" "}
                      <span className="text-foreground">
                        {tok.lastSeenAt ? timeAgo(tok.lastSeenAt) : t("domains.never")}
                      </span>
                    </span>
                    {dom && <span className="text-muted-foreground/70">{t("domains.zonePrefix")} {dom.name}</span>}
                  </div>
                </div>

                {canManageDDNS && (
                  <Button
                    variant="danger"
                    onClick={() => handleDeleteToken(tok.id)}
                    className="gap-1.5 py-1 text-xs shrink-0 self-start sm:self-center"
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                    {t("domains.revokeToken")}
                  </Button>
                )}
              </GlassCard>
            );
          })}
        </div>
      )}

      {/* Create Token Modal */}
      {showCreateModal && (
        <Modal
          title={t("domains.createDdnsToken")}
          onClose={() => setShowCreateModal(false)}
        >
          <form onSubmit={handleCreateToken} className="space-y-4 pt-2">
            <Field label={t("domains.selectDomain")}>
              <select
                value={domainId}
                onChange={(e) => handleDomainChange(Number(e.target.value))}
                className="input w-full"
                required
              >
                {domains.map((d) => (
                  <option key={d.id} value={d.id}>
                    {d.name}
                  </option>
                ))}
              </select>
            </Field>

            <Field label={t("domains.recordName")}>
              <input
                type="text"
                value={recordName}
                onChange={(e) => setRecordName(e.target.value)}
                placeholder="e.g. home.example.com"
                className="input w-full font-mono text-xs"
                required
              />
            </Field>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <Field label={t("domains.recordType")}>
                <select
                  value={recordType}
                  onChange={(e) => setRecordType(e.target.value as "A" | "AAAA")}
                  className="input w-full"
                >
                  <option value="A">A (IPv4)</option>
                  <option value="AAAA">AAAA (IPv6)</option>
                </select>
              </Field>

              <Field label={t("domains.label")}>
                <input
                  type="text"
                  value={label}
                  onChange={(e) => setLabel(e.target.value)}
                  placeholder="e.g. Home Router"
                  className="input w-full text-xs"
                />
              </Field>
            </div>

            <div className="flex justify-end gap-2 pt-2">
              <Button type="button" variant="ghost" onClick={() => setShowCreateModal(false)}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" variant="primary" disabled={creating}>
                {creating ? t("domains.loading") : "Generate Token"}
              </Button>
            </div>
          </form>
        </Modal>
      )}

      {/* Secret Display Modal (Shown ONCE) */}
      {createdResult && (
        <Modal
          title={t("domains.ddnsCreatedTitle")}
          onClose={() => setCreatedResult(null)}
        >
          <div className="space-y-4 pt-2">
            <Alert variant="warning" className="text-xs p-3 rounded-xl">
              <span>
                {t("domains.ddnsCreatedWarning")}
              </span>
            </Alert>

            <Field label={t("domains.secretLabel")}>
              <div className="flex items-center gap-2">
                <input
                  type="text"
                  readOnly
                  value={createdResult.secret}
                  className="input font-mono text-xs flex-1 bg-surface font-semibold text-accent-fg select-all"
                />
                <Button
                  variant="subtle"
                  onClick={() => copyText(createdResult.secret, setCopiedSecret)}
                  className="gap-1 py-1.5 text-xs shrink-0"
                >
                  {copiedSecret ? <Check className="h-3.5 w-3.5 text-success-fg" /> : <Copy className="h-3.5 w-3.5" />}
                  {copiedSecret ? t("domains.copied") : t("domains.copySecret")}
                </Button>
              </div>
            </Field>

            <Field label={t("domains.updateUrlLabel")}>
              <div className="flex items-center gap-2">
                <input
                  type="text"
                  readOnly
                  value={fullUpdateUrl}
                  className="input font-mono text-xs flex-1 bg-surface select-all text-muted-foreground"
                />
                <Button
                  variant="subtle"
                  onClick={() => copyText(fullUpdateUrl, setCopiedUrl)}
                  className="gap-1 py-1.5 text-xs shrink-0"
                >
                  {copiedUrl ? <Check className="h-3.5 w-3.5 text-success-fg" /> : <Copy className="h-3.5 w-3.5" />}
                  {copiedUrl ? t("domains.copied") : t("domains.copyUrl")}
                </Button>
              </div>
            </Field>

            <div className="flex justify-end pt-2">
              <Button variant="primary" onClick={() => setCreatedResult(null)}>
                {t("common.done")}
              </Button>
            </div>
          </div>
        </Modal>
      )}
    </div>
  );
}
