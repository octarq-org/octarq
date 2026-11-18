import type * as React from "react";
import { cn } from "../cn";
import { Button } from "./button";
import { Empty } from "./empty";
import { PageHeader } from "./page-header";
import { ScreenWrap } from "./screen-wrap";

function BackChevron() {
  return (
    <svg
      className="h-4 w-4 shrink-0"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.75}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M15 18l-6-6 6-6" />
    </svg>
  );
}

export function PageShell({
  title,
  description,
  eyebrow,
  action,
  backButton,
  className,
  children,
}: {
  title: React.ReactNode;
  description?: React.ReactNode;
  eyebrow?: string;
  action?: React.ReactNode;
  backButton?: {
    label: string;
    onClick: () => void;
    disabled?: boolean;
  };
  className?: string;
  children: React.ReactNode;
}) {
  const backBar = backButton ? (
    <div className="mb-3">
      <Button
        type="button"
        variant="ghost"
        size="sm"
        disabled={backButton.disabled}
        onClick={backButton.onClick}
        className="-ml-2.5 text-muted-foreground"
      >
        <BackChevron />
        {backButton.label}
      </Button>
    </div>
  ) : null;

  const header = (
    <PageHeader
      title={title}
      description={description}
      eyebrow={eyebrow}
      action={action}
    />
  );

  return (
    <ScreenWrap className={cn("flex min-h-0 min-w-0 flex-1 flex-col pb-12", className)}>
      <div data-slot="page-shell" data-width="full" className="flex min-h-0 min-w-0 flex-1 flex-col">
        {backBar}
        {header}
        <div className="mt-6 flex min-h-0 flex-1 flex-col [&>*:first-child]:mt-0 [&>section:first-child]:mt-2 [&>nav:first-child+*]:mt-6">
          {children}
        </div>
      </div>
    </ScreenWrap>
  );
}

export function PageSection({
  action,
  children,
  className,
  description,
  id,
  title,
}: {
  action?: React.ReactNode;
  children?: React.ReactNode;
  className?: string;
  description?: React.ReactNode;
  id?: string;
  title?: React.ReactNode;
}) {
  return (
    <section className={cn("mt-10", className)} id={id}>
      {(title || action) && (
        <div className="flex min-h-8 flex-row items-center justify-between gap-4">
          {title ? (
            <h2 className="text-base font-semibold tracking-tight text-foreground">
              {title}
            </h2>
          ) : (
            <span />
          )}
          {action && <div className="flex shrink-0 items-center gap-2">{action}</div>}
        </div>
      )}
      {description ? (
        <p className="mt-1 text-pretty text-sm leading-relaxed text-muted-foreground">
          {description}
        </p>
      ) : null}
      {children}
    </section>
  );
}

export function PageRows({
  children,
  className,
}: {
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <div
      data-slot="page-rows"
      className={cn(
        "mt-3 overflow-hidden rounded-lg border border-border bg-card",
        "[&_[data-slot=page-row]]:min-h-[var(--row-height,3.5rem)]",
        "[&_[data-slot=separator][data-orientation=horizontal]]:mx-4 [&_[data-slot=separator][data-orientation=horizontal]]:w-auto",
        className,
      )}
    >
      {children}
    </div>
  );
}

export function PageRow({
  title,
  description,
  icon,
  control,
  children,
  onClick,
  className,
  danger,
}: {
  title: React.ReactNode;
  description?: React.ReactNode;
  icon?: React.ReactNode;
  control?: React.ReactNode;
  children?: React.ReactNode;
  onClick?: () => void;
  className?: string;
  danger?: boolean;
}) {
  const right = control ?? children;
  const body = (
    <>
      <div className="flex min-w-0 flex-1 items-center gap-3">
        {icon && (
          <span className="flex size-8 shrink-0 items-center justify-center rounded-md border border-border bg-well text-muted-foreground [&_svg]:size-4">
            {icon}
          </span>
        )}
        <div className="min-w-0 flex-1">
          <div
            className={cn(
              "text-sm font-medium",
              danger ? "text-danger-fg" : "text-foreground",
            )}
          >
            {title}
          </div>
          {description && (
            <p className="mt-0.5 text-pretty text-xs leading-relaxed text-muted-foreground">
              {description}
            </p>
          )}
        </div>
      </div>
      {right ? <div className="flex shrink-0 items-center gap-2">{right}</div> : null}
    </>
  );

  const baseClass = cn(
    "flex min-h-[var(--row-height,3.5rem)] w-full flex-wrap items-center justify-between gap-4 px-4 py-3 text-left",
    className,
  );

  if (onClick) {
    return (
      <button
        type="button"
        data-slot="page-row"
        onClick={onClick}
        className={cn(
          baseClass,
          "cursor-pointer transition-colors duration-100 ease-out hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring",
        )}
      >
        {body}
      </button>
    );
  }

  return (
    <div data-slot="page-row" className={baseClass}>
      {body}
    </div>
  );
}

export function PageCard({
  children,
  className,
  footer,
}: {
  children: React.ReactNode;
  className?: string;
  footer?: React.ReactNode;
}) {
  return (
    <div
      data-slot="page-card"
      className={cn(
        "mt-3 rounded-lg border border-border bg-card p-4 sm:p-5",
        className,
      )}
    >
      {children}
      {footer ? (
        <div className="mt-5 flex justify-end gap-2 border-t border-border pt-4">
          {footer}
        </div>
      ) : null}
    </div>
  );
}

export function PageEmpty({
  action,
  detail,
  icon,
  title,
  className,
}: {
  action?: React.ReactNode;
  detail?: React.ReactNode;
  icon?: React.ReactNode;
  title?: React.ReactNode;
  className?: string;
}) {
  return (
    <Empty
      reason={title}
      detail={detail}
      action={action}
      className={cn("mt-3 border-dashed", className)}
    >
      {icon}
    </Empty>
  );
}
