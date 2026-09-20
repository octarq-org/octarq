import { Field, useNotificationChannelForm, useTranslation, Input } from "@octarq/plugin-sdk";

export default function WebhookForm() {
  const { t } = useTranslation();
  const { config, updateConfig } = useNotificationChannelForm();

  return (
    <Field label={t("settings.customHttpTargetUrl")} hint={t("settings.customHttpTargetHint")}>
      <Input
        className="font-mono text-xs"
        value={config.url || ""}
        onChange={(e) => updateConfig("url", e.target.value)}
        placeholder="https://my-webhook.com/alerts"
        required
      />
    </Field>
  );
}
