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
