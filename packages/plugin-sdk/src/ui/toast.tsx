import type { ReactNode } from "react";
import { Toaster, toast, type ToasterProps } from "sonner";

// Toast is the app-wide transient-notification system powered by sonner.
// It provides rich, accessible, theme-consistent messages that stack
// bottom-right and auto-dismiss. Both the host app and plugins share this
// notification surface via <ToastProvider> (or <Toaster>) at the root
// and the imperative `toast` export or `useToast()` hook anywhere below it.

export { toast, Toaster, type ToasterProps };

export type ToastTone = "success" | "error" | "info" | "warning";

export interface ToastProviderProps extends ToasterProps {
  children?: ReactNode;
}

export function ToastProvider({
  children,
  position = "bottom-right",
  richColors = true,
  closeButton = true,
  ...props
}: ToastProviderProps) {
  return (
    <>
      {children}
      <Toaster
        position={position}
        richColors={richColors}
        closeButton={closeButton}
        {...props}
      />
    </>
  );
}

// useToast returns the notification API for backward compatibility.
export function useToast(): typeof toast {
  return toast;
}
