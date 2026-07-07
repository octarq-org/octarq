import { ReactNode } from "react";
import { Dialog as BaseDialog } from "@base-ui/react/dialog";
import { cn } from "../cn";

// Dialog is a shadcn-style wrapper over Base UI's accessible Dialog primitive.
// Base UI gives us focus trapping, scroll locking, Escape-to-close, and the
// aria wiring for free — replacing the app's previous hand-rolled portal that
// re-implemented (some of) that by hand. The higher-level `Modal` in
// ../primitives adapts it to the app's `{ title, onClose }` render-when-open API.
//
// Composition follows Base UI's canonical minimal pattern (Backdrop + Popup as
// direct children of Portal); the Popup is fixed-positioned near the top to
// preserve led's existing modal placement, and caps its height so long forms
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
            "glass-strong modal-card fixed left-1/2 top-16 z-50 w-[calc(100%-2rem)] -translate-x-1/2",
            "max-h-[calc(100vh-8rem)] overflow-y-auto rounded-2xl p-5 outline-none",
            wide ? "max-w-3xl" : "max-w-md",
          )}
        >
          <div className="mb-4 flex items-center justify-between">
            <BaseDialog.Title className="font-display text-lg font-semibold text-white">
              {title}
            </BaseDialog.Title>
            <BaseDialog.Close className="btn-ghost rounded-xl px-2 py-1 text-white/50 hover:text-white">
              ✕
            </BaseDialog.Close>
          </div>
          {children}
        </BaseDialog.Popup>
      </BaseDialog.Portal>
    </BaseDialog.Root>
  );
}
