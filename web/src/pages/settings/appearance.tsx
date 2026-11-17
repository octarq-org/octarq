import {
  PageRow,
  PageRows,
  PageSection,
  PageShell,
  Separator,
  cn,
  useTableDensity,
  useSetTableDensity,
} from "@octarq/plugin-sdk";
import { LANGS, useTranslation, type Lang } from "../../i18n";
import { setTheme, useTheme, type Theme } from "../../theme";

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
      className="inline-flex flex-wrap rounded-lg border border-border bg-well p-0.5"
    >
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          role="radio"
          aria-checked={value === o.value}
          onClick={() => onChange(o.value)}
          className={cn(
            "rounded-md px-2.5 py-1 text-xs font-medium transition-colors duration-100 pointer-coarse:min-h-11 pointer-coarse:min-w-11",
            value === o.value
              ? "bg-card text-foreground shadow-xs"
              : "text-muted-foreground hover:text-foreground",
          )}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}

export function AppearanceSettings() {
  const { t, lang, setLang } = useTranslation();
  const theme = useTheme();
  const density = useTableDensity();
  const setDensity = useSetTableDensity();

  return (
    <PageShell title={t("appearance.title")} description={t("appearance.subtitle")}>
      <PageSection>
        <PageRows>
          <PageRow
            title={t("appearance.themeTitle")}
            description={t("appearance.themeDesc")}
            control={
              <Segmented<Theme>
                value={theme}
                ariaLabel={t("appearance.themeTitle")}
                onChange={setTheme}
                options={[
                  { value: "light", label: t("appearance.themeLight") },
                  { value: "dark", label: t("appearance.themeDark") },
                ]}
              />
            }
          />
          <Separator />
          <PageRow
            title={t("appearance.languageTitle")}
            description={t("appearance.languageDesc")}
            control={
              <Segmented<Lang>
                value={lang}
                ariaLabel={t("appearance.languageTitle")}
                onChange={setLang}
                options={LANGS.map((l) => ({ value: l.code, label: l.label }))}
              />
            }
          />
          <Separator />
          <PageRow
            title={t("appearance.densityTitle")}
            description={t("appearance.densityDesc")}
            control={
              <Segmented
                value={density}
                ariaLabel={t("appearance.densityTitle")}
                onChange={setDensity}
                options={[
                  { value: "comfortable" as const, label: t("appearance.densityComfortable") },
                  { value: "compact" as const, label: t("appearance.densityCompact") },
                ]}
              />
            }
          />
        </PageRows>
      </PageSection>
    </PageShell>
  );
}
