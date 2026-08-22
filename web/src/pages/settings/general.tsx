import { useEffect, useState } from "react";
import { api, type OrgSlug, type Settings } from "../../api";
import { Field, Modal, PageHeader, GlassCard, Badge, Button, toast, Code } from "../../ui";
import { ShieldAlert } from "lucide-react";
import { useTranslation } from "../../i18n";
import { ExtensionSlot } from "../../plugin-sdk";

// WorkspaceAddress edits the slug — the workspace's identity in URLs, not a
// display name. Third parties hold addresses built from it (the billing
// webhook registered with Stripe, the redirect URI registered with the org's
// identity provider), and changing it breaks those until someone updates them
// by hand. So the confirmation names them one by one rather than asking a
// generic "are you sure": the cost of this change is entirely in what it
// silently disconnects.
function WorkspaceAddress() {
  const { t } = useTranslation();
  const [current, setCurrent] = useState<OrgSlug | null>(null);
  const [slug, setSlug] = useState("");
  const [busy, setBusy] = useState(false);
  const [confirming, setConfirming] = useState(false);

  useEffect(() => {
    api.orgSlug().then((s) => { setCurrent(s); setSlug(s.slug); }).catch(() => {});
  }, []);

  async function save() {
    setBusy(true);
    try {
      const next = await api.updateOrgSlug(slug.trim());
      setCurrent(next);
      setSlug(next.slug);
      setConfirming(false);
      toast.success(t("settings.workspaceAddressSaved"));
    } catch (err: any) {
      toast.error(err.message || t("settings.workspaceAddressFailed"));
    } finally {
      setBusy(false);
    }
  }

  if (!current) return null;
  const changed = slug.trim() !== "" && slug.trim() !== current.slug;

  return (
    <GlassCard className="p-6 space-y-4">
      <h2 className="text-base font-bold text-foreground">{t("settings.workspaceAddress")}</h2>
      <form className="max-w-md" onSubmit={(e) => { e.preventDefault(); setConfirming(true); }}>
        <Field label={t("settings.workspaceAddressLabel")} hint={t("settings.workspaceAddressHint")}>
          <div className="flex gap-2">
            <input
              className="input flex-1 text-sm font-mono"
              value={slug}
              onChange={(e) => setSlug(e.target.value)}
              placeholder="acme"
              required
            />
            <Button type="submit" variant="primary" disabled={!changed} className="shrink-0">
              {t("settings.update")}
            </Button>
          </div>
        </Field>
      </form>

      {confirming && (
        <Modal title={t("settings.workspaceAddressConfirmTitle")} onClose={() => setConfirming(false)}>
          <div className="space-y-4">
            <p className="text-sm text-foreground/70">
              {t("settings.workspaceAddressConfirmDesc", { from: current.slug, to: slug.trim() })}
            </p>
            <ul className="space-y-2 rounded-xl border border-foreground/[0.05] bg-well p-4 text-xs text-foreground/60">
              <li>
                <div>{t("settings.workspaceAddressBreaksBilling")}</div>
                <code className="mt-1 block break-all text-[11px] text-foreground/45">/api/webhook/{slug.trim()}/billing/…</code>
              </li>
              <li>
                <div>{t("settings.workspaceAddressBreaksSso")}</div>
                <code className="mt-1 block break-all text-[11px] text-foreground/45">/api/sso/{slug.trim()}/callback</code>
              </li>
            </ul>
            <p className="text-xs text-foreground/50">{t("settings.workspaceAddressNoAlias")}</p>
            <div className="flex justify-end gap-3 pt-2">
              <Button variant="ghost" onClick={() => setConfirming(false)}>{t("settings.cancel")}</Button>
              <Button variant="primary" onClick={save} disabled={busy}>
                {busy ? t("settings.updating") : t("settings.workspaceAddressConfirm")}
              </Button>
            </div>
          </div>
        </Modal>
      )}
    </GlassCard>
  );
}

// TenantAddress shows the workspace's automatic tenant subdomain — its
// address on a cloud instance with a shared base domain configured — with the
// one-click copy the whole app's Code component provides. Hidden entirely when
// no base domain is configured (tenantSubdomain is "").
function TenantAddress() {
  const { t } = useTranslation();
  const [subdomain, setSubdomain] = useState("");

  useEffect(() => {
    api.settings()
      .then((s: Settings) => setSubdomain(s.tenantSubdomain || ""))
      .catch(() => {});
  }, []);

  if (!subdomain) return null;

  return (
    <GlassCard className="p-6 space-y-2">
      <h2 className="text-base font-bold text-foreground">{t("settings.tenantSubdomain")}</h2>
      <p className="text-xs text-foreground/50">{t("settings.tenantSubdomainHint")}</p>
      <Code>{subdomain}</Code>
    </GlassCard>
  );
}

export function GeneralSettings() {
  const { t } = useTranslation();
  const [workspaceName, setWorkspaceName] = useState("");
  const [workspaceBusy, setWorkspaceBusy] = useState(false);
  const [role, setRole] = useState<string | null>(null);
  const [exporting, setExporting] = useState(false);
  const [purging, setPurging] = useState(false);
  const [showDeleteModal, setShowDeleteModal] = useState(false);
  const [deleteConfirmationText, setDeleteConfirmationText] = useState("");

  useEffect(() => {
    api.me().then((me) => api.orgs().then((orgs) => {
      const o = orgs.find((x) => x.id === me.orgId);
      if (o) {
        setWorkspaceName(o.name);
        setRole(o.role || "member");
      }
    })).catch(() => {});
  }, []);

  async function renameWorkspace(e: React.FormEvent) {
    e.preventDefault();
    if (!workspaceName.trim()) return;
    setWorkspaceBusy(true);
    try {
      await api.updateOrg({ name: workspaceName });
      toast.success(t("settings.saved"));
      // Tell the shell to refresh the workspace list/switcher in place instead
      // of a full-page reload.
      window.dispatchEvent(new Event("octarq:orgs-changed"));
    } catch (err: any) { toast.error(err.message || t("settings.renameFailed")); } finally { setWorkspaceBusy(false); }
  }

  async function handleExport() {
    setExporting(true);
    try {
      const data = await api.exportWorkspaceData();
      const blob = new Blob([JSON.stringify(data, null, 2)], { type: "application/json" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `workspace-data-${new Date().toISOString().split("T")[0]}.json`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    } catch (err: any) {
      toast.error(err.message || t("settings.exportFailed"));
    } finally {
      setExporting(false);
    }
  }

  async function handlePurge() {
    if (deleteConfirmationText !== "DELETE MY DATA") {
      toast.error(t("settings.typeToConfirm", { phrase: "DELETE MY DATA" }));
      return;
    }
    setPurging(true);
    try {
      await api.purgeWorkspaceData();
      toast.success(t("settings.workspaceDeleted"));
      setShowDeleteModal(false);
      setDeleteConfirmationText("");
      // The active workspace no longer exists — reload to re-establish a valid
      // session/active org. Brief delay so the confirmation toast is seen.
      setTimeout(() => window.location.reload(), 700);
    } catch (err: any) {
      toast.error(err.message || t("settings.deleteWorkspaceFailed"));
    } finally {
      setPurging(false);
    }
  }

  const isAdminOrOwner = role === "admin" || role === "owner";

  return (
    <div className="space-y-6">
      <PageHeader title={t("settings.generalTitle")} description={t("settings.generalDescription")} />

      <GlassCard className="p-6 space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-base font-bold text-foreground">{t("settings.workspaceProfile")}</h2>
        </div>
        <form onSubmit={renameWorkspace} className="max-w-md">
          <Field label={t("settings.workspaceNameLabel")} hint={t("settings.workspaceNameHint")}>
            <div className="flex gap-2">
              <input className="input flex-1 text-sm" value={workspaceName} onChange={(e) => setWorkspaceName(e.target.value)} placeholder="Acme Production" required />
              <Button type="submit" variant="primary" disabled={workspaceBusy || !workspaceName.trim()} className="shrink-0">
                {workspaceBusy ? t("settings.updating") : t("settings.update")}
              </Button>
            </div>
          </Field>
        </form>
      </GlassCard>

      {role === "owner" && <WorkspaceAddress />}

      <TenantAddress />

      <ExtensionSlot name="settings-workspace" />

      {isAdminOrOwner && (
        <GlassCard className="p-6 space-y-4">
          <div>
            <h2 className="text-base font-bold text-foreground">{t("settings.exportWorkspaceData")}</h2>
            <p className="text-xs text-foreground/50 mt-1">
              {t("settings.exportWorkspaceDesc")}
            </p>
          </div>
          <div className="pt-2">
            <Button variant="outline" onClick={handleExport} disabled={exporting}>
              {exporting ? t("settings.preparing") : t("settings.downloadMyData")}
            </Button>
          </div>
        </GlassCard>
      )}

      {isAdminOrOwner && (
        <>
          <GlassCard className="p-6 border-danger-border bg-danger-bg/50 space-y-6">
            <div className="flex items-center gap-2 text-danger-fg">
              <ShieldAlert size={20} />
              <h2 className="text-base font-bold">{t("settings.dangerZone")}</h2>
            </div>
            <p className="text-xs text-foreground/60">
              {t("settings.dangerZoneDesc")}
            </p>
            <div className="pt-2">
              <Button variant="danger" onClick={() => setShowDeleteModal(true)}>
                {t("settings.deleteWorkspace")}
              </Button>
            </div>
          </GlassCard>
        </>
      )}

      {showDeleteModal && (
        <Modal title={t("settings.deleteWorkspaceModalTitle")} onClose={() => { setShowDeleteModal(false); setDeleteConfirmationText(""); }}>
          <div className="space-y-4">
            <p className="text-sm text-foreground/70">
              {t("settings.deleteWorkspaceModalDesc")}
            </p>
            <p className="text-sm text-foreground/70">
              {t("settings.confirmTypePre")}<span className="font-mono font-bold text-danger-fg select-all">DELETE MY DATA</span>{t("settings.confirmTypePost")}
            </p>
            <input
              type="text"
              className="input w-full text-sm font-mono text-center border-danger-border focus:border-danger-fg"
              value={deleteConfirmationText}
              onChange={(e) => setDeleteConfirmationText(e.target.value)}
              placeholder="DELETE MY DATA"
            />
            <div className="flex justify-end gap-3 pt-2">
              <Button variant="ghost" onClick={() => { setShowDeleteModal(false); setDeleteConfirmationText(""); }}>
                {t("settings.cancel")}
              </Button>
              <Button
                variant="danger"
                disabled={deleteConfirmationText !== "DELETE MY DATA" || purging}
                onClick={handlePurge}
              >
                {purging ? t("settings.deleting") : t("settings.permanentlyDelete")}
              </Button>
            </div>
          </div>
        </Modal>
      )}
    </div>
  );
}
