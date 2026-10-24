/**
 * StoryboardCanvas: Interactive visual scene cards, camera/lighting tags, and voiceover script editor.
 *
 * All user copy strictly conforms to ASD-STE100 (<= 20 words/sentence, active voice).
 */
import React, { useState } from "react";
import { Button } from "@octarq/plugin-sdk";
import { Video, Plus, Trash2, Camera, Sparkles, Mic, Download } from "lucide-react";
import { useTranslation } from "../../../i18n";
import type { StoryboardScene } from "../../cards/ScriptStoryboardCard";
import type { CardActionHandler } from "../../cards/registry";
import { useCopilotStore } from "../../store";

export interface StoryboardCanvasProps {
  artifactId: string;
  content: {
    campaign_title?: string;
    target_seconds?: number;
    aspect_ratio?: string;
    scenes?: StoryboardScene[];
  };
  onAction: CardActionHandler;
}

export function StoryboardCanvas({ artifactId, content, onAction }: StoryboardCanvasProps) {
  const { t } = useTranslation();
  const [scenes, setScenes] = useState<StoryboardScene[]>(() => {
    if (content.scenes && content.scenes.length > 0) return content.scenes;
    return [
      {
        scene_number: 1,
        duration_sec: 12,
        shot_type: "Wide Aerial",
        camera_angle: "High Angle Crane",
        lighting: "Moody Blue Cyberpunk",
        visual_prompt: "Modern engineering leader sitting at a multi-monitor desk overwhelmed with notifications.",
        voiceover: "Engineering leaders manage hundreds of systems, alerts, and operational decisions every day.",
      },
      {
        scene_number: 2,
        duration_sec: 15,
        shot_type: "Macro Close-Up",
        camera_angle: "Eye Level Slow Dolly",
        lighting: "Warm Golden Hour Key",
        visual_prompt: "Terminal window executing autonomous verification workflows and generating visual action cards.",
        voiceover: "Octarq Pro gives you an AI executive assistant running on your private infrastructure.",
      },
      {
        scene_number: 3,
        duration_sec: 13,
        shot_type: "Medium Two-Shot",
        camera_angle: "Slight Low Angle",
        lighting: "Clean Studio Daylight",
        visual_prompt: "Operator approving batch invoice payment with one click on the interactive side-canvas.",
        voiceover: "Reclaim your focus. Turn weeks of tedious operations into seconds of confident leadership.",
      },
    ];
  });

  const handleUpdateScene = (index: number, field: keyof StoryboardScene, val: unknown) => {
    const next = [...scenes];
    next[index] = { ...next[index], [field]: val };
    setScenes(next);

    useCopilotStore.getState().updateArtifactContent(artifactId, (prev: any) => ({
      ...prev,
      scenes: next,
    }));
  };

  const handleAddScene = () => {
    const newScene: StoryboardScene = {
      scene_number: scenes.length + 1,
      duration_sec: 10,
      shot_type: "Medium Shot",
      camera_angle: "Eye Level",
      lighting: "Soft Fill Studio",
      visual_prompt: "Call to action screen displaying Octarq Pro terminal command and download link.",
      voiceover: "Get started today with Octarq Pro. Single binary, zero cloud leak.",
    };
    const next = [...scenes, newScene];
    setScenes(next);
  };

  const handleDeleteScene = (index: number) => {
    if (scenes.length <= 1) return;
    const next = scenes.filter((_, i) => i !== index).map((s, i) => ({ ...s, scene_number: i + 1 }));
    setScenes(next);
  };

  const totalDuration = scenes.reduce((sum, s) => sum + (s.duration_sec || 0), 0);

  return (
    <div className="flex h-full flex-col space-y-3 p-4 overflow-y-auto" data-testid="storyboard-canvas">
      {/* Header */}
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border/40 pb-3">
        <div>
          <h3 className="text-base font-bold text-foreground">
            {`${content.campaign_title || "Launch Campaign"} ${t("copilot.scriptCanvasTitle", "— Storyboard Studio")}`}
          </h3>
          <p className="text-xs text-muted-foreground">
            {t("copilot.scriptCanvasDesc", "Direct visual scene descriptions, camera framing, lighting, and voiceover scripts.")}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <span className="rounded-full bg-well px-2.5 py-1 text-xs font-mono font-medium text-foreground">
            {`${scenes.length} ${t("copilot.scriptScenes", "Scenes ·")} ${totalDuration}s ${t("copilot.scriptTotal", "Total")}`}
          </span>
          <Button
            size="sm"
            variant="secondary"
            onClick={handleAddScene}
            data-testid="add-scene-btn"
            className="h-8 text-xs flex items-center gap-1"
          >
            <Plus className="h-3.5 w-3.5" />
            <span>{t("copilot.scriptAddScene", "Add Scene")}</span>
          </Button>
        </div>
      </div>

      {/* Grid of Scene Cards */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
        {scenes.map((scene, idx) => {
          const num = scene.scene_number || scene.scene_no || idx + 1;

          return (
            <div
              key={num}
              className="rounded-xl border border-border bg-card p-3 space-y-2 text-xs flex flex-col justify-between shadow-xs"
              data-testid="scene-card"
            >
              <div className="space-y-2">
                <div className="flex items-center justify-between border-b border-border/40 pb-1.5">
                  <div className="flex items-center gap-1.5 font-bold text-foreground">
                    <Video className="h-3.5 w-3.5 text-primary" />
                    <span>{`${t("copilot.scriptScenePrefix", "Scene #")}${num}`}</span>
                  </div>
                  <div className="flex items-center gap-1">
                    <input
                      type="number"
                      aria-label={t("copilot.scriptDurationAria", "Scene duration in seconds")}
                      min={1}
                      max={120}
                      value={scene.duration_sec}
                      onChange={(e) => handleUpdateScene(idx, "duration_sec", parseInt(e.target.value) || 5)}
                      className="w-12 rounded border border-border bg-background px-1 py-0.5 font-mono text-[11px] text-center focus:border-primary focus:outline-none"
                    />
                    <span className="text-[11px] text-muted-foreground">sec</span>
                    <button
                      onClick={() => handleDeleteScene(idx)}
                      disabled={scenes.length <= 1}
                      className="rounded p-1 text-muted-foreground hover:bg-destructive/10 hover:text-destructive disabled:opacity-30 ml-1"
                      aria-label={t("copilot.scriptDeleteSceneAria", "Delete scene")}
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </button>
                  </div>
                </div>

                {/* Framing & Lighting Tags */}
                <div className="grid grid-cols-2 gap-1.5">
                  <div>
                    <label className="text-[10px] text-muted-foreground uppercase font-semibold">{t("copilot.scriptCamera", "Camera")}</label>
                    <input
                      type="text"
                      aria-label={t("copilot.scriptCameraAria", "Camera direction")}
                      value={scene.shot_type || scene.camera_angle || ""}
                      onChange={(e) => handleUpdateScene(idx, "shot_type", e.target.value)}
                      placeholder="e.g. Wide Aerial"
                      className="w-full rounded border border-border bg-background px-1.5 py-1 text-[11px] focus:border-primary focus:outline-none"
                    />
                  </div>
                  <div>
                    <label className="text-[10px] text-muted-foreground uppercase font-semibold">{t("copilot.scriptLighting", "Lighting")}</label>
                    <input
                      type="text"
                      aria-label={t("copilot.scriptLightingAria", "Lighting style")}
                      value={scene.lighting || ""}
                      onChange={(e) => handleUpdateScene(idx, "lighting", e.target.value)}
                      placeholder="e.g. Cyberpunk"
                      className="w-full rounded border border-border bg-background px-1.5 py-1 text-[11px] focus:border-primary focus:outline-none"
                    />
                  </div>
                </div>

                {/* Visual Prompt */}
                <div>
                  <label className="text-[10px] text-muted-foreground uppercase font-semibold flex items-center gap-1">
                    <Sparkles className="h-3 w-3 text-primary" />
                    <span>{t("copilot.scriptVisualPrompt", "Visual Prompt")}</span>
                  </label>
                  <textarea
                    rows={2}
                    aria-label={t("copilot.scriptVisualPromptAria", "Visual scene prompt")}
                    value={scene.visual_description || scene.visual_prompt || ""}
                    onChange={(e) => handleUpdateScene(idx, "visual_description", e.target.value)}
                    className="w-full rounded border border-border bg-background p-1.5 text-xs text-foreground resize-none focus:border-primary focus:outline-none"
                  />
                </div>

                {/* Audio Voiceover */}
                <div>
                  <label className="text-[10px] text-muted-foreground uppercase font-semibold flex items-center gap-1">
                    <Mic className="h-3 w-3 text-muted-foreground" />
                    <span>{t("copilot.scriptVoiceover", "Voiceover Narration")}</span>
                  </label>
                  <textarea
                    rows={2}
                    aria-label={t("copilot.scriptVoiceoverAria", "Voiceover narration")}
                    value={scene.voiceover || scene.audio_voiceover || ""}
                    onChange={(e) => handleUpdateScene(idx, "voiceover", e.target.value)}
                    className="w-full rounded border border-border bg-background p-1.5 text-xs italic text-foreground/90 resize-none focus:border-primary focus:outline-none"
                  />
                </div>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
