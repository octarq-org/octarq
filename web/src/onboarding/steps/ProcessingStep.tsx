import { useEffect, useState } from "react";
import { Sparkles } from "lucide-react";
import { useTranslation } from "../../i18n";

interface ProcessingStepProps {
  onNext: () => void;
}

export function ProcessingStep({ onNext }: ProcessingStepProps) {
  const { t } = useTranslation();
  const [progress, setProgress] = useState(10);

  useEffect(() => {
    const timer = setTimeout(() => {
      setProgress(100);
    }, 300);

    const advanceTimer = setTimeout(() => {
      onNext();
    }, 1800);

    return () => {
      clearTimeout(timer);
      clearTimeout(advanceTimer);
    };
  }, [onNext]);

  return (
    <div className="flex flex-col items-center justify-center max-w-xl mx-auto space-y-6 text-center py-12">
      <div className="relative flex items-center justify-center">
        <div className="w-16 h-16 rounded-full bg-accent-soft text-accent-fg flex items-center justify-center animate-pulse">
          <Sparkles className="w-8 h-8" />
        </div>
      </div>

      <div className="space-y-2">
        <h2 className="text-2xl sm:text-3xl font-bold tracking-tight text-foreground">
          {t("onboarding.procTitle")}
        </h2>
        <p className="text-sm text-muted-foreground">{t("onboarding.procSubtitle")}</p>
      </div>

      <div className="w-full max-w-xs bg-surface-hover rounded-full h-2 overflow-hidden border border-border/80">
        <div
          className="bg-primary h-full rounded-full transition-all duration-1000 ease-out"
          style={{ width: `${progress}%` }}
        />
      </div>
    </div>
  );
}
