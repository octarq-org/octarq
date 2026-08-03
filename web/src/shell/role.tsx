// Shell-side role evaluation for advisory requiredRole metadata on UIRoute/PluginMenuItem (UX input only; backend enforces via callerOrgRole).
import { createContext, useContext } from "react";

// Rank order for built-in org roles (fail-open for unknown requiredRole).
const ROLE_RANK: Record<string, number> = { member: 1, admin: 2, owner: 3 };

// Checks if user role or instance-admin bypass meets required role.
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

// Returns current role context; defaults to non-admin outside provider.
export function useCurrentRole(): CurrentRole {
  return useContext(RoleContext);
}
