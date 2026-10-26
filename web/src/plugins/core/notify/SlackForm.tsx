import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { AlertCircle } from "lucide-react";
import { Field, Select, useNotificationChannelForm, useTranslation } from "@octarq/plugin-sdk";

interface IntegrationOption {
  id: number;
  name: string;
  provider: string;
}

export default function SlackForm() {
  const { t } = useTranslation();
  const { config, updateConfig } = useNotificationChannelForm();
  const [integrations, setIntegrations] = useState<IntegrationOption[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    fetch("/api/integrations", { credentials: "same-origin" })
      .then((res) => (res.ok ? res.json() : []))
      .then((data: any[]) => {
        if (cancelled) return;
        if (Array.isArray(data)) {
          const slackIntegrations = data.filter(
            (item) => item.provider === "slack"
          );
          setIntegrations(slackIntegrations);
          if (config?.integrationId && slackIntegrations.some((i) => i.id === config.integrationId)) {
            // Keep existing valid integrationId
          } else if (slackIntegrations.length > 0) {
            updateConfig("integrationId", slackIntegrations[0].id);
          }
          if (config?.webhookUrl) {
            updateConfig("webhookUrl", undefined);
          }
          if (config?.webhookUrlSet) {
            updateConfig("webhookUrlSet", undefined);
          }
        }
        setLoading(false);
      })
      .catch(() => {
        if (!cancelled) {
          setIntegrations([]);
          setLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const handleIntegrationSelect = (val: string) => {
    const id = Number(val);
    if (id) {
      updateConfig("integrationId", id);
    }
  };

  if (loading) {
    return (
      <div className="py-4 text-center text-xs text-muted-foreground">
        {t("settings.loadingLower", "loading…")}
      </div>
    );
  }

  if (integrations.length === 0) {
    return (
      <div className="rounded-lg border border-warning/30 bg-warning/5 p-4 text-xs text-foreground/80 space-y-3">
        <div className="flex items-start gap-2.5">
          <AlertCircle className="h-4 w-4 text-warning shrink-0 mt-0.5" />
          <div className="space-y-1">
            <p className="font-semibold text-foreground">
              {t("settings.noSlackIntegrationTitle", "No Slack integration found")}
            </p>
            <p className="text-muted-foreground">
              {t("settings.noSlackIntegration", "No Slack integration found in workspace. Add credentials in Integrations Hub first.")}
            </p>
          </div>
        </div>
        <Link
          to="/settings/integrations?category=communication"
          className="inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
        >
          {t("settings.openIntegrationsHub", "Integrations Hub")} →
        </Link>
      </div>
    );
  }

  const integrationOptions = [
    { value: "", label: "-- " + t("settings.selectIntegration", "Select Slack Integration") + " --" },
    ...integrations.map((item) => ({
      value: String(item.id),
      label: item.name ? `${item.name} (Slack)` : `Slack #${item.id}`,
    })),
  ];

  return (
    <div className="space-y-4">
      <Field
        label={t("settings.slackIntegrationLabel", "Workspace Integration")}
        hint={t("settings.slackIntegrationHint", "Pick a configured communication provider.")}
      >
        <Select
          value={config?.integrationId ? String(config.integrationId) : ""}
          onValueChange={handleIntegrationSelect}
          options={integrationOptions}
        />
      </Field>
    </div>
  );
}
