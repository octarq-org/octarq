import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { AnimatePresence, motion } from "framer-motion";
import { cn } from "./cn";

// Toast is the app-wide transient-notification system. It replaces blocking
// native alert() calls with non-blocking, theme-consistent messages that stack
// bottom-right and auto-dismiss. Pure (React + framer-motion + glass classes),
// so both the host app and plugins share one notification surface via
// <ToastProvider> at the root and the useToast() hook anywhere below it.

export type ToastTone = "success" | "error" | "info";

interface ToastItem {
  id: number;
  tone: ToastTone;
  message: ReactNode;
}

interface ToastApi {
  success: (message: ReactNode) => void;
  error: (message: ReactNode) => void;
  info: (message: ReactNode) => void;
}

const ToastContext = createContext<ToastApi | null>(null);

// Fallback no-op API so calling useToast() outside a provider degrades to
// silence rather than throwing — keeps isolated component tests trivial.
const NOOP: ToastApi = { success: () => {}, error: () => {}, info: () => {} };

// Imperative singleton — the ergonomic path for replacing native alert() in
// event handlers, where wiring a hook per component is noise. ToastProvider
// binds `dispatch` on mount; before that (or with no provider) calls no-op.
// Each bundle has its own module instance, so the app and portal don't share.
let dispatch: ((tone: ToastTone, message: ReactNode) => void) | null = null;

export const toast: ToastApi = {
  success: (m) => dispatch?.("success", m),
  error: (m) => dispatch?.("error", m),
  info: (m) => dispatch?.("info", m),
};

const TONE_STYLES: Record<ToastTone, { ring: string; dot: string; label: string }> = {
  success: { ring: "ring-emerald-400/25", dot: "bg-emerald-400", label: "Success" },
  error: { ring: "ring-rose-400/25", dot: "bg-rose-400", label: "Error" },
  info: { ring: "ring-indigo-400/25", dot: "bg-indigo-400", label: "Info" },
};

const AUTO_DISMISS_MS = 4200;

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<ToastItem[]>([]);
  const idRef = useRef(0);

  const dismiss = useCallback((id: number) => {
    setToasts((list) => list.filter((t) => t.id !== id));
  }, []);

  const push = useCallback(
    (tone: ToastTone, message: ReactNode) => {
      const id = ++idRef.current;
      setToasts((list) => [...list, { id, tone, message }]);
      // Auto-dismiss; manual close still works and just removes early.
      setTimeout(() => dismiss(id), AUTO_DISMISS_MS);
    },
    [dismiss],
  );

  const api = useMemo<ToastApi>(
    () => ({
      success: (m) => push("success", m),
      error: (m) => push("error", m),
      info: (m) => push("info", m),
    }),
    [push],
  );

  // Bind the imperative singleton to this provider's dispatcher while mounted.
  useEffect(() => {
    dispatch = push;
    return () => {
      if (dispatch === push) dispatch = null;
    };
  }, [push]);

  return (
    <ToastContext.Provider value={api}>
      {children}
      {/* aria-live region: assertive so errors interrupt, but each toast is a
          discrete message so screen readers announce them one at a time. */}
      <div
        aria-live="assertive"
        aria-atomic="false"
        className="pointer-events-none fixed bottom-4 right-4 z-[100] flex w-[min(22rem,calc(100vw-2rem))] flex-col gap-2"
      >
        <AnimatePresence initial={false}>
          {toasts.map((t) => {
            const tone = TONE_STYLES[t.tone];
            return (
              <motion.div
                key={t.id}
                layout
                initial={{ opacity: 0, y: 12, scale: 0.96 }}
                animate={{ opacity: 1, y: 0, scale: 1 }}
                exit={{ opacity: 0, x: 24, scale: 0.96 }}
                transition={{ type: "spring", stiffness: 500, damping: 40 }}
                className={cn(
                  "glass-strong pointer-events-auto flex items-start gap-3 rounded-xl px-3.5 py-3 shadow-[0_16px_48px_-12px_rgba(0,0,0,0.6)] ring-1 ring-inset",
                  tone.ring,
                )}
              >
                <span className={cn("mt-1.5 h-2 w-2 shrink-0 rounded-full", tone.dot)} />
                <span className="min-w-0 flex-1 break-words text-[13px] leading-snug text-white/90">
                  {t.message}
                </span>
                <button
                  onClick={() => dismiss(t.id)}
                  aria-label="Dismiss"
                  className="-mr-1 -mt-0.5 shrink-0 rounded-lg px-1.5 py-0.5 text-white/40 transition-colors hover:bg-white/10 hover:text-white"
                >
                  ×
                </button>
              </motion.div>
            );
          })}
        </AnimatePresence>
      </div>
    </ToastContext.Provider>
  );
}

// useToast returns the notification API. Safe to call outside a provider (no-op).
export function useToast(): ToastApi {
  return useContext(ToastContext) ?? NOOP;
}
