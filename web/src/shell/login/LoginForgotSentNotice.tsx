import { Alert } from "@octarq/plugin-sdk";
import { CheckCircle2 } from "lucide-react";
import { useTranslation } from "../../i18n";

export interface LoginForgotSentNoticeProps {
  onBackToSignIn: () => void;
}

export function LoginForgotSentNotice({ onBackToSignIn }: LoginForgotSentNoticeProps) {
  const { t } = useTranslation();

  return (
    <div className="text-center py-4 space-y-4">
      <Alert variant="success" icon={<CheckCircle2 className="h-4 w-4 shrink-0" />} className="text-xs p-3 rounded-xl">
        <span>{t("app.forgotSentNotice")}</span>
      </Alert>
      <button
        type="button"
        onClick={onBackToSignIn}
        className="text-xs text-accent-fg hover:underline font-medium"
      >
        {t("app.backToSignIn")}
      </button>
    </div>
  );
}
