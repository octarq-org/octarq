import { ReactNode } from "react";

// PageHeader is the standard title/description/action row atop every admin page.
export function PageHeader({
  title,
  description,
  action,
}: {
  title: ReactNode;
  description?: string;
  action?: ReactNode;
}) {
  return (
    <div className="mb-6 flex flex-wrap items-start justify-between gap-4">
      <div>
        <h1 className="font-display text-2xl font-bold tracking-tight text-white">{title}</h1>
        {description && <p className="mt-1 text-sm text-white/55">{description}</p>}
      </div>
      {action}
    </div>
  );
}
