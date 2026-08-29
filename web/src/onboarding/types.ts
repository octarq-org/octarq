export const STORAGE_KEY = "octarq:onboarding:answers";
export const COMPLETED_KEY = "onboarding_completed";
export const LEGACY_STORAGE_KEY = "octarq:onboarding:completed";

export interface OnboardingAnswers {
  goal: string;
  painPoints: string[];
  tinderChoices: Record<string, "agree" | "skip">;
  preferences: string[];
  demoPicks: string[];
}

export const INITIAL_ANSWERS: OnboardingAnswers = {
  goal: "",
  painPoints: [],
  tinderChoices: {},
  preferences: [],
  demoPicks: [],
};

export const TOTAL_STEPS = 12;

export interface DemoLinkItem {
  id: string;
  slug: string;
  domain: string;
  titleKey: string;
  category: string;
  utmSource: string;
  utmMedium: string;
  utmCampaign: string;
}

export const DEMO_LINKS: DemoLinkItem[] = [
  {
    id: "launch-2026",
    slug: "launch-2026",
    domain: "go.yourdomain.co",
    titleKey: "onboarding.demoLaunchTitle",
    category: "slugs",
    utmSource: "octarq",
    utmMedium: "onboarding",
    utmCampaign: "launch-2026",
  },
  {
    id: "discord",
    slug: "discord",
    domain: "go.yourdomain.co",
    titleKey: "onboarding.demoDiscordTitle",
    category: "analytics",
    utmSource: "octarq",
    utmMedium: "social",
    utmCampaign: "community",
  },
  {
    id: "deck",
    slug: "deck",
    domain: "go.yourdomain.co",
    titleKey: "onboarding.demoDeckTitle",
    category: "utm",
    utmSource: "octarq",
    utmMedium: "presentation",
    utmCampaign: "investors",
  },
  {
    id: "newsletter",
    slug: "newsletter",
    domain: "go.yourdomain.co",
    titleKey: "onboarding.demoNewsletterTitle",
    category: "analytics",
    utmSource: "octarq",
    utmMedium: "email",
    utmCampaign: "weekly",
  },
  {
    id: "github",
    slug: "github",
    domain: "go.yourdomain.co",
    titleKey: "onboarding.demoGithubTitle",
    category: "wildcard",
    utmSource: "octarq",
    utmMedium: "repo",
    utmCampaign: "oss",
  },
  {
    id: "demo",
    slug: "demo",
    domain: "go.yourdomain.co",
    titleKey: "onboarding.demoDemoTitle",
    category: "webhooks",
    utmSource: "octarq",
    utmMedium: "interactive",
    utmCampaign: "showcase",
  },
];
