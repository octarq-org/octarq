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


export interface PasswordConfirmOptions {
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

const DENY: PasswordConfirmFn = () => Promise.resolve(null);

let dispatch: PasswordConfirmFn | null = null;

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

export function usePasswordConfirm(): PasswordConfirmFn {
  return useContext(PasswordConfirmContext) ?? DENY;
}
