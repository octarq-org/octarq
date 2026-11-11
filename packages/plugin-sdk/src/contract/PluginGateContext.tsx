import { createContext, useContext } from "react";

export interface PluginGateContextValue {
  disabledPlugins: Set<string>;
  disabledPaths: Set<string>;
  loaded: boolean;
}

export const PluginGateContext = createContext<PluginGateContextValue>({
  disabledPlugins: new Set(),
  disabledPaths: new Set(),
  loaded: false,
});

export function usePluginGateContext(): PluginGateContextValue {
  return useContext(PluginGateContext);
}

export interface PluginRouteGateContextValue {
  degrade: (status: number) => void;
  requiredTier?: string;
}

export const PluginRouteGateContext = createContext<PluginRouteGateContextValue | null>(null);

export function usePluginGate(): PluginRouteGateContextValue {
  return useContext(PluginRouteGateContext) ?? { degrade: () => {} };
}
