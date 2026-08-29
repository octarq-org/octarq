import { ArrowRight, CheckCircle2 } from "lucide-react";
import { Button } from "../../ui";
import { useTranslation } from "../../i18n";

interface SolutionStepProps {
  onNext: () => void;
  onBack: () => void;
}

const SOLUTIONS = [
  {
    painKey: "onboarding.solPain1",
    ansKey: "onboarding.solAns1",
    metricKey: "onboarding.solMetric1",
  },
  {
    painKey: "onboarding.solPain2",
    ansKey: "onboarding.solAns2",
    metricKey: "onboarding.solMetric2",
  },
  {
    painKey: "onboarding.solPain3",
    ansKey: "onboarding.solAns3",
    metricKey: "onboarding.solMetric3",
  },
  {
    painKey: "onboarding.solPain4",
    ansKey: "onboarding.solAns4",
    metricKey: "onboarding.solMetric4",
  },
];

export function SolutionStep({ onNext, onBack }: SolutionStepProps) {
  const { t } = useTranslation();

  return (
    <div className="flex flex-col max-w-xl mx-auto space-y-6">
      <div className="text-center space-y-2">
        <h2 className="text-2xl sm:text-3xl font-bold tracking-tight text-foreground">
          {t("onboarding.solutionTitle")}
        </h2>
        <p className="text-sm text-muted-foreground">{t("onboarding.solutionSubtitle")}</p>
      </div>

      <div className="space-y-3">
        {SOLUTIONS.map((item, idx) => (
          <div
            key={idx}
            className="p-4 rounded-2xl glass border border-border flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 shadow-sm"
          >
            <div className="flex items-center gap-2 text-xs sm:text-sm text-muted-foreground line-through decoration-muted-foreground/60">
              <span>{t(item.painKey)}</span>
            </div>

            <div className="flex items-center gap-2 self-stretch sm:self-auto justify-between sm:justify-end">
              <ArrowRight className="w-4 h-4 text-accent-fg hidden sm:block shrink-0" />
              <span className="text-xs sm:text-sm font-semibold text-foreground">
                {t(item.ansKey)}
              </span>
              <span className="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-accent-soft text-accent-fg border border-accent-border shrink-0">
                {t(item.metricKey)}
              </span>
            </div>
          </div>
        ))}
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
