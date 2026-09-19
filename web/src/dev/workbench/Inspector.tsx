import { useCallback, useEffect, useMemo, useState } from "react";
import { SEED_TOKENS, TOKENS, readTokenValues, themeAlias, type TokenDef } from "@octarq/plugin-sdk";
import { useTheme } from "../../theme";
import type { CatalogEntry } from "./catalog";

// Reads a token's declaration in BOTH themes without leaving the page in the
// other mode. Toggling the class on <html> is what actually re-resolves the
// declarations, so there is no second source to keep in sync.
//
// WARNING: this writes <html>'s class list. Never call it from a MutationObserver
// on that element — the observer would see its own write and re-fire forever.
function readBothThemes(names: string[]): { light: Record<string, string>; dark: Record<string, string> } {
  const el = document.documentElement;
  const wasDark = el.classList.contains("dark");
  const light = readTokenValues(names);
  el.classList.add("dark");
  const dark = readTokenValues(names);
  if (!wasDark) el.classList.remove("dark");
  return { light, dark };
}

const ALL_NAMES = TOKENS.map((t) => t.name);

export function Inspector({ entry }: { entry: CatalogEntry | undefined }) {
  const [tab, setTab] = useState<"tokens" | "color" | "copy">("tokens");

  return (
    <aside className="flex h-full w-96 shrink-0 flex-col border-l border-border bg-card">
      <div className="flex gap-1 border-b border-border p-2">
        {(["tokens", "color", "copy"] as const).map((id) => (
          <button
            key={id}
            onClick={() => setTab(id)}
            className={`rounded-md px-2.5 py-1 text-xs font-medium capitalize ${
              tab === id ? "bg-muted text-foreground" : "text-muted-foreground hover:bg-surface-hover"
            }`}
          >
            {id}
          </button>
        ))}
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto p-3">
        {tab === "tokens" && <TokenList />}
        {tab === "color" && <ColorEditor />}
        {tab === "copy" && <CopyList entry={entry} />}
      </div>
    </aside>
  );
}

function TokenList() {
  const theme = useTheme();
  const [values, setValues] = useState(() => readBothThemes(ALL_NAMES));

  // Driven by the theme store rather than a MutationObserver on <html>:
  // readBothThemes() itself flips the `dark` class, so an observer watching that
  // element re-fires on its own write and loops until the tab dies.
  useEffect(() => setValues(readBothThemes(ALL_NAMES)), [theme]);

  const groups = useMemo(() => {
    const out = new Map<string, TokenDef[]>();
    for (const token of TOKENS) {
      const list = out.get(token.group) ?? [];
      list.push(token);
      out.set(token.group, list);
    }
    return [...out.entries()];
  }, []);

  return (
    <div className="space-y-4">
      <p className="text-[11px] leading-relaxed text-muted-foreground">
        {TOKENS.length} tokens across {groups.length} groups. Values are read live from the document, so a
        runtime rebrand shows through. Editing a value means editing{" "}
        <code className="font-mono">web/src/styles.css</code> and{" "}
        <code className="font-mono">packages/plugin-sdk/src/tokens/index.ts</code> together — the parity test
        fails otherwise.
      </p>
      {groups.map(([group, tokens]) => (
        <section key={group}>
          <h3 className="mb-1.5 text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
            {group}
          </h3>
          <div className="space-y-1">
            {tokens.map((token) => (
              <TokenRow key={token.name} token={token} light={values.light} dark={values.dark} />
            ))}
          </div>
        </section>
      ))}
    </div>
  );
}

function TokenRow({
  token,
  light,
  dark,
}: {
  token: TokenDef;
  light: Record<string, string>;
  dark: Record<string, string>;
}) {
  // Swatches are painted with var(--name) so the browser resolves whatever the
  // declaration is, including color-mix/oklch derived from a rebranded seed.
  const isColor = token.group !== "radius" && token.group !== "typography";
  const alias = themeAlias(token);
  return (
    <div className="rounded-md border border-border px-2 py-1.5">
      <div className="flex items-center gap-2">
        <code className="min-w-0 flex-1 truncate font-mono text-[11px] text-foreground">{token.name}</code>
        {token.seed && (
          <span className="rounded bg-info-bg px-1 text-[9px] font-semibold uppercase text-info-fg">seed</span>
        )}
        {isColor && (
          <>
            <span
              aria-label={`${token.name} light`}
              className="h-4 w-4 shrink-0 rounded border border-border"
              style={{ background: `var(${token.name})` }}
            />
            {token.dark && (
              <span
                aria-label={`${token.name} dark`}
                className="h-4 w-4 shrink-0 rounded border border-border"
                style={{ background: `var(${token.name})`, filter: "invert(1)" }}
                title="dark override exists — value shown below"
              />
            )}
          </>
        )}
      </div>
      <dl className="mt-1 space-y-0.5">
        <Row label="light" value={token.root ? light[token.name] : "— (not in :root)"} />
        <Row label="dark" value={token.dark ? dark[token.name] : "= light (no override)"} />
        {alias && <Row label="theme" value={alias} />}
      </dl>
    </div>
  );
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex gap-2 text-[10px]">
      <dt className="w-12 shrink-0 text-muted-foreground">{label}</dt>
      <dd className="min-w-0 flex-1 break-all font-mono text-foreground/80">{value}</dd>
    </div>
  );
}

const HEX = /^#(?:[0-9a-f]{3}|[0-9a-f]{6})$/i;

function ColorEditor() {
  const initial = useMemo(() => readBothThemes(SEED_TOKENS.map((t) => t.name)).light, []);
  const [draft, setDraft] = useState<Record<string, string>>(initial);

  const apply = useCallback((name: string, value: string) => {
    // The same write surface the white-label plugin uses: inline seeds on <html>.
    // Nothing else is touched, and every derived token follows because the
    // stylesheet mixes them from --primary.
    document.documentElement.style.setProperty(name, value);
  }, []);

  const update = (name: string, value: string) => {
    setDraft((d) => ({ ...d, [name]: value }));
    apply(name, value);
  };

  const reset = () => {
    const next = { ...draft };
    for (const token of SEED_TOKENS) {
      document.documentElement.style.removeProperty(token.name);
      next[token.name] = initial[token.name] ?? "";
    }
    setDraft(next);
  };

  // The snippet a developer pastes into styles.css to make the tweak permanent.
  const snippet = SEED_TOKENS.filter((t) => draft[t.name] !== initial[t.name])
    .map((t) => `  ${t.name}: ${draft[t.name]};`)
    .join("\n");

  return (
    <div className="space-y-4">
      <p className="text-[11px] leading-relaxed text-muted-foreground">
        Live on the running app, using the same two inline seeds a white-label rebrand writes. Derived tokens
        (hovers, tints, info surfaces, the gradient) follow on their own — if one does not, it is hardcoded
        somewhere and that is a bug the colour lint should have caught.
      </p>
      {SEED_TOKENS.map((token) => (
        <div key={token.name}>
          <label className="block font-mono text-[11px] text-foreground" htmlFor={`seed-${token.name}`}>
            {token.name}
          </label>
          <div className="mt-1 flex items-center gap-2">
            <input
              id={`seed-${token.name}`}
              type="color"
              value={HEX.test(draft[token.name] ?? "") ? draft[token.name] : "#000000"}
              onChange={(e) => update(token.name, e.target.value)}
              className="h-8 w-10 shrink-0 cursor-pointer rounded border border-border bg-card"
            />
            <input
              value={draft[token.name] ?? ""}
              onChange={(e) => update(token.name, e.target.value)}
              className="min-w-0 flex-1 rounded-md border border-input bg-card px-2 py-1 font-mono text-[11px]"
            />
          </div>
        </div>
      ))}
      <button
        onClick={reset}
        className="rounded-md border border-border px-2.5 py-1 text-xs text-muted-foreground hover:bg-surface-hover"
      >
        Reset to stylesheet defaults
      </button>
      {snippet && (
        <div>
          <h3 className="mb-1 text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
            Make it permanent
          </h3>
          <pre className="overflow-x-auto rounded-md border border-border bg-muted/40 p-2 font-mono text-[10px] leading-relaxed">
            {`:root {\n${snippet}\n}`}
          </pre>
          <p className="mt-1 text-[10px] text-muted-foreground">
            Also update the matching entry in packages/plugin-sdk/src/tokens/index.ts if presence changes, or the
            parity test fails.
          </p>
        </div>
      )}
    </div>
  );
}

function CopyList({ entry }: { entry: CatalogEntry | undefined }) {
  if (!entry) {
    return <p className="text-[11px] text-muted-foreground">Select a component to see its copy.</p>;
  }
  return (
    <div className="space-y-3">
      <p className="text-[11px] leading-relaxed text-muted-foreground">
        Every user-visible string the <strong>{entry.name}</strong> preview renders. This is the shortest path to
        judging whether the wording is concise and consistent — the same strings the i18n audit tracks in the
        app.
      </p>
      {entry.copy.length === 0 ? (
        <p className="text-[11px] text-muted-foreground">Renders no copy of its own.</p>
      ) : (
        <ul className="space-y-1">
          {entry.copy.map((line) => (
            <li key={line} className="rounded-md border border-border px-2 py-1.5 text-[11px]">
              {line}
            </li>
          ))}
        </ul>
      )}
      <div>
        <h3 className="mb-1 text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
          SDK exports covered
        </h3>
        <div className="flex flex-wrap gap-1">
          {entry.covers.map((name) => (
            <code key={name} className="rounded bg-muted px-1 py-0.5 font-mono text-[10px]">
              {name}
            </code>
          ))}
        </div>
      </div>
    </div>
  );
}
