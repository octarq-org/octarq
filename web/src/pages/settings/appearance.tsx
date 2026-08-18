import { GlassCard, PageHeader, cn, useTableDensity, useSetTableDensity } from "../../ui";
import { LANGS, useTranslation, type Lang } from "../../i18n";
import { setTheme, useTheme, type Theme } from "../../theme";

// Every per-user display preference on one page. Theme and language also have
// shortcuts in the top bar — those are for flipping mid-task; this is where the
// choice is presented with its label and explanation.

// A segmented control: one row of options, the active one lifted. Used three
// times here, so it is a component rather than three copies of the class list.
function Segmented<T extends string>({
  value,
  options,
  onChange,
  ariaLabel,
}: {
  value: T;
  options: { value: T; label: string }[];
  onChange: (v: T) => void;
  ariaLabel: string;
}) {
  return (
    <div
      role="radiogroup"
      aria-label={ariaLabel}
      className="inline-flex flex-wrap rounded-xl bg-foreground/[0.05] p-1 border border-foreground/[0.06]"
    >
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          role="radio"
          aria-checked={value === o.value}
          onClick={() => onChange(o.value)}
          className={cn(
            "rounded-lg px-3 py-1.5 text-xs font-medium transition-colors",
            value === o.value
              ? "bg-background text-foreground shadow-sm"
              : "text-foreground/60 hover:text-foreground",
          )}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}

function Setting({ title, desc, children }: { title: string; desc: string; children: React.ReactNode }) {
  return (
    <GlassCard className="p-6 max-w-xl space-y-3">
      <div>
        <div className="font-semibold text-sm text-foreground">{title}</div>
        <div className="text-xs text-foreground/50 mt-0.5">{desc}</div>
      </div>
      {children}
    </GlassCard>
  );
}

export function AppearanceSettings() {
  const { t, lang, setLang } = useTranslation();
  const theme = useTheme();
  const density = useTableDensity();
  const setDensity = useSetTableDensity();

  return (
    <div className="space-y-6">
      <PageHeader title={t("appearance.title")} description={t("appearance.subtitle")} />

      <Setting title={t("appearance.themeTitle")} desc={t("appearance.themeDesc")}>
        <Segmented<Theme>
          value={theme}
          ariaLabel={t("appearance.themeTitle")}
          onChange={setTheme}
          options={[
            { value: "light", label: t("appearance.themeLight") },
            { value: "dark", label: t("appearance.themeDark") },
          ]}
        />
      </Setting>

      <Setting title={t("appearance.languageTitle")} desc={t("appearance.languageDesc")}>
        <Segmented<Lang>
          value={lang}
          ariaLabel={t("appearance.languageTitle")}
          onChange={setLang}
          options={LANGS.map((l) => ({ value: l.code, label: l.label }))}
        />
      </Setting>

      <Setting title={t("appearance.densityTitle")} desc={t("appearance.densityDesc")}>
        <Segmented
          value={density}
          ariaLabel={t("appearance.densityTitle")}
          onChange={setDensity}
          options={[
            { value: "comfortable" as const, label: t("appearance.densityComfortable") },
            { value: "compact" as const, label: t("appearance.densityCompact") },
          ]}
        />
      </Setting>
    </div>
  );
}
