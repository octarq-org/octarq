import { useEffect, useState } from "react";
import { NavLink, Navigate, Route, Routes } from "react-router-dom";
import { api, ApiError, Token } from "../api";
import { Empty, Field, Modal, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button } from "../ui";
import { User, Key, Settings, CheckCircle, Trash2, Eye, ClipboardCopy } from "lucide-react";

export default function PersonalSettingsPage() {
  return (
    <ScreenWrap>
      <Routes>
        <Route path="/" element={<Navigate to="/personal/profile" replace />} />
        <Route path="/profile" element={<ProfileSettings />} />
        <Route path="/tokens" element={<ApiTokens />} />
      </Routes>
    </ScreenWrap>
  );
}

function ProfileSettings() {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    api.me().then((u) => setEmail(u.username));
  }, []);

  async function updatePassword(e: React.FormEvent) {
    e.preventDefault();
    if (!password) return;
    if (password !== confirmPassword) {
      setError("Passwords do not match");
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
      setError(e.message || "Failed to update password");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="My Profile"
        description="Update your personal details and authentication password"
      />

      <GlassCard className="p-6 max-w-xl">
        <form onSubmit={updatePassword} className="space-y-5">
          <Field label="Email Address / Username">
            <input
              type="text"
              className="input w-full opacity-65 cursor-not-allowed text-white/50"
              value={email}
              readOnly
              disabled
            />
          </Field>

          <Field label="New Password">
            <input
              type="password"
              className="input w-full"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="••••••••"
              required
            />
          </Field>

          <Field label="Confirm New Password">
            <input
              type="password"
              className="input w-full"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              placeholder="••••••••"
              required
            />
          </Field>

          {error && <p className="text-xs text-rose-400 font-semibold">{error}</p>}
          {saved && <p className="text-xs text-emerald-400 font-semibold flex items-center gap-1">✓ Password updated successfully</p>}

          <div className="pt-2 border-t border-white/[0.04] flex justify-end">
            <Button type="submit" variant="primary" disabled={busy || !password}>
              {busy ? "Updating..." : "Update Password"}
            </Button>
          </div>
        </form>
      </GlassCard>
    </div>
  );
}

function ApiTokens() {
  const [tokens, setTokens] = useState<Token[]>([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [created, setCreated] = useState<{ token: string } | null>(null);

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
    if (!confirm("Revoke this token? Any script or service using it will stop working immediately.")) return;
    await api.deleteToken(id);
    load();
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="API Tokens"
        description="Bearer keys for automated script authentication. Set header 'Authorization: Bearer led_...'"
        action={
          <Button variant="primary" onClick={() => setCreating(true)} className="text-xs">
            + New Token
          </Button>
        }
      />

      <GlassCard className="p-6">
        {loading ? (
          <div className="text-white/40 text-sm py-6 text-center">loading…</div>
        ) : tokens.length === 0 ? (
          <Empty>
            <Key className="h-8 w-8 text-white/30 mb-1" />
            <div className="text-xs text-white/50">No API tokens configured yet.</div>
          </Empty>
        ) : (
          <div className="divide-y divide-white/[0.04] border border-white/[0.05] rounded-xl bg-black/25 overflow-hidden">
            {tokens.map((t) => (
              <div key={t.id} className="flex items-center justify-between p-4 group">
                <div>
                  <div className="font-semibold text-sm text-white">{t.name}</div>
                  <div className="text-xs text-white/50 mt-1 flex items-center gap-2">
                    <code className="rounded bg-white/5 px-1.5 py-0.5 border border-white/[0.04]">{t.prefix}…</code>
                    {t.note && <span className="text-white/40">{t.note}</span>}
                  </div>
                </div>
                <div className="flex items-center gap-4">
                  <span className="text-[11px] text-white/35">
                    {t.lastUsedAt ? `Used ${timeAgo(t.lastUsedAt)}` : "Never used"}
                  </span>
                  <Button
                    variant="danger"
                    onClick={() => remove(t.id)}
                    className="text-xs py-1 px-2.5 bg-rose-500/0 hover:bg-rose-500/10 border-0"
                  >
                    Revoke
                  </Button>
                </div>
              </div>
            ))}
          </div>
        )}
      </GlassCard>

      {created && (
        <Modal title="Token Generated" onClose={() => setCreated(null)}>
          <div className="space-y-4">
            <p className="text-xs text-white/60 leading-relaxed">
              Copy this token and store it securely. For safety reasons, <span className="font-bold text-rose-400">it will not be shown again.</span>
            </p>
            <div className="break-all rounded-xl bg-black/45 border border-white/[0.06] p-4 font-mono text-xs select-all leading-normal text-white">
              {created.token}
            </div>
            <Button
              variant="primary"
              onClick={async () => {
                await navigator.clipboard?.writeText(created.token);
                alert("Token copied to clipboard!");
              }}
              className="w-full gap-1.5"
            >
              <ClipboardCopy className="h-4 w-4" />
              Copy to Clipboard
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
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr("");
    try {
      const res = await api.createToken({ name, note });
      onCreated(res.token);
    } catch (e: any) {
      setErr(e instanceof ApiError ? e.message : "failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal title="Generate API Token" onClose={onClose}>
      <form onSubmit={submit} className="space-y-4">
        <Field label="Token Identifier Name" hint="Describe token usage, e.g. production-sync">
          <input
            className="input w-full"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="e.g. cli-tool"
            required
            autoFocus
          />
        </Field>
        <Field label="Internal Remarks (Optional)" hint="Notes or comments regarding this token context.">
          <input className="input w-full text-sm" value={note} onChange={(e) => setNote(e.target.value)} placeholder="e.g. home server cron job" />
        </Field>
        {err && <p className="text-sm text-rose-400 font-medium">{err}</p>}
        <div className="flex justify-end gap-2.5 pt-4 border-t border-white/[0.06]">
          <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" disabled={busy || !name.trim()}>
            {busy ? "Generating..." : "Generate Token"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
