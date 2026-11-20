import { Component, createContext, ReactNode, Suspense, useContext, type ErrorInfo } from "react";
import { uiWidgets } from "./registry";
import { PluginGateContext } from "./PluginGateContext";

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
