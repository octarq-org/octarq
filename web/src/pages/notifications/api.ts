import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api";
import { NotificationPreferenceItem } from "./schemas";

export const notificationKeys = {
  all: ["notifications"] as const,
  lists: () => [...notificationKeys.all, "list"] as const,
  list: (params?: { unreadOnly?: boolean; limit?: number; offset?: number }) =>
    [...notificationKeys.lists(), params ?? {}] as const,
  unreadCount: () => [...notificationKeys.all, "unread-count"] as const,
  preferences: ["notification-preferences"] as const,
  channels: ["notification-channels-registered"] as const,
};

export function useNotificationsQuery(params?: { unreadOnly?: boolean; limit?: number; offset?: number }) {
  return useQuery({
    queryKey: notificationKeys.list(params),
    queryFn: () => api.notifications(params),
  });
}

export function useUnreadNotificationsCountQuery(refetchInterval: number | false = false) {
  return useQuery({
    queryKey: notificationKeys.unreadCount(),
    queryFn: async () => {
      const unreadList = await api.notifications({ unreadOnly: true, limit: 100 });
      return unreadList.length;
    },
    refetchInterval,
  });
}

export function useMarkReadMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api.markNotificationRead(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: notificationKeys.all });
    },
  });
}

export function useMarkAllReadMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => api.markAllNotificationsRead(),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: notificationKeys.all });
    },
  });
}

export function useDeleteNotificationMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api.deleteNotification(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: notificationKeys.all });
    },
  });
}

export function useNotificationPreferencesQuery() {
  return useQuery({
    queryKey: notificationKeys.preferences,
    queryFn: () => api.notificationPreferences(),
  });
}

export function useRegisteredChannelsQuery() {
  return useQuery({
    queryKey: notificationKeys.channels,
    queryFn: () => api.registeredNotificationChannels(),
  });
}

export function useUpdatePreferencesMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (preferences: { eventPattern: string; channels: string[] }[]) =>
      api.updateNotificationPreferences(preferences),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: notificationKeys.preferences });
    },
  });
}

export function useResetPreferencesMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => api.resetNotificationPreferences(),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: notificationKeys.preferences });
    },
  });
}
