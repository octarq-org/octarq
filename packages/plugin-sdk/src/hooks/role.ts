import { createContext, useContext } from "react";

const ROLE_RANK: Record<string, number> = { member: 1, admin: 2, owner: 3 };

export function roleSatisfies(
  required: string | undefined,
  role: string | undefined,
  isInstanceAdmin: boolean,
): boolean {
  if (!required) return true;
  if (isInstanceAdmin) return true;
  const need = ROLE_RANK[required];
  if (need === undefined) return true;
  return (ROLE_RANK[role ?? ""] ?? 0) >= need;
}

export interface CurrentRole {
  role?: string;
  isInstanceAdmin: boolean;
}

export const RoleContext = createContext<CurrentRole>({ isInstanceAdmin: false });

export const RoleProvider = RoleContext.Provider;

export function useCurrentRole(): CurrentRole {
  return useContext(RoleContext);
}
