import { Badge, Button, TD, TR } from "@octarq/plugin-sdk";
import { Link } from "../api";
import { Copy, Check, Archive, Trash2, QrCode, ExternalLink, Lock, Bot, Clock, Tag, BarChart3, Edit3 } from "lucide-react";
import { useTranslation } from "../../../i18n";

export interface LinkTableRowProps {
  link: Link;
  isCopied: boolean;
  canDeleteLink: boolean;
  onCopy: (l: Link) => void;
  onAnalytics: (l: Link) => void;
  onQr: (l: Link) => void;
  onEdit: (l: Link) => void;
  onToggleArchive: (l: Link) => void;
  onDelete: (l: Link) => void;
  linkURL: (l: Link) => string;
}

export function LinkTableRow({
  link: l,
  isCopied,
  canDeleteLink,
  onCopy,
  onAnalytics,
  onQr,
  onEdit,
  onToggleArchive,
  onDelete,
  linkURL,
}: LinkTableRowProps) {
  const { t } = useTranslation();
  const tagList = (l.tags || "").split(",").map((tag) => tag.trim()).filter(Boolean);
  const displayTitle = l.title || l.slug;

  return (
    <TR key={l.id} className="hover:bg-foreground/[0.02] transition-colors group">
      {/* Title, Short Slug & Host */}
      <TD>
        <div className="flex flex-col gap-1">
          <div className="text-sm font-medium text-foreground truncate max-w-[260px]" title={displayTitle}>
            {displayTitle}
          </div>
          <div className="flex items-center gap-1.5">
            <span className="font-mono text-xs text-foreground/50 truncate max-w-[200px]">
              {l.host ? `${l.host}/` : "/"}
              <span>{l.slug}</span>
            </span>
            <button
              type="button"
              onClick={() => onCopy(l)}
              title={t("links.copyLink")}
              aria-label={t("links.copyLink")}
              className="p-0.5 text-foreground/40 hover:text-foreground rounded transition-colors cursor-pointer"
            >
              {isCopied ? (
                <Check className="h-3.5 w-3.5 text-success-fg" />
              ) : (
                <Copy className="h-3.5 w-3.5" />
              )}
            </button>
            <a
              href={linkURL(l)}
              target="_blank"
              rel="noreferrer"
              title={t("links.openLink")}
              aria-label={t("links.openLink")}
              className="p-0.5 text-foreground/40 hover:text-accent-fg rounded transition-colors"
            >
              <ExternalLink className="h-3.5 w-3.5" />
            </a>
          </div>
          {tagList.length > 0 && (
            <div className="flex items-center gap-1 flex-wrap mt-0.5">
              {tagList.map((tag) => (
                <span
                  key={tag}
                  title={tag}
                  aria-label={tag}
                  className="inline-flex items-center gap-0.5 text-[10px] text-foreground/70 bg-foreground/[0.06] px-1.5 py-0.5 rounded font-mono"
                >
                  <Tag className="h-2.5 w-2.5 text-foreground/50" />
                  <span className="truncate max-w-[120px]">{tag}</span>
                </span>
              ))}
            </div>
          )}
        </div>
      </TD>

      {/* Destination Target URL & Badges */}
      <TD>
        <div className="flex flex-col gap-1">
          <div className="font-mono text-xs text-foreground/75 truncate max-w-[320px]" title={l.target}>
            {l.target}
          </div>
          <div className="flex items-center gap-1.5 flex-wrap">
            {l.hasPassword && (
              <span
                className="inline-flex items-center gap-1 text-[10px] text-warning-fg bg-warning-bg px-1.5 py-0.5 rounded font-mono"
                title={`${t("links.accessProtectionPassword")}${l.password ? `: ${l.password}` : ""}`}
              >
                <Lock className="h-2.5 w-2.5" />
                {l.password && <span>{l.password}</span>}
              </span>
            )}
            {l.routingRules?.length ? (
              <span className="inline-flex items-center gap-0.5 text-[10px] text-accent-fg bg-accent-soft px-1.5 py-0.5 rounded" title={t("links.routingRules")}>
                <Bot className="h-2.5 w-2.5" />
                {l.routingRules.length}
              </span>
            ) : null}
            {l.expiresAt && (
              <span className="inline-flex items-center gap-0.5 text-[10px] text-foreground/50 bg-foreground/[0.05] px-1.5 py-0.5 rounded" title={new Date(l.expiresAt).toLocaleString()}>
                <Clock className="h-2.5 w-2.5" />
              </span>
            )}
          </div>
        </div>
      </TD>

      {/* Clicks */}
      <TD className="text-center font-mono">
        <Badge tone="neutral" className="text-xs font-semibold">
          {l.clicks || 0}
        </Badge>
      </TD>

      {/* Status */}
      <TD className="text-center">
        <Badge tone={l.archived ? "neutral" : "info"} className="text-[10px]">
          {l.archived ? t("links.archived") : t("links.active")}
        </Badge>
      </TD>

      {/* Actions */}
      <TD className="text-right">
        <div className="flex items-center justify-end gap-1.5">
          <Button
            variant="ghost"
            className="p-1.5 text-xs text-foreground/60 hover:text-accent-fg"
            title={t("links.tabAnalytics")}
            onClick={() => onAnalytics(l)}
          >
            <BarChart3 className="h-4 w-4" />
          </Button>
          <Button
            variant="ghost"
            className="p-1.5 text-xs text-foreground/60 hover:text-foreground"
            title={t("links.tabQr")}
            onClick={() => onQr(l)}
          >
            <QrCode className="h-4 w-4" />
          </Button>
          <Button
            variant="ghost"
            className="p-1.5 text-xs text-foreground/60 hover:text-foreground"
            title={t("links.editLink")}
            onClick={() => onEdit(l)}
          >
            <Edit3 className="h-4 w-4" />
          </Button>
          <Button
            variant="ghost"
            className="p-1.5 text-xs text-foreground/50 hover:text-foreground"
            title={l.archived ? t("links.unarchive") : t("links.archive")}
            onClick={() => onToggleArchive(l)}
          >
            <Archive className="h-4 w-4" />
          </Button>
          {canDeleteLink && (
            <Button
              variant="ghost"
              className="p-1.5 text-xs text-danger-fg/70 hover:text-danger-fg"
              title={t("links.delete")}
              onClick={() => onDelete(l)}
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          )}
        </div>
      </TD>
    </TR>
  );
}
