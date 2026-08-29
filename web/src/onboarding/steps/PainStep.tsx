import { useState } from "react";
import { Check } from "lucide-react";
import { Button, cn } from "../../ui";
import { useTranslation } from "../../i18n";

interface PainStepProps {
  initialValues?: string[];
  onNext: (data: { painPoints: string[] }) => void;
  onBack: () => void;
}

const PAIN_POINTS = [
  { id: "saas_cost", titleKey: "onboarding.painSaas" },
  { id: "fragmented", titleKey: "onboarding.painFragmented" },
  { id: "email_deliverability", titleKey: "onboarding.painEmail" },
  { id: "lack_mcp", titleKey: "onboarding.painMcp" },
  { id: "tenants", titleKey: "onboarding.painTenants" },
  { id: "complex_ops", titleKey: "onboarding.painOps" },
];

export function PainStep({ initialValues = [], onNext, onBack }: PainStepProps) {
  const { t } = useTranslation();
  const [selected, setSelected] = useState<string[]>(initialValues);

  const toggle = (id: string) => {
    setSelected((prev) =>
      prev.includes(id) ? prev.filter((item) => item !== id) : [...prev, id],
    );
  };

  return (
    <div className="flex flex-col max-w-xl mx-auto space-y-6">
      <div className="text-center space-y-2">
        <h2 className="text-2xl sm:text-3xl font-bold tracking-tight text-foreground">
          {t("onboarding.painTitle")}
        </h2>
        <p className="text-sm text-muted-foreground">{t("onboarding.painSubtitle")}</p>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        {PAIN_POINTS.map((p) => {
          const isSelected = selected.includes(p.id);
          return (
            <button
              key={p.id}
              type="button"
              onClick={() => toggle(p.id)}
              className={cn(
                "p-4 rounded-2xl border text-left transition-all flex items-start gap-3 justify-between",
                isSelected
                  ? "bg-accent-soft/40 border-accent-border shadow-sm ring-1 ring-ring"
                  : "glass hover:bg-surface-hover border-border/80",
              )}
            >
              <span className="text-xs sm:text-sm font-medium text-foreground leading-snug">
                {t(p.titleKey)}
              </span>
              <div
                className={cn(
                  "w-4 h-4 rounded mt-0.5 shrink-0 flex items-center justify-center border transition-colors",
                  isSelected
                    ? "bg-primary border-primary text-primary-foreground"
                    : "border-border bg-surface-hover",
                )}
              >
                {isSelected && <Check className="w-3 h-3" />}
              </div>
            </button>
          );
        })}
      </div>

      <div className="flex items-center justify-between pt-2">
        <Button variant="ghost" onClick={onBack}>
          {t("onboarding.back")}
        </Button>
        <Button onClick={() => onNext({ painPoints: selected })}>
          {t("onboarding.continue")}
        </Button>
      </div>
    </div>
  );
}
