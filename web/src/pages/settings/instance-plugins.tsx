// Instance → Plugins: what this *binary* has loaded.
//
// This is the counterpart to Settings → Workspace → Features, and the two
// answer different questions. What is compiled into the instance is decided by
// the build and the plugin manifest, and an operator can only change it by
// redeploying — so this view is read-only and has no toggles. Whether a given
// workspace uses a feature is a per-org row, and that is what Features edits.
//
// It intentionally shows Core plumbing that the workspace view hides: an
// operator debugging "why is there no license page" needs to see that the
// licensing plugin is or isn't in the build.
import { useEffect, useState } from "react";
import { api, ApiError, InstancePluginInfo } from "../../api";
import { PageHeader, GlassCard, Badge, Alert, Table, THead, TBody, TR, TH, TD } from "../../ui";
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
    <div className="space-y-6">
      <PageHeader
        title={t("settings.instancePluginsTitle")}
        description={t("settings.instancePluginsDescription")}
      />

      {err && (
        <Alert variant="danger" icon={<ShieldAlert className="h-4 w-4 shrink-0" />} className="text-xs p-3 rounded-xl">
          {err}
        </Alert>
      )}

      {plugins === null ? (
        <GlassCard className="p-8 text-sm text-center text-foreground/50">{t("settings.loadingPlugins")}</GlassCard>
      ) : plugins.length === 0 ? (
        <GlassCard className="p-8 text-sm text-center text-foreground/55">{t("settings.noPlugins")}</GlassCard>
      ) : (
        <GlassCard className="overflow-hidden p-0">
          <Table>
            <THead className="border-b border-border/60">
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
                  <TD className="font-mono text-[11px] text-foreground/70">{p.featureKey}</TD>
                  <TD className="text-foreground/70 capitalize">{p.category}</TD>
                  <TD>
                    {p.requires && p.requires.length > 0 ? (
                      <div className="flex flex-wrap gap-1">
                        {p.requires.map((r) => (
                          <span
                            key={r}
                            className="rounded-md bg-foreground/[0.05] px-1.5 py-0.5 font-mono text-[10px] text-foreground/70"
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
                        // Core plumbing has no workspace toggle at all, so
                        // saying "on by default" about it would be misleading.
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
        </GlassCard>
      )}

      <p className="text-xs text-foreground/50 leading-relaxed">{t("settings.instancePluginsFootnote")}</p>
    </div>
  );
}
