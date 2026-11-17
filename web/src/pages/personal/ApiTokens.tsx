import { Fragment, useEffect, useState } from "react";
import { Key, ClipboardCopy } from "lucide-react";
import {
  Badge,
  Button,
  confirmDialog,
  Field,
  Input,
  Modal,
  PageCard,
  PageEmpty,
  PageRow,
  PageRows,
  PageSection,
  PageShell,
  Select,
  Separator,
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
    <PageShell
      title={t("personal.tokensTitle")}
      description={t("personal.tokensDesc")}
      action={
        <Button variant="primary" size="sm" onClick={() => setCreating(true)}>
          {t("personal.newToken")}
        </Button>
      }
    >
      <PageSection>
        {loading ? (
          <PageCard>
            <div className="text-muted-foreground text-sm py-6 text-center">{t("personal.loading")}</div>
          </PageCard>
        ) : tokens.length === 0 ? (
          <PageEmpty title={t("personal.noTokens")} icon={<Key />} />
        ) : (
          <PageRows>
            {tokens.map((tokenItem, idx) => (
              <Fragment key={tokenItem.id}>
                {idx > 0 && <Separator />}
                <PageRow
                  title={
                    <span className="flex flex-wrap items-center gap-2">
                      <span>{tokenItem.name}</span>
                      <code className="rounded bg-well px-1.5 py-0.5 border border-border font-mono text-[11px] font-normal text-muted-foreground">
                        {tokenItem.prefix}…
                      </code>
                      <Badge>{t(SCOPE_LABEL[tokenItem.role || "member"])}</Badge>
                    </span>
                  }
                  description={
                    <span className="flex flex-wrap items-center gap-x-3 gap-y-1">
                      {tokenItem.note && <span>{tokenItem.note}</span>}
                      <span>{tokenItem.lastUsedAt ? t("personal.usedAgo", { time: timeAgo(tokenItem.lastUsedAt) }) : t("personal.neverUsed")}</span>
                      {tokenItem.expiresAt && (
                        <span className={new Date(tokenItem.expiresAt).getTime() < Date.now() ? "text-danger-fg font-medium" : "text-muted-foreground"}>
                          {new Date(tokenItem.expiresAt).getTime() < Date.now()
                            ? t("personal.expired")
                            : t("personal.expiresAt", { time: timeAgo(tokenItem.expiresAt) })}
                        </span>
                      )}
                    </span>
                  }
                  control={
                    <div className="flex items-center gap-1.5">
                      <Button
                        variant="secondary"
                        size="sm"
                        onClick={() => setEditingToken(tokenItem)}
                      >
                        {t("personal.edit")}
                      </Button>
                      <Button
                        variant="danger"
                        size="sm"
                        onClick={() => remove(tokenItem.id)}
                      >
                        {t("personal.revoke")}
                      </Button>
                    </div>
                  }
                />
              </Fragment>
            ))}
          </PageRows>
        )}
      </PageSection>

      <PageSection>
        <McpConnectCard tokens={tokens} />
      </PageSection>

      {created && (
        <Modal title={t("personal.tokenGeneratedTitle")} onClose={() => setCreated(null)}>
          <div className="space-y-4">
            <p className="text-xs text-muted-foreground leading-relaxed">
              {t("personal.tokenGeneratedIntro")} <span className="font-bold text-danger-fg">{t("personal.tokenGeneratedWarn")}</span>
            </p>
            <div className="break-all rounded-xl bg-well border border-border p-4 font-mono text-xs select-all leading-normal text-foreground">
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
    </PageShell>
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
        <div className="flex justify-end gap-2.5 pt-4 border-t border-border">
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
        <div className="text-xs text-muted-foreground rounded-xl bg-well p-3.5 border border-border leading-relaxed">
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
        <div className="flex justify-end gap-2.5 pt-4 border-t border-border">
          <Button type="button" variant="ghost" onClick={onClose}>{t("personal.cancel")}</Button>
          <Button type="submit" variant="primary" disabled={busy || !name.trim()}>
            {busy ? t("personal.saving") : t("personal.saveToken")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
