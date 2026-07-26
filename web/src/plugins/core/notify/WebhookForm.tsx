import { Field, useNotificationChannelForm, useTranslation } from "../../../plugin-sdk";

export default function WebhookForm() {
  const { t } = useTranslation();
  const { config, updateConfig } = useNotificationChannelForm();

  return (
    <Field label={t("settings.customHttpTargetUrl")} hint={t("settings.customHttpTargetHint")}>
      <input
        className="input w-full font-mono text-xs"
        value={config.url || ""}
        onChange={(e) => updateConfig("url", e.target.value)}
        placeholder="https://my-webhook.com/alerts"
        required
      />
    </Field>
  );
}
