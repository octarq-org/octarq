import { useState } from "react";
import { AnimatePresence, motion } from "framer-motion";
import { useNavigate } from "react-router-dom";
import { useTranslation } from "../i18n";
import { BrandMark } from "../shell/BrandMark";
import { readAnswers, writeAnswers, writeCompleted } from "./storage";
import { OnboardingAnswers, TOTAL_STEPS } from "./types";
import { WelcomeStep } from "./steps/WelcomeStep";
import { GoalStep } from "./steps/GoalStep";
import { PainStep } from "./steps/PainStep";
import { SocialProofStep } from "./steps/SocialProofStep";
import { TinderStep } from "./steps/TinderStep";
import { SolutionStep } from "./steps/SolutionStep";
import { ComparisonStep } from "./steps/ComparisonStep";
import { PreferenceStep } from "./steps/PreferenceStep";
import { ProcessingStep } from "./steps/ProcessingStep";
import { DemoStep } from "./steps/DemoStep";
import { ValueStep } from "./steps/ValueStep";
import { PaywallStep } from "./steps/PaywallStep";

interface OnboardingFlowProps {
  onComplete?: () => void;
}

export function OnboardingFlow({ onComplete }: OnboardingFlowProps) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [answers, setAnswers] = useState<OnboardingAnswers>(readAnswers);
  const [step, setStep] = useState(1);
  const [isCompleting, setIsCompleting] = useState(false);

  const updateAndNext = (patch?: Partial<OnboardingAnswers>) => {
    if (patch) {
      setAnswers((prev) => {
        const next = { ...prev, ...patch };
        writeAnswers(next);
        return next;
      });
    }
    setStep((s) => Math.min(s + 1, TOTAL_STEPS));
  };

  const handleBack = () => {
    setStep((s) => Math.max(s - 1, 1));
  };

  const handleFinish = async () => {
    if (isCompleting) return;
    setIsCompleting(true);
    await writeCompleted();
    if (onComplete) {
      onComplete();
    } else {
      navigate("/overview");
    }
  };

  const progressPercent = Math.round(((step - 1) / (TOTAL_STEPS - 1)) * 100);

  return (
    <div className="octarq-aurora min-h-screen w-full flex flex-col justify-between p-4 sm:p-8 text-foreground select-none">
      {/* Top Header with Progress Bar */}
      <header className="max-w-2xl w-full mx-auto space-y-4">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2.5">
            <BrandMark size="sm" />
            <span className="font-bold text-sm tracking-tight text-foreground">Octarq</span>
          </div>

          <div className="flex items-center gap-3">
            <span className="text-xs font-mono text-muted-foreground">
              {t("onboarding.stepCount", { current: step, total: TOTAL_STEPS })}
            </span>
            {step > 1 && step < TOTAL_STEPS && (
              <button
                type="button"
                onClick={handleFinish}
                className="text-xs text-muted-foreground hover:text-foreground transition-colors"
              >
                {t("onboarding.skip")}
              </button>
            )}
          </div>
        </div>

        {/* Continuous progress bar */}
        <div className="w-full bg-border/60 rounded-full h-1.5 overflow-hidden">
          <div
            className="bg-primary h-full rounded-full transition-all duration-300 ease-out"
            style={{ width: `${progressPercent}%` }}
          />
        </div>
      </header>

      {/* Main Flow Content */}
      <main className="flex-1 flex items-center justify-center py-6 sm:py-10">
        <div className="w-full max-w-2xl">
          <AnimatePresence mode="wait">
            <motion.div
              key={step}
              initial={{ opacity: 0, x: 16 }}
              animate={{ opacity: 1, x: 0 }}
              exit={{ opacity: 0, x: -16 }}
              transition={{ duration: 0.22, ease: "easeInOut" }}
            >
              {step === 1 && <WelcomeStep onNext={() => updateAndNext()} />}
              {step === 2 && (
                <GoalStep
                  initialValue={answers.goal}
                  onNext={(data) => updateAndNext(data)}
                  onBack={handleBack}
                />
              )}
              {step === 3 && (
                <PainStep
                  initialValues={answers.painPoints}
                  onNext={(data) => updateAndNext(data)}
                  onBack={handleBack}
                />
              )}
              {step === 4 && <SocialProofStep onNext={() => updateAndNext()} onBack={handleBack} />}
              {step === 5 && (
                <TinderStep
                  initialChoices={answers.tinderChoices}
                  onNext={(data) => updateAndNext(data)}
                  onBack={handleBack}
                />
              )}
              {step === 6 && <SolutionStep onNext={() => updateAndNext()} onBack={handleBack} />}
              {step === 7 && <ComparisonStep onNext={() => updateAndNext()} onBack={handleBack} />}
              {step === 8 && (
                <PreferenceStep
                  initialValues={answers.preferences}
                  onNext={(data) => updateAndNext(data)}
                  onBack={handleBack}
                />
              )}
              {step === 9 && <ProcessingStep onNext={() => updateAndNext()} />}
              {step === 10 && (
                <DemoStep
                  initialPicks={answers.demoPicks}
                  preferences={answers.preferences}
                  onNext={(data) => updateAndNext(data)}
                  onBack={handleBack}
                />
              )}
              {step === 11 && (
                <ValueStep
                  demoPicks={answers.demoPicks}
                  onNext={() => updateAndNext()}
                  onBack={handleBack}
                />
              )}
              {step === 12 && <PaywallStep onComplete={handleFinish} />}
            </motion.div>
          </AnimatePresence>
        </div>
      </main>

      {/* Footer */}
      <footer className="max-w-2xl w-full mx-auto text-center py-2">
        <p className="text-[11px] text-muted-foreground">
          {t("onboarding.footerTagline")}
        </p>
      </footer>
    </div>
  );
}
