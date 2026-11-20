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
  replaces?: string[];
}
