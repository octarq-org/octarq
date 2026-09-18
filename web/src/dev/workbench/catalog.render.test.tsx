// @vitest-environment happy-dom
import { describe, expect, it } from "vitest";
import { render } from "@testing-library/react";
import { I18nProvider } from "@octarq/plugin-sdk";
import { CATALOG } from "./catalog";

/**
 * Renders every catalog entry.
 *
 * The coverage test proves each SDK component HAS an entry; this proves each
 * entry actually renders. A preview that throws on mount — a wrong prop, a
 * provider it needs and does not get — is worse than no preview, because the
 * workbench would be the one place that cannot show the component.
 *
 * The provider is the SDK's own, with a minimal dictionary: that is exactly the
 * contract a plugin has, so anything that renders here renders for a plugin.
 */
const resources = {
  en: {
    uiCommon: {
      clickToCopy: "click to copy",
      copied: "copied",
      formErrorStatus: "HTTP {{status}}",
      formErrorRequestId: "request {{requestId}}",
      errStatus500: "Something went wrong on the server.",
      lockedIntroPre: "This is a ",
      lockedIntroPost: " feature.",
      notAvailable: "{{feature}} is not available.",
      upgradeTo: "Upgrade to {{tier}}",
      comparePlans: "Compare plans",
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
      // Modals/Dialogs render through a portal, so the container may be empty —
      // the assertion that matters is that mounting did not throw.
      expect(container).toBeDefined();
    });
  }
});
