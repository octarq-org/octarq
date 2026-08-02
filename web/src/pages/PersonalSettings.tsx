import { useEffect, useState } from "react";
import { api, ApiError, Token } from "../api";
import { Empty, Field, Modal, timeAgo, PageHeader, GlassCard, Badge, Button, toast, confirmDialog } from "../ui";
import { User, Key, Settings, CheckCircle, Trash2, Eye, ClipboardCopy } from "lucide-react";
import { useTranslation } from "../i18n";
import { roleSatisfies, useCurrentRole } from "../shell/role";

// The per-user Account panels of the unified Settings area. Routing + the
// ScreenWrap live in SettingsPage — every settings page is served under
// /settings, so there's no separate /personal route tree.
export function ProfileSettings() {
  const [email, setEmail] = useState("");
  const [newEmail, setNewEmail] = useState("");
  const [emailPassword, setEmailPassword] = useState("");
  const [emailBusy, setEmailBusy] = useState(false);
  const [emailSuccessMsg, setEmailSuccessMsg] = useState("");
  const [emailError, setEmailError] = useState("");

  const [currentPassword, setCurrentPassword] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState("");
  const { t } = useTranslation();

  const reloadUser = () => {
    api.me().then((u) => setEmail(u.email || u.username || ""));
  };

  useEffect(() => {
    reloadUser();
  }, []);

  async function handleEmailUpdate(e: React.FormEvent) {
    e.preventDefault();
    if (!newEmail) return;
    setEmailBusy(true);
    setEmailError("");
    setEmailSuccessMsg("");
    try {
      const res = await api.changeEmail(newEmail, emailPassword);
      setEmail(res.email);
      setNewEmail("");
      setEmailPassword("");
      if (res.verificationSent) {
        setEmailSuccessMsg(t("personal.emailVerificationSent", { email: res.email }));
      } else {
        setEmailSuccessMsg(t("personal.emailUpdated"));
      }
      await reloadUser();
    } catch (err: any) {
      if (err instanceof ApiError) {
        if (err.status === 409) {
          setEmailError(t("personal.emailAlreadyExists"));
        } else if (err.status === 400 && err.message?.includes("external identity provider")) {
          setEmailError(t("personal.ssoEmailChangeForbidden"));
        } else {
          setEmailError(err.message || t("personal.updateFailed"));
        }
      } else {
        setEmailError(err?.message || t("personal.updateFailed"));
      }
    } finally {
      setEmailBusy(false);
    }
  }

  async function updatePassword(e: React.FormEvent) {
    e.preventDefault();
    if (!password) return;
    if (password.length < 8) {
      setError(t("personal.passwordTooShort"));
      return;
    }
    if (password !== confirmPassword) {
      setError(t("personal.passwordsMismatch"));
      return;
    }
    setBusy(true);
    setError("");
    setSaved(false);
    try {
      await api.changePassword(currentPassword, password);
      setSaved(true);
      setCurrentPassword("");
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
        <form onSubmit={handleEmailUpdate} className="space-y-5">
          <Field label={t("personal.emailLabel")}>
            <input
              type="text"
              className="input w-full opacity-65 cursor-not-allowed text-foreground/50"
              value={email}
              readOnly
              disabled
            />
          </Field>

          <Field label={t("personal.newEmailLabel")}>
            <input
              type="email"
              className="input w-full"
              value={newEmail}
              onChange={(e) => setNewEmail(e.target.value)}
              placeholder="you@domain.com"
              autoComplete="email"
              required
            />
          </Field>

          <Field label={t("personal.currentPasswordLabel")}>
            <input
              type="password"
              className="input w-full"
              value={emailPassword}
              onChange={(e) => setEmailPassword(e.target.value)}
              placeholder="••••••••"
              autoComplete="current-password"
            />
          </Field>

          {emailError && <p className="text-xs text-danger-fg font-semibold">{emailError}</p>}
          {emailSuccessMsg && <p className="text-xs text-success-fg font-semibold flex items-center gap-1">{emailSuccessMsg}</p>}

          <div className="pt-2 border-t border-foreground/[0.04] flex justify-end">
            <Button type="submit" variant="primary" disabled={emailBusy || !newEmail}>
              {emailBusy ? t("personal.updating") : t("personal.updateEmail")}
            </Button>
          </div>
        </form>
      </GlassCard>

      <GlassCard className="p-6 max-w-xl">
        <form onSubmit={updatePassword} className="space-y-5">
          <Field label={t("personal.currentPasswordLabel")}>
            <input
              type="password"
              className="input w-full"
              value={currentPassword}
              onChange={(e) => setCurrentPassword(e.target.value)}
              placeholder="••••••••"
              autoComplete="current-password"
              required
            />
          </Field>

          <Field label={t("personal.newPasswordLabel")} hint={t("personal.newPasswordHint")}>
            <input
              type="password"
              className="input w-full"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="••••••••"
              autoComplete="new-password"
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
              autoComplete="new-password"
              required
            />
          </Field>

          {error && <p className="text-xs text-danger-fg font-semibold">{error}</p>}
          {saved && <p className="text-xs text-success-fg font-semibold flex items-center gap-1">{t("personal.passwordUpdated")}</p>}

          <div className="pt-2 border-t border-foreground/[0.04] flex justify-end">
            <Button type="submit" variant="primary" disabled={busy || !password || !currentPassword}>
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
const MINT_ROLE_LABEL = {
  member: "personal.tokenRoleMember",
  admin: "personal.tokenRoleAdmin",
  owner: "personal.tokenRoleOwner",
} as const;

const SCOPE_LABEL = {
  member: "personal.tokenScopeMember",
  admin: "personal.tokenScopeAdmin",
  owner: "personal.tokenScopeOwner",
} as const;

export function ApiTokens() {
  const [tokens, setTokens] = useState<Token[]>([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [editingToken, setEditingToken] = useState<Token | null>(null);
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
    if (!(await confirmDialog(t("personal.revokeConfirm")))) return;
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
                    {/* An empty role is the least privilege, not the most: the
                        server reads it as "member", the same thing minting
                        defaults to. This used to render "unrestricted", which
                        was true then and is exactly backwards now. */}
                    <Badge>{t(SCOPE_LABEL[timer.role || "member"])}</Badge>
                    {timer.note && <span className="text-foreground/40">{timer.note}</span>}
                  </div>
                </div>
                <div className="flex items-center gap-4">
                  <div className="text-[11px] text-foreground/50 flex flex-col items-end">
                    <span>{timer.lastUsedAt ? t("personal.usedAgo", { time: timeAgo(timer.lastUsedAt) }) : t("personal.neverUsed")}</span>
                    {timer.expiresAt && (
                      <span className={new Date(timer.expiresAt).getTime() < Date.now() ? "text-danger-fg font-medium" : "text-foreground/40"}>
                        {new Date(timer.expiresAt).getTime() < Date.now()
                          ? t("personal.expired")
                          : t("personal.expiresAt", { time: timeAgo(timer.expiresAt) })}
                      </span>
                    )}
                  </div>
                  {canManageTokens && (
                    <div className="flex items-center gap-1.5">
                      <Button
                        variant="secondary"
                        onClick={() => setEditingToken(timer)}
                        className="text-xs py-1 px-2.5 border-0"
                      >
                        {t("personal.edit")}
                      </Button>
                      <Button
                        variant="danger"
                        onClick={() => remove(timer.id)}
                        className="text-xs py-1 px-2.5 border-0"
                      >
                        {t("personal.revoke")}
                      </Button>
                    </div>
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

      {editingToken && (
        <EditTokenModal
          token={editingToken}
          onClose={() => setEditingToken(null)}
          onUpdated={() => {
            setEditingToken(null);
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
  const [expiresInDays, setExpiresInDays] = useState<number>(0);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const { t } = useTranslation();
  const { role: myRole, isInstanceAdmin } = useCurrentRole();

  // A token caps its holder, it cannot outrank them: the server refuses to mint
  // one above the caller's own role. Offering the option anyway turns that into
  // a 403 after the user has filled the form in — so offer only what they can
  // actually create.
  const mintable = (["member", "admin", "owner"] as const).filter((r) =>
    roleSatisfies(r, myRole, isInstanceAdmin),
  );

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr("");
    try {
      const res = await api.createToken({ name, note, role, expiresInDays });
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
            {mintable.map((r) => (
              <option key={r} value={r}>{t(MINT_ROLE_LABEL[r])}</option>
            ))}
          </select>
        </Field>
        <Field label={t("personal.tokenExpiryLabel")} hint={t("personal.tokenExpiryHint")}>
          <select className="input w-full text-sm" value={expiresInDays} onChange={(e) => setExpiresInDays(Number(e.target.value))}>
            <option value={0}>{t("personal.expiryNever")}</option>
            <option value={7}>{t("personal.expiry7Days")}</option>
            <option value={30}>{t("personal.expiry30Days")}</option>
            <option value={90}>{t("personal.expiry90Days")}</option>
            <option value={365}>{t("personal.expiry365Days")}</option>
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

function EditTokenModal({
  token,
  onClose,
  onUpdated,
}: {
  token: Token;
  onClose: () => void;
  onUpdated: () => void;
}) {
  const [name, setName] = useState(token.name);
  const [note, setNote] = useState(token.note || "");
  const [role, setRole] = useState<"member" | "admin" | "owner">(
    (token.role || "member") as "member" | "admin" | "owner",
  );
  const [expiryOption, setExpiryOption] = useState<string>("keep");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const { t } = useTranslation();
  const { role: myRole, isInstanceAdmin } = useCurrentRole();

  const mintable = (["member", "admin", "owner"] as const).filter((r) =>
    roleSatisfies(r, myRole, isInstanceAdmin),
  );

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr("");
    try {
      const d: { name?: string; note?: string; role?: "member" | "admin" | "owner"; expiresInDays?: number } = {};
      if (name.trim() !== token.name) d.name = name.trim();
      if (note !== (token.note || "")) d.note = note;
      if (role !== (token.role || "member")) d.role = role;
      if (expiryOption !== "keep") {
        d.expiresInDays = Number(expiryOption);
      }
      await api.updateToken(token.id, d);
      onUpdated();
    } catch (e: any) {
      setErr(e instanceof ApiError ? e.message : t("personal.failed"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal title={t("personal.editTokenTitle")} onClose={onClose}>
      <form onSubmit={submit} className="space-y-4">
        <div className="text-xs text-foreground/70 rounded-xl bg-foreground/[0.04] p-3.5 border border-foreground/[0.06] leading-relaxed">
          {t("personal.editTokenHint")}
        </div>
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
            {mintable.map((r) => (
              <option key={r} value={r}>{t(MINT_ROLE_LABEL[r])}</option>
            ))}
          </select>
        </Field>
        <Field label={t("personal.tokenExpiryLabel")} hint={t("personal.tokenExpiryHint")}>
          <select className="input w-full text-sm" value={expiryOption} onChange={(e) => setExpiryOption(e.target.value)}>
            <option value="keep">{t("personal.expiryKeep")}</option>
            <option value="0">{t("personal.expiryNever")}</option>
            <option value="7">{t("personal.expiry7Days")}</option>
            <option value="30">{t("personal.expiry30Days")}</option>
            <option value="90">{t("personal.expiry90Days")}</option>
            <option value="365">{t("personal.expiry365Days")}</option>
          </select>
        </Field>
        <Field label={t("personal.tokenRemarksLabel")} hint={t("personal.tokenRemarksHint")}>
          <input className="input w-full text-sm" value={note} onChange={(e) => setNote(e.target.value)} placeholder={t("personal.tokenRemarksPlaceholder")} />
        </Field>
        {err && <p className="text-sm text-danger-fg font-medium">{err}</p>}
        <div className="flex justify-end gap-2.5 pt-4 border-t border-foreground/[0.06]">
          <Button type="button" variant="ghost" onClick={onClose}>{t("personal.cancel")}</Button>
          <Button type="submit" variant="primary" disabled={busy || !name.trim()}>
            {busy ? t("personal.saving") : t("personal.saveToken")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
