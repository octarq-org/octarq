/**
 * ScriptStoryboardCard: Marketing video script, shot list, and visual storyboard card.
 *
 * All user copy strictly conforms to ASD-STE100 (<= 20 words/sentence, active voice).
 */
import React from "react";
import { Button } from "@octarq/plugin-sdk";
import { Clapperboard, Video, ExternalLink, Download } from "lucide-react";
import { useTranslation } from "../../i18n";
import type { ToolCardProps } from "./registry";
import { CardShell, type CardState } from "./CardShell";
import { useCopilotStore, type Artifact } from "../store";

export interface StoryboardScene {
  scene_number?: number;
  scene_no?: number;
  duration_sec: number;
  visual_prompt?: string;
  visual_description?: string;
  shot_type?: string;
  camera_angle?: string;
  lighting?: string;
  voiceover?: string;
  audio_voiceover?: string;
  on_screen_text?: string;
}

export interface ScriptStoryboardArgs {
  campaign_title?: string;
  project_name?: string;
  target_seconds?: number;
  target_duration_seconds?: number;
  aspect_ratio?: "16:9" | "9:16" | "1:1";
  scenes?: StoryboardScene[];
  brief_id?: string;
  state?: CardState;
  errorDetail?: string;
  forbiddenReason?: string;
}

export function ScriptStoryboardCard({
  args,
  onAction,
  state: propState,
  toolName = "script_storyboard",
}: ToolCardProps & { state?: CardState }) {
  const { t } = useTranslation();
  const data = (args ?? {}) as ScriptStoryboardArgs;
  const effectiveState = propState ?? data.state;
  const errorDetail = data.errorDetail;
  const forbiddenReason = data.forbiddenReason;

  const title = data.campaign_title ?? data.project_name ?? "Octarq AI Launch Campaign";
  const durationSec = data.target_seconds ?? data.target_duration_seconds ?? 60;
  const aspectRatio = data.aspect_ratio ?? "16:9";
  const scenes = Array.isArray(data.scenes) ? data.scenes : [];

  const artifactId = `artifact-storyboard-${title.replace(/\s+/g, "-").toLowerCase()}`;

  const handleOpenCanvas = () => {
    const artifact: Artifact = {
      id: artifactId,
      type: "storyboard",
      title: `${title} — Storyboard Studio`,
      version: 1,
      status: "draft",
      content: {
        campaign_title: title,
        target_seconds: durationSec,
        aspect_ratio: aspectRatio,
        scenes,
      },
      createdAt: new Date().toISOString(),
    };

    useCopilotStore.getState().openArtifact(artifact);
    void onAction({
      type: "open_canvas",
      toolName,
      artifactId,
      payload: artifact.content,
    });
  };

  const handleExportShotList = () => {
    void onAction({
      type: "custom",
      toolName: "export_shot_list",
      payload: {
        campaign_title: title,
        scene_count: scenes.length,
      },
    });
  };

  const firstScene = scenes[0];

  return (
    <CardShell
      title={t("copilot.scriptCardTitle", "Marketing Video Script & Storyboard")}
      state={effectiveState}
      errorDetail={errorDetail}
      forbiddenReason={forbiddenReason}
    >
      <div className="space-y-2.5">
        <div className="flex items-center justify-between border-b border-border/40 pb-1.5">
          <div>
            <div className="font-semibold text-foreground text-xs" data-testid="campaign-title">
              {title}
            </div>
            <div className="text-[11px] text-muted-foreground">
              {`${scenes.length} ${t("copilot.scriptScenesPlanned", "scenes planned ·")} ${durationSec}s ${t("copilot.scriptTotalDuration", "total duration.")}`}
            </div>
          </div>
          <div className="flex items-center gap-1">
            <span className="rounded-full bg-well px-2 py-0.5 font-mono text-[10px] font-medium text-foreground">
              {aspectRatio}
            </span>
            <span className="rounded-full bg-primary/10 px-2 py-0.5 text-[10px] font-medium text-primary">
              {`${durationSec}s`}
            </span>
          </div>
        </div>

        {firstScene ? (
          <div className="rounded-lg border border-border/50 bg-background/50 p-2 space-y-1.5" data-testid="scene-preview">
            <div className="flex items-center justify-between text-[11px]">
              <span className="font-semibold text-foreground flex items-center gap-1">
                <Video className="h-3 w-3 text-primary" />
                <span>{`${t("copilot.scriptScenePrefix", "Scene #")}${firstScene.scene_number || firstScene.scene_no || 1}`}</span>
              </span>
              <span className="font-mono text-muted-foreground">{`${firstScene.duration_sec}s`}</span>
            </div>
            <div className="text-xs text-muted-foreground line-clamp-2">
              {firstScene.visual_description || firstScene.visual_prompt || "Cinematic opening sequence."}
            </div>
            {(firstScene.voiceover || firstScene.audio_voiceover) && (
              <div className="text-[11px] text-foreground/80 italic line-clamp-1 border-t border-border/30 pt-1">
                "{firstScene.voiceover || firstScene.audio_voiceover}"
              </div>
            )}
          </div>
        ) : (
          <div className="text-xs text-muted-foreground py-2 text-center" data-testid="empty-scenes">
            {t("copilot.scriptNoScenes", "No storyboard scenes to display.")}
          </div>
        )}

        <div className="flex items-center justify-between gap-2 pt-1 border-t border-border/40">
          <Button
            size="sm"
            variant="secondary"
            onClick={handleOpenCanvas}
            data-testid="open-canvas-btn"
            className="h-7 text-xs flex items-center gap-1"
          >
            <ExternalLink className="h-3 w-3" />
            <span>{t("copilot.cardOpenInCanvas", "Open in Canvas")}</span>
          </Button>

          <Button
            size="sm"
            variant="secondary"
            onClick={handleExportShotList}
            data-testid="export-shots-btn"
            className="h-7 text-xs flex items-center gap-1"
          >
            <Download className="h-3 w-3" />
            <span>{t("copilot.cardExportShotList", "Export Shot List")}</span>
          </Button>
        </div>
      </div>
    </CardShell>
  );
}
