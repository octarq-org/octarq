import { Check, X } from "lucide-react";
import { Button } from "../../ui";
import { useTranslation } from "../../i18n";

interface ComparisonStepProps {
  onNext: () => void;
  onBack: () => void;
}

const COMPARISONS = [
  {
    dimKey: "onboarding.compDim1",
    tradKey: "onboarding.compTrad1",
    octKey: "onboarding.compOct1",
  },
  {
    dimKey: "onboarding.compDim2",
    tradKey: "onboarding.compTrad2",
    octKey: "onboarding.compOct2",
  },
  {
    dimKey: "onboarding.compDim3",
    tradKey: "onboarding.compTrad3",
    octKey: "onboarding.compOct3",
  },
  {
    dimKey: "onboarding.compDim4",
    tradKey: "onboarding.compTrad4",
    octKey: "onboarding.compOct4",
  },
];

export function ComparisonStep({ onNext, onBack }: ComparisonStepProps) {
  const { t } = useTranslation();

  return (
    <div className="flex flex-col max-w-xl mx-auto space-y-6">
      <div className="text-center space-y-2">
        <h2 className="text-2xl sm:text-3xl font-bold tracking-tight text-foreground">
          {t("onboarding.compareTitle")}
        </h2>
        <p className="text-sm text-muted-foreground">{t("onboarding.compareSubtitle")}</p>
      </div>

      <div className="rounded-2xl glass border border-border overflow-hidden shadow-sm">
        <div className="grid grid-cols-12 bg-surface-hover/60 border-b border-border p-3 text-xs font-semibold text-muted-foreground">
          <div className="col-span-4">{t("onboarding.compareColDim")}</div>
          <div className="col-span-4 text-center sm:text-left">{t("onboarding.compareColTrad")}</div>
          <div className="col-span-4 text-right sm:text-left text-accent-fg font-bold">
            {t("onboarding.compareColOctarq")}
          </div>
        </div>

        <div className="divide-y divide-border/60">
          {COMPARISONS.map((row, idx) => (
            <div key={idx} className="grid grid-cols-12 p-3 text-xs sm:text-sm items-center gap-2">
              <div className="col-span-4 font-medium text-foreground">
                {t(row.dimKey)}
              </div>
              <div className="col-span-4 text-muted-foreground text-xs leading-relaxed">
                {t(row.tradKey)}
              </div>
              <div className="col-span-4 text-foreground font-semibold text-xs leading-relaxed">
                {t(row.octKey)}
              </div>
            </div>
          ))}
        </div>
      </div>

      <div className="flex items-center justify-between pt-2">
        <Button variant="ghost" onClick={onBack}>
          {t("onboarding.back")}
        </Button>
        <Button onClick={onNext}>
          {t("onboarding.continue")}
        </Button>
      </div>
    </div>
  );
}
