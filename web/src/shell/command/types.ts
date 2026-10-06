import type { A2UIWidget } from "../../components/a2ui";

export type ToolPayload = A2UIWidget | { a2ui?: A2UIWidget; [key: string]: unknown };

export interface ChatMessage {
  id: string;
  role: "user" | "assistant" | "tool";
  content: string;
  thinking?: string;
  tokens?: number;
  toolCalls?: Array<ToolPayload>;
}
