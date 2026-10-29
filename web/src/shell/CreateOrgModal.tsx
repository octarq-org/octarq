import { Button, Input, Modal } from "@octarq/plugin-sdk";
import { useTranslation } from "../i18n";

export interface CreateOrgModalProps {
  open: boolean;
  newOrgName: string;
  onNewOrgNameChange: (name: string) => void;
  onSubmit: (e: React.FormEvent) => void;
  onClose: () => void;
}

export function CreateOrgModal({
  open,
  newOrgName,
  onNewOrgNameChange,
  onSubmit,
  onClose,
}: CreateOrgModalProps) {
  const { t } = useTranslation();

  if (!open) return null;

  return (
    <Modal title={t("app.createWorkspace")} onClose={onClose}>
      <form onSubmit={onSubmit} className="space-y-4">
        <div className="space-y-1.5">
          <label htmlFor="create-org-name" className="label">{t("app.workspaceName")}</label>
          <Input
            id="create-org-name"
            value={newOrgName}
            onChange={(e) => onNewOrgNameChange(e.target.value)}
            placeholder={t("app.workspaceNamePlaceholder")}
            autoFocus
          />
        </div>
        <div className="flex justify-end gap-2.5 pt-4 border-t border-border">
          <Button type="button" variant="ghost" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button type="submit" variant="primary" disabled={!newOrgName.trim()}>
            {t("app.createAndSwitch")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
