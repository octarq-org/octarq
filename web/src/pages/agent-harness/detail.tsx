import { useEffect, useState } from "react";
import {
  Badge,
  Button,
  GlassCard,
  Modal,
  Skeleton,
  timeAgo,
  toast,
  useTranslation,
} from "@octarq/plugin-sdk";
import { api, type HarnessSpecContent, type HarnessSpecDetail } from "../../api";
import { EvidenceViewer } from "./evidence";

export interface HarnessDetailModalProps {
  open: boolean;
  onClose: () => void;
  specId: number | null;
  cachedDetail?: HarnessSpecDetail | null;
  onUseAsTemplate?: (spec: HarnessSpecContent) => void;
  onActivate?: (version: number, id: number) => void;
  onDiscard?: (version: number, id: number) => void;
  isActive?: boolean;
}

export function HarnessDetailModal({
  open,
  onClose,
  specId,
  cachedDetail,
  onUseAsTemplate,
  onActivate,
  onDiscard,
  isActive,
}: HarnessDetailModalProps) {
  const { t } = useTranslation();

  const [detail, setDetail] = useState<HarnessSpecDetail | null>(cachedDetail ?? null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (!open || !specId) {
      setDetail(null);
      setError(null);
      return;
    }

    if (cachedDetail && cachedDetail.id === specId) {
      setDetail(cachedDetail);
      return;
    }

    let active = true;
    setLoading(true);
    setError(null);

    api
      .agentHarnessSpec(specId)
      .then((res) => {
        if (active) {
          setDetail(res);
          setLoading(false);
        }
      })
      .catch((err: unknown) => {
        if (active) {
          const msg = err instanceof Error ? err.message : t("agentHarness.loadError");
          setError(msg);
          setLoading(false);
        }
      });

    return () => {
      active = false;
    };
  }, [open, specId, cachedDetail, t]);

  if (!open || !specId) return null;

  const handleCopyJson = () => {
    if (!detail) return;
    try {
      void navigator.clipboard.writeText(JSON.stringify(detail.spec, null, 2));
      setCopied(true);
      toast.success(t("agentHarness.copied"));
      setTimeout(() => setCopied(false), 2000);
    } catch {
      /* ignore */
    }
  };

  const modalTitle = t("agentHarness.specDetailTitle", {
    version: detail?.version ?? specId,
  });

  const getStatusBadge = () => {
    if (!detail) return null;
    switch (detail.status) {
      case "published":
        return <Badge variant="green">{t("agentHarness.statusPublished")}</Badge>;
      case "draft":
        return <Badge variant="amber">{t("agentHarness.statusDraft")}</Badge>;
      case "discarded":
        return <Badge variant="red">{t("agentHarness.statusDiscarded")}</Badge>;
      case "archived":
      default:
        return <Badge variant="neutral">{t("agentHarness.statusArchived")}</Badge>;
    }
  };

  return (
    <Modal
      title={modalTitle}
      onClose={onClose}
      className="max-w-2xl w-full p-6 space-y-5"
    >
      <div className="flex items-center gap-2 -mt-2">
        {getStatusBadge()}
        {isActive && (
          <Badge variant="indigo">{t("agentHarness.activeBadge")}</Badge>
        )}
        {detail?.fingerprint && (
          <span className="text-xs font-mono text-muted-foreground ml-auto" title={detail.fingerprint}>
            {detail.fingerprint.slice(0, 16)}…
          </span>
        )}
      </div>

      {detail?.status === "draft" && (
        <div className="p-2.5 rounded-xl border border-warning-border bg-warning-bg text-warning-fg text-xs font-medium">
          {t("agentHarness.draftNoticeDesc")}
        </div>
      )}

      {loading ? (
        <div className="space-y-3 py-4">
          <Skeleton className="h-6 w-1/3" />
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-16 w-full" />
        </div>
      ) : error ? (
        <div className="p-4 rounded-xl border border-danger-fg/30 bg-danger-fg/10 text-danger-fg text-sm">
          {error}
        </div>
      ) : detail ? (
        <div className="space-y-4 max-h-[65vh] overflow-y-auto pr-1">
          {/* Metadata Grid */}
          <div className="grid grid-cols-4 gap-2 text-xs">
            <GlassCard className="p-2.5">
              <span className="text-muted-foreground block text-[10px]">
                {t("agentHarness.createdBy")}
              </span>
              <span className="font-medium text-foreground">{detail.createdBy || "—"}</span>
            </GlassCard>
            <GlassCard className="p-2.5">
              <span className="text-muted-foreground block text-[10px]">
                {t("agentHarness.createdAt")}
              </span>
              <span className="font-medium text-foreground" title={detail.createdAt}>
                {detail.createdAt ? timeAgo(detail.createdAt) : "—"}
              </span>
            </GlassCard>
            <GlassCard className="p-2.5">
              <span className="text-muted-foreground block text-[10px]">
                {t("agentHarness.publishedAt")}
              </span>
              <span className="font-medium text-foreground" title={detail.publishedAt ?? undefined}>
                {detail.publishedAt ? timeAgo(detail.publishedAt) : t("agentHarness.notPublished")}
              </span>
            </GlassCard>
            <GlassCard className="p-2.5">
              <span className="text-muted-foreground block text-[10px]">
                {t("agentHarness.maxSteps")}
              </span>
              <span className="font-medium text-foreground">
                {t("agentHarness.stepsCount", { count: detail.spec.maxSteps })}
              </span>
            </GlassCard>
          </div>

          {/* Preamble */}
          <div className="space-y-1.5">
            <span className="text-xs font-semibold text-foreground">
              {t("agentHarness.preamble")}
            </span>
            <div className="rounded-xl border border-border bg-well p-3 font-mono text-xs whitespace-pre-wrap break-words max-h-48 overflow-y-auto text-foreground">
              {detail.spec.preamble}
            </div>
          </div>

          {/* Extra Rules */}
          <div className="space-y-1.5">
            <span className="text-xs font-semibold text-foreground">
              {t("agentHarness.extraRules")}
            </span>
            {detail.spec.extraRules && detail.spec.extraRules.length > 0 ? (
              <ul className="rounded-xl border border-border bg-well divide-y divide-border/50 text-xs font-mono">
                {detail.spec.extraRules.map((rule, idx) => (
                  <li key={idx} className="p-2.5 flex items-start gap-2">
                    <span className="text-muted-foreground select-none opacity-50">
                      {idx + 1}.
                    </span>
                    <span className="text-foreground">{rule}</span>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="text-xs text-muted-foreground italic">
                {t("agentHarness.noExtraRules")}
              </p>
            )}
          </div>

          {/* Tool Permissions */}
          <div className="space-y-1.5">
            <span className="text-xs font-semibold text-foreground">
              {t("agentHarness.toolAllow")}
            </span>
            {!detail.spec.toolAllow || detail.spec.toolAllow.length === 0 ? (
              <div className="flex items-center gap-2 p-2.5 rounded-xl border border-accent-border bg-accent-soft text-accent-fg text-xs">
                <span className="font-medium">
                  {t("agentHarness.toolsAllAllowed")}
                </span>
              </div>
            ) : (
              <div className="flex flex-wrap gap-1.5 p-2.5 rounded-xl border border-border bg-well">
                {detail.spec.toolAllow.map((tool) => (
                  <Badge key={tool} variant="neutral" className="font-mono text-xs">
                    {tool}
                  </Badge>
                ))}
              </div>
            )}
          </div>

          {/* Evidence */}
          {detail.evidence !== null && detail.evidence !== undefined && (
            <div className="space-y-1.5">
              <span className="text-xs font-semibold text-foreground">
                {t("agentHarness.evidence")}
              </span>
              <div className="p-3 rounded-xl border border-border bg-well max-h-40 overflow-y-auto">
                <EvidenceViewer evidence={detail.evidence} />
              </div>
            </div>
          )}
        </div>
      ) : null}

      <div className="flex items-center justify-between pt-3 border-t border-border">
        <div className="flex items-center gap-2">
          {detail && (
            <>
              <Button
                variant="outline"
                size="sm"
                onClick={handleCopyJson}
                className="text-xs"
              >
                {copied ? t("agentHarness.copied") : t("agentHarness.copyJson")}
              </Button>
              {onUseAsTemplate && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    onUseAsTemplate(detail.spec);
                    onClose();
                  }}
                  className="text-xs"
                >
                  {t("agentHarness.useAsTemplate")}
                </Button>
              )}
            </>
          )}
        </div>

        <div className="flex items-center gap-2">
          {detail && detail.status === "draft" && onDiscard && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                onClose();
                onDiscard(detail.version, detail.id);
              }}
              className="text-xs text-danger-fg border-danger-fg/30 hover:bg-danger-fg/10"
            >
              {t("agentHarness.discard")}
            </Button>
          )}
          {detail &&
            (detail.status === "draft" || detail.status === "archived") &&
            !isActive &&
            onActivate && (
              <Button
                variant="primary"
                size="sm"
                onClick={() => {
                  onClose();
                  onActivate(detail.version, detail.id);
                }}
                className="text-xs"
              >
                {t("agentHarness.activate")}
              </Button>
            )}
          <Button variant="ghost" size="sm" onClick={onClose}>
            {t("agentHarness.close")}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
