// @vitest-environment happy-dom
import { describe, expect, it } from "vitest";
import { render } from "@testing-library/react";
import { I18nProvider } from "@octarq/plugin-sdk";
import { CATALOG } from "./catalog";

const resources = {
  en: {
    uiCommon: {
      clickToCopy: "click to copy",
      copied: "copied",
      formErrorStatus: "HTTP {{status}}",
      formErrorRequestId: "request {{requestId}}",
      errStatus500: "Something went wrong on the server.",
      lockedIntro: "This is a {{tier}} feature.",
      notAvailable: "{{feature}} is not available.",
      upgradeTo: "Upgrade to {{tier}}",
      comparePlans: "Compare plans",
    },
    proTable: {
      empty: "No data yet",
      emptyReason: "No matching data found for the current filters",
      errorTitle: "Failed to load data",
      retry: "Retry",
      totalItems: "Total {{total}} items",
      pageOf: "Page {{page}} of {{totalPages}}",
      perPage: "{{size}} / page",
      prevPage: "Previous",
      nextPage: "Next",
    },
  },
};

describe("workbench catalog rendering", () => {
  for (const entry of CATALOG) {
    it(`renders ${entry.name}`, () => {
      const { container } = render(
        <I18nProvider resources={resources}>
          <entry.Component />
        </I18nProvider>,
      );
      expect(container).toBeDefined();
    });
  }
});
