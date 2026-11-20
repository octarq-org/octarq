import { ReactNode } from "react";
import { ConfirmProvider, PasswordConfirmProvider } from "@octarq/plugin-sdk";
import { useTranslation } from "./i18n";

export function ConfirmBridge({ children }: { children: ReactNode }) {
  const { t } = useTranslation();
  return (
    <ConfirmProvider
      defaultTitle={t("common.confirmTitle")}
      defaultConfirmLabel={t("common.confirm")}
      defaultCancelLabel={t("common.cancel")}
    >
      <PasswordConfirmProvider
        defaultTitle={t("common.passwordConfirmTitle")}
        defaultMessage={t("common.passwordConfirmMessage")}
        defaultPasswordLabel={t("common.passwordConfirmLabel")}
        defaultConfirmLabel={t("common.confirm")}
        defaultCancelLabel={t("common.cancel")}
      >
        {children}
      </PasswordConfirmProvider>
    </ConfirmProvider>
  );
}
