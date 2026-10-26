export type {
  NotificationItem,
  NotificationPreferenceItem,
  RegisteredChannel,
} from "./schemas";

export type NotificationFilter = "unread" | "all" | "mentions" | "system";

export interface PreferenceRouteRow {
  key: string;
  label: string;
  description: string;
  pattern: string;
  isCustom?: boolean;
}
