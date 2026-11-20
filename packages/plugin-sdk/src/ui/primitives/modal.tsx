import { forwardRef, type ReactNode } from "react";
import { Dialog } from "../base/dialog";

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
