import { useState } from "react";
import { Check, Link2, Sparkles } from "lucide-react";
import { Button, cn } from "../../ui";
import { useTranslation } from "../../i18n";
import { DEMO_LINKS, DemoLinkItem } from "../types";

interface DemoStepProps {
  initialPicks?: string[];
  preferences?: string[];
  onNext: (data: { demoPicks: string[] }) => void;
  onBack: () => void;
}

export function DemoStep({
  initialPicks = [],
  preferences = [],
  onNext,
  onBack,
}: DemoStepProps) {
  const { t } = useTranslation();
  const [selected, setSelected] = useState<string[]>(() => {
    if (initialPicks.length > 0) return initialPicks.slice(0, 3);
    // Sort candidate links prioritizing user's selected preferences
    const prioritized = [...DEMO_LINKS].sort((a, b) => {
      const aMatch = preferences.includes(a.category) ? 1 : 0;
      const bMatch = preferences.includes(b.category) ? 1 : 0;
      return bMatch - aMatch;
    });
    return prioritized.slice(0, 3).map((l) => l.id);
  });

  const toggle = (id: string) => {
    if (selected.includes(id)) {
      setSelected((prev) => prev.filter((item) => item !== id));
    } else {
      if (selected.length < 3) {
        setSelected((prev) => [...prev, id]);
      } else {
        // replace the oldest pick if already 3
        setSelected((prev) => [...prev.slice(1), id]);
      }
    }
  };

  const remaining = 3 - selected.length;

  return (
    <div className="flex flex-col max-w-xl mx-auto space-y-6">
      <div className="text-center space-y-2">
        <h2 className="text-2xl sm:text-3xl font-bold tracking-tight text-foreground">
          {t("onboarding.demoTitle")}
        </h2>
        <div className="inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-medium bg-surface-hover border border-border">
          <Sparkles className="w-3.5 h-3.5 text-accent-fg" />
          <span className="text-foreground">
            {remaining > 0
              ? t("onboarding.demoRemaining", { count: remaining })
              : t("onboarding.demoReady")}
          </span>
        </div>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        {DEMO_LINKS.map((link: DemoLinkItem) => {
          const isSelected = selected.includes(link.id);
          return (
            <button
              key={link.id}
              type="button"
              onClick={() => toggle(link.id)}
              className={cn(
                "p-4 rounded-2xl border text-left transition-all flex flex-col justify-between gap-3 shadow-sm",
                isSelected
                  ? "bg-accent-soft/40 border-accent-border shadow-md ring-1 ring-ring"
                  : "glass hover:bg-surface-hover border-border/80",
              )}
            >
              <div className="flex items-start justify-between gap-2 w-full">
                <div className="flex items-center gap-2">
                  <div className="p-1.5 rounded-lg bg-surface-hover text-accent-fg">
                    <Link2 className="w-4 h-4" />
                  </div>
                  <span className="text-xs sm:text-sm font-semibold text-foreground">
                    {t(link.titleKey)}
                  </span>
                </div>
                <div
                  className={cn(
                    "w-4 h-4 rounded-full flex items-center justify-center text-xs shrink-0 border transition-colors",
                    isSelected
                      ? "bg-primary border-primary text-primary-foreground"
                      : "border-border bg-surface-hover text-transparent",
                  )}
                >
                  <Check className="w-3 h-3" />
                </div>
              </div>

              <div className="w-full bg-background/50 rounded-lg px-2.5 py-1 font-mono text-[11px] text-muted-foreground truncate border border-border/40">
                {link.domain}/{link.slug}
              </div>
            </button>
          );
        })}
      </div>

      <div className="flex items-center justify-between pt-2">
        <Button variant="ghost" onClick={onBack}>
          {t("onboarding.back")}
        </Button>
        <Button
          disabled={selected.length !== 3}
          onClick={() => onNext({ demoPicks: selected })}
        >
          {t("onboarding.continue")}
        </Button>
      </div>
    </div>
  );
}
