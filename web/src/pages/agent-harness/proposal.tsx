import { useState, useRef } from "react";
import {
  Alert,
  Button,
  Field,
  Modal,
  Textarea,
  toast,
  useTranslation,
} from "@octarq/plugin-sdk";
import { api, type HarnessProposalInput, type HarnessSpecDetail } from "../../api";
import { UploadCloud } from "lucide-react";

export interface HarnessProposalModalProps {
  open: boolean;
  onClose: () => void;
  onSuccess: (detail: HarnessSpecDetail) => void;
}

export function HarnessProposalModal({
  open,
  onClose,
  onSuccess,
}: HarnessProposalModalProps) {
  const { t } = useTranslation();
  const fileInputRef = useRef<HTMLInputElement>(null);

  const [jsonContent, setJsonContent] = useState("");
  const [selectedFileName, setSelectedFileName] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);

  if (!open) return null;

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setErrorMsg(null);
    const file = e.target.files?.[0];
    if (!file) return;

    setSelectedFileName(file.name);
    const reader = new FileReader();
    reader.onload = (evt) => {
      const text = evt.target?.result;
      if (typeof text === "string") {
        setJsonContent(text);
      }
    };
    reader.onerror = () => {
      setErrorMsg(t("agentHarness.fileReadError"));
    };
    reader.readAsText(file);
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setErrorMsg(null);

    const raw = jsonContent.trim();
    if (!raw) {
      setErrorMsg(t("agentHarness.invalidJson"));
      return;
    }

    let parsed: unknown;
    try {
      parsed = JSON.parse(raw);
    } catch {
      setErrorMsg(t("agentHarness.invalidJson"));
      return;
    }

    if (!parsed || typeof parsed !== "object") {
      setErrorMsg(t("agentHarness.invalidJson"));
      return;
    }

    setSubmitting(true);
    try {
      const created = await api.importAgentHarnessProposal(parsed as HarnessProposalInput);
      toast.success(t("agentHarness.importProposalSuccess"));
      onSuccess(created);
      onClose();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : t("agentHarness.importProposalFailed");
      setErrorMsg(msg);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal
      title={t("agentHarness.importProposalTitle")}
      onClose={onClose}
      className="max-w-xl w-full p-6 space-y-4"
    >
      <p className="text-xs text-muted-foreground -mt-2">
        {t("agentHarness.importProposalDesc")}
      </p>

      <form onSubmit={handleSubmit} className="space-y-4">
        {errorMsg && (
          <Alert variant="danger" title={t("agentHarness.importProposalFailed")}>
            {errorMsg}
          </Alert>
        )}

        <div className="flex flex-col items-center justify-center border-2 border-dashed border-border rounded-xl p-4 bg-well text-center hover:bg-well transition-colors">
          <input
            ref={fileInputRef}
            type="file"
            accept=".json,application/json"
            onChange={handleFileChange}
            className="hidden"
            data-testid="proposal-file-input"
          />
          <UploadCloud className="h-8 w-8 text-muted-foreground mb-2" />
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => fileInputRef.current?.click()}
            disabled={submitting}
            className="text-xs"
          >
            {t("agentHarness.selectProposalFile")}
          </Button>
          <span className="text-[11px] text-muted-foreground mt-1">
            {selectedFileName || t("agentHarness.proposalFileHint")}
          </span>
        </div>

        <Field label={t("agentHarness.importProposal")}>
          <Textarea
            rows={7}
            value={jsonContent}
            onChange={(e) => {
              setJsonContent(e.target.value);
              setErrorMsg(null);
            }}
            placeholder={t("agentHarness.proposalJsonPlaceholder")}
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
            disabled={submitting || !jsonContent.trim()}
          >
            {submitting
              ? t("agentHarness.importingProposalBtn")
              : t("agentHarness.importProposalBtn")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
