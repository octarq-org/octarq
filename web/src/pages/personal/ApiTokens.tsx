import { useEffect, useState } from "react";
import { Key, ClipboardCopy } from "lucide-react";
import {
  Badge,
  Button,
  confirmDialog,
  Empty,
  Field,
  GlassCard,
  Input,
  Modal,
  PageHeader,
  Select,
  timeAgo,
  toast,
} from "@octarq/plugin-sdk";
import { api, ApiError, Token } from "../../api";
import { useTranslation } from "../../i18n";
import { roleSatisfies, useCurrentRole } from "../../shell/role";
import { McpConnectCard } from "../../components/McpConnectCard";

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

  async function load() {
    setLoading(true);
    try {
      setTokens(await api.tokens());
    } catch (e: unknown) {
      if (import.meta.env.DEV) console.error(e);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load();
  }, []);

  async function remove(id: number) {
    if (!(await confirmDialog(t("personal.revokeConfirm")))) return;
    try {
      await api.deleteToken(id);
      toast.success(t("personal.tokenRevoked"));
      load();
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : t("personal.tokenRevokeFailed");
      toast.error(msg);
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("personal.tokensTitle")}
        description={t("personal.tokensDesc")}
        action={
          <Button variant="primary" onClick={() => setCreating(true)} className="text-xs">
            {t("personal.newToken")}
          </Button>
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
            {tokens.map((tokenItem) => (
              <div key={tokenItem.id} className="flex flex-col sm:flex-row sm:items-center justify-between p-4 gap-3 sm:gap-4 group">
                <div>
                  <div className="font-semibold text-sm text-foreground">{tokenItem.name}</div>
                  <div className="text-xs text-foreground/50 mt-1 flex flex-wrap items-center gap-2">
                    <code className="rounded bg-foreground/5 px-1.5 py-0.5 border border-foreground/[0.04]">{tokenItem.prefix}…</code>
                    <Badge>{t(SCOPE_LABEL[tokenItem.role || "member"])}</Badge>
                    {tokenItem.note && <span className="text-foreground/40">{tokenItem.note}</span>}
                  </div>
                </div>
                <div className="flex flex-wrap items-center justify-between sm:justify-end gap-3 sm:gap-4 w-full sm:w-auto pt-2 sm:pt-0 border-t sm:border-t-0 border-foreground/[0.04]">
                  <div className="text-[11px] text-foreground/50 flex flex-col sm:items-end">
                    <span>{tokenItem.lastUsedAt ? t("personal.usedAgo", { time: timeAgo(tokenItem.lastUsedAt) }) : t("personal.neverUsed")}</span>
                    {tokenItem.expiresAt && (
                      <span className={new Date(tokenItem.expiresAt).getTime() < Date.now() ? "text-danger-fg font-medium" : "text-foreground/40"}>
                        {new Date(tokenItem.expiresAt).getTime() < Date.now()
                          ? t("personal.expired")
                          : t("personal.expiresAt", { time: timeAgo(tokenItem.expiresAt) })}
                      </span>
                    )}
                  </div>
                  <div className="flex items-center gap-1.5">
                    <Button
                      variant="secondary"
                      onClick={() => setEditingToken(tokenItem)}
                      className="text-xs min-h-[44px] sm:min-h-0 py-2 sm:py-1 px-3 sm:px-2.5 border-0"
                    >
                      {t("personal.edit")}
                    </Button>
                    <Button
                      variant="danger"
                      onClick={() => remove(tokenItem.id)}
                      className="text-xs min-h-[44px] sm:min-h-0 py-2 sm:py-1 px-3 sm:px-2.5 border-0"
                    >
                      {t("personal.revoke")}
                    </Button>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </GlassCard>

      <McpConnectCard tokens={tokens} />

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
              className="gap-1.5"
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
  const [role, setRole] = useState<"member" | "admin" | "owner">("member");
  const [expiresInDays, setExpiresInDays] = useState<number>(0);
  const [busy, setBusy] = useState(false);
  const { t } = useTranslation();
  const { role: myRole, isInstanceAdmin } = useCurrentRole();

  const mintable = (["member", "admin", "owner"] as const).filter((r) =>
    roleSatisfies(r, myRole, isInstanceAdmin),
  );

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      const res = await api.createToken({ name, note, role, expiresInDays });
      toast.success(t("personal.tokenGeneratedTitle"));
      onCreated(res.token);
    } catch (err: unknown) {
      toast.error(err instanceof ApiError ? err.message : t("personal.failed"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal title={t("personal.generateTokenTitle")} onClose={onClose}>
      <form onSubmit={submit} className="space-y-4">
        <Field label={t("personal.tokenNameLabel")} hint={t("personal.tokenNameHint")}>
          <Input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={t("personal.tokenNamePlaceholder")}
            required
            autoFocus
          />
        </Field>
        <Field label={t("personal.tokenRoleLabel")} hint={t("personal.tokenRoleHint")}>
          <Select
            value={role}
            onValueChange={(v) => setRole(v as typeof role)}
            options={mintable.map((r) => ({ value: r, label: t(MINT_ROLE_LABEL[r]) }))}
          />
        </Field>
        <Field label={t("personal.tokenExpiryLabel")} hint={t("personal.tokenExpiryHint")}>
          <Select
            value={String(expiresInDays)}
            onValueChange={(v) => setExpiresInDays(Number(v))}
            options={[
              { value: "0", label: t("personal.expiryNever") },
              { value: "7", label: t("personal.expiry7Days") },
              { value: "30", label: t("personal.expiry30Days") },
              { value: "90", label: t("personal.expiry90Days") },
              { value: "365", label: t("personal.expiry365Days") },
            ]}
          />
        </Field>
        <Field label={t("personal.tokenRemarksLabel")} hint={t("personal.tokenRemarksHint")}>
          <Input className="text-sm" value={note} onChange={(e) => setNote(e.target.value)} placeholder={t("personal.tokenRemarksPlaceholder")} />
        </Field>
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
  const [busy, setBusy] = useState(false);
  const { t } = useTranslation();
  const { role: myRole, isInstanceAdmin } = useCurrentRole();

  const mintable = (["member", "admin", "owner"] as const).filter((r) =>
    roleSatisfies(r, myRole, isInstanceAdmin),
  );

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      const d: { name?: string; note?: string; role?: "member" | "admin" | "owner"; expiresInDays?: number } = {};
      if (name.trim() !== token.name) d.name = name.trim();
      if (note !== (token.note || "")) d.note = note;
      if (role !== (token.role || "member")) d.role = role;
      if (expiryOption !== "keep") {
        d.expiresInDays = Number(expiryOption);
      }
      await api.updateToken(token.id, d);
      toast.success(t("settings.saved"));
      onUpdated();
    } catch (err: unknown) {
      toast.error(err instanceof ApiError ? err.message : t("personal.failed"));
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
          <Input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={t("personal.tokenNamePlaceholder")}
            required
            autoFocus
          />
        </Field>
        <Field label={t("personal.tokenRoleLabel")} hint={t("personal.tokenRoleHint")}>
          <Select
            value={role}
            onValueChange={(v) => setRole(v as typeof role)}
            options={mintable.map((r) => ({ value: r, label: t(MINT_ROLE_LABEL[r]) }))}
          />
        </Field>
        <Field label={t("personal.tokenExpiryLabel")} hint={t("personal.tokenExpiryHint")}>
          <Select
            value={expiryOption}
            onValueChange={setExpiryOption}
            options={[
              { value: "keep", label: t("personal.expiryKeep") },
              { value: "0", label: t("personal.expiryNever") },
              { value: "7", label: t("personal.expiry7Days") },
              { value: "30", label: t("personal.expiry30Days") },
              { value: "90", label: t("personal.expiry90Days") },
              { value: "365", label: t("personal.expiry365Days") },
            ]}
          />
        </Field>
        <Field label={t("personal.tokenRemarksLabel")} hint={t("personal.tokenRemarksHint")}>
          <Input className="text-sm" value={note} onChange={(e) => setNote(e.target.value)} placeholder={t("personal.tokenRemarksPlaceholder")} />
        </Field>
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
