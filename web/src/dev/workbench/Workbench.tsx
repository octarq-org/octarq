import { useState } from "react";
import { CATALOG, CATALOG_GROUPS, type CatalogGroup } from "./catalog";
import { Inspector } from "./Inspector";
import { toggleTheme, useTheme } from "../../theme";

// The UI tuning workbench: a dev-only route that renders the SDK's real
// components next to the live token set, so a change can be judged against what
// actually ships instead of against a mock.
//
// Dev-only by construction: it is reached through a dynamic import behind
// `import.meta.env.DEV` in App.tsx, so a production build contains no workbench
// chunk at all (verified — the build output has no workbench asset).
//
// The rule that keeps it honest: this directory composes SDK components, it
// never defines one. web/scripts/lint-ui-surface.mjs fails the build if a
// component here re-uses an SDK export name.
export function Workbench() {
  const [selected, setSelected] = useState<string>(CATALOG[0].name);
  const [group, setGroup] = useState<CatalogGroup | "all">("all");
  const theme = useTheme();
  const entry = CATALOG.find((e) => e.name === selected);
  const visible = group === "all" ? CATALOG : CATALOG.filter((e) => e.group === group);

  return (
    <div className="flex h-screen flex-col bg-background text-foreground">
      <header className="flex shrink-0 items-center gap-3 border-b border-border px-4 py-2">
        <h1 className="font-display text-sm font-semibold">UI workbench</h1>
        <span className="rounded bg-muted px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
          dev only
        </span>
        <span className="text-xs text-muted-foreground">
          {CATALOG.length} components · every preview imports @octarq/plugin-sdk
        </span>
        <button
          onClick={toggleTheme}
          className="ml-auto rounded-md border border-border px-2.5 py-1 text-xs hover:bg-surface-hover"
        >
          {theme === "dark" ? "Light" : "Dark"} mode
        </button>
      </header>

      <div className="flex min-h-0 flex-1">
        <nav className="flex w-56 shrink-0 flex-col border-r border-border">
          <div className="flex flex-wrap gap-1 border-b border-border p-2">
            {(["all", ...CATALOG_GROUPS] as const).map((id) => (
              <button
                key={id}
                onClick={() => setGroup(id)}
                className={`rounded-md px-2 py-0.5 text-[11px] capitalize ${
                  group === id ? "bg-muted text-foreground" : "text-muted-foreground hover:bg-surface-hover"
                }`}
              >
                {id}
              </button>
            ))}
          </div>
          <ul className="min-h-0 flex-1 overflow-y-auto p-2">
            {visible.map((e) => (
              <li key={e.name}>
                <button
                  onClick={() => setSelected(e.name)}
                  className={`w-full rounded-md px-2 py-1 text-left text-xs ${
                    selected === e.name
                      ? "bg-info-bg text-info-fg"
                      : "text-foreground/75 hover:bg-surface-hover"
                  }`}
                >
                  {e.name}
                </button>
              </li>
            ))}
          </ul>
        </nav>

        <main className="min-w-0 flex-1 overflow-y-auto p-6">
          {entry ? (
            <>
              <div className="mb-4 flex items-baseline gap-3">
                <h2 className="font-display text-base font-semibold">{entry.name}</h2>
                <span className="text-xs text-muted-foreground">{entry.group}</span>
              </div>
              <div className="rounded-xl border border-border bg-background p-6">
                <entry.Component />
              </div>
            </>
          ) : (
            <p className="text-sm text-muted-foreground">Select a component.</p>
          )}
        </main>

        <Inspector entry={entry} />
      </div>
    </div>
  );
}
