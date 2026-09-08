import React from "react";
import {
  Bell,
  Clock,
  Database,
  HardDrive,
  Info,
  Mail,
  Server,
  Shield,
  ShieldAlert,
  User,
} from "lucide-react";

export function getNotificationIcon(eventType: string, className = "h-4 w-4") {
  const type = (eventType || "").toLowerCase();

  if (type.startsWith("security.")) {
    return <ShieldAlert className={`${className} text-danger-fg`} />;
  }
  if (type.startsWith("system.")) {
    return <Server className={`${className} text-primary`} />;
  }
  if (type.startsWith("cron.") || type.startsWith("task.")) {
    return <Clock className={`${className} text-warning-fg`} />;
  }
  if (type.startsWith("storage.") || type.startsWith("backup.")) {
    return <HardDrive className={`${className} text-accent-fg`} />;
  }
  if (type.startsWith("database.") || type.startsWith("db.")) {
    return <Database className={`${className} text-accent-fg`} />;
  }
  if (type.startsWith("mail.") || type.startsWith("email.")) {
    return <Mail className={`${className} text-muted-foreground`} />;
  }
  if (type.startsWith("auth.") || type.startsWith("user.")) {
    return <User className={`${className} text-muted-foreground`} />;
  }

  return <Bell className={`${className} text-muted-foreground`} />;
}
