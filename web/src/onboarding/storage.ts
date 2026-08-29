import { api } from "../api";
import {
  COMPLETED_KEY,
  INITIAL_ANSWERS,
  LEGACY_STORAGE_KEY,
  OnboardingAnswers,
  STORAGE_KEY,
} from "./types";

export function readAnswers(): OnboardingAnswers {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return { ...INITIAL_ANSWERS };
    const parsed = JSON.parse(raw);
    return {
      goal: typeof parsed.goal === "string" ? parsed.goal : "",
      painPoints: Array.isArray(parsed.painPoints) ? parsed.painPoints : [],
      tinderChoices:
        parsed.tinderChoices && typeof parsed.tinderChoices === "object"
          ? parsed.tinderChoices
          : {},
      preferences: Array.isArray(parsed.preferences) ? parsed.preferences : [],
      demoPicks: Array.isArray(parsed.demoPicks) ? parsed.demoPicks : [],
    };
  } catch {
    return { ...INITIAL_ANSWERS };
  }
}

export function writeAnswers(answers: Partial<OnboardingAnswers>): void {
  try {
    const current = readAnswers();
    const merged = { ...current, ...answers };
    localStorage.setItem(STORAGE_KEY, JSON.stringify(merged));
  } catch {
    /* ignore storage quota or private mode errors */
  }
}

export function clearAnswers(): void {
  try {
    localStorage.removeItem(STORAGE_KEY);
  } catch {
    /* ignore */
  }
}

export async function readCompleted(): Promise<boolean> {
  try {
    const settings = await api.getUserSettings();
    if (
      settings &&
      (settings[COMPLETED_KEY] === "true" || settings.onboarding_dismissed === "true")
    ) {
      return true;
    }
  } catch {
    // fallback to local storage
  }

  try {
    const local =
      localStorage.getItem(COMPLETED_KEY) === "true" ||
      localStorage.getItem(LEGACY_STORAGE_KEY) === "true";
    return local;
  } catch {
    return false;
  }
}

export async function writeCompleted(): Promise<void> {
  try {
    localStorage.setItem(COMPLETED_KEY, "true");
    localStorage.setItem(LEGACY_STORAGE_KEY, "true");
  } catch {
    // optimistic local cache fallback
  }

  try {
    await api.updateUserSettings(COMPLETED_KEY, "true");
  } catch {
    // ignore network failure
  }
}
