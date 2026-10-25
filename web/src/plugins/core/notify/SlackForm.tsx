import { useEffect, useState } from "react";
import { Field, Input, Select, useNotificationChannelForm, useTranslation } from "@octarq/plugin-sdk";

interface IntegrationOption {
  id: number;
  name: string;
  provider: string;
}

export default function SlackForm() {
  const { t } = useTranslation();
  const { config, updateConfig } = useNotificationChannelForm();
  const [integrations, setIntegrations] = useState<IntegrationOption[]>([]);
  const [selectedSource, setSelectedSource] = useState<string>("custom");
  const [webhookUrl, setWebhookUrl] = useState(config.webhookUrl || "");

  const isConfigured = Boolean(
    config.webhookUrlSet ||
    (config.webhookUrl && config.webhookUrl.includes("***")) ||
    config.integrationId
  );

  useEffect(() => {
    fetch("/api/integrations", { credentials: "same-origin" })
      .then((res) => (res.ok ? res.json() : []))
      .then((data: any[]) => {
        if (Array.isArray(data)) {
          const slackIntegrations = data.filter(
            (item) => item.provider === "slack" || item.providerType === "communication"
          );
          setIntegrations(slackIntegrations);
          if (config.integrationId && slackIntegrations.some((i) => i.id === config.integrationId)) {
            setSelectedSource("integration");
          }
        }
      })
      .catch(() => {});
  }, [config.integrationId]);

  const handleUrlChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setWebhookUrl(e.target.value);
    updateConfig("webhookUrl", e.target.value);
    updateConfig("integrationId", undefined);
  };

  const handleIntegrationSelect = (val: string) => {
    const id = Number(val);
    if (id) {
      updateConfig("integrationId", id);
      updateConfig("webhookUrl", "");
    }
  };

  const sourceOptions = [
    { value: "custom", label: t("settings.slackDirectWebhook", "Direct Webhook URL") },
    { value: "integration", label: t("settings.slackFromIntegration", "Linked Workspace Integration") },
  ];

  const integrationOptions = [
    { value: "", label: t("settings.selectIntegration", "Select Slack Integration") },
    ...integrations.map((item) => ({
      value: String(item.id),
      label: item.name || `Slack #${item.id}`,
    })),
  ];

  return (
    <div className="space-y-4">
      {integrations.length > 0 && (
        <Field
          label={t("settings.slackSourceLabel", "Credential Source")}
          hint={t("settings.slackSourceHint", "Select an integration or enter a direct webhook URL.")}
        >
          <Select
            value={selectedSource}
            onValueChange={(src) => {
              setSelectedSource(src);
              if (src === "custom") {
                updateConfig("integrationId", undefined);
              }
            }}
            options={sourceOptions}
          />
        </Field>
      )}

      {selectedSource === "integration" && integrations.length > 0 ? (
        <Field
          label={t("settings.slackIntegrationLabel", "Workspace Integration")}
          hint={t("settings.slackIntegrationHint", "Pick a configured communication provider.")}
        >
          <Select
            value={String(config.integrationId || "")}
            onValueChange={handleIntegrationSelect}
            options={integrationOptions}
          />
        </Field>
      ) : (
        <Field
          label={t("settings.slackWebhookLabel", "Webhook URL")}
          hint={t("settings.slackWebhookHint", "Enter the incoming webhook URL provided by Slack.")}
        >
          <Input
            type="url"
            className="font-mono text-xs"
            placeholder="https://hooks.slack.com/services/..."
            value={webhookUrl}
            onChange={handleUrlChange}
            required={!isConfigured}
          />
        </Field>
      )}
    </div>
  );
}
