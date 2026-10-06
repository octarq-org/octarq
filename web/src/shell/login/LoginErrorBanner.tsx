import { ShieldAlert, Mail } from "lucide-react";
import { useTranslation } from "../../i18n";

export interface LoginErrorBannerProps {
  err: string;
  isMailUnavailableErr: boolean;
  isUnverifiedErr: boolean;
  verifySent: boolean;
  resendingVerify: boolean;
  onResendVerification: () => void;
}

export function LoginErrorBanner({
  err,
  isMailUnavailableErr,
  isUnverifiedErr,
  verifySent,
  resendingVerify,
  onResendVerification,
}: LoginErrorBannerProps) {
  const { t } = useTranslation();

  if (!err) return null;

  return (
    <div className="mb-4 p-3 rounded-xl bg-danger-fg/10 border border-danger-fg/20 text-danger-fg text-xs space-y-2">
      <div className="flex gap-2 items-center">
        <ShieldAlert className="h-4 w-4 shrink-0" />
        <span>{isMailUnavailableErr ? t("app.registerMailUnavailable") : err}</span>
      </div>
      {isUnverifiedErr && (
        <div className="pt-1">
          {verifySent ? (
            <p className="text-success-fg font-medium">
              ✓ {t("app.verificationSent")}
            </p>
          ) : (
            <button
              type="button"
              onClick={onResendVerification}
              disabled={resendingVerify}
              className="flex items-center gap-1.5 text-xs text-accent-fg font-medium underline"
            >
              <Mail className="h-3.5 w-3.5" />
              {resendingVerify
                ? t("app.sending")
                : t("app.resendVerificationBtn")}
            </button>
          )}
        </div>
      )}
    </div>
  );
}
