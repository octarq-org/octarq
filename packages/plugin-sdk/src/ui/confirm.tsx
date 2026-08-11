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

// Confirm is the app-wide "are you sure?" surface. It replaces native
// window.confirm() — which renders unstyled OS chrome, blocks the whole event
// loop, and can't say anything beyond one line of text — with the same glass
// dialog the rest of the app uses. Mirrors ./toast (which retired alert()):
// <ConfirmProvider> at the root, then either the useConfirm() hook or the
// imperative `confirmDialog` singleton anywhere below it.
//
// The API stays a near drop-in for the call sites it replaces:
//
//   if (!(await confirmDialog(t("settings.confirmRemoveMember")))) return;
//
// One hazard the native call didn't have: this returns a Promise, and a Promise
// is truthy. Forgetting `await` makes every guard pass — which is why the
// singleton is `confirmDialog`, not `confirm`. Shadowing the global would make
// a missing await invisible at the call site AND at review time.

export interface ConfirmOptions {
  message: ReactNode;
  title?: string;
  confirmLabel?: string;
  cancelLabel?: string;
  // danger styles the accept button as destructive. Default for a dialog that
  // guards a delete/revoke — pass false for a merely-consequential one.
  danger?: boolean;
}

type ConfirmFn = (opts: ConfirmOptions | string) => Promise<boolean>;

const ConfirmContext = createContext<ConfirmFn | null>(null);

// Fallback so useConfirm() outside a provider resolves false — i.e. "the user
// did not confirm" — instead of throwing. Silently proceeding would be the
// dangerous default here: these guards sit in front of destructive actions.
const DENY: ConfirmFn = () => Promise.resolve(false);

function normalize(opts: ConfirmOptions | string): ConfirmOptions {
  return typeof opts === "string" ? { message: opts } : opts;
}

// Imperative singleton — the ergonomic path for event handlers, matching
// `toast`. ConfirmProvider binds it on mount; before that (or with no provider)
// it resolves false. Each bundle has its own module instance, so the app and a
// separately-built portal don't share one.
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
  // Defaults are props rather than hardcoded strings so the host can feed them
  // from its i18n dictionary once, instead of every call site repeating labels.
  defaultTitle?: string;
  defaultConfirmLabel?: string;
  defaultCancelLabel?: string;
}) {
  const [pending, setPending] = useState<PendingConfirm | null>(null);
  // The resolver of the dialog currently on screen. Settling through this ref
  // (not through `pending`) means close-by-Escape, close-by-backdrop and the
  // buttons all funnel to one place, and a dialog can never resolve twice.
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
        // A second ask while one is open would orphan the first promise, so
        // deny the outgoing dialog before taking over the slot.
        activeRef.current?.(false);
        activeRef.current = resolve;
        setPending({ opts: normalize(opts), resolve });
      }),
    [],
  );

  // Bind the imperative singleton to this provider while mounted.
  useEffect(() => {
    dispatch = ask;
    return () => {
      if (dispatch === ask) dispatch = null;
    };
  }, [ask]);

  // Any still-open dialog must not leave its caller awaiting forever if the
  // provider unmounts mid-flight (route teardown, hot reload).
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
                variant={pending.opts.danger === false ? "primary" : "danger"}
                // The danger variant is text-only by design (it lives inline in
                // tables); as the accept button of a dialog it needs enough
                // weight to read as the primary action, hence the fill.
                className={pending.opts.danger === false ? undefined : "bg-rose-500/10 hover:bg-rose-500/20"} /* ui-color-ok */
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

// useConfirm returns the confirm function. Safe to call outside a provider —
// it resolves false, i.e. the guarded action does not run.
export function useConfirm(): ConfirmFn {
  return useContext(ConfirmContext) ?? DENY;
}
