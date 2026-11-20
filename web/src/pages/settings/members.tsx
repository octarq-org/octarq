import {
  Badge,
  Button,
  Field,
  Input,
  PageCard,
  PageRow,
  PageRows,
  PageSection,
  PageShell,
  Select,
  Separator,
  confirmDialog,
  timeAgo,
  toast,
} from "@octarq/plugin-sdk";
import { Fragment, useEffect, useState } from "react";
import { api, OrgMember } from "../../api";
import { Users } from "lucide-react";
import { useTranslation } from "../../i18n";
import { roleSatisfies, useCurrentRole } from "../../shell/role";

export function OrgMembersManager() {
  const { t } = useTranslation();
  const { role: myRole, isInstanceAdmin } = useCurrentRole();
  const canManage = roleSatisfies("admin", myRole, isInstanceAdmin);
  const [members, setMembers] = useState<OrgMember[]>([]);
  const [me, setMe] = useState<{ email?: string; username?: string; orgId?: number } | null>(null);
  const [loading, setLoading] = useState(true);
  const [email, setEmail] = useState("");
  const [role, setRole] = useState("member");
  const [busy, setBusy] = useState(false);
  const [resendingId, setResendingId] = useState<number | null>(null);

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

  const canGrantOwner = roleSatisfies("owner", myRole, isInstanceAdmin);

  async function handleRoleChange(m: OrgMember, role: string) {
    if (role === m.role) return;
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

  async function handleResend(userId: number) {
    setResendingId(userId);
    try {
      const res = await api.resendOrgMemberInvite(userId);
      if (res?.emailSent === false) {
        toast.warning(t("settings.inviteNoMail"));
      } else {
        toast.success(t("settings.inviteResent"));
      }
    } catch (e: any) {
      toast.error(e.message || t("settings.failedResendInvite"));
    } finally {
      setResendingId(null);
    }
  }

  const getRoleTone = (r: string) => {
    if (r === "owner") return "green";
    if (r === "admin") return "indigo";
    return "neutral";
  };

  return (
    <PageShell
      title={t("settings.workspaceMembers")}
      description={t("settings.workspaceMembersDesc")}
    >
      {canManage && (
        <PageSection>
          <PageCard>
            <form onSubmit={handleAdd} className="flex flex-col sm:flex-row gap-3 items-stretch sm:items-end">
              <div className="flex-1">
                <Field label={t("settings.inviteByEmail")}>
                  <Input
                    type="email"
                    className="text-xs"
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    placeholder="colleague@example.com"
                    required
                  />
                </Field>
              </div>
              <div className="sm:w-32">
                <span className="label text-xs">{t("settings.accessRole")}</span>
                <Select
                  className="mt-1 text-xs"
                  aria-label={t("settings.accessRole")}
                  value={role}
                  onValueChange={setRole}
                  options={[
                    { value: "member", label: t("settings.roleMember") },
                    { value: "admin", label: t("settings.roleAdmin") },
                    { value: "owner", label: t("settings.roleOwner") },
                  ]}
                />
              </div>
              <Button variant="primary" className="sm:w-auto py-2 text-xs shrink-0" disabled={busy || !email}>
                {busy ? t("settings.inviting") : t("settings.inviteMember")}
              </Button>
            </form>
          </PageCard>
        </PageSection>
      )}

      <PageSection>
        {loading ? (
          <PageCard>
            <div className="text-muted-foreground text-sm py-4 text-center">{t("settings.loadingMembers")}</div>
          </PageCard>
        ) : (
          <PageRows>
            {(members || []).map((m, idx) => {
              const myEmail = (me?.email || me?.username || "").toLowerCase();
              const isSelf = myEmail ? m.email.toLowerCase() === myEmail : false;
              const canEditRole = canManage && !isSelf && (canGrantOwner || m.role !== "owner");
              return (
                <Fragment key={m.userId}>
                  {idx > 0 && <Separator />}
                  <PageRow
                    title={<span className="font-mono">{m.email}</span>}
                    description={
                      <span className="flex flex-wrap items-center gap-2 mt-0.5">
                        {m.pending ? (
                          <Badge tone="amber" className="text-[10px] px-2">{t("settings.statusPending")}</Badge>
                        ) : (
                          <span>{t("settings.statusJoined", { time: m.joinedAt ? timeAgo(m.joinedAt) : "" })}</span>
                        )}
                      </span>
                    }
                    control={
                      <div className="flex flex-wrap items-center gap-2">
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
                        {canManage && !isSelf && (
                          <>
                            {m.pending && (
                              <Button
                                variant="secondary"
                                size="sm"
                                onClick={() => handleResend(m.userId)}
                                disabled={resendingId === m.userId}
                              >
                                {resendingId === m.userId ? t("settings.resending") : t("settings.resendInvite")}
                              </Button>
                            )}
                            <Button
                              variant="danger"
                              size="sm"
                              onClick={() => handleRemove(m.userId)}
                            >
                              {t("settings.remove")}
                            </Button>
                          </>
                        )}
                      </div>
                    }
                  />
                </Fragment>
              );
            })}
          </PageRows>
        )}
      </PageSection>
    </PageShell>
  );
}

