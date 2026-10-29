import { useMemo } from "react";
import { Badge, Button, GlassCard, Modal, useTranslation } from "@octarq/plugin-sdk";
import type { HarnessSpecDetail } from "../../api";
import { EvidenceViewer } from "./evidence";

export interface HarnessDiffModalProps {
  open: boolean;
  onClose: () => void;
  activeDetail: HarnessSpecDetail | null;
  targetDetail: HarnessSpecDetail | null;
  onActivate?: (version: number, id: number) => void;
}

export function HarnessDiffModal({
  open,
  onClose,
  activeDetail,
  targetDetail,
  onActivate,
}: HarnessDiffModalProps) {
  const { t } = useTranslation();

  const diffData = useMemo(() => {
    if (!targetDetail) return null;
    const active = activeDetail?.spec;
    const target = targetDetail.spec;

    if (!active) {
      return {
        hasActive: false as const,
        target,
      };
    }

    // Name diff
    const nameChanged = active.name !== target.name;

    // MaxSteps diff
    const maxStepsChanged = active.maxSteps !== target.maxSteps;

    // ToolAllow diff
    const activeUnrestricted = !active.toolAllow || active.toolAllow.length === 0;
    const targetUnrestricted = !target.toolAllow || target.toolAllow.length === 0;
    const toolAllowChanged =
      activeUnrestricted !== targetUnrestricted ||
      (!activeUnrestricted &&
        !targetUnrestricted &&
        (active.toolAllow.length !== target.toolAllow.length ||
          active.toolAllow.some((tool) => !target.toolAllow.includes(tool))));

    // Extra rules diff
    const activeRules = active.extraRules || [];
    const targetRules = target.extraRules || [];
    const addedRules = targetRules.filter((r) => !activeRules.includes(r));
    const removedRules = activeRules.filter((r) => !targetRules.includes(r));
    const unchangedRules = targetRules.filter((r) => activeRules.includes(r));
    const rulesChanged = addedRules.length > 0 || removedRules.length > 0;

    // Preamble diff
    const preambleChanged = active.preamble !== target.preamble;

    const isIdentical =
      !nameChanged && !maxStepsChanged && !toolAllowChanged && !rulesChanged && !preambleChanged;

    return {
      hasActive: true as const,
      nameChanged,
      maxStepsChanged,
      toolAllowChanged,
      activeUnrestricted,
      targetUnrestricted,
      addedRules,
      removedRules,
      unchangedRules,
      rulesChanged,
      preambleChanged,
      isIdentical,
    };
  }, [activeDetail, targetDetail]);

  if (!open || !targetDetail) return null;

  const modalTitle = t("agentHarness.diffTitle", {
    source: targetDetail.version,
    target: activeDetail ? activeDetail.version : t("agentHarness.noActiveSpec"),
  });

  const canActivate =
    onActivate &&
    (targetDetail.status === "draft" || targetDetail.status === "archived") &&
    (!activeDetail || activeDetail.id !== targetDetail.id);

  return (
    <Modal
      title={modalTitle}
      onClose={onClose}
      className="max-w-3xl w-full p-6 space-y-5"
    >
      <div className="flex items-center gap-2 -mt-2">
        <span className="text-xs font-mono text-muted-foreground" title={targetDetail.fingerprint}>
          {targetDetail.fingerprint.slice(0, 16)}…
        </span>
        <Badge
          variant={
            targetDetail.status === "published"
              ? "green"
              : targetDetail.status === "draft"
              ? "amber"
              : targetDetail.status === "discarded"
              ? "red"
              : "neutral"
          }
          className="text-[10px]"
        >
          {targetDetail.status === "published"
            ? t("agentHarness.statusPublished")
            : targetDetail.status === "draft"
            ? t("agentHarness.statusDraft")
            : targetDetail.status === "discarded"
            ? t("agentHarness.statusDiscarded")
            : t("agentHarness.statusArchived")}
        </Badge>
      </div>

      {!diffData || !diffData.hasActive ? (
        <div className="rounded-xl border border-border bg-well p-4 text-sm text-muted-foreground">
          {t("agentHarness.noActiveSpecDesc")}
        </div>
      ) : diffData.isIdentical ? (
        <div className="rounded-xl border border-border bg-well p-6 text-center text-sm text-muted-foreground">
          {t("agentHarness.diffIdentical")}
        </div>
      ) : (
        <div className="space-y-4 max-h-[60vh] overflow-y-auto pr-1">
          {/* Metadata & Numbers */}
          <div className="grid grid-cols-2 gap-3 text-xs">
            <GlassCard className="p-3">
              <span className="font-semibold text-muted-foreground block mb-1">
                {t("agentHarness.maxSteps")}
              </span>
              {diffData.maxStepsChanged ? (
                <div className="flex items-center gap-2">
                  <span className="line-through text-danger-fg">
                    {activeDetail?.spec.maxSteps}
                  </span>
                  <span>→</span>
                  <span className="font-bold text-success-fg">
                    {targetDetail.spec.maxSteps}
                  </span>
                </div>
              ) : (
                <span className="text-foreground">{targetDetail.spec.maxSteps}</span>
              )}
            </GlassCard>

            <GlassCard className="p-3">
              <span className="font-semibold text-muted-foreground block mb-1">
                {t("agentHarness.name")}
              </span>
              {diffData.nameChanged ? (
                <div className="flex items-center gap-2">
                  <span className="line-through text-danger-fg">
                    {activeDetail?.spec.name}
                  </span>
                  <span>→</span>
                  <span className="font-bold text-success-fg">
                    {targetDetail.spec.name}
                  </span>
                </div>
              ) : (
                <span className="text-foreground">{targetDetail.spec.name}</span>
              )}
            </GlassCard>
          </div>

          {/* Tool Allow Diff */}
          <GlassCard className="p-3 space-y-2 text-xs">
            <span className="font-semibold text-muted-foreground block">
              {t("agentHarness.toolAllow")}
            </span>
            <div className="grid grid-cols-2 gap-3">
              <div className="p-2.5 rounded bg-well border border-border">
                <span className="text-[11px] text-muted-foreground block mb-1.5 font-medium">
                  {t("agentHarness.before", { version: activeDetail?.version ?? 0 })}
                </span>
                {diffData.activeUnrestricted ? (
                  <Badge variant="indigo">{t("agentHarness.toolsAllAllowed")}</Badge>
                ) : (
                  <div className="flex flex-wrap gap-1">
                    {activeDetail?.spec.toolAllow.map((tool) => (
                      <Badge key={tool} variant="neutral">
                        {tool}
                      </Badge>
                    ))}
                  </div>
                )}
              </div>
              <div className="p-2.5 rounded bg-well border border-border">
                <span className="text-[11px] text-muted-foreground block mb-1.5 font-medium">
                  {t("agentHarness.after", { version: targetDetail.version })}
                </span>
                {diffData.targetUnrestricted ? (
                  <Badge variant="indigo">{t("agentHarness.toolsAllAllowed")}</Badge>
                ) : (
                  <div className="flex flex-wrap gap-1">
                    {targetDetail.spec.toolAllow.map((tool) => (
                      <Badge key={tool} variant="neutral">
                        {tool}
                      </Badge>
                    ))}
                  </div>
                )}
              </div>
            </div>
          </GlassCard>

          {/* Extra Rules Diff */}
          <GlassCard className="p-3 space-y-2 text-xs">
            <span className="font-semibold text-muted-foreground block">
              {t("agentHarness.extraRules")}
            </span>
            <div className="space-y-1.5">
              {diffData.addedRules.map((rule, idx) => (
                <div
                  key={`add-${idx}`}
                  className="flex items-start gap-2 bg-success-fg/10 border border-success-fg/20 p-2 rounded text-success-fg font-mono text-[11px]"
                >
                  <span className="font-bold select-none">+</span>
                  <span className="flex-1">{rule}</span>
                  <Badge variant="green" className="text-[10px] py-0 px-1.5">
                    {t("agentHarness.ruleAdded")}
                  </Badge>
                </div>
              ))}
              {diffData.removedRules.map((rule, idx) => (
                <div
                  key={`rem-${idx}`}
                  className="flex items-start gap-2 bg-danger-fg/10 border border-danger-fg/20 p-2 rounded text-danger-fg font-mono text-[11px]"
                >
                  <span className="font-bold select-none">-</span>
                  <span className="flex-1 line-through">{rule}</span>
                  <Badge variant="red" className="text-[10px] py-0 px-1.5">
                    {t("agentHarness.ruleRemoved")}
                  </Badge>
                </div>
              ))}
              {diffData.unchangedRules.map((rule, idx) => (
                <div
                  key={`unc-${idx}`}
                  className="flex items-start gap-2 bg-well p-2 rounded text-muted-foreground font-mono text-[11px]"
                >
                  <span className="select-none opacity-40">•</span>
                  <span className="flex-1">{rule}</span>
                </div>
              ))}
              {!diffData.rulesChanged && diffData.unchangedRules.length === 0 && (
                <p className="text-muted-foreground italic">
                  {t("agentHarness.noExtraRules")}
                </p>
              )}
            </div>
          </GlassCard>

          {/* Preamble Diff */}
          <GlassCard className="p-3 space-y-2 text-xs">
            <span className="font-semibold text-muted-foreground block">
              {t("agentHarness.preamble")}
            </span>
            <div className="grid grid-cols-2 gap-3 font-mono text-[11px]">
              <div className="p-2.5 rounded bg-well border border-border max-h-48 overflow-y-auto whitespace-pre-wrap break-words">
                <span className="text-[10px] font-sans text-muted-foreground block mb-1">
                  {t("agentHarness.before", { version: activeDetail?.version ?? 0 })}
                </span>
                <p className="text-muted-foreground">{activeDetail?.spec.preamble}</p>
              </div>
              <div className="p-2.5 rounded bg-well border border-border max-h-48 overflow-y-auto whitespace-pre-wrap break-words">
                <span className="text-[10px] font-sans text-muted-foreground block mb-1">
                  {t("agentHarness.after", { version: targetDetail.version })}
                </span>
                <p className={diffData.preambleChanged ? "text-foreground font-medium" : "text-muted-foreground"}>
                  {targetDetail.spec.preamble}
                </p>
              </div>
            </div>
          </GlassCard>

          {/* Evidence if present on target */}
          {targetDetail.evidence !== null && targetDetail.evidence !== undefined && (
            <GlassCard className="p-3 space-y-2 text-xs">
              <span className="font-semibold text-muted-foreground block">
                {t("agentHarness.evidence")}
              </span>
              <div className="p-2.5 rounded bg-well border border-border max-h-36 overflow-y-auto">
                <EvidenceViewer evidence={targetDetail.evidence} />
              </div>
            </GlassCard>
          )}
        </div>
      )}

      <div className="flex items-center justify-between pt-2 border-t border-border">
        <div>
          {canActivate && (
            <Button
              variant="primary"
              size="sm"
              onClick={() => {
                onClose();
                onActivate(targetDetail.version, targetDetail.id);
              }}
              className="text-xs"
            >
              {t("agentHarness.activate")}
            </Button>
          )}
        </div>
        <Button variant="outline" size="sm" onClick={onClose}>
          {t("agentHarness.close")}
        </Button>
      </div>
    </Modal>
  );
}
