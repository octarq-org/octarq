import { useState } from "react";
import {
  Alert,
  Button,
  Field,
  Input,
  Modal,
  Textarea,
  toast,
  useTranslation,
} from "@octarq/plugin-sdk";
import { api, type HarnessLesson } from "../../api";

export interface HarnessLessonModalProps {
  open: boolean;
  onClose: () => void;
  onSuccess: (lesson: HarnessLesson) => void;
}

export function HarnessLessonModal({
  open,
  onClose,
  onSuccess,
}: HarnessLessonModalProps) {
  const { t } = useTranslation();

  const [scope, setScope] = useState("");
  const [note, setNote] = useState("");
  const [evidenceJson, setEvidenceJson] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);

  if (!open) return null;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setErrorMsg(null);

    const cleanScope = scope.trim();
    const cleanNote = note.trim();
    if (!cleanScope || !cleanNote) return;

    let parsedEvidence: unknown = undefined;
    if (evidenceJson.trim()) {
      try {
        parsedEvidence = JSON.parse(evidenceJson.trim());
      } catch {
        setErrorMsg(t("agentHarness.invalidJson"));
        return;
      }
    }

    setSubmitting(true);
    try {
      const created = await api.createAgentHarnessLesson({
        scope: cleanScope,
        note: cleanNote,
        evidence: parsedEvidence,
      });
      toast.success(t("agentHarness.createLessonSuccess"));
      onSuccess(created);
      onClose();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : t("agentHarness.createLessonFailed");
      setErrorMsg(msg);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal
      title={t("agentHarness.addLessonTitle")}
      onClose={onClose}
      className="max-w-xl w-full p-6 space-y-4"
    >
      <p className="text-xs text-muted-foreground -mt-2">
        {t("agentHarness.addLessonDesc")}
      </p>

      <form onSubmit={handleSubmit} className="space-y-4">
        {errorMsg && (
          <Alert variant="danger" title={t("agentHarness.createLessonFailed")}>
            {errorMsg}
          </Alert>
        )}

        <Field label={t("agentHarness.lessonScope")}>
          <Input
            value={scope}
            onChange={(e) => setScope(e.target.value)}
            placeholder={t("agentHarness.lessonScopePlaceholder")}
            disabled={submitting}
            required
          />
        </Field>

        <Field label={t("agentHarness.lessonNote")}>
          <Textarea
            rows={4}
            value={note}
            onChange={(e) => setNote(e.target.value)}
            placeholder={t("agentHarness.lessonNotePlaceholder")}
            disabled={submitting}
            required
          />
        </Field>

        <Field label={t("agentHarness.lessonEvidence")}>
          <Textarea
            rows={3}
            value={evidenceJson}
            onChange={(e) => {
              setEvidenceJson(e.target.value);
              setErrorMsg(null);
            }}
            placeholder={t("agentHarness.lessonEvidencePlaceholder")}
            className="font-mono text-xs"
            disabled={submitting}
          />
        </Field>

        <div className="flex justify-end gap-2 pt-2 border-t border-border">
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
            disabled={submitting || !scope.trim() || !note.trim()}
          >
            {submitting ? t("agentHarness.creatingLessonBtn") : t("agentHarness.createLessonBtn")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
