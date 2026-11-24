import type { ComponentType, LazyExoticComponent } from "react";

export type LazyPage = LazyExoticComponent<ComponentType<Record<string, never>>>;

export interface UIRoute {
  path: string;
  Component: LazyPage;
  requiredTier?: string;
  requiredRole?: string;
  layout?: "default" | "full";
}

export interface UIWidget {
  slot: string;
  Component: LazyPage;
  order?: number;
  pluginName?: string;
  requires?: string[];
}

export interface ToolCardAction {
  type: "approve" | "reject" | "custom" | "open_canvas" | "edit_artifact" | "refresh";
  approvalId?: string;
  approvalToken?: string;
  toolName?: string;
  artifactId?: string;
  payload?: unknown;
}

export type ToolCardActionHandler = (
  action: ToolCardAction,
) => void | Promise<{ status: number }>;

export interface ToolCardProps {
  args: unknown;
  callId?: string;
  toolName: string;
  onAction: ToolCardActionHandler;
}

export type ToolCardComponent = LazyExoticComponent<ComponentType<ToolCardProps>>;

export interface ToolCardRegistration {
  match: string;
  Component: ToolCardComponent;
  schema?: unknown;
}

export interface ArtifactEditorProps {
  artifactId: string;
  content: unknown;
  onAction: ToolCardActionHandler;
}

export type ArtifactEditorComponent = LazyExoticComponent<ComponentType<ArtifactEditorProps>>;

export interface ArtifactEditorRegistration {
  type: string;
  Component: ArtifactEditorComponent;
}

export interface UIArea {
  id: string;
  title: string;
  subtitle?: string;
  icon?: string;
  groups?: string[];
}

export interface PluginMenuItem {
  id: string;
  label: string;
  path: string;
  icon: string;
  category: string;
  area?: string;
  order?: number;
  requiredRole?: string;
}

import type { Lang } from "../i18n";

export type PluginI18n = Partial<Record<Lang, Record<string, unknown>>>;

export type LockedFallback = ComponentType<{ status: number }>;

export interface UIPlugin {
  name: string;
  routes: UIRoute[];
  instanceRoutes?: UIRoute[];
  widgets?: UIWidget[];
  areas?: UIArea[];
  i18n?: PluginI18n;
  lockedFallback?: LockedFallback;
  onboarding?: { Component: LazyExoticComponent<ComponentType<{ onComplete?: () => void }>> };
  cards?: ToolCardRegistration[];
  artifacts?: ArtifactEditorRegistration[];
  replaces?: string[];
}
