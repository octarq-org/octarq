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
import { Field } from "./primitives/field";
import { Input } from "./primitives/input";

// Password confirmation ("sudo mode") is the gate a sensitive action puts in
// front of itself: changing the account email, disabling 2FA, revealing a
// secret. It is deliberately the same surface everywhere, for the same reason
// ./confirm is — a re-authentication prompt that looks different on each page is
// one an attacker can imitate, and one users learn to click through.
//
// It mirrors ./confirm exactly (provider + hook + imperative singleton), with
// one difference in the contract: it resolves the password the user typed, or
// null if they dismissed it. So the call site reads
//
//   const password = await confirmPassword({ message: t("…") });
//   if (password === null) return;              // dismissed
//   await api.changeEmail(newEmail, password);  // server verifies it
//
// The dialog never validates the password itself; it cannot. The server is the
// only thing that can say whether it was right, so a wrong one comes back as a
// failed request at the call site, not as an error in here.

export interface PasswordConfirmOptions {
  // Why the password is being asked for. Without it the prompt is just a
  // password box, which is exactly what a phishing overlay looks like.
  message?: ReactNode;
  title?: string;
  passwordLabel?: string;
  confirmLabel?: string;
  cancelLabel?: string;
}

type PasswordConfirmFn = (
  opts?: PasswordConfirmOptions,
) => Promise<string | null>;

const PasswordConfirmContext = createContext<PasswordConfirmFn | null>(null);

// Outside a provider this resolves null — "the user did not confirm" — so the
// guarded action does not run. Same reasoning as ./confirm's DENY.
const DENY: PasswordConfirmFn = () => Promise.resolve(null);

let dispatch: PasswordConfirmFn | null = null;

// Imperative singleton, the ergonomic path for event handlers. Named
// `confirmPassword` rather than `password` or `prompt` for the reason
// `confirmDialog` is not `confirm`: it returns a Promise, and shadowing a
// global would make a missing await invisible at the call site.
export const confirmPassword: PasswordConfirmFn = (opts) =>
  (dispatch ?? DENY)(opts);

interface PendingPasswordConfirm {
  opts: PasswordConfirmOptions;
}

export function PasswordConfirmProvider({
  children,
  defaultTitle = "Confirm your password",
  defaultMessage = "Enter your current password to continue.",
  defaultPasswordLabel = "Current Password",
  defaultConfirmLabel = "Confirm",
  defaultCancelLabel = "Cancel",
}: {
  children: ReactNode;
  // Props, not hardcoded strings, so the host feeds them from its dictionary
  // once instead of every call site repeating labels (see ./confirm).
  defaultTitle?: string;
  defaultMessage?: ReactNode;
  defaultPasswordLabel?: string;
  defaultConfirmLabel?: string;
  defaultCancelLabel?: string;
}) {
  const [pending, setPending] = useState<PendingPasswordConfirm | null>(null);
  const [value, setValue] = useState("");
  const activeRef = useRef<((password: string | null) => void) | null>(null);

  const settle = useCallback((password: string | null) => {
    const resolve = activeRef.current;
    activeRef.current = null;
    setPending(null);
    // The password lives in React state only while the dialog is open. Clearing
    // it on close keeps it out of a component that is still mounted and out of
    // any devtools snapshot taken afterwards.
    setValue("");
    resolve?.(password);
  }, []);

  const ask = useCallback<PasswordConfirmFn>(
    (opts) =>
      new Promise<string | null>((resolve) => {
        activeRef.current?.(null);
        activeRef.current = resolve;
        setValue("");
        setPending({ opts: opts ?? {} });
      }),
    [],
  );

  useEffect(() => {
    dispatch = ask;
    return () => {
      if (dispatch === ask) dispatch = null;
    };
  }, [ask]);

  // A provider unmounting mid-flight (route teardown, hot reload) must not leave
  // its caller awaiting forever.
  useEffect(() => () => activeRef.current?.(null), []);

  const context = useMemo(() => ask, [ask]);

  return (
    <PasswordConfirmContext.Provider value={context}>
      {children}
      {pending && (
        <Dialog
          open
          onOpenChange={(next) => {
            if (!next) settle(null);
          }}
          title={pending.opts.title ?? defaultTitle}
        >
          <form
            onSubmit={(e) => {
              e.preventDefault();
              if (value) settle(value);
            }}
            className="space-y-4"
          >
            <p className="text-sm leading-relaxed text-foreground/75">
              {pending.opts.message ?? defaultMessage}
            </p>
            <Field label={pending.opts.passwordLabel ?? defaultPasswordLabel}>
              <Input
                type="password"
                value={value}
                onChange={(e) => setValue(e.target.value)}
                placeholder="••••••••"
                autoComplete="current-password"
                autoFocus
                required
              />
            </Field>
            <div className="flex justify-end gap-2">
              <Button type="button" variant="ghost" onClick={() => settle(null)}>
                {pending.opts.cancelLabel ?? defaultCancelLabel}
              </Button>
              <Button type="submit" variant="primary" disabled={!value}>
                {pending.opts.confirmLabel ?? defaultConfirmLabel}
              </Button>
            </div>
          </form>
        </Dialog>
      )}
    </PasswordConfirmContext.Provider>
  );
}

// Safe to call outside a provider — it resolves null, i.e. the guarded action
// does not run.
export function usePasswordConfirm(): PasswordConfirmFn {
  return useContext(PasswordConfirmContext) ?? DENY;
}
