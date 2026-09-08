import { ReactNode } from "react";
import { Inbox } from "lucide-react";
import { Empty } from "../../ui";
import { useTranslation } from "../../i18n";

export interface TableEmptyProps {
  emptyText?: ReactNode;
  emptyReason?: ReactNode;
  action?: ReactNode;
}

export function TableEmpty({ emptyText, emptyReason, action }: TableEmptyProps) {
  const { t } = useTranslation();

  return (
    <div className="py-12">
      <Empty
        reason={emptyText ?? t("proTable.empty")}
        detail={emptyReason ?? t("proTable.emptyReason")}
        action={action}
      >
        <Inbox className="h-10 w-10 text-foreground/30" />
      </Empty>
    </div>
  );
}
