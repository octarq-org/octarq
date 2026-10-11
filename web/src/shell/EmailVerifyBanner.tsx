import { useState } from "react";
import { Mail } from "lucide-react";
import { Alert, toast } from "@octarq/plugin-sdk";
import { api } from "../api";
import { useTranslation } from "../i18n";

export interface EmailVerifyBannerProps {
  user: string;
  emailVerified: boolean | undefined;
  dismissed: boolean;
  onDismiss: () => void;
}

export function EmailVerifyBanner({
  user,
  emailVerified,
  dismissed,
  onDismiss,
}: EmailVerifyBannerProps) {
  const { t } = useTranslation();
  const [resendingVerify, setResendingVerify] = useState(false);

  if (emailVerified !== false || dismissed) {
    return null;
  }

  const handleResend = async () => {
    setResendingVerify(true);
    try {
      const res = await api.resendVerification(user);
      if (res.mailConfigured === false) {
        toast.error(t("app.verificationMailNotConfigured"));
      } else {
        toast.success(t("app.verificationSent"));
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Failed to send verification email.";
      toast.error(msg);
    } finally {
      setResendingVerify(false);
    }
  };

  return (
    <Alert
      variant="warning"
      align="center"
      icon={<Mail className="h-4 w-4" />}
      onDismiss={onDismiss}
      className="rounded-none border-x-0 border-t-0 text-xs py-2 px-4 z-40"
      actions={
        <button
          onClick={handleResend}
          disabled={resendingVerify}
          className="px-2.5 py-1 rounded-lg bg-warning-bg hover:brightness-95 border border-warning-border text-warning-fg font-medium transition-colors disabled:opacity-50 text-xs"
        >
          {resendingVerify ? t("app.sending") : t("app.resendVerificationBtn")}
        </button>
      }
    >
      {t("app.verifyEmailBanner")}
    </Alert>
  );
}
