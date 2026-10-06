import { Button, buttonVariants, cn, Modal } from "@octarq/plugin-sdk";
import { Link } from "../api";
import { Check, Copy, Download } from "lucide-react";
import { useTranslation } from "../../../i18n";
import { LinkEditorForm } from "./LinkEditorForm";
import { StatsView } from "./StatsView";

export interface LinkModalsProps {
  createOpen: boolean;
  editingLink: Link | null;
  analyticsLink: Link | null;
  qrLink: Link | null;
  linkHostOptions: string[];
  copiedId: number | null;
  onCloseCreate: () => void;
  onCloseEdit: () => void;
  onCloseAnalytics: () => void;
  onCloseQr: () => void;
  onRefreshLinks: () => void;
  onCopyQrLink: (l: Link) => void;
  linkURL: (l: Link) => string;
}

export function LinkModals({
  createOpen,
  editingLink,
  analyticsLink,
  qrLink,
  linkHostOptions,
  copiedId,
  onCloseCreate,
  onCloseEdit,
  onCloseAnalytics,
  onCloseQr,
  onRefreshLinks,
  onCopyQrLink,
  linkURL,
}: LinkModalsProps) {
  const { t } = useTranslation();

  return (
    <>
      {/* Create New Link Modal */}
      {createOpen && (
        <Modal title={t("links.createNewLink")} onClose={onCloseCreate}>
          <div className="p-1">
            <LinkEditorForm
              link={null}
              hosts={linkHostOptions}
              onCancel={onCloseCreate}
              onSaved={() => {
                onCloseCreate();
                onRefreshLinks();
              }}
            />
          </div>
        </Modal>
      )}

      {/* Edit Link Modal */}
      {editingLink && (
        <Modal
          title={`${t("links.editLink")} — ${editingLink.title || `/${editingLink.slug}`}`}
          onClose={onCloseEdit}
        >
          <div className="p-1">
            <LinkEditorForm
              key={editingLink.id}
              link={editingLink}
              hosts={linkHostOptions}
              onCancel={onCloseEdit}
              onSaved={() => {
                onCloseEdit();
                onRefreshLinks();
              }}
            />
          </div>
        </Modal>
      )}

      {/* Analytics Modal */}
      {analyticsLink && (
        <Modal
          title={`${t("links.clickPerformanceAnalytics")} — ${analyticsLink.title || `/${analyticsLink.slug}`}`}
          onClose={onCloseAnalytics}
        >
          <div className="p-2 max-h-[80vh] overflow-y-auto">
            <StatsView link={analyticsLink} />
          </div>
        </Modal>
      )}

      {/* QR Code Modal */}
      {qrLink && (
        <Modal
          title={`${t("links.linkQrCode")} — ${qrLink.title || `/${qrLink.slug}`}`}
          onClose={onCloseQr}
        >
          <div className="flex flex-col items-center gap-5 py-4">
            <div className="bg-white p-5 rounded-2xl shadow-sm border border-foreground/[0.08]">
              <img
                src={`/api/links/${qrLink.id}/qr`}
                alt={t("links.qrAlt")}
                className="rounded-lg"
                width={220}
                height={220}
              />
            </div>
            <div className="font-mono text-xs text-foreground/75 bg-well px-3 py-1.5 rounded-lg border border-foreground/[0.05] truncate max-w-sm">
              {linkURL(qrLink)}
            </div>
            <div className="flex items-center gap-2">
              <Button
                variant="subtle"
                className="text-xs py-1.5 px-4 gap-1.5"
                onClick={() => onCopyQrLink(qrLink)}
              >
                {copiedId === qrLink.id ? (
                  <Check className="h-3.5 w-3.5 text-success-fg" />
                ) : (
                  <Copy className="h-3.5 w-3.5" />
                )}
                {copiedId === qrLink.id ? t("links.copied") : t("links.copyLink")}
              </Button>
              <a
                href={`/api/links/${qrLink.id}/qr`}
                download={`qr-${qrLink.slug}.png`}
                className={cn(buttonVariants({ variant: "primary" }), "text-xs py-1.5 px-4 gap-1.5")}
              >
                <Download className="h-3.5 w-3.5" />
                {t("links.downloadQrCode")}
              </a>
            </div>
          </div>
        </Modal>
      )}
    </>
  );
}
