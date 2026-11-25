// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { ToastProvider } from "@octarq/plugin-sdk";
import { api, type CronJobInfo } from "../../api";
import { I18nProvider } from "../../i18n";
import { ScheduledJobsSettings } from "./jobs";

const jobs: CronJobInfo[] = [
  {
    name: "pt.tick",
    spec: "*/5 * * * *",
    defaultSpec: "*/5 * * * *",
    enabled: true,
    lastRun: new Date(Date.now() - 60_000).toISOString(),
    nextRun: new Date(Date.now() + 60_000).toISOString(),
    status: "idle",
    lastErr: "",
  },
  {
    name: "session_cleanup",
    spec: "0 * * * *",
    defaultSpec: "0 * * * *",
    enabled: false,
    lastRun: "",
    nextRun: "",
    status: "error",
    lastErr: "cleanup failed",
  },
];

function renderPage() {
  return render(
    <I18nProvider>
      <ToastProvider>
        <ScheduledJobsSettings />
      </ToastProvider>
    </I18nProvider>,
  );
}

beforeEach(() => {
  vi.spyOn(api, "cronJobs").mockResolvedValue(jobs);
  vi.spyOn(api, "runCronJob").mockResolvedValue({ name: "pt.tick", started: true });
  vi.spyOn(api, "updateCronJob").mockResolvedValue({
    name: "pt.tick",
    spec: "*/5 * * * *",
    defaultSpec: "*/5 * * * *",
    enabled: true,
  });
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("ScheduledJobsSettings", () => {
  it("lists registered tasks with their state", async () => {
    renderPage();
    expect(await screen.findByText("pt.tick")).toBeTruthy();
    expect(screen.getByText("session_cleanup")).toBeTruthy();
    expect(screen.getByText("cleanup failed")).toBeTruthy();
  });

  it("runs a task and reports when it is busy", async () => {
    renderPage();
    const runButtons = await screen.findAllByRole("button", { name: "Run now" });
    fireEvent.click(runButtons[0]);
    await waitFor(() => expect(api.runCronJob).toHaveBeenCalledWith("pt.tick"));
  });

  it("saves a changed schedule", async () => {
    renderPage();
    const input = await screen.findByDisplayValue("*/5 * * * *");
    fireEvent.change(input, { target: { value: "0 3 * * *" } });
    const saveButtons = screen.getAllByRole("button", { name: "Save" });
    fireEvent.click(saveButtons[0]);
    await waitFor(() => expect(api.updateCronJob).toHaveBeenCalledWith("pt.tick", { spec: "0 3 * * *" }));
  });

  it("pauses a task", async () => {
    renderPage();
    const toggle = await screen.findByRole("switch", { name: "Enabled pt.tick" });
    fireEvent.click(toggle);
    await waitFor(() => expect(api.updateCronJob).toHaveBeenCalledWith("pt.tick", { enabled: false }));
  });
});
