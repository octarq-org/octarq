import type { ReactNode } from "react";
import { Toaster, toast, type ToasterProps } from "sonner";


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

export function useToast(): typeof toast {
  return toast;
}
