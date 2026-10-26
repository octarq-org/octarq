import { useEffect, useState } from "react";
import { Field, Input, Select, useNotificationChannelForm, useTranslation } from "@octarq/plugin-sdk";

interface IntegrationOption {
  id: number;
  name: string;
  provider: string;
}

export default function TelegramForm() {
  const { t } = useTranslation();
  const { config, updateConfig } = useNotificationChannelForm();
  const [integrations, setIntegrations] = useState<IntegrationOption[]>([]);
  const [selectedSource, setSelectedSource] = useState<string>("custom");

  useEffect(() => {
    fetch("/api/integrations", { credentials: "same-origin" })
      .then((res) => (res.ok ? res.json() : []))
      .then((data: any[]) => {
        if (Array.isArray(data)) {
          const tgIntegrations = data.filter(
            (item) => item.provider === "telegram" || (item.providerType === "communication" && item.provider === "telegram")
          );
          setIntegrations(tgIntegrations);
          if (config.integrationId && tgIntegrations.some((i) => i.id === config.integrationId)) {
            setSelectedSource("integration");
          } else if (tgIntegrations.length > 0 && !config.botToken) {
            setSelectedSource("integration");
            updateConfig("integrationId", tgIntegrations[0].id);
          }
        }
      })
      .catch(() => {});
  }, [config.integrationId]);

  return (
    <>
      {integrations.length > 0 && (
        <Field label={t("settings.credentialSource", "Credential source")}>
          <Select
            value={selectedSource}
            onValueChange={(val: any) => {
              setSelectedSource(val);
              if (val === "custom") {
                updateConfig("integrationId", undefined);
              } else if (integrations.length > 0) {
                updateConfig("integrationId", integrations[0].id);
                updateConfig("botToken", "");
              }
            }}
            options={[
              { value: "integration", label: t("settings.telegramFromIntegration", "Linked Workspace Integration") },
              { value: "custom", label: t("settings.telegramDirectToken", "Direct Bot Token") },
            ]}
          />
        </Field>
      )}

      {selectedSource === "integration" && integrations.length > 0 ? (
        <Field label={t("settings.selectIntegration", "Select Telegram integration")}>
          <Select
            value={config.integrationId ? String(config.integrationId) : ""}
            onValueChange={(val: any) => {
              const id = Number(val);
              if (id) {
                updateConfig("integrationId", id);
                updateConfig("botToken", "");
              }
            }}
            options={[
              { value: "", label: "-- " + t("settings.selectIntegration", "Select Telegram integration") + " --" },
              ...integrations.map((i) => ({ value: String(i.id), label: `${i.name} (Telegram)` })),
            ]}
          />
        </Field>
      ) : (
        <Field label={t("settings.botAuthToken")} hint={t("settings.botAuthTokenHint")}>
          <Input
            className="font-mono text-xs"
            value={config.botToken || ""}
            onChange={(e: any) => updateConfig("botToken", e.target.value)}
            required={selectedSource === "custom"}
          />
        </Field>
      )}

      <Field label={t("settings.telegramChatId")} hint={t("settings.telegramChatIdHint")}>
        <Input
          className="font-mono text-xs"
          value={config.chatId || ""}
          onChange={(e: any) => updateConfig("chatId", e.target.value)}
          required
        />
      </Field>
    </>
  );
}
