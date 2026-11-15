// <NsgoMount /> — run an nsgo web app inside a React element.
//
// The app (`nsgo build -target web`) is a directory the daemon serves: its
// nsgo.json names the WebAssembly, and its loader exports `mount`. The host
// page supplies what the app cannot know on its own — startup parameters, the
// appearance and locale, and the answers to the app's `host.call` names — and
// gets the player back to push events and change settings.
//
// Two rules shape this component:
//   - The app is compiled once per page, however many mounts use it.
//   - A load that fails says so. A blank rectangle would read as "the workbench
//     is empty", which is a lie when the build is missing.
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
  // Sends the app an event. The payload must be JSON.
  hostEvent: (name: string, payload?: unknown) => void;
  setAppearance: (value: NsgoAppearance) => void;
  setLocale: (tag: string) => void;
  setFontScale: (scale: number) => void;
}

export interface NsgoMountProps {
  // The app's manifest, e.g. "/console/nsgo.json". Relative to the page.
  src: string;
  // Startup parameters, e.g. { orgId, embedded: true }. nsgo reads them once;
  // changing this object later has no effect — send an event instead.
  params?: Record<string, unknown>;
  appearance?: NsgoAppearance;
  // A BCP 47 tag, or "" for the browser's language.
  locale?: string;
  // A text-size multiplier, or 0 for the browser's.
  fontScale?: number;
  // Answers to the app's host calls and posts.
  handlers?: NsgoHostHandlers;
  // The app asked the host to open a URL. Omit to let the platform open it.
  onOpenURL?: (url: string) => void | Promise<void>;
  // What to render when the app cannot load. A function receives the error.
  fallback?: ReactNode | ((error: Error) => ReactNode);
  // Called once the app runs.
  onReady?: (player: NsgoPlayer) => void;
  className?: string;
  // The accessible name of the region.
  title?: string;
}

export type NsgoMountStatus = "loading" | "ready" | "error";

export interface NsgoBridge {
  containerRef: (node: HTMLDivElement | null) => void;
  status: NsgoMountStatus;
  error: Error | null;
  handle: NsgoMountHandle;
}

// useNsgoBridge is the engine behind <NsgoMount />: a component that needs its
// own layout can mount the app on any element and drive it directly.
export function useNsgoBridge(props: NsgoMountProps): NsgoBridge {
  const { src } = props;
  const [node, setNode] = useState<HTMLDivElement | null>(null);
  const [status, setStatus] = useState<NsgoMountStatus>("loading");
  const [error, setError] = useState<Error | null>(null);
  const playerRef = useRef<NsgoPlayer | null>(null);
  // Handlers and callbacks live in refs: an inline object on every render must
  // not remount the app.
  const handlers = useRef(props.handlers);
  handlers.current = props.handlers;
  const onOpenURL = useRef(props.onOpenURL);
  onOpenURL.current = props.onOpenURL;
  const onReady = useRef(props.onReady);
  onReady.current = props.onReady;
  // Startup parameters are fixed for the app's life, so the effect reads them
  // from a ref rather than depending on an object that changes every render.
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
      // The loader has no close in this version; dropping the element stops
      // the surface, and a later mount gets a fresh instance.
      if (node) node.replaceChildren();
    };
    // The app is mounted once per src+element. Settings changes go through the
    // effects below; parameters and handlers stay fixed by design.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [node, src]);

  // The host's appearance, locale and text size follow the page's.
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

// NsgoMount runs the app and fills the element. The element must have a size;
// give it the host layout's height.
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
        // An app that failed to load must not leave an empty frame around.
        hidden={failed}
      />
    </div>
  );
});
