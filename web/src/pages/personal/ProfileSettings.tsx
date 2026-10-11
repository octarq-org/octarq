import { useEffect, useState } from "react";
import { Trash2 } from "lucide-react";
import {
  Button,
  confirmPassword,
  Field,
  GlassCard,
  Input,
  Modal,
  PageHeader,
  toast,
} from "@octarq/plugin-sdk";
import { api, ApiError } from "../../api";
import { useTranslation } from "../../i18n";

export function ProfileSettings() {
  const [email, setEmail] = useState("");
  const [newEmail, setNewEmail] = useState("");
  const [changingEmail, setChangingEmail] = useState(false);
  const [emailBusy, setEmailBusy] = useState(false);

  const [currentPassword, setCurrentPassword] = useState("");
  const [password, setPassword] = useState("");
  const [repeatPassword, setRepeatPassword] = useState("");
  const [busy, setBusy] = useState(false);

  const [showDeleteModal, setShowDeleteModal] = useState(false);
  const [deleteConfirmationText, setDeleteConfirmationText] = useState("");
  const [deleting, setDeleting] = useState(false);

  const { t } = useTranslation();

  async function handleDeleteAccount() {
    if (deleteConfirmationText !== "DELETE MY ACCOUNT") {
      return;
    }
    setDeleting(true);
    try {
      await api.deleteUserAccount("DELETE MY ACCOUNT");
      toast.success(t("personal.deleteAccountSuccess"));
      setShowDeleteModal(false);
      setDeleteConfirmationText("");
      setTimeout(() => {
        window.location.href = "/login";
      }, 700);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : t("personal.deleteAccountFailed");
      toast.error(msg);
    } finally {
      setDeleting(false);
    }
  }

  const reloadUser = () => {
    api.me().then((u) => setEmail(u.email || u.username || ""));
  };

  useEffect(() => {
    reloadUser();
  }, []);

  async function handleEmailUpdate(e: React.FormEvent) {
    e.preventDefault();
    if (!newEmail) return;
    const confirmedPassword = await confirmPassword({
      message: t("personal.emailChangeConfirmMessage", { email: newEmail }),
      confirmLabel: t("personal.updateEmail"),
    });
    if (confirmedPassword === null) return;
    setEmailBusy(true);
    try {
      const res = await api.changeEmail(newEmail, confirmedPassword);
      setEmail(res.email);
      setNewEmail("");
      setChangingEmail(false);
      if (res.verificationSent) {
        toast.success(t("personal.emailVerificationSent", { email: res.email }));
      } else {
        toast.success(t("personal.emailUpdated"));
      }
      await reloadUser();
      window.dispatchEvent(new CustomEvent("octarq:auth-changed"));
    } catch (err: unknown) {
      if (err instanceof ApiError) {
        if (err.status === 409) {
          toast.error(t("personal.emailAlreadyExists"));
        } else if (err.status === 400 && err.message?.includes("external identity provider")) {
          toast.error(t("personal.ssoEmailChangeForbidden"));
        } else {
          toast.error(err.message || t("personal.updateFailed"));
        }
      } else {
        const msg = err instanceof Error ? err.message : t("personal.updateFailed");
        toast.error(msg);
      }
    } finally {
      setEmailBusy(false);
    }
  }

  async function updatePassword(e: React.FormEvent) {
    e.preventDefault();
    if (!password) return;
    if (password.length < 8) {
      toast.error(t("personal.passwordTooShort"));
      return;
    }
    if (password !== repeatPassword) {
      toast.error(t("personal.passwordsMismatch"));
      return;
    }
    setBusy(true);
    try {
      await api.changePassword(currentPassword, password);
      toast.success(t("personal.passwordUpdated"));
      setCurrentPassword("");
      setPassword("");
      setRepeatPassword("");
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : t("personal.updateFailed");
      toast.error(msg);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("personal.profileTitle")}
        description={t("personal.profileDesc")}
      />

      <GlassCard className="p-6 max-w-xl space-y-4">
        <div className="flex items-end justify-between gap-4">
          <div className="min-w-0">
            <div className="label">{t("personal.emailLabel")}</div>
            <div className="mt-1 truncate text-sm font-medium text-foreground">{email || "—"}</div>
          </div>
          {!changingEmail && (
            <Button
              variant="secondary"
              className="shrink-0 text-xs"
              onClick={() => {
                setChangingEmail(true);
              }}
            >
              {t("personal.changeEmail")}
            </Button>
          )}
        </div>

        {changingEmail && (
          <form onSubmit={handleEmailUpdate} className="space-y-4 border-t border-foreground/[0.04] pt-4">
            <Field label={t("personal.newEmailLabel")} hint={t("personal.newEmailHint")}>
              <Input
                type="email"
                spellCheck={false}
                value={newEmail}
                onChange={(e) => setNewEmail(e.target.value)}
                placeholder={t("personal.newEmailPlaceholder")}
                autoComplete="email"
                autoFocus
                required
              />
            </Field>

            <div className="flex justify-end gap-2">
              <Button
                type="button"
                variant="ghost"
                onClick={() => {
                  setChangingEmail(false);
                  setNewEmail("");
                }}
              >
                {t("personal.cancel")}
              </Button>
              <Button type="submit" variant="primary" disabled={emailBusy || !newEmail}>
                {emailBusy ? t("personal.updating") : t("personal.continue")}
              </Button>
            </div>
          </form>
        )}
      </GlassCard>

      <GlassCard className="p-6 max-w-xl">
        <form onSubmit={updatePassword} className="space-y-5">
          <Field label={t("personal.currentPasswordLabel")}>
            <Input
              type="password"
              value={currentPassword}
              onChange={(e) => setCurrentPassword(e.target.value)}
              placeholder="••••••••"
              autoComplete="current-password"
              required
            />
          </Field>

          <Field label={t("personal.newPasswordLabel")} hint={t("personal.newPasswordHint")}>
            <Input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="••••••••"
              autoComplete="new-password"
              required
            />
          </Field>

          <Field label={t("personal.confirmPasswordLabel")}>
            <Input
              type="password"
              value={repeatPassword}
              onChange={(e) => setRepeatPassword(e.target.value)}
              placeholder="••••••••"
              autoComplete="new-password"
              required
            />
          </Field>

          <div className="pt-2 border-t border-foreground/[0.04] flex justify-end">
            <Button type="submit" variant="primary" disabled={busy || !password || !currentPassword}>
              {busy ? t("personal.updating") : t("personal.updatePassword")}
            </Button>
          </div>
        </form>
      </GlassCard>

      <GlassCard className="p-6 space-y-3 border-danger/30">
        <div className="flex items-center gap-2 text-danger">
          <Trash2 size={18} />
          <h2 className="text-base font-bold">{t("personal.dangerZone")}</h2>
        </div>
        <p className="text-xs text-foreground/60">
          {t("personal.deleteAccountDesc")}
        </p>
        <div className="pt-2">
          <Button variant="danger" onClick={() => setShowDeleteModal(true)}>
            {t("personal.deleteAccountButton")}
          </Button>
        </div>
      </GlassCard>

      {showDeleteModal && (
        <Modal
          title={t("personal.deleteAccountTitle")}
          onClose={() => {
            setShowDeleteModal(false);
            setDeleteConfirmationText("");
          }}
        >
          <div className="space-y-4">
            <p className="text-sm text-foreground/70">
              {t("personal.deleteAccountModalDesc")}
            </p>
            <p className="text-sm text-foreground/70">
              {t("personal.confirmTypePre")}
              <span className="font-mono font-bold text-danger-fg select-all">DELETE MY ACCOUNT</span>
              {t("personal.confirmTypePost")}
            </p>
            <Input
              type="text"
              className="text-sm font-mono text-center border-danger-border focus:border-danger-fg"
              value={deleteConfirmationText}
              onChange={(e) => setDeleteConfirmationText(e.target.value)}
              placeholder="DELETE MY ACCOUNT"
            />
            <div className="flex justify-end gap-3 pt-2">
              <Button
                variant="ghost"
                onClick={() => {
                  setShowDeleteModal(false);
                  setDeleteConfirmationText("");
                }}
              >
                {t("personal.cancel")}
              </Button>
              <Button
                variant="danger"
                disabled={deleteConfirmationText !== "DELETE MY ACCOUNT" || deleting}
                onClick={handleDeleteAccount}
              >
                {deleting ? t("personal.deleting") : t("personal.permanentlyDelete")}
              </Button>
            </div>
          </div>
        </Modal>
      )}
    </div>
  );
}
