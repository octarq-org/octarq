import { GlassCard, Skeleton } from "../ui";

// Shared loading skeleton for list pages. Replaces the single "loading…" text
// line the list pages used to show — links/mail/dns, Abuse and Audit all load
// into the same row-list shape, so one component covers all five. The skeleton
// mirrors the real list (card frame + rows with title/line placeholders) so
// the first paint doesn't jump and never reads as an empty box.
export function ListSkeleton({ rows = 6, ariaLabel }: { rows?: number; ariaLabel?: string }) {
  return (
    <div aria-busy="true" role="status" aria-label={ariaLabel} className="w-full">
      <GlassCard className="overflow-hidden">
        <div className="divide-y divide-foreground/[0.04]">
          {Array.from({ length: rows }).map((_, i) => (
            <div key={i} className="p-4">
              <Skeleton className="mb-2 h-3.5 w-2/3" />
              <Skeleton className="h-3 w-1/3" />
            </div>
          ))}
        </div>
      </GlassCard>
    </div>
  );
}
