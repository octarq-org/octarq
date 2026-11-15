// <ExtensionSlot name="..."/> — a named extension point the host app renders
// where plugins may contribute dashboard widgets (UIPlugin.widgets). It reads
// the registry (uiWidgets), so in a build that composes no widget-bearing
// plugin — the OSS build — it renders nothing at all.
//
// Each widget is isolated in its own error boundary + Suspense: a widget whose
// chunk fails to load or that throws during render silently disappears instead
// of breaking the host page. Lives in the contract layer (not ./ui) because it
// is registry-coupled and app-independent — it imports only React and the
// registry.
import { Component, createContext, ReactNode, Suspense, useContext, type ErrorInfo } from "react";
import { uiWidgets } from "./registry";
import { PluginGateContext } from "./PluginGateContext";

// Per-widget boundary: a crashing widget renders nothing — never the page's
// problem. (A neutral note would be noise on a dashboard; absence is the
// correct degrade for an optional widget.) The crash is logged in development
// so the plugin author sees it; production stays silent.
class WidgetBoundary extends Component<{ slot: string; children: ReactNode }, { failed: boolean }> {
  state = { failed: false };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  componentDidCatch(error: Error, info: ErrorInfo) {
    if (import.meta.env.DEV) {
      console.error(`[plugin-sdk] a widget in slot "${this.props.slot}" crashed and was hidden.`, error, info.componentStack);
    }
  }
  render() {
    return this.state.failed ? null : this.props.children;
  }
}

// The slot's context value, readable by any widget rendered inside the slot.
// The host passes a value typed by the slot's own contract; the widget asks for
// that type through the generic. A widget outside a slot gets null.
const WidgetContext = createContext<unknown>(null);

export function useExtensionSlotContext<T>(): T | null {
  return useContext(WidgetContext) as T | null;
}

function filterActiveWidgets(
  widgets: ReturnType<typeof uiWidgets>,
  disabledPlugins: Set<string>,
  loaded: boolean,
) {
  if (!loaded) return widgets;
  return widgets.filter((w) => {
    if (w.pluginName && disabledPlugins.has(w.pluginName)) return false;
    // A widget that declares dependencies is gone when any of them is off.
    if (w.requires?.some((name) => disabledPlugins.has(name))) return false;
    return true;
  });
}

export function useExtensionCount(name: string): number {
  const { disabledPlugins, loaded } = useContext(PluginGateContext);
  return filterActiveWidgets(uiWidgets(name), disabledPlugins, loaded).length;
}

export function ExtensionSlot({
  name,
  wrapper,
  context,
}: {
  name: string;
  wrapper?: (children: ReactNode) => ReactNode;
  // Handed to every widget in the slot through useExtensionSlotContext().
  context?: unknown;
}) {
  const { disabledPlugins, loaded } = useContext(PluginGateContext);
  const widgets = filterActiveWidgets(uiWidgets(name), disabledPlugins, loaded);
  if (widgets.length === 0) return null;
  const content = (
    <WidgetContext.Provider value={context ?? null}>
      {widgets.map((w, i) => {
        const Widget = w.Component;
        return (
          <WidgetBoundary key={`${name}:${i}`} slot={name}>
            <Suspense fallback={null}>
              <Widget />
            </Suspense>
          </WidgetBoundary>
        );
      })}
    </WidgetContext.Provider>
  );
  return wrapper ? <>{wrapper(content)}</> : content;
}
