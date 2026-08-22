import { useEffect, useState } from "react";
import { api, OrgMember } from "../../api";
import { Field, timeAgo, PageHeader, GlassCard, Badge, Button, Select, toast, confirmDialog } from "../../ui";
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
    try {
      const res = await api.addOrgMember({ email, role });
      if (res?.emailSent === false) {
        toast.warning(t("settings.inviteNoMail", "Invite created but email could not be sent — configure an SMTP sender first."));
      } else {
        toast.success(t("settings.saved"));
      }
      setEmail("");
      setRole("member");
      load();
    } catch (e: any) {
      toast.error(e.message || t("settings.failedAddMember"));
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
      toast.success(t("settings.saved"));
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
      <form onSubmit={handleAdd} className="flex flex-col sm:flex-row gap-3 items-stretch sm:items-end p-4 rounded-xl border border-foreground/[0.05] bg-well">
        <div className="flex-1">
          <Field label={t("settings.inviteByEmail")}>
            <input
              type="email"
              className="input w-full text-xs"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="colleague@example.com"
              required
            />
          </Field>
        </div>
        <div className="w-full sm:w-32">
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
        <Button variant="primary" className="w-full sm:w-auto py-2 text-xs shrink-0" disabled={busy || !email}>
          {busy ? t("settings.inviting") : t("settings.inviteMember")}
        </Button>
      </form>
      )}

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
              <div key={m.userId} className="flex flex-col sm:flex-row sm:items-center justify-between p-4 gap-3 sm:gap-4">
                <div className="flex items-center gap-2.5 flex-wrap">
                  <span className="font-mono font-semibold text-sm text-foreground truncate">{m.email}</span>
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
                  <div className="flex justify-end w-full sm:w-auto pt-2 sm:pt-0 border-t sm:border-t-0 border-foreground/[0.04]">
                    <Button
                      variant="danger"
                      onClick={() => handleRemove(m.userId)}
                      className="text-xs min-h-[44px] sm:min-h-0 py-2 sm:py-1 px-3 sm:px-2.5 border-0"
                    >
                      {t("settings.remove")}
                    </Button>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
    </GlassCard>
    </div>
  );
}

