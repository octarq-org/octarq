import { ReactNode } from "react";
import { Dialog as BaseDialog } from "@base-ui/react/dialog";
import { cn } from "../cn";

// Dialog is a shadcn-style wrapper over Base UI's accessible Dialog primitive.
// Base UI gives us focus trapping, scroll locking, Escape-to-close, and the
// aria wiring for free. The higher-level `Modal` in ../primitives adapts it to
// the app's `{ title, onClose }` render-when-open API.
//
// Composition follows Base UI's canonical minimal pattern (Backdrop + Popup as
// direct children of Portal); the Popup is fixed-positioned near the top to
// preserve octarq's existing modal placement, and caps its height so long forms
// scroll inside the card rather than the page.
export function Dialog({
  open,
  onOpenChange,
  title,
  wide,
  children,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  wide?: boolean;
  children: ReactNode;
}) {
  return (
    <BaseDialog.Root open={open} onOpenChange={(v) => onOpenChange(v)}>
      <BaseDialog.Portal>
        <BaseDialog.Backdrop className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm modal-overlay" />
        <BaseDialog.Popup
          className={cn(
            "glass-strong modal-card fixed z-50 overflow-y-auto p-5 outline-none",
            "inset-x-0 bottom-0 top-auto w-full max-h-[85vh] rounded-b-none rounded-t-2xl",
            "sm:inset-auto sm:left-1/2 sm:top-16 sm:bottom-auto sm:-translate-x-1/2 sm:rounded-2xl sm:max-h-[calc(100vh-8rem)]",
            wide ? "sm:w-[80vw] sm:min-w-[60vw] sm:max-w-[80vw]" : "sm:w-[60vw] sm:min-w-[60vw] sm:max-w-[80vw]",
          )}
        >
          <div className="mb-4 flex items-center justify-between">
            <BaseDialog.Title className="font-display text-lg font-semibold text-foreground">
              {title}
            </BaseDialog.Title>
            <BaseDialog.Close
              aria-label="Close"
              className="btn-ghost rounded-xl px-2 py-1 text-foreground/50 hover:text-foreground"
            >
              ✕
            </BaseDialog.Close>
          </div>
          {children}
        </BaseDialog.Popup>
      </BaseDialog.Portal>
    </BaseDialog.Root>
  );
}
