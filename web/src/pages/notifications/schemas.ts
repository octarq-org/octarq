import { z } from "zod";

export const NotificationSchema = z.object({
  id: z.number(),
  userId: z.string(),
  orgId: z.string().default(""),
  eventType: z.string().default("system.notice"),
  title: z.string().default(""),
  body: z.string().default(""),
  data: z.string().optional().nullable(),
  priority: z.string().default("normal"),
  readAt: z.string().nullable().optional(),
  createdAt: z.string().default(() => new Date().toISOString()),
});

export const NotificationsListSchema = z.array(NotificationSchema);

export const NotificationPreferenceSchema = z.object({
  id: z.number().optional(),
  userId: z.string().optional(),
  eventPattern: z.string(),
  channels: z.array(z.string()).default([]),
  createdAt: z.string().optional(),
  updatedAt: z.string().optional(),
});

export const NotificationPreferencesListSchema = z.array(NotificationPreferenceSchema);

export const RegisteredChannelSchema = z.object({
  name: z.string(),
  displayName: z.string(),
  configSchema: z.any().optional(),
});

export const RegisteredChannelsListSchema = z.array(RegisteredChannelSchema);

export const SimpleOKSchema = z.object({
  ok: z.boolean().default(true),
});

export type NotificationItem = z.infer<typeof NotificationSchema>;
export type NotificationPreferenceItem = z.infer<typeof NotificationPreferenceSchema>;
export type RegisteredChannel = z.infer<typeof RegisteredChannelSchema>;
