import {
  forwardRef,
  useCallback,
  useEffect,
  useImperativeHandle,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useTranslation } from "../i18n";
import {
  loadApp,
  type NsgoAppearance,
  type NsgoHostHandlers,
  type NsgoPlayer,
} from "./loader";

export interface NsgoMountHandle {
  hostEvent: (name: string, payload?: unknown) => void;
  setAppearance: (value: NsgoAppearance) => void;
  setLocale: (tag: string) => void;
  setFontScale: (scale: number) => void;
}

export interface NsgoMountProps {
  src: string;
  params?: Record<string, unknown>;
  appearance?: NsgoAppearance;
  locale?: string;
  fontScale?: number;
  handlers?: NsgoHostHandlers;
  onOpenURL?: (url: string) => void | Promise<void>;
  fallback?: ReactNode | ((error: Error) => ReactNode);
  onReady?: (player: NsgoPlayer) => void;
  className?: string;
  title?: string;
}

export type NsgoMountStatus = "loading" | "ready" | "error";

export interface NsgoBridge {
  containerRef: (node: HTMLDivElement | null) => void;
  status: NsgoMountStatus;
  error: Error | null;
  handle: NsgoMountHandle;
}

export function useNsgoBridge(props: NsgoMountProps): NsgoBridge {
  const { src } = props;
  const [node, setNode] = useState<HTMLDivElement | null>(null);
  const [status, setStatus] = useState<NsgoMountStatus>("loading");
  const [error, setError] = useState<Error | null>(null);
  const playerRef = useRef<NsgoPlayer | null>(null);
  const handlers = useRef(props.handlers);
  handlers.current = props.handlers;
  const onOpenURL = useRef(props.onOpenURL);
  onOpenURL.current = props.onOpenURL;
  const onReady = useRef(props.onReady);
  onReady.current = props.onReady;
  const params = useRef(props.params);
  params.current = props.params;

  useEffect(() => {
    if (!node) return undefined;
    let cancelled = false;
    setStatus("loading");
    setError(null);

    const host: NsgoHostHandlers = {
      call: (name, args) => handlers.current?.call?.(name, args),
      post: (name, args) => handlers.current?.post?.(name, args),
    };
    if (onOpenURL.current) {
      host.openURL = async (url) => {
        await onOpenURL.current?.(url);
        return true;
      };
    }

    void (async () => {
      try {
        const { loader, base, module } = await loadApp(src);
        if (cancelled) return;
        const player = await loader.mount(node, base, {
          params: params.current ?? {},
          appearance: props.appearance,
          locale: props.locale,
          fontScale: props.fontScale,
          host,
          module,
        });
        if (cancelled) return;
        playerRef.current = player;
        setStatus("ready");
        onReady.current?.(player);
      } catch (e) {
        if (cancelled) return;
        setStatus("error");
        setError(e instanceof Error ? e : new Error(String(e)));
      }
    })();

    return () => {
      cancelled = true;
      playerRef.current = null;
      if (node) node.replaceChildren();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [node, src]);

  useEffect(() => {
    if (props.appearance) playerRef.current?.setAppearance(props.appearance);
  }, [props.appearance, status]);

  useEffect(() => {
    if (props.locale !== undefined) playerRef.current?.setLocale(props.locale);
  }, [props.locale, status]);

  useEffect(() => {
    if (props.fontScale !== undefined) playerRef.current?.setFontScale(props.fontScale);
  }, [props.fontScale, status]);

  const handle = useRef<NsgoMountHandle>({
    hostEvent: (name, payload) => playerRef.current?.hostEvent(name, payload),
    setAppearance: (value) => playerRef.current?.setAppearance(value),
    setLocale: (tag) => playerRef.current?.setLocale(tag),
    setFontScale: (scale) => playerRef.current?.setFontScale(scale),
  }).current;

  const containerRef = useCallback((next: HTMLDivElement | null) => setNode(next), []);

  return { containerRef, status, error, handle };
}

export const NsgoMount = forwardRef<NsgoMountHandle, NsgoMountProps>(function NsgoMount(props, ref) {
  const { t } = useTranslation();
  const bridge = useNsgoBridge(props);
  useImperativeHandle(ref, () => bridge.handle, [bridge.handle]);

  const failed = bridge.status === "error";
  const fallback =
    typeof props.fallback === "function"
      ? props.fallback(bridge.error ?? new Error("The workbench did not load."))
      : props.fallback;

  return (
    <div className={props.className} aria-label={props.title} role="region" data-nsgo-status={bridge.status}>
      {failed ? (
        fallback ?? (
          <div role="alert" className="p-4 text-sm text-muted-foreground">
            {t("nsgo.failed", "The workbench did not load.")}{" "}
            {bridge.error ? bridge.error.message : null}
          </div>
        )
      ) : null}
      <div
        ref={bridge.containerRef}
        className="h-full w-full"
        hidden={failed}
      />
    </div>
  );
});
