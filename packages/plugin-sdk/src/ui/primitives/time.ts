// Relative age for list rows and widgets.
//
// The strings are English-only today: this is a pure function with no i18n
// context to read (the SDK's useTranslation is a hook). Localizing it means a
// useTimeAgo() hook reading uiCommon.* — a real change to every call site, not
// a move. Kept behaviour-identical on promotion so this stays a relocation.
export function timeAgo(iso: string): string {
  const d = new Date(iso).getTime();
  const s = Math.floor((Date.now() - d) / 1000);
  if (s < 60) return `${s}s ago`;
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  return `${Math.floor(s / 86400)}d ago`;
}
