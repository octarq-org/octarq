import { Field, useNotificationChannelForm, useTranslation, Input } from "@octarq/plugin-sdk";

export default function TelegramForm() {
  const { t } = useTranslation();
  const { config, updateConfig } = useNotificationChannelForm();

  return (
    <>
      <Field label={t("settings.botAuthToken")} hint={t("settings.botAuthTokenHint")}>
        <Input
          className="font-mono text-xs"
          value={config.botToken || ""}
          onChange={(e) => updateConfig("botToken", e.target.value)}
          required
        />
      </Field>
      <Field label={t("settings.telegramChatId")} hint={t("settings.telegramChatIdHint")}>
        <Input
          className="font-mono text-xs"
          value={config.chatId || ""}
          onChange={(e) => updateConfig("chatId", e.target.value)}
          required
        />
      </Field>
    </>
  );
}
