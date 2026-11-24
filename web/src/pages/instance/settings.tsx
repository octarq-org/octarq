import { Field, PageShell, PageSection, PageCard, Button, Modal, toast, confirmDialog, ExtensionSlot, Input } from "@octarq/plugin-sdk";
import { useEffect, useState } from "react";
import { api } from "../../api";
import { Send } from "lucide-react";
import { useTranslation } from "../../i18n";
import { InstanceVaultSection } from "./InstanceVaultSection";
import { useInstanceSettings } from "./shared";

export function InstanceSettings() {
  const { t } = useTranslation();
  const { s: settings, reload } = useInstanceSettings();

  const [appName, setAppName] = useState("");
  const [baseDomain, setBaseDomain] = useState("");
  const [sharedHosts, setSharedHosts] = useState("");
  const [retention, setRetention] = useState(90);
  const [publicCorsOrigins, setPublicCorsOrigins] = useState("");
  const [rlAuth, setRlAuth] = useState(60);
  const [rlApi, setRlApi] = useState(600);
  const [metricsToken, setMetricsToken] = useState("");
  const [metricsTokenSet, setMetricsTokenSet] = useState(false);

  const [busy, setBusy] = useState(false);
  const [testingMail, setTestingMail] = useState(false);
  const [backingUp, setBackingUp] = useState(false);
  const [build, setBuild] = useState<{ version: string; commit: string; builtAt: string } | null>(null);

  useEffect(() => {
    api.instanceBuild?.().then(setBuild).catch(() => {});
  }, []);

  useEffect(() => {
    if (settings) {
      setAppName(settings.appName ?? "");
      setBaseDomain(settings.baseDomain ?? "");
      setSharedHosts(settings.sharedHosts ?? "");
      setRetention(settings.dataRetentionDays ?? 90);
      setPublicCorsOrigins(settings.publicCorsOrigins ?? "");
      setRlAuth(settings.ratelimitAuthRpm ?? 60);
      setRlApi(settings.ratelimitApiRpm ?? 600);
      setMetricsTokenSet(settings.metricsTokenSet ?? false);
    }
  }, [settings]);

  async function saveGeneral() {
    setBusy(true);
    try {
      const payload: Parameters<typeof api.updateInstanceSettings>[0] = {
        appName,
        baseDomain,
        sharedHosts,
        dataRetentionDays: retention,
        publicCorsOrigins,
        ratelimitAuthRpm: rlAuth,
        ratelimitApiRpm: rlApi,
        ...(metricsToken ? { metricsToken } : {}),
      };
      const v = await api.updateInstanceSettings(payload);
      setMetricsTokenSet(v.metricsTokenSet);
      setMetricsToken("");
      toast.success(t("settings.saved"));
      reload();
    } catch (err: any) {
      toast.error(err?.message || t("settings.saveFailed", "Failed to save settings"));
    } finally {
      setBusy(false);
    }
  }

  async function handleBackup() {
    setBackingUp(true);
    try {
      const { blob, filename } = await api.downloadBackup();
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = filename;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    } catch (err: any) {
      toast.error(err?.message || t("settings.backupFailed"));
    } finally {
      setBackingUp(false);
    }
  }

  if (!settings) {
    return (
      <div className="flex h-32 items-center justify-center text-sm text-muted-foreground">
        {t("settings.loadingInstanceSettings")}
      </div>
    );
  }

  return (
    <PageShell
      title={t("settings.instanceTitle")}
      description={t("settings.instanceDesc")}
    >
      <PageSection title={t("settings.generalInfo")}>
        <PageCard className="mt-0 space-y-6 p-5">
          <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
            <Field label={t("settings.instanceAppName")} hint={t("settings.instanceAppNameHint")}>
              <Input
                className="text-sm"
                value={appName}
                onChange={(e) => setAppName(e.target.value)}
                placeholder="octarq"
              />
            </Field>
            <Field label={t("settings.instanceBaseDomain")} hint={t("settings.instanceBaseDomainHint")}>
              <Input
                className="font-mono text-sm"
                value={baseDomain}
                onChange={(e) => setBaseDomain(e.target.value)}
                placeholder="e.g. app.example.com"
              />
            </Field>
            <Field label={t("settings.instanceSharedHosts")} hint={t("settings.instanceSharedHostsHint")}>
              <Input
                className="font-mono text-sm"
                value={sharedHosts}
                onChange={(e) => setSharedHosts(e.target.value)}
                placeholder={t("settings.instanceSharedHostsPlaceholder")}
              />
            </Field>
            <Field label={t("settings.retentionLabel")} hint={t("settings.retentionHint")}>
              <Input
                type="number"
                min={0}
                className="font-mono text-sm"
                value={retention}
                onChange={(e) => setRetention(Number(e.target.value))}
              />
            </Field>
            <Field label={t("settings.instancePublicCorsOrigins")} hint={t("settings.instancePublicCorsOriginsHint")}>
              <Input
                className="font-mono text-sm"
                value={publicCorsOrigins}
                onChange={(e) => setPublicCorsOrigins(e.target.value)}
                placeholder={t("settings.instancePublicCorsOriginsPlaceholder")}
              />
            </Field>
            <Field label={t("settings.instanceTestMailLabel")} hint={t("settings.instanceTestMailDesc")}>
              <Button
                type="button"
                variant="outline"
                className="w-fit text-xs gap-1.5"
                onClick={() => setTestingMail(true)}
              >
                <Send className="h-3.5 w-3.5" />
                {t("settings.instanceTestMailBtn")}
              </Button>
            </Field>
          </div>

          <Field
            label={t("settings.instanceMetricsToken")}
            hint={metricsTokenSet ? t("settings.instanceMetricsTokenSetHint") : t("settings.instanceMetricsTokenHint")}
          >
            <div className="flex gap-2 max-w-md">
              <Input
                className="font-mono text-sm"
                type="password"
                value={metricsToken}
                onChange={(e) => setMetricsToken(e.target.value)}
                placeholder={metricsTokenSet ? "••••••••" : ""}
              />
              {metricsTokenSet && (
                <Button
                  variant="ghost"
                  className="shrink-0 text-xs text-danger-fg hover:text-danger-fg"
                  onClick={async () => {
                    if (await confirmDialog(t("settings.clearMetricsToken"))) {
                      try {
                        await api.updateInstanceSettings({ metricsToken: "" });
                        toast.success(t("settings.saved"));
                        reload();
                      } catch (err: any) {
                        toast.error(err?.message || t("settings.saveFailed", "Failed to save settings"));
                      }
                    }
                  }}
                  disabled={busy}
                >
                  {t("settings.instanceMetricsClear")}
                </Button>
              )}
            </div>
          </Field>

          <div className="border-t border-border pt-5">
            <Button variant="primary" onClick={saveGeneral} disabled={busy}>
              {busy ? t("settings.saving") : t("settings.save")}
            </Button>
          </div>
        </PageCard>
      </PageSection>

      <PageSection
        title={t("settings.rateLimiting")}
        description={t("settings.instanceRlHint")}
      >
        <PageCard className="mt-0 space-y-6 p-5">
          <div className="grid grid-cols-1 gap-6 sm:grid-cols-2">
            <Field label={t("settings.instanceRlAuth")}>
              <Input
                type="number"
                min={0}
                className="font-mono text-sm"
                value={rlAuth}
                onChange={(e) => setRlAuth(Number(e.target.value))}
              />
            </Field>
            <Field label={t("settings.instanceRlApi")}>
              <Input
                type="number"
                min={0}
                className="font-mono text-sm"
                value={rlApi}
                onChange={(e) => setRlApi(Number(e.target.value))}
              />
            </Field>
          </div>
          <div className="border-t border-border pt-5">
            <Button variant="primary" onClick={saveGeneral} disabled={busy}>
              {busy ? t("settings.saving") : t("settings.save")}
            </Button>
          </div>
        </PageCard>
      </PageSection>

      {build && (
        <PageSection title={t("settings.buildTitle")}>
          <PageCard className="mt-0 p-5">
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
              <Field label={t("settings.buildVersion")}>
                <div className="font-mono text-sm text-foreground">{build.version}</div>
              </Field>
              <Field label={t("settings.buildCommit")}>
                <div className="font-mono text-sm text-foreground">
                  {build.commit ? (build.commit.length > 8 ? build.commit.slice(0, 8) : build.commit) : "unknown"}
                </div>
              </Field>
              <Field label={t("settings.buildBuiltAt")}>
                <div className="font-mono tnum text-sm text-foreground">{build.builtAt}</div>
              </Field>
            </div>
          </PageCard>
        </PageSection>
      )}

      <PageSection
        title={t("settings.backupTitle")}
        description={t("settings.backupDesc")}
      >
        <PageCard className="mt-0 p-5">
          <Button variant="outline" onClick={handleBackup} disabled={backingUp}>
            {backingUp ? t("settings.backupPreparing") : t("settings.backupDownload")}
          </Button>
        </PageCard>
      </PageSection>

      <InstanceVaultSection />

      <PageSection title={t("settings.systemSettings", "System Settings")}>
        <div className="mt-3">
          <ExtensionSlot name="settings-infra" />
        </div>
      </PageSection>

      {testingMail && (
        <TestInstanceMailModal onClose={() => setTestingMail(false)} />
      )}
    </PageShell>
  );
}

function TestInstanceMailModal({ onClose }: { onClose: () => void }) {
  const [to, setTo] = useState("");
  const [busy, setBusy] = useState(false);
  const { t } = useTranslation();

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const recipient = to.trim();
    if (!recipient) return;
    setBusy(true);
    try {
      await api.testInstanceMail(recipient);
      toast.success(t("settings.instanceTestMailSuccess", { to: recipient }));
      onClose();
    } catch (err: any) {
      toast.error(err?.message || t("settings.instanceTestMailFailed"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal title={t("settings.instanceTestMailTitle")} onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-4">
        <p className="text-xs text-muted-foreground leading-relaxed">
          {t("settings.instanceTestMailDesc")}
        </p>
        <Field label={t("settings.instanceTestMailRecipient")} hint={t("settings.instanceTestMailRecipientHint")}>
          <Input
            type="email"
            className="text-sm"
            value={to}
            onChange={(e) => setTo(e.target.value)}
            placeholder={t("settings.instanceTestMailRecipientPlaceholder")}
            autoFocus
            required
          />
        </Field>
        <div className="flex justify-end gap-2.5 pt-4 border-t border-border">
          <Button type="button" variant="ghost" onClick={onClose}>
            {t("settings.cancel")}
          </Button>
          <Button type="submit" variant="primary" disabled={busy || !to.trim()}>
            {busy ? t("settings.instanceTestMailSending") : t("settings.instanceTestMailSubmit")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
