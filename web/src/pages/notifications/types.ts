export type {
  NotificationItem,
  NotificationPreferenceItem,
  RegisteredChannel,
} from "./schemas";

export type NotificationFilter = "all" | "unread";

export interface PreferenceRouteRow {
  key: string;
  label: string;
  description: string;
  pattern: string;
  isCustom?: boolean;
}
