// Fallback spinner while a lazily-loaded route chunk is fetched. Shared by the
// shell routes (App.tsx / Settings.tsx) and the plugin routes (PluginRoutes.tsx)
// so every lazy boundary renders the same loading state — a plugin page must
// not white-screen while its chunk loads just because its Suspense boundary
// is closer than the shell's.
export function RouteFallback() {
  return (
    <div className="grid h-64 place-items-center" role="status" aria-live="polite">
      <div className="h-6 w-6 animate-spin rounded-full border-2 border-foreground/15 border-t-foreground/60" />
    </div>
  );
}
