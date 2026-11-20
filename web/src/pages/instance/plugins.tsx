import { PageShell, PageCard, PageEmpty, Badge, Alert, Table, THead, TBody, TR, TH, TD } from "@octarq/plugin-sdk";
import { useEffect, useState } from "react";
import { api, ApiError, InstancePluginInfo } from "../../api";
import { ShieldAlert } from "lucide-react";
import { useTranslation } from "../../i18n";

export function InstancePluginsSettings() {
  const { t } = useTranslation();
  const [plugins, setPlugins] = useState<InstancePluginInfo[] | null>(null);
  const [err, setErr] = useState("");

  useEffect(() => {
    api
      .instancePlugins()
      .then(setPlugins)
      .catch((e) => {
        setErr(e instanceof ApiError ? e.message : t("settings.instancePluginsLoadFailed"));
        setPlugins([]);
      });
  }, []);

  return (
    <PageShell
      title={t("settings.instancePluginsTitle")}
      description={t("settings.instancePluginsDescription")}
    >
      {err && (
        <Alert variant="danger" icon={<ShieldAlert className="h-4 w-4 shrink-0" />} className="mt-6 text-xs p-3 rounded-lg">
          {err}
        </Alert>
      )}

      {plugins === null ? (
        <PageCard className="p-8 text-sm text-center text-muted-foreground">{t("settings.loadingPlugins")}</PageCard>
      ) : plugins.length === 0 ? (
        <PageEmpty title={t("settings.noPlugins")} />
      ) : (
        <PageCard className="overflow-hidden p-0">
          <Table>
            <THead className="border-b border-border">
              <TR>
                <TH>{t("settings.instancePluginName")}</TH>
                <TH>{t("settings.instancePluginFeatureKey")}</TH>
                <TH>{t("settings.instancePluginCategory")}</TH>
                <TH>{t("settings.instancePluginRequires")}</TH>
                <TH>{t("settings.instancePluginKind")}</TH>
              </TR>
            </THead>
            <TBody className="divide-y divide-border/40">
              {plugins.map((p) => (
                <TR key={p.name}>
                  <TD>
                    <div className="font-medium text-foreground">{p.title || p.name}</div>
                    <div className="font-mono text-[10px] text-muted-foreground">{p.name}</div>
                  </TD>
                  <TD className="font-mono text-[11px] text-muted-foreground">{p.featureKey}</TD>
                  <TD className="text-muted-foreground capitalize">{p.category}</TD>
                  <TD>
                    {p.requires && p.requires.length > 0 ? (
                      <div className="flex flex-wrap gap-1">
                        {p.requires.map((r) => (
                          <span
                            key={r}
                            className="rounded-md bg-well px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground"
                          >
                            {r}
                          </span>
                        ))}
                      </div>
                    ) : (
                      <span className="text-[11px] text-muted-foreground">—</span>
                    )}
                  </TD>
                  <TD>
                    <div className="flex flex-wrap items-center gap-1.5">
                      {p.core ? (
                        <Badge tone="violet" className="text-[10px]">
                          {t("settings.instancePluginCore")}
                        </Badge>
                      ) : p.enabledByDefault ? (
                        <Badge tone="green" className="text-[10px]">
                          {t("settings.instancePluginOptOut")}
                        </Badge>
                      ) : (
                        <Badge tone="neutral" className="text-[10px]">
                          {t("settings.instancePluginOptIn")}
                        </Badge>
                      )}
                      {p.hasUI && (
                        <Badge tone="cyan" className="text-[10px]">
                          {t("settings.instancePluginHasUI")}
                        </Badge>
                      )}
                    </div>
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
        </PageCard>
      )}

      <p className="mt-4 text-xs text-muted-foreground leading-relaxed">{t("settings.instancePluginsFootnote")}</p>
    </PageShell>
  );
}
