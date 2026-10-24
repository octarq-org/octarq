/**
 * PortalCard: Customer portal and license lookup.
 * Metadata only — tokens are shown once at issuance, never here.
 *
 * All user copy strictly conforms to ASD-STE100 (<= 20 words/sentence, active voice).
 */
import React from "react";
import { Key } from "lucide-react";
import type { ToolCardProps } from "./registry";
import { CardShell, type CardState } from "./CardShell";

export interface PortalArgs {
  email?: string;
  licenseId?: number | string;
}

export function PortalCard({
  args,
  toolName,
  state,
}: ToolCardProps & { state?: CardState }) {
  const a = (args ?? {}) as PortalArgs;

  return (
    <CardShell title={`Licenses — ${(a.email ?? "").trim() || "Buyer"}`} state={state}>
      <div className="flex items-start gap-2">
        <Key className="h-4 w-4 text-primary shrink-0 mt-0.5" />
        <div className="flex-1 space-y-1 font-mono text-xs">
          {a.email && (
            <div data-testid="portal-email">
              <span className="text-muted-foreground">Buyer: </span>
              <span className="font-semibold text-foreground">{a.email}</span>
            </div>
          )}
          {a.licenseId !== undefined && (
            <div data-testid="portal-license">
              <span className="text-muted-foreground">License ID: </span>
              <span className="font-semibold text-foreground">#{a.licenseId}</span>
            </div>
          )}
          {toolName === "portal_licenses" && (
            <div className="text-[11px] text-muted-foreground">
              Metadata display only. Secret license keys never appear in chat history.
            </div>
          )}
        </div>
      </div>
    </CardShell>
  );
}
