import React from "react";
import { Maximize2, Minimize2, X, Sparkles } from "lucide-react";
import { resolveArtifactEditor } from "@octarq/plugin-sdk";
import { useTranslation } from "../../i18n";
import { useCopilotStore } from "../store";
import type { CardActionHandler } from "../cards/registry";

export interface ArtifactCanvasProps {
  onAction?: CardActionHandler;
}

export function ArtifactCanvas({ onAction = () => {} }: ArtifactCanvasProps) {
  const { t } = useTranslation();
  const { activeArtifactId, artifacts, canvasLayout, setCanvasLayout, closeCanvas } =
    useCopilotStore();

  if (!activeArtifactId || !artifacts[activeArtifactId]) {
    return (
      <div
        data-testid="artifact-canvas-empty"
        className="flex h-full flex-col items-center justify-center p-6 text-center text-xs text-muted-foreground"
      >
        <Sparkles className="h-6 w-6 text-muted-foreground/60 mb-2" />
        <p className="font-semibold text-foreground">{t("copilot.canvasEmptyTitle")}</p>
        <p className="mt-1">{t("copilot.canvasEmptyDesc")}</p>
      </div>
    );
  }

  const artifact = artifacts[activeArtifactId];
  const Editor = resolveArtifactEditor(artifact.type);

  return (
    <div
      data-testid="artifact-canvas"
      className="flex h-full w-full flex-col bg-background overflow-hidden"
    >
      <div className="flex items-center justify-between border-b border-border bg-well px-4 py-2.5 shrink-0">
        <div className="flex items-center gap-2 min-w-0">
          <span className="truncate text-xs font-bold text-foreground" data-testid="artifact-title">
            {artifact.title}
          </span>
          <span className="rounded-full bg-well px-2 py-0.5 font-mono text-[10px] font-medium text-muted-foreground shrink-0">
            {`v${artifact.version || 1}`}
          </span>
          <span className="rounded-full bg-accent-soft px-2 py-0.5 font-mono text-[10px] font-semibold text-accent-fg capitalize shrink-0">
            {artifact.status || "draft"}
          </span>
        </div>

        <div className="flex items-center gap-1 shrink-0">
          {canvasLayout === "split" ? (
            <button
              type="button"
              onClick={() => setCanvasLayout("maximized")}
              aria-label={t("copilot.canvasMaximize")}
              title={t("copilot.canvasMaximize")}
              data-testid="maximize-canvas-btn"
              className="rounded-lg p-1.5 text-muted-foreground hover:bg-surface-hover hover:text-foreground transition-colors"
            >
              <Maximize2 className="h-4 w-4" />
            </button>
          ) : (
            <button
              type="button"
              onClick={() => setCanvasLayout("split")}
              aria-label={t("copilot.canvasRestore")}
              title={t("copilot.canvasRestore")}
              data-testid="restore-split-btn"
              className="rounded-lg p-1.5 text-muted-foreground hover:bg-surface-hover hover:text-foreground transition-colors"
            >
              <Minimize2 className="h-4 w-4" />
            </button>
          )}

          <button
            type="button"
            onClick={closeCanvas}
            aria-label={t("copilot.canvasClose")}
            title={t("copilot.canvasClose")}
            data-testid="close-canvas-btn"
            className="rounded-lg p-1.5 text-muted-foreground hover:bg-surface-hover hover:text-foreground transition-colors"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
      </div>

      <div className="flex-1 overflow-hidden" data-testid="canvas-editor-body">
        {Editor ? (
          <Editor
            artifactId={artifact.id}
            content={artifact.content}
            onAction={onAction}
          />
        ) : (
          <div
            data-testid="artifact-editor-unavailable"
            className="flex h-full flex-col items-center justify-center p-6 text-center"
          >
            <p className="text-xs font-semibold text-foreground">
              {t("copilot.canvasEditorUnavailableTitle")}
            </p>
            <p className="mt-1 max-w-sm text-xs text-muted-foreground">
              {t("copilot.canvasEditorUnavailableDesc")}
            </p>
          </div>
        )}
      </div>
    </div>
  );
}
