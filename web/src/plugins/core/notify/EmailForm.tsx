import { Field, useNotificationChannelForm, useTranslation } from "../../../plugin-sdk";

export default function EmailForm() {
  const { t } = useTranslation();
  const { config, updateConfig } = useNotificationChannelForm();

  return (
    <Field
      label={t("settings.notifyEmailAddress", "Destination Email")}
      hint={t("settings.notifyEmailHint", "Email address to receive alert notifications (leave blank to use account email)")}
    >
      <input
        type="email"
        className="input w-full text-xs"
        placeholder="alerts@example.com"
        value={config.email || ""}
        onChange={(e) => updateConfig("email", e.target.value)}
      />
    </Field>
  );
}
