import { useState } from "react";
import { AnimatePresence, motion, PanInfo } from "framer-motion";
import { Check, ThumbsUp, X } from "lucide-react";
import { Button } from "../../ui";
import { useTranslation } from "../../i18n";

interface TinderStepProps {
  initialChoices?: Record<string, "agree" | "skip">;
  onNext: (data: { tinderChoices: Record<string, "agree" | "skip"> }) => void;
  onBack: () => void;
}

const TINDER_CARDS = [
  { id: "bitly", key: "onboarding.tinderCard1" },
  { id: "email_dns", key: "onboarding.tinderCard2" },
  { id: "ai_domain", key: "onboarding.tinderCard3" },
  { id: "saas_bill", key: "onboarding.tinderCard4" },
];

export function TinderStep({ initialChoices = {}, onNext, onBack }: TinderStepProps) {
  const { t } = useTranslation();
  const [choices, setChoices] = useState<Record<string, "agree" | "skip">>(initialChoices);
  const [currentIndex, setCurrentIndex] = useState(0);

  const handleChoice = (action: "agree" | "skip") => {
    const currentCard = TINDER_CARDS[currentIndex];
    if (!currentCard) return;

    const nextChoices = { ...choices, [currentCard.id]: action };
    setChoices(nextChoices);

    if (currentIndex + 1 < TINDER_CARDS.length) {
      setCurrentIndex((idx) => idx + 1);
    } else {
      onNext({ tinderChoices: nextChoices });
    }
  };

  const handleDragEnd = (_: unknown, info: PanInfo) => {
    if (info.offset.x > 80) {
      handleChoice("agree");
    } else if (info.offset.x < -80) {
      handleChoice("skip");
    }
  };

  const card = TINDER_CARDS[currentIndex];
  const isFinished = currentIndex >= TINDER_CARDS.length;

  return (
    <div className="flex flex-col max-w-xl mx-auto space-y-6">
      <div className="text-center space-y-2">
        <h2 className="text-2xl sm:text-3xl font-bold tracking-tight text-foreground">
          {t("onboarding.tinderTitle")}
        </h2>
        <p className="text-sm text-muted-foreground">{t("onboarding.tinderSubtitle")}</p>
        <span className="inline-block text-xs font-mono text-muted-foreground px-2 py-0.5 rounded-full bg-surface-hover">
          {t("onboarding.tinderProgress", {
            current: Math.min(currentIndex + 1, TINDER_CARDS.length),
            total: TINDER_CARDS.length,
          })}
        </span>
      </div>

      <div className="relative h-64 w-full flex items-center justify-center">
        <AnimatePresence mode="wait">
          {!isFinished && card && (
            <motion.div
              key={card.id}
              drag="x"
              dragConstraints={{ left: 0, right: 0 }}
              dragElastic={0.7}
              onDragEnd={handleDragEnd}
              initial={{ scale: 0.95, opacity: 0, y: 10 }}
              animate={{ scale: 1, opacity: 1, y: 0 }}
              exit={{ scale: 0.95, opacity: 0, transition: { duration: 0.2 } }}
              className="absolute inset-0 p-6 rounded-3xl glass border border-border shadow-xl flex flex-col justify-between cursor-grab active:cursor-grabbing select-none"
            >
              <div className="flex justify-between items-center text-xs text-muted-foreground">
                <span className="font-semibold text-accent-fg">{t("onboarding.tinderInsight")}</span>
                <span>{currentIndex + 1} / {TINDER_CARDS.length}</span>
              </div>

              <div className="my-auto py-2">
                <p className="text-base sm:text-lg font-medium text-foreground text-center leading-relaxed">
                  {t(card.key)}
                </p>
              </div>

              <div className="flex justify-between items-center text-xs text-muted-foreground pt-2 border-t border-border/40">
                <span className="flex items-center gap-1 text-muted-foreground">
                  <span>&larr;</span>
                  <span>{t("onboarding.tinderSkip")}</span>
                </span>
                <span className="flex items-center gap-1 text-accent-fg font-medium">
                  <span>{t("onboarding.tinderAgree")}</span>
                  <span>&rarr;</span>
                </span>
              </div>
            </motion.div>
          )}
        </AnimatePresence>
      </div>

      {/* Action buttons */}
      <div className="flex items-center justify-center gap-6 pt-2">
        <button
          type="button"
          onClick={() => handleChoice("skip")}
          aria-label={t("onboarding.tinderSkip")}
          className="w-14 h-14 rounded-full glass border border-border flex items-center justify-center text-muted-foreground hover:text-foreground hover:bg-surface-hover transition-all shadow-md active:scale-95"
        >
          <X className="w-6 h-6" />
        </button>
        <button
          type="button"
          onClick={() => handleChoice("agree")}
          aria-label={t("onboarding.tinderAgree")}
          className="w-14 h-14 rounded-full bg-primary text-primary-foreground flex items-center justify-center hover:opacity-90 transition-all shadow-lg active:scale-95"
        >
          <ThumbsUp className="w-6 h-6" />
        </button>
      </div>

      <div className="flex items-center justify-between pt-2">
        <Button variant="ghost" onClick={onBack}>
          {t("onboarding.back")}
        </Button>
        <Button variant="outline" onClick={() => onNext({ tinderChoices: choices })}>
          {t("onboarding.skip")}
        </Button>
      </div>
    </div>
  );
}
