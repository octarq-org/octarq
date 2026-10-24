/**
 * Card registry: tool name to component mapping.
 *
 * - registerCard throws on duplicates (Fail-Closed, preventing duplicate ambiguity).
 * - resolveCard returns null for unregistered tools.
 * - renderToolCall validates args against schema before component sees them.
 * - Components receive valid args or FallbackCard renders with reason.
 */
import React, { type ComponentType, type ReactNode } from "react";
import { validateArgs, type JsonSchema } from "./validate";
import { FallbackCard } from "./FallbackCard";
import { ApprovalCard } from "./ApprovalCard";
import { LinkCard } from "./LinkCard";
import { EmployeeCard } from "./EmployeeCard";
import { OrderCard } from "./OrderCard";
import { FinanceCard } from "./FinanceCard";
import { StatusCard } from "./StatusCard";
import { PortalCard } from "./PortalCard";

export interface CardAction {
  /** approve/reject feed approval endpoints; custom is domain-specific. */
  type: "approve" | "reject" | "custom";
  approvalId?: string;
  payload?: unknown;
}

export interface CardOutcome {
  status: number;
}

export type CardActionHandler = (action: CardAction) => void | Promise<CardOutcome>;

export interface ToolCardProps {
  args: unknown;
  callId?: string;
  toolName: string;
  onAction: CardActionHandler;
}

export type ToolCardComponent = ComponentType<ToolCardProps>;

const cardRegistry = new Map<string, ToolCardComponent>();
const schemaRegistry = new Map<string, JsonSchema>();

export function registerCard(
  toolName: string,
  component: ToolCardComponent,
): void {
  if (cardRegistry.has(toolName)) {
    throw new Error(`card already registered: ${toolName}`);
  }
  cardRegistry.set(toolName, component);
}

export function resolveCard(toolName: string): ToolCardComponent | null {
  return cardRegistry.get(toolName) ?? null;
}

export function registerSchema(toolName: string, schema: JsonSchema): void {
  schemaRegistry.set(toolName, schema);
}

export function schemasFor(toolName: string): JsonSchema | null {
  return schemaRegistry.get(toolName) ?? null;
}

export function clearCards(): void {
  cardRegistry.clear();
  schemaRegistry.clear();
}

/** carriesApprovalID reports whether parsed args name an approval. */
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

/**
 * Render one tool call: resolve -> parse -> schema-check -> component.
 * Every failure branch renders FallbackCard with a machine-readable reason.
 * Never throws and never returns blank.
 */
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

  // Any call carrying an approval ID renders ApprovalCard
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

  const Component = resolveCard(toolName);
  if (!Component) {
    return (
      <FallbackCard
        toolName={toolName}
        callId={callId}
        reason="unregistered"
        onAction={onAction}
      />
    );
  }

  const schema = schemasFor(toolName);
  if (!schema) {
    return (
      <FallbackCard
        toolName={toolName}
        callId={callId}
        reason="no-schema"
        onAction={onAction}
      />
    );
  }

  const errors = validateArgs(schema, parsedArgs);
  if (errors.length > 0) {
    return (
      <FallbackCard
        toolName={toolName}
        callId={callId}
        reason="invalid-args"
        detail={errors[0]}
        onAction={onAction}
      />
    );
  }

  return (
    <Component
      args={parsedArgs}
      callId={callId}
      toolName={toolName}
      onAction={onAction}
    />
  );
}

const DEFAULT_SCHEMAS: Record<string, JsonSchema> = {
  links_create: {
    type: "object",
    required: ["slug", "target"],
    properties: {
      host: { type: "string" },
      slug: { type: "string" },
      target: { type: "string" },
      enabled: { type: "boolean" },
    },
  },
  links_update: {
    type: "object",
    properties: {
      host: { type: "string" },
      slug: { type: "string" },
      target: { type: "string" },
      enabled: { type: "boolean" },
    },
  },
  links_get: {
    type: "object",
    properties: {
      slug: { type: "string" },
    },
  },
  links_list: {
    type: "object",
    properties: {
      limit: { type: "integer" },
    },
  },
  links_disable: {
    type: "object",
    properties: {
      slug: { type: "string" },
    },
  },
  links_batch_disable: {
    type: "object",
    properties: {},
  },
  crew_create: {
    type: "object",
    properties: {
      name: { type: "string" },
      soul: { type: "string" },
    },
  },
  crew_get: {
    type: "object",
    properties: {
      id: { type: "number" },
    },
  },
  crew_list: {
    type: "object",
    properties: {},
  },
  crew_update_soul: {
    type: "object",
    properties: {
      id: { type: "number" },
      soul: { type: "string" },
    },
  },
  orders_get: {
    type: "object",
    properties: {
      orderNo: { type: "string" },
    },
  },
  orders_list: {
    type: "object",
    properties: {
      limit: { type: "integer" },
    },
  },
  finance_balance: {
    type: "object",
    properties: {
      currency: { type: "string" },
    },
  },
  finance_list_entries: {
    type: "object",
    properties: {
      currency: { type: "string" },
      category: { type: "string" },
      kind: { type: "string" },
      limit: { type: "integer" },
    },
  },
  finance_list_subscriptions: {
    type: "object",
    properties: {},
  },
  statuspage_public: {
    type: "object",
    properties: {
      slug: { type: "string" },
    },
  },
  statuspage_probe: {
    type: "object",
    properties: {
      target: { type: "string" },
    },
  },
  portal_licenses: {
    type: "object",
    properties: {
      email: { type: "string" },
    },
  },
  portal_devices: {
    type: "object",
    properties: {
      email: { type: "string" },
      licenseId: { type: "number" },
    },
  },
};

/** Register the domain cards and standard schemas. Idempotent. */
export function registerDomainCards(): void {
  const registrations: Array<[string, ToolCardComponent]> = [
    ["links_create", LinkCard],
    ["links_update", LinkCard],
    ["links_get", LinkCard],
    ["links_list", LinkCard],
    ["links_disable", LinkCard],
    ["links_batch_disable", LinkCard],
    ["crew_create", EmployeeCard],
    ["crew_get", EmployeeCard],
    ["crew_list", EmployeeCard],
    ["crew_update_soul", EmployeeCard],
    ["orders_get", OrderCard],
    ["orders_list", OrderCard],
    ["finance_balance", FinanceCard],
    ["finance_list_entries", FinanceCard],
    ["finance_list_subscriptions", FinanceCard],
    ["statuspage_public", StatusCard],
    ["statuspage_probe", StatusCard],
    ["portal_licenses", PortalCard],
    ["portal_devices", PortalCard],
  ];

  for (const [name, component] of registrations) {
    if (!resolveCard(name)) {
      registerCard(name, component);
    }
  }

  for (const [name, schema] of Object.entries(DEFAULT_SCHEMAS)) {
    if (!schemasFor(name)) {
      registerSchema(name, schema);
    }
  }
}

// Initial bootstrap
registerDomainCards();
