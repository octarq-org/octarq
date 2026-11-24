import React, { type ReactNode } from "react";
import {
  resolveToolCard,
  type ToolCardAction,
  type ToolCardActionHandler,
  type ToolCardProps,
} from "@octarq/plugin-sdk";
import { FallbackCard } from "./FallbackCard";
import { ApprovalCard } from "./ApprovalCard";

export type CardAction = ToolCardAction;
export type CardOutcome = { status: number };
export type CardActionHandler = ToolCardActionHandler;
export type { ToolCardProps };

export function carriesApprovalID(args: unknown): boolean {
  if (typeof args !== "object" || args === null || Array.isArray(args)) {
    return false;
  }
  const record = args as Record<string, unknown>;
  for (const key of ["approval_id", "approvalId", "_approval_id"]) {
    const v = record[key];
    if (typeof v === "string" && v.trim() !== "") return true;
  }
  return false;
}

export function renderToolCall(
  toolName: string,
  callId: string,
  args: unknown,
  onAction: CardActionHandler,
): ReactNode {
  let parsedArgs: unknown = args;

  if (typeof args === "string") {
    try {
      parsedArgs = JSON.parse(args);
    } catch {
      return (
        <FallbackCard
          toolName={toolName}
          callId={callId}
          reason="malformed-args"
          onAction={onAction}
        />
      );
    }
  }

  const Component = resolveToolCard(toolName);
  if (Component) {
    return (
      <Component
        args={parsedArgs}
        callId={callId}
        toolName={toolName}
        onAction={onAction}
      />
    );
  }

  if (carriesApprovalID(parsedArgs)) {
    return (
      <ApprovalCard
        args={parsedArgs}
        callId={callId}
        toolName={toolName}
        onAction={onAction}
      />
    );
  }

  return (
    <FallbackCard
      toolName={toolName}
      callId={callId}
      reason="unregistered"
      onAction={onAction}
    />
  );
}
