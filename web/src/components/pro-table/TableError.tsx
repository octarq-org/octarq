import { ReactNode } from "react";
import { AlertCircle, RotateCcw } from "lucide-react";
import { Alert, Button, LockedFeature } from "../../ui";
import { useTranslation } from "../../i18n";

export interface TableErrorProps {
  error?: unknown;
  errorText?: ReactNode;
  onRetry?: () => void;
}

export function TableError({ error, errorText, onRetry }: TableErrorProps) {
  const { t } = useTranslation();

  const status =
    error && typeof error === "object" && "status" in error
      ? (error as { status: number }).status
      : error && typeof error === "object" && "response" in error && typeof (error as any).response === "object" && "status" in (error as any).response
        ? (error as any).response.status
        : 0;

  // 402: Payment Required / Locked Feature
  if (status === 402) {
    return (
      <div className="p-6">
        <LockedFeature status={402} feature="pro_table" />
      </div>
    );
  }

  const errorMessage =
    errorText ??
    (error instanceof Error
      ? error.message
      : typeof error === "string"
        ? error
        : t("proTable.errorTitle"));

  return (
    <div className="p-6">
      <Alert
        variant="danger"
        icon={<AlertCircle className="h-4 w-4" />}
        actions={
          onRetry && (
            <Button
              size="sm"
              variant="outline"
              onClick={onRetry}
              className="gap-1.5"
            >
              <RotateCcw className="h-3.5 w-3.5" />
              <span>{t("proTable.retry")}</span>
            </Button>
          )
        }
      >
        <div className="space-y-1">
          <p className="font-semibold text-danger-fg">{t("proTable.errorTitle")}</p>
          <p className="text-xs text-danger-fg/80">{String(errorMessage)}</p>
        </div>
      </Alert>
    </div>
  );
}
