import { forwardRef, type ReactNode } from "react";
import { Dialog } from "../base/dialog";

// Modal keeps its render-when-open API — callers mount it conditionally and pass
// onClose — but is backed by the Base UI Dialog wrapper, which supplies the
// focus trap, scroll lock, Escape handling, backdrop-click close, and aria
// wiring. It's always "open" while mounted; any close intent (Escape, backdrop,
// ✕) routes to onClose.
//
// The ref lands on the dialog card (the popup), which is the element a caller
// would measure or focus.
export interface ModalProps {
  title: string;
  onClose: () => void;
  children: ReactNode;
  wide?: boolean;
  className?: string;
}

export const Modal = forwardRef<HTMLDivElement, ModalProps>(
  ({ title, onClose, children, wide, className }, ref) => (
    <Dialog
      ref={ref}
      open
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
      title={title}
      wide={wide}
      className={className}
    >
      {children}
    </Dialog>
  ),
);
Modal.displayName = "Modal";
