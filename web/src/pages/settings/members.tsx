import { useEffect, useState } from "react";
import { NavLink, Navigate, Route, Routes } from "react-router-dom";
import { api, ApiError, Settings as SettingsData, OrgMember, LicenseStatus, Overview, PluginInfo } from "../../api";
import { Empty, Field, Modal, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button } from "../../ui";
import { Settings as SettingsIcon, Cloud, Mail, Bell, Users, Trash2, Pencil, ShieldAlert, KeyRound, BellRing, Webhook, Plus, Send, AlertTriangle, CreditCard, Sparkles, Shield, DollarSign, Puzzle } from "lucide-react";
import { useTranslation } from "../../i18n";
import LLMProvidersSettings from "../LLMProviders";
import { useSettingsData, SavedBadge } from "./shared";

export function OrgMembersManager() {
  const [members, setMembers] = useState<OrgMember[]>([]);
  const [loading, setLoading] = useState(true);
  const [email, setEmail] = useState("");
  const [role, setRole] = useState("member");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  async function load() {
    setLoading(true);
    try {
      setMembers(await api.orgMembers());
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
      await api.addOrgMember({ email, role });
      setEmail("");
      setRole("member");
      load();
    } catch (e: any) {
      setErr(e.message || "Failed to add member");
    } finally {
      setBusy(false);
    }
  }

  async function handleRemove(userId: number) {
    if (!confirm("Remove this member from the workspace? They will lose access instantly.")) return;
    try {
      await api.deleteOrgMember(userId);
      load();
    } catch (e: any) {
      alert(e.message || "Failed to remove member");
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
        title="Workspace Members"
        description="Invite teammates and manage their roles in this workspace."
      />
      <GlassCard className="p-6 space-y-6">

      <form onSubmit={handleAdd} className="bg-black/25 p-4 rounded-xl border border-white/[0.05] flex flex-wrap sm:flex-nowrap gap-4 items-end">
        <div className="flex-1 min-w-[200px]">
          <label className="label text-xs">Invite Colleague by Email</label>
          <input
            className="input w-full text-sm mt-1"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="colleague@example.com"
            required
          />
        </div>
        <div className="w-32">
          <label className="label text-xs">Access Role</label>
          <select className="input w-full text-xs mt-1" value={role} onChange={(e) => setRole(e.target.value)}>
            <option value="member">Member</option>
            <option value="admin">Admin</option>
            <option value="owner">Owner</option>
          </select>
        </div>
        <Button variant="primary" className="py-2 text-xs shrink-0" disabled={busy || !email}>
          {busy ? "Inviting..." : "Invite Member"}
        </Button>
      </form>
      {err && <p className="text-sm text-rose-400 font-medium">{err}</p>}

      {loading ? (
        <div className="text-white/40 text-sm py-4 text-center">loading members list…</div>
      ) : (
        <div className="divide-y divide-white/[0.04] border border-white/[0.05] rounded-xl bg-black/25 overflow-hidden">
          {(members || []).map((m) => (
            <div key={m.userId} className="flex justify-between items-center p-4">
              <div className="flex items-center gap-2.5">
                <span className="font-semibold text-sm text-white">{m.email}</span>
                <Badge tone={getRoleTone(m.role)} className="capitalize text-[10px] tracking-wide font-semibold px-2">
                  {m.role}
                </Badge>
              </div>
              <Button
                variant="danger"
                onClick={() => handleRemove(m.userId)}
                className="text-xs py-1 px-2.5 bg-rose-500/0 hover:bg-rose-500/10 border-0"
              >
                Remove
              </Button>
            </div>
          ))}
        </div>
      )}
    </GlassCard>
    </div>
  );
}

