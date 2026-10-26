import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { AlertCircle } from "lucide-react";
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
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    fetch("/api/integrations", { credentials: "same-origin" })
      .then((res) => (res.ok ? res.json() : []))
      .then((data: any[]) => {
        if (cancelled) return;
        if (Array.isArray(data)) {
          const tgIntegrations = data.filter((item) => item.provider === "telegram");
          setIntegrations(tgIntegrations);
          if (config?.integrationId && tgIntegrations.some((i) => i.id === config.integrationId)) {
            // Keep existing valid integrationId
          } else if (tgIntegrations.length > 0) {
            updateConfig("integrationId", tgIntegrations[0].id);
          }
          if (config?.botToken) {
            updateConfig("botToken", undefined);
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
              {t("settings.noTelegramIntegrationTitle", "No Telegram integration found")}
            </p>
            <p className="text-muted-foreground">
              {t("settings.noTelegramIntegration", "No Telegram integration found in workspace. Add credentials in Integrations Hub first.")}
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

  return (
    <div className="space-y-4">
      <Field label={t("settings.selectIntegration", "Select Telegram integration")}>
        <Select
          value={config.integrationId ? String(config.integrationId) : ""}
          onValueChange={(val: string) => {
            const id = Number(val);
            if (id) {
              updateConfig("integrationId", id);
            }
          }}
          options={[
            { value: "", label: "-- " + t("settings.selectIntegration", "Select Telegram integration") + " --" },
            ...integrations.map((i) => ({ value: String(i.id), label: `${i.name} (Telegram)` })),
          ]}
        />
      </Field>

      <Field label={t("settings.telegramChatId")} hint={t("settings.telegramChatIdHint")}>
        <Input
          className="font-mono text-xs"
          value={config.chatId || ""}
          onChange={(e: any) => updateConfig("chatId", e.target.value)}
          required
        />
      </Field>
    </div>
  );
}
