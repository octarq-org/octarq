import { Check, Shield, Sparkles } from "lucide-react";
import { Button, toast } from "../../ui";
import { useTranslation } from "../../i18n";

interface PaywallStepProps {
  onComplete: () => void;
}

export function PaywallStep({ onComplete }: PaywallStepProps) {
  const { t } = useTranslation();

  const handleFreeStart = () => {
    onComplete();
  };

  const handleProUnlock = () => {
    // TODO: 接 Stripe
    toast.info(t("onboarding.proToast"));
    onComplete();
  };

  const handleSkip = () => {
    onComplete();
  };

  return (
    <div className="flex flex-col max-w-xl mx-auto space-y-6">
      <div className="text-center space-y-2">
        <h2 className="text-2xl sm:text-3xl font-bold tracking-tight text-foreground">
          {t("onboarding.paywallTitle")}
        </h2>
        <p className="text-sm text-muted-foreground">{t("onboarding.paywallSubtitle")}</p>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        {/* FREE Tier */}
        <div className="p-5 rounded-2xl glass border border-border flex flex-col justify-between space-y-4 shadow-sm">
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <span className="font-bold text-sm text-foreground">
                {t("onboarding.freeTierName")}
              </span>
              <Shield className="w-4 h-4 text-muted-foreground" />
            </div>
            <div className="text-xl font-bold text-foreground">
              {t("onboarding.freeTierPrice")}
            </div>
            <div className="space-y-2 pt-2 border-t border-border/40 text-xs text-muted-foreground">
              <div className="flex items-center gap-2">
                <Check className="w-3.5 h-3.5 text-success-fg shrink-0" />
                <span>{t("onboarding.freeFeat1")}</span>
              </div>
              <div className="flex items-center gap-2">
                <Check className="w-3.5 h-3.5 text-success-fg shrink-0" />
                <span>{t("onboarding.freeFeat2")}</span>
              </div>
              <div className="flex items-center gap-2">
                <Check className="w-3.5 h-3.5 text-success-fg shrink-0" />
                <span>{t("onboarding.freeFeat3")}</span>
              </div>
              <div className="flex items-center gap-2">
                <Check className="w-3.5 h-3.5 text-success-fg shrink-0" />
                <span>{t("onboarding.freeFeat4")}</span>
              </div>
            </div>
          </div>

          <Button variant="outline" onClick={handleFreeStart} className="w-full">
            {t("onboarding.startFree")}
          </Button>
        </div>

        {/* Pro Tier */}
        <div className="p-5 rounded-2xl bg-accent-soft/40 border border-accent-border flex flex-col justify-between space-y-4 shadow-md ring-1 ring-ring">
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <span className="font-bold text-sm text-accent-fg">
                {t("onboarding.proTierName")}
              </span>
              <Sparkles className="w-4 h-4 text-accent-fg" />
            </div>
            <div className="text-xl font-bold text-foreground">
              {t("onboarding.proTierPrice")}
            </div>
            <div className="space-y-2 pt-2 border-t border-accent-border/40 text-xs text-foreground/80">
              <div className="flex items-center gap-2">
                <Check className="w-3.5 h-3.5 text-accent-fg shrink-0" />
                <span className="font-medium">{t("onboarding.proFeat1")}</span>
              </div>
              <div className="flex items-center gap-2">
                <Check className="w-3.5 h-3.5 text-accent-fg shrink-0" />
                <span>{t("onboarding.proFeat2")}</span>
              </div>
              <div className="flex items-center gap-2">
                <Check className="w-3.5 h-3.5 text-accent-fg shrink-0" />
                <span>{t("onboarding.proFeat3")}</span>
              </div>
              <div className="flex items-center gap-2">
                <Check className="w-3.5 h-3.5 text-accent-fg shrink-0" />
                <span>{t("onboarding.proFeat4")}</span>
              </div>
            </div>
          </div>

          <Button onClick={handleProUnlock} className="w-full">
            {t("onboarding.unlockPro")}
          </Button>
        </div>
      </div>

      <div className="flex justify-center pt-2">
        <button
          type="button"
          onClick={handleSkip}
          className="text-xs text-muted-foreground hover:text-foreground transition-colors underline-offset-4 hover:underline"
        >
          {t("onboarding.skipForNow")}
        </button>
      </div>
    </div>
  );
}
