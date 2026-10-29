import { forwardRef, type ReactNode } from "react";
import { Dialog as BaseDialog } from "@base-ui/react/dialog";
import { cn } from "../cn";
import { useTranslation } from "../../i18n";

// Dialog is a shadcn-style wrapper over Base UI's accessible Dialog primitive.
// Base UI gives us focus trapping, scroll locking, Escape-to-close, and the
// aria wiring for free. The higher-level `Modal` in ../primitives adapts it to
// the app's `{ title, onClose }` render-when-open API.
//
// Composition follows Base UI's canonical minimal pattern (Backdrop + Popup as
// direct children of Portal); the Popup is fixed-positioned near the top to
// preserve octarq's existing modal placement, and caps its height so long forms
// scroll inside the card rather than the page.
//
// The ref lands on the popup card.
export interface DialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  wide?: boolean;
  children: ReactNode;
  className?: string;
}

export const Dialog = forwardRef<HTMLDivElement, DialogProps>(
  ({ open, onOpenChange, title, wide, children, className }, ref) => {
    // The close control is the only way out that a screen reader has to name,
    // so it follows the host language like every other string.
    const { t } = useTranslation();
    return (
      <BaseDialog.Root open={open} onOpenChange={(v) => onOpenChange(v)}>
        <BaseDialog.Portal>
          <BaseDialog.Backdrop className="fixed inset-0 z-50 bg-black/45 backdrop-blur-[3px] modal-overlay" />
          <BaseDialog.Popup
            ref={ref}
            className={cn(
              "glass-strong modal-card fixed z-50 overflow-y-auto p-6 outline-none",
              "inset-x-0 bottom-0 top-auto w-full max-h-[85vh] rounded-b-none rounded-t-2xl",
              "sm:inset-auto sm:left-1/2 sm:top-1/2 sm:bottom-auto sm:-translate-x-1/2 sm:-translate-y-1/2 sm:rounded-2xl sm:max-h-[calc(100vh-7rem)]",
              wide ? "sm:w-full sm:max-w-3xl" : "sm:w-full sm:max-w-lg",
              className,
            )}
          >
            <div className="mb-4 flex items-center justify-between">
              <BaseDialog.Title className="font-display text-[1.125rem] font-semibold tracking-[-0.02em] text-foreground">
                {title}
              </BaseDialog.Title>
              <BaseDialog.Close
                aria-label={t("common.close", "Close")}
                className="tap-expand-sm flex h-8 w-8 cursor-pointer items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-surface-hover hover:text-foreground"
              >
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" className="h-4 w-4" aria-hidden="true">
                  <path d="M18 6 6 18M6 6l12 12" />
                </svg>
              </BaseDialog.Close>
            </div>
            {children}
          </BaseDialog.Popup>
        </BaseDialog.Portal>
      </BaseDialog.Root>
    );
  },
);
Dialog.displayName = "Dialog";
