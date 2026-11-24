// @vitest-environment happy-dom
import React from "react";
import { render, screen, cleanup } from "@testing-library/react";
import { describe, expect, it, afterEach } from "vitest";
import {
  registerUIPlugin,
  resetRegistry,
  type ArtifactEditorComponent,
  type ArtifactEditorProps,
} from "@octarq/plugin-sdk";
import { I18nProvider } from "../../i18n";
import { ArtifactCanvas } from "./ArtifactCanvas";
import { useCopilotStore } from "../store";

function EditorBody({ artifactId, content }: ArtifactEditorProps) {
  return (
    <div data-testid="stub-editor" data-artifact={artifactId}>
      editor:{JSON.stringify(content)}
    </div>
  );
}

const StubEditor = EditorBody as unknown as ArtifactEditorComponent;

function seedArtifact(type: string) {
  useCopilotStore.getState().openArtifact({
    id: "art-1",
    type,
    title: "Sample artifact",
    version: 1,
    status: "draft",
    content: { value: "payload" },
  });
}

function renderCanvas() {
  return render(
    <I18nProvider>
      <ArtifactCanvas />
    </I18nProvider>,
  );
}

describe("ArtifactCanvas", () => {
  afterEach(() => {
    resetRegistry();
    useCopilotStore.getState().resetCopilot();
    cleanup();
  });

  it("shows an unavailable panel for an unknown artifact type", () => {
    seedArtifact("plugin.unknown_artifact");
    renderCanvas();

    expect(screen.getByTestId("artifact-editor-unavailable")).toBeDefined();
    expect(screen.getByText("Editor unavailable")).toBeDefined();
    expect(screen.queryByTestId("stub-editor")).toBeNull();
  });

  it("renders a plugin artifact editor registered through the SDK", () => {
    registerUIPlugin({
      name: "test_mail",
      routes: [],
      artifacts: [{ type: "test_mail.email_draft", Component: StubEditor }],
    });
    seedArtifact("test_mail.email_draft");
    renderCanvas();

    expect(screen.getByTestId("stub-editor")).toBeDefined();
    expect(screen.queryByTestId("artifact-editor-unavailable")).toBeNull();
  });

  it("does not resolve an unnamespaced editor type", () => {
    registerUIPlugin({
      name: "test_mail_bare",
      routes: [],
      artifacts: [{ type: "test_mail_bare.email_draft", Component: StubEditor }],
    });
    seedArtifact("email_draft");
    renderCanvas();

    expect(screen.getByTestId("artifact-editor-unavailable")).toBeDefined();
    expect(screen.queryByTestId("stub-editor")).toBeNull();
  });
});
