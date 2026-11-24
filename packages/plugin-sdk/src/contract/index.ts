export type {
  UIPlugin,
  UIRoute,
  UIWidget,
  UIArea,
  PluginMenuItem,
  PluginI18n,
  LazyPage,
  LockedFallback as LockedFallbackType,
  ToolCardAction,
  ToolCardActionHandler,
  ToolCardProps,
  ToolCardComponent,
  ToolCardRegistration,
  ArtifactEditorProps,
  ArtifactEditorComponent,
  ArtifactEditorRegistration,
} from "./types";

export {
  registerUIPlugin,
  uiPlugins,
  uiRoutes,
  uiInstanceRoutes,
  uiWidgets,
  uiAreas,
  uiOnboarding,
  uiToolCards,
  uiArtifactEditors,
  resolveToolCard,
  resolveArtifactEditor,
  uiPluginI18n,
  uiPluginSharedI18n,
  resetRegistry,
} from "./registry";

export { ExtensionSlot, useExtensionCount, useExtensionSlotContext } from "./ExtensionSlot";
export {
  NotificationChannelFormContext,
  useNotificationChannelForm,
  type NotificationChannelFormContextValue,
} from "./NotificationChannelFormContext";
export {
  PluginGateContext,
  usePluginGateContext,
  type PluginGateContextValue,
  PluginRouteGateContext,
  usePluginGate,
  type PluginRouteGateContextValue,
} from "./PluginGateContext";

