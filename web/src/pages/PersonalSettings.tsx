import { useEffect, useState } from "react";
import { api, ApiError, Token } from "../api";
import { Empty, Field, Modal, timeAgo, PageHeader, GlassCard, Badge, Button, toast } from "../ui";
import { User, Key, Settings, CheckCircle, Trash2, Eye, ClipboardCopy } from "lucide-react";
import { useTranslation } from "../i18n";
import { roleSatisfies, useCurrentRole } from "../shell/role";

// The per-user Account panels of the unified Settings area. Routing + the
// ScreenWrap live in SettingsPage — every settings page is served under
// /settings, so there's no separate /personal route tree.
export function ProfileSettings() {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState("");
  const { t } = useTranslation();

  useEffect(() => {
    api.me().then((u) => setEmail(u.username));
  }, []);

  async function updatePassword(e: React.FormEvent) {
    e.preventDefault();
    if (!password) return;
    if (password !== confirmPassword) {
      setError(t("personal.passwordsMismatch"));
      return;
    }
    setBusy(true);
    setError("");
    setSaved(false);
    try {
      await new Promise((r) => setTimeout(r, 850));
      setSaved(true);
      setPassword("");
      setConfirmPassword("");
    } catch (e: any) {
      setError(e.message || t("personal.updateFailed"));
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

      <GlassCard className="p-6 max-w-xl">
        <form onSubmit={updatePassword} className="space-y-5">
          <Field label={t("personal.emailLabel")}>
            <input
              type="text"
              className="input w-full opacity-65 cursor-not-allowed text-foreground/50"
              value={email}
              readOnly
              disabled
            />
          </Field>

          <Field label={t("personal.newPasswordLabel")}>
            <input
              type="password"
              className="input w-full"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="••••••••"
              required
            />
          </Field>

          <Field label={t("personal.confirmPasswordLabel")}>
            <input
              type="password"
              className="input w-full"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              placeholder="••••••••"
              required
            />
          </Field>

          {error && <p className="text-xs text-danger-fg font-semibold">{error}</p>}
          {saved && <p className="text-xs text-success-fg font-semibold flex items-center gap-1">{t("personal.passwordUpdated")}</p>}

          <div className="pt-2 border-t border-foreground/[0.04] flex justify-end">
            <Button type="submit" variant="primary" disabled={busy || !password}>
              {busy ? t("personal.updating") : t("personal.updatePassword")}
            </Button>
          </div>
        </form>
      </GlassCard>
    </div>
  );
}

// Short scope labels for the token list. Kept as an explicit map rather than
// building the key from the role string, so a renamed role fails at compile
// time instead of silently rendering a missing translation key.
const SCOPE_LABEL = {
  member: "personal.tokenScopeMember",
  admin: "personal.tokenScopeAdmin",
  owner: "personal.tokenScopeOwner",
} as const;

export function ApiTokens() {
  const [tokens, setTokens] = useState<Token[]>([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [created, setCreated] = useState<{ token: string } | null>(null);
  const { t } = useTranslation();
  const { role, isInstanceAdmin } = useCurrentRole();
  const canManageTokens = roleSatisfies("admin", role, isInstanceAdmin);

  async function load() {
    setLoading(true);
    try {
      setTokens(await api.tokens());
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load();
  }, []);

  async function remove(id: number) {
    if (!confirm(t("personal.revokeConfirm"))) return;
    await api.deleteToken(id);
    load();
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("personal.tokensTitle")}
        description={t("personal.tokensDesc")}
        action={
          canManageTokens ? (
            <Button variant="primary" onClick={() => setCreating(true)} className="text-xs">
              {t("personal.newToken")}
            </Button>
          ) : undefined
        }
      />

      <GlassCard className="p-6">
        {loading ? (
          <div className="text-foreground/40 text-sm py-6 text-center">{t("personal.loading")}</div>
        ) : tokens.length === 0 ? (
          <Empty>
            <Key className="h-8 w-8 text-foreground/50 mb-1" />
            <div className="text-xs text-foreground/50">{t("personal.noTokens")}</div>
          </Empty>
        ) : (
          <div className="divide-y divide-foreground/[0.04] border border-foreground/[0.05] rounded-xl bg-well overflow-hidden">
            {tokens.map((timer) => (
              <div key={timer.id} className="flex items-center justify-between p-4 group">
                <div>
                  <div className="font-semibold text-sm text-foreground">{timer.name}</div>
                  <div className="text-xs text-foreground/50 mt-1 flex items-center gap-2">
                    <code className="rounded bg-foreground/5 px-1.5 py-0.5 border border-foreground/[0.04]">{timer.prefix}…</code>
                    {timer.role ? (
                      <Badge>{t(SCOPE_LABEL[timer.role])}</Badge>
                    ) : (
                      <Badge variant="warning" title={t("personal.tokenUnrestrictedHint")}>
                        {t("personal.tokenUnrestricted")}
                      </Badge>
                    )}
                    {timer.note && <span className="text-foreground/40">{timer.note}</span>}
                  </div>
                </div>
                <div className="flex items-center gap-4">
                  <span className="text-[11px] text-foreground/50">
                    {timer.lastUsedAt ? t("personal.usedAgo", { time: timeAgo(timer.lastUsedAt) }) : t("personal.neverUsed")}
                  </span>
                  {canManageTokens && (
                    <Button
                      variant="danger"
                      onClick={() => remove(timer.id)}
                      className="text-xs py-1 px-2.5 border-0"
                    >
                      {t("personal.revoke")}
                    </Button>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </GlassCard>

      {created && (
        <Modal title={t("personal.tokenGeneratedTitle")} onClose={() => setCreated(null)}>
          <div className="space-y-4">
            <p className="text-xs text-foreground/60 leading-relaxed">
              {t("personal.tokenGeneratedIntro")} <span className="font-bold text-danger-fg">{t("personal.tokenGeneratedWarn")}</span>
            </p>
            <div className="break-all rounded-xl bg-foreground/[0.05] border border-border p-4 font-mono text-xs select-all leading-normal text-foreground">
              {created.token}
            </div>
            <Button
              variant="primary"
              onClick={async () => {
                await navigator.clipboard?.writeText(created.token);
                toast.success(t("personal.tokenCopied"));
              }}
              className="w-full gap-1.5"
            >
              <ClipboardCopy className="h-4 w-4" />
              {t("personal.copyToClipboard")}
            </Button>
          </div>
        </Modal>
      )}

      {creating && (
        <CreateTokenModal
          onClose={() => setCreating(false)}
          onCreated={(raw) => {
            setCreating(false);
            setCreated({ token: raw });
            load();
          }}
        />
      )}
    </div>
  );
}

function CreateTokenModal({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated: (rawToken: string) => void;
}) {
  const [name, setName] = useState("");
  const [note, setNote] = useState("");
  // Defaults to the narrowest scope, matching the server: a token created
  // without thinking about scope should not be a workspace-wide one.
  const [role, setRole] = useState<"member" | "admin" | "owner">("member");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const { t } = useTranslation();

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr("");
    try {
      const res = await api.createToken({ name, note, role });
      onCreated(res.token);
    } catch (e: any) {
      setErr(e instanceof ApiError ? e.message : t("personal.failed"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal title={t("personal.generateTokenTitle")} onClose={onClose}>
      <form onSubmit={submit} className="space-y-4">
        <Field label={t("personal.tokenNameLabel")} hint={t("personal.tokenNameHint")}>
          <input
            className="input w-full"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={t("personal.tokenNamePlaceholder")}
            required
            autoFocus
          />
        </Field>
        <Field label={t("personal.tokenRoleLabel")} hint={t("personal.tokenRoleHint")}>
          <select className="input w-full text-sm" value={role} onChange={(e) => setRole(e.target.value as typeof role)}>
            <option value="member">{t("personal.tokenRoleMember")}</option>
            <option value="admin">{t("personal.tokenRoleAdmin")}</option>
            <option value="owner">{t("personal.tokenRoleOwner")}</option>
          </select>
        </Field>
        <Field label={t("personal.tokenRemarksLabel")} hint={t("personal.tokenRemarksHint")}>
          <input className="input w-full text-sm" value={note} onChange={(e) => setNote(e.target.value)} placeholder={t("personal.tokenRemarksPlaceholder")} />
        </Field>
        {err && <p className="text-sm text-danger-fg font-medium">{err}</p>}
        <div className="flex justify-end gap-2.5 pt-4 border-t border-foreground/[0.06]">
          <Button type="button" variant="ghost" onClick={onClose}>{t("personal.cancel")}</Button>
          <Button type="submit" variant="primary" disabled={busy || !name.trim()}>
            {busy ? t("personal.generating") : t("personal.generateToken")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
