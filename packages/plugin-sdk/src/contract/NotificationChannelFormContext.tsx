import { createContext, useContext } from "react";

export interface NotificationChannelFormContextValue {
  config: Record<string, any>;
  setConfig: (config: Record<string, any> | ((prev: Record<string, any>) => Record<string, any>)) => void;
  updateConfig: (key: string, value: any) => void;
}

const defaultValue: NotificationChannelFormContextValue = {
  config: {},
  setConfig: () => {},
  updateConfig: () => {},
};

export const NotificationChannelFormContext = createContext<NotificationChannelFormContextValue>(defaultValue);

export function useNotificationChannelForm(): NotificationChannelFormContextValue {
  return useContext(NotificationChannelFormContext);
}
