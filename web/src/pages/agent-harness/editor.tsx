import { useState } from "react";
import {
  Alert,
  Button,
  Field,
  Input,
  Modal,
  Switch,
  Textarea,
  toast,
  useTranslation,
} from "@octarq/plugin-sdk";
import { api, type HarnessSpecContent, type HarnessSpecDetail } from "../../api";

export interface HarnessEditorModalProps {
  open: boolean;
  onClose: () => void;
  onSuccess: (detail: HarnessSpecDetail) => void;
  initialSpec?: HarnessSpecContent | null;
}

export function HarnessEditorModal({
  open,
  onClose,
  onSuccess,
  initialSpec,
}: HarnessEditorModalProps) {
  const { t } = useTranslation();

  const [name, setName] = useState(initialSpec?.name ?? "default");
  const [preamble, setPreamble] = useState(initialSpec?.preamble ?? "");
  const [extraRules, setExtraRules] = useState<string[]>(
    initialSpec?.extraRules && initialSpec.extraRules.length > 0
      ? initialSpec.extraRules
      : [""]
  );
  const [isUnrestrictedTools, setIsUnrestrictedTools] = useState(
    !initialSpec?.toolAllow || initialSpec.toolAllow.length === 0
  );
  const [toolsInput, setToolsInput] = useState(
    initialSpec?.toolAllow && initialSpec.toolAllow.length > 0
      ? initialSpec.toolAllow.join(", ")
      : ""
  );
  const [maxSteps, setMaxSteps] = useState(initialSpec?.maxSteps ?? 6);

  const [submitting, setSubmitting] = useState(false);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);

  if (!open) return null;

  const handleRuleChange = (index: number, val: string) => {
    setExtraRules((prev) => {
      const next = [...prev];
      next[index] = val;
      return next;
    });
  };

  const handleAddRule = () => {
    setExtraRules((prev) => [...prev, ""]);
  };

  const handleRemoveRule = (index: number) => {
    setExtraRules((prev) => {
      const next = prev.filter((_, i) => i !== index);
      return next.length > 0 ? next : [""];
    });
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setErrorMsg(null);

    const cleanPreamble = preamble.trim();
    if (!cleanPreamble) {
      setErrorMsg(t("agentHarness.fieldPreamblePlaceholder"));
      return;
    }

    const cleanRules = extraRules.map((r) => r.trim()).filter((r) => r.length > 0);

    let tools: string[] = [];
    if (!isUnrestrictedTools) {
      tools = toolsInput
        .split(/[\n,]/)
        .map((s) => s.trim())
        .filter((s) => s.length > 0);
    }

    const payload: HarnessSpecContent = {
      version: 1, // Contract requires version: 1
      name: name.trim() || "default",
      preamble: cleanPreamble,
      extraRules: cleanRules,
      toolAllow: tools,
      maxSteps: Math.max(1, Number(maxSteps) || 1),
    };

    setSubmitting(true);
    try {
      const created = await api.createAgentHarnessDraft(payload);
      toast.success(t("agentHarness.stageSuccess"));
      onSuccess(created);
      onClose();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : t("agentHarness.stageFailed");
      setErrorMsg(msg);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal
      title={t("agentHarness.editorTitle")}
      onClose={onClose}
      className="max-w-2xl w-full p-6 space-y-5"
    >
      <div className="space-y-2 -mt-2">
        <p className="text-xs text-muted-foreground">
          {t("agentHarness.editorDesc")}
        </p>
        <div className="p-2.5 rounded-xl border border-warning-border bg-warning-bg text-warning-fg text-xs font-medium">
          {t("agentHarness.draftNoticeDesc")}
        </div>
      </div>

      <form onSubmit={handleSubmit} className="space-y-4 max-h-[65vh] overflow-y-auto pr-1">
        {errorMsg && (
          <Alert variant="danger" title={t("agentHarness.stageFailed")}>
            {errorMsg}
          </Alert>
        )}

        <div className="grid grid-cols-2 gap-4">
          <Field label={t("agentHarness.name")}>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t("agentHarness.namePlaceholder")}
              disabled={submitting}
              required
            />
          </Field>

          <Field label={t("agentHarness.maxSteps")} hint={t("agentHarness.maxStepsHelp")}>
            <Input
              type="number"
              min={1}
              max={8}
              value={maxSteps}
              onChange={(e) => setMaxSteps(Number(e.target.value))}
              disabled={submitting}
              required
            />
          </Field>
        </div>

        <Field label={t("agentHarness.preamble")}>
          <Textarea
            rows={5}
            value={preamble}
            onChange={(e) => setPreamble(e.target.value)}
            placeholder={t("agentHarness.fieldPreamblePlaceholder")}
            className="font-mono text-xs"
            disabled={submitting}
            required
          />
        </Field>

        {/* Extra rules */}
        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <label className="label">{t("agentHarness.extraRules")}</label>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={handleAddRule}
              disabled={submitting}
              className="text-xs py-0.5 px-2"
            >
              + {t("agentHarness.addRule")}
            </Button>
          </div>
          <div className="space-y-2">
            {extraRules.map((rule, idx) => (
              <div key={idx} className="flex items-center gap-2">
                <Input
                  value={rule}
                  onChange={(e) => handleRuleChange(idx, e.target.value)}
                  placeholder={t("agentHarness.rulePlaceholder")}
                  className="font-mono text-xs flex-1"
                  disabled={submitting}
                />
                {extraRules.length > 1 && (
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => handleRemoveRule(idx)}
                    disabled={submitting}
                    className="text-danger-fg text-xs px-2"
                    aria-label={t("agentHarness.removeRule")}
                  >
                    ✕
                  </Button>
                )}
              </div>
            ))}
          </div>
        </div>

        {/* Tool permissions */}
        <div className="space-y-3 rounded-xl border border-border bg-well p-3.5">
          <div className="flex items-center justify-between">
            <div>
              <span className="text-xs font-semibold text-foreground block">
                {t("agentHarness.toolAllow")}
              </span>
              <span className="text-[11px] text-muted-foreground block mt-0.5">
                {isUnrestrictedTools
                  ? t("agentHarness.toolAllowUnrestricted")
                  : t("agentHarness.toolAllowCustom")}
              </span>
            </div>
            <div className="flex items-center gap-2">
              <span className="text-xs text-muted-foreground">
                {t("agentHarness.toolAllowUnrestricted")}
              </span>
              <Switch
                checked={isUnrestrictedTools}
                onCheckedChange={setIsUnrestrictedTools}
                disabled={submitting}
              />
            </div>
          </div>

          {!isUnrestrictedTools && (
            <Field
              label={t("agentHarness.toolAllowCustom")}
              hint={t("agentHarness.toolAllowHelp")}
              className="mt-2"
            >
              <Input
                value={toolsInput}
                onChange={(e) => setToolsInput(e.target.value)}
                placeholder={t("agentHarness.toolAllowPlaceholder")}
                className="font-mono text-xs"
                disabled={submitting}
              />
            </Field>
          )}
        </div>

        <div className="flex justify-end gap-2 pt-3 border-t border-border">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={onClose}
            disabled={submitting}
          >
            {t("agentHarness.cancel")}
          </Button>
          <Button
            type="submit"
            variant="primary"
            size="sm"
            disabled={submitting || !preamble.trim()}
          >
            {submitting ? t("agentHarness.stagingBtn") : t("agentHarness.stageBtn")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
