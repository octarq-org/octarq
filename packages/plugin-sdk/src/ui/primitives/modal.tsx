import { ReactNode } from "react";
import { Dialog } from "../base/dialog";

// Modal keeps its render-when-open API — callers mount it conditionally and pass
// onClose — but is backed by the Base UI Dialog wrapper, which supplies the
// focus trap, scroll lock, Escape handling, backdrop-click close, and aria
// wiring. It's always "open" while mounted; any close intent (Escape, backdrop,
// ✕) routes to onClose.
export function Modal({
  title,
  onClose,
  children,
  wide,
}: {
  title: string;
  onClose: () => void;
  children: ReactNode;
  wide?: boolean;
}) {
  return (
    <Dialog open onOpenChange={(next) => { if (!next) onClose(); }} title={title} wide={wide}>
      {children}
    </Dialog>
  );
}
