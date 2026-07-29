import { ReactNode } from "react";
import { ConfirmProvider } from "./ui";
import { useTranslation } from "./i18n";

// ConfirmBridge mounts the SDK's ConfirmProvider with the app's translated
// default labels. The SDK can't reach the app dictionary itself (it takes the
// strings as props precisely so it stays host-agnostic), and the defaults have
// to come from a hook, so they need a component inside I18nProvider — this one.
//
// Mirrors BrandBridge, which feeds the SDK's brand context the same way.
export function ConfirmBridge({ children }: { children: ReactNode }) {
  const { t } = useTranslation();
  return (
    <ConfirmProvider
      defaultTitle={t("common.confirmTitle")}
      defaultConfirmLabel={t("common.confirm")}
      defaultCancelLabel={t("common.cancel")}
    >
      {children}
    </ConfirmProvider>
  );
}
