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
import { Component, ReactNode, Suspense } from "react";
import { uiWidgets } from "./registry";

// Per-widget boundary: a crashing widget renders nothing — never the page's
// problem. (A neutral note would be noise on a dashboard; absence is the
// correct degrade for an optional widget.)
class WidgetBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  render() {
    return this.state.failed ? null : this.props.children;
  }
}

export function ExtensionSlot({ name }: { name: string }) {
  const widgets = uiWidgets(name);
  if (widgets.length === 0) return null;
  return (
    <>
      {widgets.map((w, i) => {
        const Widget = w.Component;
        return (
          <WidgetBoundary key={`${name}:${i}`}>
            <Suspense fallback={null}>
              <Widget />
            </Suspense>
          </WidgetBoundary>
        );
      })}
    </>
  );
}
