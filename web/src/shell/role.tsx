// Current-user role plumbing — the single source of truth for how the shell
// interprets the advisory `requiredRole` metadata on UIRoute / PluginMenuItem.
// The role itself comes from /api/auth/me (api.me().role); the instance-admin
// flag from api.settings(). Both are UX inputs only: the backend keeps
// enforcing via callerOrgRole and answers 403, which ProGate degrades to the
// access-denied state.
import { createContext, useContext } from "react";

// Rank order for the built-in org roles. An unknown/blank requiredRole never
// locks anyone out (fail-open — this is presentation, not security), while an
// unknown/blank CURRENT role satisfies nothing above rank 0.
const ROLE_RANK: Record<string, number> = { member: 1, admin: 2, owner: 3 };

// Whether a user holding `role` (with an optional instance-admin bypass) meets
// an advisory `required` role. Used by both the sidebar merge (App.tsx) and
// the route pre-check (ProGate) so the two can never disagree.
export function roleSatisfies(
  required: string | undefined,
  role: string | undefined,
  isInstanceAdmin: boolean,
): boolean {
  if (!required) return true;
  if (isInstanceAdmin) return true;
  const need = ROLE_RANK[required];
  if (need === undefined) return true; // unrecognized requirement ⇒ don't hide
  return (ROLE_RANK[role ?? ""] ?? 0) >= need;
}

export interface CurrentRole {
  // "owner" | "admin" | "member", or undefined until /api/auth/me answered.
  role?: string;
  // Owner of the bootstrap org — bypasses every requiredRole check.
  isInstanceAdmin: boolean;
}

const RoleContext = createContext<CurrentRole>({ isInstanceAdmin: false });

export const RoleProvider = RoleContext.Provider;

// Safe anywhere: outside the provider (tests, portal pages) it reports an
// anonymous non-admin, so requiredRole-gated UI simply stays hidden.
export function useCurrentRole(): CurrentRole {
  return useContext(RoleContext);
}
