import { Shield, Sparkles, Star } from "lucide-react";
import { Button } from "../../ui";
import { useTranslation } from "../../i18n";

interface SocialProofStepProps {
  onNext: () => void;
  onBack: () => void;
}

const TESTIMONIALS = [
  {
    quoteKey: "onboarding.proofQuote1",
    authorKey: "onboarding.proofAuthor1",
    avatarChar: "D",
  },
  {
    quoteKey: "onboarding.proofQuote2",
    authorKey: "onboarding.proofAuthor2",
    avatarChar: "A",
  },
  {
    quoteKey: "onboarding.proofQuote3",
    authorKey: "onboarding.proofAuthor3",
    avatarChar: "S",
  },
];

export function SocialProofStep({ onNext, onBack }: SocialProofStepProps) {
  const { t } = useTranslation();

  return (
    <div className="flex flex-col max-w-xl mx-auto space-y-6">
      <div className="text-center space-y-2">
        <h2 className="text-2xl sm:text-3xl font-bold tracking-tight text-foreground">
          {t("onboarding.proofTitle")}
        </h2>
      </div>

      {/* Trust bar - TODO: 换真实数 */}
      <div className="grid grid-cols-3 gap-2 sm:gap-3 p-3 rounded-2xl glass border border-border text-center">
        <div className="flex flex-col items-center justify-center p-2">
          <div className="flex items-center gap-1 text-warning-fg mb-1">
            <Star className="w-4 h-4 fill-current" />
          </div>
          <span className="text-xs sm:text-sm font-bold text-foreground">
            {t("onboarding.proofStars")}
          </span>
        </div>
        <div className="flex flex-col items-center justify-center p-2 border-x border-border">
          <div className="flex items-center gap-1 text-accent-fg mb-1">
            <Sparkles className="w-4 h-4" />
          </div>
          <span className="text-xs sm:text-sm font-bold text-foreground">
            {t("onboarding.proofDeployments")}
          </span>
        </div>
        <div className="flex flex-col items-center justify-center p-2">
          <div className="flex items-center gap-1 text-success-fg mb-1">
            <Shield className="w-4 h-4" />
          </div>
          <span className="text-xs sm:text-sm font-bold text-foreground">
            {t("onboarding.proofOpenSource")}
          </span>
        </div>
      </div>

      {/* Testimonials */}
      <div className="space-y-3">
        {TESTIMONIALS.map((item, idx) => (
          <div key={idx} className="p-4 rounded-2xl glass border border-border/80 space-y-3">
            <p className="text-xs sm:text-sm text-foreground/90 italic leading-relaxed">
              {t(item.quoteKey)}
            </p>
            <div className="flex items-center gap-2 pt-1 border-t border-border/40">
              <div className="w-6 h-6 rounded-full bg-accent-soft text-accent-fg font-semibold text-xs flex items-center justify-center">
                {item.avatarChar}
              </div>
              <span className="text-xs text-muted-foreground font-medium">
                {t(item.authorKey)}
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
