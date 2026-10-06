import { Alert } from "@octarq/plugin-sdk";
import { Mail } from "lucide-react";
import { useTranslation } from "../../i18n";

export interface LoginVerificationNoticeProps {
  email: string;
  verifySent: boolean;
  resendingVerify: boolean;
  onResend: () => void;
  onBackToSignIn: () => void;
}

export function LoginVerificationNotice({
  email,
  verifySent,
  resendingVerify,
  onResend,
  onBackToSignIn,
}: LoginVerificationNoticeProps) {
  const { t } = useTranslation();

  return (
    <div className="py-2 space-y-4">
      <Alert variant="info" icon={<Mail className="h-4 w-4 shrink-0" />} className="text-xs p-3 rounded-xl">
        <span>{t("app.registerVerifyNotice", { email })}</span>
      </Alert>
      <div className="text-center space-y-3">
        {verifySent ? (
          <p className="text-xs text-success-fg font-medium">✓ {t("app.verificationSent")}</p>
        ) : (
          <button
            type="button"
            onClick={onResend}
            disabled={resendingVerify}
            className="text-xs text-accent-fg hover:underline font-medium"
          >
            {resendingVerify ? t("app.sending") : t("app.resendVerificationBtn")}
          </button>
        )}
        <div>
          <button
            type="button"
            onClick={onBackToSignIn}
            className="text-xs text-accent-fg hover:underline font-medium"
          >
            {t("app.backToSignIn")}
          </button>
        </div>
      </div>
    </div>
  );
}
