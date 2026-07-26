import { Field, useNotificationChannelForm, useTranslation } from "../../../plugin-sdk";

export default function TelegramForm() {
  const { t } = useTranslation();
  const { config, updateConfig } = useNotificationChannelForm();

  return (
    <>
      <Field label={t("settings.botAuthToken")} hint={t("settings.botAuthTokenHint")}>
        <input
          className="input w-full font-mono text-xs"
          value={config.botToken || ""}
          onChange={(e) => updateConfig("botToken", e.target.value)}
          required
        />
      </Field>
      <Field label={t("settings.telegramChatId")} hint={t("settings.telegramChatIdHint")}>
        <input
          className="input w-full font-mono text-xs"
          value={config.chatId || ""}
          onChange={(e) => updateConfig("chatId", e.target.value)}
          required
        />
      </Field>
    </>
  );
}
