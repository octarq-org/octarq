import { GlassCard } from "./glass-card";
import { Skeleton } from "./skeleton";

export interface ListSkeletonProps {
  rows?: number;
  ariaLabel?: string;
}

export function ListSkeleton({ rows = 6, ariaLabel }: ListSkeletonProps) {
  return (
    <div aria-busy="true" role="status" aria-label={ariaLabel} className="w-full">
      <GlassCard className="overflow-hidden">
        <div className="divide-y divide-border">
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
