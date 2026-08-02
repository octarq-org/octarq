import { useEffect, useState } from "react";
import { api, OrgMember } from "../../api";
import { Field, Modal, timeAgo, PageHeader, GlassCard, Badge, Button, Select, toast, confirmDialog } from "../../ui";
import { Users } from "lucide-react";
import { useTranslation } from "../../i18n";
import { roleSatisfies, useCurrentRole } from "../../shell/role";

export function OrgMembersManager() {
  const { t } = useTranslation();
  // Every sibling settings page gates its mutating controls this way
  // (webhooks, notifications, tokens); this one did not, so a plain member was
  // shown an invite form and Remove buttons that the API answers with 403.
  const { role: myRole, isInstanceAdmin } = useCurrentRole();
  const canManage = roleSatisfies("admin", myRole, isInstanceAdmin);
  const [members, setMembers] = useState<OrgMember[]>([]);
  const [me, setMe] = useState<{ email?: string; username?: string; orgId?: number } | null>(null);
  const [loading, setLoading] = useState(true);
  const [email, setEmail] = useState("");
  const [role, setRole] = useState("member");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  // Set when the invited address had no account yet and the server minted an
  // accept link. Shown rather than discarded: the email carrying it is
  // best-effort server-side, so on an instance with no mail plugin this dialog
  // is the only copy anyone ever sees.
  const [inviteLink, setInviteLink] = useState("");

  async function load() {
    setLoading(true);
    try {
      const [mList, meUser] = await Promise.all([
        api.orgMembers(),
        api.me().catch(() => null),
      ]);
      setMembers(mList);
      setMe(meUser);
    } catch (e: any) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load();
  }, []);

  async function handleAdd(e: React.FormEvent) {
    e.preventDefault();
    if (!email) return;
    setBusy(true);
    setErr("");
    try {
      const res = await api.addOrgMember({ email, role });
      if (res?.inviteUrl) {
        setInviteLink(`${window.location.origin}${res.inviteUrl}`);
      }
      setEmail("");
      setRole("member");
      load();
    } catch (e: any) {
      setErr(e.message || t("settings.failedAddMember"));
    } finally {
      setBusy(false);
    }
  }

  // An admin cannot grant or change the owner role — the server refuses it, so
  // the control must not offer it either. Same rule the invite form's Select
  // would need; it is stated once here because this one is per-row.
  const canGrantOwner = roleSatisfies("owner", myRole, isInstanceAdmin);

  async function handleRoleChange(m: OrgMember, role: string) {
    if (role === m.role) return;
    setErr("");
    // Optimistic: the row is a Select, and leaving it showing the old role until
    // a refetch lands reads as "that didn't work".
    setMembers((list) => list.map((x) => (x.userId === m.userId ? { ...x, role } : x)));
    try {
      await api.updateOrgMemberRole(m.userId, role);
      toast.success(t("settings.memberRoleUpdated", { email: m.email }));
    } catch (e: any) {
      setMembers((list) => list.map((x) => (x.userId === m.userId ? { ...x, role: m.role } : x)));
      toast.error(e.message || t("settings.failedUpdateMemberRole"));
    }
  }

  async function handleRemove(userId: number) {
    if (!(await confirmDialog(t("settings.confirmRemoveMember")))) return;
    try {
      await api.deleteOrgMember(userId);
      load();
    } catch (e: any) {
      toast.error(e.message || t("settings.failedRemoveMember"));
    }
  }

  const getRoleTone = (r: string) => {
    if (r === "owner") return "green";
    if (r === "admin") return "indigo";
    return "neutral";
  };

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("settings.workspaceMembers")}
        description={t("settings.workspaceMembersDesc")}
      />
      <GlassCard className="p-6 space-y-6">

      {canManage && (
      <form onSubmit={handleAdd} className="bg-well p-4 rounded-xl border border-foreground/[0.05] flex flex-wrap sm:flex-nowrap gap-4 items-end">
        <div className="flex-1 min-w-[200px]">
          <label className="label text-xs">{t("settings.inviteByEmail")}</label>
          <input
            className="input w-full text-sm mt-1"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="colleague@example.com"
            required
          />
        </div>
        <div className="w-32">
          <label className="label text-xs">{t("settings.accessRole")}</label>
          <Select
            className="mt-1 text-xs"
            value={role}
            onValueChange={setRole}
            options={[
              { value: "member", label: t("settings.roleMember") },
              { value: "admin", label: t("settings.roleAdmin") },
              { value: "owner", label: t("settings.roleOwner") },
            ]}
          />
        </div>
        <Button variant="primary" className="py-2 text-xs shrink-0" disabled={busy || !email}>
          {busy ? t("settings.inviting") : t("settings.inviteMember")}
        </Button>
      </form>
      )}
      {err && <p className="text-sm text-danger-fg font-medium">{err}</p>}

      {loading ? (
        <div className="text-foreground/40 text-sm py-4 text-center">{t("settings.loadingMembers")}</div>
      ) : (
        <div className="divide-y divide-foreground/[0.04] border border-foreground/[0.05] rounded-xl bg-well overflow-hidden">
          {(members || []).map((m) => {
            const myEmail = (me?.email || me?.username || "").toLowerCase();
            const isSelf = myEmail ? m.email.toLowerCase() === myEmail : false;
            // Your own row stays read-only: the one demotion nobody can undo for
            // you is your own. Handing the role over means promoting someone
            // else first, which is a deliberate second step.
            const canEditRole = canManage && !isSelf && (canGrantOwner || m.role !== "owner");
            return (
              <div key={m.userId} className="flex justify-between items-center p-4">
                <div className="flex items-center gap-2.5 flex-wrap">
                  <span className="font-semibold text-sm text-foreground">{m.email}</span>
                  {canEditRole ? (
                    <Select
                      className="text-xs py-1"
                      value={m.role}
                      onValueChange={(role) => handleRoleChange(m, role)}
                      options={[
                        { value: "member", label: t("settings.roleMember") },
                        { value: "admin", label: t("settings.roleAdmin") },
                        ...(canGrantOwner ? [{ value: "owner", label: t("settings.roleOwner") }] : []),
                      ]}
                    />
                  ) : (
                    <Badge tone={getRoleTone(m.role)} className="capitalize text-[10px] tracking-wide font-semibold px-2">
                      {m.role === "owner" ? t("settings.roleOwner") : m.role === "admin" ? t("settings.roleAdmin") : t("settings.roleMember")}
                    </Badge>
                  )}
                  {m.pending ? (
                    <Badge tone="amber" className="text-[10px] px-2">{t("settings.statusPending")}</Badge>
                  ) : (
                    <span className="text-xs text-foreground/40">{t("settings.statusJoined", { time: m.joinedAt ? timeAgo(m.joinedAt) : "" })}</span>
                  )}
                </div>
                {canManage && !isSelf && (
                  <Button
                    variant="danger"
                    onClick={() => handleRemove(m.userId)}
                    className="text-xs py-1 px-2.5 border-0"
                  >
                    {t("settings.remove")}
                  </Button>
                )}
              </div>
            );
          })}
        </div>
      )}
    </GlassCard>

    {inviteLink && (
      <Modal title={t("settings.inviteCreatedTitle")} onClose={() => setInviteLink("")}>
        <div className="space-y-4">
          <p className="text-xs text-foreground/60">{t("settings.inviteCreatedDesc")}</p>
          <div className="flex items-center gap-2">
            <input
              readOnly
              className="input w-full cursor-text bg-well font-mono text-xs text-foreground/80"
              value={inviteLink}
              onFocus={(e) => e.currentTarget.select()}
            />
            <Button
              variant="primary"
              className="shrink-0 text-xs"
              onClick={() => {
                navigator.clipboard.writeText(inviteLink);
                toast.success(t("settings.inviteCopied"));
              }}
            >
              {t("settings.copy")}
            </Button>
          </div>
          <p className="text-xs text-foreground/50">{t("settings.inviteExpiry")}</p>
        </div>
      </Modal>
    )}
    </div>
  );
}

