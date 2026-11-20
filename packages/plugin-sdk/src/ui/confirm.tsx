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
import { Dialog } from "./base/dialog";
import { Button } from "./primitives/button";


export interface ConfirmOptions {
  message: ReactNode;
  title?: string;
  confirmLabel?: string;
  cancelLabel?: string;
  danger?: boolean;
}

type ConfirmFn = (opts: ConfirmOptions | string) => Promise<boolean>;

const ConfirmContext = createContext<ConfirmFn | null>(null);

const DENY: ConfirmFn = () => Promise.resolve(false);

function normalize(opts: ConfirmOptions | string): ConfirmOptions {
  return typeof opts === "string" ? { message: opts } : opts;
}

let dispatch: ConfirmFn | null = null;

export const confirmDialog: ConfirmFn = (opts) => (dispatch ?? DENY)(opts);

interface PendingConfirm {
  opts: ConfirmOptions;
  resolve: (ok: boolean) => void;
}

export function ConfirmProvider({
  children,
  defaultTitle = "Are you sure?",
  defaultConfirmLabel = "Confirm",
  defaultCancelLabel = "Cancel",
}: {
  children: ReactNode;
  defaultTitle?: string;
  defaultConfirmLabel?: string;
  defaultCancelLabel?: string;
}) {
  const [pending, setPending] = useState<PendingConfirm | null>(null);
  const activeRef = useRef<((ok: boolean) => void) | null>(null);

  const settle = useCallback((ok: boolean) => {
    const resolve = activeRef.current;
    activeRef.current = null;
    setPending(null);
    resolve?.(ok);
  }, []);

  const ask = useCallback<ConfirmFn>(
    (opts) =>
      new Promise<boolean>((resolve) => {
        activeRef.current?.(false);
        activeRef.current = resolve;
        setPending({ opts: normalize(opts), resolve });
      }),
    [],
  );

  useEffect(() => {
    dispatch = ask;
    return () => {
      if (dispatch === ask) dispatch = null;
    };
  }, [ask]);

  useEffect(() => () => activeRef.current?.(false), []);

  const value = useMemo(() => ask, [ask]);

  return (
    <ConfirmContext.Provider value={value}>
      {children}
      {pending && (
        <Dialog
          open
          onOpenChange={(next) => {
            if (!next) settle(false);
          }}
          title={pending.opts.title ?? defaultTitle}
        >
          <div className="space-y-5">
            <div className="text-sm leading-relaxed text-foreground/75">{pending.opts.message}</div>
            <div className="flex justify-end gap-2">
              <Button variant="ghost" onClick={() => settle(false)}>
                {pending.opts.cancelLabel ?? defaultCancelLabel}
              </Button>
              <Button
                variant={pending.opts.danger === false ? "primary" : "danger-solid"}
                autoFocus
                onClick={() => settle(true)}
              >
                {pending.opts.confirmLabel ?? defaultConfirmLabel}
              </Button>
            </div>
          </div>
        </Dialog>
      )}
    </ConfirmContext.Provider>
  );
}

export function useConfirm(): ConfirmFn {
  return useContext(ConfirmContext) ?? DENY;
}
