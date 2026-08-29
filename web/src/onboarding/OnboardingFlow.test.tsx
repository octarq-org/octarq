// @vitest-environment happy-dom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor, cleanup } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { I18nProvider } from "../i18n";
import { OnboardingFlow } from "./OnboardingFlow";
import { readAnswers, writeCompleted } from "./storage";
import { api } from "../api";
import { COMPLETED_KEY, STORAGE_KEY } from "./types";

vi.mock("../api", () => ({
  api: {
    getUserSettings: vi.fn().mockResolvedValue({}),
    updateUserSettings: vi.fn().mockResolvedValue({ ok: true }),
  },
}));

describe("OnboardingFlow", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it("renders Welcome step initially and advances to Goal step", async () => {
    render(
      <MemoryRouter>
        <I18nProvider>
          <OnboardingFlow />
        </I18nProvider>
      </MemoryRouter>,
    );

    expect(
      screen.getByRole("heading", { level: 1 }),
    ).toBeDefined();

    const startBtn = screen.getByRole("button", { name: /Start Setup|开始搭建/i });
    fireEvent.click(startBtn);

    await waitFor(() => {
      expect(screen.getByText(/What do you want to solve first|你最想先用 Octarq 解决什么/i)).toBeDefined();
    });
  });

  it("saves single Goal selection and multiple Pain selections to storage", async () => {
    render(
      <MemoryRouter>
        <I18nProvider>
          <OnboardingFlow />
        </I18nProvider>
      </MemoryRouter>,
    );

    // Welcome -> Goal
    const startBtn = screen.getByRole("button", { name: /Start Setup|开始搭建/i });
    fireEvent.click(startBtn);

    // Goal step: select Marketing
    await waitFor(() => {
      expect(screen.getByText(/Marketing Links|营销短链/i)).toBeDefined();
    });

    const marketingOption = screen.getByText(/Marketing Links|营销短链/i).closest("button")!;
    fireEvent.click(marketingOption);

    const continueBtn = screen.getByRole("button", { name: /Continue|继续/i }) as HTMLButtonElement;
    expect(continueBtn.disabled).toBe(false);
    fireEvent.click(continueBtn);

    // Verify goal written to storage
    const storedAfterGoal = readAnswers();
    expect(storedAfterGoal.goal).toBe("marketing");

    // Pain step: select multiple
    await waitFor(() => {
      expect(screen.getByText(/What has been holding you back|是什么卡住了你/i)).toBeDefined();
    });

    const painOption1 = screen.getByText(/Bitly/i).closest("button")!;
    fireEvent.click(painOption1);

    const painContinueBtn = screen.getByRole("button", { name: /Continue|继续/i });
    fireEvent.click(painContinueBtn);

    // Verify pain written to storage
    const storedAfterPain = readAnswers();
    expect(storedAfterPain.painPoints).toContain("saas_cost");
  });

  it("completes onboarding and triggers writeCompleted", async () => {
    const onComplete = vi.fn();
    render(
      <MemoryRouter>
        <I18nProvider>
          <OnboardingFlow onComplete={onComplete} />
        </I18nProvider>
      </MemoryRouter>,
    );

    // Advance to Step 2 to reveal Skip button in header
    const startBtn = screen.getByRole("button", { name: /Start Setup|开始搭建/i });
    fireEvent.click(startBtn);

    await waitFor(() => {
      expect(screen.getByRole("button", { name: /Skip|跳过/i })).toBeDefined();
    });

    const skipBtn = screen.getByRole("button", { name: /Skip|跳过/i });
    fireEvent.click(skipBtn);

    await waitFor(() => {
      expect(api.updateUserSettings).toHaveBeenCalledWith(COMPLETED_KEY, "true");
      expect(localStorage.getItem(COMPLETED_KEY)).toBe("true");
      expect(onComplete).toHaveBeenCalled();
    });
  });

  it("writeCompleted persists to api and localStorage directly", async () => {
    await writeCompleted();
    expect(api.updateUserSettings).toHaveBeenCalledWith("onboarding_completed", "true");
    expect(localStorage.getItem("onboarding_completed")).toBe("true");
  });
});
