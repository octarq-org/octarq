import { Guide } from "../../../ui";
import { useTranslation } from "../../../i18n";

export function MailGuide() {
  const { t } = useTranslation();

  return (
    <Guide title={t("mail.guideTitle")} open>
      <ol className="ml-4 list-decimal space-y-1.5 text-sm leading-relaxed text-foreground/70">
        <li>
          {t("mail.guideStep1Pre")}
          <b>{t("mail.guideStep1Domains")}</b>
          {t("mail.guideStep1Mid")}
          <b>{t("mail.guideStep1AcceptEmail")}</b>
          {t("mail.guideStep1Post")}
        </li>
        <li>
          {t("mail.guideStep2Pre")}
          <b>{t("mail.guideStep2Routing")}</b>
          {t("mail.guideStep2Post")}
        </li>
        <li>
          {t("mail.guideStep3Pre")}
          <code>deploy/cloudflare-email-worker.js</code>
          {t("mail.guideStep3Mid1")}
          <code>OCTARQ_ENDPOINT</code>
          {t("mail.guideStep3Mid2")}
          <b>{t("mail.guideStep3WebhookUrl")}</b>
          {t("mail.guideStep3Mid3")}
          <b>{t("mail.guideStep3SettingsMailboxes")}</b>
          {t("mail.guideStep3Post")}
        </li>
        <li>{t("mail.guideStep4")}</li>
      </ol>
    </Guide>
  );
}
