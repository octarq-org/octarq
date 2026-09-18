import { ReactNode } from "react";
import { useTranslation } from "../../i18n";
import { Empty } from "./empty";

// Presentational "no rows" state for a table. Moved here from the host app's
// pro-table: it depends on nothing tanstack, and a plugin rendering a table has
// no way to reach it otherwise. Copy resolves from the host dictionary
// (proTable.*), the same host-fed contract LockedFeature and FormError use.
export interface TableEmptyProps {
  emptyText?: ReactNode;
  emptyReason?: ReactNode;
  action?: ReactNode;
  icon?: ReactNode;
}

export function TableEmpty({ emptyText, emptyReason, action, icon }: TableEmptyProps) {
  const { t } = useTranslation();

  return (
    <div className="py-12">
      <Empty
        reason={emptyText ?? t("proTable.empty")}
        detail={emptyReason ?? t("proTable.emptyReason")}
        action={action}
      >
        {icon ?? <InboxGlyph />}
      </Empty>
    </div>
  );
}

// Inline so the package stays free of an icon-library dependency — same reason
// LockedFeature draws its own key glyph.
function InboxGlyph() {
  return (
    <svg
      className="h-10 w-10 text-foreground/30"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.75}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M22 12h-6l-2 3h-4l-2-3H2" />
      <path d="M5.45 5.11 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z" />
    </svg>
  );
}
